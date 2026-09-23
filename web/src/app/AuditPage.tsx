import { useEffect, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { inventoryAPI } from '../api/inventory'
import { auditAPI, type AuditEvent } from '../audit-api'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { SplitView } from '../components/Workspace'
import { useSession } from './query'
import '../styles/audit.css'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const localTime = (value: string) => { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
const instant = (value: string) => { if (!value) return undefined; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toISOString() }
const shortID = (value: string) => value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value
const actionName = (value: string) => value.split(/[._-]/).filter(Boolean).map(part => part[0].toUpperCase() + part.slice(1)).join(' ')
const objectType = (action: string, objectID: string) => {
  const [category = '', event = ''] = action.split('.', 2)
  if (category === 'runner') {
    const subject = event.split('_', 1)[0]
    if (subject === 'pool') return 'Runner Pool'
    if (subject === 'enrollment') return 'Runner Enrollment'
    return 'Runner'
  }
  if (category === 'automation') {
    const scope = objectID.split(':', 1)[0]
    const types: Record<string, string> = { campaign: 'Campaign', model: 'Model', recipe: 'Recipe', repository: 'Repository', runner_pool: 'Runner Pool' }
    return types[scope] ?? 'Automation'
  }
  return actionName(category)
}

export function AuditPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const selectedID = search.event ?? ''
  const setSelectedID = (id?: string) => { void navigate({ search: previous => id ? { ...previous, event: id } : (() => { const next = { ...previous }; delete next.event; return next })() }) }
  const [repositoryID, setRepositoryID] = useState(search.audit_repository ?? '')
  const [action, setAction] = useState(search.audit_action ?? '')
  const [query, setQuery] = useState(search.audit_query ?? '')
  const [actorID, setActorID] = useState(search.audit_actor ?? '')
  const [since, setSince] = useState(search.audit_since ?? '')
  const [until, setUntil] = useState(search.audit_until ?? '')
  const [applied, setApplied] = useState({ repositoryID, action, query, actorID, since, until })
  const [exportPageIndex, setExportPageIndex] = useState(0)
  const [exporting, setExporting] = useState(false)
  const [exportMessage, setExportMessage] = useState('')
  const [exportStatus, setExportStatus] = useState<{ complete: boolean }>()

  useEffect(() => {
    const next = { repositoryID: search.audit_repository ?? '', action: search.audit_action ?? '', query: search.audit_query ?? '', actorID: search.audit_actor ?? '', since: search.audit_since ?? '', until: search.audit_until ?? '' }
    setRepositoryID(next.repositoryID)
    setAction(next.action)
    setQuery(next.query)
    setActorID(next.actorID)
    setSince(next.since)
    setUntil(next.until)
    setApplied(next)
  }, [search.audit_repository, search.audit_action, search.audit_query, search.audit_actor, search.audit_since, search.audit_until])

  const repositories = useInfiniteQuery({ queryKey: ['org', orgID, 'audit-repositories'], queryFn: ({ pageParam, signal }) => inventoryAPI.repositories(orgID, { cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const params = { repository_id: applied.repositoryID || undefined, actor_id: applied.actorID || undefined, action: applied.action || undefined, since: instant(applied.since), until: instant(applied.until) }
  const events = useInfiniteQuery({ queryKey: ['org', orgID, 'audit-events', params], queryFn: ({ pageParam, signal }) => auditAPI.events(orgID, { ...params, cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => { if (page.complete) return undefined; if (!page.next_cursor) throw new Error('Audit pagination stopped before completion.'); return page.next_cursor } })
  const rows = events.data?.pages.flatMap(page => page.items) ?? []
  const visibleRows = rows.filter(event => !applied.query || `${event.action} ${event.actor_id} ${event.object_id} ${event.request_id} ${JSON.stringify(event.data)}`.toLowerCase().includes(applied.query.toLowerCase()))
  const selected = rows.find(event => event.id === selectedID)
  const repositoryItems = repositories.data?.pages.flatMap(page => page.items) ?? []
  const repositoryName = (id?: string) => id ? repositoryItems.find(item => item.id === id)?.name ?? shortID(id) : 'Organisation'
  const applyFilters = () => {
    setSelectedID(undefined)
    setExportPageIndex(0)
    setExportStatus(undefined)
    setApplied({ repositoryID, action, query, actorID, since, until })
    void navigate({ search: previous => ({ ...previous, audit_repository: repositoryID || undefined, audit_action: action || undefined, audit_query: query || undefined, audit_actor: actorID || undefined, audit_since: since || undefined, audit_until: until || undefined, event: undefined }) })
  }
  const exportPage = async () => {
    setExporting(true)
    setExportMessage('')
    setExportStatus(undefined)
    const cursor = events.data?.pageParams[exportPageIndex] as string | undefined
    try {
      const result = await auditAPI.exportPage(orgID, { ...params, cursor, limit: 100 })
      setExportStatus({ complete: result.complete })
      const url = URL.createObjectURL(result.blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `audit-events-page-${exportPageIndex + 1}-${new Date().toISOString().slice(0, 10)}.ndjson`
      anchor.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
    } catch (reason) {
      setExportMessage(errorText(reason))
    } finally {
      setExporting(false)
    }
  }

  if (session.isLoading) return <StatePanel kind="loading" title="Loading audit access" detail="Checking the current session." />
  if (session.error) return <StatePanel kind="error" title="Audit access unavailable" detail={errorText(session.error)} />

  const list = <>
    <form className="audit-query" aria-label="Audit filters" onSubmit={event => { event.preventDefault(); applyFilters() }}>
      <label className="audit-query-text">
        Search loaded events
        <input aria-label="Search loaded events" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search loaded events" />
      </label>
      <label>
        Repository
        <select aria-label="Repository filter" value={repositoryID} onChange={event => setRepositoryID(event.target.value)}>
          <option value="">All repositories</option>
          {repositoryItems.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
        </select>
      </label>
      <label>
        Action
        <input aria-label="Action filter" value={action} onChange={event => setAction(event.target.value)} placeholder="Any action" />
      </label>
      <Button type="submit" className="button button-primary">Filter</Button>
      <details className="audit-advanced">
        <summary>More filters</summary>
        <div className="audit-advanced-fields">
          <label>Actor<input aria-label="Actor filter" value={actorID} onChange={event => setActorID(event.target.value)} placeholder="Actor ID" /></label>
          <label>From<input aria-label="Since filter" type="datetime-local" value={since} onChange={event => setSince(event.target.value)} /></label>
          <label>To<input aria-label="Until filter" type="datetime-local" value={until} onChange={event => setUntil(event.target.value)} /></label>
        </div>
      </details>
    </form>
    {repositories.error && <p className="error-text" role="alert">Repositories unavailable: {errorText(repositories.error)} <Button onClick={() => void repositories.refetch()}>Retry</Button></p>}
    {repositories.hasNextPage && <Button onClick={() => void repositories.fetchNextPage()} disabled={repositories.isFetchingNextPage}>{repositories.isFetchingNextPage ? 'Loading…' : 'Load repositories'}</Button>}
    {events.error && <p className="error-text" role="alert">Audit events unavailable: {errorText(events.error)} <Button onClick={() => void events.refetch()}>Retry</Button></p>}
    <div className="audit-results-bar">
      <span>{visibleRows.length} shown</span>
      {events.data?.pages.length ? <div className="audit-export">
        {events.data.pages.length > 1 && <label><span className="sr-only">Export page</span><select aria-label="Audit export page" value={exportPageIndex} onChange={event => { setExportPageIndex(Number(event.target.value)); setExportStatus(undefined); setExportMessage('') }}>{events.data.pages.map((_, index) => <option key={index} value={index}>Page {index + 1}</option>)}</select></label>}
        <Button className="audit-export-button" aria-label="Export server page" onClick={() => void exportPage()} disabled={exporting || events.isLoading || !!events.error}><span aria-hidden="true">↓</span>{exporting ? 'Exporting…' : 'Export page'}</Button>
      </div> : null}
    </div>
    {exportStatus && <span className="audit-status" role="status">Page {exportPageIndex + 1} exported{exportStatus.complete ? ' · final page' : ''}</span>}
    {exportMessage && <p className="error-text" role="alert">Export unavailable: {exportMessage}</p>}
    {events.isLoading ? <StatePanel kind="loading" title="Loading audit events" detail="Fetching audit events." /> : <AuditTable rows={visibleRows} selectedID={selectedID} repositoryName={repositoryName} currentActor={session.data?.user} onSelect={setSelectedID} />}
    {events.hasNextPage && <Button className="audit-load-more" onClick={() => void events.fetchNextPage()} disabled={events.isFetchingNextPage}>{events.isFetchingNextPage ? 'Loading…' : 'Load more events'}</Button>}
  </>

  return <div className="audit-page">
    <SplitView listLabel="Audit results and filters" list={list} selected={!!selected} onBack={() => setSelectedID(undefined)} hideBack closeControl={{ label: 'Close event details', onClose: () => setSelectedID(undefined) }} detail={selected ? <AuditDetails event={selected} repositoryName={repositoryName(selected.repository_id)} /> : null} />
  </div>
}

function AuditTable({ rows, selectedID, repositoryName, currentActor, onSelect }: { rows: AuditEvent[]; selectedID: string; repositoryName: (id?: string) => string; currentActor?: { id: string; name: string }; onSelect: (id: string) => void }) {
  return <DataTable caption="Audit events">
    <table className="audit-events-table">
      <thead><tr><th>Time</th><th>Activity</th><th>Actor / scope</th></tr></thead>
      <tbody>
        {rows.map(event => <AuditRow key={event.id} event={event} selected={event.id === selectedID} repositoryName={repositoryName(event.repository_id)} currentActor={currentActor} onSelect={onSelect} />)}
      </tbody>
    </table>
    {!rows.length && <EmptyTable label="No audit events found." />}
  </DataTable>
}

function AuditRow({ event, selected, repositoryName, currentActor, onSelect }: { event: AuditEvent; selected: boolean; repositoryName: string; currentActor?: { id: string; name: string }; onSelect: (id: string) => void }) {
  const actorName = !event.actor_id ? 'System' : event.actor_id === currentActor?.id ? currentActor.name : `Account ${shortID(event.actor_id)}`
  return <tr className={selected ? 'audit-row-selected' : undefined}>
    <td data-label="Time"><time dateTime={event.created_at}>{localTime(event.created_at)}</time></td>
    <td data-label="Activity">
      <button className="audit-event-link link-button" aria-label={`Open ${actionName(event.action)} event`} onClick={() => onSelect(event.id)}>
        <strong>{actionName(event.action)}</strong>
      </button>
      <small>{objectType(event.action, event.object_id)}</small>
    </td>
    <td data-label="Actor / scope"><div className="audit-byline"><span>{actorName}</span><span>{repositoryName}</span></div></td>
  </tr>
}

function AuditDetails({ event, repositoryName }: { event: AuditEvent; repositoryName: string }) {
  return <section className="audit-detail" aria-label="Audit event details">
    <header>
      <div>
        <time>{localTime(event.created_at)}</time>
        <h2>{actionName(event.action)}</h2>
      </div>
    </header>
    <dl>
      <AuditField label="Actor" value={event.actor_id || 'System'} />
      <AuditField label="Target" value={event.object_id} />
      <AuditField label="Repository" value={repositoryName} />
      <AuditField label="Event ID" value={event.id} />
      <AuditField label="Request ID" value={event.request_id} />
    </dl>
    <details>
      <summary>Event data</summary>
      <pre>{JSON.stringify(event.data, null, 2)}</pre>
    </details>
  </section>
}

function AuditField({ label, value }: { label: string; value: string }) {
  return <div><dt>{label}</dt><dd>{value}</dd></div>
}
