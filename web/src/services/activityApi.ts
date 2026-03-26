// file: web/src/services/activityApi.ts
// version: 1.1.0
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
  search?: string;
  source?: string;
  exclude_sources?: string;
  exclude_tiers?: string;
}

export interface SourceCount {
  source: string;
  count: number;
}

export interface SourcesResponse {
  sources: SourceCount[];
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
    if (filter.search) params.set('search', filter.search);
    if (filter.source) params.set('source', filter.source);
    if (filter.exclude_sources) params.set('exclude_sources', filter.exclude_sources);
    if (filter.exclude_tiers) params.set('exclude_tiers', filter.exclude_tiers);
  }
  const query = params.toString();
  const response = await fetch(`${API_BASE}/activity${query ? `?${query}` : ''}`);
  if (!response.ok) {
    throw new Error(`Failed to fetch activity: ${response.status}`);
  }
  return response.json();
}

export async function fetchActivitySources(filter: Partial<ActivityFilter> = {}): Promise<SourcesResponse> {
  const params = new URLSearchParams();
  if (filter.tier) params.set('tier', filter.tier);
  if (filter.level) params.set('level', filter.level);
  if (filter.since) params.set('since', filter.since);
  if (filter.until) params.set('until', filter.until);
  const url = `${API_BASE}/activity/sources?${params.toString()}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Sources API error: ${res.status}`);
  return res.json();
}
