import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useMeta } from './query'
import { Button } from '../components/Accessible'
import { BrandMark } from '../components/BrandMark'

const ssoKey = 'reforge-sso-org'
const slugPattern = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/

function initialSlug() {
  const requested = (new URLSearchParams(window.location.search).get('org') ?? '').toLowerCase()
  if (slugPattern.test(requested)) return requested
  try { return window.localStorage.getItem(ssoKey) ?? '' } catch { return '' }
}

export function SignInPage() {
  const { data: meta } = useMeta()
  const queryClient = useQueryClient()
  const [slug, setSlug] = useState(initialSlug)
  const [sso, setSSO] = useState(() => slug !== '')
  const unavailable = new URLSearchParams(window.location.search).get('sso') === 'unavailable'
  const go = (value?: string) => { queryClient.removeQueries({ queryKey: ['session'] }); api.login(value) }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const value = slug.trim().toLowerCase()
    try { window.localStorage.setItem(ssoKey, value) } catch {}
    go(value)
  }
  return <main className="signin-page"><div className="signin-card">
    <div className="brand brand-dark"><BrandMark /><span>Reforge</span></div>
    <h1>Sign in</h1>
    {meta?.development && <div className="fixture-note" role="status"><strong>Development server</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled.' : 'Authentication is configured for this server.'}</span></div>}
    {unavailable && <p className="signin-error" role="alert">Single sign-on isn't set up for that organisation.</p>}
    {sso ? <form className="signin-sso" onSubmit={submit}>
      <label>Organisation<input value={slug} onChange={event => setSlug(event.target.value)} placeholder="short-name" autoCapitalize="none" autoComplete="organization" required autoFocus /></label>
      <Button className="button button-primary signin-button" type="submit" disabled={!slugPattern.test(slug.trim().toLowerCase())}>Continue with single sign-on</Button>
    </form> : <Button className="button button-primary signin-button" onClick={() => setSSO(true)}>Sign in with single sign-on</Button>}
    <Button className="button signin-button signin-secondary" onClick={() => go()}>Sign in with a Reforge account</Button>
  </div></main>
}
