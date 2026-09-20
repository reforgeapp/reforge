import type { components } from './schema'

export type APIError = components['schemas']['APIError']
export type Meta = components['schemas']['Meta']
export type Session = components['schemas']['Session']
export type RepositoryPage = components['schemas']['RepositoryPage']

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
  login: () => { window.location.assign('/auth/login') },
  logout: (csrfToken: string) => request<void>('/auth/logout', { method: 'POST' }, csrfToken),
}
