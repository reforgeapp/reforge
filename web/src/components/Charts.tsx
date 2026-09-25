import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import '../styles/charts.css'

export type ChartSeries = { key: string; label: string; color: string }
export type ChartPoint = { label: string; detail: string; values: number[] }

const plain = (value: number) => value.toLocaleString()

function useWidth() {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const element = ref.current
    if (!element) return
    setWidth(element.clientWidth)
    const observer = new ResizeObserver(entries => setWidth(entries[0].contentRect.width))
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return [ref, width] as const
}

function scale(max: number, integer: boolean) {
  const raw = Math.max(max, integer ? 4 : Number.EPSILON) / 4
  const power = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 2.5, 5, 10].map(m => m * power).find(value => value >= raw && (!integer || Number.isInteger(value))) ?? power * 10
  return { max: step * 4, ticks: [0, 1, 2, 3, 4].map(i => i * step) }
}

const column = (x: number, y: number, w: number, h: number, r: number) => { const k = Math.min(r, h, w / 2); return `M${x},${y + h}V${y + k}Q${x},${y} ${x + k},${y}H${x + w - k}Q${x + w},${y} ${x + w},${y + k}V${y + h}Z` }

export function TrendChart({ label, points, series, format = plain, axisFormat = format, integer = false, stacked = false, height = 220, empty }: { label: string; empty: string; points: ChartPoint[]; series: ChartSeries[]; format?: (value: number) => string; axisFormat?: (value: number) => string; integer?: boolean; stacked?: boolean; height?: number }) {
  const [ref, width] = useWidth()
  const [active, setActive] = useState<number | null>(null)
  const pad = { top: 10, right: 12, bottom: 26, left: 52 }
  const plotW = Math.max(0, width - pad.left - pad.right)
  const plotH = height - pad.top - pad.bottom
  const { max, ticks } = scale(Math.max(0, ...points.map(point => stacked ? point.values.reduce((sum, value) => sum + value, 0) : Math.max(...point.values))), integer)
  const band = plotW / Math.max(1, points.length)
  const barW = Math.max(2, Math.min(24, band * 0.6))
  const x = (index: number) => stacked ? pad.left + band * (index + 0.5) : pad.left + (points.length <= 1 ? plotW / 2 : index * plotW / (points.length - 1))
  const y = (value: number) => pad.top + plotH - value / max * plotH
  const every = Math.max(1, Math.ceil(points.length / Math.max(2, Math.floor(plotW / 84))))
  const path = (s: number) => points.map((point, index) => `${index ? 'L' : 'M'}${x(index).toFixed(1)},${y(point.values[s]).toFixed(1)}`).join('')
  const pick = (event: PointerEvent<SVGSVGElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    const ratio = (event.clientX - box.left - pad.left) / Math.max(1, plotW)
    setActive(Math.min(points.length - 1, Math.max(0, stacked ? Math.floor(ratio * points.length) : Math.round(ratio * (points.length - 1)))))
  }
  const onKeyDown = (event: KeyboardEvent<SVGSVGElement>) => {
    const last = points.length - 1
    if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
      event.preventDefault()
      setActive(current => Math.min(last, Math.max(0, (current ?? (event.key === 'ArrowRight' ? -1 : last + 1)) + (event.key === 'ArrowRight' ? 1 : -1))))
    }
    if (event.key === 'Home') { event.preventDefault(); setActive(0) }
    if (event.key === 'End') { event.preventDefault(); setActive(last) }
    if (event.key === 'Escape') setActive(null)
  }
  if (!points.some(point => point.values.some(value => value > 0))) return <p className="chart-empty">{empty}</p>
  const tip = active === null ? undefined : points[active]
  const tipLeft = active === null ? 0 : x(active)
  return <figure className="chart">
    <div ref={ref} className="chart-plot" style={{ height }}>
      {width > 0 && <svg width={width} height={height} role="img" aria-label={label} tabIndex={0} onPointerMove={pick} onPointerLeave={() => setActive(null)} onKeyDown={onKeyDown} onBlur={() => setActive(null)}>
        {ticks.map(tick => <g key={tick}>
          <line className={tick ? 'chart-grid' : 'chart-baseline'} x1={pad.left} x2={width - pad.right} y1={y(tick)} y2={y(tick)} />
          <text className="chart-tick" x={pad.left - 8} y={y(tick)} dy="0.32em" textAnchor="end">{axisFormat(tick)}</text>
        </g>)}
        {points.map((point, index) => (points.length - 1 - index) % every === 0 ? <text key={point.label + index} className="chart-tick" x={x(index)} y={height - 6} textAnchor={stacked ? 'middle' : index === 0 ? 'start' : index === points.length - 1 ? 'end' : 'middle'}>{point.label}</text> : null)}
        {stacked && active !== null && <rect className="chart-band" x={pad.left + band * active} y={pad.top} width={band} height={plotH} />}
        {stacked && points.map((point, index) => { let base = 0; const top = point.values.reduce((last, value, s) => value > 0 ? s : last, -1); return <g key={point.label + index}>{point.values.map((value, s) => { if (value <= 0) return null; const y0 = y(base); base += value; const y1 = y(base) + (s === top ? 0 : 1); const h = Math.max(1, y0 - y1 - (base - value > 0 ? 1 : 0)); return <path key={series[s].key} d={s === top ? column(x(index) - barW / 2, y1, barW, h, 4) : `M${x(index) - barW / 2},${y1}h${barW}v${h}h${-barW}Z`} fill={series[s].color} /> })}</g> })}
        {!stacked && series.length === 1 && points.length > 1 && <path d={`${path(0)}L${x(points.length - 1)},${y(0)}L${x(0)},${y(0)}Z`} fill={series[0].color} opacity={0.1} />}
        {!stacked && [...series].reverse().map(item => ({ item, s: series.indexOf(item) })).map(({ item, s }) => <path key={item.key} d={path(s)} fill="none" stroke={item.color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />)}
        {!stacked && active !== null && <g>
          <line className="chart-cursor" x1={tipLeft} x2={tipLeft} y1={pad.top} y2={pad.top + plotH} />
          {series.map((item, s) => <circle key={item.key} cx={tipLeft} cy={y(points[active].values[s])} r={4} fill={item.color} className="chart-dot" />)}
        </g>}
      </svg>}
      {tip && <div className={`chart-tooltip ${tipLeft > width / 2 ? 'chart-tooltip-left' : ''}`} style={{ left: tipLeft }} aria-hidden="true">
        <strong>{tip.detail}</strong>
        {series.map((item, s) => <span key={item.key}><i style={{ background: item.color }} />{item.label}<b>{format(tip.values[s])}</b></span>)}
        {stacked && series.length > 1 && <span className="chart-tooltip-total">Total<b>{format(tip.values.reduce((sum, value) => sum + value, 0))}</b></span>}
      </div>}
    </div>
    {series.length > 1 && <ul className="chart-legend">{series.map((item, s) => <li key={item.key}><i style={{ background: item.color }} />{item.label}<b>{format(points.reduce((sum, point) => sum + point.values[s], 0))}</b></li>)}</ul>}
    <div className="sr-only"><table><caption>{label}</caption><thead><tr><th scope="col">Day</th>{series.map(item => <th key={item.key} scope="col">{item.label}</th>)}</tr></thead><tbody>{points.map((point, index) => <tr key={point.label + index}><th scope="row">{point.detail}</th>{series.map((item, s) => <td key={item.key}>{format(point.values[s])}</td>)}</tr>)}</tbody></table></div>
  </figure>
}

