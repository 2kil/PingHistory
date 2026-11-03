import { useMemo, useState } from 'react'
import { CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { formatClock, formatFullTime, formatLoss, formatMs } from '../format'
import { axisScale, fillSeriesGaps, mergeSeries, spanSeconds, timeDomain } from '../chartUtils'
import { buildColorMap } from '../palette'

function OverviewTooltip({ active, payload, colorMap, nameMap, hidden }) {
  if (!active || !payload || !payload.length) return null
  const time = payload[0]?.payload?.t
  // 延迟为空但存在丢包的桶也要展示，否则"整桶全丢"的目标会从提示里消失。
  const rows = payload
    .map((entry) => {
      const id = Number(String(entry.dataKey).slice(1))
      return {
        id,
        dataKey: entry.dataKey,
        value: entry.value,
        loss: entry.payload?.[`l${id}`] ?? 0,
      }
    })
    .filter((row) => !hidden.has(row.id) && (row.value != null || row.loss > 0))
    .sort((a, b) => (b.value ?? -1) - (a.value ?? -1))

  return (
    <div className="chart-tooltip">
      <div className="tt-time">{time ? formatFullTime(time) : ''}</div>
      {rows.length === 0 && <div className="tt-row tt-empty">全部目标无数据</div>}
      {rows.map((row) => (
        <div className="tt-row" key={row.dataKey}>
          <span className="tt-key">
            <i className="tt-dot" style={{ background: colorMap.get(row.id) }} />
            {nameMap.get(row.id) ?? '未知目标'}
          </span>
          <span className={`tt-val ${row.value == null ? 'text-danger' : ''}`}>
            {row.value == null ? '全部超时' : `${row.value.toFixed(2)} ms`}
            {row.loss > 0 && <span className="tt-loss"> · 丢包 {formatLoss(row.loss)}</span>}
          </span>
        </div>
      ))}
    </div>
  )
}

export default function OverviewChart({ targets, range, ranges, onRangeChange }) {
  const [hidden, setHidden] = useState(() => new Set())

  const colorMap = useMemo(() => buildColorMap(targets), [targets])
  const nameMap = useMemo(() => new Map(targets.map((t) => [t.id, t.name])), [targets])
  // 各目标共享同一时间桶栅格（后端用固定的 until 计算），这里取首个可用间距作为桶宽。
  const bucketMs = useMemo(() => {
    for (const target of targets) {
      const list = target.series ?? []
      if (list.length > 1) return list[1].t - list[0].t
    }
    return 0
  }, [targets])

  const data = useMemo(() => fillSeriesGaps(mergeSeries(targets), bucketMs), [targets, bucketMs])
  const xDomain = useMemo(() => timeDomain(data), [data])
  const span = useMemo(() => spanSeconds(data), [data])
  // 补齐空行只用于断线，计数时排除（空行只有 t 一个键）。
  const sampleCount = useMemo(() => data.filter((row) => Object.keys(row).length > 1).length, [data])

  const activeTargets = targets.filter((t) => !hidden.has(t.id))
  const values = activeTargets
    .flatMap((t) => (t.series ?? []).map((p) => p.rtt))
    .filter((v) => v != null)
  const maxValue = values.length ? Math.max(...values) : 10
  const { top: domainMax, ticks } = axisScale(Math.max(maxValue * 1.2, 3))

  const toggle = (id) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        // 至少保留一条曲线，避免出现空白图。
        if (next.size >= targets.length - 1) return prev
        next.add(id)
      }
      return next
    })
  }

  return (
    <section className="card chart-card">
      <div className="card-head">
        <div className="card-title">
          <h2>延迟对比</h2>
          <p>
            全部 {targets.length} 个目标叠加显示 · {range.label}内 {sampleCount} 个取样点
          </p>
        </div>
        <div className="segmented" role="tablist">
          {ranges.map((r) => (
            <button
              key={r.sec}
              type="button"
              role="tab"
              aria-selected={r.sec === range.sec}
              className={r.sec === range.sec ? 'is-active' : ''}
              onClick={() => onRangeChange(r)}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <div className="legend-chips">
        {targets.map((target) => {
          const color = colorMap.get(target.id)
          const off = hidden.has(target.id)
          return (
            <button
              type="button"
              key={target.id}
              className={`legend-chip ${off ? 'is-off' : ''}`}
              onClick={() => toggle(target.id)}
              title={off ? '点击显示该目标' : '点击隐藏该目标'}
            >
              <i className="legend-dot-fill" style={{ background: color }} />
              <span className="legend-name">{target.name}</span>
              <span className="legend-value">
                {target.last && target.last.ok ? `${formatMs(target.last.rtt)} ms` : '—'}
              </span>
            </button>
          )
        })}
      </div>

      <div className="chart-wrap">
        <ResponsiveContainer width="100%" height={340}>
          <ComposedChart data={data} margin={{ top: 18, right: 16, bottom: 4, left: 0 }}>
            <CartesianGrid stroke="#edf0f5" strokeDasharray="3 4" vertical={false} />
            <XAxis
              dataKey="t"
              type="number"
              scale="time"
              domain={xDomain}
              tickFormatter={(v) => formatClock(v, range.sec, span)}
              tick={{ fill: '#8a94a6', fontSize: 11 }}
              axisLine={{ stroke: '#e8ecf3' }}
              tickLine={false}
              minTickGap={48}
            />
            <YAxis
              domain={[0, domainMax]}
              ticks={ticks}
              allowDecimals={false}
              allowDataOverflow
              tickFormatter={(v) => `${v}ms`}
              tick={{ fill: '#8a94a6', fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              width={54}
            />
            <Tooltip
              content={<OverviewTooltip colorMap={colorMap} nameMap={nameMap} hidden={hidden} />}
              cursor={{ stroke: '#c7d2e5', strokeDasharray: '3 3' }}
            />
            {targets.map((target) => (
              <Line
                key={target.id}
                type="monotone"
                dataKey={`k${target.id}`}
                name={target.name}
                stroke={colorMap.get(target.id)}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 3.5, strokeWidth: 0 }}
                connectNulls={false}
                isAnimationActive={false}
                hide={hidden.has(target.id)}
              />
            ))}
          </ComposedChart>
        </ResponsiveContainer>

        {data.length === 0 && (
          <div className="chart-empty">
            <span>暂无采样数据</span>
            <small>目标开始采集后，这里会同时展示全部曲线</small>
          </div>
        )}
      </div>
    </section>
  )
}
