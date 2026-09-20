export const sections = [
  { id: 'overview', label: 'Overview', group: 'workspace', description: 'Decisions across your portfolio' },
  { id: 'repositories', label: 'Repositories', group: 'workspace', description: 'Connected source repositories' },
  { id: 'findings', label: 'Findings', group: 'workspace', description: 'Issues that need attention' },
  { id: 'runs', label: 'Runs', group: 'workspace', description: 'Bounded repair activity' },
  { id: 'changes', label: 'Changes', group: 'workspace', description: 'Reviews and merge gates' },
  { id: 'deployments', label: 'Deployments', group: 'workspace', description: 'Delivery and verification' },
  { id: 'campaigns', label: 'Campaigns', group: 'workspace', description: 'Portfolio maintenance plans' },
  { id: 'policies', label: 'Policies', group: 'admin', description: 'Rules and effective controls' },
  { id: 'connections', label: 'Connections', group: 'admin', description: 'Forges, models and delivery' },
  { id: 'runners', label: 'Runners', group: 'admin', description: 'Execution pools and capacity' },
  { id: 'usage', label: 'Usage', group: 'admin', description: 'Cost and quota accounting' },
  { id: 'audit', label: 'Audit', group: 'admin', description: 'Immutable action history' },
  { id: 'organisation', label: 'Organisation', group: 'admin', description: 'Membership and scope' },
] as const

export function sectionFor(value: string | undefined): typeof sections[number] {
  return sections.find(section => section.id === value) ?? sections[0]
}
