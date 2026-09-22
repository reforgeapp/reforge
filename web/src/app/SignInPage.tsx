import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useMeta } from './query'
import { Button } from '../components/Accessible'

export function SignInPage() {
  const { data: meta } = useMeta()
  const queryClient = useQueryClient()
  return <main className="signin-page"><div className="signin-card"><div className="brand brand-dark"><span className="brand-mark" aria-hidden="true">R</span><span>Reforge</span></div><h1>Sign in</h1><p className="signin-lede">Use your organisation identity provider to continue.</p>{meta?.development && <div className="fixture-note" role="status"><strong>Development server</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled.' : 'Authentication is configured for this server.'}</span></div>}<Button className="button button-primary signin-button" onClick={() => { queryClient.removeQueries({ queryKey: ['session'] }); api.login() }}>Sign in with your organisation</Button><p className="signin-foot">Credentials remain with your identity provider.</p></div></main>
}
