import { apiRequest, type Connection, type RunnerPool } from './client'

export type RepairInput = { finding_id: string; finding_version: number; recipe: string; model_connection_id: string; model_route: string; runner_pool_id: string; custom_profile_id?: string; custom_profile_version?: number; plan_digest?: string; idempotency_key?: string }
export type RepairPreview = { context: { plan: { digest: string; baseline_sha: string; target_sha: string; max_changed_lines: number; recipe: { max_files: number; max_patch_bytes: number; max_turns: number; timeout_seconds: number; commands: Array<{ id: string; args: string[]; directory: string; timeout_seconds: number; report_format: string }> } }; policy_hash: string; max_output_tokens: number; turn_timeout_ms: number }; blockers: string[]; expires_at: string }
export type RepairRun = { task: { id: string; [key: string]: unknown }; state: string; version: number; [key: string]: unknown }

export const repairAPI = {
  recipes: (orgID: string, signal?: AbortSignal) => apiRequest<Record<string, string>>(`/api/v1/orgs/${encodeURIComponent(orgID)}/repair-recipes`, { signal }),
  preview: (orgID: string, input: RepairInput, csrf: string) => apiRequest<RepairPreview>(`/api/v1/orgs/${encodeURIComponent(orgID)}/repair-preview`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  start: (orgID: string, input: RepairInput, csrf: string) => apiRequest<RepairRun>(`/api/v1/orgs/${encodeURIComponent(orgID)}/repair-runs`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  models: (orgID: string, signal?: AbortSignal) => apiRequest<{ items: Connection[] }>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections?kind=model`, { signal }),
  agentConnections: (orgID: string, signal?: AbortSignal) => apiRequest<{ items: Connection[] }>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections?kind=agent`, { signal }),
  pools: (orgID: string, signal?: AbortSignal) => apiRequest<{ items: RunnerPool[] }>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools`, { signal }),
}
