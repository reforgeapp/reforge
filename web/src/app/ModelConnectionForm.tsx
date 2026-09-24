import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, ReforgeAPIError, type ConnectionCreate } from '../api/client'
import { modelConnectionsAPI, type ModelCatalogInput, type ModelCatalogItem } from '../api/model-connections'
import { compatibleProfiles, modelProviderOption, modelProviderOptions } from '../model-providers'
import { Button } from '../components/Accessible'

type ModelConnectionFormProps = {
  orgID: string
  csrf: string
  onClose: () => void
  onCreated: () => void
  onBusyChange?: (busy: boolean) => void
}

type Phase = 'input' | 'catalogued' | 'created' | 'uncertain'

const message = (value: unknown) => value instanceof Error ? value.message : 'The server returned an unknown error.'
const isNetworkError = (value: unknown) => !(value instanceof ReforgeAPIError)
const base = modelProviderOptions[0]
const maxName = 160

export function ModelConnectionForm({ orgID, csrf, onClose, onCreated, onBusyChange }: ModelConnectionFormProps) {
  const [providerID, setProviderID] = useState(base.id)
  const [endpoint, setEndpoint] = useState(base.endpoint)
  const [profile, setProfile] = useState(base.profile)
  const [secret, setSecret] = useState('')
  const [caPEM, setCAPEM] = useState('')
  const [name, setName] = useState('')
  const [items, setItems] = useState<ModelCatalogItem[]>([])
  const [model, setModel] = useState('')
  const [manualModel, setManualModel] = useState('')
  const [phase, setPhase] = useState<Phase>('input')
  const [catalogBusy, setCatalogBusy] = useState(false)
  const [catalogError, setCatalogError] = useState('')
  const [saveBusy, setSaveBusy] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [verifyError, setVerifyError] = useState('')
  const [created, setCreated] = useState<{ id: string; version: number } | null>(null)
  const [privateRoute, setPrivateRoute] = useState(false)
  const [poolID, setPoolID] = useState('')
  const [runnerID, setRunnerID] = useState('')
  const [routeHost, setRouteHost] = useState('')
  const [cidrs, setCIDRs] = useState('')
  const seq = useRef(0)
  const controller = useRef<AbortController | null>(null)
  const orgRef = useRef(orgID)
  const option = modelProviderOption(providerID)

  const pools = useInfiniteQuery({
    queryKey: ['org', orgID, 'model-form-runner-pools'],
    queryFn: ({ pageParam, signal }) => api.getRunnerPools(orgID, { state: 'active', cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const runners = useInfiniteQuery({
    queryKey: ['org', orgID, 'model-form-runners', poolID],
    queryFn: ({ pageParam, signal }) => api.getRunners(orgID, poolID, { cursor: pageParam, limit: 100, signal }),
    initialPageParam: undefined as string | undefined,
    enabled: privateRoute && !!poolID,
    getNextPageParam: page => page.complete ? undefined : page.next_cursor,
  })
  const poolItems = pools.data?.pages.flatMap(page => page.items) ?? []
  const runnerOptions = (runners.data?.pages.flatMap(page => page.items) ?? [])
    .filter(runner => runner.state === 'active' && new Date(runner.credential_expires_at).getTime() > Date.now())
    .map(runner => ({ ...runner, pool_name: poolItems.find(pool => pool.id === poolID)?.name ?? runner.pool_name }))

  const resetCatalog = () => {
    seq.current++
    controller.current?.abort()
    controller.current = null
    setItems([])
    setModel('')
    setCatalogError('')
    setCatalogBusy(false)
    setPhase('input')
    setCreated(null)
    setVerifyError('')
  }

  const resetAll = () => {
    resetCatalog()
    setProviderID(base.id)
    setEndpoint(base.endpoint)
    setProfile(base.profile)
    setSecret('')
    setCAPEM('')
    setName('')
    setManualModel('')
    setSaveBusy(false)
    setSaveError('')
    setPrivateRoute(false)
    setPoolID('')
    setRunnerID('')
    setRouteHost('')
    setCIDRs('')
  }

  useEffect(() => () => { seq.current++; controller.current?.abort() }, [])

  useEffect(() => {
    if (orgRef.current === orgID) return
    orgRef.current = orgID
    resetAll()
  }, [orgID])

  useEffect(() => { onBusyChange?.(saveBusy) }, [saveBusy, onBusyChange])
  useEffect(() => () => { onBusyChange?.(false) }, [onBusyChange])

  const changeProvider = (value: string) => {
    const next = modelProviderOption(value)
    setProviderID(value)
    setEndpoint(next.endpoint)
    setProfile(next.profile)
    setSecret('')
    setManualModel('')
    setCAPEM('')
    setPrivateRoute(false)
    setPoolID('')
    setRunnerID('')
    setRouteHost('')
    setCIDRs('')
    resetCatalog()
  }

  const changeEndpoint = (value: string) => {
    setEndpoint(value)
    resetCatalog()
  }

  const catalogInput = (): ModelCatalogInput => ({
    provider: option.provider,
    endpoint: endpoint.trim(),
    ...(secret ? { secret } : {}),
    settings: {
      profile: option.provider === 'compatible' ? profile : option.profile,
      auth_kind: 'api_key',
      billing_route: 'direct_api',
      ...(caPEM.trim() ? { ca_pem: caPEM } : {}),
    },
  })

  const runCatalog = async () => {
    if (!csrf || catalogBusy || saveBusy || privateRoute) return
    seq.current++
    const current = seq.current
    controller.current?.abort()
    const next = new AbortController()
    controller.current = next
    setCatalogBusy(true)
    setCatalogError('')
    setItems([])
    setModel('')
    setPhase('input')
    try {
      const result = await modelConnectionsAPI.catalog(orgID, catalogInput(), csrf, next.signal)
      if (current !== seq.current) return
      setItems(result.items)
      setPhase('catalogued')
      setModel(result.items.find(item => !item.disabled)?.id ?? '')
      if (!result.items.length) setCatalogError('No models returned. Enter a model ID under Advanced.')
    } catch (reason) {
      if (current !== seq.current || next.signal.aborted) return
      setCatalogError(message(reason))
    } finally {
      if (current === seq.current) setCatalogBusy(false)
    }
  }

  const retryVerification = async () => {
    if (!created || !csrf) return
    const myGen = seq.current
    const myOrg = orgID
    setSaveBusy(true)
    setVerifyError('')
    try {
      const latest = await api.getConnection(orgID, created.id)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      await api.testConnection(orgID, latest.id, latest.version, csrf)
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      onCreated()
    } catch (reason) {
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setVerifyError(message(reason))
    } finally {
      if (myGen === seq.current && orgRef.current === myOrg) setSaveBusy(false)
    }
  }

  const effectiveModel = manualModel.trim() || model
  const usingManual = !!manualModel.trim()
  const manualDisabled = usingManual && !!items.find(item => item.id === manualModel.trim())?.disabled
  const selectionBlocked = !usingManual && !!items.find(item => item.id === model)?.disabled
  const manualUnknown = usingManual && items.length > 0 && !items.some(item => item.id === manualModel.trim())
  const hasModel = !!effectiveModel && !selectionBlocked && !manualDisabled
  const routeComplete = !!runnerID && !!routeHost.trim() && !!cidrs.trim()
  const catalogSatisfied = privateRoute || phase === 'catalogued' || usingManual
  const readyToSave = !!csrf && !!endpoint.trim() && (!option.secretRequired || !!secret) && hasModel && catalogSatisfied && (!privateRoute || routeComplete)
  const canSave = readyToSave && !saveBusy && !catalogBusy
  const disabledModels = items.filter(item => item.disabled).length
  const testDisabled = catalogBusy || saveBusy || !csrf || privateRoute || !endpoint.trim() || (option.secretRequired && !secret)

  const defaultName = () => (effectiveModel ? `${option.label}/${effectiveModel}` : option.label).slice(0, maxName)

  const doSave = async () => {
    if (!readyToSave || saveBusy || catalogBusy || phase === 'uncertain') return
    const myGen = seq.current
    const myOrg = orgID
    setSaveBusy(true)
    setSaveError('')
    const payload: ConnectionCreate = {
      kind: 'model',
      provider: option.provider,
      name: name.trim() || defaultName(),
      endpoint: endpoint.trim(),
      settings: {
        auth_kind: 'api_key',
        billing_route: 'direct_api',
        profile: option.provider === 'compatible' ? profile : option.profile,
        ...(effectiveModel ? { model: effectiveModel } : {}),
        ...(caPEM.trim() ? { ca_pem: caPEM } : {}),
      },
      ...(secret ? { secret } : {}),
      ...(privateRoute ? { private_route: { runner_id: runnerID, host: routeHost.trim(), cidrs: cidrs.split(',').map(item => item.trim()).filter(Boolean) } } : {}),
    }
    try {
      let connection: Awaited<ReturnType<typeof api.createConnection>>
      try {
        connection = await api.createConnection(orgID, 'model', payload, csrf)
      } catch (reason) {
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        if (isNetworkError(reason)) {
          setPhase('uncertain')
        } else {
          setSaveError(message(reason))
        }
        return
      }
      if (myGen !== seq.current || orgRef.current !== myOrg) return
      setCreated({ id: connection.id, version: connection.version })
      setSecret('')
      try {
        await api.testConnection(orgID, connection.id, connection.version, csrf)
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        onCreated()
      } catch (reason) {
        if (myGen !== seq.current || orgRef.current !== myOrg) return
        setVerifyError(message(reason))
        setPhase('created')
      }
    } finally {
      if (myGen === seq.current && orgRef.current === myOrg) setSaveBusy(false)
    }
  }

  const save = (event: FormEvent) => {
    event.preventDefault()
    void doSave()
  }

  if (phase === 'uncertain') {
    return <div className="form-stack model-connection-created">
      <p role="status">Save outcome unknown. Check Connections before trying again.</p>
      <div className="dialog-actions">
        <Button type="button" className="button button-primary" onClick={onCreated}>View connections</Button>
      </div>
    </div>
  }

  if (phase === 'created') {
    return <div className="form-stack model-connection-created">
      <p role="status">Connection saved; verification unavailable.</p>
      {verifyError && <p className="error-text" role="alert">{verifyError}</p>}
      <div className="dialog-actions">
        <Button type="button" onClick={onCreated}>Close</Button>
        <Button type="button" className="button button-primary" disabled={saveBusy || !csrf} onClick={() => void retryVerification()}>{saveBusy ? 'Verifying…' : 'Retry verification'}</Button>
      </div>
    </div>
  }

  const saveTitle = !csrf
    ? 'Sign in again to save a connection.'
    : !endpoint.trim()
      ? 'Enter an endpoint.'
      : option.secretRequired && !secret
        ? 'Enter an API key.'
        : !hasModel
          ? manualDisabled ? 'That model is unavailable for this provider.' : 'Test the connection and choose a model, or enter a model ID under Advanced.'
          : privateRoute && !routeComplete
            ? 'Complete private route fields before saving.'
            : undefined

  return <form className="form-stack model-connection-form" onSubmit={save}>
    <div className="form-grid">
      <label>Provider<select value={providerID} disabled={saveBusy} onChange={event => changeProvider(event.target.value)}>
        {modelProviderOptions.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
      </select></label>
      <label>API key<input type="password" autoComplete="new-password" disabled={saveBusy} required={option.secretRequired} value={secret} onChange={event => { setSecret(event.target.value); resetCatalog() }} /></label>
      {option.endpointEditable && <label className="wide">Endpoint<input value={endpoint} disabled={saveBusy} onChange={event => changeEndpoint(event.target.value)} placeholder="https://…" /></label>}
    </div>
    {privateRoute
      ? <p className="table-meta" role="status">Enter a model ID under Advanced for private routing.</p>
      : <div className="row-actions">
        <Button type="button" disabled={testDisabled} title={!endpoint.trim() ? 'Set the endpoint first.' : undefined} onClick={() => void runCatalog()}>{catalogBusy ? 'Testing…' : 'Test connection'}</Button>
        {phase === 'catalogued' && <span className="table-meta" role="status">Models loaded{disabledModels ? ` · ${disabledModels} unavailable` : ''}.</span>}
      </div>}
    {catalogError && <p className="error-text" role="alert">{catalogError} {!privateRoute && <Button type="button" disabled={catalogBusy || saveBusy || !csrf} onClick={() => void runCatalog()}>Retry</Button>}</p>}
    {phase === 'catalogued' && items.length > 0 && <label>Model<select value={model} disabled={saveBusy} onChange={event => setModel(event.target.value)}>
      <option value="">Choose model</option>
      {items.map(item => <option key={item.id} value={item.id} disabled={item.disabled}>{item.disabled ? `${item.name} — unavailable${item.reason ? ` (${item.reason})` : ''}` : item.name}</option>)}
    </select></label>}
    <details className="model-advanced">
      <summary>Advanced</summary>
      <div className="form-grid">
        <label>Name<input value={name} maxLength={maxName} disabled={saveBusy} onChange={event => setName(event.target.value)} placeholder={defaultName()} /></label>
        {!option.endpointEditable && <label>Endpoint<input value={endpoint} disabled={saveBusy} onChange={event => changeEndpoint(event.target.value)} placeholder="https://…" /></label>}
        {providerID === 'compatible' && <label>Profile<select value={profile} disabled={saveBusy} onChange={event => { setProfile(event.target.value); resetCatalog() }}>{compatibleProfiles.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>}
        <label className="wide">CA certificate<textarea rows={3} maxLength={65536} value={caPEM} disabled={saveBusy} onChange={event => { setCAPEM(event.target.value); resetCatalog() }} placeholder="Optional PEM CA certificate" /></label>
        <label className="wide">Model ID<input value={manualModel} disabled={saveBusy} onChange={event => setManualModel(event.target.value)} placeholder={privateRoute ? 'Required for a private route' : 'Optional when the catalogue is unavailable'} /></label>
        {manualDisabled && <p className="error-text wide" role="alert">That model is unavailable for this provider.</p>}
        {manualUnknown && <p className="table-meta wide" role="status">Not in the provider list; the server will validate it.</p>}
        <label className="checkbox-label wide"><input type="checkbox" checked={privateRoute} disabled={saveBusy} onChange={event => { setPrivateRoute(event.target.checked); resetCatalog() }} /> Private route via enrolled runner</label>
        {privateRoute && <>
          <label>Runner pool<select aria-label="Runner pool" value={poolID} disabled={saveBusy} onChange={event => { setPoolID(event.target.value); setRunnerID('') }}>
            <option value="">Choose pool</option>
            {poolItems.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
          </select></label>
          <label>Runner<select aria-label="Runner" value={runnerID} disabled={saveBusy} onChange={event => setRunnerID(event.target.value)}>
            <option value="">Choose active runner</option>
            {runnerOptions.map(runner => <option key={runner.id} value={runner.id}>{runner.name} · {runner.pool_name}</option>)}
          </select></label>
          <label>Route host<input value={routeHost} disabled={saveBusy} onChange={event => setRouteHost(event.target.value)} /></label>
          <label className="wide">Approved CIDRs<input value={cidrs} disabled={saveBusy} onChange={event => setCIDRs(event.target.value)} placeholder="10.0.0.0/8" /></label>
          {!!pools.error && <p className="error-text wide" role="alert">Runner pools unavailable: {message(pools.error)}</p>}
          {!!runners.error && <p className="error-text wide" role="alert">Runners unavailable: {message(runners.error)}</p>}
          {pools.hasNextPage && <Button type="button" disabled={pools.isFetchingNextPage || saveBusy} onClick={() => void pools.fetchNextPage()}>{pools.isFetchingNextPage ? 'Loading pools…' : 'Load more pools'}</Button>}
          {runners.hasNextPage && <Button type="button" disabled={runners.isFetchingNextPage || saveBusy} onClick={() => void runners.fetchNextPage()}>{runners.isFetchingNextPage ? 'Loading runners…' : 'Load more runners'}</Button>}
          {!runnerOptions.length && poolID && !runners.isLoading && <p className="table-meta wide">No active enrolled runners available in selected pool.</p>}
        </>}
      </div>
    </details>
    {saveError && <p className="error-text" role="alert">{saveError}</p>}
    <div className="dialog-actions">
      <Button type="button" disabled={saveBusy} onClick={onClose}>Cancel</Button>
      <Button
        type="submit"
        className="button button-primary"
        disabled={!canSave}
        title={saveTitle}
      >{saveBusy ? 'Saving…' : 'Save connection'}</Button>
    </div>
  </form>
}
