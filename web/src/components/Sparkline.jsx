export default function Sparkline({ series, width = 240, height = 30, stroke = '#2563eb' }) {
  const points = series.filter((p) => p.rtt != null)
  if (points.length < 2) {
    return (
      <svg className="sparkline" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
        <line x1="0" y1={height - 1} x2={width} y2={height - 1} stroke="#eef1f6" strokeWidth="1" />
      </svg>
    )
  }

  const first = series[0]?.t ?? points[0].t
  const last = series[series.length - 1]?.t ?? points[points.length - 1].t
  const span = Math.max(1, last - first)
  const maxValue = Math.max(...points.map((p) => p.rtt), 1)
  const pad = 2

  const coords = points.map((p) => {
    const x = ((p.t - first) / span) * (width - pad * 2) + pad
    const y = height - pad - (p.rtt / maxValue) * (height - pad * 2)
    return [x, y]
  })

  const line = coords.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(2)}`).join(' ')
  const area = `M${coords[0][0].toFixed(2)},${height} ${coords
    .map(([x, y]) => `L${x.toFixed(2)},${y.toFixed(2)}`)
    .join(' ')} L${coords[coords.length - 1][0].toFixed(2)},${height} Z`

  return (
    <svg className="sparkline" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
      <path d={area} fill={stroke} opacity="0.1" />
      <polyline points={line} fill="none" stroke={stroke} strokeWidth="1.6" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}
