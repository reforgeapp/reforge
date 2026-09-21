import React from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './app/AppShell'
import { SectionPage } from './app/SectionPage'
import { SignInPage } from './app/SignInPage'
import { sessionQuery } from './app/query'
import './styles/tokens.css'
import './styles/app.css'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false } },
})

const rootRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>): { scope_kind?: string; scope_id?: string; campaign?: string; environment?: string; deployment?: string; promotion?: string; q?: string; provider?: string; team_id?: string; status?: string; repository?: string; finding?: string; run?: string; state?: string; category?: string; severity?: string; finding_state?: string; finding_category?: string; finding_severity?: string } => Object.fromEntries(['scope_kind', 'scope_id', 'campaign', 'environment', 'deployment', 'promotion', 'q', 'provider', 'team_id', 'status', 'repository', 'finding', 'run', 'state', 'category', 'severity', 'finding_state', 'finding_category', 'finding_severity'].filter(key => typeof search[key] === 'string' && String(search[key]).length <= 256).map(key => [key, search[key]])),
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
const sectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/org/$orgID/$section',
  component: SectionPage,
})
const routeTree = rootRoute.addChildren([indexRoute, signInRoute, sectionRoute])
const router = createRouter({ routeTree })

declare module '@tanstack/react-router' { interface Register { router: typeof router } }

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>
  </React.StrictMode>,
)
