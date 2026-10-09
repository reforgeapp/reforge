import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { autopilotAPI } from '../policy-api'

const kindLabel = { grant: 'Permission', review: 'Review', blocked: 'Blocked' } as Record<string, string>

export function NeedsBanner({ orgID }: { orgID: string }) {
  const needs = useQuery({ queryKey: ['org', orgID, 'autopilot-needs'], queryFn: ({ signal }) => autopilotAPI.needs(orgID, signal), refetchInterval: 60_000, retry: false })
  const [open, setOpen] = useState(false)
  const items = needs.data ?? []
  if (!items.length) return null
  const org = encodeURIComponent(orgID)
  return <>
    <div className="needs-banner" role="status"><strong>{items.length} {items.length === 1 ? 'item needs' : 'items need'} you</strong><span>Reforge can't finish these on its own.</span><Button className="button button-sm" onClick={() => setOpen(true)}>Review</Button></div>
    <Dialog open={open} title="Needs you" onClose={() => setOpen(false)}>
      <ul className="needs-list">{items.map(item => <li key={item.kind + item.finding_id}>
        <div><span className="table-meta">{kindLabel[item.kind] ?? item.kind} · {item.repository}</span><strong>{item.title}</strong><span className="table-meta">{item.reason}</span></div>
        <div className="row-actions">{item.url && <a className="button button-primary button-sm" href={item.url} target="_blank" rel="noreferrer">{item.label || 'Open'}</a>}<a className="button button-sm" href={`/org/${org}/findings?finding=${encodeURIComponent(item.finding_id)}`}>Finding</a></div>
      </li>)}</ul>
    </Dialog>
  </>
}
