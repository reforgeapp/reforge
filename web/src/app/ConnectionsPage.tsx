import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api, ReforgeAPIError, type Connection, type ConnectionCreate } from '../api/client'
import { inventoryAPI } from '../api/inventory'
import { connectionQuery, connectionsListQuery, useSession } from './query'
import { Button, Dialog } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { Toolbar, SplitView, DetailPanel, SplitPlaceholder } from '../components/Workspace'
import { CustomProfilesPanel } from './CustomProfilesPanel'
import { AgentQualificationPanel } from './AgentQualificationPanel'

const profileOptions = ['openai:responses', 'anthropic:messages', 'google:gemini', 'compatible:chat_completions', 'compatible:ollama', 'compatible:vllm', 'compatible:responses']
const tabs = [
  { id: 'forge', label: 'Forges', kinds: ['forge'] },
  { id: 'models', label: 'Models & agents', kinds: ['model', 'agent'] },
  { id: 'delivery', label: 'Delivery', kinds: ['delivery'] },
] as const
const tabValue = (value: string | undefined): 'forge' | 'models' | 'delivery' => value === 'models' || value === 'delivery' ? value : 'forge'

export function ConnectionsPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const [formOpen, setFormOpen] = useState(false)
  const selectedID = search.connection ?? ''
  const csrf = session.data?.csrf_token ?? ''

  const tab = tabValue(search.connection_tab)
  const q = search.q ?? ''
  const state = search.state ?? ''
  const kinds = tabs.find(item => item.id === tab)?.kinds ?? []
  const result = useInfiniteQuery(connectionsListQuery(orgID, kinds.length === 1 ? kinds[0] : undefined))
  const loaded = [...new Map((result.data?.pages.flatMap(page => page.items) ?? []).map(item => [item.id, item])).values()]
  const lastPage = result.data?.pages.at(-1)

  const refresh = () => { void client.invalidateQueries({ queryKey: ['org', orgID, 'connections'] }) }
  const select = (id: string) => { void navigate({ search: previous => ({ ...previous, connection: id }) }) }
  const clear = () => { void navigate({ search: previous => { const next = { ...previous }; delete next.connection; return next } }) }
  const setFilter = (key: 'q' | 'state', value: string) => { void navigate({ search: previous => ({ ...previous, [key]: value || undefined }) }) }

  const rows = loaded.filter(item => kinds.includes(item.kind as never)).filter(item => !q.trim() || item.name.toLowerCase().includes(q.trim().toLowerCase())).filter(item => !state || item.state === state)
  const list = result.isLoading && !loaded.length ? <StatePanel kind="loading" title="Loading connections" detail="Fetching persisted integrations for this organisation." /> : result.error ? <StatePanel kind="error" title="Connections could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} /> : <DataTable caption={`${tabs.find(item => item.id === tab)?.label} connections`}><table><thead><tr><th>Name</th><th>Provider</th><th>State</th><th>Credential</th><th><span className="sr-only">Actions</span></th></tr></thead><tbody>{rows.map(connection => <tr key={connection.id}><td><button className="link-button" onClick={() => select(connection.id)}>{connection.name}</button></td><td>{connection.provider}</td><td><StatusBadge label={connection.state} tone={connection.state === 'healthy' ? 'green' : connection.state === 'revoked' ? 'red' : 'amber'} /></td><td>Version {connection.credential_version}</td><td><Button className="button button-sm" onClick={() => select(connection.id)}>Open</Button></td></tr>)}</tbody></table>{!rows.length && <EmptyTable label={`No ${tabs.find(item => item.id === tab)?.label.toLowerCase()} match these filters.`} />}{lastPage && !lastPage.complete && <div className="table-note"><Button disabled={result.isFetching} onClick={() => void result.fetchNextPage()}>{result.isFetchingNextPage ? 'Loading…' : 'Load more connections'}</Button></div>}</DataTable>

  return <div className="stack">
    <Toolbar label="Connection filters">
      <label>Search<input value={q} onChange={event => setFilter('q', event.target.value)} placeholder="Connection name" /></label>
      <label>State<select value={state} onChange={event => setFilter('state', event.target.value)}><option value="">All states</option><option value="healthy">Healthy</option><option value="unverified">Unverified</option><option value="degraded">Degraded</option><option value="disabled">Disabled</option><option value="revoked">Revoked</option></select></label>
      <Button onClick={refresh}>Refresh</Button>
      <Button className="button button-primary" onClick={() => setFormOpen(true)}>Add connection</Button>
    </Toolbar>
    <div className="row-actions" role="group" aria-label="Connection kind">
      {tabs.map(item => <Button key={item.id} aria-pressed={tab === item.id} onClick={() => { void navigate({ search: previous => ({ ...previous, connection_tab: item.id, connection: undefined }) }) }}>{item.label}</Button>)}
      <span className="toolbar-meta">Credentials are write-only. Capability state comes from the server probe.</span>
    </div>
    <SplitView listLabel="Connections" selected={!!selectedID} onBack={clear} list={list} detail={selectedID ? <ConnectionDetail connectionID={selectedID} orgID={orgID} csrf={csrf} onClose={clear} onRefresh={refresh} /> : <SplitPlaceholder label="Select a connection to view capability, billing route and actions." />} />
    {tab === 'models' && <details><summary>Custom command profiles</summary><CustomProfilesPanel orgID={orgID} /></details>}
    <ConnectionForm open={formOpen} orgID={orgID} csrf={csrf} onClose={() => setFormOpen(false)} onCreated={() => { setFormOpen(false); refresh() }} />
  </div>
}

