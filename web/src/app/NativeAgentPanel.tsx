import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { Connection } from '../api/client'
import { nativeAgentAPI } from '../agent-api'
import { Button } from '../components/Accessible'
import { useSession } from './query'

export function NativeAgentPanel({ orgID, connection, csrf }: { orgID: string; connection: Connection; csrf: string }) {
  const session = useSession()
  const client = useQueryClient()
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [code, setCode] = useState('')
  const [model, setModel] = useState(connection.settings.model ?? '')
  const [saved, setSaved] = useState(false)
  const own = session.data?.user.id === connection.settings.runtime_user_id
  const key = ['org', orgID, 'native-agent', connection.id]
  const status = useQuery({ queryKey: key, queryFn: ({ signal }) => nativeAgentAPI.status(orgID, connection.id, signal), enabled: own, retry: false, refetchOnWindowFocus: false, staleTime: Infinity, refetchInterval: query => ['starting', 'pending'].includes(query.state.data?.state ?? '') ? 2000 : false })
  const state = status.data
  const connected = state?.state === 'connected' || state?.state === 'ready'
  const models = useQuery({ queryKey: [...key, 'models'], queryFn: () => nativeAgentAPI.action(orgID, connection.id, 'models', csrf), enabled: own && state?.state === 'connected' && !!csrf, retry: false, staleTime: Infinity, refetchOnWindowFocus: false })
  const selected = model || models.data?.models?.[0]?.id || ''
  const run = async (action: string) => {
    setBusy(action); setError(''); setSaved(false)
    try {
      await client.cancelQueries({ queryKey: key, exact: true })
      if (action === 'default') { await nativeAgentAPI.useDefault(orgID, connection.id, csrf); setSaved(true) }
      else {
        const input = action === 'verify' ? { model: selected } : action === 'code' ? { login_id: state?.login?.id, code } : {}
        setCode('')
        const next = await nativeAgentAPI.action(orgID, connection.id, action, csrf, input)
        client.setQueryData(key, next)
        if (action === 'login' || action === 'logout') client.removeQueries({ queryKey: [...key, 'models'] })
      }
      await client.invalidateQueries({ queryKey: ['org', orgID, 'connections'] })
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Agent unavailable') }
    finally { setBusy('') }
  }
  if (!own) return <p>This subscription belongs to another organisation member.</p>
  return <section className="form-stack" aria-label="Subscription connection">
    {status.isLoading && <p role="status">Checking subscription…</p>}
    {state?.account && <p>{state.account.email}{state.account.plan ? ` · ${state.account.plan}` : ''}</p>}
    {state?.state === 'starting' && <p role="status">Preparing sign-in…</p>}
    {state?.state === 'pending' && state.login?.url && <>
      <a className="button button-primary" href={state.login.url} target="_blank" rel="noreferrer">Continue with {connection.provider === 'codex' ? 'ChatGPT' : 'Claude'}</a>
      {state.login.code && <p>Enter code: <strong>{state.login.code}</strong></p>}
      {state.code_required && <form className="form-stack" onSubmit={event => { event.preventDefault(); void run('code') }}><label>Approval code<input type="password" autoComplete="off" value={code} maxLength={2048} onChange={event => setCode(event.target.value)} /></label><Button type="submit" disabled={!code || !!busy}>Finish sign-in</Button></form>}
      <p role="status">Waiting for approval…</p>
    </>}
    {connected && <>
      {state.state === 'ready' ? <><p role="status">{state.model || connection.settings.model} · Ready</p><Button className="button button-primary" disabled={!!busy || !csrf} title="Sets organisation default. Explicit repository choices stay in effect." onClick={() => void run('default')}>{busy === 'default' ? 'Saving…' : 'Use for Autopilot'}</Button>{saved && <p role="status">Autopilot default set.</p>}</> : <><label>Model<select value={selected} disabled={!!busy} onChange={event => setModel(event.target.value)}>{models.data?.models?.map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}</select></label><Button className="button button-primary" disabled={!selected || !!busy || !csrf} title="Sends one small request using your subscription to confirm access." onClick={() => void run('verify')}>{busy === 'verify' ? 'Verifying…' : 'Verify subscription'}</Button></>}
      {models.error && <p role="alert">Models unavailable. <Button onClick={() => void models.refetch()}>Retry</Button></p>}
    </>}
    {state?.reason && <p role="status">{state.reason}</p>}
    {(!state || ['disconnected', 'failed'].includes(state.state)) && !status.isLoading && <Button className="button button-primary" disabled={!!busy || !csrf} onClick={() => void run('login')}>{busy === 'login' ? 'Starting…' : 'Sign in'}</Button>}
    {state && !['disconnected', 'failed'].includes(state.state) && <Button disabled={!!busy || !csrf} onClick={() => void run('logout')}>{busy === 'logout' ? 'Disconnecting…' : 'Disconnect'}</Button>}
    {(error || status.error) && <p className="error-text" role="alert">{error || (status.error instanceof Error ? status.error.message : 'Subscription unavailable')}</p>}
  </section>
}

export function NativeAgentForm({ orgID, csrf, onClose, onCreated }: { orgID: string; csrf: string; onClose: () => void; onCreated: () => void }) {
  const client = useQueryClient()
  const [provider, setProvider] = useState('codex')
  const [name, setName] = useState('')
  const [connection, setConnection] = useState<Connection>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const options = useQuery({ queryKey: ['org', orgID, 'native-agent-options'], queryFn: ({ signal }) => nativeAgentAPI.options(orgID, signal), retry: false })
  if (connection) return <div className="form-stack"><NativeAgentPanel orgID={orgID} connection={connection} csrf={csrf} /><div className="dialog-actions"><Button onClick={onCreated}>Done</Button></div></div>
  return <form className="form-stack" onSubmit={event => { event.preventDefault(); setBusy(true); setError(''); void nativeAgentAPI.create(orgID, provider, name, csrf).then(async created => {
      try { const state = await nativeAgentAPI.action(orgID, created.id, 'login', csrf); client.setQueryData(['org', orgID, 'native-agent', created.id], state) }
      catch (reason) { client.setQueryData(['org', orgID, 'native-agent', created.id], { state: 'failed', reason: reason instanceof Error ? reason.message : 'Sign-in unavailable' }) }
      setConnection(created)
    }, reason => setError(reason instanceof Error ? reason.message : 'Connection unavailable')).finally(() => setBusy(false)) }}>
    <label>Agent<select value={provider} disabled={busy} onChange={event => setProvider(event.target.value)}><option value="codex">Codex · ChatGPT subscription</option><option value="claude_code">Claude Code · Claude subscription</option></select></label>
    <label>Name<input value={name} maxLength={160} disabled={busy} placeholder={provider === 'codex' ? 'Codex' : 'Claude Code'} onChange={event => setName(event.target.value)} /></label>
    {options.data?.reason && <p role="status">{options.data.reason}</p>}
    {(error || options.error) && <p className="error-text" role="alert">{error || 'Agent runtime unavailable'}</p>}
    <div className="dialog-actions"><Button type="button" disabled={busy} onClick={onClose}>Cancel</Button><Button type="submit" className="button button-primary" disabled={busy || !csrf || !options.data?.available}>{busy ? 'Preparing…' : 'Continue'}</Button></div>
  </form>
}
