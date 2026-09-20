import type { Page } from '@playwright/test'
import type { RepairFixture } from './repair-fixture'

type Connection = { id: string; version: number }
type PolicyLayer = { scope: { kind: string; id: string }; version_id: string; binding_version: number }
type Effective = { hash: string; layers: PolicyLayer[]; policy: Record<string, unknown> }
type PolicyVersion = { id: string; policy: Record<string, unknown> }
type Budget = { version: number; [key: string]: unknown }

export type RepairAuthority = {
  modelID: string
  modelName: string
  modelRoute: string
  cleanup: () => Promise<void>
}

export async function configureRepairAuthority(page: Page, fixture: RepairFixture): Promise<RepairAuthority> {
  if (process.env.REFORGE_ISOLATED_BROWSER_DATABASE !== '1' || process.env.REFORGE_BASE_URL !== 'http://127.0.0.1:8081') throw new Error('repair authority requires isolated browser database at http://127.0.0.1:8081')
  const origin = process.env.REFORGE_BASE_URL
  const orgID = fixture.orgID
  const modelName = process.env.REFORGE_LIVE_REPAIR_MODEL ?? 'qwen3:1.7b'
  const modelEndpoint = validateModelEndpoint(process.env.REFORGE_BROWSER_MODEL_ENDPOINT ?? 'http://127.0.0.1:55435')
  const session = await page.request.get('/api/v1/session')
  if (!session.ok()) throw new Error(`repair authority session failed HTTP${session.status()}`)
  const { csrf_token: csrf } = await session.json() as { csrf_token: string }
  const api = async <T>(method: string, path: string, data?: unknown, headers: Record<string, string> = {}): Promise<T> => {
    const response = await page.request.fetch(path, {
      method,
      data,
      headers: {
        Accept: 'application/json',
        Origin: origin,
        ...(data === undefined ? {} : { 'Content-Type': 'application/json' }),
        ...(method === 'GET' ? {} : { 'X-CSRF-Token': csrf }),
        ...headers,
      },
    })
    if (!response.ok()) throw new Error(`repair authority ${method} ${path} failed HTTP${response.status()}`)
    return response.status() === 204 ? undefined as T : await response.json() as T
  }

  const getOptional = async <T>(path: string): Promise<T | undefined> => {
    const response = await page.request.get(path, { headers: { Accept: 'application/json', Origin: origin } })
    if (response.status() === 404) return undefined
    if (response.status() === 409 && (await response.json() as {code?: string}).code === 'budget_unconfigured') return undefined
    if (!response.ok()) throw new Error(`repair authority GET ${path} failed HTTP${response.status()}`)
    return await response.json() as T
  }
  let effective = await api<Effective>('GET', `/api/v1/orgs/${orgID}/policies/effective?repository_id=${encodeURIComponent(fixture.repositoryID)}`)
  let orgLayer = effective.layers.find(layer => layer.scope.kind === 'organisation' && layer.scope.id === orgID)
  let originalVersion: PolicyVersion
  if (orgLayer) {
    originalVersion = await api<PolicyVersion>('GET', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(orgLayer.version_id)}`)
  } else {
    const baselinePolicy = { schema: 'maintenance/v1', allow: { recipes: [], models: [], routes: [], merge_methods: [], environments: [], workflows: [] }, deny: [], forbidden_paths: [], limits: {}, required: [], defaults: {} }
    const baseline = await api<PolicyVersion>('POST', `/api/v1/orgs/${orgID}/policies/versions`, { scope: { kind: 'organisation', id: orgID }, policy: baselinePolicy, reason: 'Seed disposable repair authority baseline' })
    const simulation = await api<{ hash: string }>('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(baseline.id)}/simulate`, { repository_id: fixture.repositoryID, input: { action: 'repair' } })
    const activated = await api<{ version: number }>('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(baseline.id)}/activate`, { repository_id: fixture.repositoryID, simulation_hash: simulation.hash, reason: 'Seed disposable repair authority baseline' }, { 'If-Match': '"0"' })
    originalVersion = baseline
    orgLayer = { scope: { kind: 'organisation', id: orgID }, version_id: baseline.id, binding_version: activated.version }
    effective = await api<Effective>('GET', `/api/v1/orgs/${orgID}/policies/effective?repository_id=${encodeURIComponent(fixture.repositoryID)}`)
  }
  const existingBudget = await getOptional<Budget>(`/api/v1/orgs/${orgID}/budgets/organisation/${orgID}`)
  const originalBudget = existingBudget ?? await api<Budget>('PUT', `/api/v1/orgs/${orgID}/budgets/organisation/${orgID}`, { scope: { kind: 'organisation', id: orgID }, period: 'daily', caps: { micro_usd: 0, tokens: 0, milliseconds: 0, requests: 0, concurrency: 0 }, paused: false }, { 'If-Match': '"0"' })
  let model: Connection | undefined
  let budgetVersion = originalBudget.version
  let policyVersion: string | undefined
  let activatedBindingVersion: number | undefined
  let cleaning = false

  const cleanup = async () => {
    if (cleaning) return
    cleaning = true
    const failures: string[] = []
    if (policyVersion) {
      try {
        const current = await api<Effective>('GET', `/api/v1/orgs/${orgID}/policies/effective?repository_id=${encodeURIComponent(fixture.repositoryID)}`)
        const currentOrg = current.layers.find(layer => layer.scope.kind === 'organisation' && layer.scope.id === orgID)
        if (!currentOrg || currentOrg.version_id !== policyVersion || currentOrg.binding_version !== activatedBindingVersion) throw new Error('repair authority changed concurrently; refusing policy restore')
        const simulation = await api<{ hash: string }>('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(originalVersion.id)}/simulate`, { repository_id: fixture.repositoryID, input: { action: 'repair' } })
        await api('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(originalVersion.id)}/activate`, { repository_id: fixture.repositoryID, simulation_hash: simulation.hash, reason: 'Restore pre-test repair authority' }, { 'If-Match': `"${currentOrg.binding_version}"` })
      } catch (error) { failures.push(String(error)) }
    }
    if (budgetVersion !== originalBudget.version) {
      try {
        await api('PUT', `/api/v1/orgs/${orgID}/budgets/organisation/${orgID}`, originalBudget, { 'If-Match': `"${budgetVersion}"` })
      } catch (error) { failures.push(String(error)) }
    }
    if (model) {
      try {
        const current = await api<Connection>('GET', `/api/v1/orgs/${orgID}/connections/${model.id}`)
        await api('DELETE', `/api/v1/orgs/${orgID}/connections/${model.id}`, undefined, { 'If-Match': `"${current.version}"` })
      } catch (error) { failures.push(String(error)) }
    }
    if (failures.length) throw new Error(`repair authority cleanup failed: ${failures.join('; ')}`)
  }

  try {
    model = await api<Connection>('POST', `/api/v1/orgs/${orgID}/connections/models`, {
      kind: 'model', provider: 'compatible', name: `browser-repair-${Date.now()}`, endpoint: modelEndpoint,
      secret: 'local-ollama', settings: { auth_kind: 'api_key', billing_route: 'direct_api', model: modelName, profile: 'ollama' },
      private_route: { runner_id: fixture.runnerID, host: '127.0.0.1', cidrs: ['127.0.0.1/32'] },
    })
    await api('POST', `/api/v1/orgs/${orgID}/connections/${model.id}/test`, undefined, { 'If-Match': `"${model.version}"` })
  } catch (error) {
    try { await cleanup() } catch (cleanupError) { throw new AggregateError([error, cleanupError], 'repair authority setup and cleanup failed') }
    throw error
  }
  const modelID = model!.id
  const modelRoute = `${modelID}/default`
  try {
    const allow = (originalVersion.policy.allow ?? {}) as Record<string, unknown>
    const recipes = Array.isArray(allow.recipes) ? allow.recipes as string[] : []
    const models = Array.isArray(allow.models) ? allow.models as string[] : []
    const routes = Array.isArray(allow.routes) ? allow.routes as string[] : []
    const policy = { ...originalVersion.policy, allow: { ...allow, recipes: [...new Set([...recipes, 'go'])], models: [...new Set([...models, modelName])], routes: [...new Set([...routes, modelRoute])] } }
    const version = await api<{ id: string }>('POST', `/api/v1/orgs/${orgID}/policies/versions`, { scope: { kind: 'organisation', id: orgID }, policy, reason: 'Enable local repair acceptance authority' })
    policyVersion = version.id
    const simulation = await api<{ hash: string }>('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(version.id)}/simulate`, { repository_id: fixture.repositoryID, input: { action: 'repair' } })
    const activated = await api<{ version: number }>('POST', `/api/v1/orgs/${orgID}/policies/versions/${encodeURIComponent(version.id)}/activate`, { repository_id: fixture.repositoryID, simulation_hash: simulation.hash, reason: 'Enable local repair acceptance authority' }, { 'If-Match': `"${orgLayer.binding_version}"` })
    activatedBindingVersion = activated.version
    await api<{ version: number }>('PUT', `/api/v1/orgs/${orgID}/budget-routes/${modelID}`, { model: modelName, name: 'default', mode: 'priced', pricing_version: 'local-zero-cost', max_input_tokens: 100000, max_output_tokens: 1024, max_milliseconds: 180000, max_requests: 1 }, { 'If-Match': '"0"' })
    await api<Budget>('PUT', `/api/v1/orgs/${orgID}/budgets/organisation/${orgID}`, { scope: { kind: 'organisation', id: orgID }, period: 'daily', caps: { micro_usd: 0, tokens: 1000000, milliseconds: 900000, requests: 16, concurrency: 1 }, paused: false }, { 'If-Match': `"${originalBudget.version}"` }).then(value => { budgetVersion = value.version })
    return { modelID, modelName, modelRoute, cleanup }
  } catch (error) {
    try { await cleanup() } catch (cleanupError) { throw new AggregateError([error, cleanupError], 'repair authority setup and cleanup failed') }
    throw error
  }
}

function validateModelEndpoint(value: string): string {
  let endpoint: URL
  try {
    endpoint = new URL(value)
  } catch {
    throw new Error('repair authority model endpoint must be a loopback HTTP URL')
  }
  if (endpoint.protocol !== 'http:' || endpoint.hostname !== '127.0.0.1' || endpoint.username || endpoint.password || endpoint.search || endpoint.hash || (endpoint.pathname !== '/' && endpoint.pathname !== '')) throw new Error('repair authority model endpoint must be a loopback HTTP URL with no private path')
  return endpoint.origin
}
