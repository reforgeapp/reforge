import type { components } from './schema'
import { apiRequest } from './client'

type Repository = components['schemas']['Repository']
type RepositoryPage = components['schemas']['RepositoryPage']
type InventoryJob = components['schemas']['InventoryJob']
type InventoryJobPage = components['schemas']['InventoryJobPage']
type CandidatePage = components['schemas']['InventoryCandidatePage']
type ImportInput = components['schemas']['InventoryImportInput']
type InventoryWebhook = components['schemas']['InventoryWebhook']
type IssuedWebhook = components['schemas']['IssuedInventoryWebhook']
type ChangePage = components['schemas']['InventoryChangePage']
type TeamPage = components['schemas']['TeamPage']
type MaintenanceConfig = components['schemas']['MaintenanceConfig']
type DiscoveryScan = components['schemas']['DiscoveryScan']

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`
const query = (values: Record<string, string | number | AbortSignal | undefined>) => {
  const params = new URLSearchParams()
  Object.entries(values).forEach(([key, value]) => { if (key !== 'signal' && value !== undefined && value !== '') params.set(key, String(value)) })
  return params.size ? `?${params.toString()}` : ''
}

export const inventoryAPI = {
  repositories: (orgID: string, params: { cursor?: string; limit?: number; q?: string; provider?: string; team_id?: string; status?: string; signal?: AbortSignal } = {}) => apiRequest<RepositoryPage>(path(orgID, `/repositories${query(params)}`), { signal: params.signal }),
  repository: (orgID: string, repositoryID: string, signal?: AbortSignal) => apiRequest<Repository>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}`), { signal }),
  changes: (orgID: string, repositoryID: string, params: { cursor?: string; limit?: number; signal?: AbortSignal } = {}) => apiRequest<ChangePage>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/changes${query(params)}`), { signal: params.signal }),
  teams: async (orgID: string, signal?: AbortSignal) => collectPages<TeamPage['items'][number]>(cursor => apiRequest<TeamPage>(path(orgID, `/teams${query({ cursor, limit: 100 })}`), { signal }), signal),
  connections: async (orgID: string, signal?: AbortSignal) => collectPages<{ id: string; name: string; provider: string; endpoint: string; kind: string; version: number; state?: string }>(cursor => apiRequest<{ items: Array<{ id: string; name: string; provider: string; endpoint: string; kind: string; version: number; state?: string }>; next_cursor?: string; complete: boolean }>(path(orgID, `/connections${query({ cursor, kind: 'forge', limit: 100 })}`), { signal }), signal),
  startSync: (orgID: string, connectionID: string, namespace: string, csrf: string) => apiRequest<InventoryJob>(path(orgID, '/inventory-syncs'), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ connection_id: connectionID, ...(namespace ? { namespace } : {}) }) }, csrf),
  jobs: (orgID: string, signal?: AbortSignal) => apiRequest<InventoryJobPage>(path(orgID, '/inventory-syncs?limit=20'), { signal }),
  job: (orgID: string, syncID: string, signal?: AbortSignal) => apiRequest<InventoryJob>(path(orgID, `/inventory-syncs/${encodeURIComponent(syncID)}`), { signal }),
  cancel: (orgID: string, syncID: string, version: number, csrf: string) => apiRequest<InventoryJob>(path(orgID, `/inventory-syncs/${encodeURIComponent(syncID)}`), { method: 'DELETE', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) } }, csrf),
  candidates: (orgID: string, syncID: string, cursor?: string, signal?: AbortSignal) => apiRequest<CandidatePage>(path(orgID, `/inventory-syncs/${encodeURIComponent(syncID)}/candidates${query({ cursor, limit: 200 })}`), { signal }),
  import: (orgID: string, syncID: string, version: number, input: ImportInput, csrf: string) => apiRequest<InventoryJob>(path(orgID, `/inventory-syncs/${encodeURIComponent(syncID)}/import`), { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) }, body: JSON.stringify(input) }, csrf),
  webhook: (orgID: string, connectionID: string, signal?: AbortSignal) => apiRequest<InventoryWebhook>(path(orgID, `/connections/${encodeURIComponent(connectionID)}/webhook`), { signal }),
  issueWebhook: (orgID: string, connectionID: string, version: number, csrf: string) => apiRequest<IssuedWebhook>(path(orgID, `/connections/${encodeURIComponent(connectionID)}/webhook`), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) } }, csrf),
  revokeWebhook: (orgID: string, connectionID: string, version: number, csrf: string) => apiRequest<void>(path(orgID, `/connections/${encodeURIComponent(connectionID)}/webhook`), { method: 'DELETE', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) } }, csrf),
  maintenance: (orgID: string, repositoryID: string, signal?: AbortSignal) => apiRequest<MaintenanceConfig>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/maintenance`), { signal }),
  updateMaintenance: (orgID: string, repositoryID: string, version: number, config: MaintenanceConfig, csrf: string) => apiRequest<MaintenanceConfig>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/maintenance`), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': tag(version) }, body: JSON.stringify(config) }, csrf),
  discovery: (orgID: string, repositoryID: string, signal?: AbortSignal) => apiRequest<DiscoveryScan>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/discovery`), { signal }),
  startDiscovery: (orgID: string, repositoryID: string, csrf: string) => apiRequest<DiscoveryScan>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/discovery`), { method: 'POST' }, csrf),
}

async function collectPages<T>(fetchPage: (cursor?: string) => Promise<{ items: T[]; next_cursor?: string; complete: boolean }>, signal?: AbortSignal) {
  const items: T[] = []
  let cursor: string | undefined
  const seen = new Set<string>()
  for (let page = 0; page < 20; page += 1) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    const result = await fetchPage(cursor)
    items.push(...result.items)
    if (result.complete) return { items, complete: true }
    if (!result.next_cursor) throw new Error('Pagination stopped before completion.')
    if (seen.has(result.next_cursor)) throw new Error('Pagination cursor repeated before completion.')
    seen.add(result.next_cursor)
    cursor = result.next_cursor
  }
  throw new Error('Pagination exceeded 20 pages before completion.')
}
