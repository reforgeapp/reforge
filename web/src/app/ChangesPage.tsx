import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { Button } from '../components/Accessible'
import { EmptyTable, DataTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { MergeSettings } from './MergeSettings'
import { BotRevalidation } from './BotRevalidation'
import { useSession } from './query'
import { inventoryAPI } from '../api/inventory'
import { mergeAPI, type Change, type Gate, type Operation } from '../merge-api'

const methods = ['fast-forward-only', 'merge', 'squash', 'rebase', 'fast_forward']
const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const tone = (state: string) => ['completed', 'succeeded', 'eligible', 'satisfied'].includes(state) ? 'green' as const : ['blocked', 'failed', 'revoked', 'unknown'].includes(state) ? 'red' as const : 'amber' as const
const nativeURL = (value?: string) => { try { const url = new URL(value ?? ''); return /^https?:$/.test(url.protocol) && !url.username && !url.password ? url.toString() : undefined } catch { return undefined } }
const activeOperationStates = new Set(['requested', 'dispatching', 'queued', 'running', 'reconciling', 'unknown'])
const companionState = (state: string) => state === 'merged' ? 'merged' : state === 'merge_pending' ? 'merge pending' : 'repair pending'

export function ChangesPage({ orgID }: { orgID: string }) {
  const [showSettings, setShowSettings] = useState(false)
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const session = useSession()
  const [repositoryID, setRepositoryID] = useState(search.repository ?? '')
  const [changeID, setChangeID] = useState(search.change ?? '')
  const [repositoryQuery, setRepositoryQuery] = useState('')
  const repositories = useInfiniteQuery({
    queryKey: ['org', orgID, 'change-repositories', repositoryQuery],
    queryFn: ({ pageParam, signal }) => inventoryAPI.repositories(orgID, { cursor: pageParam, limit: 100, q: repositoryQuery || undefined, signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const repositoryPages = repositories.data?.pages ?? []
  const repositoryItems = repositoryPages.flatMap(page => page.items)
  const deepLinkedRepository = useQuery({
    queryKey: ['org', orgID, 'change-repository', search.repository],
    queryFn: ({ signal }) => inventoryAPI.repository(orgID, search.repository!, signal),
    enabled: !!search.repository && !repositoryItems.some(item => item.id === search.repository),
  })
  const repositoryOptions = deepLinkedRepository.data && !repositoryItems.some(item => item.id === deepLinkedRepository.data.id)
    ? [deepLinkedRepository.data, ...repositoryItems]
    : repositoryItems
  useEffect(() => {
    if (repositoryID && (repositoryItems.some(item => item.id === repositoryID) || deepLinkedRepository.isLoading)) return
    const next = repositoryOptions[0]?.id ?? ''
    if (next !== repositoryID) {
      setRepositoryID(next)
      setChangeID('')
      setShowSettings(false)
    }
  }, [repositoryID, repositoryItems, repositoryOptions, deepLinkedRepository.isLoading])
  const changes = useInfiniteQuery({
    queryKey: ['org', orgID, 'changes', repositoryID],
    queryFn: ({ pageParam, signal }) => inventoryAPI.changes(orgID, repositoryID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
    enabled: !!repositoryID,
  })
  const changeItems = changes.data?.pages.flatMap(page => page.items) ?? []
  const changePageLimitReached = (changes.data?.pages.length ?? 0) >= 20
  const selected = changeItems.find(item => item.id === changeID)
  useEffect(() => {
    if (!changeID && changeItems[0]) setChangeID(changeItems[0].id)
  }, [changeID, changeItems])
  useEffect(() => {
    if (changeID && !changeItems.some(item => item.id === changeID) && changes.hasNextPage && !changePageLimitReached && !changes.isFetchingNextPage && !changes.isFetchNextPageError) void changes.fetchNextPage()
  }, [changeID, changeItems, changePageLimitReached, changes.hasNextPage, changes.isFetchingNextPage, changes.isFetchNextPageError, changes.fetchNextPage])
  useEffect(() => {
    setRepositoryID(search.repository ?? '')
    setRepositoryQuery('')
    setChangeID(search.change ?? '')
    setShowSettings(false)
  }, [orgID, search.repository, search.change])
  if (repositories.isLoading && !repositoryQuery && !repositoryID) return <StatePanel kind="loading" title="Loading repositories" detail="Fetching repositories with observed changes." />
  if (repositories.error) return <StatePanel kind="error" title="Repositories unavailable" detail={text(repositories.error)} action={<Button onClick={() => repositories.refetch()}>Retry</Button>} />
  return <div className="stack">
    <div className="row-actions">
      <label>Search repositories<input aria-label="Search repositories" value={repositoryQuery} onChange={event => setRepositoryQuery(event.target.value)} placeholder="Search repositories" /></label>
      <label>Repository<select aria-label="Repository" value={repositoryID} onChange={event => { setRepositoryID(event.target.value); setChangeID(''); setShowSettings(false) }}><option value="">Choose repository</option>{repositoryOptions.map(repo => <option key={repo.id} value={repo.id}>{repo.name}</option>)}</select></label>
      {repositories.hasNextPage && <Button onClick={() => void repositories.fetchNextPage()} disabled={repositories.isFetchingNextPage}>{repositories.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}
    </div>
    <Button onClick={() => setShowSettings(value => !value)} disabled={!repositoryID}>{showSettings ? 'Close merge settings' : 'Merge settings'}</Button>
    {showSettings && repositoryID && <MergeSettings key={`${orgID}:${repositoryID}`} orgID={orgID} repositoryID={repositoryID} onSaved={() => { setShowSettings(false); void changes.refetch() }} />}
    {changes.error ? <p className="error-text" role="alert">Changes unavailable: {text(changes.error)} <Button onClick={() => void changes.refetch()}>Retry</Button></p> : null}
    {changeID && !selected && !changes.isFetching && <p role="status">Requested change is not in the loaded pages. {changes.hasNextPage ? 'Load more changes to continue searching.' : 'The requested change was not found.'}</p>}
    <DataTable caption="Native changes"><table><thead><tr><th>Change</th><th>Head</th><th>Target</th><th>State</th></tr></thead><tbody>{changeItems.map(item => <tr key={item.id}><td><button className="link-button" onClick={() => setChangeID(item.id)}>{item.title}</button><small className="table-meta">{item.author_login}</small></td><td><code>{item.head_sha.slice(0, 12)}</code></td><td><code>{item.target_sha.slice(0, 12)}</code></td><td><StatusBadge label={item.state} tone={tone(item.state)} /></td></tr>)}</tbody></table>{!changeItems.length && <EmptyTable label="No persisted changes observed." />}</DataTable>
    {changes.hasNextPage && <Button onClick={() => void changes.fetchNextPage()} disabled={changes.isFetchingNextPage}>{changes.isFetchingNextPage ? 'Loading…' : 'Load more changes'}</Button>}
    {selected && <ChangeDetail key={`${orgID}:${repositoryID}:${selected.id}:${selected.head_sha}:${selected.target_sha}`} orgID={orgID} repositoryID={repositoryID} change={selected} csrf={session.data?.csrf_token ?? ''} canWrite={session.data?.memberships.some(membership => membership.org_id === orgID && ['owner', 'admin', 'maintainer'].includes(membership.role)) ?? false} />}
  </div>
}

function ChangeDetail({ orgID, repositoryID, change, csrf, canWrite }: { orgID: string; repositoryID: string; change: Change; csrf: string; canWrite: boolean }) {
  const [method, setMethod] = useState(methods[0]); const [gate, setGate] = useState<Gate>(); const [uncertain, setUncertain] = useState(false); const initializedHistory = useRef(false); const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID()); const [now, setNow] = useState(Date.now()); const [operation, setOperation] = useState<Operation>(); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const history = useQuery({ queryKey: ['org', orgID, 'merge-operations', repositoryID, change.id], queryFn: ({ signal }) => mergeAPI.operations(orgID, repositoryID, change.id, signal) })
  useEffect(() => { if (!initializedHistory.current && history.data) { initializedHistory.current = true; if (!operation && history.data.items.length) setOperation(history.data.items[0]) } }, [history.data, operation])
  const preview = async () => { if (history.isLoading || history.error || uncertain || !canWrite) return; setBusy(true); setError(''); setOperation(undefined); try { const next = await mergeAPI.preview(orgID, repositoryID, change.id, method, csrf); setGate(next); setOperation(undefined); setUncertain(false); initializedHistory.current = true; setIdempotencyKey(crypto.randomUUID()); setNow(Date.now()) } catch (reason) { setGate(undefined); setError(text(reason)) } finally { setBusy(false) } }
  const request = async () => { if (!gate || gate.phase === 'queue_execution' || !canWrite || (expired && !uncertain) || blocked || (operation && activeOperationStates.has(operation.state))) return; setBusy(true); setError(''); try { const refreshed = await history.refetch(); if (refreshed.error) throw refreshed.error; const recovered = refreshed.data?.items.find(item => (item.requested_gate_id ?? item.gate_id) === gate.id); if (recovered) { setOperation(recovered); setUncertain(false); return }; setOperation(await mergeAPI.merge(orgID, gate.id, idempotencyKey, csrf)) } catch (reason) { const recovered = (await mergeAPI.operations(orgID, repositoryID, change.id).catch(() => undefined))?.items.find(item => (item.requested_gate_id ?? item.gate_id) === gate.id); if (recovered) { setOperation(recovered); setUncertain(false) } else { setUncertain(true); setError(text(reason)) } } finally { setBusy(false) } }
  useEffect(() => { if (!operation || !activeOperationStates.has(operation.state)) return; const timer = window.setInterval(() => { void mergeAPI.operation(orgID, operation.id).then(setOperation).catch(reason => setError(text(reason))) }, 1500); return () => window.clearInterval(timer) }, [orgID, operation?.id, operation?.state])
  useEffect(() => { if (!gate) return; const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer) }, [gate?.id, gate?.expires_at])
  const expired = gate ? Date.parse(gate.expires_at) <= now : false; const blocked = gate && gate.decision.outcome !== 'allow'; const queueAdmission = gate?.phase === 'queue_admission'; const queueExecution = gate?.phase === 'queue_execution'; const terminal = operation && !activeOperationStates.has(operation.state)
  return <section className="state-card merge-review" aria-label="Merge review"><h2>{change.title}</h2><p>{nativeURL(change.url) ? <a href={nativeURL(change.url)} target="_blank" rel="noreferrer">Open native change</a> : 'Native change link unavailable'} · {change.head_branch} → {change.target_branch}</p>{gate?.companions?.length ? <div className="detail-section" aria-label="Companion merge order"><h3>Companion merge order</h3><ol>{gate.companions.map(companion => <li key={companion.task_id}><code>{companion.task_id}</code> · {companion.change_id} · {companionState(companion.state)}</li>)}<li>Original update · requires fresh native gate validation after companion merges.</li></ol></div> : null}<BotRevalidation orgID={orgID} repositoryID={repositoryID} changeID={change.id} /><label>Merge method<select disabled={busy || uncertain || (!!operation && !['blocked', 'cancelled', 'failed'].includes(operation.state))} value={method} onChange={event => { setMethod(event.target.value); setGate(undefined) }}>{methods.map(item => <option key={item} value={item}>{item}</option>)}</select></label><Button className="button button-primary" disabled={history.isLoading || !!history.error || busy || uncertain || !canWrite || (!!operation && !['blocked', 'cancelled', 'failed'].includes(operation.state))} onClick={() => void preview()}>{busy ? 'Checking…' : 'Preview merge gate'}</Button>{history.error && <p role="alert">Operation history unavailable: {text(history.error)} <Button onClick={() => void history.refetch()}>Reload operations</Button></p>}{error && <p className="error-text" role="alert">{error}</p>}{uncertain && <p role="status">Outcome unknown. Retry uses the same request identity.</p>}{gate && <div className="stack"><p><StatusBadge label={gate.decision.outcome} tone={tone(gate.decision.outcome)} /> {expired ? 'Preview expired; run a new preview.' : 'Preview is valid for one minute.'}</p><dl className="detail-list"><div><dt>H head</dt><dd><code>{gate.binding.head}</code></dd></div><div><dt>T target</dt><dd><code>{gate.binding.target}</code></dd></div><div><dt>Checked revision</dt><dd><code>{gate.binding.tested}</code></dd></div>{gate.snapshot.queue && <div><dt>Queue</dt><dd>{gate.snapshot.queue.state} · H <code>{gate.snapshot.queue.head_sha}</code> · T <code>{gate.snapshot.queue.target_sha}</code> · C <code>{gate.snapshot.queue.tested_sha || 'Pending'}</code></dd></div>}<div><dt>Native authority</dt><dd>{gate.snapshot.native.state} · H <code>{gate.snapshot.native.head_sha}</code> · T <code>{gate.snapshot.native.target_sha}</code></dd></div><div><dt>Native rules</dt><dd>{gate.snapshot.rules.state} · {gate.snapshot.rules.reason || ''}</dd></div><div><dt>Checks</dt><dd>{(gate.snapshot.checks ?? []).length ? (gate.snapshot.checks ?? []).map(check => `${check.name}: ${check.conclusion}`).join(', ') : 'No checks recorded'}</dd></div><div><dt>Approvals</dt><dd>{(gate.snapshot.approvals ?? []).length}</dd></div>{gate.snapshot.execution_check && <div><dt>Execution check</dt><dd>{gate.snapshot.execution_check.name} · {gate.snapshot.execution_check.publisher_id}</dd></div>}</dl>{blocked && <p className="error-text">Blocked: {[...(gate.decision.blockers ?? []), ...(gate.decision.required_actions ?? [])].join('; ') || 'The server did not authorize this merge.'}</p>}{queueExecution && <p role="status">Queue execution is controlled by the existing merge operation. No new merge operation can be created.</p>}<Button disabled={busy || !csrf || !canWrite || (expired && !uncertain) || !!blocked || !!operation || queueExecution} onClick={() => void request()}>{queueExecution ? 'Queue execution handled by existing operation' : queueAdmission ? 'Request queue admission' : 'Request protected merge'}</Button></div>}{operation && <OperationView orgID={orgID} operation={operation} csrf={csrf} canWrite={canWrite} onChange={setOperation} terminal={!!terminal} />}</section>
}

function OperationView({ orgID, operation, csrf, canWrite, onChange, terminal }: { orgID: string; operation: Operation; csrf: string; canWrite: boolean; onChange: (value: Operation) => void; terminal: boolean }) { const [error, setError] = useState(''); const action = async (kind: 'cancel' | 'reconcile') => { try { onChange(kind === 'cancel' ? await mergeAPI.cancel(orgID, operation.id, operation.version, csrf) : await mergeAPI.reconcile(orgID, operation.id, operation.version, csrf)) } catch (reason) { setError(text(reason)) } }; return <div className="detail-section"><p><StatusBadge label={operation.state} tone={tone(operation.state)} /> {operation.reason || 'Native operation state is authoritative.'}</p>{['unknown', 'reconciling'].includes(operation.state) && <p className="error-text">Native outcome is unknown. Reconcile before retrying.</p>}{!terminal && <div className="row-actions"><Button disabled={!csrf || !canWrite} onClick={() => void action('cancel')}>Cancel request</Button><Button disabled={!csrf || !canWrite || !['requested', 'dispatching', 'queued', 'running', 'reconciling', 'unknown'].includes(operation.state)} onClick={() => void action('reconcile')}>Reconcile outcome</Button></div>}{error && <p className="error-text" role="alert">{error}</p>}</div> }
