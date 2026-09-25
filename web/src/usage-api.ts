import { apiRequest } from './api/client'

export type Amount = { micro_usd: number; tokens: number; milliseconds: number; requests: number; concurrency: number }
export type Caps = { micro_usd?: number; tokens?: number; milliseconds?: number; requests?: number; concurrency?: number }
export type Limit = { scope: { kind: string; id: string }; period: string; start?: string; end?: string; caps: Caps; paused: boolean; version: number; held: Amount; spent: Amount }
export type Route = { connection_id: string; model: string; name: string; mode: string; pricing_version: string; input_micro_usd_per_million: number; output_micro_usd_per_million: number; request_micro_usd: number; max_input_tokens: number; max_output_tokens: number; max_milliseconds: number; max_requests: number; qualified: boolean; qualification_ref: string; paused: boolean; version: number }
export type Reservation = { id: string; connection_id: string; maximum: Amount; actual?: Amount; state: string; created_at: string; quote: { model: string; route: string; input_tokens: number; max_output_tokens: number }; lease: { repository_id: string; task_id: string; operation_id: string }; route: Route }
export type UsageRow = { reservation: Reservation; provider: string; recipe: string; repository_name: string }
export type UsagePage = { items: UsageRow[]; complete: boolean; next_cursor?: string }
export type UsageSeries = { days: Array<{ day: string; micro_usd: number; tokens: number; records: number }>; providers: Array<{ provider: string; micro_usd: number; tokens: number; records: number }> }
export type UsageSummary = { records: number; settled: number; unknown: number; reserved: number; dispatched: number; cancelled: number; estimated_cost_micro_usd: number; known_tokens: number; unknown_maximum: Amount; held: Amount }

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`
const json = <T>(url: string, body: unknown, csrf: string, init: RequestInit = {}) => apiRequest<T>(url, { ...init, headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) }, body: JSON.stringify(body) }, csrf)
export type UsageFilters = { repository_id?: string; team_id?: string; recipe?: string; provider?: string; connection_id?: string; state?: string; since?: string; until?: string; cursor?: string; limit?: number; signal?: AbortSignal }
const query = (filters: UsageFilters, includePage = true) => { const value = new URLSearchParams(); for (const key of ['repository_id', 'team_id', 'recipe', 'provider', 'connection_id', 'state', 'since', 'until'] as const) if (filters[key]) value.set(key, filters[key]!) ; if (includePage && filters.cursor) value.set('cursor', filters.cursor); if (includePage) value.set('limit', String(Math.min(filters.limit ?? 50, 100))); return value.toString() }
export const usageAPI = {
  list: (orgID: string, filters: UsageFilters = {}) => apiRequest<UsagePage>(path(orgID, `/usage?${query(filters)}`), { signal: filters.signal }),
  summary: (orgID: string, filters: UsageFilters = {}) => apiRequest<UsageSummary>(path(orgID, `/usage/summary?${query(filters, false)}`), { signal: filters.signal }),
  series: (orgID: string, filters: UsageFilters = {}) => apiRequest<UsageSeries>(path(orgID, `/usage/series?${query(filters, false)}`), { signal: filters.signal }),
  budget: (orgID: string, kind: string, id: string, signal?: AbortSignal) => apiRequest<Limit>(path(orgID, `/budgets/${encodeURIComponent(kind)}/${encodeURIComponent(id)}`), { signal }),
  saveBudget: (orgID: string, limit: Limit, csrf: string) => json<Limit>(path(orgID, `/budgets/${encodeURIComponent(limit.scope.kind)}/${encodeURIComponent(limit.scope.id)}`), limit, csrf, { method: 'PUT', headers: { 'If-Match': tag(limit.version) } }),
  saveRoute: (orgID: string, route: Route, csrf: string) => json<Route>(path(orgID, `/budget-routes/${encodeURIComponent(route.connection_id)}`), route, csrf, { method: 'PUT', headers: { 'If-Match': tag(route.version) } }),
  route: (orgID: string, connectionID: string, model: string, routeName: string, signal?: AbortSignal) => {
    const params = new URLSearchParams({ model, route: routeName })
    return apiRequest<Route>(path(orgID, `/budget-routes/${encodeURIComponent(connectionID)}?${params}`), { signal })
  },
}
