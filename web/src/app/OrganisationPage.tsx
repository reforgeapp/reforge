import { useEffect, useState, type FormEvent } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { api, ReforgeAPIError } from '../api/client'
import { organisationAPI, type CreatedMemberInvitation, type Membership, type OrgOIDCSettings, type OrgOIDCInvitation, type CreatedOrgOIDCInvitation } from '../organisation-api'
import '../styles/organisation.css'
import { useSession } from './query'
import { Tabs } from '../components/Workspace'
import { AlertSettings } from './AlertSettings'

const roles = ['owner', 'admin', 'maintainer', 'reviewer', 'viewer'] as const
const errorText = (value: unknown) => value instanceof Error ? value.message : 'Request failed.'
const identityName = (member: Membership, ownID?: string, ownName?: string) => member.user_id === ownID ? ownName || 'You' : `Member ····${member.user_id?.slice(-6) ?? 'unknown'}`

export function OrganisationPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const [tab, setTab] = useState<'teams' | 'members' | 'identity' | 'alerts'>('teams')

  if (!session.data && session.isPending) return <StatePanel kind="loading" title="Loading organisation access" detail="" />
  if (!session.data && session.error) return <StatePanel kind="error" title="Organisation access unavailable" detail={errorText(session.error)} action={<Button onClick={() => void session.refetch()}>Retry</Button>} />
  if (role !== 'owner' && role !== 'admin') return <StatePanel kind="blocked" title="Organisation access restricted" detail="Ask an organisation owner or administrator to manage this organisation." />

  const items = [{ id: 'teams', label: 'Teams' }, ...(role === 'owner' ? [{ id: 'members', label: 'Members' }, { id: 'identity', label: 'Identity' }] : []), { id: 'alerts', label: 'Alerts' }]
  const activeTab = items.some(item => item.id === tab) ? tab : 'teams'

  return <div className="organisation-page">
    <Tabs id="organisation-workspace" label="Organisation sections" items={items} value={activeTab} onChange={value => setTab(value as typeof tab)} />
    {activeTab === 'teams' && <section id="organisation-workspace-panel-teams"><TeamsSection orgID={orgID} /></section>}
    {activeTab === 'members' && <section id="organisation-workspace-panel-members"><MembersSection orgID={orgID} /></section>}
    {activeTab === 'identity' && <section id="organisation-workspace-panel-identity"><IdentitySettings orgID={orgID} /></section>}
    {activeTab === 'alerts' && <section id="organisation-workspace-panel-alerts"><AlertSettings orgID={orgID} csrf={session.data?.csrf_token ?? ''} canWrite={role === 'owner'} /></section>}
  </div>
}


