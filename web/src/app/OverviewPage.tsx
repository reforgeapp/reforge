import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { overviewAPI } from '../overview-api'
import { Tabs } from '../components/Workspace'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const age = (seconds: number) => seconds < 3600 ? `${Math.max(1, Math.round(seconds / 60))}m` : seconds < 86400 ? `${Math.round(seconds / 3600)}h` : `${Math.round(seconds / 86400)}d`
const severityTone = (severity: string) => severity === 'critical' || severity === 'high' ? 'red' as const : severity === 'medium' ? 'amber' as const : 'neutral' as const

export function OverviewPage({ orgID }: { orgID: string }) {
  const [tab, setTab] = useState('attention')
  const query = useQuery({ queryKey: ['org', orgID, 'overview'], queryFn: ({ signal }) => overviewAPI.get(orgID, signal), refetchInterval: 30_000 })
  if (query.isLoading) return <StatePanel kind="loading" title="Loading overview" detail="Fetching scoped portfolio counts and the current attention queue." />
  if (query.error) return <StatePanel kind="error" title="Overview unavailable" detail={errorText(query.error)} action={<Button onClick={() => void query.refetch()}>Retry</Button>} />
  const value = query.data
  if (!value) return <StatePanel kind="empty" title="No overview data" detail="The server returned no portfolio records." />
  const c = value.counts
  const stale = c.stale_repositories > 0
  return <div className="stack">
    <section className="overview-metrics" aria-label="Portfolio counts">
      <div className="metric-grid">
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/findings`}><span className="metric-label">Needs decision</span><strong>{c.needs_decision}</strong><span className="metric-muted">{c.needs_decision ? 'Open findings' : 'Nothing open'}</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/runs?state=running`}><span className="metric-label">Running</span><strong>{c.running}</strong><span className="metric-muted">{c.queued_jobs} jobs queued</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/changes`}><span className="metric-label">Ready for review</span><strong>{c.ready_for_review}</strong><span className="metric-muted">Published candidates</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/changes?state=blocked`}><span className="metric-label">Blocked</span><strong>{c.blocked}</strong><span className="metric-muted">Needs review</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/deployments`}><span className="metric-label">Verified deployments</span><strong>{c.verified_deployments}</strong><span className="metric-muted">Health confirmed</span></a>
      </div>
      {stale && <p className="table-meta" role="status"><a href={`/org/${encodeURIComponent(orgID)}/repositories?status=stale`}>{c.stale_repositories} {c.stale_repositories === 1 ? 'repository needs' : 'repositories need'} sync</a></p>}
      <dl className="capacity-strip" aria-label="Capacity and spend">
        <div><dt>Queued jobs</dt><dd>{value.capacity.queued_jobs}</dd></div>
        <div><dt>Running jobs</dt><dd>{value.capacity.running_jobs}</dd></div>
        <div><dt>Active pools</dt><dd>{value.capacity.active_pools}</dd></div>
        <div><dt>Active runners</dt><dd>{value.capacity.active_runners}</dd></div>
        <div><dt>Reserved spend</dt><dd>{value.capacity.reserved_micro_usd ? `$${(value.capacity.reserved_micro_usd / 1_000_000).toFixed(2)}` : 'None held'}</dd></div>
      </dl>
    </section>
    <Tabs id="overview" label="Overview surfaces" items={[{ id: 'attention', label: 'Attention' }, { id: 'portfolio', label: 'Portfolio' }]} value={tab} onChange={setTab} />
    <section id={`overview-panel-${tab}`} role="tabpanel" aria-labelledby={`overview-tab-${tab}`}>
      {tab === 'attention' ? <DataTable caption="Attention queue"><table><thead><tr><th>Finding</th><th>Repository</th><th>Severity</th><th>Age</th><th>Owner</th></tr></thead><tbody>{value.attention.map(item => <tr key={item.id}><td><a href={`/org/${encodeURIComponent(orgID)}/findings?repository=${encodeURIComponent(item.repository_id)}&finding=${encodeURIComponent(item.id)}`}>{item.title}</a></td><td>{item.repository_name || item.repository_id}</td><td><StatusBadge label={item.severity} tone={severityTone(item.severity)} /></td><td>{age(item.age_seconds)}</td><td>{item.assigned_to || 'Unassigned'}</td></tr>)}</tbody></table>{!value.attention.length && <EmptyTable label="No open findings." />}</DataTable> : <DataTable caption="Portfolio"><table><thead><tr><th>Repository</th><th>Forge</th><th>Open work</th><th>Freshness</th><th>Blocker</th></tr></thead><tbody>{value.portfolio.map(row => <tr key={row.repository_id}><td><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(row.repository_id)}`}>{row.repository_name}</a></td><td>{row.provider}</td><td>{row.open_findings} findings · {row.open_changes} changes</td><td>{row.last_synced_at ? new Date(row.last_synced_at).toLocaleString() : 'Never synced'}</td><td>{row.blocker ? <StatusBadge label={row.blocker} tone="amber" /> : 'None recorded'}</td></tr>)}</tbody></table>{!value.portfolio.length && <EmptyTable label="No repositories imported." />}</DataTable>}
    </section>
    <div className="subsection-actions overview-footer"><span className="table-meta">Accessible repositories: {c.accessible_repositories}</span><a className="button" href={`/org/${encodeURIComponent(orgID)}/repositories`}>Open inventory</a></div>
  </div>
}
