import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ForgeLink } from '../components/ForgeLink'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { Toolbar, SplitView, SplitPlaceholder, Tabs } from '../components/Workspace'
import { useSession } from './query'
import { eventsURL, runsAPI, type RepairRun, type RunEvent, type Task } from '../api/runs'
import { inventoryAPI } from '../api/inventory'
import { ReforgeAPIError } from '../api/client'

const terminal = new Set(['completed', 'failed', 'blocked', 'cancelled'])
const tone = (state: string) => state === 'completed' ? 'green' as const : state === 'blocked' || state === 'failed' ? 'red' as const : terminal.has(state) ? 'neutral' as const : 'amber' as const
const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const sourceURL = (value: string) => { try { const url = new URL(value); return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : undefined } catch { return undefined } }

export function RunsPage({ orgID }: { orgID: string }) {
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const session = useSession(); const client = useQueryClient()
  const [state, setState] = useState(search.state ?? '')
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined])
  useEffect(() => { setState(search.state ?? ''); setCursors([undefined]) }, [search.state])
  const repositories = useQuery({ queryKey: ['org', orgID, 'run-repositories'], queryFn: ({ signal }) => inventoryAPI.repositories(orgID, { limit: 100, signal }) })
  const cursor = cursors[cursors.length - 1]
  const result = useQuery({ queryKey: ['org', orgID, 'tasks', 'list', state, cursor], queryFn: ({ signal }) => runsAPI.tasks(orgID, { cursor, limit: 30, state: state || undefined, signal }), placeholderData: previous => previous })
  const items = result.data?.items ?? []
  const repositoryName = (id: string) => repositories.data?.items.find(repo => repo.id === id)?.name ?? id
  const runSort = useSort(items, { id: task => task.id, repository: task => repositoryName(task.repository_id), recipe: task => task.recipe, route: task => task.model_route, attempts: task => task.max_attempts, state: task => task.state, created: task => Date.parse(task.created_at) }, { key: 'created', dir: 'desc' })
  const setFilter = (value: string) => { setState(value); setCursors([undefined]); void navigate({ search: previous => ({ ...previous, state: value || undefined }) }) }
  const clear = () => { void navigate({ search: previous => { const next = { ...previous }; delete next.run; return next } }) }

  const list = <><DataTable caption="Maintenance runs"><table><thead><tr>{runSort.header('id', 'Run')}{runSort.header('repository', 'Repository')}{runSort.header('recipe', 'Recipe')}{runSort.header('route', 'Route')}{runSort.header('attempts', 'Attempts')}{runSort.header('state', 'State')}{runSort.header('created', 'Created')}<th>Next action</th></tr></thead><tbody>{runSort.rows.map(task => <tr key={task.id}><td><button className="link-button" onClick={() => void navigate({ search: previous => ({ ...previous, run: task.id }) })}>{task.id.slice(0, 8)}</button></td><td><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(task.repository_id)}`}>{repositoryName(task.repository_id)}</a></td><td>{task.recipe} <small className="table-meta">v{task.recipe_version}</small></td><td>{task.model_route || '—'}</td><td>{task.max_attempts}</td><td><StatusBadge label={task.state} tone={tone(task.state)} /></td><td>{new Date(task.created_at).toLocaleString()}</td><td>{task.reason || (task.state === 'blocked' || task.state === 'reconciling' ? 'Review recorded evidence' : 'No action')}</td></tr>)}</tbody></table>{!items.length && <EmptyTable label="No maintenance runs match this filter." />}</DataTable>{(cursors.length > 1 || result.data?.next_cursor) && <nav className="pager" aria-label="Runs pages"><Button disabled={cursors.length === 1 || result.isFetching} onClick={() => setCursors(previous => previous.slice(0, -1))}>Previous</Button><span className="table-meta">Page {cursors.length}</span><Button disabled={result.data?.complete !== false || !result.data?.next_cursor || result.isFetching} onClick={() => setCursors(previous => [...previous, result.data?.next_cursor])}>Next</Button></nav>}</>

  return <div className="stack">
    <Toolbar label="Run filters"><label>State<select value={state} onChange={event => setFilter(event.target.value)}><option value="">All states</option>{['active', 'queued', 'reproducing', 'planning', 'repairing', 'validating', 'publishing', 'blocked', 'reconciling', 'completed', 'failed', 'cancelled'].map(value => <option key={value} value={value}>{value}</option>)}</select></label></Toolbar>
    <SplitView listLabel="Runs" selected={!!search.run} onBack={clear} list={result.isLoading ? <StatePanel kind="loading" title="Loading runs" detail="Fetching recorded maintenance work." /> : result.error ? <StatePanel kind="error" title="Runs could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} /> : list} detail={search.run ? <RunDetail key={search.run} orgID={orgID} repositories={repositories.data?.items ?? []} taskID={search.run} csrf={session.data?.csrf_token ?? ''} onClose={clear} onChanged={() => { void client.invalidateQueries({ queryKey: ['org', orgID, 'tasks'] }) }} /> : <SplitPlaceholder label="Select a run to inspect stages, evidence and cancellation." />} />
  </div>
}

const lifecycleStages = ['Discover', 'Reproduce', 'Plan', 'Repair', 'Validate', 'Publish'] as const
const stageFor = (state: string) => state === 'queued' ? 'Discover' : state === 'reproducing' ? 'Reproduce' : state === 'planning' ? 'Plan' : state === 'repairing' ? 'Repair' : state === 'validating' ? 'Validate' : state === 'publishing' || state === 'completed' ? 'Publish' : undefined

function RunDetail({ orgID, repositories, taskID, csrf, onClose, onChanged }: { orgID: string; repositories: Array<{ id: string; name: string }>; taskID: string; csrf: string; onClose: () => void; onChanged: () => void }) {
  const detail = useQuery({ queryKey: ['org', orgID, 'repair-run', taskID], queryFn: ({ signal }) => loadRun(orgID, taskID, signal), retry: false })
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const [tab, setTab] = useState<'summary' | 'evidence' | 'activity'>('summary')
  const stream = useTaskEvents(orgID, taskID, () => { void detail.refetch(); onChanged() })
  if (detail.isLoading) return <section className="detail-panel" aria-label="Run details"><p>Loading run details…</p></section>
  if (detail.error) return <section className="detail-panel" role="alert"><h2>Run details unavailable</h2><p>{message(detail.error)}</p><Button onClick={() => void detail.refetch()}>Retry</Button><Button onClick={onClose}>Close</Button></section>
  const run = detail.data as RepairRun; const task = run.task; const objective = run.context.finding?.title || run.report?.reason || task.reason || 'No repair objective recorded.'; const model = run.context.model || 'Not assigned'; const blocked = task.state === 'blocked' || run.state === 'blocked'; const canCancel = !terminal.has(task.state) && task.state !== 'cancelling'; const canResume = task.state === 'blocked' || task.state === 'failed' || task.state === 'reconciling'; const reconciling = task.state === 'reconciling' || run.state === 'reconciling'
  const currentStage = stageFor(task.state)
  const actionReason = (allowed: boolean, action: string) => allowed ? '' : !csrf ? 'CSRF unavailable' : busy ? 'Action in progress' : task.state === 'cancelling' ? 'Cancellation already requested' : action === 'resume' ? 'Resume requires a blocked, failed, or reconciling run' : action === 'reconcile' ? 'Reconcile requires an unknown external outcome' : 'Run is already terminal'
  const act = async (kind: 'cancel' | 'resume' | 'reconcile') => { setBusy(true); setError(''); try { if (kind === 'reconcile') await runsAPI.reconcile(orgID, task.id, run.version, csrf); else await runsAPI[kind](orgID, task.id, task.version, csrf); await detail.refetch(); onChanged() } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  return <section className="detail-panel" aria-label={`Run ${task.id}`}><h2>{task.recipe} · {task.id.slice(0, 8)}</h2><p className="table-meta">Run ID {task.id}</p><p><StatusBadge label={task.state} tone={tone(task.state)} /> {task.reason || objective}</p>{blocked && <StatePanel kind="blocked" title="Action required" detail={task.reason || 'The server blocked this run. Review the recorded evidence before resuming.'} />}{error && <p className="error-text" role="alert">{error}</p>}<div className="row-actions"><Button disabled={!csrf || busy || !canCancel} onClick={() => void act('cancel')}>Cancel run</Button><Button disabled={!csrf || busy || !canResume} onClick={() => void act('resume')}>Resume run</Button><Button disabled={!csrf || busy || !reconciling} onClick={() => void act('reconcile')}>Reconcile outcome</Button></div>{(!canCancel || !canResume || !reconciling || !csrf || busy) && <p className="table-meta">{!canCancel ? `Cancel unavailable: ${actionReason(false, 'cancel')}` : !canResume ? `Resume unavailable: ${actionReason(false, 'resume')}` : !reconciling ? `Reconcile unavailable: ${actionReason(false, 'reconcile')}` : actionReason(false, 'cancel')}</p>}<ol className="stage-rail" aria-label="Run lifecycle">{lifecycleStages.map(stage => <li key={stage} aria-current={currentStage === stage ? 'step' : undefined}><strong>{stage}</strong></li>)}</ol><Tabs id={`run-${task.id}`} label="Run detail" items={[{ id: 'summary', label: 'Summary' }, { id: 'evidence', label: 'Evidence' }, { id: 'activity', label: 'Activity' }]} value={tab} onChange={value => setTab(value as typeof tab)} />{tab === 'summary' && <div id={`run-${task.id}-panel-summary`} aria-labelledby={`run-${task.id}-tab-summary`} role="tabpanel"><p className="table-meta">Task lifecycle: {task.state}. Native change lifecycle: {run.change ? run.change.state : 'No native change recorded'}.</p>{reconciling && <p className="table-meta">External outcome is unknown. Reconcile the native change before retrying.</p>}<dl className="detail-list"><div><dt>Recipe</dt><dd>{task.recipe} · v{task.recipe_version}</dd></div><div><dt>Repository</dt><dd><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(task.repository_id)}`}>{repositories.find(repository => repository.id === task.repository_id)?.name ?? task.repository_id}</a></dd></div><div><dt>Model</dt><dd>{model}</dd></div><div><dt>Model route</dt><dd>{task.model_route || 'Unknown'}</dd></div><div><dt>Policy</dt><dd><code>{task.policy_hash || 'Unknown'}</code></dd></div></dl></div>}{tab === 'activity' && <div id={`run-${task.id}-panel-activity`} aria-labelledby={`run-${task.id}-tab-activity`} role="tabpanel"><h3>Recorded activity</h3><p className="table-meta">{stream.status}</p>{stream.events.length ? <ul className="compact-list">{stream.events.map(event => <li key={event.id}>{new Date(event.occurred_at).toLocaleString()} · {event.type} · {event.aggregate_type}/{event.aggregate_id}</li>)}</ul> : <p>No events recorded for this run yet.</p>}</div>}{tab === 'evidence' && <div id={`run-${task.id}-panel-evidence`} aria-labelledby={`run-${task.id}-tab-evidence`} role="tabpanel"><Evidence run={run} orgID={orgID} /></div>}</section>
}

