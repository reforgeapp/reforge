import type { components } from './schema'
import { apiRequest } from './client'

export type Finding = components['schemas']['Finding']
export type FindingPage = components['schemas']['FindingPage']
export type FindingUpdate = components['schemas']['FindingUpdate']
export type AdvisoryInput = components['schemas']['AdvisoryInput']

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`

export const discoveryAPI = {
  findings: (orgID: string, params: { q?: string; repository_id?: string; state?: string; category?: string; severity?: string; cursor?: string; limit?: number; signal?: AbortSignal } = {}) => {
    const query = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => { if (key !== 'signal' && value) query.set(key, String(value)) })
    return apiRequest<FindingPage>(path(orgID, `/findings${query.size ? `?${query}` : ''}`), { signal: params.signal })
  },
  finding: (orgID: string, findingID: string, signal?: AbortSignal) => apiRequest<Finding>(path(orgID, `/findings/${encodeURIComponent(findingID)}`), { signal }),
  updateFinding: (orgID: string, findingID: string, version: number, input: FindingUpdate, csrf: string) => apiRequest<Finding>(path(orgID, `/findings/${encodeURIComponent(findingID)}`), { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) }, body: JSON.stringify(input) }, csrf),
  importAdvisory: (orgID: string, input: AdvisoryInput, csrf: string) => apiRequest<Finding>(path(orgID, '/findings/advisories'), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
}
