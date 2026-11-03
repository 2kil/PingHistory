import { formatMs } from '../format'

function clock(ts) {
  const d = new Date(ts)
  const p = (n) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

export default function RecentProbes({ probes, enabled }) {
  return (
    <section className="card probes-card">
      <div className="card-head">
        <div className="card-title">
          <h2>最近探测记录</h2>
          <p>最新的原始采样，按时间倒序</p>
        </div>
        {!enabled && <span className="chip chip-muted">采集已暂停</span>}
      </div>

      <div className="probes-table">
        <div className="probes-row probes-header">
          <span>时间</span>
          <span>延迟</span>
          <span>状态</span>
        </div>
        {probes.length === 0 && <div className="probes-empty">暂无记录</div>}
        {probes.map((p) => (
          <div className="probes-row" key={p.ts}>
            <span className="mono">{clock(p.ts)}</span>
            <span className={`mono ${p.ok ? '' : 'text-danger'}`}>{p.ok ? `${formatMs(p.rtt)} ms` : '—'}</span>
            <span>
              {p.ok ? (
                <span className="chip chip-good">成功</span>
              ) : (
                <span className="chip chip-bad">超时</span>
              )}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}