function Evidence({ run, orgID }: { run: RepairRun; orgID: string }) {
  const report = run.report; const checks = [{ stage: 'H baseline', items: report?.baseline ?? [] }, { stage: 'H+patch', items: report?.candidate ?? [] }, { stage: 'T+patch', items: report?.target ?? [] }, { stage: 'native C', items: run.candidate_checks ?? [] }]; const artifacts = [...new Set([...(report?.artifacts ?? []), ...(run.candidate_artifacts ?? [])])]
  return <div className="stack"><h3>Frozen execution</h3><dl className="detail-list"><div><dt>Native head evidence</dt><dd><code>{run.context.native_head_sha || 'Unknown'}</code></dd></div><div><dt>Plan</dt><dd><code>{run.context.plan?.digest || report?.plan_digest || 'Unknown'}</code></dd></div><div><dt>Baseline</dt><dd><code>{run.context.plan?.baseline_sha || 'Unknown'}</code></dd></div><div><dt>Target</dt><dd><code>{run.context.plan?.target_sha || 'Unknown'}</code></dd></div><div><dt>Candidate head</dt><dd><code>{run.candidate_sha || 'Unknown'}</code></dd></div><div><dt>Frozen commands</dt><dd>{run.context.plan?.recipe?.commands?.length ? <ul className="compact-list">{run.context.plan.recipe.commands.map(command => <li key={command.id}><code>{command.id}: {command.args.join(" ")}</code> · {command.directory} · {command.timeout_seconds}s · {command.report_format}</li>)}</ul> : "Unknown"}</dd></div><div><dt>Policy</dt><dd><code>{run.context.policy_hash || 'Unknown'}</code></dd></div></dl><h3>Checks and report</h3><p className="table-meta">Recorded at {new Date(run.updated_at).toLocaleString()}. These results apply only to the pinned revisions above; current native gates require a fresh observation.</p>{checks.some(group => group.items.length) ? <ul className="compact-list">{checks.flatMap(group => group.items.map((check, index) => <li key={`${group.stage}-${check.command_id}-${index}`}><strong>{group.stage}</strong> · {check.command_id}: exit {check.exit_code}; {check.complete ? 'complete' : 'incomplete'}{check.reason ? ` — ${check.reason}` : ''}{check.cases && Object.keys(check.cases).length ? <ul>{Object.entries(check.cases).map(([name, outcome]) => <li key={name}>{name}: {outcome}</li>)}</ul> : null}{check.excerpt ? <pre className="command-block">{plainText(check.excerpt)}</pre> : null}</li>))}</ul> : <p>No check results recorded.</p>}{report?.reason && <p>{report.state}: {report.reason}</p>}{run.change && <p>Native change: {run.change.state}. {sourceURL(run.change.url) ? <ForgeLink href={sourceURL(run.change.url)} label={run.change.title} /> : run.change.title}</p>}{report?.diff ? <details><summary>Source diff</summary><pre className="command-block">{plainText(report.diff)}</pre></details> : report?.patches?.length ? <details><summary>Proposed file contents ({report.patches.length} files)</summary>{report.patches.map(patch => <pre className="command-block" key={patch.path}>{patch.path}{'\n'}{decodePatch(patch.content)}</pre>)}</details> : <p>No plaintext diff recorded.</p>}{artifacts.length ? <ul className="compact-list">{artifacts.map(id => <li key={id}><a href={runsAPI.artifactURL(orgID, id)} target="_blank" rel="noreferrer">Download authorized artifact {id}</a></li>)}</ul> : <p>No artifacts recorded.</p>}</div>
}

