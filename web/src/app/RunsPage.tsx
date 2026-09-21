import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
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
  const session = useSession(); const client = useQueryClient(); const [cursor, setCursor] = useState<string>(); const [items, setItems] = useState<Task[]>([])
  const [state, setState] = useState(search.state ?? '')
  useEffect(() => { setState(search.state ?? ''); setCursor(undefined); setItems([]) }, [search.state])
  const repositories = useQuery({ queryKey: ['org', orgID, 'run-repositories'], queryFn: ({ signal }) => inventoryAPI.repositories(orgID, { limit: 100, signal }) })
  const result = useQuery({ queryKey: ['org', orgID, 'tasks', state, cursor ?? 'first'], queryFn: ({ signal }) => runsAPI.tasks(orgID, { cursor, state: state || undefined, signal }) })
  useEffect(() => { if (result.data) setItems(previous => { const next = cursor ? [...previous, ...result.data.items] : result.data.items; return [...new Map(next.map(item => [item.id, item])).values()] }) }, [cursor, result.data])
  if (result.isLoading && !items.length) return <StatePanel kind="loading" title="Loading runs" detail="Fetching recorded maintenance work." />
  if (result.error) return <StatePanel kind="error" title="Runs could not be loaded" detail={message(result.error)} action={<Button onClick={() => result.refetch()}>Retry</Button>} />
  const repositoryName = (id: string) => repositories.data?.items.find(repo => repo.id === id)?.name ?? id
  const setFilter = (value: string) => { setState(value); setCursor(undefined); setItems([]); void navigate({ search: previous => ({ ...previous, state: value || undefined }) }) }
  return <div className="stack"><div className="repository-toolbar" aria-label="Run filters"><label>State<select value={state} onChange={event => setFilter(event.target.value)}><option value="">All states</option>{['queued', 'reproducing', 'planning', 'repairing', 'validating', 'publishing', 'blocked', 'reconciling', 'completed', 'failed', 'cancelled'].map(value => <option key={value} value={value}>{value}</option>)}</select></label><span className="table-meta">{items.length} loaded{result.data?.complete === false ? ' · more available' : ''}</span></div><DataTable caption="Maintenance runs"><table><thead><tr><th>Run</th><th>Repository</th><th>Recipe</th><th>Route</th><th>Attempts</th><th>State</th><th>Created</th><th>Next action</th></tr></thead><tbody>{items.map(task => <tr key={task.id}><td><button className="link-button" onClick={() => navigate({ search: previous => ({ ...previous, run: task.id }) })}>{task.id.slice(0, 8)}</button></td><td>{repositoryName(task.repository_id)}</td><td>{task.recipe} <small className="table-meta">v{task.recipe_version}</small></td><td>{task.model_route || '—'}</td><td>{task.max_attempts}</td><td><StatusBadge label={task.state} tone={tone(task.state)} /></td><td>{new Date(task.created_at).toLocaleString()}</td><td>{task.reason || (task.state === 'blocked' || task.state === 'reconciling' ? 'Review recorded evidence' : 'No action')}</td></tr>)}</tbody></table>{!items.length && <EmptyTable label="No maintenance runs match this filter." />}</DataTable>{!result.data?.complete && <Button disabled={result.isFetching} onClick={() => setCursor(result.data?.next_cursor)}>{result.isFetching ? 'Loading…' : 'Load more runs'}</Button>}{search.run && <RunDetail key={search.run} orgID={orgID} taskID={search.run} csrf={session.data?.csrf_token ?? ''} onClose={() => navigate({ search: previous => { const next = { ...previous }; delete next.run; return next } })} onChanged={() => { setCursor(undefined); void client.invalidateQueries({ queryKey: ['org', orgID, 'tasks'] }) }} />}</div>
}

