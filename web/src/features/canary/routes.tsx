import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import RunsList from '@/features/canary/RunsList';
import RunDetail from '@/features/canary/RunDetail';
import NewRunModal from '@/features/canary/NewRunModal';
import { Loading, ErrorPane } from '@/features/canary/Loading';
import { useCanaryRuns } from '@/features/canary/useCanaryRuns';
import { useCanaryRun } from '@/features/canary/useCanaryRun';
import { useRunFacts } from '@/features/canary/useRunFacts';
import { useRunAlertEvidence } from '@/features/canary/useRunAlertEvidence';
import { useRunAssessment } from '@/features/canary/useRunAssessment';
import { useCanaryTemplate } from '@/features/canary/useCanaryTemplate';
import { useCreateCanaryRun } from '@/features/canary/useCreateCanaryRun';
import type { CreateCanaryInput } from '@/features/canary/useCreateCanaryRun';
import { useRunAction } from '@/features/canary/useRunAction';
import { useCancelRun } from '@/features/canary/useCancelRun';
import { useRlccWorkflows } from '@/features/runs/useRlccWorkflows';
import { stepStatesFor } from '@/features/canary/adapters';
import { ApiException } from '@/lib/api';
import { toast } from 'sonner';

const msg = (e: unknown) => (e instanceof ApiException ? e.message : 'Unexpected error');

// The RLCC action handler the firmware canary's l11_fielddiag step drives.
// A workflow without this handler has no L11 step for the canary to verify.
const CANARY_REQUIRED_HANDLER = 'l11-fielddiag';

export function CanaryRoute() {
  const navigate = useNavigate();
  const [modalOpen, setModalOpen] = useState(false);
  const runs = useCanaryRuns();
  const create = useCreateCanaryRun();
  const workflowsQuery = useRlccWorkflows();
  // Only workflows that actually contain the L11 field-diag step can drive
  // the firmware canary — the l11_fielddiag run step asserts against that
  // action. Workflows without it (CDU bringup, power on/off, RMA, …) would
  // always fail the run, so we hide them from the picker rather than let an
  // operator pick a dud. See the l11_fielddiag step in the canary template.
  const workflows = (workflowsQuery.data?.workflows ?? [])
    .filter((w) => w.handlers?.includes(CANARY_REQUIRED_HANDLER))
    .map((w) => w.name);
  if (runs.isLoading) return <Loading label="Loading runs…" />;
  if (runs.isError) return <ErrorPane message={msg(runs.error)} />;
  return (
    <>
      <RunsList
        runs={runs.data ?? []}
        onOpenRun={(id: string) => navigate(`/runs/${id}`)}
        onNewRun={() => setModalOpen(true)}
        density="compact"
      />
      <NewRunModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        workflows={workflows}
        onCreate={(input: CreateCanaryInput) =>
          create.mutate(input, {
            onSuccess: () => {
              setModalOpen(false);
              toast.success('Run created');
            },
            onError: (e) => toast.error(msg(e)),
          })
        }
      />
    </>
  );
}

export function RunDetailRoute() {
  const navigate = useNavigate();
  const { id = '' } = useParams();
  const detail = useCanaryRun(id);
  const tpl = useCanaryTemplate(detail.data?.run.template_id ?? '');
  const facts = useRunFacts(id, detail.data?.racks ?? []);
  const evidence = useRunAlertEvidence(id);
  const assessment = useRunAssessment(id);
  const act = useRunAction(id);
  const cancel = useCancelRun(id);

  if (detail.isLoading || tpl.isLoading) return <Loading label="Loading run…" />;
  if (detail.isError) return <ErrorPane message={msg(detail.error)} />;
  const run = detail.data!.run;
  const flatSteps = tpl.data?.flatSteps ?? [];
  const stepIndex = tpl.data?.stepIndex ?? {};
  const states = stepStatesFor(run, flatSteps, stepIndex);
  return (
    <RunDetail
      run={run}
      events={detail.data!.events}
      facts={facts.data ?? { source: 'inventory · cwf where' }}
      evidence={evidence.data ?? {}}
      assessment={assessment.data ?? null}
      steps={flatSteps}
      stepIndex={stepIndex}
      states={states}
      templateVersion={tpl.data?.version ?? 0}
      showRailGroups={false}
      onBack={() => navigate('/canary')}
      onSignoff={(input: {
        step_id: string;
        verdict: string;
        signer_name: string;
        signer_role: string;
      }) =>
        act.mutate(
          { action: 'signoff', ...input },
          { onSuccess: () => toast.success('Signed off'), onError: (e) => toast.error(msg(e)) },
        )
      }
      onAdvance={(input: { step_id: string }) =>
        act.mutate(
          { action: 'advance', ...input },
          { onSuccess: () => toast.success('Advanced'), onError: (e) => toast.error(msg(e)) },
        )
      }
      onCancel={() =>
        cancel.mutate(undefined, {
          onSuccess: () => toast.success('Run cancelled'),
          onError: (e) => toast.error(msg(e)),
        })
      }
    />
  );
}