export function BarList({ label, rows, format = plain, empty }: { label: string; rows: Array<{ key: string; label: string; value: number; color?: string }>; format?: (value: number) => string; empty: string }) {
  const max = Math.max(0, ...rows.map(row => row.value))
  if (!rows.length) return <p className="chart-empty">{empty}</p>
  return <ul className="bar-list" aria-label={label}>{rows.map(row => <li key={row.key}>
    <span className="bar-list-label">{row.label}</span>
    <span className="bar-list-value">{format(row.value)}</span>
    <span className="bar-list-track" aria-hidden="true"><span style={{ width: `${max ? Math.max(row.value ? 2 : 0, row.value / max * 100) : 0}%`, background: row.color ?? 'var(--chart-1)' }} /></span>
  </li>)}</ul>
}

export function StackedBar({ label, segments, format = plain }: { label: string; segments: Array<{ key: string; label: string; value: number; color: string }>; format?: (value: number) => string }) {
  const total = segments.reduce((sum, item) => sum + item.value, 0)
  return <div className="stacked">
    <div className="stacked-bar" role="img" aria-label={`${label}: ${segments.map(item => `${item.label} ${format(item.value)}`).join(', ')}`}>
      {total ? segments.filter(item => item.value).map(item => <span key={item.key} title={`${item.label}: ${format(item.value)}`} style={{ flexGrow: item.value, background: item.color }} />) : <span className="stacked-empty" />}
    </div>
    <ul className="chart-legend">{segments.map(item => <li key={item.key}><i style={{ background: item.color }} />{item.label}<b>{format(item.value)}</b></li>)}</ul>
  </div>
}

