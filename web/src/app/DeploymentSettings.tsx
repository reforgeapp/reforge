import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api, apiRequest } from '../api/client'
import { inventoryAPI } from '../api/inventory'
import { Button } from '../components/Accessible'
import { StatePanel } from '../components/StatePanel'
import { useSession } from './query'
import { type Configuration } from '../deployment-api'

type WorkflowOption = { id: string; name: string; path: string; ref: string; url: string }
type Workflow = Configuration['workflow']
type Draft = Configuration & { native_environment?: string }

const path = (orgID: string, suffix: string) => `/api/v1/orgs/${encodeURIComponent(orgID)}${suffix}`
const tag = (version: number) => `"${version}"`
const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const dateInput = (value: string) => { if (!value) return ''; const date = new Date(value); return Number.isNaN(date.valueOf()) ? '' : `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}T${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}` }
const dateValue = (value: string) => value ? new Date(value).toISOString() : ''
const emptyWorkflow = (): Workflow => ({ id: '', path: '', ref: '', sha: '', config_sha256: '', inputs: {} })
const emptyDraft = (repositoryID: string, environment: string): Draft => ({ environment, native_environment: '', repository_id: repositoryID, version: 0, enabled: false, mode: 'observe', workflow: emptyWorkflow(), provenance_public_key: '', health_public_key: '', health_checks: [], observation_seconds: 60, max_evidence_age_seconds: 300, deadline_seconds: 900, qualification: { provider: '', server_version: '', connection_version: 0, evidence_reference: '', evidence_sha256: '', verified_at: '', expires_at: '', pinned_inputs: false, native_enforcement: false, environment_serialization: false, no_bypass: false } })

