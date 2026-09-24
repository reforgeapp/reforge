import { apiRequest } from './client'

export type ModelCatalogItem = {
  id: string
  name: string
  disabled?: boolean
  reason?: string
}

export type ModelCatalogSettings = {
  profile: string
  auth_kind: 'api_key'
  billing_route: 'direct_api'
  ca_pem?: string
}

export type ModelCatalogInput = {
  provider: string
  endpoint: string
  secret?: string
  settings: ModelCatalogSettings
}

export type ModelCatalog = {
  items: ModelCatalogItem[]
}

export const modelConnectionsAPI = {
  catalog: (orgID: string, input: ModelCatalogInput, csrfToken: string, signal?: AbortSignal) =>
    apiRequest<ModelCatalog>(
      `/api/v1/orgs/${encodeURIComponent(orgID)}/connections/model-catalog`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
        signal,
      },
      csrfToken,
    ),
}
