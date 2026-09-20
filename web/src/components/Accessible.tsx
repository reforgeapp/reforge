import { useEffect, useId, useRef } from 'react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'

export function Button({ className = 'button', ...props }: ButtonHTMLAttributes<HTMLButtonElement>) { return <button className={className} {...props} /> }

export function Dialog({ open, title, onClose, children }: { open: boolean; title: string; onClose: () => void; children: ReactNode }) {
	const titleID = useId()
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = ref.current
    if (!dialog) return
    if (open && !dialog.open) { dialog.showModal(); dialog.querySelector<HTMLElement>('button')?.focus() }
    if (!open && dialog.open) dialog.close()
  }, [open])
  return <dialog ref={ref} className="dialog" aria-labelledby={titleID} onClose={onClose}><div className="dialog-head"><h2 id={titleID}>{title}</h2><button className="icon-button" aria-label="Close dialog" onClick={onClose}>×</button></div><div className="dialog-body">{children}</div></dialog>
}
