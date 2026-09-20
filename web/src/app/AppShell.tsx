import { useEffect, useState } from 'react'
import { Link, Outlet, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, ReforgeAPIError } from '../api/client'
import { clearOrganisationQueries, useMeta, useSession } from './query'
import { sectionFor, sections } from './types'
import { Button, Dialog } from '../components/Accessible'
import { StatePanel } from '../components/StatePanel'
import { SignInPage } from './SignInPage'

export function AppShell() {
  const { data: meta, isLoading: metaLoading } = useMeta()
  const session = useSession()
  const routeSearch = useSearch({ strict: false }) as { q?: string }
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const queryClient = useQueryClient()
  const params = useParams({ strict: false }) as { orgID?: string; section?: string }
  const [navOpen, setNavOpen] = useState(false)
  const [orgDialog, setOrgDialog] = useState(false)
  const [search, setSearch] = useState(routeSearch.q ?? '')
  const [visibleUserID, setVisibleUserID] = useState<string | undefined>(undefined)
  const active = sectionFor(params.section)
  const org = session.data?.organisations.find(item => item.id === params.orgID)
  const orgs = session.data?.organisations ?? []

  useEffect(() => setSearch(routeSearch.q ?? ''), [routeSearch.q])

  useEffect(() => {
    if (session.error instanceof ReforgeAPIError && session.error.status === 401) {
      clearOrganisationQueries(queryClient)
      setVisibleUserID(undefined)
      return
    }
    const userID = session.data?.user.id
    if (userID && visibleUserID && visibleUserID !== userID) clearOrganisationQueries(queryClient)
    if (userID) setVisibleUserID(userID)
  }, [queryClient, session.data?.user.id, session.error, visibleUserID])

  const updateSearch = (value: string) => {
    setSearch(value)
    void navigate({ search: { q: value || undefined } })
  }

  const chooseOrg = (id: string) => {
    if (id === org?.id) { setOrgDialog(false); return }
    clearOrganisationQueries(queryClient)
    setOrgDialog(false)
    navigate({ to: '/org/$orgID/$section', params: { orgID: id, section: params.section ?? 'overview' }, search: { q: search || undefined } })
  }

  const signOut = async () => {
    if (!session.data) return
    try { await api.logout(session.data.csrf_token) } finally { clearOrganisationQueries(queryClient); queryClient.removeQueries({ queryKey: ['session'] }); await queryClient.invalidateQueries({ queryKey: ['session'] }) }
  }

  if (session.isLoading || metaLoading) return <main className="centered-page"><StatePanel kind="loading" title="Opening Reforge" detail="Checking your session and application status." /></main>
  if (session.error instanceof ReforgeAPIError && session.error.status === 401) return <SignInPage />
  if (session.error) return <main className="centered-page"><StatePanel kind="error" title="Session unavailable" detail={session.error.message} action={<Button onClick={() => session.refetch()}>Retry</Button>} /></main>
  if (!session.data || visibleUserID !== session.data.user.id) return <main className="centered-page"><StatePanel kind="loading" title="Checking access" detail="" /></main>
  if (params.orgID && !org) return <main className="centered-page"><StatePanel kind="blocked" title="Organisation access denied" detail="Your session does not include this organisation. Choose an organisation you can access from its deep link." /></main>
  if (!org) return <main className="centered-page"><StatePanel kind="empty" title="No organisation access" detail="Your account is signed in, but has no organisation membership." /></main>

  return <div className="app-shell">
    <a className="skip-link" href="#main-content">Skip to content</a>
    <aside id="primary-navigation" className={`sidebar ${navOpen ? 'sidebar-open' : ''}`} aria-label="Primary navigation">
      <div className="brand"><span className="brand-mark" aria-hidden="true">R</span><span>Reforge</span></div>
      <nav className="nav-list">
        <p className="nav-label">Workspace</p>
        {sections.filter(section => section.group === 'workspace').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined }} onClick={() => setNavOpen(false)}><span className="nav-icon" aria-hidden="true">{section.id === 'overview' ? '⌂' : section.id === 'repositories' ? '▦' : section.id === 'findings' ? '!' : section.id === 'runs' ? '↻' : section.id === 'changes' ? '⇄' : section.id === 'deployments' ? '↗' : '◈'}</span>{section.label}</Link>)}
        <p className="nav-label nav-label-admin">Administration</p>
        {sections.filter(section => section.group === 'admin').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined }} onClick={() => setNavOpen(false)}><span className="nav-icon" aria-hidden="true">{section.id === 'policies' ? '⚙' : section.id === 'connections' ? '◇' : section.id === 'runners' ? '▤' : section.id === 'usage' ? '◒' : section.id === 'audit' ? '≡' : '◎'}</span>{section.label}</Link>)}
      </nav>
      <div className="sidebar-footer"><span className="user-avatar" aria-hidden="true">{session.data.user.name.slice(0, 1).toUpperCase()}</span><div><strong>{session.data.user.name}</strong><span>{session.data.user.email}</span></div><button className="icon-button" aria-label="Sign out" onClick={signOut}>↪</button></div>
    </aside>
    <div className="main-column">
      <header className="topbar">
        <button className="menu-button" aria-expanded={navOpen} aria-controls="primary-navigation" onClick={() => setNavOpen(open => !open)}>☰<span className="sr-only">Menu</span></button>
        <div className="breadcrumbs"><span className="eyebrow">{active.group === 'admin' ? 'Administration' : 'Workspace'}</span><span aria-hidden="true">/</span><strong>{active.label}</strong></div>
        <div className="topbar-actions"><span className={`connection-dot ${meta?.development ? 'fixture' : ''}`} title={meta?.development ? 'Development server' : 'Connected'} /><label className="search-box"><span className="sr-only">Search repositories</span><span aria-hidden="true">⌕</span><input value={search} onChange={event => updateSearch(event.target.value)} placeholder="Search repositories" /></label><Button className="org-button" onClick={() => setOrgDialog(true)} aria-haspopup="dialog" aria-label="Switch organisation"><span className="org-dot" aria-hidden="true">{org.name.slice(0, 1)}</span>{org.name}<span aria-hidden="true">⌄</span></Button></div>
      </header>
      {meta?.development && <div className="fixture-banner" role="status"><span aria-hidden="true">◆</span><strong>Development environment</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled for this server.' : 'Live authentication is configured.'}</span></div>}
      {org.paused && <div className="pause-banner" role="status"><span aria-hidden="true">Ⅱ</span><strong>{org.name} is paused.</strong><span>New automation is blocked until an organisation administrator resumes it.</span></div>}
      <main id="main-content" className="content"><Outlet /></main>
    </div>
    <Dialog open={orgDialog} title="Switch organisation" onClose={() => setOrgDialog(false)}><p className="dialog-copy">Choose the organisation whose repositories and activity you want to view.</p><div className="org-options">{orgs.map(item => <button key={item.id} className={`org-option ${item.id === org.id ? 'selected' : ''}`} onClick={() => chooseOrg(item.id)}><span className="org-dot">{item.name.slice(0, 1)}</span><span><strong>{item.name}</strong><small>{item.paused ? 'Paused' : 'Active'} · version {item.version}</small></span>{item.id === org.id && <span className="check" aria-label="Current organisation">✓</span>}</button>)}</div></Dialog>
  </div>
}
