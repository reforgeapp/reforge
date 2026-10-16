import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { Icon } from '../components/Icons'
import { autopilotAPI, type Need } from '../policy-api'
import { useSession } from './query'

const groups: Array<{ kind: Need['kind']; label: string }> = [{ kind: 'grant', label: 'Permissions' }, { kind: 'review', label: 'Reviews' }, { kind: 'blocked', label: 'Blocked' }]

export function CheckAgain({ orgID, repositoryID, icon = false }: { orgID: string; repositoryID: string; icon?: boolean }) {
  const csrf = useSession().data?.csrf_token ?? ''
  const client = useQueryClient()
  const [state, setState] = useState<'idle' | 'busy' | 'failed'>('idle')
  const run = async () => {
    setState('busy')
    try {
      await autopilotAPI.run(orgID, repositoryID, csrf)
      for (const delay of [3000, 12000, 25000]) setTimeout(() => void client.invalidateQueries({ queryKey: ['org', orgID] }), delay)
      setTimeout(() => setState('idle'), 25000)
    } catch { setState('failed') }
  }
  const label = state === 'busy' ? 'Checking…' : state === 'failed' ? 'Retry failed' : 'Check again'
  if (icon) return <button type="button" className={`icon-button need-check${state === 'busy' ? ' is-busy' : ''}`} disabled={!csrf || state === 'busy'} aria-label={label} title={label} onClick={() => void run()}><Icon name="refresh" size={15} /></button>
  return <Button className="button button-sm" disabled={!csrf || state === 'busy'} title="Rescan the repository and retry now" onClick={() => void run()}>{label}</Button>
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
      <div className="needs-groups">{groups.map(group => { const rows = items.filter(item => item.kind === group.kind); return rows.length ? <section key={group.kind}><h3>{group.label} <span className="table-meta">{rows.length}</span></h3><ul className="needs-list">{rows.map(item => <li key={item.kind + item.finding_id} className="need">
        <div className="need-body"><a className="need-title" href={findingURL(item)}>{item.title}</a><span className="need-meta">{item.repository}</span>{item.reason && <p className="need-reason">{item.reason.replace(/^Needs a person: /, '')}</p>}</div>
        <div className="need-actions">{item.url && <a className="button button-sm button-primary" href={item.url} target="_blank" rel="noreferrer">{item.label || 'Open'} <Icon name="external" size={13} /></a>}{item.kind !== 'review' && <CheckAgain orgID={orgID} repositoryID={item.repository_id} icon />}</div>
      </li>)}</ul></section> : null })}</div>
    </Dialog>
  </>
}
