import type { ReactNode } from 'react'
import { Icon } from './Icons'

export function Toolbar({ label, children }: { label: string; children: ReactNode }) {
  return <div className="toolbar" role="group" aria-label={label}>{children}</div>
}

export function SplitView({ listLabel, list, detail, selected, onBack }: { listLabel: string; list: ReactNode; detail: ReactNode; selected: boolean; onBack: () => void }) {
  return <div className={`split-view ${selected ? 'detail-open' : ''}`}>
    <section className="split-list" aria-label={listLabel}>{list}</section>
    <section className="split-detail" aria-label="Detail">
      {selected && <button className="back-link" onClick={onBack}><Icon name="chevron" size={14} />Back to list</button>}
      {detail}
    </section>
  </div>
}

export function DetailPanel({ title, status, actions, children }: { title: string; status?: ReactNode; actions?: ReactNode; children: ReactNode }) {
  return <section className="detail-panel" aria-label={title}>
    <header><div><h2>{title}</h2>{status}</div>{actions && <div className="detail-actions">{actions}</div>}</header>
    {children}
  </section>
}

export function SplitPlaceholder({ label }: { label: string }) { return <div className="split-placeholder">{label}</div> }
