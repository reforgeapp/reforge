import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { Button } from '../components/Accessible'
import { DataTable, EmptyTable } from '../components/DataTable'
import { StatePanel } from '../components/StatePanel'
import { StatusBadge } from '../components/Status'
import { PolicyMode, presets } from './PolicyMode'
import { policyAPI, type Action, type Binding, type Input, type Limits, type Policy, type Scope, type Simulation, type Version } from '../policy-api'
import { useSession } from './query'
import { Tabs } from '../components/Workspace'
import { PolicyImpactPreview } from './PolicyImpactPreview'
import '../styles/policies.css'

const actions: Action[] = ['read', 'repair', 'publish', 'merge', 'deploy', 'recover']
const text = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const emptyPolicy = (): Policy => ({ schema: 'maintenance/v1', allow: { recipes: null, models: null, routes: null, merge_methods: null, environments: null, workflows: null }, deny: [], forbidden_paths: [], limits: {}, required: [], defaults: {}, paused: false })
const normalisePolicy = (value?: Partial<Policy> | null): Policy => {
  const input = (value ?? {}) as Partial<Policy>
  return {
    ...emptyPolicy(), ...input,
    allow: { ...emptyPolicy().allow, ...(input.allow ?? {}) },
    deny: Array.isArray(input.deny) ? input.deny : [],
    forbidden_paths: Array.isArray(input.forbidden_paths) ? input.forbidden_paths : [],
    limits: { ...(input.limits ?? {}) },
    required: Array.isArray(input.required) ? input.required : [],
    defaults: { ...(input.defaults ?? {}) },
    paused: Boolean(input.paused),
  }
}
const split = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean)
const parseJSON = <T,>(value: string, fallback: T) => value.trim() ? JSON.parse(value) as T : fallback
const shortID = (value: string) => value.length > 14 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value

