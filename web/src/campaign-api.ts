import { apiRequest } from './api/client'
import type { PreviewRequest as PipelineRequest } from './deployment-api'
import type { PreviewRequest as GitOpsRequest } from './gitops-api'

export type Window = { weekdays: number[]; start_minute: number; end_minute: number }
export type RepairRequest = { finding_id: string; finding_version: number; recipe: string; model_connection_id: string; model_route: string; runner_pool_id: string; plan_digest?: string; idempotency_key?: string }
export type MemberInput = { repository_id: string; repair?: RepairRequest; environment?: string; pipeline?: PipelineRequest; gitops?: GitOpsRequest }
export type Input = { name: string; kind: 'repair' | 'pipeline' | 'gitops'; selection: string; members: MemberInput[]; canary_ids: string[]; canary_size: number; batch_size: number; concurrency: number; success: 'published' | 'merged' | 'healthy'; observation_seconds: number; failure_limit: number; failure_percent: number; windows: Window[]; not_before?: string }
export type Member = { id: string; repository_id: string; repository_name: string; repositories: string[]; group: string; canary: boolean; input: MemberInput; pins: Record<string, string>; state: string; reason: string; stage: number; action_id?: string; succeeded_at?: string }
export type Preview = { id: string; hash: string; input: Input; members: Member[]; blockers: string[]; expires_at: string }
export type Counts = { total: number; excluded: number; pending: number; running: number; succeeded: number; failed: number; unknown: number }
export type Campaign = { id: string; name: string; kind: string; state: string; reason: string; version: number; stage: number; requested_by: string; grant_expires_at: string; spec: Input; counts: Counts; created_at: string; updated_at: string; observing_since?: string }
export type Page<T> = { items: T[]; complete: boolean; next_cursor?: string }
const path = (org: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(org)}${suffix}`
const post = <T,>(url: string, body: unknown, csrf: string, version?: number) => apiRequest<T>(url, { method: 'POST', headers: { 'Content-Type': 'application/json', ...(version === undefined ? {} : { 'If-Match': `"${version}"` }) }, body: JSON.stringify(body) }, csrf)
const query = (cursor?: string, state?: string) => { const value = new URLSearchParams({ limit: '100' }); if (cursor) value.set('cursor', cursor); if (state) value.set('state', state); return value.toString() }
export const campaignAPI = {
  preview: (org: string, input: Input, csrf: string) => post<Preview>(path(org, '/campaign-previews'), input, csrf),
  create: (org: string, previewID: string, key: string, csrf: string) => post<Campaign>(path(org, '/campaigns'), { preview_id: previewID, idempotency_key: key }, csrf),
  list: (org: string, cursor?: string, state?: string, signal?: AbortSignal) => apiRequest<Page<Campaign>>(path(org, `/campaigns?${query(cursor, state)}`), { signal }),
  get: (org: string, id: string, signal?: AbortSignal) => apiRequest<Campaign>(path(org, `/campaigns/${encodeURIComponent(id)}`), { signal }),
  members: (org: string, id: string, cursor?: string, signal?: AbortSignal) => apiRequest<Page<Member>>(path(org, `/campaigns/${encodeURIComponent(id)}/members?${query(cursor)}`), { signal }),
  control: (org: string, value: Campaign, action: 'start' | 'pause' | 'resume' | 'cancel', reason: string, csrf: string) => post<Campaign>(path(org, `/campaigns/${encodeURIComponent(value.id)}/${action}`), { reason, ...(action === 'resume' ? { stage_decision: 'continue_current_stage' } : {}) }, csrf, value.version),
}
