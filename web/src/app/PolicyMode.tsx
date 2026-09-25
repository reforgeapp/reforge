import { useState } from 'react'
import { Button } from '../components/Accessible'
import { policyAPI, type Action, type Policy, type Resolved } from '../policy-api'

export const presets = [
  { id: 'observe', label: 'Observe', deny: ['repair', 'publish', 'merge', 'deploy', 'recover'] as Action[] },
  { id: 'propose', label: 'Propose fixes', deny: ['merge', 'deploy', 'recover'] as Action[] },
  { id: 'merge', label: 'Merge eligible fixes', deny: ['deploy', 'recover'] as Action[] },
  { id: 'deliver', label: 'Deliver to approved environments', deny: ['recover'] as Action[], allow: { environments: [] as string[], workflows: [] as string[] } },
] as const

const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function PolicyMode({ orgID, repositoryID, effective, base, currentVersion, csrf, canWrite, onChanged }: { orgID: string; repositoryID: string; effective?: Resolved; base: Policy; currentVersion: number; csrf: string; canWrite: boolean; onChanged: () => void }) {
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const deny = effective?.policy.deny ?? []
  const current = !effective?.layers?.some(layer => layer.scope.kind === 'organisation') || deny.includes('repair') ? 'observe' : deny.includes('merge') ? 'propose' : deny.includes('deploy') ? 'merge' : 'deliver'
  const apply = async (preset: typeof presets[number]) => {
    if (!window.confirm(`Set the organisation to "${preset.label}"?`)) return
    setBusy(preset.id)
    setError('')
    try {
      const reason = `Mode: ${preset.label}`
      const policy: Policy = { ...base, deny: [...new Set([...base.deny.filter(action => action === 'read'), ...preset.deny])], allow: 'allow' in preset ? { ...base.allow, environments: base.allow.environments ?? [], workflows: base.allow.workflows ?? [] } : base.allow }
      const version = await policyAPI.createVersion(orgID, { kind: 'organisation', id: orgID }, policy, reason, csrf)
      const simulation = await policyAPI.simulate(orgID, version.id, repositoryID, '', { action: 'read', recipe: '', model: '', route: '', merge_method: '', environment: '', workflow: '', paths: [], usage: {}, current: { head: '', target: '', tested: '', policy_hash: '', provider_rules: '', capability_version: '', source_sha: '', artifact: '' }, starting_policy_hash: effective?.hash ?? '', evidence: [], paused_scopes: [] }, csrf)
      if (simulation.resolved.problems?.length) throw new Error(simulation.resolved.problems.join('; '))
      await policyAPI.activate(orgID, version.id, repositoryID, '', currentVersion, simulation.hash, reason, csrf)
      onChanged()
    } catch (reason) {
      setError(text(reason))
    } finally {
      setBusy('')
    }
  }
  return <section className="panel policy-mode" aria-label="Organisation mode">
    <div className="panel-head"><h2>Mode</h2></div>
    <div className="segmented" role="group" aria-label="Organisation mode">
      {presets.map(preset => <Button key={preset.id} aria-pressed={current === preset.id} disabled={!canWrite || !csrf || !repositoryID || !!busy} onClick={() => { if (current !== preset.id) void apply(preset) }}>{busy === preset.id ? 'Applying…' : preset.label}</Button>)}
    </div>
    {error && <p className="error-text" role="alert">{error}</p>}
  </section>
}
