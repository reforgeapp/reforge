import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api, ReforgeAPIError, type Connection } from '../api/client'
import { inventoryAPI } from '../api/inventory'
import { connectionQuery, connectionsListQuery, useSession } from './query'
import { Button, Dialog } from '../components/Accessible'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { SplitView, DetailPanel } from '../components/Workspace'
import { ModelPricing } from './ModelPricing'
import { CustomProfilesPanel } from './CustomProfilesPanel'
import { AgentQualificationPanel } from './AgentQualificationPanel'
import { ModelConnectionForm } from './ModelConnectionForm'
import { ForgeConnectionForm } from './ForgeConnectionForm'
import { RepositoryImportDialog, type ImportConnection } from './RepositoryImportDialog'
import { connectionProviderLabel, connectionStateStatus, modelProfileLabel } from '../model-providers'
import { Icon } from '../components/Icons'
import '../styles/connections.css'

const tabs = [
  { id: 'forge', label: 'Forges', kinds: ['forge'] },
  { id: 'models', label: 'Models & agents', kinds: ['model', 'agent'] },
  { id: 'delivery', label: 'Delivery', kinds: ['delivery'] },
] as const
const tabValue = (value: string | undefined): 'forge' | 'models' | 'delivery' => value === 'models' || value === 'delivery' ? value : 'forge'

const githubResultMessages: Record<string, string> = {
  connected: 'GitHub App connected.',
  failed: 'GitHub setup failed. Try again or use a personal access token.',
  expired: 'GitHub setup expired. Start again.',
  pending_approval: 'GitHub installation is pending approval. A GitHub organisation owner must approve the App.',
}

const githubReasonMessages: Record<string, string> = {
  github_org_admin_required: 'A GitHub organisation owner must approve the installation.',
  github_owner_mismatch: 'The signed-in GitHub user does not own or administer the target account.',
  github_app_suspended: 'The GitHub App installation is suspended.',
  github_installation_missing: 'No matching GitHub App installation was found. Install or reconfigure the App, then try again.',
}

