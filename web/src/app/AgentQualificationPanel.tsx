import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { StatusBadge } from '../components/Status'
import { agentAPI, type AgentQualificationInput } from '../agent-api'
import { useSession } from './query'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const flags = [['auth_custody', 'Credential custody'], ['native_tool_containment', 'Native tool containment'], ['terms', 'Provider terms'], ['topology', 'Deployment topology'], ['entitlement', 'Account entitlement'], ['quota', 'Quota and concurrency'], ['no_paid_overage', 'No paid overage']] as const
const dateInput = (value: string) => { const date = new Date(value); if (Number.isNaN(date.valueOf())) return ''; const pad = (n: number) => String(n).padStart(2, '0'); return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}` }

export function AgentQualificationPanel({ orgID, connectionID, provider }: { orgID: string; connectionID: string; provider: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const canWrite = role === 'owner' || role === 'admin'
  const state = useQuery({ queryKey: ['org', orgID, 'connections', connectionID, 'agent-qualification'], queryFn: ({ signal }) => agentAPI.qualification(orgID, connectionID, signal) })
  const [draft, setDraft] = useState<AgentQualificationInput>({ evidence_id: '', checked_at: '', expires_at: '', auth_custody: false, native_tool_containment: false, terms: false, topology: false, entitlement: false, quota: false, no_paid_overage: false })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const capabilities = state.data?.capabilities ?? {}
  const valid = draft.evidence_id.trim() && draft.checked_at && draft.expires_at
  const save = async () => { setBusy(true); setError(''); try { await agentAPI.putQualification(orgID, connectionID, { ...draft, evidence_id: draft.evidence_id.trim(), checked_at: new Date(draft.checked_at).toISOString(), expires_at: new Date(draft.expires_at).toISOString() }, csrf); await state.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  const clear = async () => { setBusy(true); setError(''); try { await agentAPI.clearQualification(orgID, connectionID, csrf); await state.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy(false) } }
  return <fieldset aria-label="Agent runtime qualification"><legend>Agent runtime · {provider}</legend>
    {state.isLoading ? <p className="table-meta">Loading qualification…</p> : state.error ? <p className="error-text" role="alert">Qualification unavailable: {message(state.error)} <Button onClick={() => void state.refetch()}>Retry</Button></p> : <>
      <ul className="compact-list">{Object.entries(capabilities).map(([name, capability]) => <li key={name}><StatusBadge label={capability.state} tone={capability.state === 'supported' ? 'green' : capability.state === 'unsupported' ? 'red' : 'amber'} /> <strong>{name}</strong>: {capability.reason}</li>)}</ul>
      <p className="table-meta">The route stays disabled until a dated qualification matches this runtime, account, model and topology. A login is not entitlement evidence.</p>
      {canWrite && <details><summary>Record qualification evidence</summary><div className="form-grid">
        <label>Evidence reference<input value={draft.evidence_id} onChange={event => setDraft({ ...draft, evidence_id: event.target.value })} /></label>
        <label>Verified at<input type="datetime-local" value={draft.checked_at} onChange={event => setDraft({ ...draft, checked_at: event.target.value })} /></label>
        <label>Expires at<input type="datetime-local" value={draft.expires_at} onChange={event => setDraft({ ...draft, expires_at: event.target.value })} /></label>
        {flags.map(([key, label]) => <label key={key} className="checkbox-label"><input type="checkbox" checked={draft[key]} onChange={event => setDraft({ ...draft, [key]: event.target.checked })} /> {label}</label>)}
      </div><div className="row-actions"><Button disabled={!csrf || busy || !valid} onClick={() => void save()}>{busy ? 'Saving…' : 'Save qualification'}</Button>{state.data?.qualification && <Button disabled={!csrf || busy} onClick={() => void clear()}>Clear qualification</Button>}</div></details>}
    </>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </fieldset>
}
