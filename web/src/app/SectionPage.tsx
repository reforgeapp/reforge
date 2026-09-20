import { useParams, useSearch } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { repositoriesQuery } from './query'
import { sectionFor } from './types'
import { DataTable, EmptyTable } from '../components/DataTable'
import { GateList, StatusBadge } from '../components/Status'
import { StatePanel } from '../components/StatePanel'

const copy: Record<string, { title: string; detail: string; action?: string }> = {
  overview: { title: 'Portfolio overview', detail: 'See the decisions, blockers and verified outcomes that need your attention.' },
  repositories: { title: 'Repositories', detail: 'Inventory connected repositories and their current synchronisation state.', action: 'Connect repository' },
  findings: { title: 'Findings', detail: 'Review evidence-backed issues discovered across your portfolio.', action: 'Refresh findings' },
  runs: { title: 'Runs', detail: 'Inspect bounded maintenance work and its recorded evidence.' },
  changes: { title: 'Changes', detail: 'Track pull requests, merge requests and the gates that govern them.' },
  deployments: { title: 'Deployments', detail: 'Follow immutable artifacts through native delivery and health verification.' },
  campaigns: { title: 'Campaigns', detail: 'Plan bounded maintenance across an explicit repository snapshot.', action: 'Create campaign' },
  policies: { title: 'Policies', detail: 'Understand the effective rules that control repair, merge and delivery.', action: 'Create policy version' },
  connections: { title: 'Connections', detail: 'Manage forge, model and delivery integrations with capability evidence.', action: 'Add connection' },
  runners: { title: 'Runners', detail: 'Review execution pools, trust boundaries and available capacity.', action: 'Enrol runner' },
  usage: { title: 'Usage', detail: 'Compare observed, estimated and unknown usage within configured budgets.' },
  audit: { title: 'Audit', detail: 'Review immutable actions, authority and policy versions.' },
  organisation: { title: 'Organisation', detail: 'Manage membership, repository access and organisation-level controls.' },
}

export function SectionPage() {
  const { orgID, section: sectionParam } = useParams({ from: '/org/$orgID/$section' })
  const search = useSearch({ strict: false }) as { q?: string }
  const section = sectionFor(sectionParam)
  const text = copy[section.id] ?? copy.overview
  const repositories = useQuery({ ...repositoriesQuery(orgID, search.q ?? ''), enabled: section.id === 'repositories' })
  const isRepositories = section.id === 'repositories'
  return <div className="section-page"><div className="page-header"><div><p className="eyebrow">{section.group === 'admin' ? 'Administration' : 'Workspace'}</p><h1>{text.title}</h1><p>{text.detail}</p></div>{text.action && <button className="button button-primary" disabled title="Available when this route is connected to its backend">{text.action}</button>}</div>{isRepositories ? <RepositoriesPage query={search.q ?? ''} result={repositories} /> : <PlaceholderSection id={section.id} />}</div>
}

function RepositoriesPage({ query, result }: { query: string; result: ReturnType<typeof useQuery> }) {
  if (result.isLoading) return <StatePanel kind="loading" title="Loading repositories" detail="Fetching the first page for this organisation." />
  if (result.error) return <StatePanel kind="error" title="Repositories could not be loaded" detail={result.error instanceof Error ? result.error.message : 'The server returned an unknown error.'} action={<button className="button" onClick={() => result.refetch()}>Retry</button>} />
  const data = result.data as { items?: Array<{ id: string; name: string; provider: string; default_branch: string; archived: boolean; paused: boolean; last_synced_at: string | null }>; complete?: boolean } | undefined
  const items = data?.items ?? []
  if (!items.length) return <div className="stack"><div className="state-card"><span className="state-icon" aria-hidden="true">⌕</span><div><h2>{query ? 'No repositories match this search' : 'No repositories connected'}</h2><p>{query ? 'Try a different repository name or clear the search.' : 'Connect a forge to begin a read-only repository discovery.'}</p></div></div><EmptyTable label="Repositories will appear here after a connection is configured." /></div>
  return <DataTable caption="Connected repositories"><table><thead><tr><th>Repository</th><th>Forge</th><th>Default branch</th><th>Status</th><th>Last synced</th></tr></thead><tbody>{items.map(item => <tr key={item.id}><td><strong>{item.name}</strong></td><td>{item.provider}</td><td><code>{item.default_branch}</code></td><td><StatusBadge label={item.paused ? 'Paused' : item.archived ? 'Archived' : 'Monitored'} tone={item.paused ? 'amber' : item.archived ? 'neutral' : 'green'} /></td><td>{item.last_synced_at ? new Date(item.last_synced_at).toLocaleString() : 'Never synced'}</td></tr>)}</tbody></table>{data?.complete === false && <p className="table-note">More repositories are available. Pagination will continue when this view is connected to the inventory controls.</p>}</DataTable>
}

function PlaceholderSection({ id }: { id: string }) {
  const labels: Record<string, string[]> = {
    overview: ['Needs decision', 'Running', 'Ready for review', 'Blocked', 'Verified deployments'],
    findings: ['Source', 'Category', 'Evidence age', 'Repository', 'Next action'],
    runs: ['Discover', 'Reproduce', 'Plan', 'Repair', 'Validate', 'Publish'],
    changes: ['Candidate validation', 'Required checks', 'Code owner review', 'Target freshness', 'Change policy'],
    deployments: ['Source revision', 'Artifact', 'Native approval', 'Rollout', 'Health verification'],
    campaigns: ['Recipe', 'Repository snapshot', 'Budget', 'Concurrency', 'Stop threshold'],
    policies: ['Scope', 'Recipes', 'Models & spend', 'Merge', 'Deploy'],
    connections: ['Forges', 'Models & agents', 'Delivery integrations'],
    runners: ['Trust level', 'Team binding', 'Private route', 'Heartbeat', 'Drain state'],
    usage: ['Observed', 'Estimated', 'Unknown', 'Reserved', 'Settled'],
    audit: ['Actor', 'Action', 'Policy version', 'Provider link', 'Time'],
    organisation: ['Membership', 'Teams', 'Repository access', 'OIDC', 'Retention'],
  }
  return <div className="stack"><div className="metric-grid">{(labels[id] ?? []).map(label => <div className="metric-card" key={label}><span className="metric-label">{label}</span><strong>—</strong><span className="metric-muted">No data available</span></div>)}</div><div className="state-card"><span className="state-icon teal" aria-hidden="true">◇</span><div><h2>This view is ready for connected data</h2><p>Reforge does not invent portfolio records. Once the supporting service is available, this route will show scoped, persisted records here.</p></div></div>{(id === 'changes' || id === 'deployments') && <GateList items={[{ label: 'Backend capability', detail: 'This control remains unavailable until the corresponding service is connected.', status: 'Blocked' }, { label: 'Evidence freshness', detail: 'No evidence has been recorded for this organisation.', status: 'Unknown' }]} />}</div>
}
