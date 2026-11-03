import { buildColorMap } from '../palette'
import { formatLoss, formatMs } from '../format'

export default function OverviewTable({ targets, onSelect }) {
  const colorMap = buildColorMap(targets)

  return (
    <section className="card">
      <div className="card-head">
        <div className="card-title">
          <h2>目标明细</h2>
          <p>统计区间与上方折线图一致 · 点击任意一行查看单目标曲线</p>
        </div>
      </div>

      <div className="overview-table">
        <div className="overview-row overview-header">
          <span>目标</span>
          <span className="num">当前</span>
          <span className="num">平均</span>
          <span className="num">最高</span>
          <span className="num">丢包</span>
          <span className="num">样本</span>
        </div>

        {targets.map((target) => {
          const last = target.last
          const current = !last ? '—' : last.ok ? `${formatMs(last.rtt)}` : '超时'
          return (
            <div
              key={target.id}
              className="overview-row"
              role="button"
              tabIndex={0}
              onClick={() => onSelect(target.id)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  onSelect(target.id)
                }
              }}
            >
              <span className="overview-name">
                <i className="legend-dot-fill" style={{ background: colorMap.get(target.id) }} />
                <span className="name">{target.name}</span>
                <span className="host">{target.host}</span>
                {!target.enabled && <span className="chip chip-muted chip-xs">已暂停</span>}
              </span>
              <span className={`num ${last && !last.ok ? 'text-danger' : ''}`}>{current}</span>
              <span className="num">{formatMs(target.stats.avg)}</span>
              <span className="num">{formatMs(target.stats.max)}</span>
              <span className={`num ${target.stats.loss > 0 ? 'text-danger' : ''}`}>
                {formatLoss(target.stats.loss)}
              </span>
              <span className="num">{target.stats.total}</span>
            </div>
          )
        })}
      </div>
    </section>
  )
}