export function PoliciesPage({ orgID }: { orgID: string }) {
  const session = useSession()
  const csrf = session.data?.csrf_token ?? ''
  const membership = session.data?.memberships.find(item => item.org_id === orgID)
  const role = membership?.role
  const membershipFingerprint = JSON.stringify(membership ?? null)
  const repositories = useInfiniteQuery({ queryKey: ['org', orgID, 'policy-repositories'], queryFn: ({ pageParam, signal }) => api.getRepositories(orgID, { cursor: pageParam, limit: 50, signal }), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const teams = useInfiniteQuery({ queryKey: ['org', orgID, 'teams', 'policy-picker'], queryFn: ({ pageParam, signal }) => policyAPI.teams(orgID, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor })
  const teamItems = teams.data?.pages.flatMap(page => page.items) ?? []
  const repos = repositories.data?.pages.flatMap(page => page.items) ?? []
  const [repositoryID, setRepositoryID] = useState('')
  const [scopeKind, setScopeKind] = useState('organisation')
  const canWrite = role === 'owner' || role === 'admin' && scopeKind !== 'organisation'
  const [scopeID, setScopeID] = useState(orgID)
  const [versionID, setVersionID] = useState('')
  const [error, setError] = useState('')
  const [editorTab, setEditorTab] = useState('scope')
  const effective = useQuery({ queryKey: ['org', orgID, 'policy-effective', repositoryID], queryFn: ({ signal }) => policyAPI.effective(orgID, repositoryID, signal), enabled: !!repositoryID })
  const versions = useInfiniteQuery({ queryKey: ['org', orgID, 'policy-versions', scopeKind, scopeID], queryFn: ({ pageParam, signal }) => policyAPI.versions(orgID, { kind: scopeKind, id: scopeID }, pageParam, signal), initialPageParam: undefined as string | undefined, getNextPageParam: page => page.complete ? undefined : page.next_cursor, enabled: !!scopeID })
  const selectedVersion = useQuery({ queryKey: ['org', orgID, 'policy-version', versionID], queryFn: ({ signal }) => policyAPI.version(orgID, versionID, signal), enabled: !!versionID })
  useEffect(() => { if (!repositoryID && repos[0]?.id) setRepositoryID(repos[0].id) }, [repositoryID, repos])
  useEffect(() => { if (scopeKind === 'organisation') setScopeID(orgID); else if (scopeKind === 'repository') setScopeID(repositoryID) }, [orgID, repositoryID, scopeKind])
  const rows = versions.data?.pages.flatMap(page => page.items) ?? []
  const currentVersion = effective.data?.layers?.find(layer => layer.scope.kind === scopeKind && layer.scope.id === scopeID)?.binding_version ?? 0
  if (repositories.isLoading) return <StatePanel kind="loading" title="Loading policy controls" detail="Fetching repositories and effective policy evidence." />
  if (repositories.error) return <StatePanel kind="error" title="Policy controls unavailable" detail={text(repositories.error)} action={<Button onClick={() => void repositories.refetch()}>Retry</Button>} />
  return (
    <div className="stack policies-page policy-workspace">
      <div className="policy-context-row">
        <PolicyScopeBar
        repos={repos}
        teams={teamItems}
        repositoryID={repositoryID}
        scopeKind={scopeKind}
        scopeID={scopeID}
        canWrite={canWrite}
        onRepositoryChange={value => {
          setRepositoryID(value)
          setVersionID('')
        }}
        onScopeChange={value => {
          setScopeKind(value)
          setScopeID(value === 'team' ? '' : value === 'repository' ? repositoryID : orgID)
          setVersionID('')
          setError('')
        }}
        onTeamChange={value => {
          setScopeID(value)
          setVersionID('')
          setError('')
        }}
        />
        <PolicyQueryAlerts
        repositories={repositories}
        teams={teams}
        versions={versions}
        selectedVersion={selectedVersion}
        />
        <section className="policy-effective" aria-label="Effective policy summary">
        {effective.isLoading ? <p className="table-meta">Loading effective policy…</p> : null}
        {effective.error ? <p className="error-text" role="alert">Effective policy unavailable: {text(effective.error)}</p> : null}
        {effective.data ? <EffectivePolicy value={effective.data} /> : null}
        {!effective.isLoading && !effective.error && !effective.data ? <p>Select a repository.</p> : null}
      </section>
      </div>
      <PolicyMode orgID={orgID} repositoryID={repositoryID} effective={effective.data} base={normalisePolicy(effective.data?.layers?.find(layer => layer.scope.kind === 'organisation')?.policy)} currentVersion={effective.data?.layers?.find(layer => layer.scope.kind === 'organisation')?.binding_version ?? 0} csrf={csrf} canWrite={role === 'owner'} onChanged={() => { void effective.refetch(); void versions.refetch() }} />
      <div className="policy-layout">
        <section className="policy-editor-surface" aria-label="Policy editor">
          <Tabs
            id="policy"
            label="Policy editor"
            items={[
              { id: 'scope', label: 'Scope' },
              { id: 'recipes', label: 'Recipes' },
              { id: 'changes', label: 'Changes' },
              { id: 'spend', label: 'Models & spend' },
              { id: 'merge', label: 'Merge' },
              { id: 'deploy', label: 'Deploy' },
              { id: 'review', label: 'Review' },
            ]}
            value={editorTab}
            onChange={setEditorTab}
          />
          <PolicyEditor
            key={`${scopeKind}:${scopeID}:${repositoryID}:${versionID}:${membershipFingerprint}`}
            editorTab={editorTab}
            orgID={orgID}
            scope={{ kind: scopeKind, id: scopeID }}
            repositoryID={repositoryID}
            primaryTeamID={effective.data?.primary_team_id ?? ''}
            teams={teamItems.filter(team => team.repository_ids.includes(repositoryID))}
            effectiveHash={effective.data?.hash ?? ''}
            currentVersion={currentVersion}
            selected={selectedVersion.data}
            csrf={csrf}
            canWrite={canWrite}
            error={error}
            setError={setError}
            onSaved={version => {
              setVersionID(version.id)
              void versions.refetch()
            }}
            onChanged={() => {
              void effective.refetch()
              void versions.refetch()
            }}
          />
        </section>
        <VersionHistoryRail
          rows={rows}
          selectedID={versionID}
          complete={versions.data?.pages.at(-1)?.complete ?? true}
          loading={versions.isFetchingNextPage}
          onMore={() => void versions.fetchNextPage()}
          onSelect={setVersionID}
        />
      </div>
    </div>
  )
}

function VersionHistoryRail(props: {
  rows: Version[]
  selectedID: string
  complete: boolean
  loading: boolean
  onMore: () => void
  onSelect: (id: string) => void
}) {
  const [compact, setCompact] = useState(false)
  const rail = useRef<HTMLDetailsElement>(null)

  useEffect(() => {
    const media = window.matchMedia('(max-width: 980px)')
    const update = () => setCompact(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])

  return (
    <details ref={rail} className="policy-version-rail" open={!compact}>
      <summary>Version history</summary>
      <VersionHistory {...props} onSelect={id => {
        props.onSelect(id)
        if (compact && rail.current) rail.current.open = false
      }} />
    </details>
  )
}

function PolicyScopeBar({ repos, teams, repositoryID, scopeKind, scopeID, canWrite, onRepositoryChange, onScopeChange, onTeamChange }: {
  repos: Array<{ id: string; name?: string }>
  teams: Array<{ id: string; name: string }>
  repositoryID: string
  scopeKind: string
  scopeID: string
  canWrite: boolean
  onRepositoryChange: (value: string) => void
  onScopeChange: (value: string) => void
  onTeamChange: (value: string) => void
}) {
  return (
    <div className="policy-scope-controls" role="group" aria-label="Policy scope">
      <label>
        Repository
        <select aria-label="Policy repository" value={repositoryID} onChange={event => onRepositoryChange(event.target.value)}>
          <option value="">Choose repository</option>
          {repos.map(repo => <option key={repo.id} value={repo.id}>{repo.name ?? repo.id}</option>)}
        </select>
      </label>
      <label>
        Version scope
        <select aria-label="Policy scope" value={scopeKind} onChange={event => onScopeChange(event.target.value)}>
          <option value="organisation">Organisation</option>
          <option value="repository">Repository</option>
          <option value="team">Team</option>
        </select>
      </label>
      {scopeKind === 'team' && (
        <label>
          Team
          <select aria-label="Policy team" value={scopeID} onChange={event => onTeamChange(event.target.value)}>
            <option value="">Choose team</option>
            {teams.map(team => <option key={team.id} value={team.id}>{team.name}</option>)}
          </select>
        </label>
      )}
      {!canWrite && <span className="policy-access-state">Read only</span>}
    </div>
  )
}

type PolicyPageQuery = {
  hasNextPage: boolean | undefined
  isFetchingNextPage: boolean
  error: unknown
  fetchNextPage: () => Promise<unknown>
}

type PolicyItemQuery = { error: unknown }

function PolicyQueryAlerts({ repositories, teams, versions, selectedVersion }: {
  repositories: PolicyPageQuery
  teams: PolicyPageQuery
  versions: PolicyPageQuery
  selectedVersion: PolicyItemQuery
}) {
  return (
    <div className="policy-query-status" aria-live="polite">
      {Boolean(repositories.hasNextPage) && (
        <Button disabled={repositories.isFetchingNextPage} onClick={() => void repositories.fetchNextPage()}>
          {repositories.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}
        </Button>
      )}
      {Boolean(teams.error) && <p role="alert">Teams unavailable: {text(teams.error)}</p>}
      {Boolean(teams.hasNextPage) && <Button onClick={() => void teams.fetchNextPage()} disabled={teams.isFetchingNextPage}>Load more teams</Button>}
      {Boolean(versions.error) && <p className="error-text" role="alert">Version history unavailable: {text(versions.error)}</p>}
      {Boolean(selectedVersion.error) && <p className="error-text" role="alert">Selected version unavailable: {text(selectedVersion.error)}</p>}
    </div>
  )
}

function EffectivePolicy({ value }: { value: {
  hash: string
  layers?: Array<{ scope: Scope; version_id: string; binding_version: number }>
  policy?: Policy | null
  paused: boolean
  scope_paused: boolean
  missing_defaults?: string[] | null
  problems?: string[] | null
} }) {
  const policy = normalisePolicy(value.policy)
  const layers = value.layers ?? []
  const issues = [...(value.missing_defaults ?? []), ...(value.problems ?? [])]
  const caps = [
    ['Budget (micro-USD)', policy.limits.budget],
    ['Concurrency', policy.limits.concurrency],
    ['Attempts', policy.limits.attempts],
    ['Changed files', policy.limits.changed_files],
    ['Changed lines', policy.limits.changed_lines],
    ['Open changes', policy.limits.open_changes],
  ].filter(([, cap]) => cap !== undefined)
  const paused = value.paused || value.scope_paused

  return (
    <section className="policy-effective-content" aria-label="Effective policy evidence">
      <div className="policy-effective-heading">
        <StatusBadge label={paused ? 'Paused' : 'Active'} tone={paused ? 'amber' : 'green'} />
        <span>{layers.length ? `${layers.length} inherited layer${layers.length === 1 ? '' : 's'}` : 'Organisation baseline'}</span>
        <span>{issues.length ? `${issues.length} constraint${issues.length === 1 ? '' : 's'}` : 'No constraints'}</span>
      </div>
      <dl className="detail-list policy-effective-values">
        <div>
          <dt>Environments</dt>
          <dd>{policy.allow.environments === null ? 'Inherited / unrestricted' : policy.allow.environments.join(', ') || 'None permitted'}</dd>
        </div>
        <div>
          <dt>Limits</dt>
          <dd>{caps.length ? caps.map(([label, cap]) => `${label}: ${cap}`).join(' · ') : 'No limits'}</dd>
        </div>
      </dl>
      <details>
        <summary>Evidence</summary>
        <dl className="detail-list">
          <div><dt>Effective hash</dt><dd><code>{value.hash || 'Unknown'}</code></dd></div>
          <div><dt>Inherited layers</dt><dd>{layers.map(layer => `${layer.scope.kind} · v${layer.binding_version}`).join('; ') || 'None'}</dd></div>
          <div><dt>Denied actions</dt><dd>{policy.deny.join(', ') || 'None'}</dd></div>
          <div><dt>Required rules</dt><dd>{policy.required.map(rule => rule.id).join(', ') || 'None'}</dd></div>
          <div><dt>Constraints</dt><dd>{issues.join('; ') || 'None'}</dd></div>
        </dl>
        <details>
          <summary>Rules JSON</summary>
          <pre>{JSON.stringify(policy, null, 2)}</pre>
        </details>
      </details>
    </section>
  )
}

type PolicyEditorProps = {
  editorTab: string
  orgID: string
  scope: Scope
  repositoryID: string
  primaryTeamID: string
  effectiveHash: string
  currentVersion: number
  selected?: Version
  teams: Array<{ id: string; name: string }>
  csrf: string
  canWrite: boolean
  error: string
  setError: (value: string) => void
  onSaved: (version: Version) => void
  onChanged: () => void
}

function PolicyEditor({ editorTab, orgID, scope, repositoryID, primaryTeamID, effectiveHash, currentVersion, selected, teams, csrf, canWrite, error, setError, onSaved, onChanged }: PolicyEditorProps) {
  const [primary, setPrimary] = useState(primaryTeamID)
  const [policy, setPolicy] = useState<Policy>(normalisePolicy(selected?.policy))
  const [rawJSON, setRawJSON] = useState('')
  const [reason, setReason] = useState(selected?.reason ?? '')
  const [simulation, setSimulation] = useState<Simulation>()
  const [busy, setBusy] = useState(false)
  const [action, setAction] = useState<Action>('read')
  const [recipe, setRecipe] = useState('')
  const [model, setModel] = useState('')
  const [route, setRoute] = useState('')
  const [environment, setEnvironment] = useState('')
  const [workflow, setWorkflow] = useState('')
  const [mergeMethod, setMergeMethod] = useState('')
  const [paths, setPaths] = useState('')
  const [evidenceJSON, setEvidenceJSON] = useState('[]')
  const [usage, setUsage] = useState<Limits>({})
  const [binding, setBinding] = useState<Binding>({
    head: '', target: '', tested: '', policy_hash: '', provider_rules: '',
    capability_version: '', source_sha: '', artifact: '',
  })
  const [showSimulation, setShowSimulation] = useState(false)
  const [impactAction, setImpactAction] = useState<Action>('repair')
  const [presetID, setPresetID] = useState('observe')

  useEffect(() => {
    setPolicy(normalisePolicy(selected?.policy))
    setRawJSON('')
    setReason(selected?.reason ?? '')
    setSimulation(undefined)
    setBinding({
      head: '', target: '', tested: '', policy_hash: '', provider_rules: '',
      capability_version: '', source_sha: '', artifact: '',
    })
  }, [selected?.id])

  useEffect(() => {
    setSimulation(undefined)
  }, [effectiveHash, primary, repositoryID, currentVersion])

  useEffect(() => {
    setPrimary(primaryTeamID)
  }, [primaryTeamID])

  const update = (next: Policy) => {
    setPolicy(next)
    setSimulation(undefined)
  }

  const updateList = (key: keyof Policy['allow'], value: string[] | null) => {
    update({ ...policy, allow: { ...policy.allow, [key]: value } })
  }

  const numberValue = (value: string) => value === '' ? undefined : Number(value)

  const applyPreset = () => {
    if (!canWrite || !csrf || rawJSON.trim()) return
    const preset = presets.find(item => item.id === presetID) ?? presets[0]
    const allow = 'allow' in preset
      ? {
          environments: policy.allow.environments ?? [],
          workflows: policy.allow.workflows ?? [],
        }
      : {}
    update({
      ...policy,
      deny: Array.from(new Set([
        ...policy.deny.filter(item => item === 'read'),
        ...preset.deny,
      ])) as Action[],
      allow: { ...policy.allow, ...allow },
    })
  }

  const savedPolicy = normalisePolicy(selected?.policy)
  const draftPolicy = rawJSON.trim()
    ? (() => {
        try {
          return normalisePolicy(parseJSON<Policy>(rawJSON, policy))
        } catch {
          return policy
        }
      })()
    : policy
  const dirtyDraft = Boolean(selected) && (
    rawJSON.trim() !== '' || JSON.stringify(draftPolicy) !== JSON.stringify(savedPolicy)
  )

  const input = (): Input => ({
    action,
    recipe,
    model,
    route,
    merge_method: mergeMethod,
    environment,
    workflow,
    paths: split(paths),
    usage,
    current: binding,
    starting_policy_hash: effectiveHash,
    evidence: parseJSON<unknown[]>(evidenceJSON, []),
    paused_scopes: [],
  })

  const impactInput: Input = {
    action: impactAction,
    recipe,
    model,
    route,
    merge_method: mergeMethod,
    environment,
    workflow,
    paths: split(paths),
    usage,
    current: binding,
    starting_policy_hash: effectiveHash,
    evidence: [],
    paused_scopes: [],
  }

  const save = async () => {
    setBusy(true)
    setError('')
    try {
      const next = normalisePolicy(rawJSON.trim()
        ? parseJSON<Policy>(rawJSON, policy)
        : policy)
      onSaved(await policyAPI.createVersion(orgID, scope, next, reason, csrf))
    } catch (reasonValue) {
      setError(reasonValue instanceof SyntaxError ? 'Policy JSON must be valid JSON.' : text(reasonValue))
    } finally {
      setBusy(false)
    }
  }

  const simulate = async () => {
    setBusy(true)
    setError('')
    try {
      const result = await policyAPI.simulate(
        orgID,
        selected?.id ?? '',
        repositoryID,
        scope.kind === 'repository' ? primary : '',
        input(),
        csrf,
      )
      setSimulation(result)
    } catch (reasonValue) {
      setError(reasonValue instanceof SyntaxError ? 'Rollout, binding and evidence must be valid JSON.' : text(reasonValue))
    } finally {
      setBusy(false)
    }
  }

  const activate = async () => {
    if (!simulation || !selected) return
    setBusy(true)
    setError('')
    try {
      await policyAPI.activate(
        orgID,
        selected.id,
        repositoryID,
        scope.kind === 'repository' ? primary : '',
        currentVersion,
        simulation.hash,
        reason,
        csrf,
      )
      setSimulation(undefined)
      onChanged()
    } catch (reasonValue) {
      setError(text(reasonValue))
    } finally {
      setBusy(false)
    }
  }

  const panel = (id: string, children: ReactNode) => (
    <section
      id={`policy-panel-${id}`}
      role="tabpanel"
      aria-labelledby={`policy-tab-${id}`}
      hidden={editorTab !== id}
    >
      {children}
    </section>
  )

  const list = (key: keyof Policy['allow']) => (
    <AllowList
      name={key}
      value={policy.allow[key]}
      disabled={!canWrite || busy}
      onChange={value => updateList(key, value)}
    />
  )

  return (
    <div className="stack policy-editor">
      {panel('scope', (
        <div className="form-grid">
          {scope.kind === 'repository' && (
            <label>
              Primary team
              <select aria-label="Primary team" value={primary} disabled={!canWrite || busy} onChange={event => setPrimary(event.target.value)}>
                <option value="">No primary team</option>
                {teams.map(team => <option key={team.id} value={team.id}>{team.name}</option>)}
              </select>
            </label>
          )}
          <div className="wide policy-preset">
            <label>
              Draft preset
              <select aria-label="Draft preset" value={presetID} disabled={!canWrite || busy} onChange={event => setPresetID(event.target.value)}>
                {presets.map(preset => <option key={preset.id} value={preset.id}>{preset.label}</option>)}
              </select>
            </label>
            <Button disabled={!canWrite || !csrf || busy || !!rawJSON.trim()} onClick={applyPreset}>Apply preset</Button>
            {rawJSON.trim() && <span className="table-meta">Clear JSON override to use preset.</span>}
          </div>
          <label className="wide policy-pause">
            <input type="checkbox" checked={policy.paused} disabled={!canWrite || busy} onChange={event => update({ ...policy, paused: event.target.checked })} />
            Pause this scope
          </label>
          <details className="wide">
            <summary>Advanced policy JSON</summary>
            <label>
              Schema
              <input value={policy.schema} disabled={!canWrite || busy} onChange={event => update({ ...policy, schema: event.target.value })} />
            </label>
            <label>
              Raw policy JSON
              <textarea aria-label="Raw policy JSON" value={rawJSON} placeholder={JSON.stringify(policy, null, 2)} disabled={!canWrite || busy} onChange={event => { setRawJSON(event.target.value); setSimulation(undefined) }} />
            </label>
          </details>
        </div>
      ))}
      {panel('recipes', <div className="form-grid">{list('recipes')}</div>)}
      {panel('changes', (
        <div className="form-grid">
          <label className="wide">
            Denied actions
            <input value={policy.deny.join(', ')} disabled={!canWrite || busy} onChange={event => update({ ...policy, deny: split(event.target.value) as Action[] })} />
          </label>
          <label className="wide">
            Forbidden paths
            <input value={policy.forbidden_paths.join(', ')} disabled={!canWrite || busy} onChange={event => update({ ...policy, forbidden_paths: split(event.target.value) })} />
          </label>
          <details className="wide">
            <summary>Required rules and defaults</summary>
            <pre>{JSON.stringify({ required: policy.required, defaults: policy.defaults }, null, 2)}</pre>
          </details>
        </div>
      ))}
      {panel('spend', (
        <div className="form-grid">
          {list('models')}
          {list('routes')}
          <label>
            Default model
            <input value={policy.defaults.model ?? ''} disabled={!canWrite || busy} onChange={event => update({ ...policy, defaults: { ...policy.defaults, model: event.target.value } })} />
          </label>
          <label>
            Default route
            <input value={policy.defaults.route ?? ''} disabled={!canWrite || busy} onChange={event => update({ ...policy, defaults: { ...policy.defaults, route: event.target.value } })} />
          </label>
          <label>
            Budget (micro-USD)
            <input type="number" min="0" value={policy.limits.budget ?? ''} disabled={!canWrite || busy} onChange={event => update({ ...policy, limits: { ...policy.limits, budget: numberValue(event.target.value) } })} />
          </label>
          <label>
            Concurrency
            <input type="number" min="0" value={policy.limits.concurrency ?? ''} disabled={!canWrite || busy} onChange={event => update({ ...policy, limits: { ...policy.limits, concurrency: numberValue(event.target.value) } })} />
          </label>
        </div>
      ))}
      {panel('merge', <div className="form-grid">{list('merge_methods')}</div>)}
      {panel('deploy', <div className="form-grid">{list('environments')}{list('workflows')}</div>)}
      {panel('review', (
        <div className="policy-review">
          <PolicyDraftHeader
            selected={selected}
            dirty={dirtyDraft}
            reason={reason}
            disabled={!canWrite || !csrf || busy || !scope.id}
            onReasonChange={setReason}
            onSave={() => void save()}
          />
          <div className="policy-review-stages">
            <PolicyImpactPreview
              orgID={orgID}
              scope={scope}
              candidate={selected}
              primaryTeamID={primary}
              csrf={csrf}
              disabled={!canWrite || busy || dirtyDraft}
              input={impactInput}
              onActionChange={setImpactAction}
            />
            <section className="policy-simulation" aria-label="Candidate simulation">
              <div className="subsection-actions">
                <strong>Candidate simulation</strong>
                <Button onClick={() => setShowSimulation(value => !value)} aria-expanded={showSimulation}>
                  {showSimulation ? 'Hide inputs' : 'Open simulation'}
                </Button>
              </div>
              {showSimulation && (
                <fieldset>
                  <legend>Simulate candidate rollout</legend>
                  <div className="form-grid">
                    <label>
                      Action
                      <select value={action} onChange={event => { setAction(event.target.value as Action); setSimulation(undefined) }}>
                        {actions.map(item => <option key={item} value={item}>{item}</option>)}
                      </select>
                    </label>
                    <label>Recipe<input value={recipe} onChange={event => { setRecipe(event.target.value); setSimulation(undefined) }} /></label>
                    <label>Model<input value={model} onChange={event => { setModel(event.target.value); setSimulation(undefined) }} /></label>
                    <label>Route<input value={route} onChange={event => { setRoute(event.target.value); setSimulation(undefined) }} /></label>
                    <label>Environment<input value={environment} onChange={event => { setEnvironment(event.target.value); setSimulation(undefined) }} /></label>
                    <label>Workflow<input value={workflow} onChange={event => { setWorkflow(event.target.value); setSimulation(undefined) }} /></label>
                    <label className="wide">Changed paths<input value={paths} onChange={event => { setPaths(event.target.value); setSimulation(undefined) }} /></label>
                    <label>Budget cap (micro-USD)<input type="number" min="0" value={usage.budget ?? ''} onChange={event => { setUsage({ ...usage, budget: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Concurrency cap<input type="number" min="0" value={usage.concurrency ?? ''} onChange={event => { setUsage({ ...usage, concurrency: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Attempt cap<input type="number" min="0" value={usage.attempts ?? ''} onChange={event => { setUsage({ ...usage, attempts: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Changed-file cap<input type="number" min="0" value={usage.changed_files ?? ''} onChange={event => { setUsage({ ...usage, changed_files: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Changed-line cap<input type="number" min="0" value={usage.changed_lines ?? ''} onChange={event => { setUsage({ ...usage, changed_lines: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Open-change cap<input type="number" min="0" value={usage.open_changes ?? ''} onChange={event => { setUsage({ ...usage, open_changes: numberValue(event.target.value) }); setSimulation(undefined) }} /></label>
                    <label>Merge method<input value={mergeMethod} onChange={event => { setMergeMethod(event.target.value); setSimulation(undefined) }} /></label>
                    <details>
                      <summary>Evidence JSON</summary>
                      <label>Evidence<textarea value={evidenceJSON} onChange={event => { setEvidenceJSON(event.target.value); setSimulation(undefined) }} /></label>
                    </details>
                    <label>Head revision<input value={binding.head} onChange={event => { setBinding({ ...binding, head: event.target.value }); setSimulation(undefined) }} /></label>
                    <label>Target revision<input value={binding.target} onChange={event => { setBinding({ ...binding, target: event.target.value }); setSimulation(undefined) }} /></label>
                    <label>Tested revision<input value={binding.tested} onChange={event => { setBinding({ ...binding, tested: event.target.value }); setSimulation(undefined) }} /></label>
                    <label>Policy hash<input value={binding.policy_hash} onChange={event => { setBinding({ ...binding, policy_hash: event.target.value }); setSimulation(undefined) }} /></label>
                    <label>Provider rules<input value={binding.provider_rules} onChange={event => { setBinding({ ...binding, provider_rules: event.target.value }); setSimulation(undefined) }} /></label>
                  </div>
                  <Button disabled={!canWrite || !csrf || busy || !selected?.id || !repositoryID || dirtyDraft} onClick={() => void simulate()}>
                    Simulate candidate rollout
                  </Button>
                  {simulation && <SimulationResult simulation={simulation} onActivate={() => void activate()} canActivate={canWrite && !!csrf && !!selected && !dirtyDraft && !busy && !!reason.trim()} />}
                </fieldset>
              )}
            </section>
          </div>
        </div>
      ))}
      {error && <p className="error-text" role="alert">{error}</p>}
    </div>
  )
}

function PolicyDraftHeader({ selected, dirty, reason, disabled, onReasonChange, onSave }: {
  selected?: Version
  dirty: boolean
  reason: string
  disabled: boolean
  onReasonChange: (value: string) => void
  onSave: () => void
}) {
  return (
    <div className="policy-editor-actions">
      <div className="policy-version-status">
        <strong>{selected ? 'Edit policy' : 'New policy version'}</strong>
        {selected && <span title={selected.id}>{shortID(selected.id)} · {dirty ? 'Unsaved changes' : 'Saved version'}</span>}
      </div>
      <label>
        Change reason
        <input aria-label="Reason" value={reason} disabled={disabled} onChange={event => onReasonChange(event.target.value)} />
      </label>
      <Button className="button button-primary" disabled={disabled || !reason.trim()} onClick={onSave}>
        Save immutable version
      </Button>
    </div>
  )
}

function SimulationResult({ simulation, onActivate, canActivate }: { simulation: Simulation; onActivate: () => void; canActivate: boolean }) {
  const blockers = simulation.decision.blockers ?? []
  return <section className="detail-section" aria-label="Policy simulation">
    <p><StatusBadge label={simulation.decision.outcome} tone={simulation.decision.outcome === 'allow' ? 'green' : 'red'} /> · hash <code>{simulation.hash}</code></p>
    <dl className="detail-list">
      <div><dt>Effective hash</dt><dd><code>{simulation.resolved.hash}</code></dd></div>
      <div><dt>Paused scopes</dt><dd>{simulation.resolved.paused || simulation.resolved.scope_paused ? 'Yes' : 'No'}</dd></div>
      <div><dt>Required actions</dt><dd>{(simulation.decision.required_actions ?? []).join('; ') || 'None'}</dd></div>
    </dl>
    {blockers.length > 0 && <p className="error-text">Blocked: {blockers.join('; ')}</p>}
    <Button disabled={!canActivate || (simulation.resolved.problems?.length ?? 0) > 0} onClick={onActivate}>Activate exact simulation</Button>
  </section>
}

function VersionHistory({ rows, selectedID, complete, loading, onMore, onSelect }: { rows: Version[]; selectedID: string; complete: boolean; loading: boolean; onMore: () => void; onSelect: (id: string) => void }) {
  return <>
    <DataTable caption="Version history">
      <table>
        <thead><tr><th>Version</th><th>Change</th></tr></thead>
        <tbody>{rows.map(row => <tr key={row.id} aria-current={row.id === selectedID ? 'true' : undefined}><td><button className="link-button" aria-pressed={row.id === selectedID} title={row.id} onClick={() => onSelect(row.id)}>{shortID(row.id)}</button><small>{new Date(row.created_at).toLocaleDateString()}</small></td><td>{row.reason || '—'}</td></tr>)}</tbody>
      </table>
      {!rows.length && <EmptyTable label="No policy versions recorded." />}
    </DataTable>
    {!complete && <Button disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Load more versions'}</Button>}
  </>
}

function AllowList({ name, value, disabled, onChange }: { name: string; value: string[] | null; disabled: boolean; onChange: (value: string[] | null) => void }) {
  const label = name.replaceAll('_', ' ')
  return <div><label>Allowed {label}<select value={value === null ? 'inherit' : 'explicit'} disabled={disabled} onChange={event => onChange(event.target.value === 'inherit' ? null : [])}><option value="inherit">Inherit</option><option value="explicit">Explicit list</option></select></label>{value !== null && <label>{label} values<input value={value.join(', ')} disabled={disabled} onChange={event => onChange(split(event.target.value))} /></label>}</div>
}
