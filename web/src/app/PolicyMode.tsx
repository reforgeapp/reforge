import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '../components/Accessible'
import { Icon } from '../components/Icons'
import { autopilotAPI, policyAPI, type Action, type Policy, type Resolved } from '../policy-api'

export const presets = [
  { id: 'observe', label: 'Observe', title: 'Scan and report only', deny: ['repair', 'publish', 'merge', 'deploy', 'recover'] as Action[] },
  { id: 'propose', label: 'Propose', title: 'Open fix pull requests', deny: ['merge', 'deploy', 'recover'] as Action[] },
  { id: 'merge', label: 'Merge', title: 'Merge fixes that pass the gate', deny: ['deploy', 'recover'] as Action[] },
  { id: 'deliver', label: 'Deliver', title: 'Merge and deploy to approved environments', deny: ['recover'] as Action[], allow: { environments: [] as string[], workflows: [] as string[] } },
] as const

const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'

export function PolicyMode({ orgID, repositoryID, effective, base, currentVersion, csrf, canWrite, onChanged }: { orgID: string; repositoryID: string; effective?: Resolved; base: Policy; currentVersion: number; csrf: string; canWrite: boolean; onChanged: () => void }) {
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const autopilot = useQuery({ queryKey: ['org', orgID, 'autopilot'], queryFn: ({ signal }) => autopilotAPI.get(orgID, signal), refetchInterval: query => query.state.data?.enabled ? 15_000 : false })
  const deny = effective?.policy.deny ?? []
  const current = !effective?.layers?.some(layer => layer.scope.kind === 'organisation') || deny.includes('repair') ? 'observe' : deny.includes('merge') ? 'propose' : deny.includes('deploy') ? 'merge' : 'deliver'
  const setPolicy = async (preset: typeof presets[number]) => {
    const reason = `Mode: ${preset.label}`
    const policy: Policy = { ...base, deny: [...new Set([...base.deny.filter(action => action === 'read'), ...preset.deny])], allow: 'allow' in preset ? { ...base.allow, environments: base.allow.environments ?? [], workflows: base.allow.workflows ?? [] } : base.allow }
    const version = await policyAPI.createVersion(orgID, { kind: 'organisation', id: orgID }, policy, reason, csrf)
    const simulation = await policyAPI.simulate(orgID, version.id, repositoryID, '', { action: 'read', recipe: '', model: '', route: '', merge_method: '', environment: '', workflow: '', paths: [], usage: {}, current: { head: '', target: '', tested: '', policy_hash: '', provider_rules: '', capability_version: '', source_sha: '', artifact: '' }, starting_policy_hash: effective?.hash ?? '', evidence: [], paused_scopes: [] }, csrf)
    if (simulation.resolved.problems?.length) throw new Error(simulation.resolved.problems.join('; '))
    await policyAPI.activate(orgID, version.id, repositoryID, '', currentVersion, simulation.hash, reason, csrf)
  }
  const auto = autopilot.data?.enabled ?? false
  const selected = auto ? 'auto' : current
  const choose = async (id: string) => {
    if (id === selected || !autopilot.data) return
    const preset = presets.find(item => item.id === (id === 'auto' ? 'deliver' : id))!
    if (!window.confirm(id === 'auto' ? 'Let Reforge maintain repositories on its own, including merging and delivery? Spending is limited by Usage → Budgets.' : `Set the organisation to "${preset.label}"?`)) return
    setBusy(id)
    setError('')
    try {
      if (preset.id !== current) {
        await setPolicy(preset)
        onChanged()
      }
      if ((id === 'auto') !== auto) {
        await autopilotAPI.put(orgID, autopilot.data.version, id === 'auto', csrf)
        await autopilot.refetch()
      }
    } catch (reason) {
      setError(text(reason))
    } finally {
      setBusy('')
    }
  }
  const disabled = !canWrite || !csrf || !repositoryID || !autopilot.data || !!busy
  return <section className="panel policy-mode" aria-label="Organisation mode">
    <div className="panel-head"><h2>Mode</h2></div>
    <div className="mode-buttons" role="group" aria-label="Organisation mode">
      {presets.map(preset => <Button key={preset.id} className="mode-button" title={preset.title} aria-pressed={selected === preset.id} disabled={disabled} onClick={() => void choose(preset.id)}><Icon name={preset.id} size={16} />{busy === preset.id ? 'Applying…' : preset.label}</Button>)}
      <Button className="mode-button mode-auto" title="Reforge owns maintenance: fixes, merges and delivers on its own" aria-pressed={selected === 'auto'} disabled={disabled} onClick={() => void choose('auto')}><Icon name="auto" size={16} />{busy === 'auto' ? 'Applying…' : 'Auto-mode'}</Button>
    </div>
    {auto && <p className="table-meta" role="status">{autopilot.data?.status || 'Starting'} · {autopilot.data?.queued} fixes started · {autopilot.data?.skipped} skipped</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </section>
}
