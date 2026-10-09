import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { alertsAPI, type AlertSettings as Settings } from '../alerts-api'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function AlertSettings({ orgID, csrf, canWrite }: { orgID: string; csrf: string; canWrite: boolean }) {
  const query = useQuery({ queryKey: ['org', orgID, 'alerts'], queryFn: ({ signal }) => alertsAPI.get(orgID, signal) })
  const [draft, setDraft] = useState<Settings>(); const [recipients, setRecipients] = useState(''); const [password, setPassword] = useState<string>(); const [status, setStatus] = useState(''); const [busy, setBusy] = useState(false)
  useEffect(() => { if (query.data) { setDraft(query.data); setRecipients(query.data.recipients.join(', ')); setPassword(undefined) } }, [query.data])
  if (query.isLoading || !draft) return <p role="status">Loading alerts…</p>
  if (query.error) return <p className="error-text" role="alert">Alerts unavailable: {errorText(query.error)}</p>
  const run = async (action: () => Promise<unknown>, done: string) => { setBusy(true); setStatus(''); try { await action(); setStatus(done); await query.refetch() } catch (reason) { setStatus(errorText(reason)) } finally { setBusy(false) } }
  const save = () => run(() => alertsAPI.put(orgID, { ...draft, recipients: recipients.split(',').map(value => value.trim()).filter(Boolean), smtp_password: password }, csrf), 'Saved')
  return <div className="organisation-workspace"><div className="stack organisation-detail">
    {!draft.server_configured && <p className="table-meta">Add a mail server to send alerts.</p>}
    <label className="checkbox-label"><input type="checkbox" checked={draft.enabled} disabled={!canWrite || busy} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} /> Email alerts <span className="help-tip" tabIndex={0} title="Sent when a run pauses on budget or limits, a finding is blocked, or a fix needs a person to review or merge.">?</span></label>
    <div className="form-grid">
      <label>Mail server <span className="help-tip" tabIndex={0} title="SMTP host and port, for example smtp.example.com:587. STARTTLS is used when the server offers it.">?</span><input value={draft.smtp_address} disabled={!canWrite || busy} placeholder="smtp.example.com:587" onChange={event => setDraft({ ...draft, smtp_address: event.target.value })} /></label>
      <label>From address<input type="email" value={draft.smtp_from} disabled={!canWrite || busy} placeholder="reforge@example.com" onChange={event => setDraft({ ...draft, smtp_from: event.target.value })} /></label>
      <label>Username<input value={draft.smtp_username} disabled={!canWrite || busy} autoComplete="off" onChange={event => setDraft({ ...draft, smtp_username: event.target.value })} /></label>
      <label>Password<input type="password" value={password ?? ''} disabled={!canWrite || busy} autoComplete="new-password" placeholder={draft.smtp_password_set ? 'Saved; type to replace' : ''} onChange={event => setPassword(event.target.value)} /></label>
    </div>
    <label className="organisation-field">Recipients<input value={recipients} disabled={!canWrite || busy} placeholder={draft.default_recipients.join(', ') || 'Organisation owners'} onChange={event => setRecipients(event.target.value)} /></label>
    <div className="row-actions"><Button className="button button-primary" disabled={!canWrite || busy || !csrf} onClick={() => void save()}>Save</Button><Button disabled={busy || !csrf || !draft.server_configured} onClick={() => void run(() => alertsAPI.test(orgID, csrf), 'Test email sent')}>Send test</Button></div>
    {status && <p className="table-meta" role="status">{status}</p>}
  </div></div>
}
