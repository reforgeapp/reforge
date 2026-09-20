import type { ReactNode } from 'react'

const labels = { loading: 'Loading', error: 'Error', empty: 'Empty', stale: 'Stale', blocked: 'Blocked' }
export function StatePanel({ kind, title, detail, action }: { kind: keyof typeof labels; title: string; detail: string; action?: ReactNode }) { return <section className={`state-panel state-${kind}`} role={kind === 'error' ? 'alert' : kind === 'loading' ? 'status' : undefined}><span className="state-icon" aria-hidden="true">{kind === 'loading' ? '◌' : kind === 'error' ? '!' : kind === 'blocked' ? 'Ⅱ' : kind === 'stale' ? '◷' : '◇'}</span><div><p className="eyebrow">{labels[kind]}</p><h2>{title}</h2><p>{detail}</p>{action && <div className="state-action">{action}</div>}</div></section> }
