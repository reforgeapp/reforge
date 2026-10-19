import type { components } from './api/schema'
import { apiRequest } from './api/client'
import type { Connection } from './api/client'

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

export type NativeAgentState = { state: string; login?: AgentLogin; account?: { id: string; email?: string; plan?: string }; models?: Array<{ id: string; name: string }>; version?: string; reason?: string; code_required?: boolean; model?: string }
const nativePath = (org: string, id?: string) => `/api/v1/orgs/${encodeURIComponent(org)}/agents${id ? `/${encodeURIComponent(id)}` : ''}`
export const nativeAgentAPI = {
  options: (org: string, signal?: AbortSignal) => apiRequest<{ available: boolean; reason?: string }>(nativePath(org), { signal }),
  create: (org: string, provider: string, name: string, csrf: string) => apiRequest<Connection>(nativePath(org), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider, name }) }, csrf),
  status: (org: string, id: string, signal?: AbortSignal) => apiRequest<NativeAgentState>(nativePath(org, id), { signal }),
  action: (org: string, id: string, action: string, csrf: string, input: { login_id?: string; code?: string; model?: string } = {}) => apiRequest<NativeAgentState>(`${nativePath(org, id)}/${action}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }, csrf),
  useDefault: (org: string, id: string, csrf: string) => apiRequest<void>(`${nativePath(org, id)}/default`, { method: 'POST' }, csrf),
}
