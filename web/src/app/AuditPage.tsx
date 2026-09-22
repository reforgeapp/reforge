import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { inventoryAPI } from '../api/inventory'
import { auditAPI, type AuditEvent } from '../audit-api'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { SplitView, SplitPlaceholder } from '../components/Workspace'
import { useSession } from './query'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const localTime = (value: string) => { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
const instant = (value: string) => { if (!value) return undefined; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toISOString() }

export function AuditPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const [repositoryID, setRepositoryID] = useState('')
  const [actorID, setActorID] = useState('')
  const [action, setAction] = useState('')
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const search = useSearch({ strict: false }) as Record<string, string | undefined>
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const selectedID = search.event ?? ''
  const setSelectedID = (id?: string) => { void navigate({ search: previous => id ? { ...previous, event: id } : (() => { const next = { ...previous }; delete next.event; return next })() }) }
  const [applied, setApplied] = useState({ repositoryID: '', actorID: '', action: '', since: '', until: '' })
  const [exportPageIndex, setExportPageIndex] = useState(0)
  const [exporting, setExporting] = useState(false)
  const [exportStatus, setExportStatus] = useState<{ complete: boolean; nextCursor?: string }>()
  const repositories = useInfiniteQuery({
    queryKey: ['org', orgID, 'audit-repositories'],
    queryFn: ({ pageParam, signal }) => inventoryAPI.repositories(orgID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const params = {
    repository_id: applied.repositoryID || undefined,
    actor_id: applied.actorID || undefined,
    action: applied.action || undefined,
    since: instant(applied.since),
    until: instant(applied.until),
  }
  const events = useInfiniteQuery({
    queryKey: ['org', orgID, 'audit-events', params],
    queryFn: ({ pageParam, signal }) => auditAPI.events(orgID, { ...params, cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => {
      if (page.complete) return undefined
      if (!page.next_cursor) throw new Error('Audit pagination stopped before completion.')
      return page.next_cursor
    },
  })
  const eventPages = events.data?.pages ?? []
  const rows = events.data?.pages.flatMap(page => page.items) ?? []
  const selected = rows.find(event => event.id === selectedID)
  const applyFilters = () => { setSelectedID(undefined); setExportPageIndex(0); setExportStatus(undefined); setApplied({ repositoryID, actorID, action, since, until }) }
  const exportPage = async () => {
    setExporting(true)
    setExportMessage('')
    setExportStatus(undefined)
    const cursor = events.data?.pageParams[exportPageIndex] as string | undefined
    try {
      const result = await auditAPI.exportPage(orgID, { ...params, cursor, limit: 100 })
      setExportStatus({ complete: result.complete, nextCursor: result.nextCursor ?? undefined })
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
  const [exportMessage, setExportMessage] = useState('')
  const repositoryItems = repositories.data?.pages.flatMap(page => page.items) ?? []
  const repositoryName = (id?: string) => id ? repositoryItems.find(item => item.id === id)?.name ?? id : 'Organisation'
  if (session.isLoading) return <StatePanel kind="loading" title="Loading audit access" detail="Checking the current session." />
  if (session.error) return <StatePanel kind="error" title="Audit access unavailable" detail={errorText(session.error)} />
  return <div className="stack audit-page">
    <form className="toolbar" aria-label="Audit filters" onSubmit={event => { event.preventDefault(); applyFilters() }}>
      <label>Repository<select aria-label="Repository filter" value={repositoryID} onChange={event => setRepositoryID(event.target.value)}><option value="">All visible repositories</option>{repositoryItems.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label>Actor<input aria-label="Actor filter" value={actorID} onChange={event => setActorID(event.target.value)} placeholder="Actor ID" /></label>
      <label>Action<input aria-label="Action filter" value={action} onChange={event => setAction(event.target.value)} placeholder="Action" /></label>
      <label>Since<input aria-label="Since filter" type="datetime-local" value={since} onChange={event => setSince(event.target.value)} /></label>
      <label>Until<input aria-label="Until filter" type="datetime-local" value={until} onChange={event => setUntil(event.target.value)} /></label>
      <Button type="submit" className="button button-primary">Apply filters</Button>
    </form>
    {repositories.error && <p className="error-text" role="alert">Repositories unavailable: {errorText(repositories.error)} <Button onClick={() => void repositories.refetch()}>Retry</Button></p>}
    {repositories.hasNextPage && <Button onClick={() => void repositories.fetchNextPage()} disabled={repositories.isFetchingNextPage}>{repositories.isFetchingNextPage ? 'Loading repositories…' : 'Load more repositories'}</Button>}
    {eventPages.length > 0 && <div className="row-actions"><label>Export page<select aria-label="Audit export page" value={exportPageIndex} onChange={event => { setExportPageIndex(Number(event.target.value)); setExportStatus(undefined); setExportMessage('') }}>{eventPages.map((_, index) => <option key={index} value={index}>Page {index + 1}</option>)}</select></label><Button onClick={() => void exportPage()} disabled={exporting || events.isLoading || !!events.error}>{exporting ? 'Exporting…' : 'Export selected page'}</Button><span className="table-meta">NDJSON export is limited to the selected loaded page.</span></div>}
    {exportMessage && <p className="error-text" role="alert">Export unavailable: {exportMessage}</p>}
    {exportStatus && <p role="status">Exported page {exportPageIndex + 1}. {exportStatus.complete ? 'Final page reached; earlier pages export separately.' : `More events available${exportStatus.nextCursor ? '; load the next page before exporting it.' : '.'}`}</p>}
    {events.error ? <p className="error-text" role="alert">Audit events unavailable: {errorText(events.error)} <Button onClick={() => void events.refetch()}>Retry</Button></p> : null}
    {events.isLoading ? <StatePanel kind="loading" title="Loading audit events" detail="Fetching the selected audit page." /> : <SplitView listLabel="Audit" selected={!!selected} onBack={() => setSelectedID(undefined)} list={<><DataTable caption="Audit events"><table><thead><tr><th>When</th><th>Action</th><th>Actor</th><th>Object</th><th>Repository</th></tr></thead><tbody>{rows.map(event => <tr key={event.id}><td><button className="link-button" onClick={() => setSelectedID(event.id)}>{localTime(event.created_at)}</button></td><td>{event.action}</td><td><code>{event.actor_id}</code></td><td><code>{event.object_id}</code></td><td>{repositoryName(event.repository_id)}</td></tr>)}</tbody></table>{!rows.length && <EmptyTable label="No audit events match these filters." />}</DataTable>{events.hasNextPage && <Button onClick={() => void events.fetchNextPage()} disabled={events.isFetchingNextPage}>{events.isFetchingNextPage ? 'Loading events…' : 'Load more events'}</Button>}</>} detail={selected ? <AuditDetails event={selected} /> : <SplitPlaceholder label="Select an event to inspect actor, policy and provider data." />} />}
  </div>
}

function AuditDetails({ event }: { event: AuditEvent }) {
  return <section className="detail-panel" aria-label="Audit event details"><h2>Event details</h2><dl className="detail-list"><div><dt>Event ID</dt><dd><code>{event.id}</code></dd></div><div><dt>Request ID</dt><dd><code>{event.request_id}</code></dd></div><div><dt>Actor</dt><dd><code>{event.actor_id}</code></dd></div><div><dt>Action</dt><dd>{event.action}</dd></div><div><dt>Object</dt><dd><code>{event.object_id}</code></dd></div></dl><details><summary>Event data</summary><pre>{JSON.stringify(event.data, null, 2)}</pre></details></section>
}