function IdentitySettings({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const csrf = session.data?.csrf_token ?? ''
  const queryKey = ['org', orgID, 'identity', 'oidc']
  const query = useQuery({ queryKey, queryFn: ({ signal }) => organisationAPI.oidc(orgID, signal), staleTime: 0, refetchOnMount: 'always', refetchOnWindowFocus: true })
  const [issuer, setIssuer] = useState('')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [confirmDisable, setConfirmDisable] = useState(false)
  const settings = query.data
  const dirty = !!settings && (issuer !== (settings.issuer ?? '') || clientID !== (settings.client_id ?? '') || !!clientSecret)
  const status = dirty ? 'Unsaved changes' : settings?.status === 'active' ? 'Login active' : settings?.status === 'probe_verified' ? 'Metadata verified' : settings?.status === 'disabled' ? 'Disabled' : settings?.configured ? 'Draft' : 'Not configured'

  useEffect(() => {
    if (!settings) return
    setIssuer(settings.issuer ?? '')
    setClientID(settings.client_id ?? '')
    setClientSecret('')
  }, [settings?.version])

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!settings || !issuer.trim() || !clientID.trim() || (!settings.secret_present && !clientSecret)) return
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const updated = await organisationAPI.putOIDC(orgID, settings.version, { issuer: issuer.trim(), client_id: clientID.trim(), client_secret: clientSecret }, csrf)
      client.setQueryData(queryKey, updated)
      setClientSecret('')
      setNotice('Draft saved')
    } catch (reason) {
      setError(errorText(reason))
      void query.refetch()
    } finally {
      setBusy(false)
    }
  }

  const probe = async () => {
    if (!settings || dirty) return
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const updated = await organisationAPI.probeOIDC(orgID, settings.version, csrf)
      client.setQueryData(queryKey, updated)
      setNotice('')
    } catch (reason) {
      if (reason instanceof ReforgeAPIError && reason.code === 'issuer_unverified') {
        const current = client.getQueryData<OrgOIDCSettings>(queryKey)
        if (current) client.setQueryData(queryKey, { ...current, status: 'draft', verified: false, verified_at: undefined })
      }
      setError(errorText(reason))
      void query.refetch()
    } finally {
      setBusy(false)
    }
  }

  const activate = async () => {
    if (!settings?.activation_available) return
    setBusy(true)
    setError('')
    try {
      await organisationAPI.activateOIDC(orgID, settings.version, csrf)
      await query.refetch()
    } catch (reason) {
      setError(errorText(reason))
      void query.refetch()
    } finally {
      setBusy(false)
    }
  }

  const disable = async () => {
    if (!settings) return
    setBusy(true)
    setError('')
    try {
      const updated = await organisationAPI.disableOIDC(orgID, settings.version, csrf)
      client.setQueryData(queryKey, updated)
      setConfirmDisable(false)
      setNotice('Configuration disabled')
    } catch (reason) {
      setError(errorText(reason))
      void query.refetch()
    } finally {
      setBusy(false)
    }
  }

  if (query.isPending) return <p className="organisation-state" role="status">Loading identity settings…</p>
  if (query.error && !settings) return <div className="organisation-state" role="alert"><span>Identity settings unavailable: {errorText(query.error)}</span><Button onClick={() => void query.refetch()}>Retry</Button></div>

  return <div className="identity-settings">
    <ShortName orgID={orgID} />
    {query.error && <p className="organisation-error" role="alert">Refresh failed: {errorText(query.error)} <Button onClick={() => void query.refetch()}>Retry</Button></p>}
    {error && <p className="organisation-error" role="alert">{error}</p>}
    <form className="identity-form" onSubmit={save}>
      <div className="identity-status"><span className="identity-badges"><StatusBadge label={status} tone={!dirty && settings?.verified ? 'green' : !dirty && settings?.status === 'disabled' ? 'neutral' : 'amber'} />{settings?.secret_present && <StatusBadge label="Secret saved" tone="green" />}</span><Button type="button" className="identity-refresh" disabled={busy || query.isFetching} onClick={() => void query.refetch()}>Refresh</Button></div>
      <label className="organisation-field">Issuer URL<input name="issuer" type="url" autoComplete="url" value={issuer} onChange={event => setIssuer(event.target.value)} maxLength={2048} required disabled={busy} /></label>
      <label className="organisation-field">Client ID<input name="client_id" autoComplete="off" value={clientID} onChange={event => setClientID(event.target.value)} maxLength={512} required disabled={busy} /></label>
      <label className="organisation-field">Client secret<input name="client_secret" type="password" autoComplete="new-password" value={clientSecret} onChange={event => setClientSecret(event.target.value)} placeholder={settings?.secret_present ? 'Leave blank to keep current' : ''} required={!settings?.secret_present} disabled={busy} /></label>
      <footer className="identity-actions">
        <span>{notice || (settings?.verified_at ? `Metadata checked ${new Date(settings.verified_at).toLocaleString()}` : '')}</span>
        <Button type="submit" disabled={busy || !csrf || !dirty || !issuer.trim() || !clientID.trim() || (!settings?.secret_present && !clientSecret)}>{busy ? 'Saving…' : 'Save draft'}</Button>
      </footer>
    </form>
    <div className="identity-actions identity-operations">
      {settings?.configured && <Button type="button" disabled={busy || !csrf || settings.status === 'disabled' || dirty || query.isFetching} onClick={() => void probe()}>{busy ? 'Working…' : 'Probe issuer metadata'}</Button>}
      {settings?.status === 'probe_verified' && settings.activation_available && !dirty && <Button type="button" disabled={busy || !csrf} onClick={() => void activate()}>Activate login</Button>}
      {settings?.configured && settings.status !== 'disabled' && <Button type="button" className="button button-danger" disabled={busy || !csrf || dirty} onClick={() => setConfirmDisable(true)}>Disable</Button>}
    </div>
    {settings?.status === 'active' ? <InvitationsSection orgID={orgID} csrf={csrf} /> : <section className="identity-invitations"><h2>Invitations</h2><p>Activate organisation login to invite people.</p></section>}
    <Dialog open={confirmDisable} title="Disable organisation login" onClose={() => setConfirmDisable(false)}><p>Disable this identity configuration?</p><div className="organisation-form-actions"><Button type="button" onClick={() => setConfirmDisable(false)}>Cancel</Button><Button className="button button-danger" type="button" disabled={busy || !csrf} onClick={() => void disable()}>{busy ? 'Disabling…' : 'Disable'}</Button></div></Dialog>
  </div>
}