export function DeploymentSettings({ orgID, onSaved }: { orgID: string; onSaved: () => void }) {
  const session = useSession()
  const canWrite = session.data?.memberships.some(item => item.org_id === orgID && ['owner', 'admin'].includes(item.role)) ?? false
  const csrf = session.data?.csrf_token ?? ''
  const configurations = useQuery({ queryKey: ['org', orgID, 'deployment-configurations'], queryFn: ({ signal }) => apiRequest<{ items: Configuration[] }>(path(orgID, '/deployment-configurations'), { signal }) })
  const repositories = useInfiniteQuery({ queryKey: ['org', orgID, 'deployment-settings-repositories'], queryFn: ({ pageParam, signal }) => inventoryAPI.repositories(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const [repositoryID, setRepositoryID] = useState('')
  const [environment, setEnvironment] = useState<string | null>(null)
  const [draft, setDraft] = useState<Draft>()
  const [fixedInputs, setFixedInputs] = useState('{}')
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const [recoveryEnabled, setRecoveryEnabled] = useState(false)

  const items = configurations.data?.items ?? []
  const environments = useMemo(() => [...new Set(items.filter(item => !repositoryID || item.repository_id === repositoryID).map(item => item.environment))].sort(), [items, repositoryID])
  const workflows = useQuery({ queryKey: ['org', orgID, 'delivery-workflows', repositoryID], queryFn: ({ signal }) => apiRequest<{ items: WorkflowOption[] }>(path(orgID, `/repositories/${encodeURIComponent(repositoryID)}/delivery-workflows`), { signal }), enabled: !!repositoryID })
  const connections = useInfiniteQuery({ queryKey: ['org', orgID, 'deployment-settings-connections'], queryFn: ({ pageParam, signal }) => api.getConnections(orgID, { kind: 'forge', limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })

  const repositoryItems = repositories.data?.pages.flatMap(page => page.items) ?? []
  const connectionItems = connections.data?.pages.flatMap(page => page.items) ?? []

  useEffect(() => {
    if (!repositoryID && repositoryItems[0]) setRepositoryID(repositoryItems[0].id)
  }, [repositoryItems, repositoryID])
  useEffect(() => {
    if (environment === null) setEnvironment(environments[0] ?? '')
  }, [environment, environments])
  useEffect(() => {
    if (repositoryID && environment === '') setDraft(emptyDraft(repositoryID, ''))
  }, [repositoryID, environment])
  useEffect(() => {
    if (dirty || !repositoryID || !environment) return
    const current = items.find(item => item.repository_id === repositoryID && item.environment === environment)
    if (draft && draft.repository_id === repositoryID && draft.environment === environment && draft.version >= (current?.version ?? 0)) return
    const next = current ? structuredClone(current) as Draft : emptyDraft(repositoryID, environment)
    setDraft(next); setFixedInputs(JSON.stringify(next.workflow.inputs ?? {}, null, 2)); setDirty(false)
    setRecoveryEnabled(!!current?.recovery_workflow)
    setError(''); setSaved(false)
  }, [items, repositoryID, environment, dirty, draft])

  const update = (change: Partial<Draft>) => { setDraft(current => current ? { ...current, ...change } : current); setDirty(true); setSaved(false) }
  const updateWorkflow = (change: Partial<Workflow>, recovery = false) => { if (!draft) return; const key = recovery ? 'recovery_workflow' : 'workflow'; const current = recovery ? draft.recovery_workflow ?? emptyWorkflow() : draft.workflow; update({ [key]: { ...current, ...change } } as Partial<Draft>) }
  const updateQualification = (change: Partial<Configuration['qualification']>) => update({ qualification: { ...draft!.qualification, ...change } })
  const chooseWorkflow = (value: string, recovery = false) => { const item = workflows.data?.items.find(workflow => workflow.id === value); if (item) updateWorkflow({ id: item.id, path: item.path, ref: item.ref && !item.ref.startsWith('refs/') ? `refs/heads/${item.ref}` : item.ref }, recovery) }
  const save = async () => {
    if (!draft || !csrf || !canWrite) return
    if (!draft.environment.trim() || !draft.repository_id || !draft.workflow.id || !draft.workflow.path || !draft.workflow.ref) { setError('Environment, repository, and primary workflow identity are required.'); return }
    let inputs: Record<string, string>
    try {
      inputs = JSON.parse(fixedInputs) as Record<string, string>
      if (!inputs || Array.isArray(inputs) || typeof inputs !== 'object' || Object.values(inputs).some(value => typeof value !== 'string')) throw new Error()
    } catch { setError('Fixed inputs must be a JSON object of string values.'); return }
    setBusy(true); setError('')
    try {
      const next = await apiRequest<Configuration>(path(orgID, `/deployment-configurations/${encodeURIComponent(draft.environment)}`), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': tag(draft.version) }, body: JSON.stringify({ ...draft, environment: draft.environment.trim(), workflow: { ...draft.workflow, inputs }, qualification: { ...draft.qualification, verified_at: draft.qualification.verified_at || '0001-01-01T00:00:00Z', expires_at: draft.qualification.expires_at || '0001-01-01T00:00:00Z' }, recovery_workflow: recoveryEnabled ? draft.recovery_workflow : undefined }) }, csrf)
      setDraft(next); setEnvironment(next.environment); setFixedInputs(JSON.stringify(next.workflow.inputs ?? {}, null, 2)); setDirty(false); setSaved(true); onSaved()
      void configurations.refetch()
    } catch (reason) { setError(text(reason)) } finally { setBusy(false) }
  }

  if (configurations.isLoading || repositories.isLoading || connections.isLoading) return <StatePanel kind="loading" title="Loading deployment settings" detail="Fetching repositories and delivery metadata." />
  if (configurations.error || repositories.error || connections.error) return <StatePanel kind="error" title="Deployment settings unavailable" detail={text(configurations.error ?? repositories.error ?? connections.error)} action={<Button onClick={() => { void configurations.refetch(); void repositories.refetch(); void connections.refetch() }}>Retry</Button>} />
  if (!repositoryItems.length) return <StatePanel kind="empty" title="No repositories" detail="Import a repository before configuring delivery." />
  if (!draft) return <StatePanel kind="loading" title="Loading settings" detail="Selecting deployment environment." />

  const primary = draft.workflow
  const recovery = draft.recovery_workflow ?? emptyWorkflow()
  return <section className="state-card" aria-label="Deployment settings">
    <div className="subsection-actions"><div><h2>Deployment settings</h2><p className="table-meta">Owner and administrator controls for native delivery.</p></div><label className="checkbox-label"><input type="checkbox" checked={draft.enabled} onChange={event => update({ enabled: event.target.checked })} disabled={!canWrite || busy} /> Enabled</label></div>
    <div className="form-grid"><label>Repository<select value={repositoryID} onChange={event => { setDirty(false); setRepositoryID(event.target.value); setEnvironment(null); setDraft(undefined) }} disabled={!canWrite || busy}>{repositoryItems.map(repo => <option key={repo.id} value={repo.id}>{repo.name}</option>)}</select></label><label>Environment<select value={environment !== null && environments.includes(environment) ? environment : ''} onChange={event => { setDirty(false); setEnvironment(event.target.value); if (!event.target.value) setDraft(emptyDraft(repositoryID, '')) }} disabled={!canWrite || busy}><option value="">New environment</option>{environments.map(value => <option key={value}>{value}</option>)}</select></label><label>Environment name<input value={draft.environment} onChange={event => { setEnvironment(event.target.value); update({ environment: event.target.value }) }} disabled={!canWrite || busy || draft.version > 0} /></label><label>Native environment<input value={draft.native_environment ?? ''} onChange={event => update({ native_environment: event.target.value })} disabled={!canWrite || busy} placeholder="production" /></label><label>Mode<select value={draft.mode} onChange={event => update({ mode: event.target.value })} disabled={!canWrite || busy}><option value="pipeline">Pipeline</option><option value="observe">Observe</option></select></label></div>{repositories.hasNextPage && <Button disabled={repositories.isFetchingNextPage} onClick={() => void repositories.fetchNextPage()}>{repositories.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}
    <details open><summary>Primary workflow</summary><div className="form-grid"><label>Known workflow<select value={primary.id} onChange={event => chooseWorkflow(event.target.value)} disabled={!canWrite || busy}><option value="">Choose a workflow</option>{(workflows.data?.items ?? []).map(item => <option key={item.id} value={item.id}>{item.name} · {item.path} · {item.ref || 'ref not reported'}</option>)}</select></label><label>Workflow ID<input value={primary.id} onChange={event => updateWorkflow({ id: event.target.value })} disabled={!canWrite || busy} /></label><label>Path<input value={primary.path} onChange={event => updateWorkflow({ path: event.target.value })} disabled={!canWrite || busy} /></label><label>Ref<input value={primary.ref} onChange={event => updateWorkflow({ ref: event.target.value })} disabled={!canWrite || busy} placeholder="refs/heads/main" /></label><label>Workflow SHA<input value={primary.sha} onChange={event => updateWorkflow({ sha: event.target.value })} disabled={!canWrite || busy} /></label><label>Config SHA-256<input value={primary.config_sha256} onChange={event => updateWorkflow({ config_sha256: event.target.value })} disabled={!canWrite || busy} /></label><label className="wide">Fixed inputs (JSON)<textarea value={fixedInputs} onChange={event => { const value = event.target.value; setFixedInputs(value); try { const parsed = JSON.parse(value) as Record<string, string>; if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(); updateWorkflow({ inputs: parsed }) } catch { setError('Fixed inputs must be a JSON object; the edited text is preserved.') } }} disabled={!canWrite || busy} /></label></div></details>
    <details><summary>Recovery workflow</summary><label className="checkbox-label"><input type="checkbox" checked={recoveryEnabled} onChange={event => { setRecoveryEnabled(event.target.checked); update({ recovery_workflow: event.target.checked ? recovery : undefined }) }} disabled={!canWrite || busy} /> Configure a separate recovery workflow</label>{recoveryEnabled && <div className="form-grid"><label>Known workflow<select value={recovery.id} onChange={event => chooseWorkflow(event.target.value, true)} disabled={!canWrite || busy}><option value="">Choose a workflow</option>{(workflows.data?.items ?? []).map(item => <option key={item.id} value={item.id}>{item.name} · {item.path}</option>)}</select></label><label>Workflow ID<input value={recovery.id} onChange={event => updateWorkflow({ id: event.target.value }, true)} disabled={!canWrite || busy} /></label><label>Path<input value={recovery.path} onChange={event => updateWorkflow({ path: event.target.value }, true)} disabled={!canWrite || busy} /></label><label>Ref<input value={recovery.ref} onChange={event => updateWorkflow({ ref: event.target.value }, true)} disabled={!canWrite || busy} /></label><label>Workflow SHA<input value={recovery.sha} onChange={event => updateWorkflow({ sha: event.target.value }, true)} disabled={!canWrite || busy} /></label><label>Config SHA-256<input value={recovery.config_sha256} onChange={event => updateWorkflow({ config_sha256: event.target.value }, true)} disabled={!canWrite || busy} /></label></div>}</details>
    <details><summary>Keys and health</summary><div className="form-grid"><label className="wide">Provenance Ed25519 public key<textarea value={draft.provenance_public_key} onChange={event => update({ provenance_public_key: event.target.value })} disabled={!canWrite || busy} /></label><label className="wide">Health Ed25519 public key<textarea value={draft.health_public_key} onChange={event => update({ health_public_key: event.target.value })} disabled={!canWrite || busy} /></label><label className="wide">Health checks<textarea value={draft.health_checks.join('\n')} onChange={event => update({ health_checks: event.target.value.split('\n').filter(Boolean) })} disabled={!canWrite || busy} /></label><label>Observation seconds<input type="number" min="0" value={draft.observation_seconds} onChange={event => update({ observation_seconds: Number(event.target.value) })} disabled={!canWrite || busy} /></label><label>Evidence age seconds<input type="number" min="0" value={draft.max_evidence_age_seconds} onChange={event => update({ max_evidence_age_seconds: Number(event.target.value) })} disabled={!canWrite || busy} /></label><label>Deadline seconds<input type="number" min="0" value={draft.deadline_seconds} onChange={event => update({ deadline_seconds: Number(event.target.value) })} disabled={!canWrite || busy} /></label></div></details>
    <details><summary>Qualification</summary><div className="form-grid"><label>Provider<input value={draft.qualification.provider} onChange={event => updateQualification({ provider: event.target.value })} disabled={!canWrite || busy} /></label><label>Server version<input value={draft.qualification.server_version} onChange={event => updateQualification({ server_version: event.target.value })} disabled={!canWrite || busy} /></label><label>Connection version<input type="number" value={draft.qualification.connection_version} onChange={event => updateQualification({ connection_version: Number(event.target.value) })} disabled={!canWrite || busy} /></label><label>Evidence reference<input value={draft.qualification.evidence_reference} onChange={event => updateQualification({ evidence_reference: event.target.value })} disabled={!canWrite || busy} /></label><label>Evidence SHA-256<input value={draft.qualification.evidence_sha256} onChange={event => updateQualification({ evidence_sha256: event.target.value })} disabled={!canWrite || busy} /></label><label>Verified at<input type="datetime-local" value={dateInput(draft.qualification.verified_at)} onChange={event => updateQualification({ verified_at: dateValue(event.target.value) })} disabled={!canWrite || busy} /></label><label>Expires at<input type="datetime-local" value={dateInput(draft.qualification.expires_at)} onChange={event => updateQualification({ expires_at: dateValue(event.target.value) })} disabled={!canWrite || busy} /></label>{(['pinned_inputs', 'native_enforcement', 'environment_serialization', 'no_bypass'] as const).map(key => <label className="checkbox-label" key={key}><input type="checkbox" checked={draft.qualification[key]} onChange={event => updateQualification({ [key]: event.target.checked })} disabled={!canWrite || busy} /> {key.replaceAll('_', ' ')}</label>)}</div><p className="table-meta">Forge metadata: {connectionItems.map(item => `${item.name} · ${item.provider} · v${item.version}`).join(' · ') || 'No forge connections recorded.'}</p>{connections.hasNextPage && <Button disabled={connections.isFetchingNextPage} onClick={() => void connections.fetchNextPage()}>{connections.isFetchingNextPage ? 'Loading…' : 'Load more connections'}</Button>}</details>
    {workflows.error && <p className="error-text" role="alert">{text(workflows.error)} <Button onClick={() => void workflows.refetch()}>Retry workflow discovery</Button></p>}{error && <p className="error-text" role="alert">{error}</p>}{saved && <p role="status">Deployment settings saved.</p>}<Button className="button button-primary" disabled={!canWrite || busy || !csrf} onClick={() => void save()}>{busy ? 'Saving…' : 'Save deployment settings'}</Button>
  </section>
}