export function ConnectionsPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const [formOpen, setFormOpen] = useState(false)
  const [importState, setImportState] = useState<{ connection: ImportConnection; filter: string }>()
  const [githubResult, setGithubResult] = useState<{ result: string; reason?: string }>()
  const handledResult = useRef('')
  const selectedID = search.connection ?? ''
  const syncJob = search.sync ?? ''
  const csrf = session.data?.csrf_token ?? ''
  const setSyncJob = (jobID: string | undefined) => { void navigate({ search: previous => { const next = { ...previous }; if (jobID) next.sync = jobID; else delete next.sync; return next }, replace: true }) }

  const tab = tabValue(search.connection_tab)
  const q = search.q ?? ''
  const state = search.state ?? ''
  const forgeResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'forge'), enabled: tab === 'forge' })
  const modelResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'model'), enabled: tab === 'models' })
  const agentResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'agent'), enabled: tab === 'models' })
  const deliveryResult = useInfiniteQuery({ ...connectionsListQuery(orgID, 'delivery'), enabled: tab === 'delivery' })
  const teams = useQuery({ queryKey: ['org', orgID, 'teams'], queryFn: ({ signal }) => inventoryAPI.teams(orgID, signal), enabled: formOpen || !!importState })
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

  const openImport = (connection: ImportConnection, filter = '') => {
    setFormOpen(false)
    setSyncJob(undefined)
    setImportState({ connection, filter })
  }

  useEffect(() => {
    const result = search.github_result
    if (!result) return
    const connectionID = search.connection
    const key = `${result}:${connectionID ?? ''}`
    if (result !== 'connected' || !connectionID || !csrf) {
      if (handledResult.current === key) return
      handledResult.current = key
      setGithubResult({ result, reason: search.github_reason })
      void navigate({ search: previous => { const next = { ...previous }; delete next.github_result; delete next.github_reason; return next }, replace: true })
      return
    }
    if (handledResult.current === key) return
    handledResult.current = key
    setGithubResult({ result, reason: search.github_reason })
    void navigate({ search: previous => { const next = { ...previous }; delete next.github_result; delete next.github_reason; return next }, replace: true })
    void (async () => {
      try {
        const connection = await api.getConnection(orgID, connectionID)
        let healthy = connection.state === 'healthy'
        try {
          const tested = await api.testConnection(orgID, connection.id, connection.version, csrf)
          healthy = tested.state === 'healthy'
        } catch {
          healthy = false
        }
        refresh()
        if (healthy) openImport({ id: connection.id, name: connection.name, provider: connection.provider })
      } catch {
        refresh()
      }
    })()
  }, [search.github_result, search.connection, search.github_reason, csrf, orgID])

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
    {githubResult && <div className="connections-notice" role="status">
      <span>{githubResultMessages[githubResult.result] ?? 'GitHub setup finished.'}{githubResult.reason ? ` ${githubReasonMessages[githubResult.reason] ?? ''}` : ''}</span>
      <Button className="button button-sm" onClick={() => setGithubResult(undefined)}>Dismiss</Button>
    </div>}
    <SplitView
      listLabel="Connections"
      selected={!!selectedID}
      onBack={clear}
      hideBack
      closeControl={{ label: 'Close connection details', onClose: clear }}
      list={list}
      detail={selectedID ? <ConnectionDetail key={selectedID} connectionID={selectedID} orgID={orgID} csrf={csrf} onRefresh={refresh} onImport={openImport} onDeleted={() => { clear(); refresh() }} /> : null}
    />
    {tab === 'models' && <details>
      <summary>Custom command profiles</summary>
      <CustomProfilesPanel orgID={orgID} />
    </details>}
    <ConnectionForm
      open={formOpen}
      tab={tab}
      orgID={orgID}
      csrf={csrf}
      onClose={() => setFormOpen(false)}
      onCreated={() => { setFormOpen(false); refresh() }}
      onReady={openImport}
    />
    <RepositoryImportDialog
      open={!!importState}
      orgID={orgID}
      csrf={csrf}
      teams={teams.data?.items ?? []}
      connection={importState?.connection}
      candidateFilter={importState?.filter}
      repositoriesLink={importState ? `/org/${orgID}/repositories` : undefined}
      jobID={syncJob}
      onJobChange={setSyncJob}
      autoStart
      onClose={() => { setImportState(undefined); setSyncJob(undefined) }}
      onDone={() => { refresh(); void client.invalidateQueries({ queryKey: ['org', orgID, 'inventory-repositories'] }) }}
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
  const connectionSort = useSort(props.rows, { name: connection => connection.name, provider: connection => connectionProviderLabel(connection.provider, connection.settings.profile), state: connection => connection.state, credential: connection => connection.credential_version }, { key: 'name', dir: 'asc' })
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
        <thead><tr>{connectionSort.header('name', 'Name')}{connectionSort.header('provider', 'Provider')}{connectionSort.header('state', 'State')}{connectionSort.header('credential', 'Credential')}<th><span className="sr-only">Actions</span></th></tr></thead>
        <tbody>{connectionSort.rows.map(connection => <ConnectionRow key={connection.id} connection={connection} onSelect={props.onSelect} />)}</tbody>
      </table>
      {!props.rows.length && <EmptyTable label={props.hasMore && (props.query.trim() || props.state) ? 'No matches in loaded results; more may be available.' : props.query.trim() || props.state ? 'No connections match these filters.' : `No ${category.toLowerCase()} found.`} />}
      {props.hasMore && <div className="table-note"><Button disabled={props.loadingMore} onClick={props.onLoadMore}>{props.loadingMore ? 'Loading…' : 'Load more'}</Button></div>}
    </DataTable>
  </div>
}

function ConnectionRow({ connection, onSelect }: { connection: Connection; onSelect: (id: string) => void }) {
  const status = connectionStateStatus(connection.provider, connection.settings.profile, connection.state, connection.key_confirmed_at, connection.reason)

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
  onImport: (connection: ImportConnection) => void
  onDeleted: () => void
}

