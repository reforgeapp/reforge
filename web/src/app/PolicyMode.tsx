import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Dialog } from '../components/Accessible'
import { Icon } from '../components/Icons'
import { inventoryAPI } from '../api/inventory'
import { usageAPI } from '../usage-api'
import { autopilotAPI, policyAPI, type Action, type Policy, type Resolved, type Version } from '../policy-api'

export const presets = [
  { id: 'observe', label: 'Observe', title: 'Scan and report only', deny: ['repair', 'publish', 'merge', 'deploy', 'recover'] as Action[] },
  { id: 'propose', label: 'Propose', title: 'Open fix pull requests', deny: ['merge', 'deploy', 'recover'] as Action[] },
  { id: 'merge', label: 'Merge', title: 'Merge fixes that pass the gate', deny: ['deploy', 'recover'] as Action[] },
  { id: 'deliver', label: 'Deliver', title: 'Merge and deploy to approved environments', deny: ['recover'] as Action[] },
] as const

const text = (value: unknown) => value instanceof Error ? value.message : 'Preview unavailable.'
type Impact = { name: string; actions: string[]; blockers: string[]; setupURL?: string; mergeReady?: boolean }
type Preview = { label: string; mode?: typeof presets[number]; work?: boolean; candidate?: Version; proof?: string; repositoryID?: string; impacts: Impact[]; ceiling: string }
const mutations: Action[] = ['repair', 'publish', 'merge', 'deploy', 'recover']

