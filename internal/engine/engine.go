package engine

import (
	"context"
	"fmt"
	"sync"

	"qac/internal/agent"
	"qac/internal/llmclient"
	"qac/internal/probe"
	"qac/internal/store"
)

// maxConcurrentRuns caps the number of probe goroutines that may run
// at once. Sized for the iter-4b single-developer machine. iter-4c+
// can tune via construction-time option.
const maxConcurrentRuns = 8

// Engine drives runs forward by spawning a goroutine per Kick that
// loads the template, finds the next runnable probe step, runs the
// probe, persists evidence, and emits StepStarted/StepCompleted or
// StepFailed events. Goroutine-safe.
type Engine struct {
	store   *store.Store
	probes  *probe.Registry
	clients probe.Clients

	agents       *agent.Registry
	agentClients agent.Clients

	mu       sync.Mutex
	inflight map[string]chan struct{}      // runID → done channel
	cancels  map[string]context.CancelFunc // runID → cancel func for the runner ctx

	wg  sync.WaitGroup // tracks in-flight runners for Shutdown
	sem chan struct{}   // counting semaphore: cap = maxConcurrentRuns
}

// New returns an Engine ready to Kick. Clients carries the backend ports
// the engine threads through to Probe.Run.
func New(s *store.Store, probes *probe.Registry, clients probe.Clients) *Engine {
	return &Engine{
		store:    s,
		probes:   probes,
		clients:  clients,
		inflight: map[string]chan struct{}{},
		cancels:  map[string]context.CancelFunc{},
		sem:      make(chan struct{}, maxConcurrentRuns),
	}
}

// Probes returns metadata for every registered probe, for the /api/probes
// surface. The registry is the source of truth for which probes exist.
func (e *Engine) Probes() []probe.Info { return e.probes.List() }

// RegisterAgents wires the agent registry and the ports agents need. Called
// once at server boot after New. Left unset in tests that don't exercise
// ai_assess steps.
func (e *Engine) RegisterAgents(reg *agent.Registry, clients agent.Clients) {
	e.agents = reg
	e.agentClients = clients
}

// Agents returns metadata for every registered agent, for /api/agents. Empty
// when no agents are registered.
func (e *Engine) Agents() []agent.Info {
	if e.agents == nil {
		return nil
	}
	return e.agents.List()
}

// AgentLLMInfo reports the active agent LLM backend (model + live), for
// /api/agents. Reports a "none" backend when agents are unwired.
func (e *Engine) AgentLLMInfo() llmclient.Info {
	if e.agentClients.LLM == nil {
		return llmclient.Info{Model: "none", Live: false}
	}
	return e.agentClients.LLM.Info()
}

// Kick spawns a runner goroutine for runID. Idempotent: if a runner for
// runID is already in-flight, Kick is a no-op. The done channel is
// registered synchronously before the goroutine spawns, so a Wait call
// immediately after Kick returns is race-safe.
func (e *Engine) Kick(ctx context.Context, runID string) {
	e.mu.Lock()
	if _, ok := e.inflight[runID]; ok {
		e.mu.Unlock()
		return
	}
	done := make(chan struct{})
	// Runner context is rooted at Background (not the request ctx) so a
	// timed-out POST doesn't kill an in-flight probe — but it IS cancellable
	// so Cancel(runID) can stop the walk on operator request.
	runCtx, cancel := context.WithCancel(context.Background())
	e.inflight[runID] = done
	e.cancels[runID] = cancel
	e.wg.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.wg.Done()
		defer func() {
			e.mu.Lock()
			delete(e.inflight, runID)
			delete(e.cancels, runID)
			e.mu.Unlock()
			cancel()
			close(done)
		}()

		// Block here until a concurrency slot is available.
		e.sem <- struct{}{}
		defer func() { <-e.sem }()

		e.runOnce(runCtx, runID)
	}()
}

// Cancel stops the in-flight runner for runID, if any, by cancelling its
// context — the active probe returns promptly and the walk halts without
// emitting further events. A no-op when no runner is in-flight (e.g. the
// run was already idle, or the process restarted); callers still record the
// terminal RunCancelled event so the run's status reflects the cancellation.
func (e *Engine) Cancel(runID string) {
	e.mu.Lock()
	cancel := e.cancels[runID]
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Wait returns a channel that closes when the runner for runID finishes.
// If no runner is in-flight (because it already finished, or was never
// kicked), Wait returns a pre-closed channel — so a caller polling
// "did the engine touch this run yet?" can rely on a non-blocking read.
func (e *Engine) Wait(runID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.inflight[runID]; ok {
		return ch
	}
	preClosed := make(chan struct{})
	close(preClosed)
	return preClosed
}

// Shutdown blocks until all in-flight runners finish or ctx fires.
// Returns ctx.Err() on deadline expiry; nil on clean drain.
func (e *Engine) Shutdown(ctx context.Context) error {
	allDone := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(allDone)
	}()
	select {
	case <-allDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("engine.Shutdown: %w", ctx.Err())
	}
}
