import { useCallback, useEffect, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from 'react'
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
import { BrandMark } from '../components/BrandMark'
import { NeedsBanner } from './NeedsBanner'

export function AppShell() {
  const { data: meta, isLoading: metaLoading } = useMeta()
  const session = useSession()
  const routeSearch = useSearch({ strict: false }) as { q?: string }
  const navigate = useNavigate({ from: '/org/$orgID/$section' })
  const queryClient = useQueryClient()
  const params = useParams({ strict: false }) as { orgID?: string; section?: string }
  const [navOpen, setNavOpen] = useState(false)
  const [orgDialog, setOrgDialog] = useState(false)
  const [orgMenuOpen, setOrgMenuOpen] = useState(false)
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    try {
      const stored = window.localStorage.getItem('reforge-theme')
      if (stored === 'light' || stored === 'dark') return stored
    } catch {}
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })
  const [search, setSearch] = useState(routeSearch.q ?? '')
  const [visibleUserID, setVisibleUserID] = useState<string | undefined>(undefined)
  const menuRef = useRef<HTMLButtonElement>(null)
  const orgButtonRef = useRef<HTMLButtonElement>(null)
  const orgMenuRef = useRef<HTMLDivElement>(null)
  const orgMenuRestoreRef = useRef<HTMLElement | null>(null)
  const sidebarRef = useRef<HTMLElement>(null)
  const topbarRef = useRef<HTMLElement>(null)
  const mainColumnRef = useRef<HTMLDivElement>(null)
  const navWasOpenRef = useRef(false)
  const org = session.data?.organisations.find(item => item.id === params.orgID)
  const orgs = session.data?.organisations ?? []
  const setSidebarRef = useCallback((element: HTMLElement | null) => {
    sidebarRef.current = element
    if (element) element.inert = window.matchMedia('(max-width: 720px)').matches && !navOpen
  }, [navOpen])

  const closeMobileNav = () => setNavOpen(false)

  useEffect(() => setSearch(routeSearch.q ?? ''), [routeSearch.q])

  useEffect(() => { document.documentElement.dataset.theme = theme }, [theme])

  const toggleTheme = () => setTheme(current => {
    const next = current === 'dark' ? 'light' : 'dark'
    try { window.localStorage.setItem('reforge-theme', next) } catch {}
    return next
  })

  useEffect(() => { if (meta?.docs_url) document.documentElement.dataset.docsRoot = meta.docs_url }, [meta?.docs_url])

  useEffect(() => {
    const syncSidebar = () => {
      const mobile = window.matchMedia('(max-width: 720px)').matches
      if (!mobile && navOpen) setNavOpen(false)
      if (sidebarRef.current) sidebarRef.current.inert = mobile && !navOpen
      if (topbarRef.current) topbarRef.current.inert = mobile && navOpen
      if (mainColumnRef.current) mainColumnRef.current.inert = mobile && navOpen
    }
    syncSidebar()
    if (!navWasOpenRef.current && navOpen && window.matchMedia('(max-width: 720px)').matches) {
      requestAnimationFrame(() => sidebarRef.current?.querySelector<HTMLAnchorElement>('.nav-link')?.focus())
    }
    if (navWasOpenRef.current && !navOpen && window.matchMedia('(max-width: 720px)').matches) menuRef.current?.focus()
    navWasOpenRef.current = navOpen
    window.addEventListener('resize', syncSidebar)
    return () => window.removeEventListener('resize', syncSidebar)
  }, [navOpen])

  useEffect(() => {
    if (!orgMenuOpen) return
    orgMenuRestoreRef.current = document.activeElement as HTMLElement | null
    requestAnimationFrame(() => orgMenuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus())
    const closeMenu = (restoreFocus = true) => {
      setOrgMenuOpen(false)
      if (restoreFocus) orgMenuRestoreRef.current?.focus()
      orgMenuRestoreRef.current = null
    }
    const closeOnOutside = (event: MouseEvent) => {
      if (!orgMenuRef.current?.contains(event.target as Node) && !orgButtonRef.current?.contains(event.target as Node)) {
        const target = event.target as HTMLElement
        closeMenu(!target.closest('a,button,input,select,textarea,[tabindex]'))
      }
    }
    document.addEventListener('mousedown', closeOnOutside)
    return () => document.removeEventListener('mousedown', closeOnOutside)
  }, [orgMenuOpen])

  const handleNavigationKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMobileNav()
      return
    }
    if (event.key !== 'Tab' || !window.matchMedia('(max-width: 720px)').matches) return
    const links = Array.from(sidebarRef.current?.querySelectorAll<HTMLAnchorElement>('.nav-link') ?? [])
    if (!links.length) return
    const first = links[0]
    const last = links[links.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

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
    if (id === org?.id) { setOrgMenuOpen(false); setOrgDialog(false); orgButtonRef.current?.focus(); return }
    clearOrganisationQueries(queryClient)
    setOrgMenuOpen(false)
    setOrgDialog(false)
    navigate({ to: '/org/$orgID/$section', params: { orgID: id, section: params.section ?? 'overview' }, search: { q: search || undefined } })
  }

  const handleOrgMenuKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const items = Array.from(orgMenuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
    const index = items.indexOf(document.activeElement as HTMLButtonElement)
    if (event.key === 'Escape') { event.preventDefault(); setOrgMenuOpen(false); orgButtonRef.current?.focus(); orgMenuRestoreRef.current = null; return }
    if (event.key === 'Tab') {
      setOrgMenuOpen(false)
      orgMenuRestoreRef.current = null
      orgButtonRef.current?.focus()
      return
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    event.preventDefault()
    if (index < 0) { items[event.key === 'ArrowDown' ? 0 : items.length - 1]?.focus(); return }
    const next = event.key === 'ArrowDown' ? (index + 1) % items.length : (index - 1 + items.length) % items.length
    items[next]?.focus()
  }

  const signOut = async () => {
    if (!session.data) return
    try { await api.logout(session.data.csrf_token) } finally { clearOrganisationQueries(queryClient); queryClient.removeQueries({ queryKey: ['session'] }); await queryClient.invalidateQueries({ queryKey: ['session'] }) }
  }

  if (window.location.pathname === '/invite') return <Outlet />
  if (session.isLoading || metaLoading) return <main className="centered-page"><StatePanel kind="loading" title="Opening Reforge" detail="Checking your session and application status." /></main>
  if (session.error instanceof ReforgeAPIError && session.error.status === 401) return <SignInPage />
  if (session.error) return <main className="centered-page"><StatePanel kind="error" title="Session unavailable" detail={session.error.message} action={<Button onClick={() => session.refetch()}>Retry</Button>} /></main>
  if (!session.data || visibleUserID !== session.data.user.id) return <main className="centered-page"><StatePanel kind="loading" title="Checking access" detail="" /></main>
  if (params.orgID && !org) return <main className="centered-page"><StatePanel kind="blocked" title="Organisation access denied" detail="Your session does not include this organisation. Choose an organisation you can access from its deep link." /></main>
  if (!org && meta?.edition === 'self-hosted') return <BootstrapPage csrf={session.data.csrf_token} onDone={id => { void session.refetch().then(() => navigate({ to: '/org/$orgID/$section', params: { orgID: id, section: 'overview' }, search: { q: undefined } })) }} />
  if (!org) return <main className="centered-page"><StatePanel kind="empty" title="No organisation access" detail={meta?.edition === 'hosted' ? 'Reforge is invite-only. Open the invitation link from your email to create your workspace.' : 'Your account is signed in, but has no organisation membership.'} /></main>

  const group = (name: 'workspace' | 'admin') => sections.filter(section => section.group === name)

  return <div className="app-shell">
    <a className="skip-link" href="#main-content">Skip to content</a>
    <header ref={topbarRef} className="topbar">
      <div className="masthead-brand"><button ref={menuRef} className="menu-button" aria-expanded={navOpen} aria-controls="primary-navigation" aria-label="Menu" onClick={() => setNavOpen(open => !open)}><Icon name="menu" /></button><div className="brand"><BrandMark /><span>Reforge</span></div></div>
      <div className="org-switcher">
        <button ref={orgButtonRef} className="org-button" onClick={() => setOrgMenuOpen(open => !open)} aria-haspopup="menu" aria-expanded={orgMenuOpen} aria-controls="organisation-menu" aria-label="Switch organisation"><span className="org-dot" aria-hidden="true">{org.name.slice(0, 1)}</span><span>{org.name}</span><Icon name="chevron" size={15} /></button>
        {orgMenuOpen && <div ref={orgMenuRef} id="organisation-menu" className="org-menu" role="menu" aria-label="Quick switch organisation" onKeyDown={handleOrgMenuKeyDown}>
          <div className="org-menu-list" role="group" tabIndex={0} aria-label="Organisations">
            {orgs.map(item => <button key={item.id} title={item.name} role="menuitem" tabIndex={-1} className={`org-menu-item ${item.id === org.id ? 'selected' : ''}`} onClick={() => chooseOrg(item.id)}><span>{item.name}</span>{item.paused && <small>Paused</small>}{item.id === org.id && <span className="check" aria-label="Current organisation">✓</span>}</button>)}
          </div>
          <button role="menuitem" tabIndex={-1} aria-label="More / manage organisations" className="org-menu-manage" onClick={() => { setOrgMenuOpen(false); setOrgDialog(true) }}>More…</button>
        </div>}
      </div>
      <div className="topbar-actions">
        <span className={`connection-dot ${meta?.development ? 'fixture' : ''}`} title={meta?.development ? 'Development server' : 'Connected'} />
        <form className="search-box" role="search" autoComplete="off" onSubmit={submitSearch}><label className="sr-only" htmlFor="global-search">Search repositories</label><input id="global-search" type="search" autoComplete="off" autoCorrect="off" spellCheck={false} value={search} onChange={event => updateSearch(event.target.value)} placeholder="Search repositories" /><button className="search-submit" type="submit" aria-label="Submit repository search"><Icon name="search" size={15} /></button></form>
        <button className="icon-button theme-toggle" aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'} onClick={toggleTheme}>{theme === 'dark' ? '☀' : '◐'}</button><div className="masthead-user"><span className="user-avatar" aria-hidden="true">{session.data.user.name.slice(0, 1).toUpperCase()}</span><span className="masthead-user-name">{session.data.user.name}</span><button className="icon-button" aria-label="Sign out" onClick={signOut}><Icon name="signout" /></button></div>
      </div>
    </header>
    <div className="app-body">
      {navOpen && <button className="nav-backdrop" type="button" tabIndex={-1} aria-label="Close navigation" onClick={() => closeMobileNav()} />}
      <aside ref={setSidebarRef} id="primary-navigation" className={`sidebar ${navOpen ? 'sidebar-open' : ''}`} role={navOpen && window.matchMedia('(max-width: 720px)').matches ? 'dialog' : undefined} aria-modal={navOpen && window.matchMedia('(max-width: 720px)').matches ? true : undefined} aria-label="Primary navigation" onKeyDown={handleNavigationKeyDown}>
        <nav className="nav-list" aria-label="Sections">
          <p className="nav-label">Workspace</p>
          {group('workspace').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined, provider: undefined, team_id: undefined, status: undefined, repository: undefined }} onClick={() => closeMobileNav()}><span className="nav-icon"><Icon name={section.id} /></span>{section.label}</Link>)}
          <p className="nav-label nav-label-admin">Administration</p>
          {group('admin').map(section => <Link key={section.id} className="nav-link" activeProps={{ className: 'nav-link active' }} to="/org/$orgID/$section" params={{ orgID: org.id, section: section.id }} search={{ q: undefined, provider: undefined, team_id: undefined, status: undefined, repository: undefined }} onClick={() => closeMobileNav()}><span className="nav-icon"><Icon name={section.id} /></span>{section.label}</Link>)}
        </nav>
      </aside>
      <div ref={mainColumnRef} className="main-column">
      {meta?.development && <div className="fixture-banner" role="status"><Icon name="warning" size={15} /><strong>Development environment</strong><span>{meta.fixture_auth ? 'Fixture authentication is enabled for this server.' : 'Live authentication is configured.'}</span></div>}
      <NeedsBanner orgID={org.id} />
      {org.paused && <div className="pause-banner" role="status"><strong>{org.name} is paused.</strong><span>New automation is blocked until an organisation administrator resumes it.</span></div>}
      <main id="main-content" className="content"><Outlet key={`${session.data.user.id}:${org.id}`} /></main>
      </div>
    </div>
    <Dialog open={orgDialog} title="Switch organisation" onClose={() => { setOrgDialog(false); requestAnimationFrame(() => orgButtonRef.current?.focus()) }}><div className="org-options">{orgs.map(item => <button key={item.id} className={`org-option ${item.id === org.id ? 'selected' : ''}`} onClick={() => chooseOrg(item.id)}><span className="org-dot">{item.name.slice(0, 1)}</span><span><strong>{item.name}</strong><small>{item.paused ? 'Paused' : 'Active'} · version {item.version}</small></span>{item.id === org.id && <span className="check" aria-label="Current organisation">✓</span>}</button>)}</div></Dialog>
  </div>
}
