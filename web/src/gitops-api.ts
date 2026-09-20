import { apiRequest } from './api/client'
import type { Gate as MergeGate, Operation as MergeOperation } from './merge-api'

export type Configuration = {
  environment: string; source_repository_id: string; delivery_repository_id: string; version: number; enabled: boolean
  target_branch: string; manifest_path: string; pointer: string; image_repository: string
  provenance_public_key: string; health_public_key: string; health_checks: string[]
  observation_seconds: number; max_evidence_age_seconds: number; deadline_seconds: number; recovery_allowed: boolean
}
export type PreviewRequest = { change_id: string; source_sha: string; artifact_digest: string; provenance: unknown; recovery_of?: string; restore_promotion_id?: string }
export type Gate = { id: string; operation_id: string; configuration: Configuration; request: PreviewRequest; source: { native_id: string; full_name: string }; delivery: { native_id: string; full_name: string }; target_sha: string; before: string; after: string; manifest_sha256: string; patched_manifest: string; decision: { outcome: string; blockers?: string[]; required_actions?: string[] }; blockers: string[]; expires_at: string }
export type Promotion = { id: string; environment: string; source_repository_id: string; delivery_repository_id: string; gate_id: string; state: string; reason: string; version: number; requested_by: string; branch: string; candidate_sha: string; change?: { id: string; title: string; url: string; head_sha: string; target_sha: string; state: string }; merge_sha: string; recovery_of?: string; cancel_requested: boolean; created_at: string; updated_at: string; finished_at?: string }
export type Detail = { promotion: Promotion; gate: Gate; health?: { source_sha: string; delivery_revision: string; healthy: boolean; checks: Record<string, boolean>; observed_at: string } }
export type Page<T> = { items: T[]; next_cursor?: string; complete: boolean }

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`
const json = <T>(url: string, body: unknown, csrf: string, init: RequestInit = {}) => apiRequest<T>(url, { ...init, headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) }, body: JSON.stringify(body) }, csrf)

export const gitopsAPI = {
  configurations: (orgID: string, signal?: AbortSignal) => apiRequest<{ items: Configuration[] }>(path(orgID, '/gitops-configurations'), { signal }),
  saveConfiguration: (orgID: string, configuration: Configuration, csrf: string) => json<Configuration>(path(orgID, `/gitops-configurations/${encodeURIComponent(configuration.environment)}`), configuration, csrf, { method: 'PUT', headers: { 'If-Match': tag(configuration.version) }, }),
  preview: (orgID: string, environment: string, request: PreviewRequest, csrf: string) => json<Gate>(path(orgID, `/gitops-configurations/${encodeURIComponent(environment)}/preview`), request, csrf, { method: 'POST' }),
  promotions: (orgID: string, params: { environment?: string; cursor?: string; limit?: number; signal?: AbortSignal } = {}) => { const query = new URLSearchParams(); if (params.environment) query.set('environment', params.environment); if (params.cursor) query.set('cursor', params.cursor); query.set('limit', String(params.limit ?? 50)); return apiRequest<Page<Promotion>>(path(orgID, `/gitops-promotions?${query}`), { signal: params.signal }) },
  promotion: (orgID: string, id: string, signal?: AbortSignal) => apiRequest<Detail>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}`), { signal }),
  request: (orgID: string, gateID: string, idempotencyKey: string, csrf: string) => json<Promotion>(path(orgID, '/gitops-promotions'), { gate_id: gateID, idempotency_key: idempotencyKey }, csrf, { method: 'POST' }),
  continue: (orgID: string, id: string, csrf: string) => json<Promotion>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}/continue`), {}, csrf, { method: 'POST' }),
  observe: (orgID: string, id: string, csrf: string) => json<Promotion>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}/observe`), {}, csrf, { method: 'POST' }),
  cancel: (orgID: string, id: string, version: number, csrf: string) => apiRequest<Promotion>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}/cancel`), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
  mergePreview: (orgID: string, id: string, method: string, csrf: string) => json<MergeGate>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}/merge-preview`), { method }, csrf, { method: 'POST' }),
  merge: (orgID: string, id: string, gateID: string, idempotencyKey: string, csrf: string) => json<MergeOperation>(path(orgID, `/gitops-promotions/${encodeURIComponent(id)}/merge`), { gate_id: gateID, idempotency_key: idempotencyKey }, csrf, { method: 'POST' }),
}
