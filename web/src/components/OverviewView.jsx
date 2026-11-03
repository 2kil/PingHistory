import OverviewChart from './OverviewChart'
import OverviewTable from './OverviewTable'
import Stat from './Stat'
import { IconReset } from './Icons'
import { formatLoss, formatMs } from '../format'

export default function OverviewView({ targets, range, ranges, onRangeChange, onSelectTarget, onReset }) {
  const activeCount = targets.filter((t) => t.enabled).length
  const pausedCount = targets.length - activeCount
  const totalSamples = targets.reduce((sum, t) => sum + t.stats.total, 0)
  const totalLost = targets.reduce((sum, t) => sum + t.stats.lost, 0)

  // 平均延迟按各目标样本数加权，避免采集间隔不同造成偏差。
  let weightedSum = 0
  let weight = 0
  for (const target of targets) {
    if (target.stats.avg != null && target.stats.total > 0) {
      weightedSum += target.stats.avg * target.stats.total
      weight += target.stats.total
    }
  }
  const avgAll = weight > 0 ? weightedSum / weight : null
  const lossAll = totalSamples > 0 ? totalLost / totalSamples : 0

  const ranked = targets.filter((t) => t.stats.avg != null).sort((a, b) => a.stats.avg - b.stats.avg)
  const fastest = ranked[0]
  const slowest = ranked[ranked.length - 1]
  const abnormal = targets.filter((t) => t.stats.loss > 0 || (t.last && !t.last.ok)).length

  return (
    <>
      <header className="main-head">
        <div className="head-left">
          <div className="head-title">
            <h1>总览</h1>
            <span className="live">
              <i />
              实时 · 每 3 秒刷新
            </span>
          </div>
          <div className="head-meta">
            <span className="chip chip-plain">{targets.length} 个目标</span>
            <span className="chip chip-good">{activeCount} 个采集中</span>
            {pausedCount > 0 && <span className="chip chip-muted">{pausedCount} 个已暂停</span>}
          </div>
        </div>
        <div className="head-actions">
          <button
            type="button"
            className="btn btn-ghost danger"
            onClick={onReset}
            title="清空所有目标的探测历史，并让它们从同一时刻重新开始记录"
          >
            <IconReset />
            重置统计
          </button>
        </div>
      </header>

      <div className="stats">
        <Stat label="平均延迟" value={formatMs(avgAll)} unit="ms" sub="全部目标加权" />
        <Stat
          label="最快目标"
          value={fastest ? formatMs(fastest.stats.avg) : '—'}
          unit={fastest ? 'ms' : ''}
          sub={fastest ? fastest.name : '暂无数据'}
        />
        <Stat
          label="最慢目标"
          value={slowest ? formatMs(slowest.stats.avg) : '—'}
          unit={slowest ? 'ms' : ''}
          sub={slowest ? slowest.name : '暂无数据'}
        />
        <Stat label="平均丢包" value={formatLoss(lossAll)} tone={lossAll > 0 ? 'text-danger' : ''} sub="全部目标合计" />
        <Stat
          label="异常目标"
          value={abnormal}
          tone={abnormal > 0 ? 'text-danger' : ''}
          sub="存在丢包或超时"
        />
        <Stat label="采样总数" value={totalSamples} sub="所选区间" />
      </div>

      <OverviewChart
        targets={targets}
        range={range}
        ranges={ranges}
        onRangeChange={onRangeChange}
      />
      <OverviewTable targets={targets} onSelect={onSelectTarget} />
    </>
  )
}