function ConnectionDetail({ connectionID, orgID, csrf, onClose, onRefresh }: { connectionID: string; orgID: string; csrf: string; onClose: () => void; onRefresh: () => void }) {
  const result = useQuery(connectionQuery(orgID, connectionID))
  const [secret, setSecret] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [poolID, setPoolID] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [detailTab, setDetailTab] = useState<'overview' | 'configuration' | 'qualification' | 'actions'>('overview')
  const pools = useInfiniteQuery({ queryKey: ['org', orgID, 'connection-detail-runner-pools'], queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const runners = useInfiniteQuery({ queryKey: ['org', orgID, 'connection-detail-runners', poolID], queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, enabled: !!poolID, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? []).filter(runner => runner.state === 'active').map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))
  const visibleRunnerOptions = runnerID && !runnerOptions.some(runner => runner.id === runnerID) ? [{ id: runnerID, name: 'Selected runner unavailable', pool_name: 'Unknown' }, ...runnerOptions] : runnerOptions
  useEffect(() => { const connection = result.data; if (connection) { setRouteHost(connection.private_route?.host ?? ''); setRunnerID(connection.private_route?.runner_id ?? ''); setCIDRs(connection.private_route?.cidrs.join(', ') ?? '') } }, [result.data])
  useEffect(() => { if (!result.isSuccess) setSecret('') }, [result.isSuccess])
  const run = async (name: string, action: () => Promise<unknown>) => { setBusy(name); setError(''); try { await action(); setSecret(''); onRefresh(); await result.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy('') } }
  if (result.isLoading) return <StatePanel kind="loading" title="Loading connection" detail="Fetching the current server version." />
  if (result.error || !result.data) return <StatePanel kind="error" title="Connection could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} />
  const connection = result.data
  const capabilities = Object.entries(connection.capabilities)
  return <DetailPanel title={connection.name} status={<p><StatusBadge label={connection.state} tone={connection.state === 'healthy' ? 'green' : connection.state === 'revoked' ? 'red' : 'amber'} /> <span className="table-meta">{connection.kind}/{connection.provider} · version {connection.version}</span></p>} actions={<>
    {connection.kind !== 'agent' && <Button disabled={!!busy || !csrf} onClick={() => void run('test', () => api.testConnection(orgID, connection.id, connection.version, csrf))}>{busy === 'test' ? 'Testing…' : 'Test capability'}</Button>}
    <Button disabled={!!busy || !csrf} onClick={() => void run('revoke', () => api.revokeConnection(orgID, connection.id, connection.version, csrf))}>Revoke</Button>
    <Button onClick={onClose}>Close</Button>
  </>}>
    <nav className="detail-tabs" aria-label="Connection detail"><Button aria-pressed={detailTab === 'overview'} onClick={() => setDetailTab('overview')}>Overview</Button>{connection.kind !== 'agent' && <Button aria-pressed={detailTab === 'configuration'} onClick={() => setDetailTab('configuration')}>Configuration</Button>}{connection.kind === 'agent' && <Button aria-pressed={detailTab === 'qualification'} onClick={() => setDetailTab('qualification')}>Qualification</Button>}<Button aria-pressed={detailTab === 'actions'} onClick={() => setDetailTab('actions')}>Actions</Button></nav>
    <dl className="detail-list"><div><dt>Endpoint</dt><dd>{connection.endpoint}</dd></div><div><dt>Billing route</dt><dd>{connection.settings.billing_route || 'Unknown'}</dd></div><div><dt>Model / account</dt><dd>{connection.settings.model || connection.settings.namespace || 'Not set'}</dd></div><div><dt>Runtime / protocol</dt><dd>{connection.settings.runtime_version || connection.settings.profile || 'Not set'}</dd></div><div><dt>Last verified</dt><dd>{connection.verified_at ? new Date(connection.verified_at).toLocaleString() : 'Not verified'}</dd></div><div><dt>Reason</dt><dd>{connection.reason || 'No server reason provided.'}</dd></div></dl>
    {detailTab === 'overview' && capabilities.length > 0 && <div><h3>Capabilities</h3><ul className="compact-list">{capabilities.map(([name, capability]) => <li key={name}><StatusBadge label={capability.state} tone={capability.state === 'supported' ? 'green' : capability.state === 'unsupported' ? 'red' : 'amber'} /> <strong>{name}</strong>: {capability.reason}</li>)}</ul></div>}
    {detailTab === 'qualification' && connection.kind === 'agent' && <AgentQualificationPanel orgID={orgID} connectionID={connection.id} provider={connection.provider} />}
    {detailTab === 'configuration' && connection.kind !== 'agent' && <fieldset><legend>Private route</legend><div className="form-grid"><label>Runner pool<select aria-label="Runner pool" value={poolID} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}><option value="">Choose pool</option>{poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select></label><label>Runner<select aria-label="Runner" value={runnerID} onChange={event => setRunnerID(event.target.value)}><option value="">Choose active runner</option>{runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}</select></label><label>Host<input value={routeHost} onChange={event => setRouteHost(event.target.value)} /></label><label className="wide">CIDRs<input value={cidrs} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8, 192.168.0.0/16" /></label></div>{pools.error && <p className="error-text" role="alert">Runner pools unavailable: {message(pools.error)}</p>}{runners.error && <p className="error-text" role="alert">Runners unavailable: {message(runners.error)}</p>}{pools.error && <p className="error-text" role="alert">Runner pools unavailable: {message(pools.error)}</p>}{runners.error && <p className="error-text" role="alert">Runners unavailable: {message(runners.error)}</p>}{pools.hasNextPage && <Button onClick={() => void pools.fetchNextPage()} disabled={pools.isFetchingNextPage}>{pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}{runners.hasNextPage && <Button onClick={() => void runners.fetchNextPage()} disabled={runners.isFetchingNextPage}>{runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}{!runnerOptions.length && poolID && !runners.isLoading && <p className="table-meta">No active enrolled runners in selected pool.</p>}<Button disabled={!runnerID || !routeHost || !!busy || !csrf} title={!csrf ? 'Sign in again to approve a private route.' : !runnerID ? 'Choose an active enrolled runner first.' : undefined} onClick={() => void run('route', () => api.setPrivateRoute(orgID, connection.id, connection.version, { runner_id: runnerID, host: routeHost, cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) }, csrf))}>Approve private route</Button></fieldset>}
    {detailTab === 'actions' && <>{connection.kind !== 'agent' && <><label>Rotate secret<input type="password" autoComplete="new-password" value={secret} onChange={event => setSecret(event.target.value)} /></label><Button disabled={!secret || !!busy || !csrf} onClick={() => void run('rotate', () => api.rotateConnection(orgID, connection.id, connection.version, secret, csrf))}>{busy === 'rotate' ? 'Rotating…' : 'Rotate credential'}</Button></>}{connection.kind === 'forge' && <WebhookPanel orgID={orgID} connection={connection} csrf={csrf} />}</>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </DetailPanel>
}

function WebhookPanel({ orgID, connection, csrf }: { orgID: string; connection: Connection; csrf: string }) {
  const client = useQueryClient()
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const state = useQuery({ queryKey: ['org', orgID, 'connections', connection.id, 'webhook'], queryFn: ({ signal }) => inventoryAPI.webhook(orgID, connection.id, signal) })
  const unconfigured = state.error instanceof ReforgeAPIError && state.error.code === 'webhook_unconfigured'
  const version = state.data?.version ?? (unconfigured ? 0 : undefined)
  const issue = async () => { if (version === undefined || !csrf) return; setSecret(''); setBusy(true); setError(''); try { const issued = await inventoryAPI.issueWebhook(orgID, connection.id, version, csrf); setSecret(issued.secret); await client.invalidateQueries({ queryKey: ['org', orgID, 'connections', connection.id, 'webhook'] }) } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  const revoke = async () => { if (!state.data || !csrf) return; setBusy(true); setError(''); try { await inventoryAPI.revokeWebhook(orgID, connection.id, state.data.version, csrf); setSecret(''); await client.invalidateQueries({ queryKey: ['org', orgID, 'connections', connection.id, 'webhook'] }) } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  if (state.isLoading) return <fieldset><legend>Inventory webhook</legend><p className="table-meta">Loading webhook state…</p></fieldset>
  if (state.error && !unconfigured) return <fieldset><legend>Inventory webhook</legend><p className="error-text" role="alert">Webhook state unavailable: {message(state.error)}</p></fieldset>
  return <fieldset><legend>Inventory webhook</legend><p className="table-meta">{state.data?.revoked ? 'Webhook revoked.' : state.data ? `POST ${window.location.origin}${state.data.path}` : 'No webhook configured.'}</p>{secret && <label>One-time webhook secret<input readOnly type="password" value={secret} /></label>}{error && <p className="error-text" role="alert">{error}</p>}<div className="row-actions"><Button disabled={busy || !csrf || version === undefined} onClick={() => void issue()}>{state.data && !state.data.revoked ? 'Rotate webhook secret' : 'Issue webhook secret'}</Button><Button disabled={busy || !csrf || !state.data || state.data.revoked} onClick={() => void revoke()}>Revoke webhook</Button>{secret && <Button onClick={() => setSecret('')}>Clear webhook secret</Button>}</div></fieldset>
}

function ConnectionForm({ open, orgID, csrf, onClose, onCreated }: { open: boolean; orgID: string; csrf: string; onClose: () => void; onCreated: () => void }) {
  const [kind, setKind] = useState('forge')
  const [provider, setProvider] = useState('github')
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [namespace, setNamespace] = useState('')
  const [profile, setProfile] = useState('responses')
  const [model, setModel] = useState('')
  const [authKind, setAuthKind] = useState('token')
  const [billingRoute, setBillingRoute] = useState('forge')
  const [appID, setAppID] = useState('')
  const [installationID, setInstallationID] = useState('')
  const [caPEM, setCAPEM] = useState('')
  const [secret, setSecret] = useState('')
  const [privateRoute, setPrivateRoute] = useState(false)
  const [runnerID, setRunnerID] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [poolID, setPoolID] = useState('')
  const pools = useInfiniteQuery({ queryKey: ['org', orgID, 'connection-form-runner-pools'], queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, enabled: open, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const runners = useInfiniteQuery({ queryKey: ['org', orgID, 'connection-form-runners', poolID], queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, enabled: open && !!poolID, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? []).filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now()).map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))
  useEffect(() => {
    setSecret(''); setAppID(''); setInstallationID(''); setNamespace(''); setPrivateRoute(false); setRunnerID(''); setRouteHost(''); setCIDRs('')
    if (kind === 'forge' || kind === 'delivery') { setProvider('github'); setAuthKind('token'); setBillingRoute('forge'); setProfile(''); setModel('') }
    if (kind === 'model') { setProvider('openai'); setAuthKind('api_key'); setBillingRoute('direct_api'); setProfile('responses') }
    if (kind === 'agent') { setProvider('codex'); setAuthKind('official_runtime'); setBillingRoute('subscription'); setProfile(''); setModel('') }
  }, [kind])
  useEffect(() => { if (!open) { setSecret(''); setError('') } }, [open])
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    const settings = { auth_kind: authKind, billing_route: billingRoute, ...(namespace ? { namespace } : {}), ...((kind === 'model' || kind === 'agent') && model ? { model } : {}), ...(kind === 'model' && profile ? { profile } : {}), ...(appID ? { app_id: appID } : {}), ...(installationID ? { installation_id: installationID } : {}), ...(caPEM ? { ca_pem: caPEM } : {}) }
    const payload: ConnectionCreate = { kind, provider, name, endpoint, settings, ...(secret ? { secret } : {}), ...(privateRoute ? { private_route: { runner_id: runnerID, host: routeHost, cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) } } : {}) }
    try { await api.createConnection(orgID, kind, payload, csrf); setSecret(''); onCreated() } catch (reason) { setError(message(reason)) } finally { setBusy(false) }
  }
  const forgeKind = kind === 'forge' || kind === 'delivery'
  const agentKind = kind === 'agent'
  const secretRequired = !agentKind && !(kind === 'model' && provider === 'compatible')
  const githubForge = forgeKind && provider === 'github'
  const routeIncomplete = privateRoute && (!runnerID || !routeHost || !cidrs.trim())
  return <Dialog open={open} title="Add connection" onClose={onClose}><form className="form-stack" onSubmit={submit}><div className="form-grid"><label>Kind<select value={kind} onChange={event => setKind(event.target.value)}><option value="forge">Forge</option><option value="model">Model</option><option value="agent">Agent</option><option value="delivery">Delivery</option></select></label><label>Provider<select value={kind === 'model' ? `${provider}:${profile}` : provider} onChange={event => { const [nextProvider, nextProfile] = event.target.value.split(':'); setProvider(nextProvider); setSecret(''); setAppID(''); setInstallationID(''); if (nextProfile) setProfile(nextProfile); else if (forgeKind) { setAuthKind('token'); setProfile('') } }}>{(kind === 'model' ? profileOptions : forgeKind ? ['github', 'gitlab', 'gitea'] : agentKind ? ['codex', 'claude_code', 'agy', 'gemini_cli', 'custom_command'] : ['github', 'gitlab', 'gitea']).map(option => <option key={option} value={option}>{option}</option>)}</select></label><label className="wide">Name<input required value={name} onChange={event => setName(event.target.value)} /></label><label className="wide">Endpoint<input required type="url" value={endpoint} onChange={event => setEndpoint(event.target.value)} placeholder="https://…" /></label>{forgeKind && <label>Namespace<input value={namespace} onChange={event => setNamespace(event.target.value)} placeholder="organisation or group" /></label>}{githubForge && <label>Auth<select value={authKind} onChange={event => { setAuthKind(event.target.value); setSecret(''); setAppID(''); setInstallationID('') }}><option value="token">Token</option><option value="github_app">GitHub App</option></select></label>}{githubForge && authKind === 'github_app' && <><label>App ID<input required value={appID} onChange={event => setAppID(event.target.value)} /></label><label>Installation ID<input required value={installationID} onChange={event => setInstallationID(event.target.value)} /></label></>}{forgeKind && !githubForge && <input type="hidden" value="token" readOnly />}{(kind === 'model' || kind === 'agent') && <label>Model ID<input required={!agentKind || provider === 'custom_command'} value={model} onChange={event => setModel(event.target.value)} /></label>}{agentKind && provider === 'custom_command' && <p className="table-meta">Custom command runs bind an approved profile per repair and a quota budget route. Exit 0 is never a validated repair.</p>}{kind === 'model' && <label>Auth<select value={authKind} onChange={event => setAuthKind(event.target.value)}><option value="api_key">API key</option></select></label>}{kind === 'model' && <label>Billing route<select value={billingRoute} onChange={event => setBillingRoute(event.target.value)}><option value="direct_api">Direct API</option></select></label>}{agentKind && <label>Billing route<select value={billingRoute} onChange={event => setBillingRoute(event.target.value)}><option value="subscription">Subscription</option></select></label>}<label className="wide">CA certificate<input value={caPEM} onChange={event => setCAPEM(event.target.value)} placeholder="Optional PEM CA certificate" /></label>{!agentKind && <label className="wide">Secret<input type="password" autoComplete="new-password" required={secretRequired} value={secret} onChange={event => setSecret(event.target.value)} /></label>}<label className="checkbox-label"><input type="checkbox" checked={privateRoute} onChange={event => setPrivateRoute(event.target.checked)} /> Private route via enrolled runner</label>{privateRoute && <><label>Runner pool<select aria-label="Runner pool" value={poolID} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}><option value="">Choose active pool</option>{poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select></label><label>Runner<select value={runnerID} onChange={event => setRunnerID(event.target.value)}><option value="">Choose active runner</option>{runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}</select></label><label>Route host<input value={routeHost} onChange={event => setRouteHost(event.target.value)} /></label><label className="wide">Approved CIDRs<input value={cidrs} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>{pools.error && <p className="error-text" role="alert">Runner pools unavailable: {message(pools.error)}</p>}{runners.error && <p className="error-text" role="alert">Runners unavailable: {message(runners.error)}</p>}{pools.hasNextPage && <Button onClick={() => void pools.fetchNextPage()} disabled={pools.isFetchingNextPage}>{pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}{runners.hasNextPage && <Button onClick={() => void runners.fetchNextPage()} disabled={runners.isFetchingNextPage}>{runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}{!runnerOptions.length && poolID && !runners.isLoading && <p className="table-meta">No active enrolled runners available in selected pool.</p>}</>}</div>{routeIncomplete && <p className="table-meta">Choose an active runner, route host and approved CIDR before saving.</p>}{!csrf && <p className="table-meta">Write actions unavailable until the session is refreshed.</p>}{error && <p className="error-text" role="alert">{error}</p>}<div className="dialog-actions"><Button type="button" onClick={onClose}>Cancel</Button><Button className="button button-primary" disabled={busy || !csrf || routeIncomplete} title={!csrf ? 'Refresh session before creating a connection.' : routeIncomplete ? 'Complete private route fields before saving.' : undefined}>{busy ? 'Saving…' : 'Create connection'}</Button></div></form></Dialog>
}

function message(value: unknown) { return value instanceof Error ? value.message : 'The server returned an unknown error.' }
