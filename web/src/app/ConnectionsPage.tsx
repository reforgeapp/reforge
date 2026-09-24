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
import { SplitView, DetailPanel } from '../components/Workspace'
import { CustomProfilesPanel } from './CustomProfilesPanel'
import { AgentQualificationPanel } from './AgentQualificationPanel'
import { ModelConnectionForm } from './ModelConnectionForm'
import { connectionProviderLabel, connectionStateStatus, modelProfileLabel } from '../model-providers'
import { Icon } from '../components/Icons'
import '../styles/connections.css'

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
  const forgeResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'forge'), enabled: tab === 'forge' })
  const modelResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'model'), enabled: tab === 'models' })
  const agentResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'agent'), enabled: tab === 'models' })
  const deliveryResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'delivery'), enabled: tab === 'delivery' })
  const activeResults = tab === 'forge' ? [forgeResult] : tab === 'models' ? [modelResult, agentResult] : [deliveryResult]
  const kinds = tabs.find(item => item.id === tab)?.kinds ?? []
  const loaded = [...new Map(activeResults.flatMap(result => result.data?.pages.flatMap(page => page.items) ?? []).map(item => [item.id, item])).values()]
  const error = activeResults.find(result => result.error)?.error
  const hasData = activeResults.some(result => result.data !== undefined)
  const loading = activeResults.some(result => result.isLoading) && !loaded.length
  const refreshing = activeResults.some(result => result.isFetching)
  const hasMore = activeResults.some(result => result.hasNextPage)
  const loadingMore = activeResults.some(result => result.isFetchingNextPage)

  const refresh = () => { void client.invalidateQueries({ queryKey: ['org', orgID, 'connections'] }) }
  const select = (id: string) => { void navigate({ search: previous => ({ ...previous, connection: id }) }) }
  const clear = () => { void navigate({ search: previous => { const next = { ...previous }; delete next.connection; return next } }) }
  const setFilter = (key: 'q' | 'state', value: string) => { void navigate({ search: previous => ({ ...previous, [key]: value || undefined }) }) }

  const normalizedQuery = q.trim().toLowerCase()
  const rows = loaded
    .filter(item => kinds.includes(item.kind as never))
    .filter(item => !normalizedQuery || `${item.name} ${item.provider}`.toLowerCase().includes(normalizedQuery))
    .filter(item => !state || item.state === state)
  const setTab = (value: typeof tab) => {
    void navigate({ search: previous => ({ ...previous, connection_tab: value, connection: undefined }) })
  }

  const list = <ConnectionList
    rows={rows}
    tab={tab}
    query={q}
    state={state}
    loading={loading}
    error={error}
    hasData={hasData}
    refreshing={refreshing}
    hasMore={hasMore}
    loadingMore={loadingMore}
    onRetry={() => { void Promise.all(activeResults.map(result => result.refetch())) }}
    onRefresh={refresh}
    onAdd={() => setFormOpen(true)}
    onSelect={select}
    onTabChange={setTab}
    onFilter={setFilter}
    onLoadMore={() => { void Promise.all(activeResults.filter(result => result.hasNextPage).map(result => result.fetchNextPage())) }}
  />

  return <div className="stack connections-page">
    <SplitView
      listLabel="Connections"
      selected={!!selectedID}
      onBack={clear}
      hideBack
      closeControl={{ label: 'Close connection details', onClose: clear }}
      list={list}
      detail={selectedID ? <ConnectionDetail connectionID={selectedID} orgID={orgID} csrf={csrf} onRefresh={refresh} /> : null}
    />
    {tab === 'models' && <details>
      <summary>Custom command profiles</summary>
      <CustomProfilesPanel orgID={orgID} />
    </details>}
    <ConnectionForm
      open={formOpen}
      orgID={orgID}
      csrf={csrf}
      onClose={() => setFormOpen(false)}
      onCreated={() => { setFormOpen(false); refresh() }}
    />
  </div>
}

type ConnectionTab = 'forge' | 'models' | 'delivery'

type ConnectionListProps = {
  rows: Connection[]
  tab: ConnectionTab
  query: string
  state: string
  loading: boolean
  error: unknown
  hasData: boolean
  refreshing: boolean
  hasMore: boolean
  loadingMore: boolean
  onRetry: () => void
  onRefresh: () => void
  onAdd: () => void
  onSelect: (id: string) => void
  onTabChange: (tab: ConnectionTab) => void
  onFilter: (key: 'q' | 'state', value: string) => void
  onLoadMore: () => void
}

