import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { overviewAPI } from '../overview-api'
import { Tabs } from '../components/Workspace'
import { Donut, TrendChart } from '../components/Charts'
import { SetupChecklist } from './SetupChecklist'
import { OwnershipTable } from './OwnershipTable'
import { NeedsPanel } from './NeedsBanner'
import '../styles/overview.css'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const age = (seconds: number) => seconds < 3600 ? `${Math.max(1, Math.round(seconds / 60))}m` : seconds < 86400 ? `${Math.round(seconds / 3600)}h` : `${Math.round(seconds / 86400)}d`
const utc = (day: string, options: Intl.DateTimeFormatOptions) => new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, { ...options, timeZone: 'UTC' })
const severityOrder = ['critical', 'high', 'medium', 'low', 'info']
const severityColor = (severity: string) => severity === 'critical' ? 'var(--red)' : severity === 'high' ? 'var(--chart-2)' : severity === 'medium' ? 'var(--amber)' : severity === 'low' ? 'var(--chart-1)' : 'var(--border-strong)'
const activitySeries = [{ key: 'findings', label: 'Findings', color: 'var(--chart-1)' }, { key: 'runs', label: 'Runs', color: 'var(--chart-2)' }, { key: 'merges', label: 'Merged', color: 'var(--chart-3)' }, { key: 'deployments', label: 'Healthy deployments', color: 'var(--chart-4)' }]
const severityTone = (severity: string) => severity === 'critical' || severity === 'high' ? 'red' as const : severity === 'medium' ? 'amber' as const : 'neutral' as const

