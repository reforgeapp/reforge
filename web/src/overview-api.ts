import type { components } from './api/schema'
import { apiRequest } from './api/client'

export type Overview = components['schemas']['Overview']

export const overviewAPI = {
  get: (orgID: string, signal?: AbortSignal) => apiRequest<Overview>(`/api/v1/orgs/${encodeURIComponent(orgID)}/overview`, { signal }),
}
