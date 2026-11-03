import { useEffect, useMemo, useState } from 'react'
import Sidebar from './components/Sidebar'
import StatsRow from './components/StatsRow'
import ChartPanel from './components/ChartPanel'
import OverviewView from './components/OverviewView'
import RecentProbes from './components/RecentProbes'
import TargetFormModal from './components/TargetFormModal'
import ConfirmDialog from './components/ConfirmDialog'
import ResetDialog from './components/ResetDialog'
import { IconEdit, IconEmpty, IconPlus, IconTrash } from './components/Icons'
import { colorAt } from './palette'
import { createTarget, deleteTarget, fetchHealth, fetchOverview, fetchProbes, resetStats, updateTarget } from './api'

const RANGES = [
  { label: '15 分钟', sec: 900, points: 180 },
  { label: '1 小时', sec: 3600, points: 240 },
  { label: '6 小时', sec: 21600, points: 240 },
  { label: '24 小时', sec: 86400, points: 288 },
  { label: '7 天', sec: 604800, points: 336 },
]

const POLL_MS = 3000

export default function App() {
  const [range, setRange] = useState(RANGES[1])
  const [targets, setTargets] = useState([])
  // view 为 'overview' 或目标 ID
  const [view, setView] = useState('overview')
  const [probes, setProbes] = useState([])
  const [health, setHealth] = useState(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)

  const [modalOpen, setModalOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState('')
  const [pendingDelete, setPendingDelete] = useState(null)
  const [resetOpen, setResetOpen] = useState(false)
  const [pendingReset, setPendingReset] = useState(null)
  const [resetting, setResetting] = useState(false)
  const [editTarget, setEditTarget] = useState(null)
  const [saving, setSaving] = useState(false)
  const [editError, setEditError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const data = await fetchOverview(range.sec, range.points)
        if (cancelled) return
        const list = data.targets ?? []
        setTargets(list)
        // 选择状态在数据刷新时统一收敛：当前目标仍存在则保留，否则回到总览。
        setView((prev) => {
          if (prev === 'overview') return prev
          return list.some((t) => t.id === prev) ? prev : 'overview'
        })
        setError('')
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoaded(true)
      }
    }
    load()
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') load()
    }, POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [range, refreshKey])

  const selectedId = typeof view === 'number' ? view : null

  useEffect(() => {
    if (!selectedId) {
      setProbes([])
      return undefined
    }
    let cancelled = false
    const load = async () => {
      try {
        const data = await fetchProbes(selectedId, 24)
        if (!cancelled) setProbes(data.probes ?? [])
      } catch {
        /* 历史记录失败不影响主视图 */
      }
    }
    load()
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') load()
    }, POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [selectedId, refreshKey])

  useEffect(() => {
    let cancelled = false
    const load = () => {
      fetchHealth()
        .then((data) => !cancelled && setHealth(data))
        .catch(() => {})
    }
    load()
    const timer = window.setInterval(load, 60000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  const selected = useMemo(() => targets.find((t) => t.id === selectedId) ?? null, [targets, selectedId])
  const selectedColor = useMemo(() => {
    const index = targets.findIndex((t) => t.id === selectedId)
    return colorAt(index)
  }, [targets, selectedId])

  const refresh = () => setRefreshKey((key) => key + 1)

  const handleAdd = async (payload) => {
    setSubmitting(true)
    setFormError('')
    try {
      const created = await createTarget(payload)
      setModalOpen(false)
      // 在总览页添加时留在总览，方便立即看到新曲线；单目标视图下则跳转到新目标。
      if (created?.id && view !== 'overview') setView(created.id)
      refresh()
    } catch (err) {
      setFormError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleToggle = async (target) => {
    try {
      await updateTarget(target.id, { enabled: !target.enabled })
      refresh()
    } catch (err) {
      setError(err.message)
    }
  }

  const openEdit = (target) => {
    setEditError('')
    setEditTarget(target)
  }

  const handleSaveEdit = async (payload) => {
    if (!editTarget) return
    setSaving(true)
    setEditError('')
    try {
      await updateTarget(editTarget.id, payload)
      setEditTarget(null)
      refresh()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    const target = pendingDelete
    setPendingDelete(null)
    if (!target) return
    try {
      await deleteTarget(target.id)
      if (view === target.id) setView('overview')
      refresh()
    } catch (err) {
      setError(err.message)
    }
  }

  const openAdd = () => {
    setFormError('')
    setModalOpen(true)
  }

  const openReset = () => {
    setResetOpen(true)
    // 打开对话框时刷新一次健康信息，确保保留天数与样本总数是最新的。
    fetchHealth()
      .then((data) => data && setHealth(data))
      .catch(() => {})
  }

  // 第一层对话框点「重置」后进入二次确认，确认后才真正删除。
  const handleConfirmReset = async () => {
    const beforeDays = pendingReset ?? 0
    setResetting(true)
    try {
      await resetStats(beforeDays)
      setPendingReset(null)
      setResetOpen(false)
      setView('overview')
      refresh()
    } catch (err) {
      setError(err.message)
    } finally {
      setResetting(false)
    }
  }

  const totalProbes = health?.total_probes ?? targets.reduce((sum, t) => sum + t.stats.total, 0)

  return (
    <div className="app">
      <Sidebar
        targets={targets}
        view={view}
        onSelect={setView}
        onSelectOverview={() => setView('overview')}
        onAdd={openAdd}
        onToggle={handleToggle}
        onDelete={(target) => setPendingDelete(target)}
        health={health}
      />

      <main className="main">
        {error && (
          <div className="banner-error">
            <span>无法获取数据：{error}</span>
            <button type="button" className="banner-retry" onClick={refresh}>
              重试
            </button>
          </div>
        )}

        {!loaded ? (
          <div className="loading">正在连接本地监测服务…</div>
        ) : targets.length === 0 ? (
          <div className="empty-state">
            <IconEmpty />
            <h2>还没有监测目标</h2>
            <p>添加一个域名或 IP，系统会按设定的间隔持续 ping 并记录延迟曲线。</p>
            <button type="button" className="btn btn-primary" onClick={openAdd}>
              <IconPlus />
              添加第一个目标
            </button>
          </div>
        ) : view === 'overview' || !selected ? (
          <OverviewView
            targets={targets}
            range={range}
            ranges={RANGES}
            onRangeChange={setRange}
            onSelectTarget={setView}
            onReset={openReset}
          />
        ) : (
          <>
            <header className="main-head">
              <div className="head-left">
                <div className="head-title">
                  <h1>{selected.name}</h1>
                  <span className="live">
                    <i />
                    实时 · 每 {POLL_MS / 1000} 秒刷新
                  </span>
                </div>
                <div className="head-meta">
                  <span className="chip chip-mono">{selected.host}</span>
                  <span className={`chip ${selected.enabled ? 'chip-good' : 'chip-muted'}`}>
                    {selected.enabled ? '采集中' : '已暂停'}
                  </span>
                  <span className="chip chip-plain">每 {selected.interval_sec}s 探测</span>
                  <span className="chip chip-plain">数据 {selected.packet_size}B</span>
                </div>
              </div>
              <div className="head-actions">
                <button type="button" className="btn btn-ghost" onClick={() => setView('overview')}>
                  返回总览
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => openEdit(selected)}>
                  <IconEdit />
                  编辑
                </button>
                <button
                  type="button"
                  className="btn btn-ghost danger"
                  onClick={() => setPendingDelete(selected)}
                >
                  <IconTrash />
                  删除
                </button>
              </div>
            </header>

            <StatsRow target={selected} />
            <ChartPanel
              target={selected}
              color={selectedColor}
              range={range}
              ranges={RANGES}
              onRangeChange={setRange}
            />
            <RecentProbes probes={probes} enabled={selected.enabled} />
          </>
        )}
      </main>

      <TargetFormModal
        open={modalOpen}
        mode="add"
        onClose={() => setModalOpen(false)}
        onSubmit={handleAdd}
        submitting={submitting}
        error={formError}
      />

      <TargetFormModal
        open={Boolean(editTarget)}
        mode="edit"
        initial={editTarget}
        onClose={() => {
          if (!saving) setEditTarget(null)
        }}
        onSubmit={handleSaveEdit}
        submitting={saving}
        error={editError}
      />

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title="删除监测目标"
        message={`将删除「${pendingDelete?.name ?? ''}」及其全部历史延迟数据，此操作不可恢复。`}
        confirmText="删除"
        onCancel={() => setPendingDelete(null)}
        onConfirm={handleDelete}
      />

      {resetOpen && (
        <ResetDialog
          initialRetention={health?.retention_days ?? 7}
          busy={resetting}
          onClose={() => {
            if (resetting) return
            setResetOpen(false)
            setPendingReset(null)
          }}
          onConfirm={(beforeDays) => setPendingReset(beforeDays)}
          onRetentionSaved={(days) => setHealth((prev) => (prev ? { ...prev, retention_days: days } : prev))}
        />
      )}

      <ConfirmDialog
        open={pendingReset !== null}
        title="确认重置"
        message={
          pendingReset === 0
            ? `将删除全部历史探测记录（约 ${totalProbes} 条），并让所有目标立即同时开始新的记录。此操作不可恢复。`
            : `将删除 ${pendingReset} 天之前的全部探测记录，最近 ${pendingReset} 天的数据会保留。此操作不可恢复。`
        }
        confirmText={resetting ? '处理中…' : '确认删除'}
        busy={resetting}
        onCancel={() => setPendingReset(null)}
        onConfirm={handleConfirmReset}
      />
    </div>
  )
}
