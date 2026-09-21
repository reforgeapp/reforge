import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { repairAPI, type RepairInput } from '../api/repair'
import { customProfileAPI } from '../custom-profile-api'
import { Button } from '../components/Accessible'

type Props = { orgID: string; findingID: string; findingVersion: number; repositoryID?: string; csrf: string }

export function RepairPreview({ orgID, findingID, findingVersion, repositoryID = '', csrf }: Props) {
  const recipes = useQuery({ queryKey: ['org', orgID, 'repair-recipes'], queryFn: ({ signal }) => repairAPI.recipes(orgID, signal) })
  const models = useQuery({ queryKey: ['org', orgID, 'repair-models'], queryFn: ({ signal }) => repairAPI.models(orgID, signal) })
  const agents = useQuery({ queryKey: ['org', orgID, 'repair-agent-connections'], queryFn: ({ signal }) => repairAPI.agentConnections(orgID, signal) })
  const profiles = useQuery({ queryKey: ['org', orgID, 'repair-custom-profiles'], queryFn: ({ signal }) => customProfileAPI.list(orgID, undefined, signal) })
  const pools = useQuery({ queryKey: ['org', orgID, 'repair-pools'], queryFn: ({ signal }) => repairAPI.pools(orgID, signal) })
  const [recipe, setRecipe] = useState('')
  const [profile, setProfile] = useState('')
  const [model, setModel] = useState('')
  const [route, setRoute] = useState('default')
  const [pool, setPool] = useState('')
  const [preview, setPreview] = useState<Awaited<ReturnType<typeof repairAPI.preview>>>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [runID, setRunID] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID())
  const [, setClock] = useState(Date.now())

  useEffect(() => {
    setPreview(undefined); setRunID(''); setError(''); setIdempotencyKey(crypto.randomUUID())
  }, [orgID, findingID, findingVersion])
  useEffect(() => {
    if (!preview) return
    const timer = window.setInterval(() => setClock(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [preview])

  const approvedProfiles = useMemo(() => (profiles.data?.items ?? []).filter(item => item.approved_at && !item.revoked_at), [profiles.data])
  const healthyModels = useMemo(() => profile
    ? (agents.data?.items ?? []).filter(item => item.provider === 'custom_command' && item.state === 'healthy')
    : (models.data?.items ?? []).filter(item => item.kind === 'model' && item.state === 'healthy' && item.settings.billing_route === 'direct_api'), [agents.data, models.data, profile])
  const activePools = useMemo(() => (pools.data?.items ?? []).filter(item => item.state === 'active' && (!repositoryID || item.repository_ids.includes(repositoryID))), [pools.data, repositoryID])
  const selectedModel = healthyModels.find(item => item.id === model)
  const selectedProfile = approvedProfiles.find(item => item.id === profile)
  const selectedPool = activePools.find(item => item.id === pool)
  const selectedRecipe = recipe && recipes.data?.[recipe]
  const imageNames = Object.keys(recipes.data ?? {})
  const optionsError = recipes.isError || models.isError || agents.isError || profiles.isError || pools.isError
  const optionsReady = !optionsError && imageNames.length > 0 && !!selectedRecipe && !!selectedPool && !!route.trim() && !!csrf && (profile ? !!selectedProfile : !!selectedModel)
  const input: RepairInput = { finding_id: findingID, finding_version: findingVersion, recipe, model_connection_id: model, model_route: route.trim(), runner_pool_id: pool, ...(profile && selectedProfile ? { custom_profile_id: profile, custom_profile_version: selectedProfile.version } : {}) }
  const expires = preview ? new Date(preview.expires_at).getTime() : 0
 const expired = !!preview && (!Number.isFinite(expires) || expires <= Date.now())

  const reset = () => { setPreview(undefined); setRunID(''); setError(''); setIdempotencyKey(crypto.randomUUID()) }
  useEffect(() => {
    if ((model && !selectedModel) || (profile && !selectedProfile) || (pool && !selectedPool) || (recipe && !selectedRecipe)) reset()
  }, [model, profile, pool, recipe, selectedModel?.id, selectedProfile?.id, selectedPool?.id, selectedRecipe])
  const retryOptions = () => { void recipes.refetch(); void models.refetch(); void pools.refetch() }
  const inspect = async () => {
    if (!optionsReady) return
    setBusy(true); setError('')
    try { setPreview(await repairAPI.preview(orgID, input, csrf)); setRunID('') } catch (reason) { setError(reason instanceof Error ? reason.message : 'Preview unavailable.') } finally { setBusy(false) }
  }
  const start = async () => {
    if (!preview || expired || !optionsReady || preview.blockers.length || !preview.context.plan.digest) { setError('Preview expired. Generate a new preview.'); return }
    setBusy(true); setError('')
    try { const run = await repairAPI.start(orgID, { ...input, plan_digest: preview.context.plan.digest, idempotency_key: idempotencyKey }, csrf); setRunID(run.task.id) } catch (reason) { setError(reason instanceof Error ? reason.message : 'Repair could not be queued.') } finally { setBusy(false) }
  }

  return <fieldset disabled={busy}>
    <legend>Repair preview</legend>
    {recipes.isLoading || models.isLoading || agents.isLoading || profiles.isLoading || pools.isLoading ? <p className="table-meta">Loading repair policy and execution options…</p> : <>
      <label>Recipe<select aria-label="Repair recipe" value={recipe} onChange={event => { setRecipe(event.target.value); reset() }}><option value="">Choose registered recipe</option>{Object.entries(recipes.data ?? {}).map(([name, image]) => <option key={name} value={name}>{name} · {image}</option>)}</select></label>
      {!imageNames.length && <p className="error-text">No operator images are configured. Configure a registered operator image before preview.</p>}
      <label>Custom command profile<select aria-label="Custom command profile" value={profile} onChange={event => { setProfile(event.target.value); setModel(''); reset() }}><option value="">None — use a model route</option>{approvedProfiles.map(item => <option key={item.id} value={item.id}>{item.name} · v{item.version}</option>)}</select></label>
      <label>{profile ? 'Custom runtime connection' : 'Model'}<select aria-label="Model connection" value={model} onChange={event => { setModel(event.target.value); reset() }}><option value="">{profile ? 'Choose healthy custom_command agent connection' : 'Choose healthy direct API model'}</option>{healthyModels.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label>Model route<input aria-label="Model route" value={route} onChange={event => { setRoute(event.target.value); reset() }} placeholder="default" /></label>
      <label>Runner pool<select aria-label="Runner pool" value={pool} onChange={event => { setPool(event.target.value); reset() }}><option value="">Choose active runner pool</option>{activePools.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      {optionsError && <p className="error-text" role="alert">Repair options unavailable. <Button onClick={retryOptions}>Retry options</Button></p>}
      {!selectedModel && model && !profile && <p className="error-text">Selected model is no longer healthy or direct API enabled. Choose another model.</p>}
      {!selectedProfile && profile && <p className="error-text">Selected custom command profile is no longer approved. Choose another profile.</p>}
      {profile && !healthyModels.length && <p className="error-text">No healthy custom_command agent connection is configured. Add one in Connections and attach a quota budget route.</p>}
      {!selectedPool && pool && <p className="error-text">Selected runner pool is no longer active or assigned to this repository. Choose another pool.</p>}
      <Button disabled={busy || !optionsReady} onClick={inspect}>Generate preview</Button>
      {preview && <div className="stack">
        <p>Expires {new Date(preview.expires_at).toLocaleString()}{expired ? ' · expired' : ''} · Policy {preview.context.policy_hash}</p>
        <p>Head {preview.context.plan.baseline_sha} · Target {preview.context.plan.target_sha}</p>
        <p>Limits: {preview.context.max_output_tokens} output tokens · {preview.context.turn_timeout_ms} ms/turn · {preview.context.plan.max_changed_lines} changed lines · {preview.context.plan.recipe.max_files} files · {preview.context.plan.recipe.max_patch_bytes} bytes · {preview.context.plan.recipe.max_turns} turns · {preview.context.plan.recipe.timeout_seconds}s total</p>
        {(preview.context.plan.recipe.commands ?? []).map(command => <pre className="command-block" key={command.id}>{`${command.id}: ${command.args.join(' ')}\nDirectory: ${command.directory}\nTimeout: ${command.timeout_seconds}s · Report: ${command.report_format}`}</pre>)}
        {preview.blockers.length ? <ul className="compact-list">{preview.blockers.map(item => <li key={item}>{item}</li>)}</ul> : <Button disabled={busy || expired || !optionsReady || !preview.context.plan.digest || !!runID} onClick={start}>Confirm and queue repair</Button>}
        {runID && <p role="status"><a href={`/org/${encodeURIComponent(orgID)}/runs?run=${encodeURIComponent(runID)}`}>Queued run {runID}</a></p>}
      </div>}
      {error && <p className="error-text" role="alert">{error}</p>}
    </>}
  </fieldset>
}
