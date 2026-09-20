import React from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './app/AppShell'
import { SectionPage } from './app/SectionPage'
import { SignInPage } from './app/SignInPage'
import { api } from './api/client'
import './styles/tokens.css'
import './styles/app.css'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false } },
})

const rootRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>) => ({ q: typeof search.q === 'string' ? search.q : undefined }),
  component: AppShell,
})
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: async () => {
    try {
      const session = await queryClient.ensureQueryData({ queryKey: ['session'], queryFn: api.getSession })
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
