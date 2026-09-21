import type { components } from './api/schema'
import { apiRequest } from './api/client'

export type AgentQualification = components['schemas']['AgentQualification']
export type AgentQualificationInput = components['schemas']['AgentQualificationInput']
export type AgentQualificationResponse = components['schemas']['AgentQualificationResponse']
export type AgentLogin = { id: string; url: string; code?: string }

const base = (orgID: string, connectionID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}/agent-qualification`
const auth = (orgID: string, connectionID: string, action: 'login' | 'logout') => `/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}/agent/${action}`

export const agentAPI = {
  qualification: (orgID: string, connectionID: string, signal?: AbortSignal) => apiRequest<AgentQualificationResponse>(base(orgID, connectionID), { signal }),
  putQualification: (orgID: string, connectionID: string, input: AgentQualificationInput, csrf: string) => apiRequest<AgentQualificationResponse>(base(orgID, connectionID), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  clearQualification: (orgID: string, connectionID: string, csrf: string) => apiRequest<void>(base(orgID, connectionID), { method: 'DELETE' }, csrf),
  login: (orgID: string, connectionID: string, csrf: string) => apiRequest<{ login: AgentLogin }>(auth(orgID, connectionID, 'login'), { method: 'POST' }, csrf),
  logout: (orgID: string, connectionID: string, csrf: string) => apiRequest<void>(auth(orgID, connectionID, 'logout'), { method: 'POST' }, csrf),
}
