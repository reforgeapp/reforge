import { useParams } from '@tanstack/react-router'
import { sectionFor } from './types'
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
import { OverviewPage } from './OverviewPage'
import { OrganisationPage } from './OrganisationPage'
import { HelpLink } from '../components/Help'

export function SectionPage() {
  const { orgID, section: sectionParam } = useParams({ from: '/org/$orgID/$section' })
  const section = sectionFor(sectionParam)
  const body = section.id === 'overview' ? <OverviewPage key={orgID} orgID={orgID} /> : section.id === 'campaigns' ? <CampaignsPage key={orgID} orgID={orgID} /> : section.id === 'policies' ? <PoliciesPage key={orgID} orgID={orgID} /> : section.id === 'usage' ? <UsagePage key={orgID} orgID={orgID} /> : section.id === 'audit' ? <AuditPage key={orgID} orgID={orgID} /> : section.id === 'deployments' ? <DeploymentsPage key={orgID} orgID={orgID} /> : section.id === 'changes' ? <ChangesPage key={orgID} orgID={orgID} /> : section.id === 'runs' ? <RunsPage key={orgID} orgID={orgID} /> : section.id === 'findings' ? <FindingsPage orgID={orgID} /> : section.id === 'connections' ? <ConnectionsPage orgID={orgID} /> : section.id === 'runners' ? <RunnersPage orgID={orgID} /> : section.id === 'organisation' ? <OrganisationPage key={orgID} orgID={orgID} /> : <RepositoriesPage orgID={orgID} />
  return <div className="section-page"><div className="page-header"><h1>{section.label}</h1><HelpLink route={section.id} /></div>{body}</div>
}
