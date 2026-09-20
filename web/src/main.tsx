import React, { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import type { components } from './api/schema'

function App() {
  const [meta, setMeta] = useState<components['schemas']['Meta']>()
  const [error, setError] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    fetch('/api/v1/meta', { signal: controller.signal })
      .then(response => { if (!response.ok) throw new Error('Unavailable'); return response.json() })
      .then(setMeta)
      .catch(() => { if (!controller.signal.aborted) setError(true) })
    return () => controller.abort()
  }, [])
  return <main>
    <h1>Reforge</h1>
    {meta?.fixture_auth && <p role="status">Development · fixture authentication</p>}
    {error ? <p role="alert">Cannot reach Reforge. Reload to retry.</p> : !meta ? <p role="status">Connecting…</p> : <p>Repository maintenance</p>}
  </main>
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode><App /></React.StrictMode>,
)
