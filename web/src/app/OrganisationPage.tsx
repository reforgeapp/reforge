import { useEffect, useState, type FormEvent } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { api } from '../api/client'
import { organisationAPI, type Membership } from '../organisation-api'
import '../styles/organisation.css'
import { useSession } from './query'
import { Tabs } from '../components/Workspace'

const roles = ['owner', 'admin', 'maintainer', 'reviewer', 'viewer'] as const
const errorText = (value: unknown) => value instanceof Error ? value.message : 'Request failed.'
const identityName = (member: Membership, ownID?: string, ownName?: string) => member.user_id === ownID ? ownName || 'You' : `Member ····${member.user_id?.slice(-6) ?? 'unknown'}`

export function OrganisationPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const [tab, setTab] = useState<'teams' | 'members' | 'identity'>('teams')

  if (!session.data && session.isPending) return <StatePanel kind="loading" title="Loading organisation access" detail="" />
  if (!session.data && session.error) return <StatePanel kind="error" title="Organisation access unavailable" detail={errorText(session.error)} action={<Button onClick={() => void session.refetch()}>Retry</Button>} />
  if (role !== 'owner' && role !== 'admin') return <StatePanel kind="blocked" title="Organisation access restricted" detail="Ask an organisation owner or administrator to manage this organisation." />

  return <div className="organisation-page">
    <Tabs id="organisation-workspace" label="Organisation sections" items={[{ id: 'teams', label: 'Teams' }, { id: 'members', label: 'Members' }, { id: 'identity', label: 'Identity' }]} value={tab} onChange={value => setTab(value as typeof tab)} />
    {tab === 'teams' && <section id="organisation-workspace-panel-teams"><TeamsSection orgID={orgID} /></section>}
    {tab === 'members' && <section id="organisation-workspace-panel-members">{role === 'owner' ? <MembersSection orgID={orgID} /> : <StatePanel kind="blocked" title="Owner access required" detail="Ask an organisation owner to manage membership." />}</section>}
    {tab === 'identity' && <section id="organisation-workspace-panel-identity"><p className="organisation-pending" role="status">Identity settings are not available yet.</p></section>}
  </div>
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

  const refreshTeams = () => client.invalidateQueries({ queryKey: ['org', orgID, 'teams', 'list'] })
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
        <div className="organisation-detail-head"><h2>{selected.name}</h2><Button className="button-danger" type="button" disabled={busy || !csrf} onClick={() => setConfirmDelete(true)}>Delete</Button></div>
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
    <Dialog open={confirmDelete} title="Delete team" onClose={() => setConfirmDelete(false)}><p>Delete {selected?.name}? Members lose access granted by this team.</p><div className="organisation-form-actions"><Button type="button" onClick={() => setConfirmDelete(false)}>Cancel</Button><Button className="button-danger" type="button" disabled={busy || !csrf} onClick={() => void remove()}>{busy ? 'Deleting…' : 'Delete team'}</Button></div></Dialog>
  </div>
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
    <header className="organisation-toolbar"><div><strong>{items.length}</strong> {items.length === 1 ? 'member' : 'members'}</div></header>
    {members.error && <p className="organisation-error" role="alert">Could not refresh members: {errorText(members.error)} <Button onClick={() => void members.refetch()}>Retry</Button></p>}
    {error && <p className="organisation-error" role="alert">{error}</p>}
    {items.length === 0 ? <p className="organisation-empty">No members found.</p> : <div className="organisation-split">
      <nav className="organisation-list" aria-label="Members">
        {items.map(member => <button key={member.user_id} className={`organisation-list-row${member.user_id === selectedID ? ' selected' : ''}`} aria-current={member.user_id === selectedID ? 'true' : undefined} onClick={() => setSelectedID(member.user_id ?? '')}><span>{displayName(member)}</span><small>{member.role}</small></button>)}
        {members.hasNextPage && <Button disabled={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>{members.isFetchingNextPage ? 'Loading…' : 'Load more members'}</Button>}
      </nav>
      {selected && <form className="organisation-detail" onSubmit={save}>
        <div className="organisation-detail-head"><h2>{displayName(selected)}</h2><Button className="button-danger" type="button" disabled={busy || !csrf} onClick={() => setConfirmRemove(true)}>Remove member</Button></div>
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
    <Dialog open={confirmRemove} title="Remove member" onClose={() => setConfirmRemove(false)}><p>Remove {selected ? displayName(selected) : 'member'} from this organisation?</p><div className="organisation-form-actions"><Button type="button" onClick={() => setConfirmRemove(false)}>Cancel</Button><Button className="button-danger" type="button" disabled={busy || !csrf} onClick={() => void remove()}>{busy ? 'Removing…' : 'Remove member'}</Button></div></Dialog>
  </div>
}

function sameIDs(left: string[], right: string[]) {
  if (left.length !== right.length) return false
  const normalized = new Set(left)
  return right.every(id => normalized.has(id))
}
