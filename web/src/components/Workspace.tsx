import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react'
import { Icon } from './Icons'

export function Tabs({ id, label, items, value, onChange }: { id: string; label: string; items: Array<{ id: string; label: string }>; value: string; onChange: (id: string) => void }) {
  const refs = useRef<Array<HTMLButtonElement | null>>([])
  const move = (index: number) => { const next = (index + items.length) % items.length; onChange(items[next].id); refs.current[next]?.focus() }
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (event.key === 'ArrowRight') { event.preventDefault(); move(index + 1) }
    if (event.key === 'ArrowLeft') { event.preventDefault(); move(index - 1) }
    if (event.key === 'Home') { event.preventDefault(); move(0) }
    if (event.key === 'End') { event.preventDefault(); move(items.length - 1) }
  }
  return <div className="tabs" role="tablist" aria-label={label}>{items.map((item, index) => <button key={item.id} ref={element => { refs.current[index] = element }} id={`${id}-tab-${item.id}`} role="tab" aria-selected={item.id === value} aria-controls={`${id}-panel-${item.id}`} tabIndex={item.id === value ? 0 : -1} className={`tab ${item.id === value ? 'active' : ''}`} onClick={() => onChange(item.id)} onKeyDown={event => onKeyDown(event, index)}>{item.label}</button>)}</div>
}

export function Toolbar({ label, children }: { label: string; children: ReactNode }) {
  return <div className="toolbar" role="group" aria-label={label}>{children}</div>
}

export function SplitView({ listLabel, list, detail, selected, onBack, hideBack, closeControl }: { listLabel: string; list: ReactNode; detail: ReactNode; selected: boolean; onBack: () => void; hideBack?: boolean; closeControl?: { label: string; onClose: () => void } }) {
  const listRef = useRef<HTMLElement>(null)
  const detailRef = useRef<HTMLElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const originRef = useRef<HTMLElement | null>(null)
  const mountedRef = useRef(false)
  const [listFocusNonce, setListFocusNonce] = useState(0)

  useEffect(() => {
    const firstRender = !mountedRef.current
    mountedRef.current = true
    const active = document.activeElement
    const fromDetail = active instanceof HTMLElement && !!detailRef.current?.contains(active)
    const noMeaningfulFocus = !active || active === document.body || active === document.documentElement
    if (!selected) {
      if (!firstRender && (fromDetail || noMeaningfulFocus)) {
        if (originRef.current?.isConnected) originRef.current.focus()
        else listRef.current?.focus()
      }
      return
    }
    const fromList = active instanceof HTMLElement && !!listRef.current?.contains(active)
    if (fromList) originRef.current = active
    if (fromList || noMeaningfulFocus) {
      if (closeControl) closeRef.current?.focus()
      else detailRef.current?.focus()
    }
  }, [selected, Boolean(closeControl), listFocusNonce])

  const close = closeControl?.onClose ?? onBack
  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    if (event.key === 'Escape' && selected && closeControl && !event.defaultPrevented && !target.closest('dialog, [role="dialog"], input, textarea, select, [contenteditable="true"]')) { event.preventDefault(); close() }
  }
  const onListClick = (event: MouseEvent<HTMLElement>) => {
    if (!selected || !closeControl) return
    if (!(event.target as HTMLElement).closest('button.link-button')) return
    setListFocusNonce(value => value + 1)
  }

  return <div className={`split-view ${selected ? 'detail-open' : ''}`}>
    <section ref={listRef} className="split-list" aria-label={listLabel} tabIndex={-1} onClick={onListClick}>{list}</section>
    <section ref={detailRef} className="split-detail" aria-label="Detail" tabIndex={-1} hidden={!selected} onKeyDown={onKeyDown}>
      {selected && closeControl && <button ref={closeRef} className="icon-button panel-close" aria-label={closeControl.label} onClick={close}><Icon name="close" /></button>}
      {selected && !hideBack && <button className="back-link" onClick={onBack}><Icon name="chevron" size={14} />Back to list</button>}
      {detail}
    </section>
  </div>
}

export function DetailPanel({ title, status, actions, onClose, closeLabel, children }: { title: string; status?: ReactNode; actions?: ReactNode; onClose?: () => void; closeLabel?: string; children: ReactNode }) {
  return <section className="detail-panel" aria-label={title}>
    <header><div><h2>{title}</h2>{status}</div>{(actions || onClose) && <div className="detail-actions">{actions}{onClose && <button className="icon-button panel-close" aria-label={closeLabel ?? 'Close details'} onClick={onClose}><Icon name="close" /></button>}</div>}</header>
    {children}
  </section>
}

export function SplitPlaceholder({ label }: { label: string }) { return <div className="split-placeholder">{label}</div> }
