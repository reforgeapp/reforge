import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, ReforgeAPIError, type ConnectionCreate } from '../api/client'
import { Button } from '../components/Accessible'
import { GitHubAppSetup } from './GitHubAppSetup'

const maxPEM = 65536
const maxName = 160
const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const isNetworkError = (value: unknown) => !(value instanceof ReforgeAPIError)

type ForgeProvider = 'github' | 'gitlab' | 'gitea'
type GitHubAuth = 'guided' | 'token' | 'app'
type Phase = 'input' | 'created' | 'uncertain'

const cloudEndpoints: Record<ForgeProvider, string> = {
  github: 'https://api.github.com',
  gitlab: 'https://gitlab.com',
  gitea: 'https://gitea.com',
}

const providerLabels: Record<ForgeProvider, string> = { github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' }

const tokenPages: Record<ForgeProvider, string> = {
  github: 'https://github.com/settings/tokens',
  gitlab: 'https://gitlab.com/-/user_settings/personal_access_tokens',
  gitea: 'https://gitea.com/user/settings/applications',
}

function tokenPage(provider: ForgeProvider, endpoint: string) {
  if (provider === 'github') return tokenPages.github
  let host = ''
  try {
    const url = new URL(endpoint.trim())
    if (url.protocol === 'https:' && !url.username && !url.password) host = url.hostname
  } catch {}
  if (provider === 'gitlab') return host && host !== 'gitlab.com' ? `https://${host}/-/user_settings/personal_access_tokens` : tokenPages.gitlab
  return host && host !== 'gitea.com' ? `https://${host}/user/settings/applications` : tokenPages.gitea
}

const tokenPermissions = 'Read access to repositories (GitHub repo, GitLab read_api, Gitea repo).'

function repositoryURL(endpoint: string) {
  const value = endpoint.trim()
  if (!value || value.endsWith('.git')) return true
  try {
    const url = new URL(value)
    if (!/^(www\.)?(github|gitlab|gitea)\.com$/i.test(url.hostname)) return false
    return url.pathname.split('/').filter(Boolean).length >= 2
  } catch {
    return /^(https?:\/\/)?(www\.)?(github|gitlab|gitea)\.com\/[^/\s]+\/[^/\s]+/i.test(value)
  }
}

function normalisePEM(value: string) { return value.replace(/\r\n?/g, '\n') }

export type CreatedForgeConnection = { id: string; name: string; provider: string }

type ForgeConnectionFormProps = {
  orgID: string
  csrf: string
  kind: 'forge' | 'delivery'
  onClose: () => void
  onReady: (connection: CreatedForgeConnection, repositoryFilter: string) => void
  onBusyChange?: (busy: boolean) => void
}

export function ForgeConnectionForm({ orgID, csrf, kind, onClose, onReady, onBusyChange }: ForgeConnectionFormProps) {
  const [provider, setProvider] = useState<ForgeProvider>('github')
  const [githubAuth, setGithubAuth] = useState<GitHubAuth>('guided')
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState(cloudEndpoints.github)
  const [namespace, setNamespace] = useState('')
  const [secret, setSecret] = useState('')
  const [appID, setAppID] = useState('')
  const [installationID, setInstallationID] = useState('')
  const [pemText, setPemText] = useState('')
  const [pemName, setPemName] = useState('')
  const [caPEM, setCAPEM] = useState('')
  const [repoFilter, setRepoFilter] = useState('')
  const [privateRoute, setPrivateRoute] = useState(false)
  const [poolID, setPoolID] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const [phase, setPhase] = useState<Phase>('input')
  const [created, setCreated] = useState<{ id: string; version: number } | null>(null)
  const [error, setError] = useState('')
  const [testError, setTestError] = useState('')
  const [busy, setBusy] = useState(false)
  const [busyAction, setBusyAction] = useState('')
  const [guidedBusy, setGuidedBusy] = useState(false)
  const seq = useRef(0)
  const orgRef = useRef(orgID)

  const guidedAvailable = provider === 'github' && kind === 'forge'
  const githubApp = provider === 'github' && githubAuth === 'app'
  const guided = guidedAvailable && githubAuth === 'guided'

  const pools = useInfiniteQuery({
    queryKey: ['org', orgID, 'forge-form-runner-pools'],
    queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const runners = useInfiniteQuery({
    queryKey: ['org', orgID, 'forge-form-runners', poolID],
    queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute && !!poolID,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? [])
    .filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now())
    .map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))

  useEffect(() => () => { seq.current++ }, [])
  useEffect(() => { onBusyChange?.(busy || guidedBusy) }, [busy, guidedBusy, onBusyChange])
  useEffect(() => () => { onBusyChange?.(false) }, [onBusyChange])
  useEffect(() => {
    if (orgRef.current === orgID) return
    orgRef.current = orgID
    resetCredentials()
  }, [orgID])

  function resetCredentials() {
    seq.current++
    setProvider('github')
    setEndpoint(cloudEndpoints.github)
    setGithubAuth('guided')
    setSecret('')
    setAppID('')
    setInstallationID('')
    setPemText('')
    setPemName('')
    setNamespace('')
    setCAPEM('')
    setRepoFilter('')
    setPrivateRoute(false)
    setPoolID('')
    setRunnerID('')
    setRouteHost('')
    setCIDRs('')
    setPhase('input')
    setCreated(null)
    setError('')
    setTestError('')
    setBusyAction('')
  }

  const changeProvider = (value: ForgeProvider) => {
    seq.current++
    setProvider(value)
    setEndpoint(cloudEndpoints[value])
    setGithubAuth(value === 'github' && kind === 'forge' ? 'guided' : 'token')
    setSecret('')
    setAppID('')
    setInstallationID('')
    setPemText('')
    setPemName('')
    setNamespace('')
    setCAPEM('')
    setRepoFilter('')
    setPrivateRoute(false)
    setPoolID('')
    setRunnerID('')
    setRouteHost('')
    setCIDRs('')
    setPhase('input')
    setCreated(null)
    setError('')
    setTestError('')
  }

  const changeAuth = (value: GitHubAuth) => {
    seq.current++
    setGithubAuth(value)
    setSecret('')
    setAppID('')
    setInstallationID('')
    setPemText('')
    setPemName('')
    setPhase('input')
    setCreated(null)
    setError('')
    setTestError('')
  }

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    if (file.size > maxPEM) {
      setError('Private key is larger than 64 KiB.')
      setPemText('')
      setPemName('')
      return
    }
    const myGen = seq.current
    const myOrg = orgID
    const raw = await file.text()
    if (myGen !== seq.current || orgRef.current !== myOrg) return
    const value = normalisePEM(raw)
    if (value.length > maxPEM) {
      setError('Private key is larger than 64 KiB.')
      setPemText('')
      setPemName('')
      return
    }
    setError('')
    setPemText(value)
    setPemName(file.name)
  }

  const routeIncomplete = privateRoute && (!runnerID || !routeHost.trim() || !cidrs.trim())
  const repositoryInEndpoint = repositoryURL(endpoint)
  const repoURLError = `That looks like a repository URL. Enter the API address (for example ${cloudEndpoints[provider]}) and put the repository in the repository filter.`
  const pemValue = normalisePEM(pemText).trim()
  const secretValue = githubApp ? pemValue : secret
  const pemTooLarge = pemValue.length > maxPEM
  const missingCredential = githubApp ? (!appID.trim() || !installationID.trim() || !pemValue || pemTooLarge) : !secret
  const canSave = !!csrf && !!name.trim() && name.trim().length <= maxName && !!endpoint.trim() && !missingCredential && !routeIncomplete && !busy && phase === 'input'

  const settings = () => ({
    auth_kind: githubApp ? 'github_app' : 'token',
    billing_route: 'forge',
    ...(githubApp ? { namespace: 'installation' } : namespace.trim() ? { namespace: namespace.trim() } : {}),
    ...(githubApp ? { app_id: appID.trim(), installation_id: installationID.trim() } : {}),
    ...(caPEM.trim() ? { ca_pem: caPEM } : {}),
  })

  const retryTest = async () => {
    if (!created || !csrf) return
    const myGen = seq.current
    const myOrg = orgID
    setBusy(true)
    setBusyAction('retry')
    setTestError('')
    try {
      const latest = await api.getConnection(orgID, created.id)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      const tested = await api.testConnection(orgID, latest.id, latest.version, csrf)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      if (tested.state !== 'healthy') { setTestError(tested.reason || 'The capability test did not report a healthy connection.'); return }
      onReady({ id: created.id, name: name.trim(), provider }, repoFilter.trim())
    } catch (reason) {
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setTestError(message(reason))
    } finally {
      if (myGen === seq.current && orgRef.current === myOrg) { setBusy(false); setBusyAction('') }
    }
  }

  const updateCredentials = async () => {
    if (!created || !csrf) return
    const replacement = githubApp ? normalisePEM(pemText).trim() : secret
    if (!replacement) return
    const myGen = seq.current
    const myOrg = orgID
    setBusy(true)
    setBusyAction('update')
    setTestError('')
    try {
      const latest = await api.getConnection(orgID, created.id)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      const rotated = await api.rotateConnection(orgID, latest.id, latest.version, replacement, csrf)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setSecret('')
      setPemText('')
      setPemName('')
      const tested = await api.testConnection(orgID, rotated.id, rotated.version, csrf)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      if (tested.state !== 'healthy') { setTestError(tested.reason || 'The capability test did not report a healthy connection.'); return }
      onReady({ id: created.id, name: name.trim(), provider }, repoFilter.trim())
    } catch (reason) {
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setTestError(message(reason))
    } finally {
      if (myGen === seq.current && orgRef.current === myOrg) { setBusy(false); setBusyAction('') }
    }
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (repositoryInEndpoint) {
      setError(repoURLError)
      return
    }
    if (!canSave) return
    const myGen = seq.current
    const myOrg = orgID
    setBusy(true)
    setError('')
    const payload: ConnectionCreate = {
      kind,
      provider,
      name: name.trim(),
      endpoint: endpoint.trim(),
      settings: settings(),
      ...(secretValue ? { secret: secretValue } : {}),
      ...(privateRoute ? { private_route: { runner_id: runnerID, host: routeHost.trim(), cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) } } : {}),
    }
    try {
      let connection: Awaited<ReturnType<typeof api.createConnection>>
      try {
        connection = await api.createConnection(orgID, kind, payload, csrf)
      } catch (reason) {
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        if (isNetworkError(reason)) setPhase('uncertain')
        else setError(message(reason))
        return
      }
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setCreated({ id: connection.id, version: connection.version })
      setSecret('')
      setPemText('')
      setPemName('')
      try {
        const tested = await api.testConnection(orgID, connection.id, connection.version, csrf)
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        if (tested.state !== 'healthy') {
          setTestError(tested.reason || 'The capability test did not report a healthy connection.')
          setPhase('created')
          return
        }
        onReady({ id: connection.id, name: connection.name, provider }, repoFilter.trim())
      } catch (reason) {
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        setTestError(message(reason))
        setPhase('created')
      }
    } finally {
      if (myGen === seq.current && orgRef.current === myOrg) setBusy(false)
    }
  }

  if (phase === 'uncertain') {
    return <div className="form-stack forge-connection-created">
      <p role="status">Save outcome unknown. Check Connections before trying again.</p>
      <div className="dialog-actions"><Button type="button" onClick={onClose}>Close</Button><Button type="button" className="button button-primary" onClick={onClose}>View connections</Button></div>
    </div>
  }

  if (phase === 'created' && created) {
    const canUpdate = githubApp ? !!normalisePEM(pemText).trim() : !!secret
    return <form className="form-stack forge-connection-created" onSubmit={event => { event.preventDefault(); void updateCredentials() }}>
      <p role="status">Connection saved; capability test failed.</p>
      {testError && <p className="error-text" role="alert">{testError}</p>}
      <div className="form-grid">
        {githubApp
          ? <>
            <label className="wide">RSA private key file<input type="file" accept=".pem,.key,text/plain" disabled={busy} onChange={event => void handleFile(event.target.files?.[0])} /></label>
            <label className="wide">RSA private key PEM<textarea rows={6} maxLength={maxPEM} value={pemText} disabled={busy} onChange={event => { setPemText(normalisePEM(event.target.value)); setPemName('') }} placeholder="-----BEGIN RSA PRIVATE KEY-----" /></label>
          </>
          : <label className="wide">Personal access token<input type="password" autoComplete="new-password" value={secret} disabled={busy} onChange={event => setSecret(event.target.value)} /></label>}
      </div>
      {pemName && <p className="table-meta" role="status">Loaded {pemName}.</p>}
      {pemTooLarge && <p className="error-text" role="alert">Private key is larger than 64 KiB.</p>}
      <div className="dialog-actions">
        <Button type="button" disabled={busy} onClick={onClose}>Close</Button>
        <Button type="button" disabled={busy || !csrf} onClick={() => void retryTest()}>{busyAction === 'retry' ? 'Testing…' : 'Retry test'}</Button>
        <Button type="submit" className="button button-primary" disabled={busy || !csrf || !canUpdate}>{busyAction === 'update' ? 'Updating…' : 'Update credentials'}</Button>
      </div>
    </form>
  }

  return <form className="form-stack forge-connection-form" onSubmit={submit}>
    <div className="form-grid">
      <label>Provider<select value={provider} disabled={busy} onChange={event => changeProvider(event.target.value as ForgeProvider)}>
        {(['github', 'gitlab', 'gitea'] as const).map(value => <option key={value} value={value}>{providerLabels[value]}</option>)}
      </select></label>
      {provider === 'github' && <label>Authentication<select value={guidedAvailable ? githubAuth : githubAuth === 'guided' ? 'token' : githubAuth} disabled={busy} onChange={event => changeAuth(event.target.value as GitHubAuth)}>
        {guidedAvailable && <option value="guided">GitHub App (guided)</option>}
        <option value="token">Personal access token</option>
        <option value="app">GitHub App (manual)</option>
      </select></label>}
      {(provider === 'gitea' || !guided) && <label className="wide">Name<input required maxLength={maxName} value={name} disabled={busy} onChange={event => setName(event.target.value)} placeholder={`${providerLabels[provider]} connection`} /></label>}
      {provider === 'gitea' && <label className="wide">Server URL<input required type="url" value={endpoint} disabled={busy} onChange={event => setEndpoint(event.target.value)} placeholder="https://gitea.example.com" /></label>}
    </div>

    {guided
      ? <GitHubAppSetup orgID={orgID} csrf={csrf} onUseToken={() => changeAuth('token')} onBusyChange={setGuidedBusy} />
      : <>
        <div className="form-grid">
          {!githubApp && <label className="wide">Personal access token<input type="password" autoComplete="new-password" required value={secret} disabled={busy} onChange={event => setSecret(event.target.value)} /></label>}
          {!githubApp && <p className="table-meta wide"><a href={tokenPage(provider, endpoint)} target="_blank" rel="noreferrer" title={tokenPermissions}>Get token</a></p>}
          {githubApp && <>
            <label>App ID<input required value={appID} disabled={busy} onChange={event => setAppID(event.target.value)} /></label>
            <label>Installation ID<input required value={installationID} disabled={busy} onChange={event => setInstallationID(event.target.value)} /></label>
            <label className="wide">Private key file<input type="file" accept=".pem,.key,text/plain" disabled={busy} onChange={event => void handleFile(event.target.files?.[0])} /></label>
            <label className="wide">Private key PEM<textarea rows={6} maxLength={maxPEM} value={pemText} disabled={busy} onChange={event => { setPemText(normalisePEM(event.target.value)); setPemName('') }} placeholder="-----BEGIN RSA PRIVATE KEY-----" /></label>
          </>}
          {pemName && <p className="table-meta wide" role="status">Loaded {pemName}.</p>}
          {pemTooLarge && <p className="error-text wide" role="alert">Private key is larger than 64 KiB.</p>}
        </div>

        <label className="wide">Repository filter<input value={repoFilter} disabled={busy} onChange={event => setRepoFilter(event.target.value)} placeholder="Optional repository URL or name to filter imported repositories" /></label>
        {repositoryInEndpoint && <p className="error-text" role="alert">{repoURLError}</p>}

        <details className="forge-advanced">
          <summary>Advanced</summary>
          <div className="form-grid">
            {!githubApp && <label>Namespace<input value={namespace} disabled={busy} onChange={event => setNamespace(event.target.value)} placeholder={provider === 'github' ? 'org, user:name or owner/repo' : 'Organisation, group or owner'} /></label>}
            {provider !== 'gitea' && <label>API address<input value={endpoint} disabled={busy} onChange={event => setEndpoint(event.target.value)} /></label>}
            <label className="wide">CA certificate<textarea rows={3} maxLength={65536} value={caPEM} disabled={busy} onChange={event => setCAPEM(event.target.value)} placeholder="Optional PEM CA certificate" /></label>
          </div>
        </details>

        <label className="checkbox-label"><input type="checkbox" checked={privateRoute} disabled={busy} onChange={event => setPrivateRoute(event.target.checked)} /> Private route via enrolled runner</label>
        {privateRoute && <div className="form-grid">
          <label>Runner pool<select aria-label="Runner pool" value={poolID} disabled={busy} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}>
            <option value="">Choose pool</option>
            {poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
          </select></label>
          <label>Runner<select aria-label="Runner" value={runnerID} disabled={busy} onChange={event => setRunnerID(event.target.value)}>
            <option value="">Choose active runner</option>
            {runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}
          </select></label>
          <label>Route host<input value={routeHost} disabled={busy} onChange={event => setRouteHost(event.target.value)} /></label>
          <label className="wide">Approved CIDRs<input value={cidrs} disabled={busy} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>
          {!!pools.error && <p className="error-text wide" role="alert">Runner pools unavailable: {message(pools.error)}</p>}
          {!!runners.error && <p className="error-text wide" role="alert">Runners unavailable: {message(runners.error)}</p>}
          {pools.hasNextPage && <Button type="button" disabled={pools.isFetchingNextPage || busy} onClick={() => void pools.fetchNextPage()}>{pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}
          {runners.hasNextPage && <Button type="button" disabled={runners.isFetchingNextPage || busy} onClick={() => void runners.fetchNextPage()}>{runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}
          {!runnerOptions.length && poolID && !runners.isLoading && <p className="table-meta wide">No active enrolled runners available in selected pool.</p>}
        </div>}

        {routeIncomplete && <p className="table-meta">Choose an active runner, route host and approved CIDR before saving.</p>}
        {error && <p className="error-text" role="alert">{error}</p>}
        <div className="dialog-actions">
          <Button type="button" disabled={busy} onClick={onClose}>Cancel</Button>
          <Button type="submit" className="button button-primary" disabled={!canSave} title={!csrf ? 'Sign in again to create a connection.' : routeIncomplete ? 'Complete private route fields before saving.' : undefined}>{busy ? 'Creating…' : 'Create connection'}</Button>
        </div>
      </>}
  </form>
}
