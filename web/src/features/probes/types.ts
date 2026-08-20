// Probe is one registered probe type as served by GET /api/probes. The
// backend registry is the source of truth for type + category; title and
// description are presentation copy.
export interface Probe {
  type: string;
  category: string; // gatherer | assertion | action
  title: string;
  description: string;
}
