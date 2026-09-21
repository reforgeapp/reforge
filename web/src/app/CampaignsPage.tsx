import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { campaignAPI, type Campaign, type Input, type Member, type Preview } from '../campaign-api'
import { ReforgeAPIError } from '../api/client'
import { CampaignCreateForm } from './CampaignCreateForm'
import { useSession } from './query'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const tone = (state: string) => ['completed', 'healthy', 'succeeded', 'merged', 'published'].includes(state) ? 'green' as const : ['failed', 'blocked', 'cancelled'].includes(state) ? 'red' as const : 'amber' as const
const stale = (value: string) => !Number.isFinite(Date.parse(value)) || Date.parse(value) <= Date.now()
const stateLabel = (state: string) => state === 'completed' ? 'Completed' : state === 'unknown' ? 'Unknown' : state

export function CampaignsPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const canMutate = role === 'owner' || role === 'admin' || role === 'maintainer'
  const [state, setState] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [preview, setPreview] = useState<Preview>()
  const [createKey, setCreateKey] = useState(() => crypto.randomUUID())
  const previewGeneration = useRef(0)
  const [busy, setBusy] = useState(false)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [error, setError] = useState('')
  const campaigns = useInfiniteQuery({ queryKey: ['org', orgID, 'campaigns', state], queryFn: ({ pageParam, signal }) => campaignAPI.list(orgID, pageParam, state || undefined, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const rows = campaigns.data?.pages.flatMap(page => page.items) ?? []
  const detail = useQuery({ queryKey: ['org', orgID, 'campaign', selectedID], queryFn: ({ signal }) => campaignAPI.get(orgID, selectedID, signal), enabled: !!selectedID, refetchInterval: query => query.state.data && ['planned', 'canary', 'observing', 'expanding', 'paused', 'cancelled'].includes(query.state.data.state) ? 5000 : false })
  const resetPreview = () => { previewGeneration.current += 1; setPreview(undefined); setCreateKey(crypto.randomUUID()); setPreviewBusy(false) }
  const previewInput = async (input: Input) => { const generation = ++previewGeneration.current; setCreateKey(crypto.randomUUID()); setPreviewBusy(true); setError(''); setPreview(undefined); try { const value = await campaignAPI.preview(orgID, input, csrf); if (generation === previewGeneration.current) setPreview(value) } catch (reason) { if (generation === previewGeneration.current) setError(errorText(reason)) } finally { if (generation === previewGeneration.current) setPreviewBusy(false) } }
  const create = async () => { if (!preview || stale(preview.expires_at)) { setError('Preview expired. Create a new preview.'); return } setBusy(true); setError(''); try { const value = await campaignAPI.create(orgID, preview.id, createKey, csrf); setSelectedID(value.id); setPreview(undefined); void campaigns.refetch() } catch (reason) { const known = reason instanceof ReforgeAPIError && reason.status < 500; setError(`${known ? 'Campaign was not created' : 'Campaign creation outcome unknown'}: ${errorText(reason)}${known ? '' : '. Retry keeps the same request identity.'}`) } finally { setBusy(false) } }
  const control = async (action: 'start' | 'pause' | 'resume' | 'cancel', value: Campaign, reason: string) => { setBusy(true); setError(''); try { await campaignAPI.control(orgID, value, action, reason, csrf); await Promise.all([detail.refetch(), campaigns.refetch()]) } catch (reasonValue) { if (reasonValue instanceof ReforgeAPIError && reasonValue.status === 409) { await Promise.all([detail.refetch(), campaigns.refetch()]); setError('Campaign changed on the server; current state was refreshed. Review it before retrying.') } else setError(errorText(reasonValue)) } finally { setBusy(false) } }
  if (campaigns.isLoading) return <StatePanel kind="loading" title="Loading campaigns" detail="Fetching persisted campaign snapshots and lifecycle state." />
  if (campaigns.error) return <StatePanel kind="error" title="Campaigns unavailable" detail={errorText(campaigns.error)} action={<Button onClick={() => void campaigns.refetch()}>Retry</Button>} />
  return <div className="stack campaigns-page">
    <section className="state-card" aria-label="Campaign planning">
      <div className="subsection-actions"><label>State<select aria-label="Campaign state" value={state} onChange={event => { setState(event.target.value); setSelectedID('') }}><option value="">All states</option><option value="planned">Planned</option><option value="canary">Canary</option><option value="observing">Observing</option><option value="expanding">Expanding</option><option value="paused">Paused</option><option value="failed">Failed</option><option value="completed">Completed</option></select></label></div>
      {canMutate && <CampaignCreateForm orgID={orgID} onPreview={previewInput} onChange={resetPreview} busy={busy} pending={previewBusy} />}
      {preview && <PreviewCard preview={preview} canCreate={canMutate} csrf={csrf} busy={busy} onCreate={() => void create()} />}
      {error && <p className="error-text" role="alert">{error}</p>}
    </section>
    <CampaignList rows={rows} complete={campaigns.data?.pages.at(-1)?.complete ?? true} loading={campaigns.isFetchingNextPage} onMore={() => void campaigns.fetchNextPage()} onSelect={setSelectedID} />
    {detail.error && <p className="error-text" role="alert">Campaign detail unavailable: {errorText(detail.error)} <Button onClick={() => void detail.refetch()}>Retry</Button></p>}
    {detail.data && <CampaignDetail orgID={orgID} campaign={detail.data} csrf={csrf} canMutate={canMutate} busy={busy} onControl={control} />}
  </div>
}

function PreviewCard({ preview, canCreate, csrf, busy, onCreate }: { preview: Preview; canCreate: boolean; csrf: string; busy: boolean; onCreate: () => void }) {
  const [, tick] = useState(0)
  useEffect(() => { const timer = window.setInterval(() => tick(value => value + 1), 1000); return () => window.clearInterval(timer) }, [])
  const expired = stale(preview.expires_at)
  const members = preview.members ?? []
  const blockers = preview.blockers ?? []
  const excluded = members.filter(member => member.state === 'excluded')
  const canaries = members.filter(member => member.canary)
  return <section className="detail-section" aria-label="Campaign preview"><p><StatusBadge label={expired ? 'expired' : 'preview'} tone={expired ? 'red' : 'amber'} /> · {members.length} pinned members · expires {new Date(preview.expires_at).toLocaleString()}</p><dl className="detail-list"><div><dt>Preview hash</dt><dd><code>{preview.hash}</code></dd></div><div><dt>Canaries</dt><dd>{canaries.length} · {canaries.map(member => member.repository_name || member.repository_id).join(', ') || 'None selected'}</dd></div><div><dt>Exclusions</dt><dd>{excluded.length ? excluded.map(member => `${member.repository_name || member.repository_id}: ${member.reason}`).join('; ') : 'None'}</dd></div><div><dt>Blockers</dt><dd>{blockers.join('; ') || 'None'}</dd></div></dl><details><summary>Exact preview members</summary><ul className="compact-list">{members.map(member => <li key={member.id}>{member.repository_name || member.repository_id} · {member.state}{member.reason ? ` · ${member.reason}` : ''}</li>)}</ul></details><Button disabled={!canCreate || !csrf || busy || expired || blockers.length > 0} onClick={onCreate}>{busy ? 'Creating…' : 'Create planned campaign'}</Button></section>
}

function CampaignList({ rows, complete, loading, onMore, onSelect }: { rows: Campaign[]; complete: boolean; loading: boolean; onMore: () => void; onSelect: (id: string) => void }) {
  return <section className="state-card"><DataTable caption="Campaign history"><table><thead><tr><th>Campaign</th><th>Kind</th><th>State</th><th>Progress</th><th>Updated</th></tr></thead><tbody>{rows.map(row => <tr key={row.id}><td><button className="link-button" onClick={() => onSelect(row.id)}>{row.name || row.id}</button></td><td>{row.kind}</td><td><StatusBadge label={stateLabel(row.state)} tone={tone(row.state)} /></td><td>{row.counts.succeeded}/{row.counts.total} succeeded · {row.counts.unknown} unknown</td><td>{new Date(row.updated_at).toLocaleString()}</td></tr>)}</tbody></table>{!rows.length && <EmptyTable label="No campaigns recorded." />}</DataTable>{!complete && <Button disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Load more campaigns'}</Button>}</section>
}

function CampaignDetail({ orgID, campaign, csrf, canMutate, busy, onControl }: { orgID: string; campaign: Campaign; csrf: string; canMutate: boolean; busy: boolean; onControl: (action: 'start' | 'pause' | 'resume' | 'cancel', value: Campaign, reason: string) => void }) {
  const [reason, setReason] = useState('')
  const members = useInfiniteQuery({ queryKey: ['org', orgID, 'campaign-members', campaign.id], queryFn: ({ pageParam, signal }) => campaignAPI.members(orgID, campaign.id, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, refetchInterval: ['planned', 'canary', 'observing', 'expanding', 'paused'].includes(campaign.state) ? 5000 : false })
  const items = members.data?.pages.flatMap(page => page.items) ?? []
  const terminal = ['completed', 'failed', 'cancelled'].includes(campaign.state)
  return <section className="state-card" aria-label={`Campaign ${campaign.id}`}><h2>{campaign.name || campaign.id}</h2><p><StatusBadge label={stateLabel(campaign.state)} tone={tone(campaign.state)} /> · stage {campaign.stage} · {campaign.reason || 'No reason recorded'}</p><dl className="detail-list"><div><dt>Snapshot</dt><dd>{campaign.counts.total} members · {campaign.counts.excluded} excluded · {campaign.counts.pending} pending · {campaign.counts.running} running</dd></div><div><dt>Outcomes</dt><dd>{campaign.counts.succeeded} succeeded · {campaign.counts.failed} failed · {campaign.counts.unknown} unknown</dd></div><div><dt>Success criterion</dt><dd>{campaign.spec.success}; observation {campaign.spec.observation_seconds}s; thresholds {campaign.spec.failure_limit} failures / {campaign.spec.failure_percent}%</dd></div><div><dt>Approval expiry</dt><dd>{new Date(campaign.grant_expires_at).toLocaleString()}</dd></div><div><dt>Budget</dt><dd><a href={`/org/${encodeURIComponent(orgID)}/usage?scope_kind=campaign&scope_id=${encodeURIComponent(campaign.id)}`}>Open campaign budget</a></dd></div></dl><details><summary>Pinned campaign inputs</summary><pre>{JSON.stringify(campaign.spec, null, 2)}</pre></details>{!terminal && <label>Action reason<input aria-label="Campaign action reason" value={reason} onChange={event => setReason(event.target.value)} /></label>}<div className="row-actions">{campaign.state === 'planned' && <Button disabled={!canMutate || !csrf || busy || !reason.trim()} onClick={() => onControl('start', campaign, reason.trim())}>Start canary</Button>}{['canary', 'observing', 'expanding'].includes(campaign.state) && <Button disabled={!canMutate || !csrf || busy || !reason.trim()} onClick={() => onControl('pause', campaign, reason.trim())}>Pause</Button>}{campaign.state === 'paused' && <Button disabled={!canMutate || !csrf || busy || !reason.trim()} onClick={() => onControl('resume', campaign, reason.trim())}>Resume current stage</Button>}{!terminal && <Button disabled={!canMutate || !csrf || busy || !reason.trim()} onClick={() => onControl('cancel', campaign, reason.trim())}>Cancel</Button>}</div>{members.error && <p className="error-text" role="alert">Members unavailable: {errorText(members.error)} <Button onClick={() => void members.refetch()}>Retry members</Button></p>}<MemberList orgID={orgID} kind={campaign.kind} items={items} complete={members.data?.pages.at(-1)?.complete ?? true} loading={members.isFetchingNextPage} onMore={() => void members.fetchNextPage()} /></section>
}

function MemberList({ orgID, kind, items, complete, loading, onMore }: { orgID: string; kind: string; items: Member[]; complete: boolean; loading: boolean; onMore: () => void }) {
  return <div className="detail-section"><h3>Snapshot members</h3><DataTable caption="Campaign members"><table><thead><tr><th>Repository</th><th>Group</th><th>State</th><th>Evidence</th></tr></thead><tbody>{items.map(member => <tr key={member.id}><td>{member.repository_name || member.repository_id}{member.canary && ' · canary'}</td><td>{member.group || '—'}</td><td><StatusBadge label={stateLabel(member.state)} tone={tone(member.state)} /></td><td>{member.reason || 'No evidence yet'}{member.action_id && kind === 'repair' && <span> · <a href={`/org/${encodeURIComponent(orgID)}/runs?run=${encodeURIComponent(member.action_id)}`}>Run {member.action_id}</a></span>}{member.action_id && kind !== 'repair' && member.input.environment && <span> · <a href={`/org/${encodeURIComponent(orgID)}/deployments?repository=${encodeURIComponent(member.repository_id)}&environment=${encodeURIComponent(member.input.environment)}&${kind === 'gitops' ? `promotion=${encodeURIComponent(member.action_id)}` : `deployment=${encodeURIComponent(member.action_id)}`}`}>{kind === 'gitops' ? `GitOps promotion ${member.action_id}` : `Pipeline deployment ${member.action_id}`}</a></span>}</td></tr>)}</tbody></table>{!items.length && <EmptyTable label="No members in this snapshot." />}</DataTable>{!complete && <Button disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Load more members'}</Button>}</div>
}
