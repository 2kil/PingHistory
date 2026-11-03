export function pad2(n) {
  return String(n).padStart(2, '0')
}

// 延迟统一保留 1 位小数，超过 100ms 取整，避免数字抖动。
export function formatMs(value) {
  if (value == null || Number.isNaN(value)) return '—'
  if (value >= 100) return String(Math.round(value))
  return value.toFixed(1)
}

export function formatLoss(loss) {
  if (loss == null || Number.isNaN(loss)) return '0%'
  const pct = loss * 100
  if (pct === 0) return '0%'
  if (pct < 10) return `${pct.toFixed(1)}%`
  return `${Math.round(pct)}%`
}

// rangeSec 是所选时间范围，spanSec 是当前数据的实际跨度。
// 取两者较小值决定刻度粒度：重置统计后数据只有几十秒，仍按所选区间格式化会出现多个相同标签。
export function formatClock(ts, rangeSec, spanSec = rangeSec) {
  const d = new Date(ts)
  const effective = Math.min(rangeSec || spanSec, spanSec || rangeSec)
  if (effective > 86400) {
    return `${d.getMonth() + 1}/${d.getDate()} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  }
  if (effective > 1800) {
    return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  }
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

export function formatShortClock(ts, rangeSec) {
  const d = new Date(ts)
  if (rangeSec > 86400) {
    return `${d.getMonth() + 1}/${d.getDate()}`
  }
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

export function formatFullTime(ts) {
  const d = new Date(ts)
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(
    d.getMinutes(),
  )}:${pad2(d.getSeconds())}`
}

export function formatTimeOfDay(ts) {
  const d = new Date(ts)
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

export function formatRelative(ts, now = Date.now()) {
  const diff = Math.max(0, Math.round((now - ts) / 1000))
  if (diff < 5) return '刚刚'
  if (diff < 60) return `${diff} 秒前`
  if (diff < 3600) return `${Math.round(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.round(diff / 3600)} 小时前`
  return `${Math.round(diff / 86400)} 天前`
}
