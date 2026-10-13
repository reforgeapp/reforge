import { useParams } from '@tanstack/react-router'
import { sectionFor } from './types'
import { ConnectionsPage } from './ConnectionsPage'
import { RunnersPage } from './RunnersPage'
import { ChangesPage } from './ChangesPage'
import { DeploymentsPage } from './DeploymentsPage'
import { RunsPage } from './RunsPage'
import { WorkPage } from './WorkPage'
import { useSearch } from '@tanstack/react-router'
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
  const search=useSearch({strict:false}) as {state?:string;run?:string}
  const workBucket=['active','blocked','review','needs'].includes(search.state??'') && !search.run ? search.state : undefined
  const body = section.id === 'overview' ? <OverviewPage key={orgID} orgID={orgID} /> : section.id === 'campaigns' ? <CampaignsPage key={orgID} orgID={orgID} /> : section.id === 'policies' ? <PoliciesPage key={orgID} orgID={orgID} /> : section.id === 'usage' ? <UsagePage key={orgID} orgID={orgID} /> : section.id === 'audit' ? <AuditPage key={orgID} orgID={orgID} /> : section.id === 'deployments' ? <DeploymentsPage key={orgID} orgID={orgID} /> : section.id === 'changes' ? <ChangesPage key={orgID} orgID={orgID} /> : section.id === 'runs' ? workBucket ? <WorkPage key={`${orgID}/${workBucket}`} orgID={orgID} bucket={workBucket} /> : <RunsPage key={orgID} orgID={orgID} /> : section.id === 'findings' ? <FindingsPage orgID={orgID} /> : section.id === 'connections' ? <ConnectionsPage orgID={orgID} /> : section.id === 'runners' ? <RunnersPage orgID={orgID} /> : section.id === 'organisation' ? <OrganisationPage key={orgID} orgID={orgID} /> : <RepositoriesPage orgID={orgID} />
  return <div className="section-page"><div className="page-header"><h1>{section.label}</h1><HelpLink route={section.id} /></div>{body}</div>
}
