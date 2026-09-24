import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { githubAppAPI, isSameOriginPath, type GitHubAppSetupInput } from '../api/github-app'
import { Button } from '../components/Accessible'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const orgPattern = /^[A-Za-z0-9-]{1,39}$/

const reasonHelp: Record<string, string> = {
  public_https_required: 'Set a public HTTPS URL for this deployment, then retry. Localhost installs cannot receive GitHub webhooks.',
  not_configured: 'The hosted GitHub App is not configured. Ask an operator to set the GitHub App environment variables.',
  github_app_unavailable: 'The hosted GitHub App is not available on this server. Ask an operator to configure it.',
}

function appsLink(githubOrg: string) {
  const slug = githubOrg.trim()
  return slug && orgPattern.test(slug) ? `https://github.com/organizations/${slug}/settings/apps` : 'https://github.com/settings/apps'
}

type GitHubAppSetupProps = {
  orgID: string
  csrf: string
  onUseToken: () => void
  onBusyChange?: (busy: boolean) => void
}

export function GitHubAppSetup({ orgID, csrf, onUseToken, onBusyChange }: GitHubAppSetupProps) {
  const client = useQueryClient()
  const [name, setName] = useState('Reforge')
  const [githubOrg, setGithubOrg] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const orgRef = useRef(orgID)
  const mounted = useRef(true)
  const status = useQuery({
    queryKey: ['org', orgID, 'github-app'],
    queryFn: ({ signal }) => githubAppAPI.status(orgID, signal),
  })

  useEffect(() => () => { mounted.current = false }, [])
  useEffect(() => { onBusyChange?.(!!busy) }, [busy, onBusyChange])
  useEffect(() => () => { onBusyChange?.(false) }, [onBusyChange])
  useEffect(() => {
    if (orgRef.current === orgID) return
    orgRef.current = orgID
    setBusy('')
    setError('')
  }, [orgID])

  const refresh = () => { void client.invalidateQueries({ queryKey: ['org', orgID, 'github-app'] }) }
  const mode = status.data?.mode
  const hosted = mode === 'hosted'

  const start = async () => {
    if (!csrf) return
    const myOrg = orgID
    const manifest = !hosted
    if (manifest && githubOrg.trim() && !orgPattern.test(githubOrg.trim())) {
      setError('GitHub organisation must be 1–39 letters, numbers or hyphens.')
      return
    }
    setBusy('start')
    setError('')
    const input: GitHubAppSetupInput = { name: name.trim() || 'Reforge', ...(manifest && githubOrg.trim() ? { github_org: githubOrg.trim() } : {}) }
    try {
      const setup = await githubAppAPI.start(orgID, input, csrf)
      if (!mounted.current || orgRef.current !== myOrg) return
      if (!isSameOriginPath(setup.handoff_url)) throw new Error('The server returned an unsafe handoff address.')
      window.location.assign(setup.handoff_url)
    } catch (reason) {
      if (!mounted.current || orgRef.current !== myOrg) return
      setError(message(reason))
      setBusy('')
      refresh()
    }
  }

  const cancel = async (setupID: string) => {
    if (!csrf) return
    const myOrg = orgID
    setBusy('cancel')
    setError('')
    try {
      await githubAppAPI.cancel(orgID, setupID, csrf)
      if (!mounted.current || orgRef.current !== myOrg) return
      refresh()
    } catch (reason) {
      if (!mounted.current || orgRef.current !== myOrg) return
      setError(message(reason))
    } finally {
      if (mounted.current && orgRef.current === myOrg) setBusy('')
    }
  }

  if (status.isLoading) return <p className="table-meta" role="status">Checking GitHub App setup…</p>
  if (status.error || !status.data) return <p className="error-text" role="alert">GitHub setup unavailable: {message(status.error)} <Button type="button" onClick={refresh}>Retry</Button></p>
  const state = status.data

  if (state.mode === 'unavailable') {
    return <section aria-label="GitHub App unavailable">
      <p><strong>GitHub App setup is unavailable.</strong> {reasonHelp[state.reason ?? ''] ?? 'An operator must configure the GitHub App before guided setup can run.'}</p>
      <div className="row-actions">
        <Button type="button" onClick={onUseToken}>Use a personal access token</Button>
        <a className="button" href={appsLink(githubOrg)} target="_blank" rel="noreferrer">Manage GitHub Apps</a>
      </div>
    </section>
  }

  if (state.pending) {
    const pending = state.pending
    const resume = pending.resume_url
    return <section aria-label="GitHub App setup pending">
      <p><strong>Setup in progress.</strong> {pending.app_slug ? `App ${pending.app_slug}` : 'GitHub App'} · Expires {new Date(pending.expires_at).toLocaleString()}.</p>
      {error && <p className="error-text" role="alert">{error}</p>}
      <div className="row-actions">
        {resume && isSameOriginPath(resume) && <Button type="button" className="button button-primary" disabled={!!busy} onClick={() => window.location.assign(resume)}>Resume setup</Button>}
        <Button type="button" disabled={busy === 'cancel' || !csrf} onClick={() => void cancel(pending.id)}>{busy === 'cancel' ? 'Cancelling…' : 'Cancel setup'}</Button>
        <a className="button" href={appsLink(githubOrg)} target="_blank" rel="noreferrer">Manage GitHub Apps</a>
      </div>
    </section>
  }

  return <section aria-label="GitHub App setup">
    {state.reason && <p className="error-text" role="alert">{reasonHelp[state.reason] ?? 'Setup could not be prepared. Try again or use a token.'}</p>}
    <div className="form-grid">
      <label>{hosted ? 'Connection name' : 'App name'}<input value={name} maxLength={160} disabled={!!busy} onChange={event => setName(event.target.value)} /></label>
      {!hosted && <label>GitHub organisation<input value={githubOrg} disabled={!!busy} onChange={event => setGithubOrg(event.target.value)} placeholder="Optional; personal account if empty" /></label>}
    </div>
    {error && <p className="error-text" role="alert">{error}</p>}
    <div className="row-actions">
      <Button type="button" className="button button-primary" disabled={!!busy || !csrf} title={!csrf ? 'Sign in again to start GitHub setup.' : undefined} onClick={() => void start()}>{busy === 'start' ? 'Starting…' : hosted ? 'Connect GitHub' : 'Set up GitHub App'}</Button>
    </div>
  </section>
}
