import type { SVGProps } from 'react'

const paths: Record<string, string[]> = {
  overview: ['M3 11.5 12 4l9 7.5', 'M5.5 10v9.5h13V10', 'M10 19.5V14h4v5.5'],
  repositories: ['M4 7.5 12 4l8 3.5v9L12 20l-8-3.5z', 'M4 7.5 12 11l8-3.5', 'M12 11v9'],
  findings: ['M12 4 3 19h18z', 'M12 10v4', 'M12 16.6v.2'],
  runs: ['M4 12a8 8 0 1 0 2.4-5.7', 'M4 4.5V9h4.5'],
  changes: ['M7 4v12', 'M7 16a3 3 0 1 0 2.2 1', 'M17 20V8a3 3 0 0 0-3-3h-2', 'M14 3l-2 2 2 2'],
  deployments: ['M12 3 5 16h14z', 'M12 12v.01', 'M9 20h6'],
  campaigns: ['M12 4a8 8 0 1 0 0 16 8 8 0 0 0 0-16z', 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z', 'M12 11.5a.5.5 0 1 0 0 1 .5.5 0 0 0 0-1z'],
  policies: ['M12 3 5 6v5c0 4.2 2.9 7.6 7 9 4.1-1.4 7-4.8 7-9V6z', 'M9 12l2 2 4-4'],
  connections: ['M9.5 14.5 6 18a3 3 0 0 1-4-4l3.5-3.5', 'M14.5 9.5 18 6a3 3 0 0 1 4 4l-3.5 3.5', 'M9 15l6-6'],
  runners: ['M4 5h16v5H4z', 'M4 14h16v5H4z', 'M7 7.5h.01', 'M7 16.5h.01'],
  usage: ['M5 19V9', 'M10 19V5', 'M15 19v-7', 'M20 19v-4'],
  audit: ['M5 4h14v16H5z', 'M8 8h8', 'M8 12h8', 'M8 16h5'],
  organisation: ['M12 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z', 'M5 20c0-3.3 3.1-5.5 7-5.5s7 2.2 7 5.5'],
  search: ['M10.5 17a6.5 6.5 0 1 0 0-13 6.5 6.5 0 0 0 0 13z', 'M20 20l-4.5-4.5'],
  menu: ['M4 7h16', 'M4 12h16', 'M4 17h16'],
  close: ['M6 6l12 12', 'M18 6 6 18'],
  chevron: ['M7 10l5 5 5-5'],
  signout: ['M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4', 'M10 12h10', 'M12 8l-4 4 4 4'],
  plus: ['M12 5v14', 'M5 12h14'],
  refresh: ['M4 12a8 8 0 1 0 2.4-5.7', 'M4 4.5V9h4.5'],
  warning: ['M12 4 3 19h18z', 'M12 10v4', 'M12 16.6v.2'],
  external: ['M14 4h6v6', 'M20 4l-9 9', 'M18 14v5H5V6h5'],
  play: ['M8 5.5v13l10.5-6.5z'],
  observe: ['M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12z', 'M12 14.8a2.8 2.8 0 1 0 0-5.6 2.8 2.8 0 0 0 0 5.6z'],
  propose: ['M9.5 18h5', 'M10.5 21h3', 'M12 3a6 6 0 0 0-3.6 10.8c.7.5 1.1 1.3 1.1 2.2h5c0-.9.4-1.7 1.1-2.2A6 6 0 0 0 12 3z'],
  merge: ['M6 3.5v11', 'M6 20.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z', 'M18 8.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z', 'M18 8.5a9 9 0 0 1-9 9'],
  deliver: ['M21 3 10.5 13.5', 'M21 3l-6.5 18-4-7.5L3 9.5z'],
  auto: ['M11 3l1.8 5.2L18 10l-5.2 1.8L11 17l-1.8-5.2L4 10l5.2-1.8z', 'M18.5 14.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z'],
}

export type IconName = keyof typeof paths

export function Icon({ name, size = 18, ...props }: { name: string; size?: number } & SVGProps<SVGSVGElement>) {
  const shape = paths[name] ?? paths.overview
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>
      {shape.map((d, index) => <path key={index} d={d} />)}
    </svg>
  )
}
