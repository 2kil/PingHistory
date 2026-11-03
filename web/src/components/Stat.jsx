export default function Stat({ label, value, unit, sub, tone }) {
  return (
    <div className="stat">
      <div className="stat-label">{label}</div>
      <div className={`stat-value ${tone ?? ''}`}>
        {value}
        {unit && <em>{unit}</em>}
      </div>
      <div className="stat-sub">{sub ?? '\u00a0'}</div>
    </div>
  )
}
