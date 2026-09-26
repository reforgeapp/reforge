import { useEffect, useState, type FormEvent } from 'react'
import { ReforgeAPIError } from '../api/client'
import { organisationAPI } from '../organisation-api'
import '../styles/invite.css'
import { BrandMark } from '../components/BrandMark'

function takeInvitationToken() {
  if (typeof window === 'undefined' || window.location.pathname !== '/invite') return ''
  const token = new URLSearchParams(window.location.hash.slice(1)).get('token')
  if (token === null) return ''
  window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
  return token
}

let pendingToken = takeInvitationToken()

type Status = 'ready' | 'starting' | 'retry' | 'invalid'

export function InviteLandingPage() {
  const [token, setToken] = useState(() => pendingToken)
  const [status, setStatus] = useState<Status>('ready')

  useEffect(() => {
    const onHashChange = () => {
      const next = takeInvitationToken()
      if (next) { setToken(next); setStatus('ready') }
    }
    const onPageShow = (event: PageTransitionEvent) => { if (event.persisted) setStatus(current => current === 'starting' ? 'ready' : current) }
    window.addEventListener('hashchange', onHashChange)
    window.addEventListener('pageshow', onPageShow)
    return () => {
      pendingToken = ''
      window.removeEventListener('hashchange', onHashChange)
      window.removeEventListener('pageshow', onPageShow)
    }
  }, [])

  const accept = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!token || status === 'starting') return
    setStatus('starting')
    try {
      const target = new URL((await organisationAPI.redeemInvitation(token)).authorization_url)
      if (target.protocol !== 'https:' && target.protocol !== 'http:') throw new Error('invalid authorization URL')
      pendingToken = ''
      window.location.assign(target.href)
    } catch (reason) {
      if (reason instanceof ReforgeAPIError && !reason.retryable && reason.status < 500 && reason.status !== 429) {
        pendingToken = ''
        setToken('')
        setStatus('invalid')
        return
      }
      setStatus('retry')
    }
  }

  return <main className="invite-page">
    <section className="invite-card" aria-labelledby="invite-title">
      <a className="invite-brand" href="/" aria-label="Reforge home"><BrandMark /><span>Reforge</span></a>
      <h1 id="invite-title">Join your organisation</h1>
      {!token ? <>
        <p role="alert">{status === 'invalid' ? 'Invitation is invalid, expired or already used.' : 'Invitation link is missing or invalid.'}</p>
        <a className="button" href="/sign-in">Sign in</a>
      </> : <form onSubmit={accept}>
        {status === 'retry' && <p className="invite-error" role="alert">Sign-in could not start.</p>}
        <button className="button button-primary" type="submit" disabled={status === 'starting'}>{status === 'starting' ? 'Opening sign-in…' : status === 'retry' ? 'Retry' : 'Accept invitation'}</button>
      </form>}
    </section>
  </main>
}
