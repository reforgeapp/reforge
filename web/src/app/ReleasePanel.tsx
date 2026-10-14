import { useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { apiRequest } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Accessible'
import { useSession } from './query'

export function ReleasePanel({ orgID, repositoryID, csrf }: { orgID: string; repositoryID: string; csrf: string }) {
  const session = useSession()
  const canWrite = session.data?.memberships.some(item => item.org_id === orgID && ['owner', 'admin', 'maintainer'].includes(item.role)) ?? false
  const root = `/api/v1/orgs/${encodeURIComponent(orgID)}`
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [connect, setConnect] = useState(false)
  const setup = useQuery({queryKey: ['org', orgID, 'deployment-setup', repositoryID], queryFn: ({signal}) => apiRequest<{workflows: Array<{path: string}>; paths: Array<{kind: string; environment: string; ready: boolean; blockers: string[]}>; blockers: string[]}>(`${root}/repositories/${encodeURIComponent(repositoryID)}/deployment-setup`, {signal}), enabled: connect})
  const releases = useInfiniteQuery({ queryKey: ['org', orgID, 'releases', repositoryID], queryFn: ({ pageParam, signal }) => apiRequest<components['schemas']['ReleaseReceiptPage']>(`${root}/repositories/${encodeURIComponent(repositoryID)}/releases?limit=30${pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''}`, {signal}), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, refetchInterval: 15000 })
  const items = releases.data?.pages.flatMap(page => page.items) ?? []
  const deploy = async (id: string) => {
    setBusy(id); setError('')
    try {
      const result = await apiRequest<components['schemas']['DeliveryResult']>(`${root}/release-receipts/${encodeURIComponent(id)}/deploy`, {method: 'POST'}, csrf)
      window.location.assign(`/org/${encodeURIComponent(orgID)}/${result.kind === 'gitops' ? 'gitops?promotion=' : 'deployments?deployment='}${encodeURIComponent(result.id)}`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Delivery unavailable.') }
    finally { setBusy('') }
  }
  return <section className="detail-section" aria-label="Delivery"><h3>Delivery</h3><Button disabled={setup.isFetching} onClick={() => setConnect(true)}>Connect deployment</Button>{connect && setup.data && <div role="status">{setup.data.paths.map(path => <p key={path.kind + path.environment}>{path.environment}: {path.ready ? 'Connected; CI behaviour verified' : path.blockers.join('; ')}</p>)}{setup.data.blockers.map(blocker => <p key={blocker}>{blocker}</p>)}{setup.data.workflows.length > 0 && <details><summary>Detected workflows</summary>{setup.data.workflows.map(workflow => <p key={workflow.path}>{workflow.path}</p>)}</details>}</div>}{setup.error && <p role="alert" className="error-text">{setup.error instanceof Error ? setup.error.message : 'Deployment discovery unavailable.'}</p>}{releases.isLoading && <p role="status">Checking releases…</p>}{(error || releases.error) && <p role="alert" className="error-text">{error || (releases.error instanceof Error ? releases.error.message : 'Releases unavailable.')}</p>}{releases.data && !items.length && <p>Operator: connect the repository’s CI evidence reporter to its approved deployment path.</p>}{items.map(item => <div className="row-actions" key={item.id}><span>{item.provenance.document.environment} · {new Date(item.provenance.document.issued_at).toLocaleString()}</span><Button disabled={!canWrite || !csrf || !!busy} onClick={() => void deploy(item.id)}>{busy === item.id ? 'Starting…' : item.mode === 'observe' ? 'Track release' : 'Deploy this fix'}</Button><details><summary>Release evidence</summary><code>{item.provenance.document.source_sha}</code> · {item.provenance.document.build_id}</details></div>)}{releases.hasNextPage && <Button disabled={releases.isFetchingNextPage} onClick={() => void releases.fetchNextPage()}>Load more releases</Button>}</section>
}
