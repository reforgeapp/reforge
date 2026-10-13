import { useEffect, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { apiRequest } from '../api/client'
import { Button } from '../components/Accessible'
import { StatusBadge } from '../components/Status'

type Health = { repository_id: string; repository: string; environment: string; kind: string; operation_id: string; state: string; observed_at?: string; expires_at?: string }

export function EnvironmentHealth({ orgID }: { orgID: string }) {
  const search = useSearch({ strict: false }) as { health_state?: string }
  const [state, setState] = useState(search.health_state ?? '')
  const [now, setNow] = useState(Date.now())
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer) }, [])
  const query = useInfiniteQuery({ queryKey: ['org', orgID, 'environment-health', state], queryFn: ({ pageParam, signal }) => { const params = new URLSearchParams({ limit: '50', state }); if (pageParam) params.set('cursor', pageParam); return apiRequest<{ items: Health[]; complete: boolean; next_cursor?: string }>(`/api/v1/orgs/${encodeURIComponent(orgID)}/environment-health?${params}`, { signal }) }, initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, refetchInterval: 30_000 })
  return <section className="panel" aria-label="Application health"><div className="panel-head"><h2>Application health</h2><select aria-label="Application health filter" value={state} onChange={event => setState(event.target.value)}><option value="">All environments</option><option value="healthy">Healthy</option><option value="unhealthy">Failing</option><option value="unknown">Unknown</option></select></div>{query.error && <p role="alert">Health unavailable <Button onClick={() => void query.refetch()}>Retry</Button></p>}{query.data?.pages.flatMap(page => page.items).map(item => { const current = item.expires_at && Date.parse(item.expires_at) > now ? item.state : 'unknown'; return <div className="subsection-actions" key={`${item.repository_id}/${item.kind}/${item.environment}`}><span>{item.repository} · {item.environment} · {item.kind}</span><span><StatusBadge label={current} tone={current === 'healthy' ? 'green' : current === 'unhealthy' ? 'red' : 'neutral'} />{item.observed_at && <small className="table-meta">{new Date(item.observed_at).toLocaleString()}</small>}</span></div> })}{query.data?.pages[0].items.length === 0 && <p className="table-meta">No application health configured.</p>}{query.hasNextPage && <Button disabled={query.isFetching} onClick={() => void query.fetchNextPage()}>Load more environments</Button>}</section>
}
