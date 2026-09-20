import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type RunnerPool, type RunnerPoolInput } from '../api/client'
import { runnerPoolsQuery, runnersQuery, useMeta, useSession } from './query'
import { Button, Dialog } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'

export function RunnersPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const [cursor, setCursor] = useState<string>()
  const [loaded, setLoaded] = useState<RunnerPool[]>([])
  const pools = useQuery(runnerPoolsQuery(orgID, cursor))
  const client = useQueryClient()
  const [selectedPool, setSelectedPool] = useState<RunnerPool | undefined>()
  const [formPool, setFormPool] = useState<RunnerPool | undefined>()
  const [newPool, setNewPool] = useState(false)
  const [enrollment, setEnrollment] = useState<{ token: string; expires_at: string; pool: string }>()
  const [enrollmentError, setEnrollmentError] = useState('')
  useEffect(() => { if (pools.data) setLoaded(previous => [...new Map((cursor ? [...previous, ...pools.data.items] : pools.data.items).map(item => [item.id, item])).values()]) }, [cursor, pools.data])
  if (pools.isLoading && !loaded.length) return <StatePanel kind="loading" title="Loading runner pools" detail="Fetching execution trust boundaries for this organisation." />
  if (pools.error) return <StatePanel kind="error" title="Runner pools could not be loaded" detail={message(pools.error)} action={<Button onClick={() => pools.refetch()}>Retry</Button>} />
  const items = loaded
  const csrf = session.data?.csrf_token ?? ''
  const refresh = () => { setCursor(undefined); setLoaded([]); void client.invalidateQueries({ queryKey: ['org', orgID, 'runner-pools'] }) }
  return <div className="stack"><div className="subsection-actions"><p className="table-meta">Pools may start empty. Enrollment tokens are shown once and are never persisted in browser storage.</p><Button className="button button-primary" onClick={() => setNewPool(true)}>Create pool</Button></div>{enrollmentError && <p className="error-text" role="alert">{enrollmentError}</p>}{!items.length ? <div className="state-card"><span className="state-icon teal" aria-hidden="true">▤</span><div><h2>No runner pools configured</h2><p>Create an empty pool, then enroll a runner on an approved private route.</p></div></div> : <DataTable caption="Runner pools"><table><thead><tr><th>Pool</th><th>State</th><th>Repositories</th><th>Version</th><th>Actions</th></tr></thead><tbody>{items.map(pool => <tr key={pool.id}><td><button className="link-button" onClick={() => setSelectedPool(pool)}>{pool.name}</button></td><td><StatusBadge label={pool.state} tone={pool.state === 'active' ? 'green' : pool.state === 'draining' ? 'amber' : 'red'} /></td><td>{pool.repository_ids.length ? pool.repository_ids.length : '0 (onboarding only)'}</td><td>{pool.version}</td><td><div className="row-actions"><Button onClick={() => setFormPool(pool)}>Edit</Button><Button disabled={!csrf} onClick={async () => { setEnrollmentError(''); try { const token = await api.createEnrollment(orgID, pool.id, csrf); setEnrollment({ ...token, pool: pool.name }) } catch (reason) { setEnrollment(undefined); setEnrollmentError(message(reason)) } }}>Enroll runner</Button></div></td></tr>)}</tbody></table></DataTable>}{!pools.data?.complete && <Button disabled={pools.isFetching} onClick={() => setCursor(pools.data?.next_cursor)}>{pools.isFetching ? 'Loading…' : 'Load more pools'}</Button>}{selectedPool && <PoolRunners pool={selectedPool} orgID={orgID} csrf={csrf} onClose={() => setSelectedPool(undefined)} />}{(newPool || formPool) && <PoolForm pool={formPool} orgID={orgID} csrf={csrf} onClose={() => { setNewPool(false); setFormPool(undefined) }} onSaved={() => { setNewPool(false); setFormPool(undefined); refresh() }} />}{enrollment && <EnrollmentDialog enrollment={enrollment} onClose={() => setEnrollment(undefined)} />}</div>
}

