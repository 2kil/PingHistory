import { useEffect, useRef, useState } from 'react'
import {
  Area,
  Bar,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { formatClock, formatFullTime, formatLoss, formatMs } from '../format'
import { axisScale, median, spanSeconds, timeDomain } from '../chartUtils'

// 丢包率分档配色：颜色越深，桶内丢包越严重。
function lossColor(loss) {
  if (loss <= 0) return 'transparent'
  if (loss <= 0.05) return '#fecdca'
  if (loss <= 0.2) return '#fda29b'
  if (loss <= 0.5) return '#f97066'
  return '#d92d20'
}

function ChartTooltip({ active, payload }) {
  if (!active || !payload || !payload.length) return null
  const point = payload.find((entry) => entry.dataKey === 'rtt')?.payload ?? payload[0].payload
  return (
    <div className="chart-tooltip">
      <div className="tt-time">{formatFullTime(point.t)}</div>
      <div className="tt-row">
        <span className="tt-key">平均延迟</span>
        <span className={`tt-val ${point.rtt == null ? 'text-danger' : ''}`}>
          {point.rtt == null ? '全部超时' : `${point.rtt.toFixed(2)} ms`}
        </span>
      </div>
      <div className="tt-row">
        <span className="tt-key">区间峰值</span>
        <span className="tt-val">{point.max == null ? '—' : `${point.max.toFixed(2)} ms`}</span>
      </div>
      <div className="tt-row">
        <span className="tt-key">丢包</span>
        <span className={`tt-val ${point.loss > 0 ? 'text-danger' : ''}`}>
          {formatLoss(point.loss)}
          {point.loss > 0 && point.n ? ` · ${Math.round(point.loss * point.n)}/${point.n}` : ''}
        </span>
      </div>
      <div className="tt-row">
        <span className="tt-key">样本</span>
        <span className="tt-val">{point.n}</span>
      </div>
    </div>
  )
}

export default function ChartPanel({ target, color = '#2563eb', range, ranges, onRangeChange }) {
  const series = target?.series ?? []
  const avgs = series.map((p) => p.rtt).filter((v) => v != null)
  // 坐标轴按平均序列自适应，避免个别尖峰把整条曲线压成一条直线；
  // 超出范围的峰值线会被裁切，真实数值仍可在 tooltip 与统计卡片中看到。
  const bucketMax = series.reduce((m, p) => (p.max_rtt != null && p.max_rtt > m ? p.max_rtt : m), 0)
  const rawMax = target?.stats?.max ?? bucketMax
  // 纵轴以"典型延迟"（桶均值的中位数）为基准，而不是被尖峰抬高的最大值：
  // 典型值 1.5 倍内正常显示，尖峰最多放大到典型值的 3 倍，再高则裁切并在图例处标注。
  const typical = Math.max(median(avgs), 1)
  const axisCap = Math.max(typical * 3, 5)
  const axisBase = Math.min(Math.max(typical * 1.5, bucketMax), axisCap)
  const { top: domainMax, ticks: yTicks } = axisScale(Math.max(axisBase, 3))
  const peakClipped = rawMax > domainMax + 0.01
  const gradientId = `rttFill-${color.replace('#', '')}`

  const data = series.map((p) => ({
    t: p.t,
    rtt: p.rtt,
    max: p.max_rtt,
    loss: p.loss,
    n: p.n,
    // 有丢包的桶在曲线上打红点；整桶全丢时贴底显示，避免被坐标轴遮住。
    lossMark: p.loss > 0 ? (p.rtt ?? Number((domainMax * 0.04).toFixed(2))) : null,
    // 底部热力条带：发生丢包的桶画满格，颜色深度代表丢包率。
    lossBand: p.loss > 0 ? 100 : 0,
  }))

  const hasLoss = series.some((p) => p.loss > 0)

  // 丢包条带的柱子宽度按"每个时间桶占多少像素"自适应，数据稀疏时也能铺满自己的时间段。
  const stripRef = useRef(null)
  const [stripWidth, setStripWidth] = useState(900)
  useEffect(() => {
    const el = stripRef.current
    if (!el || typeof ResizeObserver === 'undefined') return undefined
    const observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect?.width
      if (width > 0) setStripWidth(width)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  const bucketPx = (stripWidth - 70) / Math.max(data.length, 1)
  const lossBarSize = Math.max(1.5, bucketPx - 0.6)

  const xDomain = timeDomain(data)
  const span = spanSeconds(data)

  const successful = data.filter((d) => d.rtt != null)

  return (
    <section className="card chart-card">
      <div className="card-head">
        <div className="card-title">
          <h2>延迟趋势</h2>
          <p>
            {range.label}内 {successful.length} 个取样点
            {target?.enabled ? ` · 每 ${target.interval_sec}s 探测一次` : ' · 采集已暂停'}
            {' · '}
            <span className={target.stats?.loss > 0 ? 'text-danger' : ''}>
              丢包 {formatLoss(target.stats?.loss)}
            </span>
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

      <div className="chart-legend">
        <span className="legend-item">
          <i className="legend-line" style={{ background: color }} /> 平均延迟
        </span>
        <span className="legend-item">
          <i className="legend-line legend-line-peak" /> 区间峰值
        </span>
        <span className="legend-item">
          <i className="legend-dot" /> 丢包时刻
        </span>
        <span className="legend-item">
          <i className="legend-block" /> 丢包（颜色越深丢包率越高）
        </span>
        {peakClipped && (
          <span className="chart-note" title="纵轴按平均延迟缩放，个别尖峰已超出显示范围">
            峰值 {formatMs(rawMax)}ms 超出纵轴
          </span>
        )}
      </div>

      <div className="chart-wrap">
        <div className="chart-main">
          <ResponsiveContainer width="100%" height={300}>
          <ComposedChart data={data} margin={{ top: 18, right: 16, bottom: 0, left: 0 }}>
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity="0.2" />
                <stop offset="100%" stopColor={color} stopOpacity="0.02" />
              </linearGradient>
            </defs>
            <CartesianGrid stroke="#edf0f5" strokeDasharray="3 4" vertical={false} />
            <XAxis dataKey="t" type="number" scale="time" domain={xDomain} hide />
            <YAxis
              domain={[0, domainMax]}
              ticks={yTicks}
              allowDecimals={false}
              allowDataOverflow
              tickFormatter={(v) => `${v}ms`}
              tick={{ fill: '#8a94a6', fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              width={54}
            />
            <Tooltip content={<ChartTooltip />} cursor={{ stroke: '#c7d2e5', strokeDasharray: '3 3' }} />
            <Area
              type="monotone"
              dataKey="rtt"
              stroke="none"
              fill={`url(#${gradientId})`}
              connectNulls={false}
              isAnimationActive={false}
              activeDot={false}
            />
            <Line
              type="monotone"
              dataKey="max"
              stroke={color}
              strokeOpacity={0.32}
              strokeWidth={1.2}
              strokeDasharray="3 3"
              dot={false}
              connectNulls={false}
              isAnimationActive={false}
              activeDot={false}
            />
            <Line
              type="monotone"
              dataKey="rtt"
              stroke={color}
              strokeWidth={2}
              dot={false}
              connectNulls={false}
              isAnimationActive={false}
              activeDot={{ r: 4, strokeWidth: 2, stroke: '#ffffff' }}
            />
            <Line
              type="monotone"
              dataKey="lossMark"
              stroke="none"
              dot={{ r: 3.2, fill: '#f04438', stroke: '#ffffff', strokeWidth: 1.2 }}
              activeDot={false}
              connectNulls={false}
              isAnimationActive={false}
              legendType="none"
            />
          </ComposedChart>
          </ResponsiveContainer>

          {successful.length === 0 && (
            <div className="chart-empty">
              <span>{data.length === 0 ? '暂无采样数据' : '全部探测超时'}</span>
              <small>
                {!target?.enabled
                  ? '该目标已暂停采集'
                  : data.length === 0
                    ? '正在等待首次探测结果…'
                    : `${formatLoss(target.stats?.loss)} 的探测未收到响应`}
              </small>
            </div>
          )}
        </div>

        <div className="loss-strip" ref={stripRef}>
          <span className="loss-strip-label">丢包</span>
          <ResponsiveContainer width="100%" height={44}>
            <ComposedChart data={data} margin={{ top: 0, right: 16, bottom: 0, left: 0 }}>
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
                height={22}
                tickMargin={7}
              />
              <YAxis domain={[0, 100]} hide width={54} />
              <Tooltip content={<ChartTooltip />} cursor={{ stroke: '#c7d2e5', strokeDasharray: '3 3' }} />
              <Bar
                dataKey="lossBand"
                isAnimationActive={false}
                barSize={lossBarSize}
                barCategoryGap={0}
                radius={[1, 1, 0, 0]}
              >
                {data.map((entry) => (
                  <Cell key={entry.t} fill={lossColor(entry.loss)} />
                ))}
              </Bar>
            </ComposedChart>
          </ResponsiveContainer>
          {!hasLoss && <span className="loss-strip-empty">区间内无丢包</span>}
        </div>
      </div>
    </section>
  )
}
