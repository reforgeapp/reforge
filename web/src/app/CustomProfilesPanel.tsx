import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatusBadge } from '../components/Status'
import { customProfileAPI, type CustomProfile, type CustomProfileInput } from '../custom-profile-api'
import { useSession } from './query'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const empty = { name: '', image_digest: '', executable: '', argv: '', max_wall_seconds: 60, max_output_bytes: 1048576, max_turns: 4, concurrency: 1 }
const tone = (state: string) => state === 'approved' ? 'green' as const : state === 'revoked' ? 'red' as const : 'amber' as const

export function CustomProfilesPanel({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const canWrite = role === 'owner' || role === 'admin'
  const profiles = useQuery({ queryKey: ['org', orgID, 'custom-profiles'], queryFn: ({ signal }) => customProfileAPI.list(orgID, undefined, signal) })
  const [draft, setDraft] = useState(empty)
  const [evidence, setEvidence] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const items = profiles.data?.items ?? []

  const create = async () => {
    const argv = draft.argv.split('\n').map(value => value.trim()).filter(Boolean)
    const input: CustomProfileInput = { name: draft.name.trim(), image_digest: draft.image_digest.trim(), executable: draft.executable.trim(), argv, protocol_version: 1, max_wall_seconds: Number(draft.max_wall_seconds), max_output_bytes: Number(draft.max_output_bytes), max_turns: Number(draft.max_turns), concurrency: Number(draft.concurrency) }
    setBusy('create'); setError('')
    try { await customProfileAPI.create(orgID, input, csrf); setDraft(empty); await profiles.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy('') }
  }
  const act = async (name: string, profile: CustomProfile, run: () => Promise<unknown>) => { setBusy(name); setError(''); try { await run(); await profiles.refetch() } catch (reason) { setError(message(reason)) } finally { setBusy('') } }
  const valid = draft.name.trim() && /^sha256:[a-f0-9]{64}$/.test(draft.image_digest.trim()) && draft.executable.trim().startsWith('/')

  return <section className="state-card" aria-label="Custom command profiles"><div className="stack">
    <p className="table-meta">Unapproved profiles stay disabled. Approved image digest and fixed argv required.</p>
    {error && <p className="error-text" role="alert">{error}</p>}
    {canWrite && <><Button onClick={() => { setCreateOpen(value => !value); setSelectedID('') }}>{createOpen ? 'Close profile editor' : 'Create draft profile'}</Button>{createOpen && <details open><summary>New profile</summary><div className="form-grid">
      <label>Name<input value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} /></label>
      <label>Image digest<input value={draft.image_digest} onChange={event => setDraft({ ...draft, image_digest: event.target.value })} placeholder="sha256:…" /></label>
      <label>Executable<input value={draft.executable} onChange={event => setDraft({ ...draft, executable: event.target.value })} placeholder="/bin/review" /></label>
      <label>Max wall seconds<input type="number" min={1} max={3600} value={draft.max_wall_seconds} onChange={event => setDraft({ ...draft, max_wall_seconds: Number(event.target.value) })} /></label>
      <label>Max output bytes<input type="number" min={1} max={16777216} value={draft.max_output_bytes} onChange={event => setDraft({ ...draft, max_output_bytes: Number(event.target.value) })} /></label>
      <label>Max turns<input type="number" min={1} max={128} value={draft.max_turns} onChange={event => setDraft({ ...draft, max_turns: Number(event.target.value) })} /></label>
      <label>Concurrency<input type="number" min={1} max={8} value={draft.concurrency} onChange={event => setDraft({ ...draft, concurrency: Number(event.target.value) })} /></label>
      <label className="wide">Fixed argv (one per line)<textarea value={draft.argv} onChange={event => setDraft({ ...draft, argv: event.target.value })} /></label>
    </div><Button disabled={!canWrite || !csrf || busy === 'create' || !valid} onClick={() => void create()}>{busy === 'create' ? 'Creating…' : 'Save draft profile'}</Button></details>}</>}
    {profiles.isLoading ? <p className="table-meta">Loading profiles…</p> : profiles.error ? <p className="error-text" role="alert">Profiles unavailable: {message(profiles.error)} <Button onClick={() => void profiles.refetch()}>Retry</Button></p> :
      <DataTable caption="Approved profiles"><table><thead><tr><th>Profile</th><th>State</th><th>Image</th><th>Executable</th><th>Limits</th><th>Actions</th></tr></thead><tbody>{items.map(profile => <tr key={profile.id}>
        <td><Button className="link-button" onClick={() => { setSelectedID(profile.id); setCreateOpen(false) }}>{profile.name}</Button><small className="table-meta">v{profile.version}</small></td>
        <td><StatusBadge label={profile.revoked_at ? 'revoked' : profile.approved_at ? 'approved' : 'draft'} tone={tone(profile.revoked_at ? 'revoked' : profile.approved_at ? 'approved' : 'draft')} /></td>
        <td><code>{profile.image_digest.slice(0, 19)}…</code></td>
        <td><code>{profile.executable}</code></td>
        <td>{profile.max_wall_seconds}s · {Math.round(profile.max_output_bytes / 1024)}KiB · {profile.max_turns} turns · ×{profile.concurrency}</td>
        <td><Button className="link-button" onClick={() => { setSelectedID(profile.id); setCreateOpen(false) }}>Open</Button></td>
      </tr>)}</tbody></table>{!items.length && <EmptyTable label="No custom command profiles. Draft one, then approve it with evidence." />}</DataTable>}
    {selectedID && items.find(profile => profile.id === selectedID) && (() => { const profile = items.find(item => item.id === selectedID)!; const state = profile.revoked_at ? 'revoked' : profile.approved_at ? 'approved' : 'draft'; return <section className="detail-panel" aria-label="Selected custom profile"><h3>{profile.name}</h3><p><StatusBadge label={state} tone={tone(state)} /> · configuration version {profile.version}</p><dl className="detail-list"><div><dt>Image digest</dt><dd><code>{profile.image_digest}</code></dd></div><div><dt>Executable</dt><dd><code>{profile.executable}</code></dd></div><div><dt>Fixed argv</dt><dd><pre>{profile.argv.join('\n')}</pre></dd></div><div><dt>Limits</dt><dd>{profile.max_wall_seconds}s · {profile.max_output_bytes} bytes · {profile.max_turns} turns · concurrency {profile.concurrency}</dd></div><div><dt>Protocol</dt><dd>v{profile.protocol_version}</dd></div><div><dt>Approval evidence</dt><dd>{profile.approval_evidence || 'Unknown'}</dd></div></dl>{canWrite && !profile.revoked_at && (profile.approved_at ? <Button disabled={!csrf || busy === profile.id} onClick={() => void act(profile.id, profile, () => customProfileAPI.revoke(orgID, profile.id, profile.version, csrf))}>Revoke profile</Button> : <div className="row-actions"><input aria-label={`Approval evidence for ${profile.name}`} value={evidence[profile.id] ?? ''} onChange={event => setEvidence({ ...evidence, [profile.id]: event.target.value })} /><Button disabled={!csrf || busy === profile.id || !(evidence[profile.id] ?? '').trim()} onClick={() => void act(profile.id, profile, () => customProfileAPI.approve(orgID, profile.id, profile.version, (evidence[profile.id] ?? '').trim(), csrf))}>Approve profile</Button></div>)}</section> })()}
  </div></section>
}
