import { useState } from 'react'
import { apiRequest } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Accessible'

export function RestoreRelease({ orgID, kind, id, version, csrf, allowed }: { orgID: string; kind: 'native' | 'gitops'; id: string; version: number; csrf: string; allowed: boolean }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<components['schemas']['RestorePlan']>()
  const path = `/api/v1/orgs/${encodeURIComponent(orgID)}/delivery/${kind}/${encodeURIComponent(id)}/restore`
  const preview = async () => {
    setBusy(true); setError(''); setPlan(undefined)
    try { setPlan(await apiRequest<components['schemas']['RestorePlan']>(path + '/preview', {method: 'POST', headers: {'If-Match': '"' + version + '"'}}, csrf)) }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Recovery unavailable.') }
    finally { setBusy(false) }
  }
  const restore = async () => {
    setBusy(true); setError('')
    try {
      const result = await apiRequest<components['schemas']['DeliveryResult']>(path, {method: 'POST', headers: {'If-Match': '"' + version + '"'}, body: JSON.stringify({gate_id: plan?.gate_id})}, csrf)
      window.location.assign(`/org/${encodeURIComponent(orgID)}/${kind === 'gitops' ? 'gitops?promotion=' : 'deployments?deployment='}${encodeURIComponent(result.id)}`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Recovery unavailable.') }
    finally { setBusy(false) }
  }
  return <div><Button disabled={!allowed || !csrf || busy} onClick={() => void preview()}>{busy ? 'Checking recovery…' : 'Restore previous working version'}</Button>
    {plan && <div role="region" aria-label="Restore preview"><p>Restore {plan.environment} release from {new Date(plan.previous_at).toLocaleString()}.</p><ul>{plan.limits.map(limit => <li key={limit}>{limit}</li>)}</ul>{plan.blockers.length ? <p role="alert">{plan.blockers.join('; ')}</p> : <Button disabled={busy || Date.parse(plan.expires_at) <= Date.now()} onClick={() => void restore()}>Restore this release</Button>}</div>}
    {error && <p role="alert" className="error-text">{error}</p>}</div>
}
