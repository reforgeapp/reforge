import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useQueries, useQueryClient, useQuery } from '@tanstack/react-query'
import { api, ReforgeAPIError, type Connection, type ConnectionCreate } from '../api/client'
import { inventoryAPI } from '../api/inventory'
import { connectionQuery, connectionsQuery, runnerPoolsQuery, runnersQuery, useSession } from './query'
import { Button, Dialog } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { CustomProfilesPanel } from './CustomProfilesPanel'

const profileOptions = ['openai:responses', 'anthropic:messages', 'google:gemini', 'compatible:chat_completions', 'compatible:ollama', 'compatible:vllm', 'compatible:responses']

export function ConnectionsPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const [cursor, setCursor] = useState<string>()
  const [loaded, setLoaded] = useState<Connection[]>([])
  const result = useQuery(connectionsQuery(orgID, undefined, cursor))
  const client = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [selectedID, setSelectedID] = useState<string>()
  useEffect(() => { if (result.data) setLoaded(previous => [...new Map((cursor ? [...previous, ...result.data.items] : result.data.items).map(item => [item.id, item])).values()]) }, [cursor, result.data])
  if (result.isLoading && !loaded.length) return <StatePanel kind="loading" title="Loading connections" detail="Fetching persisted integrations for this organisation." />
  if (result.error) return <StatePanel kind="error" title="Connections could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} />
  const items = loaded
  const csrf = session.data?.csrf_token ?? ''
  const refresh = () => { setCursor(undefined); setLoaded([]); void client.invalidateQueries({ queryKey: ['org', orgID, 'connections'] }) }
  return <div className="stack">
    <div className="subsection-actions"><p className="table-meta">Credentials are write-only. Capability state comes from the server probe.</p><Button className="button button-primary" onClick={() => setFormOpen(true)}>Add connection</Button></div>
    {!items.length ? <div className="state-card"><span className="state-icon teal" aria-hidden="true">◇</span><div><h2>No connections configured</h2><p>Add a forge, model, agent or delivery endpoint to begin scoped discovery.</p></div></div> : <DataTable caption="Configured connections"><table><thead><tr><th>Name</th><th>Kind</th><th>Provider</th><th>State</th><th>Credential</th><th>Actions</th></tr></thead><tbody>{items.map(connection => <ConnectionRow key={connection.id} connection={connection} orgID={orgID} csrf={csrf} onRefresh={refresh} onSelect={setSelectedID} />)}</tbody></table></DataTable>}
    {!result.data?.complete && <Button disabled={result.isFetching} onClick={() => setCursor(result.data?.next_cursor)}>{result.isFetching ? 'Loading…' : 'Load more connections'}</Button>}
    <ConnectionForm open={formOpen} orgID={orgID} csrf={csrf} onClose={() => setFormOpen(false)} onCreated={() => { setFormOpen(false); refresh() }} />
    {selectedID && <ConnectionDetails connectionID={selectedID} orgID={orgID} csrf={csrf} onClose={() => setSelectedID(undefined)} onRefresh={refresh} />}
    <CustomProfilesPanel orgID={orgID} />
  </div>
}

