import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import AssessmentCard from './AssessmentCard';

describe('AssessmentCard', () => {
  it('renders verdict, confidence, reasoning, and the advisory badge', () => {
    render(
      <AssessmentCard
        assessment={{
          verdict: 'fail',
          confidence: 'high',
          reasoning: 'Two racks did not converge to the target bundle.',
          ranked_causes: [{ summary: 'BMC firmware stuck', likely_owner: 'hardware' }],
          likely_owner: 'hardware',
          sources: ['claude-opus-4-8'],
        }}
      />,
    );
    expect(screen.getByText(/fail/i)).toBeInTheDocument();
    expect(screen.getByText(/high/i)).toBeInTheDocument();
    expect(screen.getByText(/did not converge/i)).toBeInTheDocument();
    expect(screen.getByText(/advisory — operator decides/i)).toBeInTheDocument();
    expect(screen.getByText(/BMC firmware stuck/i)).toBeInTheDocument();
  });
});