async function loadRun(orgID: string, taskID: string, signal?: AbortSignal): Promise<RepairRun> {
  try { return await runsAPI.repair(orgID, taskID, signal) }
  catch (error) {
    if (!(error instanceof ReforgeAPIError) || error.status !== 404) throw error
    const task = await runsAPI.task(orgID, taskID, signal)
    return { task, state: task.state, version: task.version, branch: '', candidate_sha: '', candidate_checks: [], context: {}, updated_at: task.created_at }
  }
}

function decodePatch(value: string) { try { if (value.length > 1400000) return 'Content omitted: artifact exceeds browser display limit.'; const binary = atob(value); const bytes = Uint8Array.from(binary, char => char.charCodeAt(0)); return plainText(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) } catch { return 'Content is not valid UTF-8; download the authorized artifact.' } }

function useTaskEvents(orgID: string, taskID: string, onRefresh: () => void) {
  const [events, setEvents] = useState<RunEvent[]>([])
  const [status, setStatus] = useState('Connecting to event stream…')
  const callback = useRef(onRefresh)
  const lastID = useRef(0)
  useEffect(() => { callback.current = onRefresh }, [onRefresh])
  useEffect(() => {
    let source: EventSource | undefined
    let stopped = false
    let retry = 0
    let timeout: number | undefined
    setEvents([]); lastID.current = 0
    const connect = () => {
      if (stopped) return
      source = new EventSource(eventsURL(orgID, lastID.current || undefined))
      source.addEventListener('reforge', event => {
        try {
          const item = JSON.parse((event as MessageEvent).data) as RunEvent
          lastID.current = Math.max(lastID.current, item.id)
          if (item.aggregate_id === taskID) { setEvents(previous => previous.some(existing => existing.id === item.id) ? previous : [...previous.slice(-49), item]); callback.current() }
        } catch { setStatus('Received an unreadable event; current state remains authoritative.') }
      })
      source.addEventListener('reset', event => {
        let code = 'cursor_expired'
        try { code = (JSON.parse((event as MessageEvent).data) as { code?: string }).code ?? code } catch { setStatus('Event reset was malformed; current state reloaded.') }
        setStatus(code === 'access_revoked' ? 'Event access revoked; reload to authenticate again.' : 'Event cursor reset; current state reloaded.')
        setEvents([]); lastID.current = 0; callback.current()
        if (code === 'access_revoked') { stopped = true; source?.close() }
      })
      source.onerror = () => {
        source?.close()
        if (stopped) return
        setStatus('Event stream disconnected; reconnecting…')
        timeout = window.setTimeout(connect, Math.min(5000, 500 * 2 ** retry++))
      }
      source.onopen = () => { retry = 0; setStatus('Live event stream connected.') }
    }
    connect()
    return () => { stopped = true; source?.close(); if (timeout) window.clearTimeout(timeout) }
  }, [orgID, taskID])
  return { events, status }
}

function plainText(value: string) { return value.replace(/[\u0000-\u0008\u000B-\u001F\u007F-\u009F\u202A-\u202E\u2066-\u2069]/g, char => '⟨U+' + char.charCodeAt(0).toString(16).padStart(4, '0') + '⟩') }