function InvitationsSection({ orgID, csrf }: { orgID: string; csrf: string }) {
  const client = useQueryClient()
  const queryKey = ['org', orgID, 'identity', 'oidc', 'invitations']
  const invitations = useInfiniteQuery({ queryKey, queryFn: ({ pageParam, signal }) => organisationAPI.invitations(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const items = invitations.data?.pages.flatMap(page => page.items) ?? []
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Membership['role']>('viewer')
  const [expiryDays, setExpiryDays] = useState('7')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [created, setCreated] = useState<CreatedOrgOIDCInvitation | null>(null)
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState('')
  const [revoking, setRevoking] = useState('')
  const [revokeTarget, setRevokeTarget] = useState<OrgOIDCInvitation | null>(null)
  const [revokeError, setRevokeError] = useState('')

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const result = await organisationAPI.createInvitation(orgID, { email: email.trim(), role, expires_at: new Date(Date.now() + Number(expiryDays) * 86400000 - 60_000).toISOString() }, csrf)
      setEmail('')
      setCreated(result)
      setCopied(false)
      setCopyError('')
      await client.invalidateQueries({ queryKey })
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }

  const revoke = async (item: OrgOIDCInvitation) => {
    setRevoking(item.id)
    setRevokeError('')
    try {
      await organisationAPI.revokeInvitation(orgID, item.id, csrf)
      setRevokeTarget(null)
      await client.invalidateQueries({ queryKey })
    } catch (reason) {
      setRevokeError(errorText(reason))
    } finally {
      setRevoking('')
    }
  }

  const copy = async () => {
    if (!created) return
    setCopyError('')
    try {
      await navigator.clipboard.writeText(created.redemption_url)
      setCopied(true)
    } catch {
      setCopyError('Clipboard unavailable. Select and copy the link.')
    }
  }

  const closeCreated = () => { setCreated(null); setCopied(false); setCopyError('') }

  return <section className="identity-invitations" aria-labelledby="identity-invitations-heading">
    <header className="identity-invitations-heading"><h2 id="identity-invitations-heading">Invitations</h2><span>{items.length}</span></header>
    {error && <p className="organisation-error" role="alert">{error}</p>}
    <form className="identity-invitation-form" onSubmit={create}>
      <label className="organisation-field">Email<input type="email" name="invitation_email" autoComplete="email" maxLength={320} value={email} onChange={event => setEmail(event.target.value)} required disabled={busy} /></label>
      <label className="organisation-field">Role<select value={role} onChange={event => setRole(event.target.value as Membership['role'])} disabled={busy}>{roles.map(item => <option key={item} value={item}>{item}</option>)}</select></label>
      <label className="organisation-field">Expires<select value={expiryDays} onChange={event => setExpiryDays(event.target.value)} disabled={busy}><option value="1">1 day</option><option value="7">7 days</option><option value="30">30 days</option></select></label>
      <Button className="button button-primary" type="submit" disabled={busy || !csrf || !email.trim()}>{busy ? 'Creating…' : 'Create invitation'}</Button>
    </form>
    {invitations.error && <p className="organisation-error" role="alert">{invitations.isFetchNextPageError ? 'More invitations unavailable' : 'Invitations unavailable'}: {errorText(invitations.error)} <Button disabled={invitations.isFetching} onClick={() => void (invitations.isFetchNextPageError ? invitations.fetchNextPage() : invitations.refetch())}>Retry</Button></p>}
    {invitations.isPending ? <p className="organisation-state" role="status">Loading invitations…</p> : !invitations.data ? null : items.length === 0 ? <p className="identity-invitation-empty">No invitations</p> : <div className="identity-invitation-list" aria-label="Invitations">
      {items.map(item => {
        const expired = Date.parse(item.expires_at) <= Date.now()
        const state = item.redeemed ? 'Accepted' : expired ? 'Expired' : 'Pending'
        return <div className="identity-invitation-row" key={item.id}><div className="identity-invitation-main"><strong>{item.email}</strong><span>{item.role} · {state} · expires {new Date(item.expires_at).toLocaleDateString()}</span></div>{!item.redeemed && !expired && <Button className="button button-sm" disabled={revoking === item.id || !csrf} onClick={() => setRevokeTarget(item)}>{revoking === item.id ? 'Revoking…' : 'Revoke'}</Button>}</div>
      })}
      {invitations.hasNextPage && !invitations.isFetchNextPageError && <Button disabled={invitations.isFetchingNextPage} onClick={() => void invitations.fetchNextPage()}>{invitations.isFetchingNextPage ? 'Loading…' : 'Load more'}</Button>}
    </div>}
    <Dialog open={!!created} title="Invitation created" onClose={closeCreated}>
      <div className="identity-invitation-created"><label className="organisation-field">Invitation link<input readOnly autoFocus value={created?.redemption_url ?? ''} onFocus={event => event.currentTarget.select()} /></label><p>Copy this link now. It is shown once.</p>{copyError && <p className="organisation-error" role="alert">{copyError}</p>}<div className="organisation-form-actions"><Button type="button" onClick={closeCreated}>Done</Button><Button className="button button-primary" type="button" onClick={() => void copy()}>{copied ? 'Copied' : 'Copy link'}</Button></div></div>
    </Dialog>
    <Dialog open={!!revokeTarget} title="Revoke invitation" onClose={() => { setRevokeTarget(null); setRevokeError('') }}>
      <div className="organisation-create-form"><p>Revoke {revokeTarget?.email}?</p>{revokeError && <p className="organisation-error" role="alert">{revokeError}</p>}<div className="organisation-form-actions"><Button type="button" disabled={!!revoking} onClick={() => { setRevokeTarget(null); setRevokeError('') }}>Cancel</Button><Button type="button" className="button button-danger" disabled={!!revoking || !csrf} onClick={() => revokeTarget && void revoke(revokeTarget)}>{revoking ? 'Revoking…' : 'Revoke'}</Button></div></div>
    </Dialog>
  </section>
}

function TeamsSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const csrf = session.data?.csrf_token ?? ''
  const query = useInfiniteQuery({ queryKey: ['org', orgID, 'teams', 'list'], queryFn: ({ pageParam, signal }) => organisationAPI.teams(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const repos = useInfiniteQuery({ queryKey: ['org', orgID, 'repositories', 'list'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const teams = query.data?.pages.flatMap(page => page.items) ?? []
  const repositories = repos.data?.pages.flatMap(page => page.items) ?? []
  const [selectedID, setSelectedID] = useState('')
  const selected = teams.find(team => team.id === selectedID)
  const [createOpen, setCreateOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [newName, setNewName] = useState('')
  const [name, setName] = useState('')
  const [repositoryIDs, setRepositoryIDs] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const repositoryStatus = repos.error ? 'Repository scope unknown' : repos.isPending ? 'Loading repository scope' : repos.isFetching ? 'Refreshing repository scope' : repos.hasNextPage ? 'Repository scope partial' : 'Repository scope complete'
  const repositoryTone = repos.error || repos.isPending || repos.isFetching || repos.hasNextPage ? 'amber' : 'green'
  const dirty = !!selected && (name !== selected.name || repositoryIDs.length !== (selected.repository_ids ?? []).length || repositoryIDs.some(id => !(selected.repository_ids ?? []).includes(id)))

  useEffect(() => {
    if (!teams.length) { setSelectedID(''); return }
    if (!teams.some(team => team.id === selectedID)) setSelectedID(teams[0].id)
  }, [teams, selectedID])

  useEffect(() => {
    setName(selected?.name ?? '')
    setRepositoryIDs(selected?.repository_ids ?? [])
    setError('')
  }, [selected?.id, selected?.version])

  const refreshTeams = () => client.invalidateQueries({ queryKey: ['org', orgID, 'teams'] })
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected || !name.trim()) return
    setBusy(true)
    setError('')
    try {
      await organisationAPI.putTeam(orgID, selected.id, selected.version, { name: name.trim(), repository_ids: repositoryIDs }, csrf)
      await refreshTeams()
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (!newName.trim()) return
    setBusy(true)
    setError('')
    try {
      const created = await organisationAPI.putTeam(orgID, crypto.randomUUID(), 0, { name: newName.trim(), repository_ids: [] }, csrf)
      setNewName('')
      setCreateOpen(false)
      await refreshTeams()
      setSelectedID(created.id)
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  const remove = async () => {
    if (!selected) return
    setBusy(true)
    setError('')
    try {
      await organisationAPI.deleteTeam(orgID, selected.id, selected.version, csrf)
      setConfirmDelete(false)
      await refreshTeams()
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  const retryRepositories = () => void (repos.hasNextPage ? repos.fetchNextPage() : repos.refetch())

  if (query.isLoading) return <p className="organisation-state" role="status">Loading teams…</p>
  if (query.error && !query.data) return <div className="organisation-state" role="alert"><span>Teams unavailable: {errorText(query.error)}</span><Button onClick={() => void query.refetch()}>Retry</Button></div>

  return <div className="organisation-workspace">
    <header className="organisation-toolbar"><div><strong>{teams.length}</strong> {teams.length === 1 ? 'team' : 'teams'}</div>{teams.length > 0 && <Button onClick={() => setCreateOpen(true)}>Create team</Button>}</header>
    {query.error && <p className="organisation-error" role="alert">Could not refresh teams: {errorText(query.error)} <Button onClick={() => void query.refetch()}>Retry</Button></p>}
    {error && <p className="organisation-error" role="alert">{error}</p>}
    {teams.length === 0 ? <div className="organisation-empty"><p>No teams yet</p><Button onClick={() => setCreateOpen(true)}>Create team</Button></div> : <div className="organisation-split">
      <nav className="organisation-list" aria-label="Teams">
        {teams.map(team => { const count = (team.repository_ids ?? []).length; return <button key={team.id} className={`organisation-list-row${team.id === selectedID ? ' selected' : ''}`} aria-current={team.id === selectedID ? 'true' : undefined} onClick={() => setSelectedID(team.id)}><span>{team.name}</span><small>{count} {count === 1 ? 'repository' : 'repositories'}</small></button> })}
        {query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? 'Loading…' : 'Load more teams'}</Button>}
      </nav>
      {selected && <form className="organisation-detail" onSubmit={save}>
        <div className="organisation-detail-head"><h2>{selected.name}</h2><Button className="button button-danger" type="button" disabled={busy || !csrf} onClick={() => setConfirmDelete(true)}>Delete</Button></div>
        <label className="organisation-field">Team name<input aria-label={`Team name ${selected.id}`} value={name} maxLength={160} onChange={event => setName(event.target.value)} disabled={busy} required /></label>
        <section className="organisation-scope" aria-labelledby="team-scope-heading">
          <div className="organisation-subhead"><h3 id="team-scope-heading">Repository access</h3><span><StatusBadge label={repositoryStatus} tone={repositoryTone} /></span></div>
          {repos.error && <p className="organisation-error" role="alert">{repos.hasNextPage ? 'More repositories unavailable.' : 'Repository scope unavailable.'} <Button type="button" disabled={repos.isFetching} onClick={retryRepositories}>Retry</Button></p>}
          {repos.isPending ? <p role="status">Loading repositories…</p> : repositories.length ? <div className="organisation-repo-list">{repositories.map(repo => <label key={repo.id} className="organisation-repo"><input type="checkbox" checked={repositoryIDs.includes(repo.id)} onChange={event => setRepositoryIDs(current => event.target.checked ? [...current, repo.id] : current.filter(id => id !== repo.id))} disabled={busy} /><span>{repo.name}</span></label>)}</div> : <p>No repositories available.</p>}
          {repos.hasNextPage && <Button type="button" disabled={repos.isFetchingNextPage} onClick={() => void repos.fetchNextPage()}>{repos.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}
        </section>
        <footer className="organisation-form-actions"><span>{dirty ? 'Unsaved changes' : `Version ${selected.version}`}</span><Button type="submit" disabled={busy || !csrf || !dirty || !name.trim()}>{busy ? 'Saving…' : 'Save changes'}</Button></footer>
      </form>}
    </div>}
    <Dialog open={createOpen} title="Create team" onClose={() => setCreateOpen(false)}><form className="organisation-create-form" onSubmit={create}><label className="organisation-field">Team name<input autoFocus value={newName} maxLength={160} onChange={event => setNewName(event.target.value)} required /></label><div className="organisation-form-actions"><Button type="button" onClick={() => setCreateOpen(false)}>Cancel</Button><Button type="submit" disabled={busy || !csrf || !newName.trim()}>{busy ? 'Creating…' : 'Create team'}</Button></div></form></Dialog>
    <Dialog open={confirmDelete} title="Delete team" onClose={() => setConfirmDelete(false)}><p>Delete {selected?.name}? Members lose access granted by this team.</p><div className="organisation-form-actions"><Button type="button" onClick={() => setConfirmDelete(false)}>Cancel</Button><Button className="button button-danger" type="button" disabled={busy || !csrf} onClick={() => void remove()}>{busy ? 'Deleting…' : 'Delete team'}</Button></div></Dialog>
  </div>
}

function ShortName({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const csrf = session.data?.csrf_token ?? ''
  const current = session.data?.organisations.find(item => item.id === orgID)?.slug ?? ''
  const [slug, setSlug] = useState(current)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  useEffect(() => setSlug(current), [current])
  const link = current ? `${window.location.origin}/sign-in?org=${encodeURIComponent(current)}` : ''
  const save = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await organisationAPI.putSlug(orgID, slug.trim().toLowerCase(), csrf)
      await client.invalidateQueries({ queryKey: ['session'] })
    } catch (reason) {
      setError(reason instanceof ReforgeAPIError && reason.status === 409 ? 'That short name is already in use.' : errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  const copy = async () => { try { await navigator.clipboard.writeText(link); setCopied(true) } catch {} }
  return <form className="identity-short-name" onSubmit={save}>
    <label className="organisation-field">Short name <span className="help-tip" tabIndex={0} title="Members enter this on the sign-in page to use your single sign-on. Lowercase letters, numbers and hyphens.">?</span><input value={slug} onChange={event => setSlug(event.target.value)} disabled={busy || !csrf} placeholder="acme" autoCapitalize="none" /></label>
    <Button type="submit" disabled={busy || !csrf || slug.trim().toLowerCase() === current}>{busy ? 'Saving…' : 'Save'}</Button>
    {link && <span className="identity-signin-link"><code>{link}</code><Button type="button" onClick={() => void copy()}>{copied ? 'Copied' : 'Copy sign-in link'}</Button></span>}
    {error && <p className="organisation-error" role="alert">{error}</p>}
  </form>
}

function InviteMember({ orgID, csrf }: { orgID: string; csrf: string }) {
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Membership['role']>('maintainer')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [created, setCreated] = useState<CreatedMemberInvitation>()
  const close = () => { setOpen(false); setEmail(''); setError(''); setCreated(undefined) }
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      setCreated(await organisationAPI.inviteMember(orgID, { email: email.trim(), role }, csrf))
      await client.invalidateQueries({ queryKey: ['org', orgID, 'member-invitations'] })
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  return <>
    <Button className="button button-primary" disabled={!csrf} onClick={() => setOpen(true)}>Invite</Button>
    <Dialog open={open} title="Invite member" onClose={close}>
      {created ? <div className="organisation-invite-result">
        <p role="status">{created.email_sent ? `Invitation sent to ${email.trim()}.` : 'Email could not be sent. Share this link; it works once and expires in 7 days.'}</p>
        {!created.email_sent && <input readOnly value={created.link} onFocus={event => event.target.select()} aria-label="Invitation link" />}
        <div className="organisation-form-actions"><Button type="button" onClick={close}>Done</Button></div>
      </div> : <form onSubmit={submit}>
        <label className="organisation-field">Email<input type="email" required value={email} onChange={event => setEmail(event.target.value)} disabled={busy} autoFocus /></label>
        <label className="organisation-field">Role<select value={role} onChange={event => setRole(event.target.value as Membership['role'])} disabled={busy}>{roles.map(item => <option key={item} value={item}>{item}</option>)}</select></label>
        {error && <p className="organisation-error" role="alert">{error}</p>}
        <div className="organisation-form-actions"><Button type="button" onClick={close}>Cancel</Button><Button className="button button-primary" type="submit" disabled={busy || !email.trim()}>{busy ? 'Sending…' : 'Send invitation'}</Button></div>
      </form>}
    </Dialog>
  </>
}

function PendingInvitations({ orgID, csrf }: { orgID: string; csrf: string }) {
  const client = useQueryClient()
  const invitations = useQuery({ queryKey: ['org', orgID, 'member-invitations'], queryFn: ({ signal }) => organisationAPI.memberInvitations(orgID, signal) })
  const [error, setError] = useState('')
  const items = invitations.data?.items ?? []
  if (!items.length) return null
  const revoke = async (id: string) => {
    setError('')
    try {
      await organisationAPI.revokeMemberInvitation(orgID, id, csrf)
      await client.invalidateQueries({ queryKey: ['org', orgID, 'member-invitations'] })
    } catch (reason) {
      setError(errorText(reason))
    }
  }
  return <section className="organisation-pending" aria-label="Pending invitations">
    <h3>Pending invitations</h3>
    {error && <p className="organisation-error" role="alert">{error}</p>}
    <ul>{items.map(item => <li key={item.id}><span>{item.email}</span><span className="table-meta">{item.role} · expires {new Date(item.expires_at).toLocaleDateString()}</span><Button className="button button-sm" disabled={!csrf} onClick={() => void revoke(item.id)}>Revoke</Button></li>)}</ul>
  </section>
}

function MembersSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const client = useQueryClient()
  const csrf = session.data?.csrf_token ?? ''
  const members = useInfiniteQuery({ queryKey: ['org', orgID, 'memberships'], queryFn: ({ pageParam, signal }) => organisationAPI.memberships(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const teamsQuery = useInfiniteQuery({ queryKey: ['org', orgID, 'teams', 'list'], queryFn: ({ pageParam, signal }) => organisationAPI.teams(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const reposQuery = useInfiniteQuery({ queryKey: ['org', orgID, 'repositories', 'list'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const items = members.data?.pages.flatMap(page => page.items) ?? []
  const teamItems = teamsQuery.data?.pages.flatMap(page => page.items) ?? []
  const repoItems = reposQuery.data?.pages.flatMap(page => page.items) ?? []
  const [selectedID, setSelectedID] = useState('')
  const selected = items.find(member => member.user_id === selectedID)
  const [role, setRole] = useState<Membership['role']>('viewer')
  const [allRepositories, setAllRepositories] = useState(false)
  const [teamIDs, setTeamIDs] = useState<string[]>([])
  const [repositoryIDs, setRepositoryIDs] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [confirmRemove, setConfirmRemove] = useState(false)
  const ownID = session.data?.user.id
  const ownName = session.data?.user.name
  const displayName = (member: Membership) => identityName(member, ownID, ownName)
  const dirty = !!selected && (role !== selected.role || allRepositories !== selected.all_repositories || !sameIDs(teamIDs, selected.team_ids ?? []) || !sameIDs(repositoryIDs, selected.repository_ids ?? []))

  useEffect(() => {
    if (!items.length) { setSelectedID(''); return }
    if (!items.some(member => member.user_id === selectedID)) setSelectedID(items[0].user_id ?? '')
  }, [items, selectedID])
  useEffect(() => {
    if (!selected) return
    setRole(selected.role)
    setAllRepositories(selected.all_repositories)
    setTeamIDs(selected.team_ids ?? [])
    setRepositoryIDs(selected.repository_ids ?? [])
    setError('')
    setConfirmRemove(false)
  }, [selected?.user_id, selected?.version])
  useEffect(() => setNotice(''), [selected?.user_id])

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected?.user_id) return
    setBusy(true)
    setError('')
    setNotice('')
    try {
      await organisationAPI.putMember(orgID, selected.user_id, selected.version ?? 0, { role, all_repositories: allRepositories, team_ids: teamIDs, repository_ids: repositoryIDs }, csrf)
      await client.invalidateQueries({ queryKey: ['org', orgID, 'memberships'] })
      setNotice('Changes saved')
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }
  const remove = async () => {
    if (!selected?.user_id) return
    setBusy(true)
    setError('')
    try {
      await organisationAPI.deleteMember(orgID, selected.user_id, selected.version ?? 0, csrf)
      await client.invalidateQueries({ queryKey: ['org', orgID, 'memberships'] })
      setConfirmRemove(false)
    } catch (reason) {
      setError(errorText(reason))
    } finally {
      setBusy(false)
    }
  }

  if (members.isLoading) return <p className="organisation-state" role="status">Loading members…</p>
  if (members.error && !members.data) return <div className="organisation-state" role="alert"><span>Members unavailable: {errorText(members.error)}</span><Button onClick={() => void members.refetch()}>Retry</Button></div>

  return <div className="organisation-workspace">
    <header className="organisation-toolbar"><div><strong>{items.length}</strong> {items.length === 1 ? 'member' : 'members'}</div><InviteMember orgID={orgID} csrf={csrf} /></header>
    <PendingInvitations orgID={orgID} csrf={csrf} />
    {members.error && <p className="organisation-error" role="alert">Could not refresh members: {errorText(members.error)} <Button onClick={() => void members.refetch()}>Retry</Button></p>}
    {error && <p className="organisation-error" role="alert">{error}</p>}
    {items.length === 0 ? <p className="organisation-empty">No members found.</p> : <div className="organisation-split">
      <nav className="organisation-list" aria-label="Members">
        {items.map(member => <button key={member.user_id} className={`organisation-list-row${member.user_id === selectedID ? ' selected' : ''}`} aria-current={member.user_id === selectedID ? 'true' : undefined} onClick={() => setSelectedID(member.user_id ?? '')}><span>{displayName(member)}</span><small>{member.role}</small></button>)}
        {members.hasNextPage && <Button disabled={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>{members.isFetchingNextPage ? 'Loading…' : 'Load more members'}</Button>}
      </nav>
      {selected && <form className="organisation-detail" onSubmit={save}>
        <div className="organisation-detail-head"><h2>{displayName(selected)}</h2><Button className="button button-danger" type="button" disabled={busy || !csrf} onClick={() => setConfirmRemove(true)}>Remove member</Button></div>
        <p className="organisation-member-id">Account ID <code>{selected.user_id}</code></p>
        <label className="organisation-field">Role<select aria-label={`Role for ${selected.user_id}`} value={role} onChange={event => setRole(event.target.value as Membership['role'])} disabled={busy}>{roles.map(item => <option key={item} value={item}>{item}</option>)}</select></label>
        <fieldset className="organisation-access"><legend>Repository access</legend>
          <label className="organisation-choice"><input type="radio" name={`access-${selected.user_id}`} checked={allRepositories} onChange={() => setAllRepositories(true)} disabled={busy} /><span>All repositories</span></label>
          <label className="organisation-choice"><input type="radio" name={`access-${selected.user_id}`} checked={!allRepositories} onChange={() => setAllRepositories(false)} disabled={busy} /><span>Selected scope</span></label>
          {!allRepositories && <div className="organisation-scope-pickers">
            <fieldset><legend>Teams</legend>{teamsQuery.error && <p className="organisation-error" role="alert">Teams unavailable <Button type="button" onClick={() => void teamsQuery.refetch()}>Retry</Button></p>}{teamItems.map(team => <label className="organisation-repo" key={team.id}><input type="checkbox" checked={teamIDs.includes(team.id)} onChange={event => setTeamIDs(current => event.target.checked ? [...current, team.id] : current.filter(id => id !== team.id))} disabled={busy} /><span>{team.name}</span></label>)}{teamsQuery.hasNextPage && <Button type="button" disabled={teamsQuery.isFetchingNextPage} onClick={() => void teamsQuery.fetchNextPage()}>{teamsQuery.isFetchingNextPage ? 'Loading…' : 'Load more teams'}</Button>}</fieldset>
            <fieldset><legend>Repositories</legend>{reposQuery.error && <p className="organisation-error" role="alert">Repositories unavailable <Button type="button" onClick={() => void reposQuery.refetch()}>Retry</Button></p>}{repoItems.map(repo => <label className="organisation-repo" key={repo.id}><input type="checkbox" checked={repositoryIDs.includes(repo.id)} onChange={event => setRepositoryIDs(current => event.target.checked ? [...current, repo.id] : current.filter(id => id !== repo.id))} disabled={busy} /><span>{repo.name}</span></label>)}{reposQuery.hasNextPage && <Button type="button" disabled={reposQuery.isFetchingNextPage} onClick={() => void reposQuery.fetchNextPage()}>{reposQuery.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}</fieldset>
          </div>}
        </fieldset>
        {notice && <p role="status" className="organisation-saved">{notice}</p>}
        <footer className="organisation-form-actions"><span>{dirty ? 'Unsaved changes' : `Version ${selected.version ?? 0}`}</span><Button type="submit" disabled={busy || !csrf || !dirty}>{busy ? 'Saving…' : 'Save changes'}</Button></footer>
      </form>}
    </div>}
    <Dialog open={confirmRemove} title="Remove member" onClose={() => setConfirmRemove(false)}><p>Remove {selected ? displayName(selected) : 'member'} from this organisation?</p><div className="organisation-form-actions"><Button type="button" onClick={() => setConfirmRemove(false)}>Cancel</Button><Button className="button button-danger" type="button" disabled={busy || !csrf} onClick={() => void remove()}>{busy ? 'Removing…' : 'Remove member'}</Button></div></Dialog>
  </div>
}

function sameIDs(left: string[], right: string[]) {
  if (left.length !== right.length) return false
  const normalized = new Set(left)
  return right.every(id => normalized.has(id))
}