function ConnectionList(props: ConnectionListProps) {
  if (props.loading) {
    return <StatePanel kind="loading" title="Loading connections" detail="Loading connections…" />
  }

  if (props.error && !props.hasData) {
    return <StatePanel kind="error" title="Connections unavailable" detail={message(props.error)} action={<Button onClick={props.onRetry}>Retry</Button>} />
  }

  const category = tabs.find(item => item.id === props.tab)?.label ?? 'Connections'

  return <div className="connections-inventory">
    {props.error ? <p className="connections-load-error" role="alert">Could not load all connection results. {message(props.error)} <Button onClick={props.onRetry}>Retry</Button></p> : null}
    <header className="connections-list-head">
      <h2>{props.rows.length} {props.hasMore ? 'shown' : props.rows.length === 1 ? 'connection' : 'connections'}</h2>
      <div className="connections-list-actions">
        <Button className="connections-refresh" aria-label="Refresh connections" title="Refresh" disabled={props.refreshing} onClick={props.onRefresh}>
          <Icon name="refresh" />
        </Button>
        <Button className="button button-primary" onClick={props.onAdd}>Add connection</Button>
      </div>
    </header>
    <nav className="connections-categories" aria-label="Connection category">
      {tabs.map(item => <Button key={item.id} aria-pressed={props.tab === item.id} onClick={() => props.onTabChange(item.id)}>{item.label}</Button>)}
    </nav>
    <div className="connections-filters" role="group" aria-label="Filter connections">
      <label className="connections-search">Search<input aria-label="Search" value={props.query} onChange={event => props.onFilter('q', event.target.value)} placeholder="Name or provider" /></label>
      <label>State<select aria-label="State" value={props.state} onChange={event => props.onFilter('state', event.target.value)}>
        <option value="">All states</option>
        <option value="healthy">Healthy</option>
        <option value="unverified">Unverified</option>
        <option value="degraded">Degraded</option>
        <option value="disabled">Disabled</option>
        <option value="revoked">Revoked</option>
      </select></label>
    </div>
    <DataTable caption={`${category} connections`}>
      <table className="connections-table">
        <thead><tr><th>Name</th><th>Provider</th><th>State</th><th>Credential</th><th><span className="sr-only">Actions</span></th></tr></thead>
        <tbody>{props.rows.map(connection => <ConnectionRow key={connection.id} connection={connection} onSelect={props.onSelect} />)}</tbody>
      </table>
      {!props.rows.length && <EmptyTable label={props.hasMore && (props.query.trim() || props.state) ? 'No matches in loaded results; more may be available.' : props.query.trim() || props.state ? 'No connections match these filters.' : `No ${category.toLowerCase()} found.`} />}
      {props.hasMore && <div className="table-note"><Button disabled={props.loadingMore} onClick={props.onLoadMore}>{props.loadingMore ? 'Loading…' : 'Load more'}</Button></div>}
    </DataTable>
  </div>
}

function ConnectionRow({ connection, onSelect }: { connection: Connection; onSelect: (id: string) => void }) {
  const status = connectionStateStatus(connection.provider, connection.settings.profile, connection.state)

  return <tr>
    <td><button className="link-button" onClick={() => onSelect(connection.id)}>{connection.name}</button></td>
    <td className="connection-provider">{connectionProviderLabel(connection.provider, connection.settings.profile)}</td>
    <td><span title={status.title}><StatusBadge label={status.label} tone={status.tone} /></span></td>
    <td>Version {connection.credential_version}</td>
    <td><Button className="button button-sm" onClick={() => onSelect(connection.id)}>Open</Button></td>
  </tr>
}

type ConnectionDetailProps = {
  connectionID: string
  orgID: string
  csrf: string
  onRefresh: () => void
}

