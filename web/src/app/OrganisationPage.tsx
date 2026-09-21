import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { api } from '../api/client'
import { organisationAPI, type Membership, type Team } from '../organisation-api'
import { useSession } from './query'

const errorText = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const roles = ['owner', 'admin', 'maintainer', 'viewer']

export function OrganisationPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const role = session.data?.memberships.find(item => item.org_id === orgID)?.role
  if (role !== 'owner' && role !== 'admin') return <StatePanel kind="blocked" title="Organisation administration restricted" detail="Team and membership administration requires the owner or administrator role. Ask an organisation owner for access." />
  return <div className="stack"><TeamsSection orgID={orgID} />{role === 'owner' ? <MembersSection orgID={orgID} /> : <section className="state-card"><h2>Membership</h2><p className="table-meta">Listing and editing members requires the owner role. Your administrator access covers teams and repository scope.</p></section>}<section className="state-card"><h2>Identity and retention</h2><p className="table-meta">OIDC issuer, session lifetime and audit retention are configured by the operator at install time. See the operator runbook for host prerequisites and recovery; the browser cannot change infrastructure authentication.</p></section></div>
}

function MembersSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const query = useQuery({ queryKey: ['org', orgID, 'memberships'], queryFn: ({ signal }) => organisationAPI.memberships(orgID, undefined, signal) })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const change = async (member: Membership, next: { role?: string; all_repositories?: boolean }) => { setBusy(member.user_id ?? ''); setError(''); try { await organisationAPI.putMember(orgID, member.user_id ?? '', member.version ?? 0, { role: (next.role ?? member.role) as Membership['role'], all_repositories: next.all_repositories ?? member.all_repositories, team_ids: member.team_ids, repository_ids: member.repository_ids }, csrf); await query.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const remove = async (member: Membership) => { setBusy(member.user_id ?? ''); setError(''); try { await organisationAPI.deleteMember(orgID, member.user_id ?? '', member.version ?? 0, csrf); await query.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  if (query.isLoading) return <section className="state-card"><h2>Membership</h2><p className="table-meta">Loading membership…</p></section>
  if (query.error) return <section className="state-card"><h2>Membership</h2><p className="error-text" role="alert">Membership unavailable: {errorText(query.error)} <Button onClick={() => void query.refetch()}>Retry</Button></p></section>
  const items = query.data?.items ?? []
  return <section className="state-card"><h2>Membership</h2>{error && <p className="error-text" role="alert">{error}</p>}<DataTable caption="Organisation members"><table><thead><tr><th>User</th><th>Role</th><th>Repository access</th><th>Actions</th></tr></thead><tbody>{items.map(member => <tr key={member.user_id}><td><code>{member.user_id}</code></td><td><select aria-label={`Role for ${member.user_id}`} value={member.role} disabled={busy === member.user_id} onChange={event => void change(member, { role: event.target.value })}>{roles.map(item => <option key={item} value={item}>{item}</option>)}</select></td><td>{member.all_repositories ? 'All repositories' : `${member.repository_ids.length} repositories · ${member.team_ids.length} teams`}</td><td><Button disabled={busy === member.user_id} onClick={() => void remove(member)}>Remove</Button></td></tr>)}</tbody></table>{!items.length && <EmptyTable label="No members recorded." />}</DataTable></section>
}

function TeamsSection({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const teams = useQuery({ queryKey: ['org', orgID, 'teams'], queryFn: ({ signal }) => organisationAPI.teams(orgID, undefined, signal) })
  const repos = useQuery({ queryKey: ['org', orgID, 'repositories', { query: '' }], queryFn: ({ signal }) => api.getRepositories(orgID, { limit: 100, signal }) })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const repositories = repos.data?.items ?? []
  const save = async (team: Team, patch: Partial<Team>) => { setBusy(team.id); setError(''); try { await organisationAPI.putTeam(orgID, team.id, team.version, { name: patch.name ?? team.name, repository_ids: patch.repository_ids ?? team.repository_ids }, csrf); await teams.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const create = async () => { setBusy('new'); setError(''); try { await organisationAPI.putTeam(orgID, crypto.randomUUID(), 0, { name: 'New team', repository_ids: [] }, csrf); await teams.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  const remove = async (team: Team) => { setBusy(team.id); setError(''); try { await organisationAPI.deleteTeam(orgID, team.id, team.version, csrf); await teams.refetch() } catch (reason) { setError(errorText(reason)) } finally { setBusy('') } }
  if (teams.isLoading) return <section className="state-card"><h2>Teams</h2><p className="table-meta">Loading teams…</p></section>
  if (teams.error) return <section className="state-card"><h2>Teams</h2><p className="error-text" role="alert">Teams unavailable: {errorText(teams.error)} <Button onClick={() => void teams.refetch()}>Retry</Button></p></section>
  const items = teams.data?.items ?? []
  return <section className="state-card"><div className="subsection-actions"><h2>Teams</h2><Button disabled={busy === 'new'} onClick={() => void create()}>Create team</Button></div>{error && <p className="error-text" role="alert">{error}</p>}<DataTable caption="Repository teams"><table><thead><tr><th>Team</th><th>Repositories</th><th>Actions</th></tr></thead><tbody>{items.map(team => <tr key={team.id}><td><input aria-label={`Team name ${team.id}`} defaultValue={team.name} disabled={busy === team.id} onBlur={event => { if (event.target.value.trim() && event.target.value !== team.name) void save(team, { name: event.target.value.trim() }) }} /></td><td><details><summary>{team.repository_ids.length} repositories</summary><div className="checkbox-list">{repositories.map(repo => <label key={repo.id} className="checkbox-label"><input type="checkbox" checked={team.repository_ids.includes(repo.id)} disabled={busy === team.id} onChange={event => void save(team, { repository_ids: event.target.checked ? [...team.repository_ids, repo.id] : team.repository_ids.filter(id => id !== repo.id) })} />{repo.name}</label>)}</div></details></td><td><Button disabled={busy === team.id} onClick={() => void remove(team)}>Delete</Button></td></tr>)}</tbody></table>{!items.length && <EmptyTable label="No teams configured. Create a team to group repository scope." />}</DataTable><p className="table-meta"><StatusBadge label={repos.error ? 'Repository scope unknown' : 'Repository scope loaded'} tone={repos.error ? 'amber' : 'green'} /> Team changes use each team's current version.</p></section>
}
