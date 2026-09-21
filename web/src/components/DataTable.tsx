import type { ReactNode } from 'react'
export function DataTable({ caption, children }: { caption: string; children: ReactNode }) { return <section className="table-card" aria-label={caption}><div className="table-head"><h2>{caption}</h2></div><div className="table-wrap" tabIndex={0} role="group" aria-label={caption}>{children}</div></section> }
export function EmptyTable({ label }: { label: string }) { return <div className="empty-table"><span aria-hidden="true">▦</span><p>{label}</p></div> }
