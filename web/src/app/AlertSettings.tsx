import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { alertsAPI, type AlertSettings as Settings } from '../alerts-api'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function AlertSettings({ orgID, csrf, canWrite }: { orgID: string; csrf: string; canWrite: boolean }) {
  const query = useQuery({ queryKey: ['org', orgID, 'alerts'], queryFn: ({ signal }) => alertsAPI.get(orgID, signal) })
  const [draft, setDraft] = useState<Settings>(); const [recipients, setRecipients] = useState(''); const [status, setStatus] = useState(''); const [busy, setBusy] = useState(false)
  useEffect(() => { if (query.data) { setDraft(query.data); setRecipients(query.data.recipients.join(', ')) } }, [query.data])
  if (query.isLoading || !draft) return <p role="status">Loading alerts…</p>
  if (query.error) return <p className="error-text" role="alert">Alerts unavailable: {errorText(query.error)}</p>
  const run = async (action: () => Promise<unknown>, done: string) => { setBusy(true); setStatus(''); try { await action(); setStatus(done); await query.refetch() } catch (reason) { setStatus(errorText(reason)) } finally { setBusy(false) } }
  const save = () => run(() => alertsAPI.put(orgID, { ...draft, recipients: recipients.split(',').map(value => value.trim()).filter(Boolean) }, csrf), 'Saved')
  return <div className="organisation-workspace"><div className="stack organisation-detail">
    {!draft.server_configured && <p className="table-meta">Server email is not configured. Set <code>REFORGE_SMTP_ADDRESS</code> and <code>REFORGE_SMTP_FROM</code> to send alerts.</p>}
    <label className="checkbox-label"><input type="checkbox" checked={draft.enabled} disabled={!canWrite || busy} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} /> Email alerts <span className="help-tip" tabIndex={0} title="Sent when a run pauses on budget or limits, a finding is blocked, or a fix needs a person to review or merge.">?</span></label>
    <label className="organisation-field">Recipients<input value={recipients} disabled={!canWrite || busy} placeholder={draft.default_recipients.join(', ') || 'Organisation owners'} onChange={event => setRecipients(event.target.value)} /></label>
    <div className="row-actions"><Button className="button button-primary" disabled={!canWrite || busy || !csrf} onClick={() => void save()}>Save</Button><Button disabled={busy || !csrf || !draft.server_configured} onClick={() => void run(() => alertsAPI.test(orgID, csrf), 'Test email sent')}>Send test</Button></div>
    {status && <p className="table-meta" role="status">{status}</p>}
  </div></div>
}
