import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '../components/Accessible'
import { DataTable } from '../components/DataTable'
import { inventoryAPI } from '../api/inventory'
import { discoveryAPI, type Finding } from '../api/discovery'
import { policyAPI, type Input, type Simulation, type Version } from '../policy-api'
import type { components } from '../api/schema'

type Repository = components['schemas']['Repository']
type Change = components['schemas']['ForgeChange']
type Scope = { kind: string; id: string }
type Props = { orgID: string; scope: Scope; candidate?: Version; input: Input; csrf: string; disabled?: boolean; disabledReason?: string; primaryTeamID?: string; onActionChange?: (action: Input['action']) => void }
type Source = { kind: 'finding' | 'change'; id: string; title: string; head: string; target: string; state?: string; blockers: string[]; detail?: string }
type Row = { repository: Repository; sources: Source[]; complete: boolean; cursor?: string; loading?: boolean; error?: string }
type Result = { key: string; source?: Source; simulation?: Simulation; gateChanges?: string; outcome?: string; blockers: string[]; error?: string }
type Page<T> = { items: T[]; complete: boolean; next_cursor?: string; snapshot_state?: string }

const errorText = (value: unknown) => value instanceof Error ? value.message : 'Impact preview failed.'
const sourceKind = (input: Input): Source['kind'] => input.action === 'repair' ? 'finding' : 'change'
const unsupported = (action: Input['action']) => !['repair', 'publish', 'merge'].includes(action)
const same = (left: unknown, right: unknown) => JSON.stringify(left) === JSON.stringify(right)
const display = (value: unknown, label: string) => value === null ? label.startsWith('allow.') ? 'inherit / unrestricted' : 'none' : value === undefined ? label.startsWith('limits.') ? 'unlimited' : 'unset' : Array.isArray(value) ? value.length > 4 ? `${value.slice(0, 4).join(', ')} (+${value.length - 4} more)` : value.join(', ') || 'none' : typeof value === 'object' ? Object.entries(value as Record<string, unknown>).map(([key, item]) => `${key}=${String(item)}`).join(', ') || 'none' : String(value)
const delta = (label: string, before: unknown, after: unknown) => same(before, after) ? '' : `${label}: ${display(before, label)} → ${display(after, label)}`

function gateChanges(before: Version['policy'], after: Simulation['resolved']['policy']) {
  const allowKeys = ['recipes', 'models', 'routes', 'merge_methods', 'environments', 'workflows'] as const
  const limitKeys = ['budget', 'concurrency', 'attempts', 'changed_files', 'changed_lines', 'open_changes'] as const
  const changes = allowKeys.map(key => delta(`allow.${key}`, before.allow?.[key], after.allow?.[key])).concat(delta('deny', before.deny, after.deny), limitKeys.map(key => delta(`limits.${key}`, before.limits?.[key], after.limits?.[key]))).flat().filter(Boolean)
  return changes.length ? changes.join('; ') : 'No allow, deny, or limit changes.'
}

function sourceFromFinding(finding: Finding): Source {
  const evidence = finding.evidence ?? ({} as Finding['evidence'])
  const blockers = [...(evidence.blockers ?? [])]
  const lastSeen = Date.parse(finding.last_seen)
  if (!evidence.complete) blockers.push('Stored finding evidence is incomplete.')
  if (!Number.isFinite(lastSeen) || lastSeen <= Date.now() - 15 * 60 * 1000) blockers.push('Stored finding evidence is stale.')
  return { kind: 'finding', id: finding.id, title: finding.title, head: evidence.head_sha ?? '', target: evidence.target_sha ?? '', state: finding.state, blockers }
}

function sourceFromChange(change: Change, snapshotState?: string): Source {
  const blockers = snapshotState ? [`Snapshot state: ${snapshotState}.`] : ['Snapshot state unavailable.']
  if (!change.head_sha || !change.target_sha) blockers.push('Stored change lacks head or target revision.')
  return { kind: 'change', id: change.id, title: change.title, head: change.head_sha ?? '', target: change.target_sha ?? '', state: change.state, blockers, detail: snapshotState ? `Snapshot ${snapshotState}` : 'Snapshot unknown' }
}

