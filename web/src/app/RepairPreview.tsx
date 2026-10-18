import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { repairAPI, type RepairInput } from '../api/repair'
import { customProfileAPI } from '../custom-profile-api'
import { Button } from '../components/Accessible'

type Props = { orgID: string; findingID: string; findingVersion: number; repositoryID?: string; csrf: string }
const message=(error:unknown)=>error instanceof Error?error.message:'Repair unavailable.'

export function RepairPreview({ orgID, findingID, findingVersion, repositoryID = '', csrf }: Props) {
 const [advanced,setAdvanced]=useState(false)
 const defaults=useQuery({queryKey:['org',orgID,'repair-defaults',repositoryID],queryFn:({signal})=>repairAPI.defaults(orgID,repositoryID,signal),enabled:!!repositoryID})
 const recipes=useQuery({queryKey:['org',orgID,'repair-recipes'],queryFn:({signal})=>repairAPI.recipes(orgID,signal)})
 const models=useQuery({queryKey:['org',orgID,'repair-models'],queryFn:({signal})=>repairAPI.models(orgID,signal),enabled:advanced})
 const agents=useQuery({queryKey:['org',orgID,'repair-agent-connections'],queryFn:({signal})=>repairAPI.agentConnections(orgID,signal),enabled:advanced})
 const profiles=useQuery({queryKey:['org',orgID,'repair-custom-profiles'],queryFn:({signal})=>customProfileAPI.list(orgID,undefined,signal),enabled:advanced})
 const pools=useQuery({queryKey:['org',orgID,'repair-pools'],queryFn:({signal})=>repairAPI.pools(orgID,signal),enabled:advanced})
 const [recipe,setRecipe]=useState('')
 const [instructions,setInstructions]=useState('')
 const [profile,setProfile]=useState('')
 const [model,setModel]=useState('')
 const [route,setRoute]=useState('default')
 const [pool,setPool]=useState('')
 const [preview,setPreview]=useState<Awaited<ReturnType<typeof repairAPI.preview>>>()
 const [error,setError]=useState('')
 const [busy,setBusy]=useState(false)
 const [runID,setRunID]=useState('')
 const [idempotencyKey,setIdempotencyKey]=useState(()=>crypto.randomUUID())
 const [,setClock]=useState(Date.now())
 const reset=()=>{setPreview(undefined);setRunID('');setError('');setIdempotencyKey(crypto.randomUUID())}
 useEffect(reset,[orgID,findingID,findingVersion])
 useEffect(()=>{if(!preview)return;const timer=window.setInterval(()=>setClock(Date.now()),1000);return()=>window.clearInterval(timer)},[preview])
 const approvedProfiles=useMemo(()=>(profiles.data?.items??[]).filter(item=>item.approved_at&&!item.revoked_at),[profiles.data])
 const healthyModels=useMemo(()=>profile?(agents.data?.items??[]).filter(item=>item.provider==='custom_command'&&item.state==='healthy'):(models.data?.items??[]).filter(item=>item.kind==='model'&&item.state==='healthy'&&(item.settings.billing_route==='direct_api'||item.settings.auth_kind==='chatgpt_oauth')),[agents.data,models.data,profile])
 const activePools=useMemo(()=>(pools.data?.items??[]).filter(item=>item.state==='active'&&(!repositoryID||item.repository_ids.includes(repositoryID))),[pools.data,repositoryID])
 const selectedProfile=approvedProfiles.find(item=>item.id===profile)
 const imageNames=Object.keys(recipes.data??{})
 const optionsError=advanced?(recipes.error||models.error||agents.error||profiles.error||pools.error):(recipes.error||defaults.error)
 const ready=!!csrf&&!optionsError&&imageNames.length>0&&(advanced?!!healthyModels.find(item=>item.id===model)&&!!activePools.find(item=>item.id===pool)&&!!route.trim()&&(!profile||!!selectedProfile):!!defaults.data?.model_connection_id&&!!defaults.data?.runner_pool_id&&!defaults.data?.blockers.length)
 const input:RepairInput={finding_id:findingID,finding_version:findingVersion,recipe,model_connection_id:advanced?model:defaults.data?.model_connection_id??'',model_route:advanced?route.trim():defaults.data?.model_route??'default',runner_pool_id:advanced?pool:defaults.data?.runner_pool_id??'',owner:!profile,instructions:instructions.trim(),...(profile&&selectedProfile?{custom_profile_id:profile,custom_profile_version:selectedProfile.version}:{})}
 const expired=!!preview&&(!Number.isFinite(Date.parse(preview.expires_at))||Date.parse(preview.expires_at)<=Date.now())
 useEffect(()=>{if(advanced&&!model&&healthyModels.length===1)setModel(healthyModels[0].id)},[advanced,model,healthyModels])
 useEffect(()=>{if(advanced&&!pool&&activePools.length===1)setPool(activePools[0].id)},[advanced,pool,activePools])
 const inspect=async()=>{if(!ready)return;setBusy(true);setError('');try{setPreview(await repairAPI.preview(orgID,input,csrf));setRunID('')}catch(error){setError(message(error))}finally{setBusy(false)}}
 const start=async()=>{if(!preview||expired||!ready||preview.blockers.length||!preview.context.plan.digest){setError('Check fix again before starting.');return}setBusy(true);setError('');try{const run=await repairAPI.start(orgID,{...input,plan_digest:preview.context.plan.digest,idempotency_key:idempotencyKey},csrf);setRunID(run.task.id)}catch(error){setError(message(error))}finally{setBusy(false)}}
 return <fieldset disabled={busy}><legend>Fix this</legend>
 {defaults.isLoading&&<p role="status">Checking approved maintenance setup…</p>}
 {defaults.data?.subscription&&<p className="table-meta">Uses ChatGPT plan · {defaults.data.requests} requests per {defaults.data.period} period.</p>}
 {defaults.data?.micro_usd!==undefined&&!defaults.data.subscription&&<p className="table-meta">Shared maintenance ceiling ${(defaults.data.micro_usd/1_000_000).toFixed(2)} per {defaults.data.period} period.<span className="help-tip" tabIndex={0} title="Organisation spending limits also apply.">?</span></p>}
 {!advanced&&defaults.data?.blockers.length? <div><ul>{defaults.data.blockers.map(item=><li key={item}>{item}</li>)}</ul><a href={`/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(repositoryID)}`}>Set up maintenance</a></div>:null}
 {!imageNames.length&&!recipes.isLoading&&<p>Operator: enable repair execution.</p>}
 <details open={advanced} onToggle={event=>{const next=event.currentTarget.open;if(next!==advanced){setAdvanced(next);setProfile('');reset()}}}><summary>Advanced</summary>
 <label>Model<select aria-label="Model connection" value={model} onChange={event=>{setModel(event.target.value);reset()}}><option value="">Choose approved connection</option>{healthyModels.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
 <label>Runner pool<select aria-label="Runner pool" value={pool} onChange={event=>{setPool(event.target.value);reset()}}><option value="">Choose runner</option>{activePools.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
 <label>Recipe<select aria-label="Repair recipe" value={recipe} onChange={event=>{setRecipe(event.target.value);reset()}}><option value="">Auto</option>{Object.keys(recipes.data??{}).map(name=><option key={name} value={name}>{name}</option>)}</select></label>
 <label>Custom command profile<select aria-label="Custom command profile" value={profile} onChange={event=>{setProfile(event.target.value);setModel('');reset()}}><option value="">Use approved model</option>{approvedProfiles.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
 <label>Model route<input aria-label="Model route" value={route} onChange={event=>{setRoute(event.target.value);reset()}} /></label>
 <label>Instructions<textarea aria-label="Instructions" value={instructions} maxLength={2000} rows={3} onChange={event=>{setInstructions(event.target.value);reset()}} /></label>
 </details>
 {optionsError&&<p role="alert">Options unavailable: {message(optionsError)} <Button onClick={()=>{void defaults.refetch();void recipes.refetch();if(advanced){void models.refetch();void agents.refetch();void profiles.refetch();void pools.refetch()}}}>Retry options</Button></p>}
 <Button disabled={!ready||busy} onClick={()=>void inspect()}>Check fix</Button>
 {preview&&<div className="stack"><p>Bounded fix: up to {preview.context.plan.recipe.max_files} files. Existing checks must remain.</p><p>{preview.context.plan.recipe.commands.length} configured checks. Execution results follow after repair.</p><details><summary>Technical evidence</summary><p>Policy {preview.context.policy_hash} · Expires {new Date(preview.expires_at).toLocaleString()}{expired?' · expired':''}</p><p>Head {preview.context.plan.baseline_sha} · Target {preview.context.plan.target_sha}</p><p>Recipe {preview.context.plan.recipe.name} · {preview.context.max_output_tokens} output tokens · {preview.context.turn_timeout_ms} ms/turn</p>{preview.context.plan.recipe.commands.map(command=><pre className="command-block" key={command.id}>{`${command.id}: ${command.args.join(' ')}\nDirectory: ${command.directory}\nTimeout: ${command.timeout_seconds}s · Report: ${command.report_format}`}</pre>)}</details>{preview.blockers.length?<ul>{preview.blockers.map(item=><li key={item}>{item}</li>)}</ul>:<Button disabled={busy||expired||!ready||!!runID} onClick={()=>void start()}>Start fix</Button>}{runID&&<p role="status"><a href={`/org/${encodeURIComponent(orgID)}/runs?run=${encodeURIComponent(runID)}`}>Fix started</a></p>}</div>}
 {error&&<p role="alert">{error}</p>}
 </fieldset>
}
