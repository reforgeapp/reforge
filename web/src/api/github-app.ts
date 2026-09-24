import { apiRequest } from './client'

export type GitHubAppMode = 'manifest' | 'hosted' | 'unavailable'

export type GitHubAppPending = {
  id: string
  phase: string
  app_slug?: string
  expires_at: string
  resume_url?: string
}

export type GitHubAppStatus = {
  mode: GitHubAppMode
  reason?: string
  pending?: GitHubAppPending
}

export type GitHubAppSetupInput = {
  name: string
  github_org?: string
}

export type GitHubAppSetupResult = {
  id: string
  handoff_url: string
}

const base = (orgID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}/github-app`

export const githubAppAPI = {
  status: (orgID: string, signal?: AbortSignal) => apiRequest<GitHubAppStatus>(base(orgID), { signal }),
  start: (orgID: string, input: GitHubAppSetupInput, csrf: string) => apiRequest<GitHubAppSetupResult>(`${base(orgID)}/setups`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  cancel: (orgID: string, setupID: string, csrf: string) => apiRequest<void>(`${base(orgID)}/setups/${encodeURIComponent(setupID)}`, { method: 'DELETE' }, csrf),
}

export function isSameOriginPath(value: string) {
  return value.startsWith('/') && !value.startsWith('//') && !value.includes('\\')
}
