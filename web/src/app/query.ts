import { useQuery, type QueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

export const sessionQuery = () => ({ queryKey: ['session'] as const, queryFn: ({ signal }: { signal: AbortSignal }) => api.getSession(signal) })
export const metaQuery = () => ({ queryKey: ['meta'] as const, queryFn: api.getMeta })
export const repositoriesQuery = (orgID: string, query: string) => ({
  queryKey: ['org', orgID, 'repositories', { query }] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getRepositories(orgID, { limit: 50, query: query || undefined, signal }),
})
export const connectionsQuery = (orgID: string, kind?: string, cursor?: string) => ({
  queryKey: ['org', orgID, 'connections', kind ?? 'all', cursor ?? 'first'] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getConnections(orgID, { kind, cursor, signal }),
})
export const connectionQuery = (orgID: string, connectionID: string) => ({
  queryKey: ['org', orgID, 'connections', 'detail', connectionID] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getConnection(orgID, connectionID, signal),
})
export const runnerPoolsQuery = (orgID: string, filter: { q?: string; state?: string } = {}, cursor?: string) => ({
  queryKey: ['org', orgID, 'runner-pools', filter.q ?? '', filter.state ?? '', cursor ?? 'first'] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getRunnerPools(orgID, { q: filter.q, state: filter.state, cursor, signal }),
})
export const runnersQuery = (orgID: string, poolID: string) => ({
  queryKey: ['org', orgID, 'runner-pools', poolID, 'runners'] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getRunners(orgID, poolID, signal),
})

export function useSession() { return useQuery({ ...sessionQuery(), retry: false, refetchInterval: 30_000, refetchOnWindowFocus: true }) }
export function useMeta() { return useQuery(metaQuery()) }

export function clearOrganisationQueries(queryClient: QueryClient) {
  void queryClient.cancelQueries({ predicate: query => query.queryKey[0] === 'org' })
  queryClient.removeQueries({ predicate: query => query.queryKey[0] === 'org' })
}
