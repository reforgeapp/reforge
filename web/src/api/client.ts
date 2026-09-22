import type { components } from './schema'

export type APIError = components['schemas']['APIError']
export type Meta = components['schemas']['Meta']
export type Session = components['schemas']['Session']
export type RepositoryPage = components['schemas']['RepositoryPage']
export type Connection = components['schemas']['Connection']
export type ConnectionCreate = components['schemas']['ConnectionCreate']
export type ConnectionPage = components['schemas']['ConnectionPage']
export type RunnerPool = components['schemas']['RunnerPool']
export type RunnerPoolInput = components['schemas']['RunnerPoolInput']
export type Runner = components['schemas']['Runner']
export type RunnerPage = components['schemas']['RunnerPage']
export type EnrollmentToken = components['schemas']['EnrollmentToken']

export class ReforgeAPIError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId: string
  readonly retryable: boolean

  constructor(status: number, error: APIError) {
    super(error.message)
    this.name = 'ReforgeAPIError'
    this.status = status
    this.code = error.code
    this.requestId = error.request_id
    this.retryable = error.retryable
  }
}

function versionTag(version: number) { return `"${version}"` }

async function request<T>(path: string, init: RequestInit = {}, csrfToken?: string): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (csrfToken) headers.set('X-CSRF-Token', csrfToken)
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  if (!response.ok) {
    let body: Partial<APIError> = {}
    try { body = await response.json() as APIError } catch { body = {} }
    throw new ReforgeAPIError(response.status, {
      code: body.code ?? (response.status === 401 ? 'unauthenticated' : 'request_failed'),
      message: body.message ?? `Request failed (${response.status})`,
      request_id: body.request_id ?? '',
      retryable: body.retryable ?? response.status >= 500,
      details: body.details,
    })
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const apiRequest = request

export const api = {
  getMeta: () => request<Meta>('/api/v1/meta'),
  getSession: (signal?: AbortSignal) => request<Session>('/api/v1/session', { signal }),
  getRepositories: (orgID: string, params: { cursor?: string; limit?: number; query?: string; signal?: AbortSignal } = {}) => {
    const search = new URLSearchParams()
    if (params.cursor) search.set('cursor', params.cursor)
    if (params.limit) search.set('limit', String(params.limit))
    if (params.query) search.set('q', params.query)
    const suffix = search.size ? `?${search.toString()}` : ''
    return request<RepositoryPage>(`/api/v1/orgs/${encodeURIComponent(orgID)}/repositories${suffix}`, { signal: params.signal })
  },
  getConnections: (orgID: string, params: { kind?: string; cursor?: string; limit?: number; signal?: AbortSignal } = {}) => {
    const search = new URLSearchParams()
    if (params.kind) search.set('kind', params.kind)
    if (params.cursor) search.set('cursor', params.cursor)
    if (params.limit) search.set('limit', String(params.limit))
    const suffix = search.size ? `?${search.toString()}` : ''
    return request<ConnectionPage>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections${suffix}`, { signal: params.signal })
  },
  getConnection: (orgID: string, connectionID: string, signal?: AbortSignal) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}`, { signal }),
  createConnection: (orgID: string, kind: string, payload: ConnectionCreate, csrfToken: string) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${kind === 'forge' ? 'forges' : kind === 'delivery' ? 'delivery' : `${kind}s`}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) }, csrfToken),
  testConnection: (orgID: string, connectionID: string, version: number, csrfToken: string) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}/test`, { method: 'POST', headers: { 'If-Match': versionTag(version) } }, csrfToken),
  rotateConnection: (orgID: string, connectionID: string, version: number, secret: string, csrfToken: string) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}/rotate`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify({ secret }) }, csrfToken),
  revokeConnection: (orgID: string, connectionID: string, version: number, csrfToken: string) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}`, { method: 'DELETE', headers: { 'If-Match': versionTag(version) } }, csrfToken),
  setPrivateRoute: (orgID: string, connectionID: string, version: number, route: { runner_id: string; host: string; cidrs: string[] }, csrfToken: string) => request<Connection>(`/api/v1/orgs/${encodeURIComponent(orgID)}/connections/${encodeURIComponent(connectionID)}/private-route`, { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify({ route }) }, csrfToken),
  getRunnerPools: (orgID: string, params: { cursor?: string; limit?: number; q?: string; state?: string; signal?: AbortSignal } = {}) => {
    const search = new URLSearchParams()
    if (params.cursor) search.set('cursor', params.cursor)
    if (params.limit) search.set('limit', String(params.limit))
    if (params.q) search.set('q', params.q)
    if (params.state) search.set('state', params.state)
    const suffix = search.size ? `?${search.toString()}` : ''
    return request<{ items: RunnerPool[]; next_cursor?: string; complete: boolean }>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools${suffix}`, { signal: params.signal })
  },
  getRunnerPool: (orgID: string, poolID: string, signal?: AbortSignal) => request<RunnerPool>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools/${encodeURIComponent(poolID)}`, { signal }),
  createRunnerPool: (orgID: string, payload: RunnerPoolInput, csrfToken: string) => request<RunnerPool>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) }, csrfToken),
  updateRunnerPool: (orgID: string, poolID: string, version: number, payload: RunnerPoolInput, csrfToken: string) => request<RunnerPool>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools/${encodeURIComponent(poolID)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': versionTag(version) }, body: JSON.stringify(payload) }, csrfToken),
  createEnrollment: (orgID: string, poolID: string, csrfToken: string) => request<EnrollmentToken>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools/${encodeURIComponent(poolID)}/enrollments`, { method: 'POST' }, csrfToken),
  getRunners: (orgID: string, poolID: string, signal?: AbortSignal) => request<RunnerPage>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runner-pools/${encodeURIComponent(poolID)}/runners`, { signal }),
  revokeRunner: (orgID: string, runnerID: string, version: number, csrfToken: string) => request<void>(`/api/v1/orgs/${encodeURIComponent(orgID)}/runners/${encodeURIComponent(runnerID)}`, { method: 'DELETE', headers: { 'If-Match': versionTag(version) } }, csrfToken),
  login: () => { window.location.assign('/auth/login') },
  logout: (csrfToken: string) => request<void>('/auth/logout', { method: 'POST' }, csrfToken),
  bootstrap: (name: string, token: string, csrfToken: string) => request<{ id: string; name: string }>('/auth/bootstrap', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, token }) }, csrfToken),
}
