import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { Icon } from '../components/Icons'
import { autopilotAPI, type Need } from '../policy-api'
import { useSession } from './query'

const groups: Array<{ kind: Need['kind']; label: string }> = [{ kind: 'grant', label: 'Permissions' }, { kind: 'review', label: 'Reviews' }, { kind: 'blocked', label: 'Blocked' }]

export function CheckAgain({ orgID, repositoryID }: { orgID: string; repositoryID: string }) {
  const csrf = useSession().data?.csrf_token ?? ''
  const [state, setState] = useState<'idle' | 'busy' | 'done' | 'failed'>('idle')
  const run = async () => { setState('busy'); try { await autopilotAPI.run(orgID, repositoryID, csrf); setState('done') } catch { setState('failed') } }
  return <Button className="button button-sm" disabled={!csrf || state === 'busy' || state === 'done'} title="Rescan the repository and retry now" onClick={() => void run()}>{state === 'done' ? 'Checking…' : state === 'failed' ? 'Retry failed' : 'Check again'}</Button>
}

export function NeedsBanner({ orgID }: { orgID: string }) {
  const needs = useQuery({ queryKey: ['org', orgID, 'autopilot-needs'], queryFn: ({ signal }) => autopilotAPI.needs(orgID, signal), refetchInterval: 60_000, retry: false })
  const [open, setOpen] = useState(false)
  const items = needs.data ?? []
  if (!items.length) return null
  const findingURL = (item: Need) => `/org/${encodeURIComponent(orgID)}/findings?finding=${encodeURIComponent(item.finding_id)}`
  return <>
    <button type="button" className="needs-banner" onClick={() => setOpen(true)}><strong>{items.length} {items.length === 1 ? 'item needs' : 'items need'} you</strong><span className="button button-sm">Review</span></button>
    <Dialog open={open} title="Needs you" onClose={() => setOpen(false)}>
      <div className="needs-groups">{groups.map(group => { const rows = items.filter(item => item.kind === group.kind); return rows.length ? <section key={group.kind}><h3>{group.label} <span className="table-meta">{rows.length}</span></h3><ul className="needs-list">{rows.map(item => { const external = !!item.url; return <li key={item.kind + item.finding_id}><a href={external ? item.url : findingURL(item)} target={external ? '_blank' : undefined} rel={external ? 'noreferrer' : undefined} title={item.reason}><span className="needs-title">{item.title}</span><span className="needs-meta">{item.repository}</span>{external ? <span className="needs-action">{item.label || 'Open'} <Icon name="external" size={13} /></span> : <Icon name="chevron" size={14} />}</a>{item.kind !== 'review' && <CheckAgain orgID={orgID} repositoryID={item.repository_id} />}</li> })}</ul></section> : null })}</div>
    </Dialog>
  </>
}
