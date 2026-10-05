import { useQuery } from '@tanstack/react-query'
import { DataTable, EmptyTable, useSort } from '../components/DataTable'
import { autopilotAPI } from '../policy-api'

const hours = (value?: number) => value === undefined ? '—' : value < 48 ? `${value.toFixed(1)}h` : `${(value / 24).toFixed(1)}d`
const rate = (part: number, whole: number) => whole === 0 ? '—' : `${Math.round((part / whole) * 100)}%`

export function OwnershipTable({ orgID }: { orgID: string }) {
  const query = useQuery({ queryKey: ['org', orgID, 'autopilot-metrics'], queryFn: ({ signal }) => autopilotAPI.metrics(orgID, signal), refetchInterval: 60_000 })
  const rows = query.data ?? []
  const sort = useSort(rows, { repository: row => row.repository, class: row => row.class, open: row => row.open, blocked: row => row.blocked, fixed: row => row.fixed, time: row => row.median_hours_to_fix ?? Infinity }, { key: 'open', dir: 'desc' })
  return <DataTable caption="Ownership by repository and class, last 90 days"><table><thead><tr>{sort.header('repository', 'Repository')}{sort.header('class', 'Class')}{sort.header('open', 'Open')}{sort.header('blocked', 'Blocked')}{sort.header('fixed', 'Fixed')}{sort.header('time', 'Median time to fix')}<th>Auto-merged</th><th>Regressed</th><th>Reverted</th></tr></thead><tbody>{sort.rows.map(row => <tr key={row.repository_id + row.class}><td>{row.repository}</td><td>{row.class.replaceAll('_', ' ')}</td><td>{row.open}</td><td>{row.blocked} <small className="table-meta">{rate(row.blocked, row.open)}</small></td><td>{row.fixed}</td><td>{hours(row.median_hours_to_fix)}</td><td>{rate(row.auto_merged, row.fixed)}</td><td>{rate(row.regressed, row.fixed)}</td><td>{rate(row.reverted, row.fixed)}</td></tr>)}</tbody></table>{!rows.length && <EmptyTable label={query.isLoading ? 'Loading ownership metrics…' : 'No findings in the last 90 days.'} />}</DataTable>
}
