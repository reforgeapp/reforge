import { useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { Icon } from '../components/Icons'
import { autopilotAPI, type Need } from '../policy-api'
import { useSession } from './query'

const groups: Array<{ kinds: Need['kind'][]; label: string }> = [{ kinds: ['grant', 'approve', 'review'], label: 'Approvals' }, { kinds: ['reconnect', 'restore_runner', 'raise_limit'], label: 'Setup and limits' }, { kinds: ['unsupported', 'exhausted'], label: 'Needs help' }]
const message = (error: unknown) => error instanceof Error ? error.message : 'Request failed.'
const useNeeds = (orgID: string) => useInfiniteQuery({ queryKey: ['org', orgID, 'autopilot-needs'], queryFn: ({ pageParam, signal }) => autopilotAPI.needs(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, refetchInterval: 60_000, retry: false })

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

function RetryFinding({ orgID, item }: { orgID: string; item: Need }) {
 const session=useSession().data
 const role=session?.memberships.find(member=>member.org_id===orgID)?.role
 const client=useQueryClient()
 const [reason,setReason]=useState('')
 const [busy,setBusy]=useState(false)
 const [error,setError]=useState('')
 const retry=async()=>{setBusy(true);setError('');try {await autopilotAPI.retryFinding(orgID,item.finding_id,item.finding_version,reason,session?.csrf_token??'');await client.invalidateQueries({queryKey:['org',orgID]})}catch(error){setError(message(error))}finally{setBusy(false)}}
 if(!['owner','admin','maintainer'].includes(role??'')) return null
 return <details><summary>Try one more fix</summary><label>What changed?<input value={reason} maxLength={2000} onChange={event=>setReason(event.target.value)} /></label><Button disabled={busy || !reason.trim() || !session?.csrf_token} onClick={()=>void retry()}>Retry one fix</Button>{error && <p role="alert">{error}</p>}</details>
}

function NeedsList({orgID,items}:{orgID:string;items:Need[]}) {
 const findingURL=(item:Need)=>`/org/${encodeURIComponent(orgID)}/findings?finding=${encodeURIComponent(item.finding_id)}`
 const actionURL=(item:Need)=> /^https?:\/\//.test(item.url??'') ? item.url : /^(connections|usage|runners|repositories|findings)(\?|$)/.test(item.url??'') ? `/org/${encodeURIComponent(orgID)}/${item.url}` : findingURL(item)
 return <div className="needs-groups">{groups.map(group=>{const rows=items.filter(item=>group.kinds.includes(item.kind));return rows.length ? <section key={group.label}><h3>{group.label}</h3><ul className="needs-list">{rows.map(item=><li key={item.id} className="need"><div className="need-body"><a className="need-title" href={item.finding_id?findingURL(item):actionURL(item)}>{item.title}</a><span className="need-meta">{item.repository} · {item.owner}</span>{item.kind==='exhausted' ? <><p className="need-reason">Automated fix attempts are exhausted. Review the finding before retrying.</p>{item.reason && <details><summary>Attempt details</summary><p className="need-reason">{item.reason.replace(/^Needs a person: /,'')}</p></details>}</> : item.reason && <p className="need-reason">{item.reason.replace(/^Needs a person: /,'')}</p>}{item.next_retry_at && <small>Next check {new Date(item.next_retry_at).toLocaleString()}</small>}{item.kind==='exhausted' && <RetryFinding orgID={orgID} item={item} />}</div><div className="need-actions">{item.url && <a className="button button-sm button-primary" href={actionURL(item)} target={/^https?:\/\//.test(item.url)?'_blank':undefined} rel="noreferrer">{item.label||'Open'}</a>}{!['review','exhausted','unsupported'].includes(item.kind) && <CheckAgain orgID={orgID} repositoryID={item.repository_id} icon />}</div></li>)}</ul></section>:null})}</div>
}

export function NeedsPanel({orgID}:{orgID:string}) {
 const query=useNeeds(orgID)
 const items=query.data?.pages.flatMap(page=>page.items)??[]
 return <section className="panel" aria-label="Needs you">{query.error && <p role="alert">Decisions unavailable: {message(query.error)} <Button onClick={()=>void query.refetch()}>Retry</Button></p>}{query.isLoading && <p role="status">Loading decisions…</p>}<NeedsList orgID={orgID} items={items} />{query.data && !items.length && <p>No decisions needed.</p>}{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={()=>void query.fetchNextPage()}>Load more decisions</Button>}</section>
}

export function NeedsBanner({ orgID }: { orgID: string }) {
 const query=useNeeds(orgID)
 const [open,setOpen]=useState(false)
 const items=query.data?.pages.flatMap(page=>page.items)??[]
 const total=query.data?.pages[0].total??0
 if(query.error) return <p className="table-meta" role="alert">Needs you unavailable <Button onClick={()=>void query.refetch()}>Retry</Button></p>
 if(!total) return null
 return <><button type="button" className="needs-banner" onClick={()=>setOpen(true)}><strong>{total} {total===1?'item needs':'items need'} you</strong><span className="button button-sm">Review</span></button><Dialog open={open} title="Needs you" onClose={()=>setOpen(false)}><NeedsList orgID={orgID} items={items} />{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={()=>void query.fetchNextPage()}>Load more decisions</Button>}</Dialog></>
}
