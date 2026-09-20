import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { api } from '../api/client'
import { useMeta, useSession } from './query'
import { StatePanel } from '../components/StatePanel'
import { Button } from '../components/Accessible'

export function SignInPage() {
  const session = useSession()
  const { data: meta } = useMeta()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  useEffect(() => {
    if (session.data?.organisations[0]) navigate({ to: '/org/$orgID/$section', params: { orgID: session.data.organisations[0].id, section: 'overview' }, search: { q: undefined } })
  }, [navigate, session.data])
  if (session.isLoading) return <main className="centered-page"><StatePanel kind="loading" title="Checking sign-in" detail="Reforge is checking your existing session." /></main>
  return <main className="signin-page"><div className="signin-card"><div className="brand brand-dark"><span className="brand-mark" aria-hidden="true">R</span><span>Reforge</span></div><p className="eyebrow">Engineering portfolio operations</p><h1>Keep maintenance work moving.</h1><p className="signin-lede">Connect repositories, review bounded repairs, and follow changes through delivery with the controls your organisation defines.</p>{meta?.development && <div className="fixture-note" role="status"><strong>Development server</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled.' : 'Authentication is configured for this server.'}</span></div>}{session.error && !(session.error as { status?: number }).status && <p className="error-text" role="alert">{session.error.message}</p>}<Button className="button button-primary signin-button" onClick={() => { queryClient.removeQueries({ queryKey: ['session'] }); api.login() }}>Sign in with your organisation</Button><p className="signin-foot">Sign-in is handled by your organisation's identity provider. This browser does not retain credentials.</p></div></main>
}