export function Meter({ label, used, held, cap, format = plain }: { label: string; used: number; held: number; cap: number; format?: (value: number) => string }) {
  const pct = (value: number) => cap > 0 ? Math.min(100, value / cap * 100) : 0
  const over = used + held > cap
  return <div className={`meter ${over ? 'meter-over' : ''}`}>
    <div className="meter-head"><span>{label}</span><span>{format(used)}{held ? ` + ${format(held)} held` : ''} / {format(cap)}</span></div>
    <div className="meter-track" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={cap} aria-valuenow={Math.min(cap, used + held)} aria-valuetext={`${format(used + held)} of ${format(cap)}`}>
      <span className="meter-used" style={{ width: `${pct(used)}%` }} />
      <span className="meter-held" style={{ width: `${Math.max(0, pct(used + held) - pct(used))}%` }} />
    </div>
  </div>
}

export function Donut({ label, segments, empty, format = plain }: { label: string; segments: Array<{ key: string; label: string; value: number; color: string }>; empty: string; format?: (value: number) => string }) {
  const [active, setActive] = useState<string>()
  const total = segments.reduce((sum, item) => sum + item.value, 0)
  if (!total) return <p className="chart-empty">{empty}</p>
  const radius = 58
  const circumference = 2 * Math.PI * radius
  const gap = segments.filter(item => item.value).length > 1 ? 2 : 0
  let offset = 0
  const percent = (value: number) => `${Math.round(value / total * 100)}%`
  const focus = segments.find(item => item.key === active)
  return <figure className="donut">
    <svg viewBox="0 0 160 160" role="img" aria-label={`${label}: ${segments.map(item => `${item.label} ${format(item.value)}`).join(', ')}`}>
      <circle cx="80" cy="80" r={radius} fill="none" stroke="var(--surface-3)" strokeWidth="22" />
      {segments.filter(item => item.value).map(item => {
        const length = item.value / total * circumference
        const arc = <circle key={item.key} cx="80" cy="80" r={radius} fill="none" stroke={item.color} strokeWidth={active === item.key ? 26 : 22} strokeDasharray={`${Math.max(0.5, length - gap)} ${circumference}`} strokeDashoffset={-offset} transform="rotate(-90 80 80)" onPointerEnter={() => setActive(item.key)} onPointerLeave={() => setActive(undefined)}><title>{`${item.label}: ${format(item.value)} (${percent(item.value)})`}</title></circle>
        offset += length
        return arc
      })}
      <text x="80" y="76" textAnchor="middle" className="donut-total">{format(focus?.value ?? total)}</text>
      <text x="80" y="96" textAnchor="middle" className="donut-caption">{focus?.label ?? 'total'}</text>
    </svg>
    <ul className="chart-legend donut-legend">{segments.map(item => <li key={item.key} onPointerEnter={() => setActive(item.key)} onPointerLeave={() => setActive(undefined)}><i style={{ background: item.color }} />{item.label}<b>{format(item.value)}</b><span>{percent(item.value)}</span></li>)}</ul>
    <div className="sr-only"><table><caption>{label}</caption><thead><tr><th scope="col">Severity</th><th scope="col">Count</th></tr></thead><tbody>{segments.map(item => <tr key={item.key}><th scope="row">{item.label}</th><td>{format(item.value)}</td></tr>)}</tbody></table></div>
  </figure>
}
