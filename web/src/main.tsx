import React from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './app/AppShell'
import { SectionPage } from './app/SectionPage'
import { SignInPage } from './app/SignInPage'
import { InviteLandingPage } from './app/InviteLandingPage'
import { sessionQuery } from './app/query'
import './styles/tokens.css'
import './styles/app.css'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false } },
})

const rootRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>): { scope_kind?: string; scope_id?: string; campaign?: string; environment?: string; deployment?: string; promotion?: string; q?: string; provider?: string; team_id?: string; status?: string; repository?: string; finding?: string; run?: string; state?: string; category?: string; severity?: string; finding_state?: string; finding_category?: string; finding_severity?: string; pool?: string; connection?: string; connection_tab?: string; kind?: string; change?: string; event?: string; github_result?: string; github_reason?: string; sync?: string; audit_repository?: string; audit_action?: string; audit_query?: string; audit_actor?: string; audit_since?: string; audit_until?: string } => Object.fromEntries(['scope_kind', 'scope_id', 'campaign', 'environment', 'deployment', 'promotion', 'q', 'provider', 'team_id', 'status', 'repository', 'finding', 'run', 'state', 'category', 'severity', 'finding_state', 'finding_category', 'finding_severity', 'pool', 'connection', 'connection_tab', 'kind', 'change', 'event', 'github_result', 'github_reason', 'sync', 'audit_repository', 'audit_action', 'audit_query', 'audit_actor', 'audit_since', 'audit_until'].filter(key => typeof search[key] === 'string' && String(search[key]).length <= 256).map(key => [key, search[key]])),
  component: AppShell,
})
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: async () => {
    try {
      const session = await queryClient.ensureQueryData(sessionQuery())
      const org = session.organisations[0]
      if (org) throw redirect({ to: '/org/$orgID/$section', params: { orgID: org.id, section: 'overview' }, search: { q: undefined } })
    } catch (error) {
      if (error instanceof Response || (error && typeof error === 'object' && 'to' in error)) throw error
      throw redirect({ to: '/sign-in', search: { q: undefined } })
    }
  },
  component: () => null,
})
const signInRoute = createRoute({ getParentRoute: () => rootRoute, path: '/sign-in', component: SignInPage })
const inviteRoute = createRoute({ getParentRoute: () => rootRoute, path: '/invite', component: InviteLandingPage })
const sectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/org/$orgID/$section',
  component: SectionPage,
})
const routeTree = rootRoute.addChildren([indexRoute, signInRoute, inviteRoute, sectionRoute])
const router = createRouter({ routeTree })

declare module '@tanstack/react-router' { interface Register { router: typeof router } }

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>
  </React.StrictMode>,
)