export function PolicyImpactPreview({ orgID, scope, candidate, input, csrf, primaryTeamID = '', disabled = false, disabledReason = '', onActionChange }: Props) {
  const [repositories, setRepositories] = useState<Repository[]>([])
  const [repoCursor, setRepoCursor] = useState<string>()
  const [repoComplete, setRepoComplete] = useState(false)
  const [repoLoading, setRepoLoading] = useState(false)
  const [repoError, setRepoError] = useState('')
  const [rows, setRows] = useState<Row[]>([])
  const [results, setResults] = useState<Result[]>([])
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState('')
  const generation = useRef(0)
  const controllerRef = useRef<AbortController | null>(null)
  const repoCursors = useRef(new Set<string>())
  const sourceCursors = useRef(new Map<string, Set<string>>())
  const inputKey = useMemo(() => JSON.stringify(input), [input])
  const kind = sourceKind(input)
  const supported = !unsupported(input.action)

  useEffect(() => {
    generation.current += 1
    controllerRef.current?.abort()
    controllerRef.current = new AbortController()
    repoCursors.current = new Set()
    sourceCursors.current = new Map()
    setRepositories([]); setRepoCursor(undefined); setRepoComplete(false); setRepoError(''); setRows([]); setResults([]); setRunError(''); setRunning(false); setRepoLoading(false)
    return () => { generation.current += 1; controllerRef.current?.abort() }
  }, [orgID, scope.kind, scope.id, candidate?.id, candidate?.hash, inputKey, primaryTeamID, disabled, csrf])

  const active = (controller: AbortController, current: number) => current === generation.current && !controller.signal.aborted
  const fetchRepositories = async (cursor: string | undefined, controller: AbortController) => scope.kind === 'repository' ? { items: [await inventoryAPI.repository(orgID, scope.id, controller.signal)], complete: true } as Page<Repository> : await inventoryAPI.repositories(orgID, { limit: 25, cursor, ...(scope.kind === 'team' ? { team_id: scope.id } : {}), signal: controller.signal })
  const fetchSources = async (repository: Repository, cursor: string | undefined, controller: AbortController): Promise<Page<Finding> | Page<Change>> => kind === 'finding' ? discoveryAPI.findings(orgID, { repository_id: repository.id, limit: 50, cursor, signal: controller.signal }) : inventoryAPI.changes(orgID, repository.id, { limit: 50, cursor, signal: controller.signal })
  const makeSources = (page: Page<Finding> | Page<Change>): Source[] => kind === 'finding' ? (page as Page<Finding>).items.map(sourceFromFinding) : (page as Page<Change>).items.map(change => sourceFromChange(change, (page as Page<Change>).snapshot_state))
  const setRow = (repositoryID: string, update: (row: Row) => Row) => setRows(previous => previous.map(row => row.repository.id === repositoryID ? update(row) : row))

  const run = () => {
    if (!candidate || !csrf || disabled || running || !supported) return
    const controller = controllerRef.current
    if (!controller) return
    const current = generation.current
    setRunning(true); setRunError(''); setResults([])
    void (async () => {
      const next: Result[] = []
      try {
        let repoList = repositories
        let localRows = rows
        if (!repoList.length) {
          const page = await fetchRepositories(undefined, controller)
          if (!active(controller, current)) return
          if (!page.complete && !page.next_cursor) setRepoError('Repository pagination stopped before completion.')
          repoList = page.items; localRows = repoList.map(repository => ({ repository, sources: [], complete: false }))
          setRepositories(repoList); setRepoCursor(page.next_cursor); setRepoComplete(page.complete); setRows(localRows)
        }
        for (const repository of repoList) {
          if (!active(controller, current)) return
          let row = localRows.find(item => item.repository.id === repository.id) ?? { repository, sources: [], complete: false }
          if (!row.complete && !row.error && !row.sources.length) {
            try {
              const page = await fetchSources(repository, undefined, controller)
              if (!active(controller, current)) return
              const sources = makeSources(page)
              row = { ...row, sources, cursor: page.next_cursor, complete: page.complete, error: !page.complete && !page.next_cursor ? 'Evidence pagination stopped before completion.' : undefined }
              localRows = localRows.map(item => item.repository.id === repository.id ? row : item)
              setRows(localRows)
            } catch (reason) { if (!active(controller, current)) return; row = { ...row, error: errorText(reason) }; localRows = localRows.map(item => item.repository.id === repository.id ? row : item); setRow(repository.id, () => row); continue }
          }
          let effective
          try { effective = await policyAPI.effective(orgID, repository.id, controller.signal) } catch (reason) { if (!active(controller, current)) return; const failure = errorText(reason); row = { ...row, error: failure }; localRows = localRows.map(item => item.repository.id === repository.id ? row : item); setRow(repository.id, () => row); for (const source of row.sources) next.push({ key: `${repository.id}:${source.id}`, source, blockers: [], error: failure }); setResults([...next]); continue }
          if (!active(controller, current)) return
          if (!row.sources.length) {
            const blockers = ['No stored evidence is available for this repository.']
            const candidateInput: Input = { ...input, current: { head: '', target: '', tested: '', policy_hash: effective.hash, provider_rules: '', capability_version: '', source_sha: '', artifact: '' }, starting_policy_hash: effective.hash, evidence: [], paused_scopes: [] }
            try {
              const simulation = await policyAPI.simulate(orgID, candidate.id, repository.id, scope.kind === 'repository' ? primaryTeamID : '', candidateInput, csrf)
              if (!active(controller, current)) return
              const allBlockers = [...new Set([...blockers, ...(simulation.resolved.problems ?? []), ...(simulation.decision.blockers ?? [])])]
              next.push({ key: `${repository.id}:`, simulation, gateChanges: gateChanges(effective.policy, simulation.resolved.policy), outcome: simulation.decision.outcome === 'allow' && allBlockers.length ? 'unknown' : simulation.decision.outcome, blockers: allBlockers })
            } catch (reason) {
              if (!active(controller, current)) return
              next.push({ key: `${repository.id}:`, blockers, error: errorText(reason) })
            }
            setResults([...next])
            continue
          }
          for (const source of row.sources) {
            if (!active(controller, current)) return
            const blockers = [...source.blockers]
            if (!source.head || !source.target) blockers.push('Stored source binding is incomplete.')
            const candidateInput: Input = { ...input, current: { head: source.head, target: source.target, tested: '', policy_hash: effective.hash, provider_rules: '', capability_version: '', source_sha: '', artifact: '' }, starting_policy_hash: effective.hash, evidence: [], paused_scopes: [] }
            try {
              const simulation = await policyAPI.simulate(orgID, candidate.id, repository.id, scope.kind === 'repository' ? primaryTeamID : '', candidateInput, csrf)
              if (!active(controller, current)) return
              if (simulation.resolved.hash !== effective.hash) blockers.push('Stored evidence belongs to the current policy, not this candidate.')
              const allBlockers = [...new Set([...blockers, ...(simulation.resolved.problems ?? []), ...(simulation.decision.blockers ?? [])])]
              next.push({ key: `${repository.id}:${source.id}`, source, simulation, gateChanges: gateChanges(effective.policy, simulation.resolved.policy), outcome: simulation.decision.outcome === 'allow' && allBlockers.length ? 'unknown' : simulation.decision.outcome, blockers: allBlockers })
            } catch (reason) {
              if (!active(controller, current)) return
              const failure = errorText(reason); row = { ...row, error: failure }; localRows = localRows.map(item => item.repository.id === repository.id ? row : item); next.push({ key: `${repository.id}:${source.id}`, source, blockers, error: failure }); setRow(repository.id, () => row)
            }
            setResults([...next])
          }
        }
      } catch (reason) { if (active(controller, current)) setRunError(errorText(reason)) } finally { if (active(controller, current)) setRunning(false) }
    })()
  }

  const loadMoreRepositories = () => {
    if (running || repoLoading || repoComplete || scope.kind === 'repository') return
    const cursor = repoCursor
    const controller = controllerRef.current
    if (!cursor || !controller) { setRepoError('Repository pagination stopped before completion.'); return }
    if (repoCursors.current.has(cursor)) { setRepoCursor(undefined); setRepoError('Repository pagination cursor repeated before completion.'); return }
    repoCursors.current.add(cursor); setRepoLoading(true)
    const current = generation.current
    void (async () => {
      try {
        const page = await fetchRepositories(cursor, controller)
        if (!active(controller, current)) return
        if (!page.complete && !page.next_cursor) setRepoError('Repository pagination stopped before completion.')
        setRepositories(previous => [...previous, ...page.items.filter(item => !previous.some(existing => existing.id === item.id))]); setRepoCursor(page.next_cursor); setRepoComplete(page.complete)
        for (const repository of page.items) {
          if (!active(controller, current)) return
          try {
            const sourcePage = await fetchSources(repository, undefined, controller)
            if (!active(controller, current)) return
            setRows(previous => [...previous.filter(row => row.repository.id !== repository.id), { repository, sources: makeSources(sourcePage), cursor: sourcePage.next_cursor, complete: sourcePage.complete, error: !sourcePage.complete && !sourcePage.next_cursor ? 'Evidence pagination stopped before completion.' : undefined }])
          } catch (reason) { if (active(controller, current)) setRows(previous => [...previous.filter(row => row.repository.id !== repository.id), { repository, sources: [], complete: false, error: errorText(reason) }]) }
        }
      } catch (reason) { if (active(controller, current)) setRepoError(errorText(reason)) } finally { if (active(controller, current)) setRepoLoading(false) }
    })()
  }

  const loadMoreEvidence = (row: Row) => {
    if (running || row.loading || row.complete || !row.cursor) return
    const controller = controllerRef.current
    if (!controller) return
    const seen = sourceCursors.current.get(row.repository.id) ?? new Set<string>()
    if (seen.has(row.cursor)) { setRow(row.repository.id, value => ({ ...value, cursor: undefined, error: 'Evidence pagination cursor repeated before completion.' })); return }
    seen.add(row.cursor); sourceCursors.current.set(row.repository.id, seen); setRow(row.repository.id, value => ({ ...value, loading: true }))
    const cursor = row.cursor; const current = generation.current
    void (async () => {
      try {
        const page = await fetchSources(row.repository, cursor, controller)
        if (!active(controller, current)) return
        setRow(row.repository.id, value => { const added = makeSources(page).filter(source => !value.sources.some(existing => existing.kind === source.kind && existing.id === source.id)); return { ...value, sources: [...value.sources, ...added], cursor: page.next_cursor, complete: page.complete, loading: false, error: !page.complete && !page.next_cursor ? 'Evidence pagination stopped before completion.' : undefined } })
      } catch (reason) { if (active(controller, current)) setRow(row.repository.id, value => ({ ...value, loading: false, error: errorText(reason) })) }
    })()
  }

  const coverageComplete = repoComplete && !repoError && !runError && !results.some(result => result.error) && rows.length === repositories.length && rows.every(row => row.complete && !row.error)
  const busy = running || repoLoading || rows.some(row => row.loading)
  const resultFor = (repositoryID: string, sourceID?: string) => results.find(result => result.key === (sourceID === undefined ? repositoryID : `${repositoryID}:${sourceID}`))
  const hasRun = running || repoComplete || repositories.length > 0 || rows.length > 0 || results.length > 0 || !!repoError || !!runError
  const renderEmptyRow = (row: Row) => { const result = resultFor(row.repository.id, ''); return <tr key={row.repository.id}><td>{row.repository.name}</td><td>{row.loading ? `Loading stored ${kind}s…` : row.error ? <span className="error-text">{row.error}</span> : 'No stored findings or changes.'}</td><td>{result?.gateChanges ?? 'Not simulated'}</td><td>{result?.error ? <span className="error-text">{result.error}</span> : result?.simulation ? <><strong>{result.outcome}</strong>{result.blockers.length > 0 && <><br /><details><summary>{result.blockers.length} blockers</summary><small>{result.blockers.join('; ')}</small></details></>}</> : 'Not simulated'}</td></tr> }
  return <section className="policy-impact-preview policy-simulation" aria-label="Policy impact preview">
    <div className="subsection-actions"><strong>Impact preview</strong><label>Preview action<select value={input.action} disabled={disabled || busy || !onActionChange} onChange={event => onActionChange?.(event.target.value as Input['action'])}><option value="repair">Repair</option><option value="publish">Publish</option><option value="merge">Merge</option></select></label><Button className="button button-primary" disabled={disabled || !candidate || !csrf || busy || !supported} onClick={run}>{running ? 'Simulating…' : 'Run impact preview'}</Button></div>
    {!candidate && <p className="table-note">Choose a candidate policy version before running preview.</p>}
    {disabled && disabledReason && <p className="table-note">{disabledReason}</p>}
    {!supported && <p className="table-note">No stored evidence binding is available for {input.action}.</p>}
    {repoError && <p className="error-text" role="alert">Repository coverage unavailable: {repoError}</p>}
    {hasRun && <><p className="table-meta">Coverage: {repositories.length} repositories, {coverageComplete ? 'complete' : 'partial'}.</p><DataTable caption="Policy impact evidence"><table><thead><tr><th>Repository</th><th>Stored work</th><th>Gate changes</th><th>Decision</th></tr></thead><tbody>{rows.length ? rows.flatMap(row => row.sources.length ? row.sources.map(source => { const result = resultFor(row.repository.id, source.id); return <tr key={`${row.repository.id}:${source.id}`}><td>{row.repository.name}</td><td>{source.title}<br /><small>{source.kind} · {source.state ?? 'stored'}{source.detail ? ` · ${source.detail}` : ''}</small></td><td>{result?.gateChanges ?? 'Not simulated'}</td><td>{result?.error ? <span className="error-text">{result.error}</span> : result?.simulation ? <><strong>{result.outcome}</strong>{result.blockers.length > 0 && <><br /><details><summary>{result.blockers.length} blockers</summary><small>{result.blockers.join('; ')}</small></details></>}</> : 'Not simulated'}{row.error && <><br /><small className="error-text">{row.error}</small></>}</td></tr> }) : [renderEmptyRow(row)]) : repoComplete ? <tr><td colSpan={4}>No affected repositories.</td></tr> : null}</tbody></table></DataTable></>}
    {rows.map(row => !row.complete && !row.loading && row.cursor ? <Button key={`more-${row.repository.id}`} type="button" disabled={busy} onClick={() => loadMoreEvidence(row)}>Load more evidence for {row.repository.name}</Button> : null)}
    {repositories.length > 0 && !repoComplete && repoCursor && scope.kind !== 'repository' && <Button type="button" disabled={busy} onClick={loadMoreRepositories}>{repoLoading ? 'Loading repositories…' : 'Load more repositories'}</Button>}
    {runError && <p className="error-text" role="alert">{runError}</p>}
  </section>
}
