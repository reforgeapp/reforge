import { useQuery, type QueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

export const sessionQuery = () => ({ queryKey: ['session'] as const, queryFn: api.getSession })
export const metaQuery = () => ({ queryKey: ['meta'] as const, queryFn: api.getMeta })
export const repositoriesQuery = (orgID: string, query: string) => ({
  queryKey: ['org', orgID, 'repositories', { query }] as const,
  queryFn: ({ signal }: { signal: AbortSignal }) => api.getRepositories(orgID, { limit: 50, query: query || undefined, signal }),
})

export function useSession() { return useQuery({ ...sessionQuery(), retry: false }) }
export function useMeta() { return useQuery(metaQuery()) }

export function clearOrganisationQueries(queryClient: QueryClient) {
  void queryClient.cancelQueries({ predicate: query => query.queryKey[0] === 'org' })
  queryClient.removeQueries({ predicate: query => query.queryKey[0] === 'org' })
}
