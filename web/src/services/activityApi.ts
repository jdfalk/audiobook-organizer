// file: web/src/services/activityApi.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

const API_BASE = import.meta.env.VITE_API_URL || '/api/v1';

export interface ActivityEntry {
  id: string;
  timestamp: string;
  tier: 'audit' | 'change' | 'debug';
  type: string;
  level: string;
  source: string;
  operation_id?: string;
  book_id?: string;
  summary: string;
  details?: Record<string, unknown>;
  tags?: string[];
  pruned_at?: string;
}

export interface ActivityResponse {
  entries: ActivityEntry[];
  total: number;
}

export interface ActivityFilter {
  limit?: number;
  offset?: number;
  type?: string;
  tier?: string;
  level?: string;
  operation_id?: string;
  book_id?: string;
  since?: string;
  until?: string;
  tags?: string;
}

export async function fetchActivity(filter?: ActivityFilter): Promise<ActivityResponse> {
  const params = new URLSearchParams();
  if (filter) {
    if (filter.limit !== undefined) params.set('limit', String(filter.limit));
    if (filter.offset !== undefined) params.set('offset', String(filter.offset));
    if (filter.type) params.set('type', filter.type);
    if (filter.tier) params.set('tier', filter.tier);
    if (filter.level) params.set('level', filter.level);
    if (filter.operation_id) params.set('operation_id', filter.operation_id);
    if (filter.book_id) params.set('book_id', filter.book_id);
    if (filter.since) params.set('since', filter.since);
    if (filter.until) params.set('until', filter.until);
    if (filter.tags) params.set('tags', filter.tags);
  }
  const query = params.toString();
  const response = await fetch(`${API_BASE}/activity${query ? `?${query}` : ''}`);
  if (!response.ok) {
    throw new Error(`Failed to fetch activity: ${response.status}`);
  }
  return response.json();
}
