package vm

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// warnOnce ensures the unauthed-fallback warning is emitted at most once
// per process, even when multiple Clients are built.
var (
	warnOnce     sync.Once
	warnWriterMu sync.Mutex
	warnWriterOv io.Writer
)

// maybeWarnUnauthed emits the one-time unauthed-fallback warning.
func maybeWarnUnauthed(reason string) {
	warnOnce.Do(func() {
		if w := warnWriter(); w != nil {
			fmt.Fprintf(w, "vm: no VMauth credentials (%s); falling back to unauthed vmui endpoints\n", reason)
		}
	})
}

func warnWriter() io.Writer {
	warnWriterMu.Lock()
	defer warnWriterMu.Unlock()
	if warnWriterOv != nil {
		return warnWriterOv
	}
	return os.Stderr
}

// SetWarnWriter overrides the unauthed-warning destination (io.Discard to
// suppress, nil to restore the default of os.Stderr). Returns the previous
// writer. Intended for tests.
func SetWarnWriter(w io.Writer) io.Writer {
	warnWriterMu.Lock()
	defer warnWriterMu.Unlock()
	prev := warnWriterOv
	warnWriterOv = w
	return prev
}

// ResetWarnOnce resets the once-gate so tests can observe the warning
// across subtests. Intended for tests.
func ResetWarnOnce() { warnOnce = sync.Once{} }
