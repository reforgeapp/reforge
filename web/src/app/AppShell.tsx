import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, Outlet, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, ReforgeAPIError } from '../api/client'
import { clearOrganisationQueries, useMeta, useSession } from './query'
import { sections } from './types'
import { Button, Dialog } from '../components/Accessible'
import { StatePanel } from '../components/StatePanel'
import { Icon } from '../components/Icons'
import { SignInPage } from './SignInPage'
import { BootstrapPage } from './BootstrapPage'

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
  const menuRef = useRef<HTMLButtonElement>(null)
  const sidebarRef = useRef<HTMLElement>(null)
  const navWasOpenRef = useRef(false)
  const org = session.data?.organisations.find(item => item.id === params.orgID)
  const orgs = session.data?.organisations ?? []
  const setSidebarRef = useCallback((element: HTMLElement | null) => {
    sidebarRef.current = element
    if (element) element.inert = window.matchMedia('(max-width: 720px)').matches && !navOpen
  }, [navOpen])

  useEffect(() => setSearch(routeSearch.q ?? ''), [routeSearch.q])

  useEffect(() => { if (meta?.docs_url) document.documentElement.dataset.docsRoot = meta.docs_url }, [meta?.docs_url])

  useEffect(() => {
    const syncSidebar = () => {
      if (sidebarRef.current) sidebarRef.current.inert = window.matchMedia('(max-width: 720px)').matches && !navOpen
    }
    syncSidebar()
    if (navWasOpenRef.current && !navOpen && window.matchMedia('(max-width: 720px)').matches) menuRef.current?.focus()
    navWasOpenRef.current = navOpen
    window.addEventListener('resize', syncSidebar)
    return () => window.removeEventListener('resize', syncSidebar)
  }, [navOpen])

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && navOpen) { setNavOpen(false); menuRef.current?.focus() }
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [navOpen])

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
  }

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!org) return
    void navigate({ to: '/org/$orgID/$section', params: { orgID: org.id, section: 'repositories' }, search: previous => {
      const next: Record<string, unknown> = {}
      if (params.section === 'repositories') for (const key of ['provider', 'team_id', 'status', 'repository']) { const value = (previous as Record<string, string | undefined>)[key]; if (value) next[key] = value }
      if (search) next.q = search
      return next
    } })
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
  if (!org && meta?.edition === 'self-hosted') return <BootstrapPage csrf={session.data.csrf_token} onDone={id => { void session.refetch().then(() => navigate({ to: '/org/$orgID/$section', params: { orgID: id, section: 'overview' }, search: { q: undefined } })) }} />
  if (!org) return <main className="centered-page"><StatePanel kind="empty" title="No organisation access" detail="Your account is signed in, but has no organisation membership." /></main>

  const group = (name: 'workspace' | 'admin') => sections.filter(section => section.group === name)

  return <div className="app-shell">
    <a className="skip-link" href="#main-content">Skip to content</a>
    <header className="topbar">
      <div className="masthead-brand"><button ref={menuRef} className="menu-button" aria-expanded={navOpen} aria-controls="primary-navigation" aria-label="Menu" onClick={() => setNavOpen(open => !open)}><Icon name="menu" /></button><div className="brand"><span className="brand-mark" aria-hidden="true">R</span><span>Reforge</span></div></div>
      <Button className="org-button" onClick={() => setOrgDialog(true)} aria-haspopup="dialog" aria-label="Switch organisation"><span className="org-dot" aria-hidden="true">{org.name.slice(0, 1)}</span><span>{org.name}</span><Icon name="chevron" size={15} /></Button>
      <div className="topbar-actions">
        <span className={`connection-dot ${meta?.development ? 'fixture' : ''}`} title={meta?.development ? 'Development server' : 'Connected'} />
        <form className="search-box" role="search" onSubmit={submitSearch}><label className="sr-only" htmlFor="global-search">Search repositories</label><Icon name="search" size={15} /><input id="global-search" value={search} onChange={event => updateSearch(event.target.value)} placeholder="Search repositories" /></form>
        <div className="masthead-user"><span className="user-avatar" aria-hidden="true">{session.data.user.name.slice(0, 1).toUpperCase()}</span><span className="masthead-user-name">{session.data.user.name}</span><button className="icon-button" aria-label="Sign out" onClick={signOut}><Icon name="signout" /></button></div>
      </div>
    </header>
    <div className="app-body">
      <aside ref={setSidebarRef} id="primary-navigation" className={`sidebar ${navOpen ? 'sidebar-open' : ''}`} aria-label="Primary navigation">
        <nav className="nav-list">
          <p className="nav-label">Workspace</p>
          {group('workspace').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined }} onClick={() => setNavOpen(false)}><span className="nav-icon"><Icon name={section.id} /></span>{section.label}</Link>)}
          <p className="nav-label nav-label-admin">Administration</p>
          {group('admin').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined }} onClick={() => setNavOpen(false)}><span className="nav-icon"><Icon name={section.id} /></span>{section.label}</Link>)}
        </nav>
      </aside>
      <div className="main-column">
      {meta?.development && <div className="fixture-banner" role="status"><Icon name="warning" size={15} /><strong>Development environment</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled for this server.' : 'Live authentication is configured.'}</span></div>}
      {org.paused && <div className="pause-banner" role="status"><strong>{org.name} is paused.</strong><span>New automation is blocked until an organisation administrator resumes it.</span></div>}
      <main id="main-content" className="content"><Outlet key={`${session.data.user.id}:${org.id}`} /></main>
      </div>
    </div>
    <Dialog open={orgDialog} title="Switch organisation" onClose={() => setOrgDialog(false)}><p className="dialog-copy">Choose the organisation whose repositories and activity you want to view.</p><div className="org-options">{orgs.map(item => <button key={item.id} className={`org-option ${item.id === org.id ? 'selected' : ''}`} onClick={() => chooseOrg(item.id)}><span className="org-dot">{item.name.slice(0, 1)}</span><span><strong>{item.name}</strong><small>{item.paused ? 'Paused' : 'Active'} · version {item.version}</small></span>{item.id === org.id && <span className="check" aria-label="Current organisation">✓</span>}</button>)}</div></Dialog>
  </div>
}
