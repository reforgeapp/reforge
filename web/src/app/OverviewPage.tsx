import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { overviewAPI } from '../overview-api'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const age = (seconds: number) => seconds < 3600 ? `${Math.max(1, Math.round(seconds / 60))}m` : seconds < 86400 ? `${Math.round(seconds / 3600)}h` : `${Math.round(seconds / 86400)}d`
const severityTone = (severity: string) => severity === 'critical' || severity === 'high' ? 'red' as const : severity === 'medium' ? 'amber' as const : 'neutral' as const

export function OverviewPage({ orgID }: { orgID: string }) {
  const query = useQuery({ queryKey: ['org', orgID, 'overview'], queryFn: ({ signal }) => overviewAPI.get(orgID, signal), refetchInterval: 30_000 })
  if (query.isLoading) return <StatePanel kind="loading" title="Loading overview" detail="Fetching scoped portfolio counts and the current attention queue." />
  if (query.error) return <StatePanel kind="error" title="Overview unavailable" detail={errorText(query.error)} action={<Button onClick={() => void query.refetch()}>Retry</Button>} />
  const value = query.data
  if (!value) return <StatePanel kind="empty" title="No overview data" detail="The server returned no portfolio records." />
  const c = value.counts
  const stale = c.stale_repositories > 0
  return <div className="stack">
    <section className="state-card" aria-label="Portfolio counts">
      <div className="metric-grid">
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/findings`}><span className="metric-label">Needs decision</span><strong>{c.needs_decision}</strong><span className="metric-muted">{c.needs_decision ? 'Open findings' : 'Nothing open'}</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/runs?state=running`}><span className="metric-label">Running</span><strong>{c.running}</strong><span className="metric-muted">{c.queued_jobs} jobs queued</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/changes`}><span className="metric-label">Ready for review</span><strong>{c.ready_for_review}</strong><span className="metric-muted">Published candidates</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/changes?state=blocked`}><span className="metric-label">Blocked</span><strong>{c.blocked}</strong><span className="metric-muted">Needs review</span></a>
        <a className="metric-card metric-link" href={`/org/${encodeURIComponent(orgID)}/deployments`}><span className="metric-label">Verified deployments</span><strong>{c.verified_deployments}</strong><span className="metric-muted">Health confirmed</span></a>
      </div>
      {stale && <p className="table-meta" role="status">Stale/unsynced repositories: {c.stale_repositories}. A clear attention queue does not mean the inventory is healthy.</p>}
    </section>
    <section className="state-card"><DataTable caption="Attention queue"><table><thead><tr><th>Finding</th><th>Repository</th><th>Severity</th><th>Age</th><th>Owner</th></tr></thead><tbody>{value.attention.map(item => <tr key={item.id}><td><a href={`/org/${encodeURIComponent(orgID)}/findings?repository=${encodeURIComponent(item.repository_id)}&finding=${encodeURIComponent(item.id)}`}>{item.title}</a></td><td>{item.repository_name || item.repository_id}</td><td><StatusBadge label={item.severity} tone={severityTone(item.severity)} /></td><td>{age(item.age_seconds)}</td><td>{item.assigned_to || 'Unassigned'}</td></tr>)}</tbody></table>{!value.attention.length && <EmptyTable label="No open findings." />}</DataTable></section>
    <section className="state-card"><div className="subsection-actions"><div><h2>Inventory readiness</h2><p className="table-meta">Accessible repositories: {c.accessible_repositories}. Stale records stay visible until a sync confirms provider truth.</p></div><a className="button" href={`/org/${encodeURIComponent(orgID)}/repositories`}>Open inventory</a></div></section>
  </div>
}
