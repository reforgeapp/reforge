import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { inventoryAPI } from '../api/inventory'
import { api } from '../api/client'
import { ForgeLink, forgeName, forgePages } from '../components/ForgeLink'
import { Icon } from '../components/Icons'
import { autopilotAPI } from '../policy-api'
import { useSession } from './query'
import { RepositoryBaseline } from './RepositoryBaseline'
import { RepositoryImportDialog } from './RepositoryImportDialog'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { SplitView, SplitPlaceholder, Tabs } from '../components/Workspace'
import { StatusBadge } from '../components/Status'
import type { components } from '../api/schema'
import '../styles/repositories.css'

type Repository = components['schemas']['Repository']
type SavedView = { q: string; provider: string; teamID: string; status: string }
type MaintenanceConfig = components['schemas']['MaintenanceConfig']

const statuses = ['all', 'active', 'archived', 'paused', 'missing', 'stale']
const tone = (value: string) => value === 'active' || value === 'complete' || value === 'fresh' ? 'green' : value === 'failed' || value === 'missing' ? 'red' : value === 'paused' || value === 'stale' || value === 'running' ? 'amber' : 'neutral'
const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function RepositoriesPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const routeSearch = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const [q, setQ] = useState(routeSearch.q ?? '')
  const [provider, setProvider] = useState(routeSearch.provider ?? '')
  const [teamID, setTeamID] = useState(routeSearch.team_id ?? '')
  const [status, setStatus] = useState(routeSearch.status ?? 'all')
  const [detailID, setDetailID] = useState(routeSearch.repository ?? '')
  const [syncOpen, setSyncOpen] = useState(false)
  const syncJob = routeSearch.sync ?? ''
  const [savedName, setSavedName] = useState('')
  const [savedSelection, setSavedSelection] = useState('')
  const qTimer = useRef<number | undefined>(undefined)
  const [running, setRunning] = useState('')
  const [runMessage, setRunMessage] = useState<{ ok: boolean; text: string }>()
  const run = async (repo: { id: string; name: string }) => {
    setRunning(repo.id); setRunMessage(undefined)
    try { await autopilotAPI.run(orgID, repo.id, session.data?.csrf_token ?? ''); setRunMessage({ ok: true, text: `Scanning ${repo.name}; fixes start once the scan completes.` }) } catch (reason) { setRunMessage({ ok: false, text: text(reason) }) } finally { setRunning('') }
  }
  const [savedViews, setSavedViews] = useState<Record<string, SavedView>>({})
  const [notice, setNotice] = useState('')
  const teams = useQuery({ queryKey: ['org', orgID, 'teams'], queryFn: ({ signal }) => inventoryAPI.teams(orgID, signal) })
  const result = useInfiniteQuery({ queryKey: ['org', orgID, 'inventory-repositories', 'list', { q, provider, teamID, status }], queryFn: ({ pageParam, signal }) => inventoryAPI.repositories(orgID, { q, provider: provider || undefined, team_id: teamID || undefined, status: status === 'all' ? undefined : status, cursor: pageParam, limit: 50, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const rows = [...new Map((result.data?.pages.flatMap(page => page.items) ?? []).map(item => [item.id, item])).values()]
  const repoSort = useSort(rows, { name: repo => repo.name, forge: repo => repo.provider, team: repo => repo.team_ids.map(id => teams.data?.items.find(team => team.id === id)?.name ?? id).join(', '), status: repo => repo.paused ? 'paused' : repo.archived ? 'archived' : repo.accessible ? (repo.sync_state ?? 'active') : 'missing', freshness: repo => repo.last_synced_at ? Date.parse(repo.last_synced_at) : undefined }, { key: 'name', dir: 'asc' })
  useEffect(() => { setQ(routeSearch.q ?? ''); setProvider(routeSearch.provider ?? ''); setTeamID(routeSearch.team_id ?? ''); setStatus(routeSearch.status ?? 'all') }, [routeSearch.q, routeSearch.provider, routeSearch.team_id, routeSearch.status])
  useEffect(() => () => { if (qTimer.current) window.clearTimeout(qTimer.current) }, [])
  useEffect(() => { setDetailID(routeSearch.repository ?? '') }, [routeSearch.repository])
  useEffect(() => {
    const key = `reforge.saved-views.${session.data?.user.id ?? 'unknown'}.${orgID}`
    try {
      const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? '{}')
      if (!parsed || typeof parsed !== 'object') throw new Error('invalid saved views')
      const valid: Record<string, SavedView> = {}
      for (const [name, value] of Object.entries(parsed).slice(0, 25)) {
        if (name.length > 80 || !value || typeof value !== 'object') continue
        const candidate = value as Record<string, unknown>
        if ([candidate.q, candidate.provider, candidate.teamID, candidate.status].every(item => typeof item === 'string' && item.length <= 256)) valid[name] = candidate as SavedView
      }
      setSavedViews(valid)
    } catch { setNotice('Saved views unavailable in this browser.') }
  }, [orgID, session.data?.user.id])
  const updateURL = (next: Record<string, string>, replace = true) => { const current = { ...routeSearch }; Object.entries(next).forEach(([key, value]) => { if (value) current[key] = value; else delete current[key] }); void navigate({ search: current, replace }) }
  const filter = (setter: (value: string) => void, key: string, value: string, replace = true) => { setter(value); if (key === 'q') { if (qTimer.current) window.clearTimeout(qTimer.current); qTimer.current = window.setTimeout(() => updateURL({ q: value }, replace), 1000) } else updateURL({ [key]: value }, replace) }
  const persistViews = (views: typeof savedViews) => { try { localStorage.setItem(`reforge.saved-views.${session.data?.user.id ?? 'unknown'}.${orgID}`, JSON.stringify(views)); setSavedViews(views); setNotice('Saved view saved.'); return true } catch { setNotice('Saved views could not be saved.'); return false } }
  const saveView = () => { const name = savedName.trim(); if (!name || name.length > 80) { setNotice('Use a view name between 1 and 80 characters.'); return }; if (!Object.hasOwn(savedViews, name) && Object.keys(savedViews).length >= 25) { setNotice('Delete a saved view before adding another.'); return }; if (persistViews({ ...savedViews, [name]: { q, provider, teamID, status } })) { setNotice(`Saved view “${name}”.`); setSavedName('') } }
  const loadView = (name: string) => { const view = savedViews[name]; if (!view) return; setQ(view.q); setProvider(view.provider); setTeamID(view.teamID); setStatus(view.status); updateURL({ q: view.q, provider: view.provider, team_id: view.teamID, status: view.status }, false) }
  const csrf = session.data?.csrf_token ?? ''
  return <div className="stack repositories-page">
    <div className="repository-toolbar" aria-label="Repository filters"><label>Search<input value={q} onChange={event => filter(setQ, 'q', event.target.value)} placeholder="Repository name" /></label><label>Forge<select value={provider} onChange={event => filter(setProvider, 'provider', event.target.value, false)}><option value="">All forges</option><option value="github">GitHub</option><option value="gitlab">GitLab</option><option value="gitea">Gitea</option></select></label><label>Team<select value={teamID} onChange={event => filter(setTeamID, 'team_id', event.target.value, false)}><option value="">All teams</option>{(teams.data?.items ?? []).map(team => <option key={team.id} value={team.id}>{team.name}</option>)}</select></label><label>Status<select value={status} onChange={event => filter(setStatus, 'status', event.target.value, false)}>{statuses.map(value => <option key={value} value={value}>{value === 'all' ? 'All status' : value}</option>)}</select></label><Button className="button button-primary" onClick={() => setSyncOpen(true)}>Sync inventory</Button></div>
    {teams.error && <p className="error-text" role="alert">Teams unavailable: {text(teams.error)} <Button onClick={() => void teams.refetch()}>Retry</Button></p>}
    <div className="subsection-actions"><div className="row-actions"><input aria-label="Saved view name" value={savedName} onChange={event => setSavedName(event.target.value)} placeholder="Saved view name" /><Button onClick={saveView}>Save view</Button><select aria-label="Saved views" value={savedSelection} onChange={event => { setSavedSelection(event.target.value); loadView(event.target.value) }}><option value="">Load saved view</option>{Object.keys(savedViews).map(name => <option key={name} value={name}>{name}</option>)}</select>{savedSelection && <Button onClick={() => { const next = { ...savedViews }; delete next[savedSelection]; persistViews(next); setSavedSelection('') }}>Delete saved view</Button>}</div>{notice && <span role="status" className="table-meta">{notice}</span>}</div>
    {runMessage && <p className={runMessage.ok ? 'run-notice' : 'error-text'} role="status">{runMessage.text}{runMessage.ok && <> <a href={`/org/${encodeURIComponent(orgID)}/runs`}>View runs</a></>}</p>}
    <SplitView listLabel="Repositories" selected={!!detailID} onBack={() => { setDetailID(''); updateURL({ repository: '' }) }} list={result.isLoading ? <StatePanel kind="loading" title="Loading repositories" detail="Fetching persisted inventory for this organisation." /> : result.error ? <StatePanel kind="error" title="Repositories could not be loaded" detail={text(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} /> : <>{!rows.length ? <><div className="state-card"><span className="state-icon teal" aria-hidden="true">▦</span><div><h2>{q || provider || teamID || status !== 'all' ? 'No repositories match these filters' : 'No repositories imported'}</h2><p>{q || provider || teamID || status !== 'all' ? 'Adjust filters or start a new inventory sync.' : 'Run a forge inventory sync to preview and import repositories.'}</p></div></div><EmptyTable label="Inventory records appear here after an import completes." /></> : <DataTable caption="Repository inventory"><table><thead><tr>{repoSort.header('name', 'Repository')}{repoSort.header('forge', 'Forge')}{repoSort.header('team', 'Team')}{repoSort.header('status', 'Status')}{repoSort.header('freshness', 'Freshness')}<th><span className="sr-only">Run</span></th></tr></thead><tbody>{repoSort.rows.map(repo => <tr key={repo.id}><td><button className="link-button" onClick={() => { setDetailID(repo.id); updateURL({ repository: repo.id }, false) }}>{repo.name}</button><ForgeLink href={repo.url} provider={repo.provider} label={`Open ${repo.name} on ${forgeName(repo.provider)}`} iconOnly /><small className="table-meta">{repo.default_branch}</small></td><td>{repo.provider}</td><td>{repo.team_ids.length ? repo.team_ids.map(id => teams.data?.items.find(team => team.id === id)?.name ?? id).join(', ') : 'Unassigned'}</td><td><StatusBadge label={repo.paused ? 'paused' : repo.archived ? 'archived' : repo.accessible ? (repo.sync_state ?? 'active') : 'missing'} tone={tone(repo.paused ? 'paused' : repo.archived ? 'archived' : repo.accessible ? (repo.sync_state ?? 'active') : 'missing')} /></td><td>{repo.last_synced_at ? new Date(repo.last_synced_at).toLocaleString() : 'Never synced'}</td><td><button className="icon-button run-button" aria-label={`Scan and fix ${repo.name}`} title="Scan and fix now" disabled={!session.data?.csrf_token || running === repo.id} onClick={() => void run(repo)}><Icon name="play" size={16} /></button></td></tr>)}</tbody></table>{result.hasNextPage && <div className="table-note"><Button disabled={result.isFetching} onClick={() => void result.fetchNextPage()}>{result.isFetchingNextPage ? 'Loading…' : 'Load more'}</Button></div>}</DataTable>}</>} detail={detailID ? <RepositoryDetail orgID={orgID} repositoryID={detailID} onClose={() => { setDetailID(''); updateURL({ repository: '' }) }} /> : <SplitPlaceholder label="Select a repository to inspect baseline, discovery and maintenance." />} />
    <RepositoryImportDialog open={syncOpen} orgID={orgID} csrf={csrf} teams={teams.data?.items ?? []} jobID={syncJob} onJobChange={id => updateURL({ sync: id ?? '' })} onClose={() => { setSyncOpen(false); updateURL({ sync: '' }) }} onDone={() => { void client.invalidateQueries({ queryKey: ['org', orgID, 'inventory-repositories'] }) }} />
  </div>
}

function RepositoryDetail({ orgID, repositoryID, onClose }: { orgID: string; repositoryID: string; onClose: () => void }) {
  const session = useSession(); const [tab, setTab] = useState<'summary' | 'changes' | 'baseline' | 'settings'>('summary')
  const repository = useQuery({ queryKey: ['org', orgID, 'repository', repositoryID], queryFn: ({ signal }) => inventoryAPI.repository(orgID, repositoryID, signal) })
  const maintenance = useQuery({ queryKey: ['org', orgID, 'repository', repositoryID, 'maintenance'], queryFn: ({ signal }) => inventoryAPI.maintenance(orgID, repositoryID, signal), enabled: !!repository.data })
  const discovery = useQuery({ queryKey: ['org', orgID, 'repository', repositoryID, 'discovery'], queryFn: ({ signal }) => inventoryAPI.discovery(orgID, repositoryID, signal), enabled: !!repository.data, refetchInterval: query => ['queued', 'running'].includes(query.state.data?.state ?? '') ? 1500 : false })
  const [changeCursor, setChangeCursor] = useState<string>()
  const [changeItems, setChangeItems] = useState<components['schemas']['ForgeChange'][]>([])
  const changes = useQuery({ queryKey: ['org', orgID, 'repository', repositoryID, 'changes', changeCursor], queryFn: ({ signal }) => inventoryAPI.changes(orgID, repositoryID, { limit: 50, cursor: changeCursor, signal }), enabled: !!repository.data })
  useEffect(() => { if (changes.data) setChangeItems(previous => [...new Map((changeCursor ? [...previous, ...changes.data.items] : changes.data.items).map(item => [item.id, item])).values()]) }, [changeCursor, changes.data])
  if (repository.isLoading) return <section className="detail-panel" aria-label="Repository"><StatePanel kind="loading" title="Loading repository" detail="Fetching current inventory and freshness." /></section>
  if (repository.error || !repository.data) return <section className="detail-panel" aria-label="Repository"><StatePanel kind="error" title="Repository could not be loaded" detail={text(repository.error)} action={<Button onClick={() => repository.refetch()}>Retry</Button>} /></section>
  const repo = repository.data
  return <section className="detail-panel" aria-label={repo.name}><div className="stack"><h2>{repo.name}</h2><div className="forge-links"><ForgeLink href={repo.url} provider={repo.provider} />{forgePages(repo.provider, repo.url).map(page => <ForgeLink key={page.label} href={page.href} label={page.label} />)}</div><Tabs id="repository-detail" label="Repository detail" items={[{ id: 'summary', label: 'Summary' }, { id: 'changes', label: 'Changes' }, { id: 'baseline', label: 'Baseline' }, { id: 'settings', label: 'Settings' }]} value={tab} onChange={value => setTab(value as typeof tab)} />{tab === 'summary' && <div id="repository-detail-panel-summary" aria-labelledby="repository-detail-tab-summary" role="tabpanel"><dl className="detail-list"><div><dt>Forge</dt><dd>{repo.provider}</dd></div><div><dt>Branch</dt><dd>{repo.default_branch}</dd></div><div><dt>Inventory freshness</dt><dd>{repo.last_synced_at ? new Date(repo.last_synced_at).toLocaleString() : 'Never synced'}</dd></div><div><dt>Sync state</dt><dd>{repo.sync_state ?? 'unknown'}{repo.sync_reason ? ` · ${repo.sync_reason}` : ''}</dd></div></dl><DiscoveryPanel orgID={orgID} repositoryID={repositoryID} csrf={session.data?.csrf_token ?? ''} scan={discovery.data} error={discovery.error} onRetry={() => void discovery.refetch()} /></div>}{tab === 'changes' && <div id="repository-detail-panel-changes" aria-labelledby="repository-detail-tab-changes" role="tabpanel">{changes.error && <p className="error-text" role="alert">Changes unavailable: {text(changes.error)}</p>}{changes.data?.snapshot_state !== 'fresh' && !changes.error && <p className="table-meta">Change snapshot is {changes.data?.snapshot_state ?? 'unavailable'}; native changes may be incomplete.</p>}<h3>Native changes</h3>{changeItems.length ? <ul className="compact-list">{changeItems.map(change => <li key={change.id}><ForgeLink href={change.url} label={change.title} /> · {change.state}</li>)}</ul> : <EmptyTable label="No persisted changes available." />}{changes.data?.complete === false && <Button disabled={changes.isFetching} onClick={() => setChangeCursor(changes.data?.next_cursor)}>{changes.isFetching ? 'Loading…' : 'Load more changes'}</Button>}</div>}{tab === 'baseline' && <div id="repository-detail-panel-baseline" aria-labelledby="repository-detail-tab-baseline" role="tabpanel"><RepositoryBaseline orgID={orgID} repositoryID={repositoryID} /></div>}{tab === 'settings' && <div id="repository-detail-panel-settings" aria-labelledby="repository-detail-tab-settings" role="tabpanel"><RunnerPoolsPanel orgID={orgID} repositoryID={repositoryID} csrf={session.data?.csrf_token ?? ''} /><MaintenancePanel orgID={orgID} repositoryID={repositoryID} csrf={session.data?.csrf_token ?? ''} changes={changeItems} config={maintenance.data} error={maintenance.error} onRetry={() => void maintenance.refetch()} onSaved={() => void maintenance.refetch()} /></div>}</div></section>
}

function DiscoveryPanel({ orgID, repositoryID, csrf, scan, error, onRetry }: { orgID: string; repositoryID: string; csrf: string; scan?: components['schemas']['DiscoveryScan']; error: unknown; onRetry: () => void }) {
  const [busy, setBusy] = useState(false); const [message, setMessage] = useState('')
  const start = async () => { setBusy(true); setMessage(''); try { await inventoryAPI.startDiscovery(orgID, repositoryID, csrf); onRetry() } catch (reason) { setMessage(text(reason)) } finally { setBusy(false) } }
  return <section className="detail-section"><h3>Discovery</h3>{error ? <p className="error-text" role="alert">{text(error)} <Button onClick={onRetry}>Retry</Button></p> : null}<p><StatusBadge label={scan?.state ?? 'not_started'} tone={tone(scan?.state ?? 'not_started')} /> {scan?.observed_at ? `Observed ${new Date(scan.observed_at).toLocaleString()}` : 'No scan recorded'}</p>{scan?.reason && <p className="table-meta">{scan.reason}</p>}{message && <p className="error-text" role="alert">{message}</p>}<Button disabled={busy || !csrf || ['queued', 'running'].includes(scan?.state ?? '')} onClick={start}>{busy ? 'Starting…' : 'Start scan'}</Button></section>
}

function RunnerPoolsPanel({ orgID, repositoryID, csrf }: { orgID: string; repositoryID: string; csrf: string }) {
  const pools = useQuery({ queryKey: ['org', orgID, 'repository-runner-pools'], queryFn: ({ signal }) => api.getRunnerPools(orgID, { state: 'active', limit: 100, signal }) })
  const [busy, setBusy] = useState(''); const [message, setMessage] = useState('')
  const toggle = async (pool: components['schemas']['RunnerPool'], on: boolean) => {
    setBusy(pool.id); setMessage('')
    try { await api.updateRunnerPool(orgID, pool.id, pool.version, { name: pool.name, state: pool.state, repository_ids: on ? [...pool.repository_ids, repositoryID] : pool.repository_ids.filter(id => id !== repositoryID) }, csrf); await pools.refetch() } catch (reason) { setMessage(text(reason)) } finally { setBusy('') }
  }
  return <section className="detail-section"><h3>Runner pools</h3>{pools.error ? <p className="error-text" role="alert">{text(pools.error)} <Button onClick={() => void pools.refetch()}>Retry</Button></p> : null}{pools.data && !pools.data.items.length && <p className="table-meta">No active runner pools. <a href={`/org/${encodeURIComponent(orgID)}/runners`}>Create one in Runners</a>.</p>}<div className="checkbox-list">{pools.data?.items.map(pool => <label key={pool.id} className="checkbox-label"><input type="checkbox" disabled={!csrf || !!busy || pool.builtin} checked={pool.repository_ids.includes(repositoryID)} onChange={event => void toggle(pool, event.target.checked)} />{pool.name}</label>)}</div>{message && <p className="error-text" role="alert">{message}</p>}</section>
}

function MaintenancePanel({ orgID, repositoryID, csrf, changes, config, error, onRetry, onSaved }: { orgID: string; repositoryID: string; csrf: string; changes: components['schemas']['ForgeChange'][]; config?: MaintenanceConfig; error: unknown; onRetry: () => void; onSaved: () => void }) {
  const [authority, setAuthority] = useState<MaintenanceConfig['merge_authority']>('observe'); const [bots, setBots] = useState<MaintenanceConfig['trusted_bots']>([]); const [kind, setKind] = useState<'renovate' | 'dependabot'>('renovate'); const [actorID, setActorID] = useState(''); const [busy, setBusy] = useState(false); const [message, setMessage] = useState('')
  useEffect(() => { if (config) { setAuthority(config.merge_authority); setBots(config.trusted_bots) } }, [config])
  const save = async (trusted = bots) => { if (!config) return; setBusy(true); setMessage(''); try { await inventoryAPI.updateMaintenance(orgID, repositoryID, config.version, { repository_id: repositoryID, trusted_bots: trusted, merge_authority: authority, version: config.version }, csrf); setMessage('Maintenance configuration saved.'); onSaved() } catch (reason) { setMessage(text(reason)) } finally { setBusy(false) } }
  const detected = [...new Map(changes.filter(change => change.author_type.toLowerCase() === 'bot' && change.author_id && /dependabot|renovate/i.test(change.author_login) && !bots.some(bot => bot.actor_id === change.author_id)).map(change => [change.author_id, { kind: /renovate/i.test(change.author_login) ? 'renovate' as const : 'dependabot' as const, actor_id: change.author_id, login: change.author_login }])).values()]
  const addBot = () => { if (!actorID.trim() || bots.some(bot => bot.kind === kind && bot.actor_id === actorID.trim())) return; setBots([...bots, { kind, actor_id: actorID.trim() }]); setActorID('') }
  return <section className="detail-section"><h3>Maintenance configuration</h3>{error ? <p className="error-text" role="alert">{text(error)} <Button onClick={onRetry}>Retry</Button></p> : null}{config && <><label>Requested merge authority<select value={authority} onChange={event => setAuthority(event.target.value as MaintenanceConfig['merge_authority'])}><option value="observe">Observe</option><option value="reforge">Reforge</option><option value="bot">Bot</option></select></label><fieldset><legend>Trusted native bot identities</legend>{detected.map(bot => <p key={bot.actor_id} className="row-actions"><span>{bot.login} opens pull requests here</span><Button className="button button-primary" disabled={busy || !csrf} onClick={() => { const next = [...bots, { kind: bot.kind, actor_id: bot.actor_id }]; setBots(next); void save(next) }}>Trust {bot.login}</Button></p>)}{bots.map((bot, index) => <p key={`${bot.kind}-${bot.actor_id}`} className="table-meta">{bot.kind} · {bot.actor_id} <Button onClick={() => setBots(bots.filter((_, item) => item !== index))}>Remove</Button></p>)}<div className="row-actions"><select aria-label="Bot kind" value={kind} onChange={event => setKind(event.target.value as 'renovate' | 'dependabot')}><option value="renovate">Renovate</option><option value="dependabot">Dependabot</option></select><input aria-label="Native actor ID" value={actorID} onChange={event => setActorID(event.target.value)} placeholder="Verified native actor ID" /><Button onClick={addBot}>Add identity</Button></div></fieldset>{message && <p className={message.includes('saved') ? 'table-meta' : 'error-text'} role="alert">{message}</p>}<Button className="button button-primary" disabled={busy || !csrf} onClick={() => void save()}>{busy ? 'Saving…' : 'Save maintenance configuration'}</Button></>}</section>
}
