import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ReforgeAPIError } from '../api/client'
import { Button } from '../components/Accessible'
import { usageAPI, type Route } from '../usage-api'

const dollars = (micro: number) => (micro / 1_000_000).toString()
const micro = (value: string) => Math.round(Number(value) * 1_000_000)
const valid = (value: string) => value.trim() !== '' && Number.isFinite(Number(value)) && Number(value) >= 0

export function ModelPricing({ orgID, connectionID, model, csrf }: { orgID: string; connectionID: string; model: string; csrf: string }) {
  const route = useQuery({ queryKey: ['org', orgID, 'budget-route', connectionID, model, 'default'], queryFn: ({ signal }) => usageAPI.route(orgID, connectionID, model, 'default', signal), retry: false })
  const pricing=useQuery({queryKey:['org',orgID,'model-pricing',connectionID],queryFn:({signal})=>usageAPI.pricing(orgID,connectionID,signal)})
  const preset=pricing.data?.presets[0]
  const missing = route.error instanceof ReforgeAPIError && route.error.code === 'budget_unconfigured'
  const [input, setInput] = useState(''); const [output, setOutput] = useState(''); const [busy, setBusy] = useState(false); const [message, setMessage] = useState('')
  useEffect(() => { if (route.data) { setInput(dollars(route.data.input_micro_usd_per_million)); setOutput(dollars(route.data.output_micro_usd_per_million)) } }, [route.data])
  const save = async (event: FormEvent) => {
    event.preventDefault()
    const base: Route = route.data ?? { connection_id: connectionID, model, name: 'default', mode: 'priced', pricing_version: '', input_micro_usd_per_million: 0, output_micro_usd_per_million: 0, request_micro_usd: 0, max_input_tokens: 64_000, max_output_tokens: 4096, max_milliseconds: 600_000, max_requests: 1, qualified: false, qualification_ref: '', paused: false, version: 0 }
    setBusy(true); setMessage('')
    try {
      await usageAPI.saveRoute(orgID, { ...base, qualified: false, qualification_ref: '', pricing_preset_id: '', pricing_source: '', pricing_checked_at: '', cost_basis: 'operator_override', pricing_version: `manual-${new Date().toISOString().slice(0, 10)}`, input_micro_usd_per_million: micro(input), output_micro_usd_per_million: micro(output) }, csrf)
      await route.refetch();await pricing.refetch();setMessage('Pricing saved.')
    } catch (reason) { setMessage(reason instanceof Error ? reason.message : 'Pricing could not be saved.') } finally { setBusy(false) }
  }
 const applyPreset=async()=>{if(!preset)return;setBusy(true);setMessage('');try{await usageAPI.applyPricePreset(orgID,connectionID,preset.id,route.data?.version??0,csrf);await route.refetch();await pricing.refetch();setMessage('Published price ceiling saved.')}catch(error){setMessage(error instanceof Error?error.message:'Pricing could not be saved.')}finally{setBusy(false)}}
  if (route.isLoading) return null
  if (route.error && !missing) return <p className="error-text" role="alert">Pricing unavailable: {route.error.message}</p>
  return <form className="model-pricing" aria-label="Model pricing" onSubmit={save}>
    <h3>Pricing</h3>
    {pricing.data && <p role="status">{pricing.data.manual?'Manual repair supported':'Manual repair unavailable'} · {pricing.data.automatic?'Automatic maintenance supported':'Automatic maintenance unavailable'}<span className="help-tip" tabIndex={0} title={pricing.data.reason}>?</span></p>}
    {pricing.error && <p role="alert">Eligibility unavailable <Button onClick={()=>void pricing.refetch()}>Retry</Button></p>}
    {preset && <div className="row-actions"><Button disabled={busy || !csrf} onClick={()=>void applyPreset()}>Use published price ceiling</Button><a href={preset.source} target="_blank" rel="noreferrer">Source · {preset.checked_at}</a><span className="help-tip" tabIndex={0} title="Conservative standard API rates include longer context and cache writes. Measured token usage determines allowance used; provider invoice may differ.">?</span></div>}
    {!preset && missing && <p className="table-meta">Operator pricing required for this endpoint.</p>}
    {route.data && <p className="table-meta">{route.data.cost_basis==='published_ceiling'?'Published upper estimate':'Operator pricing'} · {route.data.pricing_version}</p>}
    <details open={!preset && missing}><summary>Custom prices</summary><div className="form-grid">
      <label>Input $ per 1M tokens<input type="number" min="0" step="0.000001" required value={input} onChange={event => setInput(event.target.value)} /></label>
      <label>Output $ per 1M tokens<input type="number" min="0" step="0.000001" required value={output} onChange={event => setOutput(event.target.value)} /></label>
    </div>
    <div className="row-actions"><Button type="submit" className="button button-primary" disabled={busy || !csrf || !valid(input) || !valid(output)}>{busy ? 'Saving…' : 'Save pricing'}</Button></div></details>{message && <p className="table-meta" role="status">{message}</p>}
  </form>
}

export function ModelEligibility({orgID,connectionID}:{orgID:string;connectionID:string}) {
 const query=useQuery({queryKey:['org',orgID,'model-pricing',connectionID],queryFn:({signal})=>usageAPI.pricing(orgID,connectionID,signal)})
 if(query.error) return <p role="alert">Eligibility unavailable <Button onClick={()=>void query.refetch()}>Retry</Button></p>
 if(!query.data) return null
 return <p>{query.data.manual?'Manual repair supported':'Manual repair unavailable'} · {query.data.automatic?'Automatic maintenance supported':'Automatic maintenance unavailable'}<span className="help-tip" tabIndex={0} title={query.data.reason}>?</span>{!query.data.manual && <small className="table-meta">{query.data.reason}</small>}</p>
}
