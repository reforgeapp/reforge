import { apiRequest } from './api/client'

export type AlertSettings = { enabled: boolean; recipients: string[]; default_recipients: string[]; server_configured: boolean; smtp_address: string; smtp_username: string; smtp_from: string; smtp_security: 'starttls' | 'tls' | 'none'; smtp_verify: boolean; smtp_password_set: boolean; smtp_password?: string; version: number }
const path = (orgID: string, suffix = '') => `/api/v1/orgs/${encodeURIComponent(orgID)}/alerts${suffix}`
export const alertsAPI = {
  get: (orgID: string, signal?: AbortSignal) => apiRequest<AlertSettings>(path(orgID), { signal }),
  put: (orgID: string, value: AlertSettings, csrf: string) => apiRequest<AlertSettings>(path(orgID), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': `"${value.version}"` }, body: JSON.stringify(value) }, csrf),
  test: (orgID: string, csrf: string) => apiRequest<void>(path(orgID, '/test'), { method: 'POST' }, csrf),
}
