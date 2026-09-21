import type { components } from './api/schema'
import { apiRequest } from './api/client'

export type CustomProfile = components['schemas']['CustomProfile']
export type CustomProfileInput = components['schemas']['CustomProfileInput']
export type CustomProfilePage = components['schemas']['CustomProfilePage']

const versionTag = (version: number) => `"${version}"`
const base = (orgID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}/custom-profiles`

export const customProfileAPI = {
  list: (orgID: string, cursor?: string, signal?: AbortSignal) => apiRequest<CustomProfilePage>(`${base(orgID)}?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, { signal }),
  create: (orgID: string, input: CustomProfileInput, csrf: string) => apiRequest<CustomProfile>(base(orgID), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  approve: (orgID: string, id: string, version: number, evidence: string, csrf: string) => apiRequest<CustomProfile>(`${base(orgID)}/${encodeURIComponent(id)}/approve`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify({ evidence }) }, csrf),
  revoke: (orgID: string, id: string, version: number, csrf: string) => apiRequest<CustomProfile>(`${base(orgID)}/${encodeURIComponent(id)}/revoke`, { method: 'POST', headers: { 'If-Match': versionTag(version) } }, csrf),
}
