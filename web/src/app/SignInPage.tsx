import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useMeta } from './query'
import { Button } from '../components/Accessible'
import { BrandMark } from '../components/BrandMark'

const ssoKey = 'reforge-sso-org'
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

function ssoOrg() {
  const requested = new URLSearchParams(window.location.search).get('org') ?? ''
  try {
    if (uuid.test(requested)) window.localStorage.setItem(ssoKey, requested)
    const remembered = window.localStorage.getItem(ssoKey) ?? ''
    return uuid.test(remembered) ? remembered : ''
  } catch {
    return uuid.test(requested) ? requested : ''
  }
}

export function SignInPage() {
  const { data: meta } = useMeta()
  const queryClient = useQueryClient()
  const org = ssoOrg()
  const signIn = (orgID?: string) => { queryClient.removeQueries({ queryKey: ['session'] }); api.login(orgID) }
  const forget = () => { try { window.localStorage.removeItem(ssoKey) } catch {} signIn() }
  return <main className="signin-page"><div className="signin-card">
    <div className="brand brand-dark"><BrandMark /><span>Reforge</span></div>
    <h1>Sign in</h1>
    {meta?.development && <div className="fixture-note" role="status"><strong>Development server</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled.' : 'Authentication is configured for this server.'}</span></div>}
    {org ? <>
      <Button className="button button-primary signin-button" onClick={() => signIn(org)}>Sign in with single sign-on</Button>
      <button type="button" className="signin-alt" onClick={forget}>Use a different account</button>
    </> : <Button className="button button-primary signin-button" onClick={() => signIn()}>Sign in</Button>}
  </div></main>
}
