import { z } from 'zod';

// MIRRORED in internal/api/runs_validate.go (rackPattern).
// If you change one, change the other in the same commit.
const rackPattern = /^[a-z]+\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$/;

export const newRunSchema = z.object({
  bundle: z
    .string()
    .trim()
    .min(1, 'Bundle is required')
    .max(200, 'Bundle is too long (max 200 characters)'),
  rack: z
    .string()
    .trim()
    .min(1, 'At least one rack is required')
    .refine(
      (val) =>
        val
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
          .every((r) => rackPattern.test(r)),
      'Rack format example: dh3-r012-us-east-01a',
    ),
});

export type NewRunInput = z.infer<typeof newRunSchema>;
