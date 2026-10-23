import { useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { api } from '../api/client'
import { autopilotAPI, type MaintenanceCommand, type MaintenancePlan } from '../policy-api'

const modes = [{ id: 'merge', label: 'Autopilot' }, { id: 'propose', label: 'Ask me before merging' }, { id: 'observe', label: 'Scan only' }, { id: 'deliver', label: 'Autopilot with deployment' }, { id: 'recover', label: 'Autopilot with deployment and recovery' }]

export function MaintenanceSetup({ orgID, repositoryID, name, csrf }: { orgID: string; repositoryID: string; name: string; csrf: string }) {
  const client = useQueryClient()
  const status = useQuery({ queryKey: ['org', orgID, 'maintenance-status', repositoryID], queryFn: ({ signal }) => autopilotAPI.maintenanceStatus(orgID, repositoryID, signal) })
  const [open, setOpen] = useState(new URLSearchParams(window.location.search).get('setup') === 'autopilot')
  const [connectionID, setConnectionID] = useState<string>()
  const [mode, setMode] = useState<string>()
  const [dollars, setDollars] = useState<string>()
  const [continuous, setContinuous] = useState<boolean>()
  const [plan, setPlan] = useState<MaintenancePlan>()
  const [consent, setConsent] = useState(false)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [saved, setSaved] = useState(false)
  const models = useInfiniteQuery({ queryKey: ['org', orgID, 'setup-models'], queryFn: ({ pageParam, signal }) => api.getConnections(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, enabled: open })
  const selectedMode = mode ?? 'merge'
  const selectedConnection = connectionID ?? status.data?.connection_id ?? ''
  const ceiling = dollars ?? String((status.data?.micro_usd ?? 10_000_000) / 1_000_000)
  const keepWorking = continuous ?? true
  const subscription = plan?.subscription || models.data?.pages.flatMap(page => page.items).find(item => item.id === selectedConnection)?.settings.managed === 'native_runtime'
  const automatic = selectedMode === 'merge' || selectedMode === 'deliver' || selectedMode === 'recover'
  const command = (): MaintenanceCommand => ({ mode: selectedMode, micro_usd: selectedMode === 'observe' || subscription ? 0 : Math.round(Number(ceiling) * 1_000_000), continuous: !!keepWorking, ...(selectedConnection && { connection_id: selectedConnection }) })
  const change = () => { setPlan(undefined); setConsent(false); setMessage(''); setSaved(false) }
  const preview = async () => {
    setBusy(true); setMessage(''); setConsent(false); setSaved(false)
    try { setPlan(await autopilotAPI.previewMaintenance(orgID, repositoryID, command(), csrf)) }
    catch (error) { setPlan(undefined); setMessage(error instanceof Error ? error.message : 'Setup unavailable') }
    finally { setBusy(false) }
  }
  const apply = async () => {
    if (!plan?.ready || !consent) return
    setBusy(true); setMessage('')
    try {
      const result = await autopilotAPI.configureMaintenance(orgID, repositoryID, { ...command(), proof: plan.proof }, csrf)
      setPlan(result); setSaved(result.ready)
      setMessage(result.ready ? automatic ? 'Autopilot ready. Maintenance started.' : 'Maintenance configured. First scan started.' : 'Setup saved. Recheck the remaining step before starting.')
      await client.invalidateQueries({ queryKey: ['org', orgID] })
    } catch (error) { setMessage(error instanceof Error ? error.message : 'Setup could not be saved'); setPlan(undefined); setConsent(false) }
    finally { setBusy(false) }
  }
  const valid = selectedMode === 'observe' || subscription || Number.isFinite(Number(ceiling)) && Number(ceiling) > 0
  return <div className="detail-section"><div className="row-actions"><Button className="mode-button mode-auto" disabled={!csrf} onClick={() => { change(); setMode(undefined); setDollars(undefined); setContinuous(undefined); setConnectionID(undefined); setOpen(true) }}>{status.data?.continuous && status.data.mode !== 'observe' && status.data.mode !== 'propose' ? 'Check Autopilot setup' : 'Set up Autopilot'}</Button>{status.error && <Button onClick={() => void status.refetch()}>Reload setup</Button>}</div>
    <Dialog open={open} title={`Set up ${name}`} onClose={() => { if (!busy) setOpen(false) }}><div className="stack">
      {!plan && <><label>Maintenance<select value={selectedMode} disabled={busy || saved} onChange={event => { setMode(event.target.value); change() }}>{modes.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</select></label>
        {automatic && <p>Reforge fixes issues and merges verified changes after checks pass.</p>}
        {selectedMode !== 'observe' && <>{!subscription && <label>Spending ceiling ($)<input disabled={busy} type="number" min="0.01" step="0.01" max="1000000" value={ceiling} onChange={event => { setDollars(event.target.value); change() }} /></label>}<details><summary>Options</summary><label className="checkbox-label"><input disabled={busy} type="checkbox" checked={!!keepWorking} onChange={event => { setContinuous(event.target.checked); change() }} />Keep maintaining this repository</label><label>Approved model<select disabled={busy} value={selectedConnection} onChange={event => { setConnectionID(event.target.value); change() }}><option value="">Keep approved default</option>{models.data?.pages.flatMap(page => page.items).filter(item => item.state === 'healthy' && (item.settings.billing_route === 'direct_api' || item.settings.managed === 'native_runtime')).map(item => <option key={item.id} value={item.id}>{item.name} · {item.provider}</option>)}</select></label>{models.hasNextPage && <Button onClick={() => void models.fetchNextPage()}>More models</Button>}</details></>}
      </>}
      {status.isLoading && <p role="status">Loading current settings…</p>}{status.error && <p role="alert">Current settings unavailable. <Button onClick={() => void status.refetch()}>Retry</Button></p>}
      {plan && <><p>{plan.repository}: {automatic ? 'fix and merge verified changes' : selectedMode === 'observe' ? 'scan only' : 'prepare fixes for your approval'}.{plan.subscription ? ' Uses connected subscription allowance.' : selectedMode !== 'observe' && ` Up to $${(plan.micro_usd / 1_000_000).toFixed(2)} per ${plan.budget_period || 'monthly'} budget period. Organisation ceiling: ${plan.organisation_micro_usd === undefined ? 'not configured' : '$' + (plan.organisation_micro_usd / 1_000_000).toFixed(2) + ' per ' + plan.organisation_period + ' period'}.`}</p>
        {plan.merge_setup && <section aria-label="Repository permissions"><strong>{plan.ready ? saved ? 'Autopilot enabled' : 'Ready to enable Autopilot' : plan.merge_setup.ready ? 'Merge permission confirmed' : 'Finish repository permissions'}</strong>{!plan.merge_setup.ready && <><p>{plan.merge_setup.reason}</p>{plan.merge_setup.settings_url && <a className="button" href={plan.merge_setup.settings_url} target="_blank" rel="noreferrer">Open repository rules</a>}<details><summary>Permission details</summary><p>{plan.merge_setup.reason.includes("approval") ? 'Allow Reforge through the human review rule only. If review and checks share a ruleset, separate the review rule first so checks stay enforced. Return here and recheck.' : 'Ask your repository operator to verify the merge rule and Reforge connection. Keep protections enabled, then return here and recheck.'}</p></details></>}</section>}
        {!!plan.blockers.filter(blocker => blocker !== plan.merge_setup?.reason).length && <div role="status"><ul>{plan.blockers.filter(blocker => blocker !== plan.merge_setup?.reason).map(blocker => <li key={blocker}>{blocker}</li>)}</ul><a href={`/org/${encodeURIComponent(orgID)}/connections`}>Connections</a> · <a href={`/org/${encodeURIComponent(orgID)}/usage`}>Spending</a> · <a href={`/org/${encodeURIComponent(orgID)}/policies`}>Maintenance permissions</a></div>}
        {!!plan.environments?.length && <p>Deployment: {plan.environments.join(', ')}.</p>}
        {plan.ready && !saved && <label className="checkbox-label"><input disabled={busy} type="checkbox" checked={consent} onChange={event => setConsent(event.target.checked)} />{automatic ? `Allow Reforge to fix and merge in ${name} within these limits` : `Allow ${selectedMode === 'observe' ? 'scanning' : 'proposed fixes'} in ${name} within these limits`}</label>}
        {!saved && <div className="row-actions"><Button disabled={busy} onClick={change}>Back</Button>{!plan.ready && <Button disabled={busy} onClick={() => void preview()}>Recheck setup</Button>}{plan.ready && <Button className={automatic ? 'mode-button mode-auto' : 'button button-primary'} disabled={busy || !consent} onClick={() => void apply()}>{busy ? 'Starting…' : automatic ? 'Enable Autopilot' : 'Set up maintenance'}</Button>}</div>}
      </>}
      {message && <p role="status">{message}</p>}
      {!plan && <Button className="button button-primary" disabled={!csrf || busy || !valid || !status.data} onClick={() => void preview()}>{busy ? 'Checking…' : 'Check setup'}</Button>}
      {saved && <Button onClick={() => setOpen(false)}>Done</Button>}
    </div></Dialog>
  </div>
}
