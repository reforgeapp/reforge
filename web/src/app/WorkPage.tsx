import { useInfiniteQuery } from '@tanstack/react-query'
import { apiRequest } from '../api/client'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatusBadge } from '../components/Status'
import { NeedsPanel } from './NeedsBanner'

const stages: Record<string,string>={repair:'Fix',merge:'Merge',native:'Deployment',gitops:'GitOps delivery',review:'Review'}

type Work = { id: string; repository_id: string; repository: string; kind: string; state: string; bucket: string; title: string; reason: string; url: string; created_at: string }

export function WorkPage({ orgID, bucket }: { orgID: string; bucket: string }) {
 const query=useInfiniteQuery({queryKey:['org',orgID,'work',bucket],queryFn:({pageParam,signal})=>{const params=new URLSearchParams({state:bucket,limit:'50'});if(pageParam) params.set('cursor',pageParam);return apiRequest<{items:Work[];complete:boolean;next_cursor?:string}>(`/api/v1/orgs/${encodeURIComponent(orgID)}/work?${params}`,{signal})},initialPageParam:undefined as string|undefined,getNextPageParam:page=>page.complete?undefined:page.next_cursor,enabled:bucket!=='needs',refetchInterval:15_000})
 const rows=query.data?.pages.flatMap(page=>page.items)??[]
 const target=(item:Work)=>/^https?:\/\//.test(item.url)?item.url:`/org/${encodeURIComponent(orgID)}/${item.url}`
 if(bucket==='needs') return <NeedsPanel orgID={orgID} />
 return <section className="stack" aria-label={bucket==='active'?'Running work':bucket==='blocked'?'Blocked work':'Ready for review'}>{query.error && <p role="alert">Work unavailable <Button onClick={()=>void query.refetch()}>Retry</Button></p>}{query.isLoading && <p role="status">Loading work…</p>}<DataTable caption={bucket==='active'?'Running work':bucket==='blocked'?'Blocked work':'Ready for review'}><table><thead><tr><th>Work</th><th>Repository</th><th>Stage</th><th>Status</th></tr></thead><tbody>{rows.map(item=><tr key={item.id}><td><a href={target(item)}>{item.title}</a>{item.reason && <small className="table-meta">{item.reason}</small>}</td><td>{item.repository}</td><td>{stages[item.kind]??item.kind}</td><td><StatusBadge label={item.state} tone={item.bucket==='blocked'?'amber':'neutral'} /></td></tr>)}</tbody></table>{query.data && !rows.length && <EmptyTable label="No work waiting here." />}</DataTable>{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={()=>void query.fetchNextPage()}>Load more work</Button>}</section>
}
