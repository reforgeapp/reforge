import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { StatusBadge } from '../components/Status'
import { apiRequest } from '../api/client'
import type { Gate } from '../merge-api'

type Revalidation = { task_id: string; change_id: string; companion_id: string; state: 'pending' | 'waiting_companion' | 'blocked' | 'ready' | 'merged' | 'closed'; reason: string; observed_at: string | null; gate?: Gate }
type RevalidationPage = { items: Revalidation[] }

const path = (orgID: string, repositoryID: string, changeID: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}/repositories/${encodeURIComponent(repositoryID)}/changes/${encodeURIComponent(changeID)}/revalidations`
const tone = (state: string) => state === 'ready' || state === 'merged' ? 'green' as const : state === 'blocked' || state === 'closed' ? 'red' as const : 'amber' as const
const time = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not observed'

export function BotRevalidation({ orgID, repositoryID, changeID }: { orgID: string; repositoryID: string; changeID: string }) {
  const result = useQuery({
    queryKey: ['org', orgID, 'repository', repositoryID, 'change', changeID, 'revalidations'],
    queryFn: ({ signal }) => apiRequest<RevalidationPage>(path(orgID, repositoryID, changeID), { signal }),
    enabled: !!orgID && !!repositoryID && !!changeID,
    refetchInterval: 15_000,
  })
  const [now, setNow] = useState(Date.now())
  const items = result.data?.items ?? []
  const hasReady = items.some(item => item.state === 'ready')
  useEffect(() => { if (!hasReady) return; const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer) }, [hasReady])
  if (result.error) return <section className="detail-section" aria-label="Automatic bot revalidation" role="alert"><h3>Automatic bot revalidation</h3><p className="error-text">Automatic revalidation unavailable. <Button onClick={() => void result.refetch()}>Retry</Button></p></section>
  if (!items.length) return null
  return <section className="detail-section" aria-label="Automatic bot revalidation"><h3>Automatic bot revalidation</h3><div className="stack">{items.map(item => { const stale = item.state === 'ready' && (!item.gate || !Number.isFinite(Date.parse(item.gate.expires_at)) || Date.parse(item.gate.expires_at) <= now); const label = stale ? 'stale · refresh needed' : item.state; return <div className="bot-revalidation-row" key={item.task_id}><p><StatusBadge label={label} tone={stale ? 'amber' : tone(item.state)} /> <code>{item.task_id}</code></p>{item.reason && <p className="table-meta">{item.reason}</p>}<p className="table-meta">Observed {time(item.observed_at)} · companion {item.companion_id}</p>{item.gate?.snapshot.train_gate && <p className="table-meta">Pipeline {item.gate.snapshot.train_gate.pipeline_id} · job {item.gate.snapshot.train_gate.job_id} · {item.gate.snapshot.train_gate.state}</p>}</div> })}</div></section>
}
