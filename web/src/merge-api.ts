import { apiRequest } from './api/client'

export type Change = {
  id: string; title: string; body: string; url: string; head_sha: string; target_sha: string
  head_branch: string; target_branch: string; state: string; merge_status: string; operation_id: string
  repository: { native_id: string; full_name: string }; head_repository: { native_id: string; full_name: string }
  target_repository: { native_id: string; full_name: string }; author_login: string; author_type: string
}
type Evidence = { id?: string; state?: string; reference?: string; binding?: { head?: string; target?: string }; observed_at?: string }
export type Gate = {
  id: string; repository_id: string; connection_id: string; connection_version: number; configuration_version: number
  method: string; expires_at: string; snapshot: { change: Change; rules: { state: string; reason: string; hash: string; required_checks: Array<{ name: string; publisher_id: string }>; required_approvals: number; require_code_owners: boolean }; checks: Array<{ name: string; publisher_id: string; head_sha: string; conclusion: string; status: string }>; approvals: Array<{ actor_id: string; state: string; head_sha: string; dismissed: boolean }>; native: { state: string; head_sha: string; target_sha: string; url?: string }; capabilities: { provider: string; server_version: string } }
  decision: { outcome: string; blockers: string[]; required_actions: string[]; rules: string[]; evidence?: Evidence[] }; binding: { head: string; target: string; tested: string; policy_hash: string; provider_rules: string }; companions?: Array<{ task_id: string; change_id: string; head_sha: string; merge_sha: string; state: 'repair_pending' | 'merge_pending' | 'merged' }>
}
export type OperationPage = { items: Operation[]; next_cursor?: string; complete: boolean }
export type Operation = { id: string; repository_id: string; gate_id: string; requested_gate_id?: string; change_id: string; state: string; reason: string; cancel_requested: boolean; version: number; created_at: string; updated_at: string; native_result?: { state: string; merge_sha: string; head_sha: string; url: string } }
const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`
export const mergeAPI = {
  preview: (orgID: string, repositoryID: string, changeID: string, method: string, csrf: string, signal?: AbortSignal) => apiRequest<Gate>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/changes/${encodeURIComponent(changeID)}/merge-preview`), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ method }), signal }, csrf),
  merge: (orgID: string, gateID: string, idempotencyKey: string, csrf: string) => apiRequest<Operation>(path(orgID, '/merge-operations'), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ gate_id: gateID, idempotency_key: idempotencyKey }) }, csrf),
  operations: (orgID: string, repositoryID: string, changeID: string, signal?: AbortSignal) => apiRequest<OperationPage>(path(orgID, `/merge-operations?repository_id=${encodeURIComponent(repositoryID)}&change_id=${encodeURIComponent(changeID)}&limit=50`), { signal }),
  operation: (orgID: string, operationID: string, signal?: AbortSignal) => apiRequest<Operation>(path(orgID, `/merge-operations/${encodeURIComponent(operationID)}`), { signal }),
  reconcile: (orgID: string, operationID: string, version: number, csrf: string) => apiRequest<Operation>(path(orgID, `/merge-operations/${encodeURIComponent(operationID)}/reconcile`), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
  cancel: (orgID: string, operationID: string, version: number, csrf: string) => apiRequest<Operation>(path(orgID, `/merge-operations/${encodeURIComponent(operationID)}/cancel`), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
}
