import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { chatGPTAPI, continueWithChatGPT } from '../api/chatgpt'
import type { Connection } from '../api/client'

const message = (error: unknown) => error instanceof Error ? error.message : 'ChatGPT connection unavailable.'

function useChatGPTLogin(orgID: string, csrf: string) {
  const controller = useRef<AbortController | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [instruction, setInstruction] = useState('')
  useEffect(() => { setBusy(false); setError(''); setInstruction(''); return () => controller.current?.abort() }, [orgID])
  const connect = async (name: string, connection?: Connection) => {
    controller.current?.abort()
    const pending = new AbortController()
    controller.current = pending
    setBusy(true); setError(''); setInstruction('')
    try {
      if (await continueWithChatGPT(orgID, name, csrf, connection, pending.signal) === 'helper') {
        setInstruction('Open downloaded reforge-chatgpt.py with Python 3 on this computer. Complete sign-in in the browser it opens.')
        setBusy(false)
      }
    }
    catch (error) { if (!pending.signal.aborted) { setError(message(error)); setBusy(false) } }
  }
  return { busy, error, instruction, connect }
}

export function ChatGPTConnectionForm({ orgID, csrf, onClose, onBusyChange }: { orgID: string; csrf: string; onClose: () => void; onBusyChange?: (busy: boolean) => void }) {
  const status = useQuery({ queryKey: ['org', orgID, 'chatgpt-status'], queryFn: ({ signal }) => chatGPTAPI.status(orgID, signal) })
  const [name, setName] = useState('ChatGPT')
  const { busy, error, instruction, connect } = useChatGPTLogin(orgID, csrf)
  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  useEffect(() => () => onBusyChange?.(false), [onBusyChange])
  return <form className="form-stack" onSubmit={event => { event.preventDefault(); void connect(name.trim()) }}>
    <label>Name<input required maxLength={160} value={name} disabled={busy} onChange={event => setName(event.target.value)} /></label>
    {status.data && !status.data.available && <p role="status">{status.data.reason}</p>}
    {status.error && <p role="alert">{message(status.error)} <Button type="button" onClick={() => void status.refetch()}>Retry</Button></p>}
    {error && <p role="alert">{error}</p>}
    {instruction && <p role="status">{instruction}</p>}
    <div className="dialog-actions"><Button type="button" disabled={busy} onClick={onClose}>Cancel</Button><Button type="submit" className="button button-primary" disabled={!csrf || !name.trim() || busy || !status.data?.available}>{busy ? 'Connecting…' : 'Continue with ChatGPT'}</Button></div>
  </form>
}

export function ChatGPTSetupDialog({ orgID, id, csrf, onClose, onSaved }: { orgID: string; id: string; csrf: string; onClose: () => void; onSaved: (connection: Connection) => void }) {
  const setup = useQuery({ queryKey: ['org', orgID, 'chatgpt-setup', id], queryFn: ({ signal }) => chatGPTAPI.setup(orgID, id, signal), retry: false })
  const [model, setModel] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setModel(setup.data?.model ?? setup.data?.models[0]?.id ?? '') }, [setup.data])
  const save = async () => { setBusy(true); setError(''); try { onSaved(await chatGPTAPI.save(orgID, id, model, csrf)) } catch (error) { setError(message(error)) } finally { setBusy(false) } }
  return <Dialog open title="Connect ChatGPT plan" onClose={() => { if (!busy) onClose() }}><div className="form-stack">
    {setup.isLoading && <p role="status">Loading available models…</p>}
    {setup.error && <p role="alert">{message(setup.error)} <Button onClick={() => void setup.refetch()}>Retry</Button></p>}
    {setup.data && <><p>{setup.data.account || setup.data.name}</p><label>Model<select aria-label="Model" value={model} disabled={busy || !!setup.data.model} onChange={event => setModel(event.target.value)}>{setup.data.models.map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}</select></label><p>Repairs use your ChatGPT plan. <a href="https://chatgpt.com/settings/usage" target="_blank" rel="noreferrer">Manage usage</a></p></>}
    {error && <p role="alert">{error}</p>}
    <div className="dialog-actions"><Button disabled={busy} onClick={onClose}>Cancel</Button><Button className="button button-primary" disabled={!csrf || busy || !model || !setup.data?.models.some(item => item.id === model)} onClick={() => void save()}>{busy ? 'Connecting…' : 'Connect plan'}</Button></div>
  </div></Dialog>
}

export function ChatGPTConnectionActions({ orgID, connection, csrf }: { orgID: string; connection: Connection; csrf: string }) {
  const { busy, error, instruction, connect } = useChatGPTLogin(orgID, csrf)
  return <div className="form-stack"><div className="row-actions"><a className="button" href="https://chatgpt.com/settings/usage" target="_blank" rel="noreferrer">Manage usage</a><Button disabled={!csrf || busy} onClick={() => void connect(connection.name, connection)}>{busy ? 'Connecting…' : 'Reconnect ChatGPT'}</Button><span className="help-tip" tabIndex={0} title="Uses your ChatGPT plan. Manage any credit allowance in ChatGPT. After restoring access, test this connection and resume paused runs.">?</span></div>{error && <p role="alert">{error}</p>}{instruction && <p role="status">{instruction}</p>}</div>
}
