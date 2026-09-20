import { apiRequest } from './api/client'

export type AuditEvent = {
  id: string
  actor_id: string
  action: string
  object_id: string
  request_id: string
  repository_id?: string
  created_at: string
  data: Record<string, unknown>
}

export type AuditEventPage = { items: AuditEvent[]; complete: boolean; next_cursor?: string }

export type AuditEventParams = {
  repository_id?: string
  actor_id?: string
  action?: string
  since?: string
  until?: string
  cursor?: string
  limit?: number
  signal?: AbortSignal
}

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`

function query(params: AuditEventParams) {
  const values = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (key !== 'signal' && value !== undefined && value !== '') values.set(key, String(value))
  }
  return values.size ? `?${values.toString()}` : ''
}

export const auditAPI = {
  events: (orgID: string, params: AuditEventParams = {}) => apiRequest<AuditEventPage>(path(orgID, `/audit-events${query(params)}`), { signal: params.signal }),
  exportPage: async (orgID: string, params: AuditEventParams = {}) => {
    const response = await fetch(path(orgID, `/audit-events/export${query(params)}`), { credentials: 'same-origin', headers: { Accept: 'application/x-ndjson' }, signal: params.signal })
    if (!response.ok) {
      let message = `Export failed (${response.status})`
      try {
        const body = await response.json() as { message?: string }
        if (body.message) message = body.message
      } catch { }
      throw new Error(message)
    }
    return { blob: await response.blob(), nextCursor: response.headers.get('X-Next-Cursor'), complete: response.headers.get('X-Export-Complete') === 'true' }
  },
}