function ConnectionRow({ connection, orgID, csrf, onRefresh, onSelect }: { connection: Connection; orgID: string; csrf: string; onRefresh: () => void; onSelect: (connectionID: string) => void }) {
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const action = async (name: string, run: () => Promise<unknown>) => { setBusy(name); setError(''); try { await run(); onRefresh() } catch (reason) { setError(message(reason)) } finally { setBusy('') } }
  return <tr><td><button className="link-button" onClick={() => onSelect(connection.id)}>{connection.name}</button>{error && <small className="error-text">{error}</small>}</td><td>{connection.kind}</td><td>{connection.provider}</td><td><StatusBadge label={connection.state} tone={connection.state === 'healthy' ? 'green' : connection.state === 'revoked' ? 'red' : 'amber'} /></td><td>Version {connection.credential_version}</td><td><div className="row-actions"><Button disabled={!!busy} onClick={() => action('test', () => api.testConnection(orgID, connection.id, connection.version, csrf))}>{busy === 'test' ? 'Testing…' : 'Test'}</Button><Button disabled={!!busy} onClick={() => action('revoke', () => api.revokeConnection(orgID, connection.id, connection.version, csrf))}>{busy === 'revoke' ? 'Revoking…' : 'Revoke'}</Button></div></td></tr>
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
  const pools = useQuery({ ...runnerPoolsQuery(orgID), enabled: open })
  const runnerQueries = useQueries({ queries: (pools.data?.items ?? []).map(pool => ({ ...runnersQuery(orgID, pool.id), enabled: open })) })
  const runners = runnerQueries.flatMap((result, index) => (result.data?.items ?? []).filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now() && pools.data?.items[index]?.state === 'active').map(runner => ({ ...runner, poolName: pools.data?.items[index]?.name ?? 'Runner pool' })))
  const runnerError = pools.error ?? runnerQueries.find(result => result.error)?.error
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
  return <Dialog open={open} title="Add connection" onClose={onClose}><form className="form-stack" onSubmit={submit}><p className="dialog-copy">Credentials are write-only and capability state comes from the server.</p><div className="form-grid"><label>Kind<select value={kind} onChange={event => setKind(event.target.value)}><option value="forge">Forge</option><option value="model">Model</option><option value="agent">Agent</option><option value="delivery">Delivery</option></select></label><label>Provider<select value={kind === 'model' ? `${provider}:${profile}` : provider} onChange={event => { const [nextProvider, nextProfile] = event.target.value.split(':'); setProvider(nextProvider); setSecret(''); setAppID(''); setInstallationID(''); if (nextProfile) setProfile(nextProfile); else if (forgeKind) { setAuthKind('token'); setProfile('') } }}>{(kind === 'model' ? profileOptions : forgeKind ? ['github', 'gitlab', 'gitea'] : agentKind ? ['codex', 'claude_code', 'gemini_cli'] : ['github', 'gitlab', 'gitea']).map(option => <option key={option} value={option}>{option}</option>)}</select></label><label className="wide">Name<input required value={name} onChange={event => setName(event.target.value)} /></label><label className="wide">Endpoint<input required type="url" value={endpoint} onChange={event => setEndpoint(event.target.value)} placeholder="https://…" /></label>{forgeKind && <label>Namespace<input value={namespace} onChange={event => setNamespace(event.target.value)} placeholder="organisation or group" /></label>}{githubForge && <label>Auth<select value={authKind} onChange={event => { setAuthKind(event.target.value); setSecret(''); setAppID(''); setInstallationID('') }}><option value="token">Token</option><option value="github_app">GitHub App</option></select></label>}{githubForge && authKind === 'github_app' && <><label>App ID<input required value={appID} onChange={event => setAppID(event.target.value)} /></label><label>Installation ID<input required value={installationID} onChange={event => setInstallationID(event.target.value)} /></label></>}{forgeKind && !githubForge && <input type="hidden" value="token" readOnly />}{(kind === 'model' || kind === 'agent') && <label>Model ID<input required={!agentKind} value={model} onChange={event => setModel(event.target.value)} /></label>}{kind === 'model' && <label>Auth<select value={authKind} onChange={event => { setAuthKind(event.target.value); setSecret('') }}><option value="api_key">API key</option></select></label>}{kind === 'agent' && <label>Auth<select value={authKind} onChange={event => setAuthKind(event.target.value)}><option value="official_runtime">Official runtime</option></select></label>}{kind === 'model' && <label>Billing route<select value={billingRoute} onChange={event => setBillingRoute(event.target.value)}><option value="direct_api">Direct API</option></select></label>}{agentKind && <label>Billing route<select value={billingRoute} onChange={event => setBillingRoute(event.target.value)}><option value="subscription">Subscription</option></select></label>}<label className="wide">CA PEM<input value={caPEM} onChange={event => setCAPEM(event.target.value)} placeholder="Optional custom CA certificate" /></label>{!agentKind && <label className="wide">Secret<input type="password" autoComplete="new-password" required={secretRequired} value={secret} onChange={event => setSecret(event.target.value)} /></label>}<label className="wide checkbox-label"><input type="checkbox" checked={privateRoute} onChange={event => setPrivateRoute(event.target.checked)} /> Use an enrolled private runner</label>{privateRoute && <>{runnerError && <p className="error-text" role="alert">{message(runnerError)}</p>}<label>Runner<select required value={runnerID} onChange={event => setRunnerID(event.target.value)}><option value="">Select a runner</option>{runners.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.poolName}</option>)}</select></label><label>Route host<input required value={routeHost} onChange={event => setRouteHost(event.target.value)} placeholder="forge.internal" /></label><label className="wide">Allowed CIDRs<input required value={cidrs} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>{!runners.length && !runnerError && <p className="table-meta">No active enrolled runners are available for this route.</p>}</>}</div>{error && <p className="error-text" role="alert">{error}</p>}<div className="dialog-actions"><Button type="button" onClick={onClose}>Cancel</Button><Button className="button button-primary" disabled={busy || !csrf || (privateRoute && (!runners.length || !!runnerError))}>{busy ? 'Saving…' : 'Save connection'}</Button></div></form></Dialog>
}