function ConnectionDetail({ connectionID, orgID, csrf, onRefresh }: ConnectionDetailProps) {
  const result = useQuery(connectionQuery(orgID, connectionID))
  const [secret, setSecret] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [poolID, setPoolID] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [detailTab, setDetailTab] = useState<'overview' | 'configuration' | 'qualification' | 'actions'>('overview')
  const [confirmRevoke, setConfirmRevoke] = useState(false)
  const pools = useInfiniteQuery({
    queryKey: ['org', orgID, 'connection-detail-runner-pools'],
    queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const runners = useInfiniteQuery({
    queryKey: ['org', orgID, 'connection-detail-runners', poolID],
    queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: !!poolID,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? [])
    .filter(runner => runner.state === 'active')
    .map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))
  const visibleRunnerOptions = runnerID && !runnerOptions.some(runner => runner.id === runnerID)
    ? [{ id: runnerID, name: 'Selected runner unavailable', pool_name: 'Unknown' }, ...runnerOptions]
    : runnerOptions

  useEffect(() => {
    const connection = result.data
    if (!connection) return
    setRouteHost(connection.private_route?.host ?? '')
    setRunnerID(connection.private_route?.runner_id ?? '')
    setCIDRs(connection.private_route?.cidrs.join(', ') ?? '')
  }, [result.data])

  useEffect(() => {
    if (!result.isSuccess) setSecret('')
  }, [result.isSuccess])

  const run = async (name: string, action: () => Promise<unknown>) => {
    setBusy(name)
    setError('')
    try {
      await action()
      setSecret('')
      onRefresh()
      await result.refetch()
    } catch (reason) {
      setError(message(reason))
    } finally {
      setBusy('')
    }
  }

  if (result.isLoading) {
    return <StatePanel kind="loading" title="Loading connection" detail="Loading connection…" />
  }
  if (result.error || !result.data) {
    return <StatePanel kind="error" title="Connection unavailable" detail={message(result.error)} action={<Button onClick={() => void result.refetch()}>Retry</Button>} />
  }

  const connection = result.data
  const capabilities = Object.entries(connection.capabilities)
  const connectionStatus = connectionStateStatus(connection.provider, connection.settings.profile, connection.state)
  const status = <p>
    <span title={connectionStatus.title}><StatusBadge label={connectionStatus.label} tone={connectionStatus.tone} /></span>
    <span className="table-meta">{connection.kind}/{connectionProviderLabel(connection.provider, connection.settings.profile)} · version {connection.version}</span>
  </p>
  const actions = <>
    {connection.kind !== 'agent' && <Button
      disabled={!!busy || !csrf}
      onClick={() => void run('test', () => api.testConnection(orgID, connection.id, connection.version, csrf))}
    >{busy === 'test' ? 'Testing…' : 'Test capability'}</Button>}
    <Button disabled={!!busy || !csrf || connection.state === 'revoked'} onClick={() => setConfirmRevoke(true)}>Revoke</Button>
  </>

  return <><DetailPanel title={connection.name} status={status} actions={actions}>
    <ConnectionDetailTabs kind={connection.kind} value={detailTab} onChange={setDetailTab} />
    {detailTab === 'overview' && <ConnectionOverview connection={connection} capabilities={capabilities} />}
    {detailTab === 'qualification' && connection.kind === 'agent' && <AgentQualificationPanel orgID={orgID} connectionID={connection.id} provider={connection.provider} />}
    {detailTab === 'configuration' && connection.kind !== 'agent' && <PrivateRouteEditor
      csrf={csrf}
      busy={busy}
      pools={pools}
      runners={runners}
      poolID={poolID}
      runnerID={runnerID}
      routeHost={routeHost}
      cidrs={cidrs}
      poolItems={poolItems}
      runnerOptions={visibleRunnerOptions}
      onPoolChange={value => { setPoolID(value); setRunnerID('') }}
      onRunnerChange={setRunnerID}
      onHostChange={setRouteHost}
      onCIDRsChange={setCIDRs}
      onApprove={payload => void run('route', () => api.setPrivateRoute(orgID, connection.id, connection.version, payload, csrf))}
    />}
    {detailTab === 'actions' && <ConnectionActions
      connection={connection}
      orgID={orgID}
      csrf={csrf}
      busy={busy}
      secret={secret}
      onSecretChange={setSecret}
      onRotate={() => void run('rotate', () => api.rotateConnection(orgID, connection.id, connection.version, secret, csrf))}
    />}
    {error && <p className="error-text" role="alert">{error}</p>}
  </DetailPanel>
  <Dialog open={confirmRevoke} title="Revoke connection" onClose={() => setConfirmRevoke(false)}><p>Revoke {connection.name}? It will stop being available to this organisation.</p><div className="connection-confirm-actions"><Button type="button" disabled={!!busy} onClick={() => setConfirmRevoke(false)}>Cancel</Button><Button type="button" className="button-danger" disabled={!!busy || !csrf} onClick={() => void run('revoke', async () => { await api.revokeConnection(orgID, connection.id, connection.version, csrf); setConfirmRevoke(false) })}>{busy === 'revoke' ? 'Revoking…' : 'Revoke connection'}</Button></div></Dialog>
  </>
}

type ConnectionDetailTab = 'overview' | 'configuration' | 'qualification' | 'actions'

function ConnectionDetailTabs({ kind, value, onChange }: { kind: string; value: ConnectionDetailTab; onChange: (tab: ConnectionDetailTab) => void }) {
  return <nav className="detail-tabs" aria-label="Connection detail">
    <Button aria-pressed={value === 'overview'} onClick={() => onChange('overview')}>Overview</Button>
    {kind !== 'agent' && <Button aria-pressed={value === 'configuration'} onClick={() => onChange('configuration')}>Configuration</Button>}
    {kind === 'agent' && <Button aria-pressed={value === 'qualification'} onClick={() => onChange('qualification')}>Qualification</Button>}
    <Button aria-pressed={value === 'actions'} onClick={() => onChange('actions')}>Actions</Button>
  </nav>
}

function ConnectionOverview({ connection, capabilities }: { connection: Connection; capabilities: Array<[string, Connection['capabilities'][string]]> }) {
  return <div className="connection-overview">
    <dl className="detail-list">
      <div><dt>Endpoint</dt><dd>{connection.endpoint}</dd></div>
      <div><dt>Billing route</dt><dd>{connection.settings.billing_route || 'Unknown'}</dd></div>
      <div><dt>Model / account</dt><dd>{connection.settings.model || connection.settings.namespace || 'Not set'}</dd></div>
      <div><dt>Runtime / protocol</dt><dd>{connection.settings.runtime_version || (connection.settings.profile ? modelProfileLabel(connection.provider, connection.settings.profile) : '') || 'Not set'}</dd></div>
      <div><dt>Last verified</dt><dd>{connection.verified_at ? new Date(connection.verified_at).toLocaleString() : 'Not verified'}</dd></div>
      {connection.reason && <div><dt>Reason</dt><dd>{connection.reason}</dd></div>}
    </dl>
    {!!capabilities.length && <section className="connection-capabilities" aria-label="Capabilities">
      <h3>Capabilities</h3>
      <ul className="compact-list">{capabilities.map(([name, capability]) => {
        const tone = capability.state === 'supported' ? 'green' : capability.state === 'unsupported' ? 'red' : 'amber'
        return <li key={name}><StatusBadge label={capability.state} tone={tone} /> <strong>{name}</strong>: {capability.reason}</li>
      })}</ul>
    </section>}
  </div>
}

type RunnerChoice = { id: string; name: string; pool_name: string }
type PrivateRouteEditorProps = {
  csrf: string
  busy: string
  pools: ReturnType<typeof useInfiniteQuery>
  runners: ReturnType<typeof useInfiniteQuery>
  poolID: string
  runnerID: string
  routeHost: string
  cidrs: string
  poolItems: Array<{ id: string; name: string }>
  runnerOptions: RunnerChoice[]
  onPoolChange: (value: string) => void
  onRunnerChange: (value: string) => void
  onHostChange: (value: string) => void
  onCIDRsChange: (value: string) => void
  onApprove: (payload: { runner_id: string; host: string; cidrs: string[] }) => void
}

function PrivateRouteEditor(props: PrivateRouteEditorProps) {
  const incomplete = !props.runnerID || !props.routeHost || !props.cidrs.trim()

  return <fieldset className="connection-private-route">
    <legend>Private route</legend>
    <div className="form-grid">
      <label>Runner pool<select aria-label="Runner pool" value={props.poolID} onChange={event => props.onPoolChange(event.target.value)}>
        <option value="">Choose pool</option>
        {props.poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
      </select></label>
      <label>Runner<select aria-label="Runner" value={props.runnerID} onChange={event => props.onRunnerChange(event.target.value)}>
        <option value="">Choose active runner</option>
        {props.runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}
      </select></label>
      <label>Host<input value={props.routeHost} onChange={event => props.onHostChange(event.target.value)} /></label>
      <label className="wide">CIDRs<input value={props.cidrs} onChange={event => props.onCIDRsChange(event.target.value)} placeholder="10.0.0.0/8, 192.168.0.0/16" /></label>
    </div>
    {!!props.pools.error && <p className="error-text" role="alert">Runner pools unavailable: {message(props.pools.error)}</p>}
    {!!props.runners.error && <p className="error-text" role="alert">Runners unavailable: {message(props.runners.error)}</p>}
    {props.pools.hasNextPage && <Button
      onClick={() => void props.pools.fetchNextPage()}
      disabled={props.pools.isFetchingNextPage}
    >{props.pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}
    {props.runners.hasNextPage && <Button
      onClick={() => void props.runners.fetchNextPage()}
      disabled={props.runners.isFetchingNextPage}
    >{props.runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}
    {!props.runnerOptions.length && props.poolID && !props.runners.isLoading && <p className="table-meta">No active enrolled runners in selected pool.</p>}
    <Button
      disabled={incomplete || !!props.busy || !props.csrf}
      title={!props.csrf ? 'Sign in again to approve a private route.' : !props.runnerID ? 'Choose an active enrolled runner first.' : undefined}
      onClick={() => props.onApprove({
        runner_id: props.runnerID,
        host: props.routeHost,
        cidrs: props.cidrs.split(',').map(item => item.trim()).filter(Boolean),
      })}
    >Approve private route</Button>
  </fieldset>
}

type ConnectionActionsProps = {
  connection: Connection
  orgID: string
  csrf: string
  busy: string
  secret: string
  onSecretChange: (value: string) => void
  onRotate: () => void
}

function ConnectionActions({ connection, orgID, csrf, busy, secret, onSecretChange, onRotate }: ConnectionActionsProps) {
  return <section className="connection-actions">
    {connection.kind !== 'agent' && <>
      <label>Rotate secret<input type="password" autoComplete="new-password" value={secret} onChange={event => onSecretChange(event.target.value)} /></label>
      <Button disabled={!secret || !!busy || !csrf} onClick={onRotate}>{busy === 'rotate' ? 'Rotating…' : 'Rotate credential'}</Button>
    </>}
    {connection.kind === 'forge' && <WebhookPanel orgID={orgID} connection={connection} csrf={csrf} />}
  </section>
}

function WebhookPanel({ orgID, connection, csrf }: { orgID: string; connection: Connection; csrf: string }) {
  const client = useQueryClient()
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const state = useQuery({
    queryKey: ['org', orgID, 'connections', connection.id, 'webhook'],
    queryFn: ({ signal }) => inventoryAPI.webhook(orgID, connection.id, signal),
  })
  const unconfigured = state.error instanceof ReforgeAPIError && state.error.code === 'webhook_unconfigured'
  const version = state.data?.version ?? (unconfigured ? 0 : undefined)

  const issue = async () => {
    if (version === undefined || !csrf) return
    setSecret('')
    setBusy(true)
    setError('')
    try {
      const issued = await inventoryAPI.issueWebhook(orgID, connection.id, version, csrf)
      setSecret(issued.secret)
      await client.invalidateQueries({ queryKey: ['org', orgID, 'connections', connection.id, 'webhook'] })
    } catch (reason) {
      setError(message(reason))
    } finally {
      setBusy(false)
    }
  }

  const revoke = async () => {
    if (!state.data || !csrf) return
    setBusy(true)
    setError('')
    try {
      await inventoryAPI.revokeWebhook(orgID, connection.id, state.data.version, csrf)
      setSecret('')
      await client.invalidateQueries({ queryKey: ['org', orgID, 'connections', connection.id, 'webhook'] })
    } catch (reason) {
      setError(message(reason))
    } finally {
      setBusy(false)
    }
  }

  if (state.isLoading) {
    return <fieldset><legend>Inventory webhook</legend><p className="table-meta">Loading webhook state…</p></fieldset>
  }
  if (state.error && !unconfigured) {
    return <fieldset><legend>Inventory webhook</legend><p className="error-text" role="alert">Webhook state unavailable: {message(state.error)}</p></fieldset>
  }

  const label = state.data?.revoked
    ? 'Webhook revoked.'
    : state.data
      ? `POST ${window.location.origin}${state.data.path}`
      : 'No webhook configured.'

  return <fieldset>
    <legend>Inventory webhook</legend>
    <p className="table-meta">{label}</p>
    {secret && <label>One-time webhook secret<input readOnly type="password" value={secret} /></label>}
    {error && <p className="error-text" role="alert">{error}</p>}
    <div className="row-actions">
      <Button disabled={busy || !csrf || version === undefined} onClick={() => void issue()}>
        {state.data && !state.data.revoked ? 'Rotate webhook secret' : 'Issue webhook secret'}
      </Button>
      <Button disabled={busy || !csrf || !state.data || state.data.revoked} onClick={() => void revoke()}>Revoke webhook</Button>
      {secret && <Button onClick={() => setSecret('')}>Clear webhook secret</Button>}
    </div>
  </fieldset>
}

type ConnectionFormProps = {
  open: boolean
  orgID: string
  csrf: string
  onClose: () => void
  onCreated: () => void
}

function ConnectionForm({ open, orgID, csrf, onClose, onCreated }: ConnectionFormProps) {
  const [kind, setKind] = useState('forge')
  const [modelBusy, setModelBusy] = useState(false)
  const [provider, setProvider] = useState('github')
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [namespace, setNamespace] = useState('')
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
  const pools = useInfiniteQuery({
    queryKey: ['org', orgID, 'connection-form-runner-pools'],
    queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: open,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const runners = useInfiniteQuery({
    queryKey: ['org', orgID, 'connection-form-runners', poolID],
    queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: open && !!poolID,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? [])
    .filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now())
    .map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))

  useEffect(() => {
    setSecret('')
    setAppID('')
    setInstallationID('')
    setNamespace('')
    setPrivateRoute(false)
    setRunnerID('')
    setRouteHost('')
    setCIDRs('')
    if (kind === 'forge' || kind === 'delivery') {
      setProvider('github')
      setAuthKind('token')
      setBillingRoute('forge')
      setModel('')
    }
    if (kind === 'agent') {
      setProvider('codex')
      setAuthKind('official_runtime')
      setBillingRoute('subscription')
      setModel('')
    }
  }, [kind])

  useEffect(() => {
    if (!open) {
      setSecret('')
      setError('')
    }
  }, [open])

  const handleProviderChange = (value: string) => {
    setProvider(value)
    setSecret('')
    setAppID('')
    setInstallationID('')
    if (kind === 'forge' || kind === 'delivery') setAuthKind('token')
  }

  const handleAuthChange = (value: string) => {
    setAuthKind(value)
    setSecret('')
    setAppID('')
    setInstallationID('')
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    const settings = {
      auth_kind: authKind,
      billing_route: billingRoute,
      ...(namespace ? { namespace } : {}),
      ...(kind === 'agent' && model ? { model } : {}),
      ...(appID ? { app_id: appID } : {}),
      ...(installationID ? { installation_id: installationID } : {}),
      ...(caPEM ? { ca_pem: caPEM } : {}),
    }
    const payload: ConnectionCreate = {
      kind,
      provider,
      name,
      endpoint,
      settings,
      ...(secret ? { secret } : {}),
      ...(privateRoute ? { private_route: { runner_id: runnerID, host: routeHost, cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) } } : {}),
    }
    try {
      await api.createConnection(orgID, kind, payload, csrf)
      setSecret('')
      onCreated()
    } catch (reason) {
      setError(message(reason))
    } finally {
      setBusy(false)
    }
  }

  const forgeKind = kind === 'forge' || kind === 'delivery'
  const agentKind = kind === 'agent'
  const secretRequired = !agentKind
  const githubForge = forgeKind && provider === 'github'
  const routeIncomplete = privateRoute && (!runnerID || !routeHost || !cidrs.trim())
  const providerOptions = agentKind ? ['codex', 'claude_code', 'agy', 'gemini_cli', 'custom_command'] : ['github', 'gitlab', 'gitea']

  return <Dialog open={open} title="Add connection" onClose={() => { if (!modelBusy) onClose() }}>
    <div className="form-stack">
      <label>Kind<select value={kind} disabled={modelBusy} onChange={event => setKind(event.target.value)}>
        <option value="forge">Forge</option>
        <option value="model">Model API</option>
        <option value="agent">Agent runtime</option>
        <option value="delivery">Delivery</option>
      </select></label>
      {kind === 'model'
        ? open && <ModelConnectionForm orgID={orgID} csrf={csrf} onClose={onClose} onCreated={onCreated} onBusyChange={setModelBusy} />
        : <form className="form-stack" onSubmit={submit}>
          <div className="form-grid">
            <label>Provider<select value={provider} onChange={event => handleProviderChange(event.target.value)}>
              {providerOptions.map(option => <option key={option} value={option}>{option}</option>)}
            </select></label>
            <label className="wide">Name<input required value={name} onChange={event => setName(event.target.value)} /></label>
            <label className="wide">Endpoint<input required type="url" value={endpoint} onChange={event => setEndpoint(event.target.value)} placeholder="https://…" /></label>
            {forgeKind && <label>Namespace<input value={namespace} onChange={event => setNamespace(event.target.value)} placeholder="organisation or group" /></label>}
            {githubForge && <label>Auth<select value={authKind} onChange={event => handleAuthChange(event.target.value)}>
              <option value="token">Token</option>
              <option value="github_app">GitHub App</option>
            </select></label>}
            {githubForge && authKind === 'github_app' && <>
              <label>App ID<input required value={appID} onChange={event => setAppID(event.target.value)} /></label>
              <label>Installation ID<input required value={installationID} onChange={event => setInstallationID(event.target.value)} /></label>
            </>}
            {forgeKind && !githubForge && <input type="hidden" value="token" readOnly />}
            {agentKind && <label>Model ID<input required={provider === 'custom_command'} value={model} onChange={event => setModel(event.target.value)} /></label>}
            {agentKind && <label>Billing route<select value={billingRoute} onChange={event => setBillingRoute(event.target.value)}><option value="subscription">Subscription</option></select></label>}
            <label className="wide">CA certificate<input value={caPEM} onChange={event => setCAPEM(event.target.value)} placeholder="Optional PEM CA certificate" /></label>
            {!agentKind && <label className="wide">Secret<input type="password" autoComplete="new-password" required={secretRequired} value={secret} onChange={event => setSecret(event.target.value)} /></label>}
            <label className="checkbox-label"><input type="checkbox" checked={privateRoute} onChange={event => setPrivateRoute(event.target.checked)} /> Private route via enrolled runner</label>
            {privateRoute && <>
              <label>Runner pool<select aria-label="Runner pool" value={poolID} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}>
                <option value="">Choose active pool</option>
                {poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
              </select></label>
              <label>Runner<select value={runnerID} onChange={event => setRunnerID(event.target.value)}>
                <option value="">Choose active runner</option>
                {runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}
              </select></label>
              <label>Route host<input value={routeHost} onChange={event => setRouteHost(event.target.value)} /></label>
              <label className="wide">Approved CIDRs<input value={cidrs} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>
              {!!pools.error && <p className="error-text" role="alert">Runner pools unavailable: {message(pools.error)}</p>}
              {!!runners.error && <p className="error-text" role="alert">Runners unavailable: {message(runners.error)}</p>}
              {pools.hasNextPage && <Button type="button" disabled={pools.isFetchingNextPage} onClick={() => void pools.fetchNextPage()}>{pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}
              {runners.hasNextPage && <Button type="button" disabled={runners.isFetchingNextPage} onClick={() => void runners.fetchNextPage()}>{runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}
              {!runnerOptions.length && poolID && !runners.isLoading && <p className="table-meta">No active enrolled runners available in selected pool.</p>}
            </>}
          </div>
          {routeIncomplete && <p className="table-meta">Choose an active runner, route host and approved CIDR before saving.</p>}
          {error && <p className="error-text" role="alert">{error}</p>}
          <div className="dialog-actions">
            <Button type="button" onClick={onClose}>Cancel</Button>
            <Button
              className="button button-primary"
              disabled={busy || !csrf || routeIncomplete}
              title={!csrf ? 'Sign in again to create a connection.' : routeIncomplete ? 'Complete private route fields before saving.' : undefined}
            >{busy ? 'Saving…' : 'Create connection'}</Button>
          </div>
        </form>}
    </div>
  </Dialog>
}

function message(value: unknown) { return value instanceof Error ? value.message : 'The server returned an unknown error.' }