function ConnectionDetail({ connectionID, orgID, csrf, onRefresh, onImport, onDeleted }: ConnectionDetailProps) {
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
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [edit, setEdit] = useState<{ name: string; namespace: string }>()
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
  const connectionStatus = connectionStateStatus(connection.provider, connection.settings.profile, connection.state, connection.key_confirmed_at, connection.reason)
  const forgeLike = connection.kind === 'forge'
  const namespaceEditable = forgeLike && connection.settings.auth_kind !== 'github_app' && connection.settings.auth_kind !== 'github_app_platform'
  const revoked = connection.state === 'revoked'
  const status = <p>
    <span title={connectionStatus.title}><StatusBadge label={connectionStatus.label} tone={connectionStatus.tone} /></span>
    <span className="table-meta">{connection.kind}/{connectionProviderLabel(connection.provider, connection.settings.profile)} · version {connection.version}</span>
  </p>
  const actions = <>
    {forgeLike && <Button
      disabled={connection.state === 'revoked' || connection.state !== 'healthy'}
      title={connection.state !== 'healthy' ? 'Run a capability test before importing repositories.' : undefined}
      onClick={() => onImport({ id: connection.id, name: connection.name, provider: connection.provider })}
    >Add repositories</Button>}
    {connection.kind !== 'agent' && <Button
      disabled={!!busy || !csrf || connection.state === 'revoked'}
      title={connection.state === 'revoked' ? 'Revoked connections cannot be tested.' : undefined}
      onClick={() => void run('test', () => api.testConnection(orgID, connection.id, connection.version, csrf))}
    >{busy === 'test' ? 'Testing…' : 'Test connection'}</Button>}
    <Button disabled={!!busy || !csrf || revoked || !!edit} onClick={() => setEdit({ name: connection.name, namespace: connection.settings.namespace ?? '' })}>Edit</Button>
    <span className="detail-actions-end">
      <Button className="button button-danger-ghost" disabled={!!busy || !csrf || revoked} onClick={() => setConfirmRevoke(true)}>Revoke</Button>
      <Button className="button button-danger-ghost" disabled={!!busy || !csrf} onClick={() => setConfirmDelete(true)}>Delete</Button>
    </span>
  </>

  return <><DetailPanel title={connection.name} status={status} actions={actions}>
    {edit && <form className="connection-edit" aria-label="Edit connection" onSubmit={event => { event.preventDefault(); void run('edit', async () => { await api.updateConnection(orgID, connection.id, connection.version, { name: edit.name.trim(), namespace: namespaceEditable ? edit.namespace.trim() : connection.settings.namespace ?? '' }, csrf); setEdit(undefined) }) }}>
      <label>Name<input value={edit.name} required maxLength={160} onChange={event => setEdit({ ...edit, name: event.target.value })} /></label>
      {namespaceEditable && <label>Namespace<input value={edit.namespace} placeholder={connection.provider === 'github' ? 'org, user:name or owner/repo' : 'Organisation, group or owner'} onChange={event => setEdit({ ...edit, namespace: event.target.value })} /></label>}
      <div className="row-actions"><Button type="submit" className="button button-primary" disabled={!!busy || !edit.name.trim()}>{busy === 'edit' ? 'Saving…' : 'Save'}</Button><Button type="button" disabled={!!busy} onClick={() => setEdit(undefined)}>Cancel</Button></div>
    </form>}
    <ConnectionDetailTabs kind={connection.kind} value={detailTab} onChange={setDetailTab} />
    {detailTab === 'overview' && <ConnectionOverview connection={connection} capabilities={capabilities} />}
    {detailTab === 'overview' && connection.kind === 'model' && connection.settings.billing_route === 'direct_api' && connection.settings.model && !revoked && <ModelPricing orgID={orgID} connectionID={connection.id} model={connection.settings.model} csrf={csrf} />}
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
  <Dialog open={confirmDelete} title="Delete connection" onClose={() => setConfirmDelete(false)}><p>Delete {connection.name}? This cannot be undone.</p>{error && confirmDelete && <p className="error-text" role="alert">{error}</p>}<div className="connection-confirm-actions"><Button type="button" disabled={!!busy} onClick={() => setConfirmDelete(false)}>Cancel</Button><Button type="button" className="button-danger" disabled={!!busy || !csrf} onClick={() => { setBusy('delete'); setError(''); void api.deleteConnection(orgID, connection.id, connection.version, csrf).then(() => { setConfirmDelete(false); onDeleted() }, reason => setError(message(reason))).finally(() => setBusy('')) }}>{busy === 'delete' ? 'Deleting…' : 'Delete connection'}</Button></div></Dialog>
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

function authLabel(connection: Connection) {
  const authKind = connection.settings.auth_kind
  if (authKind === 'token') return 'Personal access token'
  if (authKind === 'github_app' || authKind === 'github_app_platform') return connection.settings.managed ? 'GitHub App' : 'GitHub App (manual)'
  if (authKind === 'api_key') return 'API key'
  if (authKind === 'official_runtime') return 'Official runtime'
  return authKind || 'Unknown'
}

function capabilityTone(state: string) {
  return state === 'supported' ? 'green' as const : state === 'unsupported' ? 'red' as const : 'amber' as const
}

function humanize(value: string) {
  return value.replace(/[._:-]+/g, ' ').trim().replace(/\b\w/g, character => character.toUpperCase())
}

function ConnectionOverview({ connection, capabilities }: { connection: Connection; capabilities: Array<[string, Connection['capabilities'][string]]> }) {
  const forgeLike = connection.kind === 'forge' || connection.kind === 'delivery'
  const healthy = connection.state === 'healthy'
  const webhookPending = !!connection.settings.webhook_pending
  return <div className="connection-overview">
    <dl className="detail-list">
      <div><dt>Provider</dt><dd>{connectionProviderLabel(connection.provider, connection.settings.profile)}</dd></div>
      <div><dt>{forgeLike ? 'API address' : 'Endpoint'}</dt><dd>{connection.endpoint}</dd></div>
      <div><dt>Authentication</dt><dd>{authLabel(connection)}</dd></div>
      <div><dt>Last checked</dt><dd>{connection.verified_at ? new Date(connection.verified_at).toLocaleString() : 'Not verified'}</dd></div>
      {!forgeLike && <div><dt>Billing route</dt><dd>{connection.settings.billing_route || 'Unknown'}</dd></div>}
      {!forgeLike && <div><dt>Model / account</dt><dd>{connection.settings.model || connection.settings.namespace || 'Not set'}</dd></div>}
      {!forgeLike && <div><dt>Runtime / protocol</dt><dd>{connection.settings.runtime_version || (connection.settings.profile ? modelProfileLabel(connection.provider, connection.settings.profile) : '') || 'Not set'}</dd></div>}
      {!healthy && connection.reason && <div><dt>Reason</dt><dd>{connection.reason}</dd></div>}
    </dl>
    {webhookPending && <p className="connection-reason-help" role="status">Webhook setup pending. Recreate this App with a public HTTPS URL.</p>}
    {!!capabilities.length && <details className="connection-capabilities">
      <summary>{`Capabilities (${capabilities.length})`}</summary>
      <ul className="compact-list connection-capability-list">{capabilities.map(([name, capability]) => <li key={name}>
        <span className="connection-capability-row"><StatusBadge label={humanize(capability.state)} tone={capabilityTone(capability.state)} /> <strong>{humanize(name)}</strong></span>
        {capability.reason && <span className="help-tip" tabIndex={0} title={capability.reason} aria-label={`${humanize(name)}: ${capability.reason}`}>?</span>}
      </li>)}</ul>
    </details>}
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
  const managed = !!connection.settings.managed
  return <section className="connection-actions">
    {connection.kind !== 'agent' && !managed && <>
      <label>Rotate secret<input type="password" autoComplete="new-password" value={secret} onChange={event => onSecretChange(event.target.value)} /></label>
      <Button disabled={!secret || !!busy || !csrf || connection.state === 'revoked'} onClick={onRotate}>{busy === 'rotate' ? 'Rotating…' : 'Rotate credential'}</Button>
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

  if (connection.settings.managed) {
    return <fieldset>
      <legend>Inventory webhook</legend>
      <a className="button button-sm" href="https://github.com/settings/apps" target="_blank" rel="noreferrer" title="This webhook is managed by the GitHub App installation.">Configure on GitHub</a>
    </fieldset>
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
  tab: ConnectionTab
  orgID: string
  csrf: string
  onClose: () => void
  onCreated: () => void
  onReady: (connection: ImportConnection, repositoryFilter: string) => void
}

function ConnectionForm({ open, tab, orgID, csrf, onClose, onCreated, onReady }: ConnectionFormProps) {
  const [modelKind, setModelKind] = useState<'model' | 'agent'>('model')
  const [modelBusy, setModelBusy] = useState(false)
  const [forgeBusy, setForgeBusy] = useState(false)
  const busy = modelBusy || forgeBusy

  useEffect(() => {
    if (!open) { setModelKind('model'); setModelBusy(false); setForgeBusy(false) }
  }, [open])

  return <Dialog open={open} title="Add connection" onClose={() => { if (!busy) onClose() }}>
    <div className="form-stack">
      {tab === 'models' && <label>Connection type<select value={modelKind} disabled={busy} onChange={event => setModelKind(event.target.value as 'model' | 'agent')}>
        <option value="model">Model API</option>
        <option value="agent">Agent runtime</option>
      </select></label>}
      {tab === 'models'
        ? modelKind === 'model'
          ? open && <ModelConnectionForm orgID={orgID} csrf={csrf} onClose={onClose} onCreated={onCreated} onBusyChange={setModelBusy} />
          : open && <AgentConnectionForm orgID={orgID} csrf={csrf} onClose={onClose} onCreated={onCreated} onBusyChange={setModelBusy} />
        : open && <ForgeConnectionForm orgID={orgID} csrf={csrf} kind={tab === 'delivery' ? 'delivery' : 'forge'} onClose={onClose} onReady={onReady} onBusyChange={setForgeBusy} />}
    </div>
  </Dialog>
}

type AgentConnectionFormProps = {
  orgID: string
  csrf: string
  onClose: () => void
  onCreated: () => void
  onBusyChange?: (busy: boolean) => void
}

function AgentConnectionForm({ orgID, csrf, onClose, onCreated, onBusyChange }: AgentConnectionFormProps) {
  const [provider, setProvider] = useState('codex')
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [model, setModel] = useState('')
  const [caPEM, setCAPEM] = useState('')
  const [privateRoute, setPrivateRoute] = useState(false)
  const [poolID, setPoolID] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const pools = useInfiniteQuery({
    queryKey: ['org', orgID, 'agent-form-runner-pools'],
    queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const runners = useInfiniteQuery({
    queryKey: ['org', orgID, 'agent-form-runners', poolID],
    queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute && !!poolID,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? [])
    .filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now())
    .map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))

  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  useEffect(() => () => { onBusyChange?.(false) }, [onBusyChange])

  const routeIncomplete = privateRoute && (!runnerID || !routeHost.trim() || !cidrs.trim())
  const canSave = !!csrf && !!name.trim() && !!endpoint.trim() && (provider !== 'custom_command' || !!model.trim()) && !routeIncomplete && !busy

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.createConnection(orgID, 'agent', {
        kind: 'agent',
        provider,
        name: name.trim(),
        endpoint: endpoint.trim(),
        settings: { auth_kind: 'official_runtime', billing_route: 'subscription', ...(model.trim() ? { model: model.trim() } : {}), ...(caPEM.trim() ? { ca_pem: caPEM } : {}) },
        ...(privateRoute ? { private_route: { runner_id: runnerID, host: routeHost.trim(), cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) } } : {}),
      }, csrf)
      onCreated()
    } catch (reason) {
      setError(message(reason))
    } finally {
      setBusy(false)
    }
  }

  return <form className="form-stack" onSubmit={submit}>
    <div className="form-grid">
      <label>Provider<select value={provider} disabled={busy} onChange={event => setProvider(event.target.value)}>
        {['codex', 'claude_code', 'agy', 'gemini_cli', 'custom_command'].map(option => <option key={option} value={option}>{option}</option>)}
      </select></label>
      <label className="wide">Name<input required value={name} disabled={busy} onChange={event => setName(event.target.value)} /></label>
      <label className="wide">Endpoint<input required type="url" value={endpoint} disabled={busy} onChange={event => setEndpoint(event.target.value)} placeholder="https://…" /></label>
      <label>Model ID<input required={provider === 'custom_command'} value={model} disabled={busy} onChange={event => setModel(event.target.value)} /></label>
      <label className="wide">CA certificate<textarea rows={3} maxLength={65536} value={caPEM} disabled={busy} onChange={event => setCAPEM(event.target.value)} placeholder="Optional PEM CA certificate" /></label>
    </div>
    <label className="checkbox-label"><input type="checkbox" checked={privateRoute} disabled={busy} onChange={event => setPrivateRoute(event.target.checked)} /> Private route via enrolled runner</label>
    {privateRoute && <div className="form-grid">
      <label>Runner pool<select aria-label="Runner pool" value={poolID} disabled={busy} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}>
        <option value="">Choose pool</option>
        {poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
      </select></label>
      <label>Runner<select aria-label="Runner" value={runnerID} disabled={busy} onChange={event => setRunnerID(event.target.value)}>
        <option value="">Choose active runner</option>
        {runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}
      </select></label>
      <label>Route host<input value={routeHost} disabled={busy} onChange={event => setRouteHost(event.target.value)} /></label>
      <label className="wide">Approved CIDRs<input value={cidrs} disabled={busy} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>
    </div>}
    {routeIncomplete && <p className="table-meta">Choose an active runner, route host and approved CIDR before saving.</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
    <div className="dialog-actions">
      <Button type="button" disabled={busy} onClick={onClose}>Cancel</Button>
      <Button type="submit" className="button button-primary" disabled={!canSave}>{busy ? 'Saving…' : 'Create connection'}</Button>
    </div>
  </form>
}

function message(value: unknown) { return value instanceof Error ? value.message : 'The server returned an unknown error.' }
