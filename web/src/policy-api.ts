import { apiRequest } from './api/client'
import type { components } from './api/schema'

export type Action = 'read' | 'repair' | 'publish' | 'merge' | 'deploy' | 'recover'
export type Scope = { kind: string; id: string }
export type Lists = { recipes: string[] | null; models: string[] | null; routes: string[] | null; merge_methods: string[] | null; environments: string[] | null; workflows: string[] | null }
export type Limits = { budget?: number; concurrency?: number; attempts?: number; changed_files?: number; changed_lines?: number; open_changes?: number }
export type Defaults = { model?: string; route?: string; branch_prefix?: string }
export type Requirement = { id: string; identity?: string; actions: Action[]; approvals?: number }
export type Proof = 'tests' | 'structural'
export type Policy = { schema: string; allow: Lists; deny: Action[]; forbidden_paths: string[]; review_proofs?: Proof[]; limits: Limits; required: Requirement[]; defaults: Defaults; max_evidence_age_seconds?: number; paused: boolean }
export type Layer = { scope: Scope; version_id: string; binding_version: number; policy: Policy }
export type Resolved = { hash: string; layers: Layer[]; primary_team_id?: string; repository_id: string; paused: boolean; policy: Policy; scope_paused: boolean; missing_defaults: string[]; problems: string[] }
export type Binding = { head: string; target: string; tested: string; policy_hash: string; provider_rules: string; capability_version: string; source_sha: string; artifact: string }
export type Input = { stage?: string; action: Action; recipe: string; model: string; route: string; merge_method: string; environment: string; workflow: string; paths: string[]; usage: Limits; current: Binding; starting_policy_hash: string; evidence: unknown[]; paused_scopes: string[]; now?: string }
export type Result = { outcome: string; blockers?: string[]; required_actions?: string[]; bindings: Layer[]; evidence_references: string[]; starting_policy_hash: string }
export type Version = { id: string; scope: Scope; policy: Policy; hash: string; actor_id: string; reason: string; created_at: string }
export type Simulation = { hash: string; resolved: Resolved; decision: Result }
export type VersionPage = { items: Version[]; next_cursor?: string; complete: boolean }

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
export const policyAPI = {
  teams: (orgID: string, cursor?: string, signal?: AbortSignal) => apiRequest<{ items: Array<{ id: string; name: string; repository_ids: string[] }>; complete: boolean; next_cursor?: string }>(path(orgID, `/teams?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`), { signal }),
  effective: (orgID: string, repositoryID: string, signal?: AbortSignal) => apiRequest<Resolved>(path(orgID, `/policies/effective?repository_id=${encodeURIComponent(repositoryID)}`), { signal }),
  versions: (orgID: string, scope: Scope, cursor?: string, signal?: AbortSignal) => { const query = new URLSearchParams({ scope_kind: scope.kind, scope_id: scope.id, limit: '50' }); if (cursor) query.set('cursor', cursor); return apiRequest<VersionPage>(path(orgID, `/policies/versions?${query}`), { signal }) },
  version: (orgID: string, versionID: string, signal?: AbortSignal) => apiRequest<Version>(path(orgID, `/policies/versions/${encodeURIComponent(versionID)}`), { signal }),
  createVersion: (orgID: string, scope: Scope, policy: Policy, reason: string, csrf: string) => apiRequest<Version>(path(orgID, '/policies/versions'), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ scope, policy, reason }) }, csrf),
  simulate: (orgID: string, versionID: string, repositoryID: string, primaryTeamID: string, input: Input, csrf: string) => apiRequest<Simulation>(path(orgID, `/policies/versions/${encodeURIComponent(versionID)}/simulate`), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ repository_id: repositoryID, primary_team_id: primaryTeamID, input }) }, csrf),
  activate: (orgID: string, versionID: string, repositoryID: string, primaryTeamID: string, version: number, simulationHash: string, reason: string, csrf: string) => apiRequest<{ version: number }>(path(orgID, `/policies/versions/${encodeURIComponent(versionID)}/activate`), { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': `"${version}"` }, body: JSON.stringify({ repository_id: repositoryID, primary_team_id: primaryTeamID, simulation_hash: simulationHash, reason }) }, csrf),
}

export type Autopilot = components['schemas']['Autopilot']
export type AutopilotMetric = components['schemas']['AutopilotMetric']
export type Need = { kind: 'grant' | 'review' | 'blocked'; finding_id: string; repository_id: string; repository: string; title: string; reason: string; url?: string; label?: string }
export const autopilotAPI = {
  get: (orgID: string, signal?: AbortSignal) => apiRequest<Autopilot>(path(orgID, '/autopilot'), { signal }),
  needs: (orgID: string, signal?: AbortSignal) => apiRequest<Need[]>(path(orgID, '/autopilot/needs'), { signal }),
  metrics: (orgID: string, signal?: AbortSignal) => apiRequest<AutopilotMetric[]>(path(orgID, '/autopilot/metrics'), { signal }),
  run: (orgID: string, repositoryID: string, csrf: string) => apiRequest<void>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/run`), { method: 'POST' }, csrf),
  put: (orgID: string, version: number, enabled: boolean, csrf: string) => apiRequest<Autopilot>(path(orgID, '/autopilot'), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': `"${version}"` }, body: JSON.stringify({ enabled }) }, csrf),
}
