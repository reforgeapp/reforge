import { Icon } from './Icons'

const names: Record<string, string> = { github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' }

export const forgeName = (provider?: string) => names[provider ?? ''] ?? 'forge'

export function forgeURL(value?: string) {
  try {
    const url = new URL(value ?? '')
    return (url.protocol === 'https:' || url.protocol === 'http:') && !url.username && !url.password ? url.toString().replace(/\/$/, '') : undefined
  } catch {
    return undefined
  }
}

export function forgePages(provider: string | undefined, repository: string) {
  const base = forgeURL(repository)
  if (!base) return []
  return provider === 'gitlab'
    ? [{ label: 'Merge requests', href: `${base}/-/merge_requests` }, { label: 'Issues', href: `${base}/-/issues` }]
    : [{ label: 'Pull requests', href: `${base}/pulls` }, { label: 'Issues', href: `${base}/issues` }]
}

export function ForgeLink({ href, provider, label, iconOnly = false }: { href?: string; provider?: string; label?: string; iconOnly?: boolean }) {
  const url = forgeURL(href)
  if (!url) return null
  const text = label ?? `Open on ${forgeName(provider)}`
  return <a className={`forge-link${iconOnly ? ' forge-link-icon' : ''}`} href={url} target="_blank" rel="noreferrer" aria-label={iconOnly ? text : undefined} title={iconOnly ? text : undefined}>{!iconOnly && text}<Icon name="external" size={14} aria-hidden="true" /></a>
}
