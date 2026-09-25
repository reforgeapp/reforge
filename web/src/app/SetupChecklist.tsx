import { useQuery } from '@tanstack/react-query'
import { setupAPI, type Setup } from '../overview-api'
import '../styles/setup.css'

type Step = Setup['steps'][number]

const steps: Record<Step['id'], { label: string; action?: string; href?: (org: string, step: Step) => string; note?: string }> = {
  forge: { label: 'Connect your code host', action: 'Connect', href: org => `/org/${org}/connections` },
  repository: { label: 'Import a repository', action: 'Import', href: (org, step) => `/org/${org}/connections${step.connection_id ? `?connection=${step.connection_id}` : ''}` },
  scan: { label: 'Scan a repository', action: 'Scan', href: (org, step) => `/org/${org}/repositories${step.repository_id ? `?repository=${step.repository_id}` : ''}` },
  model: { label: 'Add an AI model', action: 'Add model', href: org => `/org/${org}/connections?connection_tab=models` },
  pricing: { label: 'Set model pricing', action: 'Set pricing', href: (org, step) => `/org/${org}/connections?connection_tab=models${step.connection_id ? `&connection=${step.connection_id}` : ''}` },
  budget: { label: 'Set a spending limit', action: 'Budgets', href: org => `/org/${org}/usage` },
  runner: { label: 'Start a runner', action: 'Runners', href: org => `/org/${org}/runners` },
  assignment: { label: 'Give a runner your repositories', action: 'Assign', href: (org, step) => `/org/${org}/repositories${step.repository_id ? `?repository=${step.repository_id}` : ''}` },
  policy: { label: 'Allow Reforge to propose fixes', action: 'Policies', href: org => `/org/${org}/policies` },
  publish: { label: 'Install the GitHub App to publish fixes', action: 'Connections', href: org => `/org/${org}/connections` },
  server: { label: 'Enable fixes on this server', note: 'Ask your administrator to configure repair images.' },
}

export function SetupChecklist({ orgID }: { orgID: string }) {
  const setup = useQuery({ queryKey: ['org', orgID, 'setup'], queryFn: ({ signal }) => setupAPI.get(orgID, signal), retry: false, staleTime: 0 })
  const items = setup.data?.steps ?? []
  const done = items.filter(item => item.done).length
  if (!items.length || done === items.length) return null
  const next = items.find(item => !item.done)?.id
  const org = encodeURIComponent(orgID)
  return <section className="panel setup" aria-labelledby="setup-title">
    <div className="panel-head"><h2 id="setup-title">Get started</h2><span className="table-meta">{done} of {items.length} done</span></div>
    <div className="setup-progress" aria-hidden="true"><span style={{ width: `${done / items.length * 100}%` }} /></div>
    <ol className="setup-steps">{items.map((item, index) => {
      const step = steps[item.id]
      const detail = item.done ? '' : item.reason || step.note || ''
      return <li key={item.id} className={item.done ? 'done' : item.id === next ? 'next' : ''}>
        <span className="setup-mark" aria-hidden="true">{item.done ? '✓' : index + 1}</span>
        <span className="setup-text"><span>{step.label}<span className="sr-only">{item.done ? ' (done)' : ' (to do)'}</span></span>{detail && <small>{detail}</small>}</span>
        {!item.done && step.href && <a className={`button button-sm ${item.id === next ? 'button-primary' : ''}`} href={step.href(org, item)}>{step.action}</a>}
      </li>
    })}</ol>
  </section>
}
