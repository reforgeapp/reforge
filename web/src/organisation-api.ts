import type { components } from './api/schema'
import { apiRequest } from './api/client'

export type Membership = components['schemas']['Membership']
export type MembershipPage = components['schemas']['MembershipPage']
export type Team = components['schemas']['Team']
export type TeamPage = components['schemas']['TeamPage']
export type MembershipInput = components['schemas']['MembershipInput']
export type TeamInput = components['schemas']['TeamInput']

export type OrgOIDCSettings = {
  configured: boolean
  secret_present: boolean
  issuer?: string
  client_id?: string
  status: 'unconfigured' | 'draft' | 'probe_verified' | 'active' | 'disabled'
  version: number
  verified_at?: string
  verified: boolean
  activation_available: boolean
  activation_blocked?: string
}

const versionTag = (version: number) => `"${version}"`
const base = (orgID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}`

export const organisationAPI = {
  memberships: (orgID: string, cursor?: string, signal?: AbortSignal) => apiRequest<MembershipPage>(`${base(orgID)}/memberships?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, { signal }),
  putMember: (orgID: string, userID: string, version: number, input: MembershipInput, csrf: string) => apiRequest<Membership>(`${base(orgID)}/memberships/${encodeURIComponent(userID)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify(input) }, csrf),
  deleteMember: (orgID: string, userID: string, version: number, csrf: string) => apiRequest<void>(`${base(orgID)}/memberships/${encodeURIComponent(userID)}`, { method: 'DELETE', headers: { 'If-Match': versionTag(version) } }, csrf),
  teams: (orgID: string, cursor?: string, signal?: AbortSignal) => apiRequest<TeamPage>(`${base(orgID)}/teams?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, { signal }),
  putTeam: (orgID: string, teamID: string, version: number, input: TeamInput, csrf: string) => apiRequest<Team>(`${base(orgID)}/teams/${encodeURIComponent(teamID)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify(input) }, csrf),
  deleteTeam: (orgID: string, teamID: string, version: number, csrf: string) => apiRequest<void>(`${base(orgID)}/teams/${encodeURIComponent(teamID)}`, { method: 'DELETE', headers: { 'If-Match': versionTag(version) } }, csrf),
  oidc: (orgID: string, signal?: AbortSignal) => apiRequest<OrgOIDCSettings>(`${base(orgID)}/identity/oidc`, { signal }),
  putOIDC: (orgID: string, version: number, input: { issuer: string; client_id: string; client_secret: string }, csrf: string) => apiRequest<OrgOIDCSettings>(`${base(orgID)}/identity/oidc`, { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify(input) }, csrf),
  probeOIDC: (orgID: string, version: number, csrf: string) => apiRequest<OrgOIDCSettings>(`${base(orgID)}/identity/oidc/probe`, { method: 'POST', headers: { 'If-Match': versionTag(version) } }, csrf),
  activateOIDC: (orgID: string, version: number, csrf: string) => apiRequest<void>(`${base(orgID)}/identity/oidc/activate`, { method: 'POST', headers: { 'If-Match': versionTag(version) } }, csrf),
  disableOIDC: (orgID: string, version: number, csrf: string) => apiRequest<OrgOIDCSettings>(`${base(orgID)}/identity/oidc/disable`, { method: 'POST', headers: { 'If-Match': versionTag(version) } }, csrf),
}
