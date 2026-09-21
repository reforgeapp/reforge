import { useParams } from '@tanstack/react-router'
import { sectionFor } from './types'
import { GateList } from '../components/Status'
import { ConnectionsPage } from './ConnectionsPage'
import { RunnersPage } from './RunnersPage'
import { ChangesPage } from './ChangesPage'
import { DeploymentsPage } from './DeploymentsPage'
import { RunsPage } from './RunsPage'
import { FindingsPage } from './FindingsPage'
import { PoliciesPage } from './PoliciesPage'
import { UsagePage } from './UsagePage'
import { CampaignsPage } from './CampaignsPage'
import { AuditPage } from './AuditPage'
import { RepositoriesPage } from './RepositoriesPage'

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
  const section = sectionFor(sectionParam)
  const text = copy[section.id] ?? copy.overview
  const isRepositories = section.id === 'repositories'
  const body = section.id === 'campaigns' ? <CampaignsPage key={orgID} orgID={orgID} /> : section.id === 'policies' ? <PoliciesPage key={orgID} orgID={orgID} /> : section.id === 'usage' ? <UsagePage key={orgID} orgID={orgID} /> : section.id === 'audit' ? <AuditPage key={orgID} orgID={orgID} /> : section.id === 'deployments' ? <DeploymentsPage key={orgID} orgID={orgID} /> : section.id === 'changes' ? <ChangesPage key={orgID} orgID={orgID} /> : section.id === 'runs' ? <RunsPage key={orgID} orgID={orgID} /> : section.id === 'findings' ? <FindingsPage orgID={orgID} /> : section.id === 'connections' ? <ConnectionsPage orgID={orgID} /> : section.id === 'runners' ? <RunnersPage orgID={orgID} /> : isRepositories ? <RepositoriesPage orgID={orgID} /> : <PlaceholderSection id={section.id} />
  return <div className="section-page"><div className="page-header"><div><p className="eyebrow">{section.group === 'admin' ? 'Administration' : 'Workspace'}</p><h1>{text.title}</h1><p>{text.detail}</p></div>{text.action && !['campaigns', 'connections', 'runners', 'repositories', 'findings', 'policies'].includes(section.id) && <button className="button button-primary" disabled title="Available when this route is connected to its backend">{text.action}</button>}</div>{body}</div>
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