function ConnectionDetails({ connectionID, orgID, csrf, onClose, onRefresh }: { connectionID: string; orgID: string; csrf: string; onClose: () => void; onRefresh: () => void }) {
  const result = useQuery(connectionQuery(orgID, connectionID))
  const [secret, setSecret] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  useEffect(() => { const connection = result.data; if (connection) { setRouteHost(connection.private_route?.host ?? ''); setRunnerID(connection.private_route?.runner_id ?? ''); setCIDRs(connection.private_route?.cidrs.join(', ') ?? '') } }, [result.data])
  useEffect(() => { if (!result.isSuccess) setSecret('') }, [result.isSuccess])
  const run = async (name: string, action: () => Promise<unknown>) => { setBusy(name); setError(''); try { await action(); setSecret(''); onRefresh(); await result.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy('') } }
  if (result.isLoading) return <Dialog open title="Connection" onClose={onClose}><StatePanel kind="loading" title="Loading connection" detail="Fetching the current server version." /></Dialog>
  if (result.error || !result.data) return <Dialog open title="Connection" onClose={onClose}><StatePanel kind="error" title="Connection could not be loaded" detail={message(result.error)} /></Dialog>
  const connection = result.data
  return <Dialog open title={connection.name} onClose={() => { setSecret(''); onClose() }}><div className="stack"><dl className="detail-list"><div><dt>Endpoint</dt><dd>{connection.endpoint}</dd></div><div><dt>Version</dt><dd>{connection.version}</dd></div><div><dt>Billing route</dt><dd>{connection.settings.billing_route}</dd></div><div><dt>Reason</dt><dd>{connection.reason || 'No server reason provided.'}</dd></div><div><dt>Verified</dt><dd>{connection.verified_at ? new Date(connection.verified_at).toLocaleString() : 'Not verified'}</dd></div></dl>{Object.keys(connection.capabilities).length > 0 && <div><h3>Capabilities</h3><ul className="compact-list">{Object.entries(connection.capabilities).map(([name, capability]) => <li key={name}><strong>{name}</strong>: {capability.state} · {capability.reason}</li>)}</ul></div>}{connection.kind === 'forge' && <WebhookPanel orgID={orgID} connection={connection} csrf={csrf} /> }<label>Rotate secret<input type="password" autoComplete="new-password" value={secret} onChange={event => setSecret(event.target.value)} /></label><Button disabled={!secret || !!busy || !csrf} onClick={() => run('rotate', () => api.rotateConnection(orgID, connection.id, connection.version, secret, csrf))}>{busy === 'rotate' ? 'Rotating…' : 'Rotate credential'}</Button><fieldset><legend>Private route</legend><div className="form-grid"><label>Runner ID<input value={runnerID} onChange={event => setRunnerID(event.target.value)} /></label><label>Host<input value={routeHost} onChange={event => setRouteHost(event.target.value)} /></label><label className="wide">CIDRs<input value={cidrs} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8, 192.168.0.0/16" /></label></div><Button disabled={!runnerID || !routeHost || !!busy || !csrf} onClick={() => run('route', () => api.setPrivateRoute(orgID, connection.id, connection.version, { runner_id: runnerID, host: routeHost, cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) }, csrf))}>{busy === 'route' ? 'Saving…' : 'Save private route'}</Button></fieldset>{error && <p className="error-text" role="alert">{error}</p>}</div></Dialog>
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
  return <fieldset><legend>Inventory webhook</legend><p className="table-meta">{state.data?.revoked ? 'Webhook revoked.' : state.data ? `POST ${window.location.origin}${state.data.path}` : 'No webhook configured.'}</p>{secret && <label>One-time webhook secret<input readOnly type="password" value={secret} /></label>}{error && <p className="error-text" role="alert">{error}</p>}<div className="row-actions"><Button disabled={busy || !csrf || version === undefined} onClick={issue}>{state.data && !state.data.revoked ? 'Rotate webhook secret' : 'Issue webhook secret'}</Button><Button disabled={busy || !csrf || !state.data || state.data.revoked} onClick={revoke}>Revoke webhook</Button>{secret && <Button onClick={() => setSecret('')}>Clear webhook secret</Button>}</div></fieldset>
}

function message(value: unknown) { return value instanceof Error ? value.message : 'The server returned an unknown error.' }