export function PolicyMode({ orgID, repositoryID, effective, base, currentVersion, csrf, canWrite, onChanged }: { orgID: string; repositoryID: string; effective?: Resolved; base: Policy; currentVersion: number; csrf: string; canWrite: boolean; onChanged: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<Preview>()
  const autopilot = useQuery({ queryKey: ['org', orgID, 'autopilot'], queryFn: ({ signal }) => autopilotAPI.get(orgID, signal), refetchInterval: query => query.state.data?.enabled ? 15_000 : false })
  const orgBound = effective?.layers?.some(layer => layer.scope.kind === 'organisation')
  const current = !orgBound || base.deny.includes('repair') ? 'observe' : base.deny.includes('merge') ? 'propose' : base.deny.includes('deploy') ? 'merge' : 'deliver'
  const auto = autopilot.data?.enabled ?? false
  const prepare = async (mode?: typeof presets[number], work?: boolean) => {
    setBusy(true); setOpen(true); setError(''); setPreview(undefined)
    try {
      const label = mode && work ? 'Enable Autopilot' : mode ? `Set organisation permissions to ${mode.label}` : work ? 'Enable organisation maintenance' : 'Disable organisation maintenance'
      const policy: Policy = mode ? { ...base, repository_setup_required: false, deny: [...new Set([...base.deny.filter(action => action === 'read'), ...mode.deny])] } : base
      const activeID = effective?.layers?.find(layer => layer.scope.kind === 'organisation')?.version_id
      if (!mode && !activeID) throw new Error('Set up a repository before enabling organisation work.')
      const candidate = mode ? await policyAPI.createVersion(orgID, { kind: 'organisation', id: orgID }, policy, label, csrf) : await policyAPI.version(orgID, activeID!)
      const repositories = []
      const seen = new Set<string>()
      let cursor: string | undefined
      for (let page = 0; page < 20; page += 1) {
        const result = await inventoryAPI.repositories(orgID, { limit: 100, cursor })
        repositories.push(...result.items)
        if (result.complete) break
        if (!result.next_cursor || seen.has(result.next_cursor) || page === 19) throw new Error('Repository preview incomplete. No change applied.')
        seen.add(result.next_cursor); cursor = result.next_cursor
      }
      const limit = await usageAPI.budget(orgID, 'organisation', orgID)
      const cap = limit.caps.micro_usd
      const ceiling = cap === undefined ? 'No dollar ceiling configured' : `$${(cap / 1_000_000).toFixed(2)} per ${limit.period === 'monthly' ? 'month' : limit.period === 'daily' ? 'day' : 'budget period'}${limit.paused ? ' · paused' : ''}`
      const impacts: Impact[] = []
      let proof: string | undefined
      let firstRepository: string | undefined
      for (const repo of repositories) {
        const currentPolicy = await policyAPI.effective(orgID, repo.id)
        let resolved = currentPolicy
        const blockers = new Set<string>()
        const requested = mutations.filter(action => !policy.deny.includes(action))
        for (const action of requested.length ? requested : ['repair' as Action]) {
          if (!candidate) continue
          const simulation = await policyAPI.simulate(orgID, candidate.id, repo.id, '', { action, recipe: '', model: '', route: '', merge_method: '', environment: '', workflow: '', paths: [], usage: {}, current: { head: '', target: '', tested: '', policy_hash: currentPolicy.hash, provider_rules: '', capability_version: '', source_sha: '', artifact: '' }, starting_policy_hash: currentPolicy.hash, evidence: [], paused_scopes: [] }, csrf)
          resolved = simulation.resolved
          if (!proof) { proof = simulation.hash; firstRepository = repo.id }
          for (const problem of [...(resolved.problems ?? []), ...(simulation.decision.blockers ?? [])]) blockers.add(problem)
        }
        if (resolved.paused) blockers.add('Paused')
        for (const action of requested) if (resolved.policy.deny.includes(action)) blockers.add(`${action} prohibited by repository, team or server controls`)
        let mergeReady: boolean | undefined
        let setupURL: string | undefined
        if (mode?.id === 'deliver' && work) {
          const status = await autopilotAPI.maintenanceStatus(orgID, repo.id)
          const setup = await autopilotAPI.previewMaintenance(orgID, repo.id, { ...status, mode: 'merge', continuous: true }, csrf)
          mergeReady = setup.merge_setup?.ready === true
          if (!mergeReady) {
            blockers.add(setup.merge_setup?.reason || 'Repository merge permissions need checking')
            setupURL = `/org/${encodeURIComponent(orgID)}/repositories?repository=${encodeURIComponent(repo.id)}&setup=autopilot`
          }
        }
        impacts.push({ name: repo.name, actions: mutations.filter(action => !resolved.policy.deny.includes(action)), blockers: [...blockers], mergeReady, setupURL })
      }
      if (!repositories.length) throw new Error('Import a repository before changing organisation maintenance.')
      setPreview({ label, mode, work, candidate, proof, repositoryID: firstRepository, impacts, ceiling })
    } catch (reason) { setError(text(reason)) } finally { setBusy(false) }
  }
  const apply = async () => {
    if (!preview || !autopilot.data || preview.impacts.some(item => item.mergeReady === false)) return
    setBusy(true); setError('')
    let permissionsApplied = false
    try {
      if (preview.mode && preview.candidate && preview.proof && preview.repositoryID) {
        await policyAPI.activate(orgID, preview.candidate.id, preview.repositoryID, '', currentVersion, preview.proof, preview.label, csrf)
        permissionsApplied = true
      }
      if (preview.work !== undefined) await autopilotAPI.put(orgID, autopilot.data.version, preview.work, csrf)
      await autopilot.refetch(); onChanged(); setOpen(false); setPreview(undefined)
    } catch (reason) {
      if (permissionsApplied && preview.work === true) {
        await autopilot.refetch(); onChanged()
        setError(`Permissions updated, but Autopilot did not start: ${text(reason)}`)
      } else setError(text(reason))
      setPreview(undefined)
    } finally { setBusy(false) }
  }
  const disabled = !canWrite || !csrf || !repositoryID || !autopilot.data || busy
  return <section className="panel policy-mode" aria-label="Organisation permissions">
    <div className="panel-head"><h2>Organisation permissions</h2><span className="help-tip" tabIndex={0} title="Applies across the organisation. Repository and team restrictions still apply. Set up one repository from Repositories.">?</span></div>
    <div className="mode-buttons" role="group" aria-label="Organisation permissions">{presets.map(preset => {
      const autopilotButton = preset.id === 'deliver'
      const selected = current === preset.id && (!autopilotButton || auto)
      return <Button key={preset.id} className={autopilotButton ? 'mode-button mode-auto' : 'mode-button'} title={autopilotButton ? 'Automatically fix and merge; deploy only where configured' : preset.title} aria-pressed={selected} disabled={disabled || selected} onClick={() => void prepare(preset, autopilotButton ? true : undefined)}><Icon name={autopilotButton ? 'auto' : preset.id} size={16} />{autopilotButton ? 'Autopilot' : preset.label}</Button>
    })}</div>
    <div className="row-actions"><span className="table-meta">Organisation work {auto ? 'enabled' : 'disabled'}{base.repository_setup_required ? ' · configured repositories only' : ''}</span><Button disabled={disabled} onClick={() => void prepare(undefined, !auto)}>{auto ? 'Disable work' : 'Enable work'}</Button></div>
    {auto && <p className="table-meta" role="status">{autopilot.data?.status || 'Starting'} · {autopilot.data?.queued} fixes started{autopilot.data?.awaiting ? ` · ${autopilot.data.awaiting} awaiting a person` : ''}</p>}
    <Dialog open={open} title="Organisation maintenance preview" onClose={() => { if (!busy) setOpen(false) }}><div className="stack">{busy && <p role="status">Checking repository scope…</p>}{preview && <><p>{preview.label}. Affects {preview.impacts.length} repositories. {preview.ceiling}.</p>{preview.impacts.some(item => item.mergeReady === false) && <div role="status"><p>Finish repository permissions before enabling unattended merges.</p><ul>{preview.impacts.filter(item => item.mergeReady === false).map(item => <li key={item.name}><a href={item.setupURL}>Finish Autopilot setup for {item.name}</a></li>)}</ul></div>}<details><summary>Affected repositories and gates</summary><ul className="compact-list">{preview.impacts.map(item => <li key={item.name}>{item.name}: {item.actions.join(', ') || 'scan only'}{item.blockers.length > 0 && <details><summary>{item.blockers.length} controls or evidence requirements</summary><ul>{item.blockers.map(blocker => <li key={blocker}>{blocker}</li>)}</ul></details>}</li>)}</ul></details><Button className="button button-primary" disabled={busy || preview.impacts.some(item => item.mergeReady === false)} onClick={() => void apply()}>Apply across organisation</Button></>}{error && <p className="error-text" role="alert">{error}</p>}</div></Dialog>
  </section>
}
