import { useState, type FormEvent } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { api } from '../api/client'
import { organisationAPI, type Membership, type Team } from '../organisation-api'
import { useSession } from './query'
import { Tabs } from '../components/Workspace'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const roles = ['owner', 'admin', 'maintainer', 'viewer']

export function OrganisationPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  const [tab, setTab] = useState<'teams' | 'members' | 'identity'>('teams')
  if (role !== 'owner' && role !== 'admin') return <StatePanel kind="blocked" title="Organisation administration restricted" detail="Team and membership administration requires the owner or administrator role. Ask an organisation owner for access." />
  return <div className="stack"><Tabs id="organisation-workspace" label="Organisation administration" items={[{ id: 'teams', label: 'Teams' }, { id: 'members', label: 'Members' }, { id: 'identity', label: 'Identity' }]} value={tab} onChange={value => setTab(value as typeof tab)} />{tab === 'teams' && <section id="organisation-workspace-panel-teams"><TeamsSection orgID={orgID} /></section>}{tab === 'members' && <section id="organisation-workspace-panel-members">{role === 'owner' ? <MembersSection orgID={orgID} /> : <section className="state-card"><h2>Membership</h2><p className="table-meta">Listing and editing members requires the owner role.</p></section>}</section>}{tab === 'identity' && <section id="organisation-workspace-panel-identity" className="state-card"><h2>Identity and retention</h2><p className="table-meta">OIDC issuer, session lifetime and audit retention are operator configured. Browser changes unavailable.</p></section>}</div>
}

function MembersSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const query = useInfiniteQuery({ queryKey: ['org', orgID, 'memberships'], queryFn: ({ pageParam, signal }) => organisationAPI.memberships(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const change = async (member: Membership, next: { role?: string; all_repositories?: boolean }) => { setBusy(member.user_id ?? ''); setError(''); try { await organisationAPI.putMember(orgID, member.user_id ?? '', member.version ?? 0, { role: (next.role ?? member.role) as Membership['role'], all_repositories: next.all_repositories ?? member.all_repositories, team_ids: member.team_ids ?? [], repository_ids: member.repository_ids ?? [] }, csrf); await query.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const remove = async (member: Membership) => { if (!window.confirm(`Remove ${member.user_id} from this organisation?`)) return; setBusy(member.user_id ?? ''); setError(''); try { await organisationAPI.deleteMember(orgID, member.user_id ?? '', member.version ?? 0, csrf); await query.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  if (query.isLoading) return <section className="state-card"><h2>Membership</h2><p className="table-meta">Loading membership…</p></section>
  if (query.error) return <section className="state-card"><h2>Membership</h2><p className="error-text" role="alert">Membership unavailable: {errorText(query.error)} <Button onClick={() => void query.refetch()}>Retry</Button></p></section>
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  const ownID = session.data?.user.id
  const displayName = (member: Membership) => member.user_id === ownID ? `${session.data?.user.name || 'You'} (${member.user_id})` : `User ${member.user_id} (name unavailable)`
  return <section className="state-card"><h2>Membership</h2>{error && <p className="error-text" role="alert">{error}</p>}<DataTable caption="Organisation members"><table><thead><tr><th>User</th><th>Role</th><th>Repository access</th><th>Actions</th></tr></thead><tbody>{items.map(member => { const repositoryIDs = member.repository_ids ?? []; const teamIDs = member.team_ids ?? []; return <tr key={member.user_id}><td>{displayName(member)}</td><td><select aria-label={`Role for ${member.user_id}`} value={member.role} disabled={busy === member.user_id} onChange={event => void change(member, { role: event.target.value })}>{roles.map(item => <option key={item} value={item}>{item}</option>)}</select></td><td>{member.all_repositories ? 'All repositories' : `${repositoryIDs.length} repositories · ${teamIDs.length} teams`}</td><td><Button disabled={busy === member.user_id} onClick={() => void remove(member)}>Remove</Button></td></tr>})}</tbody></table>{!items.length && <EmptyTable label="No members recorded." />}</DataTable>{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? 'Loading…' : 'Load more members'}</Button>}</section>
}

function TeamsSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const queryClient = useQueryClient()
  const csrf = session.data?.csrf_token ?? ''
  const teams = useInfiniteQuery({ queryKey: ['org', orgID, 'teams', 'list'], queryFn: ({ pageParam, signal }) => organisationAPI.teams(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const repos = useInfiniteQuery({ queryKey: ['org', orgID, 'repositories', 'list'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { limit: 100, cursor: pageParam, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [newName, setNewName] = useState('')
  const repositories = repos.data?.pages.flatMap(page => page.items) ?? []
  const refreshTeams = () => queryClient.invalidateQueries({ queryKey: ['org', orgID, 'teams'] })
  const save = async (team: Team, patch: Partial<Team>) => { setBusy(team.id); setError(''); try { await organisationAPI.putTeam(orgID, team.id, team.version, { name: patch.name ?? team.name, repository_ids: patch.repository_ids ?? team.repository_ids }, csrf); await refreshTeams() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const create = async (event: FormEvent) => { event.preventDefault(); if (!newName.trim()) return; setBusy('new'); setError(''); try { await organisationAPI.putTeam(orgID, crypto.randomUUID(), 0, { name: newName.trim(), repository_ids: [] }, csrf); setNewName(''); await refreshTeams() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const remove = async (team: Team) => { if (!window.confirm(`Delete team ${team.name}? Repository assignments will be removed.`)) return; setBusy(team.id); setError(''); try { await organisationAPI.deleteTeam(orgID, team.id, team.version, csrf); await refreshTeams() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  if (teams.isLoading) return <section className="state-card"><h2>Teams</h2><p className="table-meta">Loading teams…</p></section>
  if (teams.error) return <section className="state-card"><h2>Teams</h2><p className="error-text" role="alert">Teams unavailable: {errorText(teams.error)} <Button onClick={() => void teams.refetch()}>Retry</Button></p></section>
  const items = teams.data?.pages.flatMap(page => page.items) ?? []
  return <section className="state-card"><div className="subsection-actions"><h2>Teams</h2><form className="row-actions" onSubmit={create}><label>New team name<input required value={newName} onChange={event => setNewName(event.target.value)} /></label><Button type="submit" disabled={busy === 'new' || !csrf}>{busy === 'new' ? 'Creating…' : 'Create team'}</Button></form></div>{error && <p className="error-text" role="alert">{error}</p>}<DataTable caption="Repository teams"><table><thead><tr><th>Team</th><th>Repositories</th><th>Actions</th></tr></thead><tbody>{items.map(team => { const repositoryIDs = team.repository_ids ?? []; return <tr key={team.id}><td><input aria-label={`Team name ${team.id}`} defaultValue={team.name} disabled={busy === team.id} onBlur={event => { if (event.target.value.trim() && event.target.value !== team.name) void save(team, { name: event.target.value.trim() }) }} /></td><td><details><summary>{repositoryIDs.length} repositories</summary><div className="checkbox-list">{repositories.map(repo => <label key={repo.id} className="checkbox-label"><input type="checkbox" checked={repositoryIDs.includes(repo.id)} disabled={busy === team.id} onChange={event => void save(team, { repository_ids: event.target.checked ? [...repositoryIDs, repo.id] : repositoryIDs.filter(id => id !== repo.id) })} />{repo.name}</label>)}</div></details></td><td><Button disabled={busy === team.id} onClick={() => void remove(team)}>Delete</Button></td></tr>})}</tbody></table>{!items.length && <EmptyTable label="No teams configured. Create a team to group repository scope." />}</DataTable>{teams.hasNextPage && <Button disabled={teams.isFetchingNextPage} onClick={() => void teams.fetchNextPage()}>{teams.isFetchingNextPage ? 'Loading…' : 'Load more teams'}</Button>}{repos.hasNextPage && <Button disabled={repos.isFetchingNextPage} onClick={() => void repos.fetchNextPage()}>{repos.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}<p className="table-meta"><StatusBadge label={repos.error ? 'Repository scope unknown' : 'Repository scope loaded'} tone={repos.error ? 'amber' : 'green'} /> Team changes use each team's current version.</p></section>
}
