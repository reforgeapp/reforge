import { useEffect, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { gitopsAPI, type Configuration, type Detail, type Gate, type PreviewRequest, type Promotion } from '../gitops-api'
import type { Gate as MergeGate } from '../merge-api'
import { useSession } from './query'

const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const tone = (state: string) => ['healthy', 'recovered', 'allow'].includes(state) ? 'green' as const : ['failed', 'cancelled', 'blocked'].includes(state) ? 'red' as const : 'amber' as const
const safeURL = (value?: string) => { try { const url = new URL(value ?? ''); return /^https?:$/.test(url.protocol) && !url.username && !url.password ? url.toString() : undefined } catch { return undefined } }
const expired = (value: string) => !Number.isFinite(Date.parse(value)) || Date.parse(value) <= Date.now()
const active = (p?: Promotion) => !!p && !p.finished_at && !['blocked', 'cancelled', 'healthy', 'recovered', 'failed', 'recovery_failed'].includes(p.state)
const repoName = (repo: { id?: string; name?: string; full_name?: string }) => repo.full_name ?? repo.name ?? repo.id ?? ''

export function GitOpsPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const canWrite = role === 'owner' || role === 'admin'
  const canPreview = canWrite || role === 'maintainer'
  const canPromote = canPreview
  const configurations = useQuery({ queryKey: ['org', orgID, 'gitops-configurations'], queryFn: ({ signal }) => gitopsAPI.configurations(orgID, signal) })
  const repositories = useInfiniteQuery({ queryKey: ['org', orgID, 'gitops-repositories'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { cursor: pageParam, limit: 50, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const promotions = useInfiniteQuery({ queryKey: ['org', orgID, 'gitops-promotions'], queryFn: ({ pageParam, signal }) => gitopsAPI.promotions(orgID, { cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, refetchInterval: 5000 })
  const [environment, setEnvironment] = useState<string | null>(null)
  const [draftConfig, setDraftConfig] = useState<Configuration>()
  const [newEnvironment, setNewEnvironment] = useState('')
  const [detailID, setDetailID] = useState('')
  const config = draftConfig ?? (environment === null ? configurations.data?.items?.[0] : configurations.data?.items.find(item => item.environment === environment))
  const detail = useQuery({ queryKey: ['org', orgID, 'gitops-promotion', detailID], queryFn: ({ signal }) => gitopsAPI.promotion(orgID, detailID, signal), enabled: !!detailID, refetchInterval: query => active(query.state.data?.promotion) ? 5000 : false })
  const repos = repositories.data?.pages.flatMap(page => page.items) ?? []
  const requested = (p: Promotion) => { setDetailID(p.id); void promotions.refetch() }
  const saved = (c: Configuration) => { setDraftConfig(undefined); setEnvironment(c.environment); setNewEnvironment(''); void configurations.refetch() }
  const rows = promotions.data?.pages.flatMap(page => page.items) ?? []
  if (configurations.isLoading) return <StatePanel kind="loading" title="Loading GitOps controls" detail="Fetching promotion configuration and history." />
  if (configurations.error || repositories.error) return <StatePanel kind="error" title="GitOps controls unavailable" detail={text(configurations.error ?? repositories.error)} action={<Button onClick={() => { void configurations.refetch(); void repositories.refetch() }}>Retry</Button>} />
  return <div className="stack gitops-page">
    <section className="state-card" aria-label="GitOps configuration">
      <div className="subsection-actions"><h2>Configuration</h2><label>Environment<select value={config?.environment ?? ""} onChange={event => { setDraftConfig(undefined); setEnvironment(event.target.value) }}>{draftConfig && <option value={draftConfig.environment}>{draftConfig.environment} (new)</option>}{(configurations.data?.items ?? []).map(item => <option key={item.environment} value={item.environment}>{item.environment}</option>)}</select></label></div>
      <div className="row-actions"><label>New environment<input value={newEnvironment} onChange={event => setNewEnvironment(event.target.value)} placeholder="production" /></label><Button disabled={!canWrite || !newEnvironment.trim()} onClick={() => { const name = newEnvironment.trim(); setEnvironment(name); setDraftConfig(configurations.data?.items.some(c => c.environment === name) ? undefined : emptyConfiguration(name)) }}>Create draft</Button></div>
      {config ? <ConfigurationEditor key={`${orgID}:${config.environment}:${config.version}`} orgID={orgID} config={config} repos={repos} csrf={csrf} canWrite={canWrite} canPreview={canPromote} onSaved={saved} onRequested={requested} /> : <EmptyTable label="Create an environment to configure GitOps." />}
      {repositories.hasNextPage && <Button disabled={repositories.isFetchingNextPage} onClick={() => void repositories.fetchNextPage()}>Load more repositories</Button>}
    </section>
    {promotions.isLoading && <p role="status">Loading promotions…</p>}
    {promotions.error && <p className="error-text" role="alert">{text(promotions.error)} <Button onClick={() => void promotions.refetch()}>Retry history</Button></p>}
    <PromotionList rows={rows} complete={promotions.data?.pages.at(-1)?.complete ?? true} loading={promotions.isFetchingNextPage} onMore={() => void promotions.fetchNextPage()} onSelect={setDetailID} />
    {detailID && <PromotionDetail key={`${orgID}:${detailID}`} orgID={orgID} detail={detail.data} loading={detail.isLoading} error={detail.error} csrf={csrf} canWrite={canPromote} onChanged={() => { void detail.refetch(); void promotions.refetch() }} />}
  </div>
}

const emptyConfiguration = (environment: string): Configuration => ({ environment, source_repository_id: '', delivery_repository_id: '', version: 0, enabled: false, target_branch: 'main', manifest_path: 'manifests/app.yaml', pointer: '/spec/template/spec/containers/0/image', image_repository: '', provenance_public_key: '', health_public_key: '', health_checks: ['smoke'], observation_seconds: 60, max_evidence_age_seconds: 300, deadline_seconds: 1800, recovery_allowed: false })

function ConfigurationEditor({ orgID, config, repos, csrf, canWrite, canPreview, onSaved, onRequested }: { orgID: string; config: Configuration; repos: Array<{ id?: string; name?: string; full_name?: string }>; csrf: string; canWrite: boolean; canPreview: boolean; onSaved: (c: Configuration) => void; onRequested: (p: Promotion) => void }) {
  const [draft, setDraft] = useState(config)
  const [sourceSHA, setSourceSHA] = useState('')
  const [changeID, setChangeID] = useState('')
  const [artifact, setArtifact] = useState('')
  const [provenance, setProvenance] = useState('{}')
  const [recoveryOf, setRecoveryOf] = useState('')
  const [restorePromotionID, setRestorePromotionID] = useState('')
  const [gate, setGate] = useState<Gate>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [key, setKey] = useState(() => crypto.randomUUID())
  const dirty = JSON.stringify(draft) !== JSON.stringify(config)

  useEffect(() => { setDraft(config); setGate(undefined); setKey(crypto.randomUUID()) }, [config])
  const update = <K extends keyof Configuration>(key: K, value: Configuration[K]) => { setDraft(previous => ({ ...previous, [key]: value })); setGate(undefined); setKey(crypto.randomUUID()) }
  const save = async () => { setBusy(true); setError(''); try { const saved = await gitopsAPI.saveConfiguration(orgID, draft, csrf); setDraft(saved); onSaved(saved) } catch (reason) { setError(text(reason)) } finally { setBusy(false) } }
  const preview = async () => { setBusy(true); setError(''); try { const request: PreviewRequest = { change_id: changeID.trim(), source_sha: sourceSHA.trim(), artifact_digest: artifact.trim(), provenance: JSON.parse(provenance), ...(recoveryOf ? { recovery_of: recoveryOf.trim(), restore_promotion_id: restorePromotionID.trim() } : {}) }; setGate(await gitopsAPI.preview(orgID, draft.environment, request, csrf)); setKey(crypto.randomUUID()) } catch (reason) { setError(reason instanceof SyntaxError ? 'Provenance must be valid JSON.' : text(reason)) } finally { setBusy(false) } }

  return <div className="stack">
    <div className="form-grid">
      <label>Source repository<select disabled={!canWrite || busy} value={draft.source_repository_id} onChange={event => update('source_repository_id', event.target.value)}><option value="">Choose repository</option>{repos.map(repo => <option key={repo.id} value={repo.id}>{repoName(repo)}</option>)}</select></label>
      <label>Delivery repository<select disabled={!canWrite || busy} value={draft.delivery_repository_id} onChange={event => update('delivery_repository_id', event.target.value)}><option value="">Choose repository</option>{repos.map(repo => <option key={repo.id} value={repo.id}>{repoName(repo)}</option>)}</select></label>
      <label>Target branch<input disabled={!canWrite || busy} value={draft.target_branch} onChange={event => update('target_branch', event.target.value)} /></label>
      <label>Manifest path<input disabled={!canWrite || busy} value={draft.manifest_path} onChange={event => update('manifest_path', event.target.value)} /></label>
      <label>Pointer<input disabled={!canWrite || busy} value={draft.pointer} onChange={event => update('pointer', event.target.value)} /></label>
      <label>Image repository<input disabled={!canWrite || busy} value={draft.image_repository} onChange={event => update('image_repository', event.target.value)} /></label>
      <label>Health checks<input disabled={!canWrite || busy} value={draft.health_checks.join(', ')} onChange={event => update('health_checks', event.target.value.split(',').map(item => item.trim()).filter(Boolean))} /></label>
      <label>Provenance public key<input disabled={!canWrite || busy} value={draft.provenance_public_key} onChange={event => update('provenance_public_key', event.target.value)} /></label>
      <label>Health public key<input disabled={!canWrite || busy} value={draft.health_public_key} onChange={event => update('health_public_key', event.target.value)} /></label>
      <label>Observation seconds<input type="number" disabled={!canWrite || busy} value={draft.observation_seconds} onChange={event => update('observation_seconds', Number(event.target.value))} /></label>
      <label>Evidence age seconds<input type="number" disabled={!canWrite || busy} value={draft.max_evidence_age_seconds} onChange={event => update('max_evidence_age_seconds', Number(event.target.value))} /></label>
      <label>Deadline seconds<input type="number" disabled={!canWrite || busy} value={draft.deadline_seconds} onChange={event => update('deadline_seconds', Number(event.target.value))} /></label>
      <label className="checkbox-label"><input type="checkbox" disabled={!canWrite || busy} checked={draft.recovery_allowed} onChange={event => update('recovery_allowed', event.target.checked)} /> Recovery allowed</label>
      <label className="checkbox-label"><input type="checkbox" disabled={!canWrite || busy} checked={draft.enabled} onChange={event => update('enabled', event.target.checked)} /> Enabled</label>
    </div>
    <div className="row-actions"><Button className="button button-primary" disabled={!canWrite || !csrf || busy} onClick={() => void save()}>{busy ? 'Saving…' : 'Save configuration'}</Button></div>
    <fieldset>
      <legend>Promotion</legend>
      {(dirty || config.version < 1) && <p>Save configuration before previewing.</p>}
      <div className="form-grid">
        <label>Change ID<input value={changeID} disabled={!canPreview || busy} onChange={event => { setChangeID(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
        <label>Source SHA<input value={sourceSHA} disabled={!canPreview || busy} onChange={event => { setSourceSHA(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
        <label>Artifact digest<input value={artifact} disabled={!canPreview || busy} onChange={event => { setArtifact(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
        <label>Recovery promotion ID<input value={recoveryOf} disabled={!canPreview || busy} onChange={event => { setRecoveryOf(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
        <label>Restore known healthy promotion<input value={restorePromotionID} disabled={!canPreview || busy} onChange={event => { setRestorePromotionID(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
        <label className="wide">Signed provenance JSON<textarea value={provenance} disabled={!canPreview || busy} onChange={event => { setProvenance(event.target.value); setGate(undefined); setKey(crypto.randomUUID()) }} /></label>
      </div>
      <Button className="button button-primary" disabled={!canPreview || !csrf || busy || dirty || config.version < 1 || !changeID || !sourceSHA || !artifact} onClick={() => void preview()}>{busy ? 'Checking…' : 'Preview promotion'}</Button>
    </fieldset>
    {error && <p className="error-text" role="alert">{error}</p>}
    {gate && <GateCard orgID={orgID} gate={gate} csrf={csrf} canWrite={canPreview} idempotencyKey={key} onRequested={onRequested} />}
  </div>
}

function GateCard({ orgID, gate, csrf, canWrite, idempotencyKey, onRequested }: { orgID: string; gate: Gate; csrf: string; canWrite: boolean; idempotencyKey: string; onRequested: (p: Promotion) => void }) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [, tick] = useState(0)
  useEffect(() => { const timer = window.setInterval(() => tick(n => n + 1), 1000); return () => window.clearInterval(timer) }, [])
  const stale = expired(gate.expires_at)
  const request = async () => { setBusy(true); setError(''); try { if (expired(gate.expires_at)) throw new Error("Preview expired. Run a new preview."); const result = await gitopsAPI.request(orgID, gate.id, idempotencyKey, csrf); onRequested(result) } catch (reason) { setError(`Promotion outcome unknown: ${text(reason)}. Retry uses the same request identity.`) } finally { setBusy(false) } }

  return <div className="detail-section">
    <p><StatusBadge label={gate.decision.outcome} tone={tone(gate.decision.outcome)} /> · expires {new Date(gate.expires_at).toLocaleString()}</p>
    <dl className="detail-list">
      <div><dt>Source</dt><dd><code>{gate.request.source_sha}</code></dd></div>
      <div><dt>Delivery target</dt><dd><code>{gate.before}</code> → <code>{gate.after}</code></dd></div>
      <div><dt>Manifest</dt><dd><code>{gate.configuration.manifest_path}</code> · {gate.configuration.pointer}</dd></div>
    </dl>
    {gate.blockers.length > 0 && <p className="error-text">Blocked: {gate.blockers.join('; ')}</p>}
    {stale && <p className="error-text">Preview expired. Run a new preview.</p>}
    <Button disabled={!canWrite || !csrf || busy || stale || gate.decision.outcome !== 'allow' || gate.blockers.length > 0} onClick={() => void request()}>{busy ? 'Requesting…' : 'Request promotion'}</Button>
    {error && <p className="error-text" role="alert">{error}</p>}
  </div>
}

function PromotionList({ rows, complete, loading, onMore, onSelect }: { rows: Promotion[]; complete: boolean; loading: boolean; onMore: () => void; onSelect: (id: string) => void }) {
  return <section className="state-card">
    <h2>Promotion history</h2>
    <DataTable caption="GitOps promotions">
      <table>
        <thead><tr><th>Promotion</th><th>Delivery candidate</th><th>Delivery revision</th><th>State</th></tr></thead>
        <tbody>{rows.map(row => <tr key={row.id}><td><button className="link-button" onClick={() => onSelect(row.id)}>{row.id}</button>{safeURL(row.change?.url) && <a href={safeURL(row.change?.url)} target="_blank" rel="noreferrer">PR</a>}</td><td><code>{row.candidate_sha || 'Pending'}</code></td><td><code>{row.merge_sha || 'Pending'}</code></td><td><StatusBadge label={row.state} tone={tone(row.state)} /></td></tr>)}</tbody>
      </table>
      {!rows.length && <EmptyTable label="No GitOps promotions recorded." />}
    </DataTable>
    {!complete && <Button disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Load more promotions'}</Button>}
  </section>
}

function PromotionDetail({ orgID, detail, loading, error, csrf, canWrite, onChanged }: { orgID: string; detail?: Detail; loading: boolean; error: unknown; csrf: string; canWrite: boolean; onChanged: () => void }) {
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [method, setMethod] = useState('fast-forward-only')
  const [mergeGate, setMergeGate] = useState<MergeGate>()
  const [mergeKey, setMergeKey] = useState(() => crypto.randomUUID())
  const [, tick] = useState(0)
  useEffect(() => { const timer = window.setInterval(() => tick(n => n + 1), 1000); return () => window.clearInterval(timer) }, [])
  if (loading) return <StatePanel kind="loading" title="Loading promotion" detail="Fetching canonical delivery and health state." />
  if (error || !detail) return <StatePanel kind="error" title="Promotion unavailable" detail={text(error)} action={<Button onClick={onChanged}>Retry</Button>} />
  const p = detail.promotion
  const live = active(p)
  const canMerge = live && p.state === 'awaiting_merge' && !p.cancel_requested
  const disabled = !canWrite || !csrf || busy
  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true); setMessage('')
    try { await fn(); onChanged() } catch (reason) { setMessage(text(reason)) } finally { setBusy(false) }
  }
  const previewMerge = async () => {
    setMergeGate(undefined)
    await act(async () => { setMergeGate(await gitopsAPI.mergePreview(orgID, p.id, method, csrf)); setMergeKey(crypto.randomUUID()) })
  }
  const requestMerge = async () => {
    if (!mergeGate || expired(mergeGate.expires_at)) { setMessage('Merge preview expired. Refresh it.'); return }
    await act(async () => { const result = await gitopsAPI.merge(orgID, p.id, mergeGate.id, mergeKey, csrf); setMessage(`Merge ${result.state}: ${result.reason}`) })
  }
  const mergeBlockers = mergeGate?.decision.blockers ?? []
  return <section className="state-card" aria-label={`Promotion ${p.id}`}>
    <h2>Promotion {p.id}</h2>
    <p><StatusBadge label={p.state} tone={tone(p.state)} /> {p.reason}</p>
    <dl className="detail-list">
      <div><dt>Source revision</dt><dd><code>{detail.gate.request.source_sha}</code></dd></div>
      <div><dt>Artifact</dt><dd><code>{detail.gate.request.artifact_digest}</code></dd></div>
      <div><dt>Delivery candidate</dt><dd><code>{p.candidate_sha || 'Pending'}</code></dd></div>
      <div><dt>Delivery revision</dt><dd><code>{p.merge_sha || 'Pending'}</code></dd></div>
      <div><dt>Health</dt><dd>{detail.health ? `${detail.health.healthy ? 'Healthy observation' : 'Unhealthy observation'} · ${detail.health.observed_at}` : 'No authenticated health report'}</dd></div>
    </dl>
    {safeURL(p.change?.url) && <a href={safeURL(p.change?.url)} target="_blank" rel="noreferrer">Open native delivery change</a>}
    <div className="row-actions">
      <Button disabled={disabled || !['requested', 'staged'].includes(p.state) || p.cancel_requested} onClick={() => void act(() => gitopsAPI.continue(orgID, p.id, csrf))}>Continue</Button>
      <Button disabled={disabled || !live} onClick={() => void act(() => gitopsAPI.observe(orgID, p.id, csrf))}>Observe</Button>
      <Button disabled={disabled || !live || p.cancel_requested} onClick={() => void act(() => gitopsAPI.cancel(orgID, p.id, p.version, csrf))}>Cancel</Button>
      <label>Merge method<select value={method} disabled={disabled || !canMerge} onChange={event => { setMethod(event.target.value); setMergeGate(undefined) }}>
        {['fast-forward-only', 'merge', 'squash', 'rebase'].map(value => <option key={value} value={value}>{value}</option>)}
      </select></label>
      <Button disabled={disabled || !canMerge} onClick={() => void previewMerge()}>Protected merge preview</Button>
    </div>
    {mergeGate && <div className="detail-section">
      <p><StatusBadge label={mergeGate.decision.outcome} tone={tone(mergeGate.decision.outcome)} /> {mergeBlockers.join('; ')}</p>
      {expired(mergeGate.expires_at) && <p>Merge preview expired. Refresh it.</p>}
      <Button disabled={disabled || !canMerge || expired(mergeGate.expires_at) || mergeGate.decision.outcome !== 'allow' || mergeBlockers.length > 0} onClick={() => void requestMerge()}>Request protected merge</Button>
    </div>}
    {message && <p role="status">{message}</p>}
  </section>
}
