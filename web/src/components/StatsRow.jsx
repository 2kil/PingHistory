import Stat from './Stat'
import { formatLoss, formatMs, formatRelative, formatTimeOfDay } from '../format'

export default function StatsRow({ target }) {
  const { stats, last } = target
  const current = last ? (last.ok ? formatMs(last.rtt) : '超时') : '—'
  const currentUnit = last && last.ok ? 'ms' : ''
  const currentSub = last ? formatRelative(last.ts) : '等待采样'
  const lossTone = stats.loss > 0 ? 'text-danger' : ''
  // 长区间下后端不再单独查询峰值时刻（避免全表排序），改为从聚合序列里定位峰值桶。
  const peakBucket =
    stats.max_ts || stats.max == null
      ? null
      : (target.series ?? []).find((p) => p.max_rtt === stats.max)
  const peakTs = stats.max_ts || peakBucket?.t || 0
  const peakSub = peakTs ? `${formatTimeOfDay(peakTs)} 出现` : '所选区间'

  return (
    <div className="stats">
      <Stat
        label="当前延迟"
        value={current}
        unit={currentUnit}
        tone={last && !last.ok ? 'text-danger' : ''}
        sub={currentSub}
      />
      <Stat label="平均" value={formatMs(stats.avg)} unit="ms" sub="所选区间" />
      <Stat label="最低" value={formatMs(stats.min)} unit="ms" sub="所选区间" />
      <Stat label="最高" value={formatMs(stats.max)} unit="ms" sub={peakSub} />
      <Stat
        label="丢包率"
        value={formatLoss(stats.loss)}
        tone={lossTone}
        sub={`${stats.lost} / ${stats.total} 次`}
      />
      <Stat label="采样数" value={stats.total} sub={`每 ${target.interval_sec}s 一次`} />
    </div>
  )
}