function RunDetail({ orgID, taskID, csrf, onClose, onChanged }: { orgID: string; taskID: string; csrf: string; onClose: () => void; onChanged: () => void }) {
  const detail = useQuery({ queryKey: ['org', orgID, 'repair-run', taskID], queryFn: ({ signal }) => loadRun(orgID, taskID, signal), retry: false })
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const stream = useTaskEvents(orgID, taskID, () => { void detail.refetch(); onChanged() })
  if (detail.isLoading) return <section className="state-card" aria-label="Run details"><p>Loading run details…</p></section>
  if (detail.error) return <section className="state-card" role="alert"><h2>Run details unavailable</h2><p>{message(detail.error)}</p><Button onClick={() => void detail.refetch()}>Retry</Button><Button onClick={onClose}>Close</Button></section>
  const run = detail.data as RepairRun; const task = run.task; const blocked = task.state === 'blocked' || run.state === 'blocked'; const canCancel = !terminal.has(task.state) && task.state !== 'cancelling'; const canResume = task.state === 'blocked' || task.state === 'failed' || task.state === 'reconciling'; const reconciling = task.state === 'reconciling' || run.state === 'reconciling'
  const act = async (kind: 'cancel' | 'resume' | 'reconcile') => { setBusy(true); setError(''); try { if (kind === 'reconcile') await runsAPI.reconcile(orgID, task.id, run.version, csrf); else await runsAPI[kind](orgID, task.id, task.version, csrf); await detail.refetch(); onChanged() } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  return <section className="state-card" aria-label={`Run ${task.id}`}><div className="subsection-actions"><h2>{task.recipe} · {task.id}</h2><Button onClick={onClose}>Close</Button></div><p><StatusBadge label={task.state} tone={tone(task.state)} /> {task.reason || 'No server reason recorded.'}</p>{blocked && <StatePanel kind="blocked" title="Action required" detail={task.reason || 'The server blocked this run. Review the recorded evidence before resuming.'} />}{error && <p className="error-text" role="alert">{error}</p>}<div className="row-actions"><Button disabled={!csrf || busy || !canCancel} onClick={() => void act('cancel')}>Cancel run</Button><Button disabled={!csrf || busy || !canResume} onClick={() => void act('resume')}>Resume run</Button><Button disabled={!csrf || busy || !reconciling} onClick={() => void act('reconcile')}>Reconcile outcome</Button></div><p className="table-meta">Task lifecycle: {task.state}. Native change lifecycle: {run.change ? run.change.state : 'No native change recorded'}.</p>{reconciling && <p className="table-meta">External outcome is unknown. Reconcile the native change before retrying.</p>}<h3>Stages and events</h3><p className="table-meta">{stream.status}</p>{stream.events.length ? <ul className="compact-list">{stream.events.map(event => <li key={event.id}>{new Date(event.occurred_at).toLocaleString()} · {event.type} · {event.aggregate_type}/{event.aggregate_id}</li>)}</ul> : <p>No events recorded for this run yet.</p>}<Evidence run={run} orgID={orgID} /></section>
}

function Evidence({ run, orgID }: { run: RepairRun; orgID: string }) {
  const report = run.report; const checks = [{ stage: 'H baseline', items: report?.baseline ?? [] }, { stage: 'H+patch', items: report?.candidate ?? [] }, { stage: 'T+patch', items: report?.target ?? [] }, { stage: 'native C', items: run.candidate_checks ?? [] }]; const artifacts = [...new Set([...(report?.artifacts ?? []), ...(run.candidate_artifacts ?? [])])]
  return <div className="stack"><h3>Frozen execution</h3><dl className="detail-list"><div><dt>Native head evidence</dt><dd><code>{run.context.native_head_sha || 'Unknown'}</code></dd></div><div><dt>Plan</dt><dd><code>{run.context.plan?.digest || report?.plan_digest || 'Unknown'}</code></dd></div><div><dt>Baseline</dt><dd><code>{run.context.plan?.baseline_sha || 'Unknown'}</code></dd></div><div><dt>Target</dt><dd><code>{run.context.plan?.target_sha || 'Unknown'}</code></dd></div><div><dt>Candidate head</dt><dd><code>{run.candidate_sha || 'Unknown'}</code></dd></div><div><dt>Frozen commands</dt><dd>{run.context.plan?.recipe?.commands?.length ? <ul className="compact-list">{run.context.plan.recipe.commands.map(command => <li key={command.id}><code>{command.id}: {command.args.join(" ")}</code> · {command.directory} · {command.timeout_seconds}s · {command.report_format}</li>)}</ul> : "Unknown"}</dd></div><div><dt>Policy</dt><dd><code>{run.context.policy_hash || 'Unknown'}</code></dd></div></dl><h3>Checks and report</h3><p className="table-meta">Recorded at {new Date(run.updated_at).toLocaleString()}. These results apply only to the pinned revisions above; current native gates require a fresh observation.</p>{checks.some(group => group.items.length) ? <ul className="compact-list">{checks.flatMap(group => group.items.map((check, index) => <li key={`${group.stage}-${check.command_id}-${index}`}><strong>{group.stage}</strong> · {check.command_id}: exit {check.exit_code}; {check.complete ? 'complete' : 'incomplete'}{check.reason ? ` — ${check.reason}` : ''}{check.cases && Object.keys(check.cases).length ? <ul>{Object.entries(check.cases).map(([name, outcome]) => <li key={name}>{name}: {outcome}</li>)}</ul> : null}{check.excerpt ? <pre className="command-block">{plainText(check.excerpt)}</pre> : null}</li>))}</ul> : <p>No check results recorded.</p>}{report?.reason && <p>{report.state}: {report.reason}</p>}{run.change && <p>Native change: {run.change.state}. {sourceURL(run.change.url) ? <a href={sourceURL(run.change.url)} target="_blank" rel="noreferrer">{run.change.title}</a> : run.change.title}</p>}{report?.diff ? <details><summary>Source diff</summary><pre className="command-block">{plainText(report.diff)}</pre></details> : report?.patches?.length ? <details><summary>Proposed file contents ({report.patches.length} files)</summary>{report.patches.map(patch => <pre className="command-block" key={patch.path}>{patch.path}{'\n'}{decodePatch(patch.content)}</pre>)}</details> : <p>No plaintext diff recorded.</p>}{artifacts.length ? <ul className="compact-list">{artifacts.map(id => <li key={id}><a href={runsAPI.artifactURL(orgID, id)} target="_blank" rel="noreferrer">Download authorized artifact {id}</a></li>)}</ul> : <p>No artifacts recorded.</p>}</div>
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
