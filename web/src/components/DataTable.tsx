import type { ReactNode } from 'react'

export function DataTable({ caption, actions, showHeading = false, children }: { caption: string; actions?: ReactNode; showHeading?: boolean; children: ReactNode }) { return <section className="table-card" aria-label={caption}>{(showHeading || actions) && <div className={`table-head ${showHeading ? '' : 'table-head-actions-only'}`}>{showHeading && <h2>{caption}</h2>}{actions && <div className="row-actions">{actions}</div>}</div>}<div className="table-wrap" tabIndex={0} role="group" aria-label={caption}>{children}</div></section> }
export function EmptyTable({ label }: { label: string }) { return <div className="empty-table"><span aria-hidden="true">▦</span><p>{label}</p></div> }
