import { useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { discoveryAPI } from '../api/discovery'
import { api, type Connection, type RunnerPool } from '../api/client'
import { apiRequest } from '../api/client'
import { deploymentAPI, type PreviewRequest as PipelineRequest } from '../deployment-api'
import { gitopsAPI, type PreviewRequest as GitOpsRequest } from '../gitops-api'
import { type Input, type MemberInput } from '../campaign-api'
import { usageAPI } from '../usage-api'

type Props = { orgID: string; onPreview: (input: Input) => Promise<void>; onChange: () => void; busy?: boolean; pending?: boolean }
type Recipe = { name: string; image: string }
type Finding = { id: string; repository_id: string; title: string; version: number; state: string }
const text = (value: unknown) => value instanceof Error ? value.message : 'Unable to load campaign inputs.'
const iso = (value: string) => value ? `${value}:00Z` : undefined

export function CampaignCreateForm({ orgID, onPreview, onChange, busy = false, pending = false }: Props) {
  const [kind, setKind] = useState<Input['kind']>('repair')
  const [name, setName] = useState('')
  const [selection, setSelection] = useState('')
  const [chosen, setChosen] = useState<Record<string, Finding>>({})
  const [nativeMembers, setNativeMembers] = useState<MemberInput[]>([])
  const [bulk, setBulk] = useState('')
  const [windows, setWindows] = useState<Input['windows']>([])
  const [recipe, setRecipe] = useState('')
  const [model, setModel] = useState('')
  const [route, setRoute] = useState('default')
  const [runner, setRunner] = useState('')
  const [environment, setEnvironment] = useState('')
  const [sourceSHA, setSourceSHA] = useState('')
  const [artifactDigest, setArtifactDigest] = useState('')
  const [provenance, setProvenance] = useState('{}')
  const [changeID, setChangeID] = useState('')
  const [canaryIDs, setCanaryIDs] = useState('')
  const [canarySize, setCanarySize] = useState(1)
  const [batchSize, setBatchSize] = useState(1)
  const [concurrency, setConcurrency] = useState(1)
  const [success, setSuccess] = useState<Input['success']>('published')
  const [observation, setObservation] = useState(0)
  const [failureLimit, setFailureLimit] = useState(0)
  const [failurePercent, setFailurePercent] = useState(0)
  const [notBefore, setNotBefore] = useState('')
  const [error, setError] = useState('')
  const findings = useInfiniteQuery({ enabled: kind === 'repair', queryKey: ['org', orgID, 'campaign-findings', selection], queryFn: ({ pageParam, signal }) => discoveryAPI.findings(orgID, { q: selection || undefined, state: 'open', cursor: pageParam, limit: 100, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const recipes = useQuery({ queryKey: ['org', orgID, 'campaign-recipes'], queryFn: ({ signal }) => apiRequest<Record<string, string>>(`/api/v1/orgs/${encodeURIComponent(orgID)}/repair-recipes`, { signal }), enabled: kind === 'repair' })
  const connections = useInfiniteQuery({ queryKey: ['org', orgID, 'campaign-models'], queryFn: ({ pageParam, signal }) => api.getConnections(orgID, { cursor: pageParam, limit: 100, signal }), enabled: kind === 'repair', initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const pools = useInfiniteQuery({ queryKey: ['org', orgID, 'campaign-pools'], queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { cursor: pageParam, limit: 100, signal }), enabled: kind === 'repair', initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const deployments = useQuery({ queryKey: ['org', orgID, 'campaign-deployment-configs'], queryFn: ({ signal }) => deploymentAPI.configurations(orgID, signal), enabled: kind === 'pipeline' })
  const gitops = useQuery({ queryKey: ['org', orgID, 'campaign-gitops-configs'], queryFn: ({ signal }) => gitopsAPI.configurations(orgID, signal), enabled: kind === 'gitops' })
  const findingItems = findings.data?.pages.flatMap(page => page.items) as Finding[] ?? []
  const selected = Object.values(chosen)
  const selectedRepos = kind === 'repair' ? selected.map(item => item.repository_id) : nativeMembers.map(item => item.repository_id)
  const recipesList = Object.entries(recipes.data ?? {}).map(([name, image]) => ({ name, image })) as Recipe[]
  const models = (connections.data?.pages.flatMap(page => page.items) as Connection[] ?? []).filter(item => item.kind === 'model' || item.kind === 'agent')
  const runnerPools = pools.data?.pages.flatMap(page => page.items) as RunnerPool[] ?? []
  const environments = useMemo(() => kind === 'pipeline' ? (deployments.data?.items ?? []) : (gitops.data?.items ?? []), [kind, deployments.data, gitops.data])
  const change = (fn: () => void) => { fn(); onChange() }
  const toggle = (finding: Finding) => change(() => setChosen(previous => { const next = { ...previous }; if (next[finding.id]) delete next[finding.id]; else if (!Object.values(previous).some(item => item.repository_id === finding.repository_id)) next[finding.id] = finding; return next }))
  const selectedModel = models.find(item => item.id === model)
  const modelName = typeof selectedModel?.settings?.model === 'string' ? selectedModel.settings.model : ''
  const billing = useQuery({ queryKey: ['org', orgID, 'campaign-route', model, modelName, route], queryFn: ({ signal }) => usageAPI.route(orgID, model, modelName, route, signal), enabled: kind === 'repair' && !!model && !!modelName && !!route })
  const addNative = () => {
    setError('')
    try {
      const config = environments.find(item => item.environment === environment) as { repository_id?: string; source_repository_id?: string } | undefined
      const repository_id = config?.repository_id ?? config?.source_repository_id
      if (!repository_id || !changeID.trim() || !sourceSHA.trim() || !artifactDigest.trim()) throw new Error('Select an environment and supply its change, source and artifact.')
      if (nativeMembers.some(item => item.repository_id === repository_id)) throw new Error('One environment per repository is allowed in a campaign.')
      const base = { change_id: changeID.trim(), source_sha: sourceSHA.trim(), artifact_digest: artifactDigest.trim(), provenance: JSON.parse(provenance) }
      const member: MemberInput = kind === 'pipeline' ? { repository_id, environment, pipeline: base as PipelineRequest } : { repository_id, environment, gitops: base as GitOpsRequest }
      change(() => setNativeMembers([...nativeMembers, member]))
      setEnvironment(''); setChangeID(''); setSourceSHA(''); setArtifactDigest(''); setProvenance('{}')
    } catch (reason) { setError(reason instanceof SyntaxError ? 'Provenance must be valid JSON.' : text(reason)) }
  }
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setError('')
    try {
      if (!name.trim()) throw new Error('Enter a campaign name.')
      const members: MemberInput[] = bulk.trim() ? JSON.parse(bulk) : kind === 'repair' ? selected.map(finding => ({ repository_id: finding.repository_id, repair: { finding_id: finding.id, finding_version: finding.version, recipe, model_connection_id: model, model_route: route, runner_pool_id: runner } })) : nativeMembers
      if (!Array.isArray(members) || !members.length || members.length > 1000) throw new Error('Select or supply between 1 and 1,000 members.')
      await onPreview({ name: name.trim(), kind, selection, members, canary_ids: canaryIDs.split(',').map(value => value.trim()).filter(Boolean), canary_size: canarySize, batch_size: batchSize, concurrency, success, observation_seconds: observation, failure_limit: failureLimit, failure_percent: failurePercent, windows, ...(notBefore ? { not_before: iso(notBefore) } : {}) })
    } catch (reason) { setError(reason instanceof SyntaxError ? 'Member JSON must be a valid array.' : text(reason)) }
  }
  const activeQueries = kind === 'repair' ? [findings, recipes, connections, pools] : kind === 'pipeline' ? [deployments] : [gitops]
  const loading = activeQueries.some(query => query.isLoading)
  const loadError = activeQueries.find(query => query.error)?.error
  return <form className="stack" onSubmit={submit}>{loadError && <p className="error-text" role="alert">{text(loadError)} <Button type="button" onClick={() => { activeQueries.forEach(query => { void query.refetch() }) }}>Retry</Button></p>}
    <fieldset><legend>Campaign</legend><div className="form-grid"><label>Name<input required value={name} onChange={event => change(() => setName(event.target.value))} /></label><label>Kind<select value={kind} onChange={event => change(() => { setKind(event.target.value as Input['kind']); setChosen({}); setNativeMembers([]); setEnvironment(''); setBulk(''); setSuccess(event.target.value === 'repair' ? 'published' : 'healthy') })}><option value="repair">Repair</option><option value="pipeline">Pipeline</option><option value="gitops">GitOps</option></select></label><label>Finding filter<input value={selection} onChange={event => { setSelection(event.target.value); onChange() }} /></label></div></fieldset>
    <fieldset><legend>Members</legend>{kind !== 'repair' && <div className="form-grid"><label>Configured environment<select value={environment} onChange={event => change(() => setEnvironment(event.target.value))}><option value="">Select configured environment</option>{environments.map(item => <option key={item.environment} value={item.environment}>{item.environment}{!item.enabled ? ' (disabled)' : ''}</option>)}</select></label><ul>{nativeMembers.map(member => <li key={`${member.environment}:${member.repository_id}`}>{member.environment} · {member.repository_id} <Button type="button" onClick={() => change(() => setNativeMembers(nativeMembers.filter(item => item !== member)))}>Remove</Button></li>)}</ul></div>}{kind === 'repair' && (loading ? <p>Loading campaign inputs…</p> : findings.error ? <p className="error-text" role="alert">{text(findings.error)}</p> : <><p className="table-meta">Select up to 1,000 loaded findings. One finding per repository; unloaded pages are not selected.</p><div className="compact-list">{findingItems.map(finding => <label key={finding.id}><input type="checkbox" checked={!!chosen[finding.id]} disabled={!chosen[finding.id] && !!selected.find(item => item.repository_id === finding.repository_id)} onChange={() => toggle(finding)} /> {finding.title} · {finding.repository_id}</label>)}</div></>)}{kind === 'repair' && findings.hasNextPage && <Button type="button" disabled={findings.isFetchingNextPage} onClick={() => void findings.fetchNextPage()}>Load more findings</Button>}<label>Advanced member JSON<textarea value={bulk} onChange={event => change(() => setBulk(event.target.value))} /></label></fieldset>
    <fieldset><legend>{kind === 'repair' ? 'Repair execution' : 'Pinned native delivery'}</legend>{kind === 'repair' ? <div className="form-grid"><label>Recipe<select value={recipe} onChange={event => change(() => setRecipe(event.target.value))}><option value="">Select recipe</option>{recipesList.map(item => <option key={item.name} value={item.name}>{item.name}</option>)}</select></label><label>Model connection<select value={model} onChange={event => change(() => setModel(event.target.value))}><option value="">Select connection</option>{models.map(item => <option key={item.id} value={item.id} disabled={item.state !== 'healthy'}>{item.name} · {item.state}</option>)}</select></label>{connections.hasNextPage && <Button type="button" onClick={() => void connections.fetchNextPage()}>Load more connections</Button>}<label>Route<input value={route} onChange={event => change(() => setRoute(event.target.value))} /></label>{billing.error && <p role="alert">Route: {text(billing.error)}</p>}{billing.data && <p>Route {!billing.data.paused && (billing.data.mode === 'priced' || billing.data.qualified) ? 'ready' : 'paused or requires qualification'}</p>}<label>Runner pool<select value={runner} onChange={event => change(() => setRunner(event.target.value))}><option value="">Select pool</option>{runnerPools.map(item => <option key={item.id} value={item.id} disabled={item.state !== 'active'}>{item.name} · {item.state}</option>)}</select></label>{pools.hasNextPage && <Button type="button" onClick={() => void pools.fetchNextPage()}>Load more pools</Button>}</div> : <div className="form-grid"><label>Change ID<input value={changeID} onChange={event => change(() => setChangeID(event.target.value))} /></label><label>Source SHA<input value={sourceSHA} onChange={event => change(() => setSourceSHA(event.target.value))} /></label><label>Artifact digest<input value={artifactDigest} onChange={event => change(() => setArtifactDigest(event.target.value))} /></label><label>Provenance JSON<textarea value={provenance} onChange={event => change(() => setProvenance(event.target.value))} /></label><Button type="button" onClick={addNative}>Add configured member</Button></div>}</fieldset>
    <fieldset><legend>Stages and safety</legend><div className="form-grid"><label>Canary size<input type="number" min="1" max="100" value={canarySize} onChange={event => change(() => setCanarySize(Number(event.target.value)))} /></label><label>Explicit canary IDs<input value={canaryIDs} onChange={event => change(() => setCanaryIDs(event.target.value))} placeholder="repository IDs, comma separated" /></label><label>Batch size<input type="number" min="1" max="100" value={batchSize} onChange={event => change(() => setBatchSize(Number(event.target.value)))} /></label><label>Concurrency<input type="number" min="1" max="100" value={concurrency} onChange={event => change(() => setConcurrency(Number(event.target.value)))} /></label><label>Success<select value={success} onChange={event => change(() => setSuccess(event.target.value as Input['success']))}>{kind === 'repair' && <option value="published">Published</option>}{kind !== 'repair' && <option value="healthy">Healthy</option>}{kind === 'repair' && <option value="merged">Merged</option>}</select></label><label>Observation seconds<input type="number" min="0" max="86400" value={observation} onChange={event => change(() => setObservation(Number(event.target.value)))} /></label><label>Failure limit<input type="number" min="0" max="1000" value={failureLimit} onChange={event => change(() => setFailureLimit(Number(event.target.value)))} /></label><label>Failure percent<input type="number" min="0" max="100" value={failurePercent} onChange={event => change(() => setFailurePercent(Number(event.target.value)))} /></label><label>Not before (UTC)<input type="datetime-local" value={notBefore} onChange={event => change(() => setNotBefore(event.target.value))} /></label><div className="stack"><span>Maintenance windows (UTC)</span>{windows.map((window, index) => <div className="form-grid" key={index}><label>Weekdays (0 Sun–6 Sat)<input aria-label={`Window ${index + 1} weekdays`} value={window.weekdays.join(',')} onChange={event => change(() => setWindows(windows.map((item, i) => i === index ? { ...item, weekdays: event.target.value.split(',').filter(value => value.trim()).map(Number) } : item)))} /></label><label>Start minute (inclusive)<input type="number" min="0" max="1439" value={window.start_minute} onChange={event => change(() => setWindows(windows.map((item, i) => i === index ? { ...item, start_minute: Number(event.target.value) } : item)))} /></label><label>End minute (exclusive)<input type="number" min="1" max="1440" value={window.end_minute} onChange={event => change(() => setWindows(windows.map((item, i) => i === index ? { ...item, end_minute: Number(event.target.value) } : item)))} /></label><Button type="button" onClick={() => change(() => setWindows(windows.filter((_, i) => i !== index)))}>Remove window {index + 1}</Button></div>)}<Button type="button" disabled={windows.length >= 14} onClick={() => change(() => setWindows([...windows, { weekdays: [1,2,3,4,5], start_minute: 0, end_minute: 1440 }]))}>Add UTC window</Button></div></div></fieldset>
    {error && <p className="error-text" role="alert">{error}</p>}<Button className="button button-primary" disabled={busy || pending || (!bulk.trim() && !selectedRepos.length)}>{pending ? 'Preparing preview…' : 'Preview campaign'}</Button>
  </form>
}
