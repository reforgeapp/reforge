import { useState } from 'react'
import { api, ReforgeAPIError } from '../api/client'
import { Button } from '../components/Accessible'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function BootstrapPage({ csrf, onDone }: { csrf: string; onDone: (orgID: string) => void }) {
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const submit = async () => {
    setBusy(true); setError('')
    try {
      const created = await api.bootstrap(name.trim(), token.trim(), csrf)
      setToken('')
      onDone(created.id)
    } catch (reason) {
      setError(reason instanceof ReforgeAPIError && reason.status === 403 ? 'The bootstrap token is invalid, already used or expired.' : message(reason))
    } finally {
      setBusy(false)
    }
  }
  return <main className="centered-page"><section className="state-card" aria-label="Self-hosted bootstrap"><div className="stack">
    <h1>Create the first organisation</h1>
    <p className="table-meta">This is the one-time self-hosted bootstrap. It creates the organisation and makes your account its owner. The token is single-use, expires, and is never stored in the browser.</p>
    <label>Organisation name<input value={name} onChange={event => setName(event.target.value)} autoComplete="off" /></label>
    <label>Bootstrap token<input type="password" value={token} onChange={event => setToken(event.target.value)} autoComplete="off" /></label>
    {error && <p className="error-text" role="alert">{error}</p>}
    <Button className="button button-primary" disabled={busy || !csrf || !name.trim() || !token.trim()} onClick={() => void submit()}>{busy ? 'Creating…' : 'Create organisation'}</Button>
  </div></section></main>
}
