import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { inventoryAPI } from '../api/inventory'
import { Button, Dialog } from '../components/Accessible'
import { StatusBadge } from '../components/Status'
import type { components } from '../api/schema'

type Job = components['schemas']['InventoryJob']
type Candidate = components['schemas']['ForgeRepository']

const tone = (value: string) => value === 'active' || value === 'complete' || value === 'fresh' ? 'green' : value === 'failed' || value === 'missing' ? 'red' : value === 'paused' || value === 'stale' || value === 'running' ? 'amber' : 'neutral'
const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export type ImportConnection = { id: string; name: string; provider: string }

type RepositoryImportDialogProps = {
  open: boolean
  orgID: string
  csrf: string
  teams: Array<{ id: string; name: string }>
  connection?: ImportConnection
  autoStart?: boolean
  candidateFilter?: string
  repositoriesLink?: string
  jobID?: string
  onJobChange?: (jobID: string | undefined) => void
  onClose: () => void
  onDone: () => void
}

function normaliseFilter(value: string) {
  const raw = value.trim().toLowerCase()
  if (!raw) return ''
  try {
    const url = new URL(raw.includes('://') ? raw : `https://${raw}`)
    const segments = url.pathname.split('/').filter(Boolean)
    if (segments.length >= 2) return `${segments[0]}/${segments[1]}`.replace(/\.git$/, '')
  } catch {}
  const parts = raw.replace(/^https?:\/\//, '').replace(/\.git$/, '').split('/').filter(Boolean)
  return parts.length >= 2 ? `${parts[parts.length - 2]}/${parts[parts.length - 1]}` : raw
}

export function RepositoryImportDialog({ open, orgID, csrf, teams, connection, autoStart, candidateFilter, repositoriesLink, jobID, onJobChange, onClose, onDone }: RepositoryImportDialogProps) {
  const fixedConnection = !!connection
  const forgeConnections = useQuery({ queryKey: ['org', orgID, 'connections', 'forge-picker'], queryFn: ({ signal }) => inventoryAPI.connections(orgID, signal), enabled: open && !fixedConnection, staleTime: 0, refetchOnMount: 'always' })
  const connections = (forgeConnections.data?.items ?? []).filter(item => item.state === 'healthy')
  const [connectionID, setConnectionID] = useState('')
  const [job, setJob] = useState<Job>()
  const [candidates, setCandidates] = useState<Candidate[]>([])
  const [candidateCursor, setCandidateCursor] = useState<string>()
  const [candidateComplete, setCandidateComplete] = useState(true)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [teamID, setTeamID] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const completedImport = useRef<string | undefined>(undefined)
  const startedFor = useRef<string | undefined>(undefined)
  const lastSynced = useRef<string | undefined>(undefined)
  const selectedConnectionAvailable = fixedConnection ? connection?.id === connectionID : connections.some(item => item.id === connectionID)
  const candidateController = useRef<AbortController | undefined>(undefined)
  const orgRef = useRef(orgID)
  const openRef = useRef(open)
  const gen = useRef(0)
  const filter = normaliseFilter(candidateFilter ?? '')
  const matchFilter = (candidate: Candidate) => !filter || candidate.full_name.toLowerCase().includes(filter)
  const visibleCandidates = filter ? candidates.filter(matchFilter) : candidates
  const importableIDs = visibleCandidates.filter(candidate => selected.has(candidate.native_id)).map(candidate => candidate.native_id)
  const current = (myGen: number, myOrg: string) => openRef.current && orgRef.current === myOrg && gen.current === myGen

  useEffect(() => {
    openRef.current = open
    if (open) return
    gen.current++
    candidateController.current?.abort()
    startedFor.current = undefined
    setConnectionID(connection?.id ?? '')
    setJob(undefined)
    setCandidates([])
    setCandidateCursor(undefined)
    setCandidateComplete(true)
    setSelected(new Set())
    setError('')
    setBusy(false)
  }, [open, connection?.id])
  useEffect(() => {
    if (orgRef.current === orgID) return
    orgRef.current = orgID
    gen.current++
    startedFor.current = undefined
    setJob(undefined)
    setCandidates([])
    setCandidateCursor(undefined)
    setCandidateComplete(true)
    setSelected(new Set())
    setError('')
    setBusy(false)
  }, [orgID])
  useEffect(() => { if (connection) setConnectionID(connection.id) }, [connection?.id])
  useEffect(() => { if (!fixedConnection && forgeConnections.data && !forgeConnections.isFetching && !selectedConnectionAvailable) setConnectionID('') }, [fixedConnection, forgeConnections.data, forgeConnections.isFetching, selectedConnectionAvailable])
  useEffect(() => () => candidateController.current?.abort(), [])
  useEffect(() => {
    if (!open || !jobID || job) return
    const myOrg = orgID
    const controller = new AbortController()
    void inventoryAPI.job(orgID, jobID, controller.signal).then(restored => {
      if (controller.signal.aborted || orgRef.current !== myOrg) return
      setJob(restored)
      setConnectionID(restored.connection_id)
    }).catch(() => {})
    return () => controller.abort()
  }, [open, jobID, job?.id, orgID])
  useEffect(() => {
    const next = job && ['queued', 'running'].includes(job.state) ? job.id : undefined
    if (lastSynced.current === next) return
    lastSynced.current = next
    onJobChange?.(next)
  }, [job?.id, job?.state])
  useEffect(() => { if (!job || !['queued', 'running'].includes(job.state)) return; const myOrg = orgID; const controller = new AbortController(); const timer = window.setInterval(() => { void inventoryAPI.job(orgID, job.id, controller.signal).then(next => { if (!controller.signal.aborted && orgRef.current === myOrg) setJob(next) }).catch(reason => { if (!controller.signal.aborted && orgRef.current === myOrg) setError(text(reason)) }) }, 1200); return () => { controller.abort(); window.clearInterval(timer) } }, [job?.id, job?.state, orgID])
  useEffect(() => { if (job?.kind !== 'scan' || job.state !== 'complete') return; const myOrg = orgID; const controller = new AbortController(); void inventoryAPI.candidates(orgID, job.id, undefined, controller.signal).then(page => { if (!controller.signal.aborted && orgRef.current === myOrg) { setCandidates(page.items); setCandidateCursor(page.next_cursor); setCandidateComplete(page.complete); setSelected(new Set(page.items.filter(matchFilter).map(item => item.native_id))) } }).catch(reason => { if (!controller.signal.aborted && orgRef.current === myOrg) setError(text(reason)) }); return () => controller.abort() }, [job?.id, job?.kind, job?.state, orgID])
  useEffect(() => { if (job?.kind === 'import' && job.state === 'complete' && completedImport.current !== job.id) { completedImport.current = job.id; onDone() } }, [job?.id, job?.kind, job?.state, onDone])
  const loadCandidates = async () => { if (!job || !candidateCursor) return; const myGen = gen.current; const myOrg = orgID; candidateController.current?.abort(); const controller = new AbortController(); candidateController.current = controller; setBusy(true); try { const page = await inventoryAPI.candidates(orgID, job.id, candidateCursor, controller.signal); if (!controller.signal.aborted && current(myGen, myOrg)) { setCandidates(previous => [...previous, ...page.items]); setCandidateCursor(page.next_cursor); setCandidateComplete(page.complete); setSelected(previous => new Set([...previous, ...page.items.filter(matchFilter).map(item => item.native_id)])) } } catch (reason) { if (!controller.signal.aborted && current(myGen, myOrg)) setError(text(reason)) } finally { if (!controller.signal.aborted && current(myGen, myOrg)) setBusy(false) } }
  const start = async () => { const myGen = gen.current; const myOrg = orgID; setBusy(true); setError(''); try { const next = await inventoryAPI.startSync(orgID, connectionID, '', csrf); if (!current(myGen, myOrg)) return; setJob(next) } catch (reason) { if (current(myGen, myOrg)) setError(text(reason)) } finally { if (current(myGen, myOrg)) setBusy(false) } }
  const cancel = async () => { if (!job) return; const myGen = gen.current; const myOrg = orgID; setBusy(true); try { const next = await inventoryAPI.cancel(orgID, job.id, job.version, csrf); if (!current(myGen, myOrg)) return; setJob(next) } catch (reason) { if (current(myGen, myOrg)) setError(text(reason)) } finally { if (current(myGen, myOrg)) setBusy(false) } }
  const importSelected = async (all: boolean) => { if (!job) return; const myGen = gen.current; const myOrg = orgID; setBusy(true); setError(''); try { const next = await inventoryAPI.import(orgID, job.id, job.version, { ...(all ? { all: true } : { native_ids: [...importableIDs] }), ...(teamID ? { team_ids: [teamID] } : {}) }, csrf); if (!current(myGen, myOrg)) return; setJob(next) } catch (reason) { if (current(myGen, myOrg)) setError(text(reason)) } finally { if (current(myGen, myOrg)) setBusy(false) } }

  useEffect(() => {
    if (!open || !autoStart || jobID || !connectionID || !selectedConnectionAvailable || job) return
    if (startedFor.current === connectionID) return
    startedFor.current = connectionID
    void start()
  }, [open, autoStart, jobID, connectionID, selectedConnectionAvailable, job, csrf])

  const picker = fixedConnection
    ? <p className="table-meta" role="status">{connection?.name} · {connection?.provider}</p>
    : <><label>Forge connection<select required value={connectionID} disabled={forgeConnections.isFetching || !!forgeConnections.error} onChange={event => setConnectionID(event.target.value)}><option value="">{forgeConnections.isFetching ? 'Loading connections…' : 'Choose connection'}</option>{!forgeConnections.isFetching && !forgeConnections.error && connections.map(item => <option key={item.id} value={item.id}>{item.name} · {item.provider}</option>)}</select></label>{forgeConnections.error ? <p className="error-text" role="alert">Forge connections unavailable: {text(forgeConnections.error)} <Button type="button" disabled={forgeConnections.isFetching} onClick={() => void forgeConnections.refetch()}>Retry</Button></p> : !forgeConnections.isFetching && connections.length === 0 ? <p role="status">No healthy forge connections available.</p> : null}</>

  return <Dialog open={open} title="Sync forge inventory" onClose={onClose}><div className="form-stack">{!job && <>{picker}<Button className="button button-primary" disabled={!connectionID || !selectedConnectionAvailable || !csrf || busy || (!fixedConnection && (forgeConnections.isFetching || !!forgeConnections.error))} onClick={start}>{busy ? 'Starting…' : 'Start preview'}</Button></>}{job && <><p><StatusBadge label={job.kind === 'import' ? `import ${job.state}` : job.state} tone={tone(job.state)} /> <span className="table-meta">{job.kind === 'import' ? `${job.processed} imported` : `${job.processed} repositories found`}</span></p>{job.reason && <p className="error-text" role="alert">{job.reason}</p>}{['queued', 'running'].includes(job.state) && <div className="row-actions"><Button disabled={busy} onClick={cancel}>Cancel sync</Button><span role="status" className="table-meta">{job.kind === 'import' ? 'Importing repositories…' : 'Discovering repositories…'}</span></div>}{(job.state === 'failed' || job.state === 'cancelled') && <div className="row-actions"><Button onClick={() => { setJob(undefined); setError('') }}>Retry</Button><Button onClick={onClose}>Close</Button></div>}{job.kind === 'scan' && job.state === 'complete' && <><fieldset><legend>Preview candidates</legend><label className="checkbox-label"><input type="checkbox" checked={visibleCandidates.length > 0 && visibleCandidates.every(item => selected.has(item.native_id))} onChange={event => setSelected(previous => { const next = new Set(previous); visibleCandidates.forEach(item => event.target.checked ? next.add(item.native_id) : next.delete(item.native_id)); return next })} /> Select all loaded ({visibleCandidates.length})</label><div className="candidate-list">{visibleCandidates.map(candidate => <label className="checkbox-label" key={candidate.native_id}><input type="checkbox" checked={selected.has(candidate.native_id)} onChange={event => setSelected(previous => { const next = new Set(previous); event.target.checked ? next.add(candidate.native_id) : next.delete(candidate.native_id); return next })} />{candidate.full_name}{candidate.archived ? ' · archived' : ''}</label>)}</div>{filter && !visibleCandidates.length && <p className="table-meta">No loaded candidates match the repository filter.</p>}{!candidateComplete && <Button disabled={busy} onClick={loadCandidates}>{busy ? 'Loading…' : 'Load more candidates'}</Button>}</fieldset><label>Assign team<select value={teamID} onChange={event => setTeamID(event.target.value)}><option value="">No team assignment</option>{teams.map(team => <option key={team.id} value={team.id}>{team.name}</option>)}</select></label><div className="dialog-actions"><Button onClick={onClose}>Close</Button><Button disabled={!importableIDs.length || busy || !csrf} onClick={() => importSelected(false)}>Import selected ({importableIDs.length})</Button>{!filter && <Button className="button button-primary" disabled={busy || !csrf} onClick={() => importSelected(true)}>Import all</Button>}</div></>}{job.kind === 'import' && job.state === 'complete' && <><p role="status">Import complete. Repository inventory will refresh.</p>{repositoriesLink && <a className="button" href={repositoriesLink}>View repositories</a>}</>}{job.state === 'stale' && <div className="row-actions"><Button onClick={() => { setJob(undefined); setError('') }}>Start new sync</Button><Button onClick={onClose}>Close</Button></div>}</>}{error && <p className="error-text" role="alert">{error}</p>}</div></Dialog>
}
