import { apiRequest, type Connection } from './client'
import type { ModelCatalogItem } from './model-connections'

const base = (orgID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}/chatgpt`
export type ChatGPTSetup = { id: string; name: string; account: string; model?: string; models: ModelCatalogItem[] }
export const chatGPTAPI = {
  status: (orgID: string, signal?: AbortSignal) => apiRequest<{ available: boolean; reason?: string }>(base(orgID), { signal }),
  begin: (orgID: string, name: string, csrf: string, connection?: Connection, signal?: AbortSignal) => apiRequest<{ url: string; helper?: string }>(`${base(orgID)}/setups`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, ...(connection ? { connection_id: connection.id, version: connection.version } : {}) }), signal }, csrf),
  setup: (orgID: string, id: string, signal?: AbortSignal) => apiRequest<ChatGPTSetup>(`${base(orgID)}/setups/${encodeURIComponent(id)}`, { signal }),
  save: (orgID: string, id: string, model: string, csrf: string) => apiRequest<Connection>(`${base(orgID)}/setups/${encodeURIComponent(id)}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ model }) }, csrf),
}

export async function continueWithChatGPT(orgID: string, name: string, csrf: string, connection?: Connection, signal?: AbortSignal) {
  const result = await chatGPTAPI.begin(orgID, name, csrf, connection, signal)
  signal?.throwIfAborted()
  const location = new URL(result.url)
  if (location.origin !== 'https://auth.openai.com' || location.pathname !== '/api/accounts/authorize' || location.username || location.password) throw new Error('ChatGPT sign-in address could not be verified.')
  if (result.helper) {
    const file = URL.createObjectURL(new Blob([result.helper], { type: 'text/x-python;charset=utf-8' }))
    const download = document.createElement('a')
    download.href = file
    download.download = 'reforge-chatgpt.py'
    download.click()
    setTimeout(() => URL.revokeObjectURL(file), 1000)
    return 'helper' as const
  }
  window.location.assign(location.href)
  return 'redirect' as const
}