export function OverviewPage({ orgID }: { orgID: string }) {
  const [tab, setTab] = useState('attention')
  const query = useQuery({ queryKey: ['org', orgID, 'overview'], queryFn: ({ signal }) => overviewAPI.get(orgID, signal), refetchInterval: 30_000 })
  const attentionSort = useSort(query.data?.attention ?? [], { title: item => item.title, repository: item => item.repository_name || item.repository_id, severity: item => ['info', 'low', 'medium', 'high', 'critical'].indexOf(item.severity), age: item => item.age_seconds, owner: item => item.assigned_to || '' }, { key: 'severity', dir: 'desc' })
  const portfolioSort = useSort(query.data?.portfolio ?? [], { repository: row => row.repository_name, forge: row => row.provider, work: row => row.open_findings + row.open_changes, freshness: row => row.last_synced_at ? Date.parse(row.last_synced_at) : undefined, blocker: row => row.blocker || '' }, { key: 'work', dir: 'desc' })
  if (query.isLoading) return <StatePanel kind="loading" title="Loading overview" detail="Fetching scoped portfolio counts and the current attention queue." />
  if (query.error) return <StatePanel kind="error" title="Overview unavailable" detail={errorText(query.error)} action={<Button onClick={() => void query.refetch()}>Retry</Button>} />
  const value = query.data
  if (!value) return <StatePanel kind="empty" title="No overview data" detail="The server returned no portfolio records." />
  const c = value.counts
  const stale = c.stale_repositories > 0
  const trend = value.trend ?? []
  const severity = value.severity ?? []
  const progress = value.maintenance ?? []
  const count = (state: string) => progress.filter(item => item.state === state).reduce((sum, item) => sum + item.count, 0)
  const waitingLimit = count('waiting_limit')
  const waiting = count('waiting')
  const remaining = severity.reduce((sum, item) => sum + item.count, 0)
  const priority = severity.filter(item => item.severity === 'high' || item.severity === 'critical').reduce((sum, item) => sum + item.count, 0)
  const merged = trend.reduce((sum, day) => sum + day.merges, 0)
  const waitingCount = waitingLimit + waiting + count('queued')
  const tracked = value.maintenance ? progress.reduce((sum, item) => sum + item.count, 0) : remaining
  const heading = c.needs_decision ? 'A decision is holding up maintenance' : waitingLimit && !value.capacity.running_jobs ? 'Maintenance is waiting for allowance' : value.capacity.running_jobs ? 'Reforge is working' : waiting ? 'Some repairs are waiting' : tracked ? 'Your maintenance queue' : c.accessible_repositories ? 'No open issues recorded' : 'Add your first repository'
  const stateLabels: Record<string, string> = { fixing: 'being fixed', queued: 'queued for a fix', waiting_limit: 'waiting for budget or provider allowance', checking: 'proposed fixes awaiting verification or merge', waiting: 'waiting for a verifiable fix or service recovery', scheduled: 'awaiting a repair' }
  const repositoryIDs = [...new Set([...value.portfolio.map(row => row.repository_id), ...progress.map(row => row.repository_id)])]
  return <div className="stack">
    <SetupChecklist orgID={orgID} />
    <section className="panel overview-owner-status" aria-label="Maintenance status">
      <h2>{heading}</h2>
      <p>{c.needs_decision ? 'Review the decision below so maintenance can continue.' : 'Nothing needs your decision.'}{waitingLimit > 0 && ' Paused work resumes automatically when budget or provider allowance is available.'}</p>
      {priority > 0 && <p className="table-meta">{priority} higher-priority {priority === 1 ? 'issue remains' : 'issues remain'} open. Progress and waits are shown below.</p>}
    </section>
    <section className="overview-metrics" aria-label="Portfolio counts">
      <div className="metric-grid">
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/changes?state=merged`}><span className="metric-label">Fixes merged</span><strong>{merged}</strong><span className="metric-muted">Last 14 days</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/runs?state=active`}><span className="metric-label">In progress</span><strong>{value.capacity.running_jobs}</strong><span className="metric-muted">{value.capacity.queued_jobs} queued</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/runs?state=review`}><span className="metric-label">Fixes being checked</span><strong>{c.ready_for_review}</strong><span className="metric-muted">Verification or merge checks pending</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/runs?state=needs`}><span className="metric-label">Needs you</span><strong>{c.needs_decision}</strong><span className="metric-muted">{c.needs_decision ? 'Decisions to unblock work' : 'No decisions waiting'}</span></a>
      </div>
      {stale && <div className="overview-sync-attention" role="status">
        <a className="overview-sync-link" href={`/org/${encodeURIComponent(orgID)}/repositories?status=stale`}>
          <span className="overview-sync-mark" aria-hidden="true">!</span>
          <span className="overview-sync-copy">
            <strong>{c.stale_repositories} {c.stale_repositories === 1 ? 'repository needs' : 'repositories need'} sync</strong>
            <span>Review stale repositories <span aria-hidden="true">→</span></span>
          </span>
        </a>
      </div>}
    </section>
    {c.needs_decision > 0 && <NeedsPanel orgID={orgID} />}
    <section className="panel" aria-label="Your repositories"><div className="panel-head"><h2>Your repositories</h2><a href={`/org/${encodeURIComponent(orgID)}/repositories`}>View all</a></div>
      <div className="overview-repositories">{repositoryIDs.map(repositoryID => {
        const repository = value.portfolio.find(row => row.repository_id === repositoryID)
        const rows = progress.filter(row => row.repository_id === repositoryID)
        const name = repository?.repository_name ?? rows[0]?.repository_name ?? repositoryID
        return <article className="overview-repository" key={repositoryID}><h3><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(repositoryID)}`}>{name}</a></h3>
          {repository?.blocker && <p>{repository.blocker}</p>}
          {rows.length ? <ul>{rows.map(row => <li key={row.state}>{row.count} {stateLabels[row.state] ?? 'awaiting maintenance'}</li>)}</ul> : <p className="table-meta">{repository?.open_findings ? `${repository.open_findings} issues tracked. Status is being refreshed.` : 'No open issues recorded.'}</p>}
          <a className="link-button" href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(repositoryID)}&setup=autopilot`}>Autopilot setup</a>
        </article>
      })}</div>
      {!repositoryIDs.length && <p>Import a repository to begin maintenance.</p>}
      {waitingCount > 0 && <p className="table-meta">{waitingCount} {waitingCount === 1 ? 'repair is' : 'repairs are'} waiting. Reforge keeps these tracked until a verified fix is available.</p>}
    </section>
    <details className="overview-details"><summary>Technical details</summary><div className="stack">
      <div className="row-actions"><a href={`/org/${encodeURIComponent(orgID)}/runs?state=blocked`}>Waiting work ({c.blocked})</a><a href={`/org/${encodeURIComponent(orgID)}/findings`}>All tracked issues ({remaining})</a>{c.verified_deployments > 0 && <a href={`/org/${encodeURIComponent(orgID)}/deployments?health_state=healthy`}>{c.verified_deployments} healthy environments</a>}</div>
      <dl className="capacity-strip" aria-label="Capacity and spend">
        <div><dt>Queued jobs</dt><dd>{value.capacity.queued_jobs}</dd></div>
        <div><dt>Running jobs</dt><dd>{value.capacity.running_jobs}</dd></div>
        <div><dt>Active pools</dt><dd>{value.capacity.active_pools}</dd></div>
        <div><dt>Active runners</dt><dd>{value.capacity.active_runners}</dd></div>
        <div><dt>Reserved spend</dt><dd>{value.capacity.reserved_micro_usd ? `$${(value.capacity.reserved_micro_usd / 1_000_000).toFixed(2)}` : 'None held'}</dd></div>
      </dl>
    <div className="panel-grid">
      <section className="panel" aria-labelledby="overview-activity-title">
        <div className="panel-head"><h2 id="overview-activity-title">Activity · 14 days</h2></div>
        <TrendChart label="Activity over the last 14 days" empty="No activity in the last 14 days." integer stacked series={activitySeries} points={trend.map(day => ({ label: utc(day.day, { month: 'short', day: 'numeric' }), detail: utc(day.day, { weekday: 'short', month: 'short', day: 'numeric' }), values: [day.findings, day.runs, day.merges, day.deployments] }))} />
      </section>
      <section className="panel" aria-labelledby="overview-severity-title">
        <div className="panel-head"><h2 id="overview-severity-title">Open findings</h2><a className="link-button" href={`/org/${encodeURIComponent(orgID)}/findings`}>View all</a></div>
        <Donut label="Open findings by severity" empty="No open findings." segments={[...severity].sort((a, b) => severityOrder.indexOf(a.severity) - severityOrder.indexOf(b.severity)).map(item => ({ key: item.severity, label: item.severity, value: item.count, color: severityColor(item.severity) }))} />
      </section>
    </div>
    <Tabs id="overview" label="Overview surfaces" items={[{ id: 'attention', label: 'Attention' }, { id: 'portfolio', label: 'Portfolio' }, { id: 'ownership', label: 'Ownership' }]} value={tab} onChange={setTab} />
    <section id={`overview-panel-${tab}`} role="tabpanel" aria-labelledby={`overview-tab-${tab}`}>
      {tab === 'ownership' ? <OwnershipTable orgID={orgID} /> : tab === 'attention' ? <DataTable caption="Attention queue"><table><thead><tr>{attentionSort.header('title', 'Finding')}{attentionSort.header('repository', 'Repository')}{attentionSort.header('severity', 'Severity')}{attentionSort.header('age', 'Age')}{attentionSort.header('owner', 'Owner')}</tr></thead><tbody>{attentionSort.rows.map(item => <tr key={item.id}><td><a href={`/org/${encodeURIComponent(orgID)}/findings?repository=${encodeURIComponent(item.repository_id)}&finding=${encodeURIComponent(item.id)}`}>{item.title}</a></td><td>{item.repository_name || item.repository_id}</td><td><StatusBadge label={item.severity} tone={severityTone(item.severity)} /></td><td>{age(item.age_seconds)}</td><td>{item.assigned_to || 'Unassigned'}</td></tr>)}</tbody></table>{!value.attention.length && <EmptyTable label="No open findings." />}</DataTable> : <DataTable caption="Portfolio"><table><thead><tr>{portfolioSort.header('repository', 'Repository')}{portfolioSort.header('forge', 'Forge')}{portfolioSort.header('work', 'Open work')}{portfolioSort.header('freshness', 'Freshness')}{portfolioSort.header('blocker', 'Blocker')}</tr></thead><tbody>{portfolioSort.rows.map(row => <tr key={row.repository_id}><td><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(row.repository_id)}`}>{row.repository_name}</a></td><td>{row.provider}</td><td>{row.open_findings} findings · {row.open_changes} changes</td><td>{row.last_synced_at ? new Date(row.last_synced_at).toLocaleString() : 'Never synced'}</td><td>{row.blocker ? <StatusBadge label={row.blocker} tone="amber" /> : 'None recorded'}</td></tr>)}</tbody></table>{!value.portfolio.length && <EmptyTable label="No repositories imported." />}</DataTable>}
    </section>
    </div></details>
    <div className="subsection-actions overview-footer"><span className="table-meta">Accessible repositories: {c.accessible_repositories}</span></div>
  </div>
}