function PoolForm({ pool, orgID, csrf, onClose, onSaved }: { pool?: RunnerPool; orgID: string; csrf: string; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(pool?.name ?? '')
  const [state, setState] = useState<RunnerPoolInput['state']>(pool?.state ?? 'active')
  const [repositories, setRepositories] = useState(pool?.repository_ids.join(', ') ?? '')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); const payload: RunnerPoolInput = { name, state, repository_ids: repositories.split(',').map(item => item.trim()).filter(Boolean) }; try { if (pool) await api.updateRunnerPool(orgID, pool.id, pool.version, payload, csrf); else await api.createRunnerPool(orgID, payload, csrf); onSaved() } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  return <Dialog open title={pool ? 'Edit runner pool' : 'Create runner pool'} onClose={onClose}><form className="form-stack" onSubmit={save}><p className="dialog-copy">Repository IDs are explicit. Leave the list empty for an onboarding pool.</p><label>Name<input required value={name} onChange={event => setName(event.target.value)} /></label>{pool && <label>State<select value={state} onChange={event => setState(event.target.value as RunnerPoolInput['state'])}><option value="active">Active</option><option value="draining">Draining</option><option value="revoked">Revoked</option></select></label>}<label>Repository IDs<input value={repositories} onChange={event => setRepositories(event.target.value)} placeholder="repo-id-1, repo-id-2" /></label>{error && <p className="error-text" role="alert">{error}</p>}<div className="dialog-actions"><Button type="button" onClick={onClose}>Cancel</Button><Button className="button button-primary" disabled={busy || !csrf}>{busy ? 'Saving…' : 'Save pool'}</Button></div></form></Dialog>
}

function PoolRunners({ pool, orgID, csrf, onClose }: { pool: RunnerPool; orgID: string; csrf: string; onClose: () => void }) {
  const result = useQuery(runnersQuery(orgID, pool.id))
  const client = useQueryClient()
  const [error, setError] = useState('')
  if (result.isLoading) return <Dialog open title={pool.name} onClose={onClose}><StatePanel kind="loading" title="Loading runners" detail="Fetching enrolled runners." /></Dialog>
  if (result.error) return <Dialog open title={pool.name} onClose={onClose}><StatePanel kind="error" title="Runners could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} /></Dialog>
  const runners = result.data?.items ?? []
  const revoke = async (runner: typeof runners[number]) => { setError(''); try { await api.revokeRunner(orgID, runner.id, runner.version, csrf); await client.invalidateQueries({ queryKey: ['org', orgID, 'runner-pools', pool.id, 'runners'] }) } catch (reason) { setError(message(reason)) } }
  return <Dialog open title={`${pool.name} runners`} onClose={onClose}>{!runners.length ? <EmptyTable label="No runners have enrolled in this pool." /> : <DataTable caption="Enrolled runners"><table><thead><tr><th>Name</th><th>State</th><th>Credential expiry</th><th /></tr></thead><tbody>{runners.map(runner => <tr key={runner.id}><td>{runner.name}</td><td><StatusBadge label={runner.state} tone={runner.state === 'active' ? 'green' : 'amber'} /></td><td>{new Date(runner.credential_expires_at).toLocaleString()}</td><td><Button disabled={!csrf} onClick={() => revoke(runner)}>Revoke</Button></td></tr>)}</tbody></table></DataTable>}{error && <p className="error-text" role="alert">{error}</p>}</Dialog>
}

function EnrollmentDialog({ enrollment, onClose }: { enrollment: { token: string; expires_at: string; pool: string }; onClose: () => void }) {
  const endpoint = window.location.origin
  const meta = useMeta()
  const loopback = window.location.hostname === '127.0.0.1' || window.location.hostname === '::1'
  const development = meta.data?.development && loopback ? ' --development' : ''
  return <Dialog open title="Runner enrollment token" onClose={onClose}><div className="stack"><p className="dialog-copy">This token is shown once for {enrollment.pool}. Save it in a private file with mode 0600 before it expires.</p><label>One-use token<input readOnly value={enrollment.token} /></label><p className="table-meta">Expires {new Date(enrollment.expires_at).toLocaleString()}.</p><pre className="command-block">{`chmod 600 /path/token\nreforge-runner enroll --endpoint ${endpoint} --credentials /path/runner-credentials --token-file /path/token${development}`}</pre><pre className="command-block">{`reforge-runner connector --endpoint ${endpoint} --credentials /path/runner-credentials${development}`}</pre><p className="table-meta">Run connector after enrollment with saved credential file. Do not put token in command history or browser storage.</p><Button className="button button-primary" onClick={onClose}>Clear token</Button></div></Dialog>
}

function message(value: unknown) { return value instanceof Error ? value.message : 'The server returned an unknown error.' }
