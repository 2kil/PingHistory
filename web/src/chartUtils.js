// 把坐标轴上下限收敛到易读刻度：优先 1/2/5 的整数倍步长，约 5 条主网格线。
export function axisScale(maxValue) {
  const value = Math.max(maxValue, 1)
  const rawStep = value / 5
  const pow = Math.pow(10, Math.floor(Math.log10(rawStep)))
  const step = [1, 2, 5, 10].map((m) => m * pow).find((s) => s >= rawStep) ?? pow * 10
  const top = Math.ceil(value / step) * step
  const ticks = []
  for (let v = 0; v <= top + 1e-9; v += step) {
    ticks.push(Math.round(v * 10) / 10)
  }
  return { top, ticks }
}

// 取中位数，用于在含极端尖峰的序列里估计"典型水平"。
export function median(values) {
  if (!values.length) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

// 计算时间轴范围。数据只有一个点时（例如刚重置统计），给一个对称的窗口，
// 否则 domain 会退化成 [t, t] 导致曲线被拉成整屏的直线。
export function timeDomain(data, fallbackMs = 60000) {
  if (!data.length) {
    const now = Date.now()
    return [now - fallbackMs, now]
  }
  const min = data[0].t
  const max = data[data.length - 1].t
  if (max - min >= 1000) return [min, max]
  const center = (min + max) / 2
  return [center - 15000, center + 15000]
}

// 数据实际跨度（秒），用于决定刻度的时间粒度。
export function spanSeconds(data) {
  if (data.length < 2) return 0
  return (data[data.length - 1].t - data[0].t) / 1000
}

// fillSeriesGaps 在缺失的时间桶位置插入空行。
// 没有这一步，折线会直接跨过停机/暂停区间连成一条直线，看起来像"这段时间延迟很稳定"。
export function fillSeriesGaps(rows, bucketMs, maxGap = 2000) {
  if (!bucketMs || rows.length < 2) return rows
  const out = []
  for (let i = 0; i < rows.length; i += 1) {
    const current = rows[i]
    if (i > 0) {
      const previous = rows[i - 1]
      const missing = Math.round((current.t - previous.t) / bucketMs) - 1
      if (missing > 0 && missing <= maxGap) {
        for (let k = 1; k <= missing; k += 1) {
          out.push({ t: previous.t + k * bucketMs })
        }
      }
    }
    out.push(current)
  }
  return out
}

// 把多个目标的时间序列按时间桶对齐成一张表，供多折线图使用。
// 所有目标使用相同的 range/points 请求，因此桶边界天然一致。
export function mergeSeries(targets) {
  const rows = new Map()
  for (const target of targets) {
    for (const point of target.series ?? []) {
      let row = rows.get(point.t)
      if (!row) {
        row = { t: point.t }
        rows.set(point.t, row)
      }
      row[`k${target.id}`] = point.rtt
      row[`l${target.id}`] = point.loss
    }
  }
  return [...rows.values()].sort((a, b) => a.t - b.t)
}
