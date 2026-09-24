import { useState } from 'react'
import { Dialog } from './Accessible'
import { Icon } from './Icons'

const topics: Record<string, { summary: string; links: Array<{ label: string; path: string }> }> = {
  overview: { summary: 'Portfolio counts, activity and the attention queue. Counts open their filtered list.', links: [{ label: 'Overview', path: 'overview/' }, { label: 'Triage findings', path: 'findings/' }, { label: 'Runners and capacity', path: 'runners/' }] },
  repositories: { summary: 'Import and scope repositories from each forge.', links: [{ label: 'Connect a forge', path: 'connections/' }, { label: 'Private runner routes', path: 'runners/' }] },
  findings: { summary: 'Evidence-backed issues and repair options.', links: [{ label: 'Repair lifecycle', path: 'repair/' }, { label: 'Budgets', path: 'usage/' }] },
  runs: { summary: 'Bounded repair runs, stage evidence and cancellation.', links: [{ label: 'Validation recipes', path: 'repair/' }, { label: 'Model routes', path: 'models/' }] },
  changes: { summary: 'Native checks, reviews and merge gating.', links: [{ label: 'Merge policy', path: 'merge/' }, { label: 'Provider auth', path: 'connections/' }] },
  deployments: { summary: 'Native pipeline and GitOps delivery with health evidence.', links: [{ label: 'Delivery', path: 'delivery/' }, { label: 'Native approvals', path: 'delivery/' }] },
  campaigns: { summary: 'Pinned repository snapshots, canaries and stop thresholds.', links: [{ label: 'Campaigns', path: 'campaigns/' }, { label: 'Budgets', path: 'usage/' }] },
  policies: { summary: 'Versioned policy, inheritance and simulation.', links: [{ label: 'Policy model', path: 'policies/' }, { label: 'Merge gates', path: 'merge/' }] },
  connections: { summary: 'Forges, models, agents and delivery credentials.', links: [{ label: 'Connections', path: 'connections/' }, { label: 'Agent runtime custody', path: 'agents/' }] },
  runners: { summary: 'Customer-owned execution pools and private routes.', links: [{ label: 'Host prerequisites', path: 'install/' }, { label: 'Runners', path: 'runners/' }] },
  usage: { summary: 'Observed, reserved and unknown usage against budgets.', links: [{ label: 'Budgets', path: 'usage/' }, { label: 'Model billing routes', path: 'models/' }] },
  audit: { summary: 'Immutable action history and export.', links: [{ label: 'Audit', path: 'audit/' }, { label: 'Security model', path: 'security/' }] },
  organisation: { summary: 'Teams, membership and repository scope.', links: [{ label: 'Administration', path: 'admin/' }, { label: 'Security model', path: 'security/' }] },
}

export function HelpLink({ route }: { route: string }) {
  const [open, setOpen] = useState(false)
  const topic = topics[route] ?? topics.overview
  const root = document.documentElement.dataset.docsRoot || '/docs/'
  return <>
    <button className="help-link" onClick={() => setOpen(true)} aria-haspopup="dialog"><Icon name="search" size={14} />Help</button>
    <Dialog open={open} title={`Help · ${route}`} onClose={() => setOpen(false)}>
      <p>{topic.summary}</p>
      <ul className="compact-list">{topic.links.map(link => <li key={link.path}><a href={`${root}${link.path}`} target="_blank" rel="noreferrer">{link.label}</a></li>)}</ul>
      <p className="table-meta">Documentation opens in a new tab. Operator installation and provider-native approval steps live in the operator runbook.</p>
    </Dialog>
  </>
}
