import type { ReactNode } from 'react'
type Tone = 'green' | 'amber' | 'red' | 'teal' | 'neutral'
export function StatusBadge({ label, tone = 'neutral' }: { label: string; tone?: Tone }) { return <span className={`status-badge status-${tone}`}><span aria-hidden="true" className="status-dot" />{label}</span> }
export function GateList({ items }: { items: Array<{ label: string; detail: string; status: string; action?: ReactNode }> }) { return <section className="gate-list" aria-label="Gate status">{items.map(item => <div className="gate-row" key={item.label}><div><strong>{item.label}</strong><span>{item.detail}</span>{item.action}</div><StatusBadge label={item.status} tone={item.status === 'Passed' ? 'green' : item.status === 'Blocked' ? 'amber' : 'neutral'} /></div>)}</section> }
