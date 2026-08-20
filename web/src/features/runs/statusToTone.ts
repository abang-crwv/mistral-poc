import type { RunStatus } from './runs.types';
import type { ComponentProps } from 'react';
import type { Badge } from '@/components/Badge';

type BadgeTone = NonNullable<ComponentProps<typeof Badge>['tone']>;

export function statusToTone(status: RunStatus): BadgeTone {
  switch (status) {
    case 'pending':
      return 'neutral';
    case 'running':
      return 'info';
    case 'passed':
      return 'success';
    case 'warning':
      return 'warn';
    case 'failed':
      return 'danger';
    case 'signed_off':
      return 'neutral';
  }
}
