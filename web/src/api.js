const BASE = ''

async function request(path, options = {}) {
  const res = await fetch(BASE + path, {
    headers: options.body ? { 'Content-Type': 'application/json' } : undefined,
    ...options,
  })
  const text = await res.text()
  let payload = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }
  if (!res.ok) {
    const message = payload?.error || `请求失败（HTTP ${res.status}）`
    throw new Error(message)
  }
  return payload
}

export function fetchOverview(rangeSec, points, signal) {
  return request(`/api/overview?range=${rangeSec}&points=${points}`, { signal })
}

export function fetchProbes(targetId, limit = 24, signal) {
  return request(`/api/targets/${targetId}/probes?limit=${limit}`, { signal })
}

export function fetchHealth(signal) {
  return request('/api/health', { signal })
}

export function createTarget(payload) {
  return request('/api/targets', { method: 'POST', body: JSON.stringify(payload) })
}

export function updateTarget(id, payload) {
  return request(`/api/targets/${id}`, { method: 'PATCH', body: JSON.stringify(payload) })
}

export function deleteTarget(id) {
  return request(`/api/targets/${id}`, { method: 'DELETE' })
}

// resetStats 按范围删除探测历史：beforeDays=0 清空全部并让所有目标重新开始采集，
// >0 只删除该天数之前的记录（保留最近 N 天）。
export function resetStats(beforeDays = 0) {
  return request('/api/reset', {
    method: 'POST',
    body: JSON.stringify({ before_days: beforeDays }),
  })
}

// updateSettings 修改运行设置并持久化，目前用于自动保留天数（0 = 永久）。
export function updateSettings(payload) {
  return request('/api/settings', {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}
