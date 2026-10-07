import { apiRequest } from './client'

export type Task = {
  id: string; org_id: string; repository_id: string; operation_id: string; recipe: string; recipe_version: string; target_branch: string
  model_connection_id?: string; model_route: string; campaign_id?: string; runner_pool_id?: string; policy_hash: string; starting_policy_hash: string
  state: string; reason: string; version: number; cancel_version: number; max_attempts: number; created_at: string
}
export type TaskPage = { items: Task[]; next_cursor?: string; complete: boolean }
export type CheckResult = { command_id: string; exit_code: number; output_sha256: string; complete: boolean; cases?: Record<string, string>; reason: string; excerpt?: string }
export type RepairRun = {
  candidate_artifacts?: string[]; branch: string; candidate_sha: string; candidate_checks?: CheckResult[] | null; change?: { id: string; title: string; url: string; state: string; head_sha: string; target_sha: string; target_branch: string; head_branch: string }
  task: Task; context: { native_head_sha?: string; model?: string; finding?: { title?: string; category?: string; source?: string }; max_output_tokens?: number; turn_timeout_ms?: number; policy_hash?: string; plan?: { digest: string; baseline_sha?: string; target_sha?: string; recipe?: { commands: Array<{ id: string; args: string[]; directory: string; timeout_seconds: number; report_format: string }> } } }
  report?: { diff?: string; plan_digest: string; state: string; reason: string; baseline: CheckResult[]; candidate: CheckResult[]; target: CheckResult[]; patches: Array<{ path: string; content: string }>; artifacts: string[]; turns: number }
  state: string; version: number; updated_at: string
}
export type RunLog = { seq: number; attempt_id: string; kind: 'stage' | 'model' | 'tool' | 'result'; message: string; created_at: string }
export type RunEvent = { id: number; type: string; aggregate_type: string; aggregate_id: string; aggregate_version: number; occurred_at: string; data: unknown }
const path = (orgID: string, suffix: string) => '/api/v1/orgs/' + encodeURIComponent(orgID) + suffix
const tag = (version: number) => '"' + version + '"'

export const runsAPI = {
  tasks: (orgID: string, params: { cursor?: string; limit?: number; state?: string; signal?: AbortSignal } = {}) => {
    const query = new URLSearchParams()
    if (params.cursor) query.set('cursor', params.cursor)
    if (params.state) query.set('state', params.state)
    query.set('limit', String(params.limit ?? 30))
    return apiRequest<TaskPage>(path(orgID, '/tasks?' + query.toString()), { signal: params.signal })
  },
  task: (orgID: string, taskID: string, signal?: AbortSignal) => apiRequest<Task>(path(orgID, '/tasks/' + encodeURIComponent(taskID)), { signal }),
  repair: (orgID: string, taskID: string, signal?: AbortSignal) => apiRequest<RepairRun>(path(orgID, '/repair-runs/' + encodeURIComponent(taskID)), { signal }),
  cancel: (orgID: string, taskID: string, version: number, csrf: string) => apiRequest<Task>(path(orgID, '/tasks/' + encodeURIComponent(taskID) + '/cancel'), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
  resume: (orgID: string, taskID: string, version: number, csrf: string) => apiRequest<Task>(path(orgID, '/tasks/' + encodeURIComponent(taskID) + '/resume'), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
  reconcile: (orgID: string, taskID: string, version: number, csrf: string) => apiRequest<RepairRun>(path(orgID, '/repair-runs/' + encodeURIComponent(taskID) + '/reconcile'), { method: 'POST', headers: { 'If-Match': tag(version) } }, csrf),
  logs: (orgID: string, taskID: string, after: number, signal?: AbortSignal) => apiRequest<RunLog[]>(path(orgID, '/repair-runs/' + encodeURIComponent(taskID) + '/logs?after=' + after), { signal }),
  artifactURL: (orgID: string, artifactID: string) => path(orgID, '/artifacts/' + encodeURIComponent(artifactID) + '/download'),
}

export function eventsURL(orgID: string, after?: number) { return path(orgID, '/events' + (after ? '?after=' + after : '')) }
