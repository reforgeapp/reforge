import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api, type Runner, type RunnerPool, type RunnerPoolInput } from '../api/client'
import { runnerPoolsListQuery, useSession } from './query'
import { Button, Dialog } from '../components/Accessible'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { Toolbar, SplitView, DetailPanel, SplitPlaceholder } from '../components/Workspace'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const tone = (state: string) => state === 'active' ? 'green' as const : state === 'draining' ? 'amber' as const : 'red' as const
const heartbeat = (value: string) => { const timestamp = new Date(value).getTime(); if (!Number.isFinite(timestamp)) return 'Unknown'; const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000)); return seconds < 60 ? `${seconds}s ago` : seconds < 3600 ? `${Math.round(seconds / 60)}m ago` : `${Math.round(seconds / 3600)}h ago` }

export function RunnersPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const selectedID = search.pool ?? ''
  const [detailTab, setDetailTab] = useState<'overview' | 'runners' | 'repositories'>('overview')
  const [newPool, setNewPool] = useState(false)
  const [editPool, setEditPool] = useState<RunnerPool>()
  const [enrollment, setEnrollment] = useState<{ token: string; expires_at: string; pool: string }>()
  const [enrollmentError, setEnrollmentError] = useState('')
  const [actionError, setActionError] = useState('')
  const csrf = session.data?.csrf_token ?? ''

  const q = search.q ?? ''
  const state = search.state ?? ''
  useEffect(() => { setDetailTab('overview') }, [selectedID])
  const pools = useInfiniteQuery(runnerPoolsListQuery(orgID, { q: q || undefined, state: state || undefined }))
  const loaded = [...new Map((pools.data?.pages.flatMap(page => page.items) ?? []).map(item => [item.id, item])).values()]
  const poolSort = useSort(loaded, { name: pool => pool.name, state: pool => pool.state, runners: pool => pool.runner_count, busy: pool => pool.busy_slots, repositories: pool => pool.repository_ids.length }, { key: 'name', dir: 'asc' })
  const lastPage = pools.data?.pages.at(-1)

  const detail = useQuery({ queryKey: ['org', orgID, 'runner-pool', selectedID], queryFn: ({ signal }) => api.getRunnerPool(orgID, selectedID, signal), enabled: !!selectedID })
  const runners = useInfiniteQuery({ queryKey: ['org', orgID, 'runner-pools', selectedID, 'runners'], queryFn: ({ pageParam, signal }) => api.getRunners(orgID, selectedID, { cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, enabled: !!selectedID })
  const repositories = useInfiniteQuery({ queryKey: ['org', orgID, 'runner-repositories'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const runnerItems = runners.data?.pages.flatMap(page => page.items) ?? []
  const runnerSort = useSort(runnerItems, { name: runner => runner.name, state: runner => runner.state, heartbeat: runner => Date.parse(runner.last_seen_at), busy: runner => runner.busy_slots, routes: runner => runner.route_count, credential: runner => Date.parse(runner.credential_expires_at) }, { key: 'heartbeat', dir: 'desc' })
  const repositoryItems = repositories.data?.pages.flatMap(page => page.items) ?? []
  const repositoryName = (id: string) => repositoryItems.find(item => item.id === id)?.name ?? `Repository unavailable (${id})`

  const refresh = () => { void client.invalidateQueries({ queryKey: ['org', orgID, 'runner-pools'] }); void client.invalidateQueries({ queryKey: ['org', orgID, 'runner-pool'] }) }
  const select = (id: string) => { void navigate({ search: previous => ({ ...previous, pool: id }) }) }
  const clear = () => { void navigate({ search: previous => { const next = { ...previous }; delete next.pool; return next } }) }
  const setFilter = (key: 'q' | 'state', value: string) => { void navigate({ search: previous => ({ ...previous, [key]: value || undefined }) }) }
  const enrol = async (pool: RunnerPool) => { setEnrollmentError(''); try { const token = await api.createEnrollment(orgID, pool.id, csrf); setEnrollment({ ...token, pool: pool.name }) } catch (reason) { setEnrollment(undefined); setEnrollmentError(message(reason)) } }
  const act = async (run: () => Promise<unknown>) => { setActionError(''); try { await run(); refresh() } catch (reason) { setActionError(message(reason)) } }

  const poolTable = pools.isLoading && !loaded.length ? <StatePanel kind="loading" title="Loading runner pools" detail="Fetching execution trust boundaries for this organisation." /> : pools.error ? <StatePanel kind="error" title="Runner pools could not be loaded" detail={message(pools.error)} action={<Button onClick={() => pools.refetch()}>Retry</Button>} /> : <DataTable caption="Runner pools"><table><thead><tr>{poolSort.header('name', 'Pool')}{runnerSort.header('state', 'State')}{poolSort.header('runners', 'Runners')}{runnerSort.header('busy', 'Busy')}{poolSort.header('repositories', 'Repositories')}<th><span className="sr-only">Actions</span></th></tr></thead><tbody>{poolSort.rows.map(pool => <tr key={pool.id}><td><button className="link-button" onClick={() => select(pool.id)}>{pool.name}</button>{pool.builtin && <> <StatusBadge label="built-in" tone="teal" /></>}</td><td><StatusBadge label={pool.state} tone={tone(pool.state)} /></td><td>{pool.runner_count}</td><td>{pool.busy_slots}</td><td>{pool.repository_ids.length}</td><td><div className="row-actions"><Button className="button button-sm" onClick={() => select(pool.id)}>Open</Button>{!pool.builtin && <Button className="button button-sm" disabled={!csrf || pool.state === 'revoked'} onClick={() => void enrol(pool)}>Enroll runner</Button>}</div></td></tr>)}</tbody></table>{!loaded.length && <EmptyTable label="No runner pools match these filters." />}{lastPage && !lastPage.complete && <div className="table-note"><Button disabled={pools.isFetching} onClick={() => void pools.fetchNextPage()}>{pools.isFetchingNextPage ? 'Loading…' : 'Load more pools'}</Button></div>}</DataTable>

  const poolDetail = !selectedID ? <SplitPlaceholder label="Select a pool to view runners, capacity and routes." />
    : detail.isLoading ? <StatePanel kind="loading" title="Loading pool" detail="Fetching capacity and trust data." />
    : detail.error || !detail.data ? <StatePanel kind="error" title="Pool unavailable" detail={message(detail.error)} action={<Button onClick={() => detail.refetch()}>Retry</Button>} />
    : <DetailPanel title={detail.data.name} status={<p><StatusBadge label={detail.data.state} tone={tone(detail.data.state)} /> <span className="table-meta">trust unknown · runtime unknown · configuration version {detail.data.version}</span></p>} actions={<>
        {!detail.data.builtin && <Button className="button button-primary" disabled={!csrf || detail.data.state === 'revoked'} onClick={() => void enrol(detail.data!)}>Enrol runner</Button>}
        {!detail.data.builtin && <Button disabled={!csrf} onClick={() => setEditPool(detail.data)}>Edit</Button>}
        <Button disabled={!csrf || detail.data.state === 'revoked'} onClick={() => void act(() => setPoolState(orgID, detail.data!, detail.data!.state === 'draining' ? 'active' : 'draining', csrf))}>{detail.data.state === 'draining' ? 'Activate' : 'Drain'}</Button>
        {!detail.data.builtin && <Button className="button button-danger" disabled={!csrf || detail.data.state === 'revoked'} onClick={() => { if (window.confirm(`Revoke runner pool ${detail.data!.name}? Existing runner credentials will stop working.`)) void act(() => setPoolState(orgID, detail.data!, 'revoked', csrf)) }}>Revoke</Button>}
      </>}>
      <nav className="detail-tabs" aria-label="Runner pool detail"><Button aria-pressed={detailTab === 'overview'} onClick={() => setDetailTab('overview')}>Overview</Button><Button aria-pressed={detailTab === 'runners'} onClick={() => setDetailTab('runners')}>Runners</Button><Button aria-pressed={detailTab === 'repositories'} onClick={() => setDetailTab('repositories')}>Repositories</Button></nav>
      {detailTab === 'overview' && <dl className="metric-inline">
        <div><dt>Active runners</dt><dd>{detail.data.runner_count}</dd></div>
        <div><dt>Busy slots</dt><dd>{detail.data.busy_slots}</dd></div>
        <div><dt>Repositories</dt><dd>{detail.data.repository_ids.length}</dd></div>
      </dl>}
      {detailTab === 'repositories' && <div><h3>Repositories</h3>{detail.data.repository_ids.length ? <ul className="compact-list">{detail.data.repository_ids.map(id => <li key={id}>{repositoryName(id)}</li>)}</ul> : <p className="table-meta">No repositories assigned.</p>}</div>}
      {detailTab === 'runners' && <div><h3>Enrolled runners</h3>{runners.isLoading ? <p className="table-meta">Loading runners…</p> : runners.error ? <p className="error-text" role="alert">Runners unavailable: {message(runners.error)}</p> : !runnerItems.length ? <EmptyTable label="No runners have enrolled in this pool." /> : <DataTable caption="Enrolled runners"><table><thead><tr>{runnerSort.header('name', 'Runner')}{poolSort.header('state', 'State')}{runnerSort.header('heartbeat', 'Heartbeat')}{poolSort.header('busy', 'Busy')}{runnerSort.header('routes', 'Routes')}{runnerSort.header('credential', 'Credential')}<th><span className="sr-only">Actions</span></th></tr></thead><tbody>{runnerSort.rows.map((runner: Runner) => <tr key={runner.id}><td>{runner.name}</td><td><StatusBadge label={runner.state} tone={runner.state === 'active' ? 'green' : 'red'} /></td><td>{heartbeat(runner.last_seen_at)}</td><td>{runner.busy_slots}</td><td>{runner.route_count}</td><td>{new Date(runner.credential_expires_at).toLocaleDateString()}</td><td>{!detail.data.builtin && <Button className="button button-sm" disabled={!csrf} onClick={() => { if (window.confirm(`Revoke runner ${runner.name}?`)) void act(() => revokeRunner(orgID, runner, csrf)) }}>Revoke</Button>}</td></tr>)}</tbody></table></DataTable>}{runners.hasNextPage && <Button disabled={runners.isFetchingNextPage} onClick={() => void runners.fetchNextPage()}>{runners.isFetchingNextPage ? 'Loading…' : 'Load more runners'}</Button>}</div>}
      {actionError && <p className="error-text" role="alert">{actionError}</p>}
      {enrollmentError && <p className="error-text" role="alert">{enrollmentError}</p>}
    </DetailPanel>

  return <div className="stack">
    <Toolbar label="Runner pool filters">
      <label>Search<input value={q} onChange={event => setFilter('q', event.target.value)} placeholder="Pool name" /></label>
      <label>State<select value={state} onChange={event => setFilter('state', event.target.value)}><option value="">All states</option><option value="active">Active</option><option value="draining">Draining</option><option value="revoked">Revoked</option></select></label>
      <Button onClick={refresh}>Refresh</Button>
      <Button className="button button-primary" disabled={!csrf} title={!csrf ? 'Refresh session before creating a runner pool.' : undefined} onClick={() => setNewPool(true)}>Create pool</Button>
    </Toolbar>
    {!csrf && <p className="table-meta" role="status">Write actions unavailable until the session is refreshed.</p>}
    <SplitView listLabel="Runner pool inventory" selected={!!selectedID} onBack={clear} list={poolTable} detail={poolDetail} />
    {(newPool || editPool) && <PoolForm pool={editPool} orgID={orgID} csrf={csrf} repositories={repositoryItems} repositoriesError={repositories.error ? message(repositories.error) : ''} moreRepositories={repositories.hasNextPage} loadingRepositories={repositories.isFetchingNextPage} onMoreRepositories={() => void repositories.fetchNextPage()} onClose={() => { setNewPool(false); setEditPool(undefined) }} onSaved={() => { setNewPool(false); setEditPool(undefined); refresh() }} />}
    {enrollment && <EnrollmentDialog enrollment={enrollment} onClose={() => setEnrollment(undefined)} />}
  </div>
}

async function setPoolState(orgID: string, pool: RunnerPool, state: RunnerPoolInput['state'], csrf: string) {
  await api.updateRunnerPool(orgID, pool.id, pool.version, { name: pool.name, state, repository_ids: pool.repository_ids }, csrf)
}
async function revokeRunner(orgID: string, runner: Runner, csrf: string) {
  await api.revokeRunner(orgID, runner.id, runner.version, csrf)
}

function PoolForm({ pool, orgID, csrf, repositories, repositoriesError, moreRepositories, loadingRepositories, onMoreRepositories, onClose, onSaved }: { pool?: RunnerPool; orgID: string; csrf: string; repositories: Array<{ id: string; name: string }>; repositoriesError: string; moreRepositories?: boolean; loadingRepositories: boolean; onMoreRepositories: () => void; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(pool?.name ?? '')
  const [state, setState] = useState<RunnerPoolInput['state']>(pool?.state ?? 'active')
  const [selected, setSelected] = useState<string[]>(pool?.repository_ids ?? [])
  const [search, setSearch] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const matches = useMemo(() => repositories.filter(item => item.name.toLowerCase().includes(search.trim().toLowerCase())), [repositories, search])
  const save = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    const payload: RunnerPoolInput = { name: name.trim(), state, repository_ids: selected }
    try { if (pool) await api.updateRunnerPool(orgID, pool.id, pool.version, payload, csrf); else await api.createRunnerPool(orgID, payload, csrf); onSaved() } catch (reason) { setError(message(reason)) } finally { setBusy(false) }
  }
  return <Dialog open title={pool ? 'Edit runner pool' : 'Create runner pool'} onClose={onClose}><form className="form-stack" onSubmit={save}>
    <label>Name<input required value={name} onChange={event => setName(event.target.value)} /></label>
    {pool && <label>State<select value={state} onChange={event => setState(event.target.value as RunnerPoolInput['state'])}><option value="active">Active</option><option value="draining">Draining</option><option value="revoked">Revoked</option></select></label>}
    <fieldset><legend>Repository scope</legend><p className="table-meta">Leave the list empty for an onboarding pool with no repository job authority.</p>
      <label>Search repositories<input value={search} onChange={event => setSearch(event.target.value)} placeholder="Filter by name" /></label>
      {repositoriesError && <p className="error-text" role="alert">Repositories unavailable: {repositoriesError}</p>}<div className="checkbox-list">{matches.map(repository => <label key={repository.id} className="checkbox-label"><input type="checkbox" checked={selected.includes(repository.id)} onChange={event => setSelected(previous => event.target.checked ? [...previous, repository.id] : previous.filter(id => id !== repository.id))} />{repository.name}</label>)}{!matches.length && <p className="table-meta">No repositories match.</p>}</div>{moreRepositories && <Button type="button" disabled={loadingRepositories} onClick={onMoreRepositories}>{loadingRepositories ? 'Loading…' : 'Load more repositories'}</Button>}
      {selected.length > 0 && <p className="table-meta">{selected.length} repositories assigned.</p>}
    </fieldset>
    {error && <p className="error-text" role="alert">{error}</p>}
    <div className="dialog-actions"><Button type="button" onClick={onClose}>Cancel</Button><Button className="button button-primary" disabled={busy || !csrf}>{busy ? 'Saving…' : 'Save pool'}</Button></div>
  </form></Dialog>
}

function EnrollmentDialog({ enrollment, onClose }: { enrollment: { token: string; expires_at: string; pool: string }; onClose: () => void }) {
  const endpoint = window.location.origin
  const loopback = window.location.hostname === '127.0.0.1' || window.location.hostname === '::1'
  const development = loopback ? ' --development' : ''
  return <Dialog open title="Runner enrollment token" onClose={onClose}><div className="stack"><p className="dialog-copy">This token is shown once for {enrollment.pool}. Save it in a private file with mode 0600 before it expires.</p><label>One-use token<input readOnly value={enrollment.token} /></label><p className="table-meta">Expires {new Date(enrollment.expires_at).toLocaleString()}.</p><pre className="command-block">{`chmod 600 /path/token\nreforge-runner enroll --endpoint ${endpoint} --credentials /path/runner-credentials --token-file /path/token${development}`}</pre><pre className="command-block">{`reforge-runner connector --endpoint ${endpoint} --credentials /path/runner-credentials${development}`}</pre><p className="table-meta">Run connector after enrollment with the saved credential file. Do not put the token in command history or browser storage.</p><Button className="button button-primary" onClick={onClose}>Clear token</Button></div></Dialog>
}
