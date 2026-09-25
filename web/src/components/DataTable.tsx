import { useState, type ReactNode } from 'react'

export function DataTable({ caption, actions, showHeading = false, children }: { caption: string; actions?: ReactNode; showHeading?: boolean; children: ReactNode }) { return <section className="table-card" aria-label={caption}>{(showHeading || actions) && <div className={`table-head ${showHeading ? '' : 'table-head-actions-only'}`}>{showHeading && <h2>{caption}</h2>}{actions && <div className="row-actions">{actions}</div>}</div>}<div className="table-wrap" tabIndex={0} role="group" aria-label={caption}>{children}</div></section> }
export function EmptyTable({ label }: { label: string }) { return <div className="empty-table"><span aria-hidden="true">▦</span><p>{label}</p></div> }

type SortValue = string | number | boolean | null | undefined
type Sort = { key: string; dir: 'asc' | 'desc' }
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function useSort<T>(rows: T[], columns: Record<string, (row: T) => SortValue>, initial: Sort) {
  const [sort, setSort] = useState(initial)
  const value = columns[sort.key]
  const sorted = value ? [...rows].sort((a, b) => {
    const x = value(a), y = value(b)
    if (x == null || x === '') return y == null || y === '' ? 0 : 1
    if (y == null || y === '') return -1
    const order = typeof x === 'string' && typeof y === 'string' ? collator.compare(x, y) : Number(x) - Number(y)
    return sort.dir === 'asc' ? order : -order
  }) : rows
  const toggle = (key: string) => setSort(current => {
    if (current.key === key) return { key, dir: current.dir === 'asc' ? 'desc' : 'asc' }
    const sample = rows.map(row => columns[key]?.(row)).find(item => item != null && item !== '')
    return { key, dir: typeof sample === 'string' ? 'asc' : 'desc' }
  })
  const header = (key: string, label: ReactNode) => <th aria-sort={sort.key === key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}><button type="button" className={`sort-button${sort.key === key ? ' sorted' : ''}`} onClick={() => toggle(key)}>{label}<span className="sort-indicator" aria-hidden="true">{sort.key === key ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span></button></th>
  return { rows: sorted, header }
}
