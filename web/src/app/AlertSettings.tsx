import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { alertsAPI, type AlertSettings as Settings } from '../alerts-api'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const ports = { starttls: '587', tls: '465', none: '25' } as const
const split = (address: string) => { const index = address.lastIndexOf(':'); return index > 0 ? { host: address.slice(0, index), port: address.slice(index + 1) } : { host: address, port: '' } }

export function AlertSettings({ orgID, csrf, canWrite }: { orgID: string; csrf: string; canWrite: boolean }) {
  const query = useQuery({ queryKey: ['org', orgID, 'alerts'], queryFn: ({ signal }) => alertsAPI.get(orgID, signal) })
  const [draft, setDraft] = useState<Settings>(); const [host, setHost] = useState(''); const [port, setPort] = useState(''); const [recipients, setRecipients] = useState(''); const [password, setPassword] = useState<string>(); const [status, setStatus] = useState(''); const [busy, setBusy] = useState(false)
  useEffect(() => { if (query.data) { setDraft(query.data); const parts = split(query.data.smtp_address); setHost(parts.host); setPort(parts.port); setRecipients(query.data.recipients.join(', ')); setPassword(undefined) } }, [query.data])
  if (query.error) return <p className="error-text" role="alert">Alerts unavailable: {errorText(query.error)}</p>
  if (!draft) return <p role="status">Loading alerts…</p>
  const disabled = !canWrite || busy
  const run = async (action: () => Promise<unknown>, done: string) => { setBusy(true); setStatus(''); try { await action(); setStatus(done); await query.refetch() } catch (reason) { setStatus(errorText(reason)) } finally { setBusy(false) } }
  const save = () => run(() => alertsAPI.put(orgID, { ...draft, smtp_address: host.trim() ? `${host.trim()}:${port.trim() || ports[draft.smtp_security]}` : '', recipients: recipients.split(',').map(value => value.trim()).filter(Boolean), smtp_password: password }, csrf), 'Saved')
  return <div className="organisation-workspace alert-settings">
    <label className="checkbox-label"><input type="checkbox" checked={draft.enabled} disabled={disabled} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} /> Email alerts <span className="help-tip" tabIndex={0} title="Sent when a run pauses on budget or limits, a finding is blocked, or something needs a person.">?</span></label>
    <div className="alert-grid">
      <label className="span-2">Host<input value={host} disabled={disabled} placeholder="smtp.example.com" onChange={event => setHost(event.target.value)} /></label>
      <label>Port<input value={port} disabled={disabled} inputMode="numeric" placeholder={ports[draft.smtp_security]} onChange={event => setPort(event.target.value.replace(/\D/g, ''))} /></label>
      <label>Security<select value={draft.smtp_security} disabled={disabled} onChange={event => { const security = event.target.value as Settings['smtp_security']; if (!port || Object.values(ports).includes(port as never)) setPort(ports[security]); setDraft({ ...draft, smtp_security: security }) }}><option value="starttls">STARTTLS</option><option value="tls">TLS (SMTPS)</option><option value="none">None (relay)</option></select></label>
      <label className="span-2">From address<input type="email" value={draft.smtp_from} disabled={disabled} placeholder="reforge@example.com" onChange={event => setDraft({ ...draft, smtp_from: event.target.value })} /></label>
      <label>Username<input value={draft.smtp_username} disabled={disabled || draft.smtp_security === 'none'} autoComplete="off" onChange={event => setDraft({ ...draft, smtp_username: event.target.value })} /></label>
      <label>Password<input type="password" value={password ?? ''} disabled={disabled || draft.smtp_security === 'none'} autoComplete="new-password" placeholder={draft.smtp_password_set ? 'Saved' : ''} onChange={event => setPassword(event.target.value)} /></label>
      <label className="checkbox-label span-4"><input type="checkbox" checked={draft.smtp_verify} disabled={disabled || draft.smtp_security === 'none'} onChange={event => setDraft({ ...draft, smtp_verify: event.target.checked })} /> Verify certificate <span className="help-tip" tabIndex={0} title="Turn off only for an internal relay whose certificate does not match its host name.">?</span></label>
      <label className="span-4">Recipients<input value={recipients} disabled={disabled} placeholder={draft.default_recipients.join(', ') || 'Organisation owners'} onChange={event => setRecipients(event.target.value)} /></label>
    </div>
    <div className="row-actions"><Button className="button button-primary" disabled={disabled || !csrf} onClick={() => void save()}>Save</Button><Button disabled={busy || !csrf || !draft.server_configured} title={draft.server_configured ? undefined : 'Save a mail server first'} onClick={() => void run(() => alertsAPI.test(orgID, csrf), 'Test email sent')}>Send test</Button>{status && <span className="table-meta" role="status">{status}</span>}</div>
  </div>
}
