import { useEffect, useState } from 'react'
import { IconClose } from './Icons'
import { updateSettings } from '../api'

// 清除范围：删除 N 天「之前」的记录；0 表示全部。
const RANGE_OPTIONS = [
  { value: 1, label: '1 天前' },
  { value: 7, label: '7 天前' },
  { value: 30, label: '30 天前' },
  { value: 0, label: '全部数据' },
]

// 自动保留：超过 N 天的数据会被定时清理；0 表示永久保留。
const RETENTION_OPTIONS = [
  { value: 1, label: '1 天' },
  { value: 3, label: '3 天' },
  { value: 7, label: '7 天' },
  { value: 0, label: '永久' },
]

export default function ResetDialog({ initialRetention = 7, busy, onClose, onConfirm, onRetentionSaved }) {
  const [range, setRange] = useState(7)
  const [retention, setRetention] = useState(initialRetention)
  const [saving, setSaving] = useState(false)
  const [hint, setHint] = useState('')
  const [error, setError] = useState('')

  // 组件挂载即视为"打开"（由父组件条件渲染控制），状态天然是干净的。
  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  // 自动保留是即时生效的：选择后立刻写盘，不需要点重置。
  const changeRetention = async (value) => {
    setRetention(value)
    setSaving(true)
    setError('')
    setHint('')
    try {
      await updateSettings({ retention_days: value })
      setHint(value === 0 ? '已设为永久保留，不再自动清理' : `已生效：超过 ${value} 天的数据会被自动清理`)
      onRetentionSaved?.(value)
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const rangeLabel = RANGE_OPTIONS.find((o) => o.value === range)?.label ?? ''

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && !busy && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="reset-title">
        <div className="modal-head">
          <div>
            <h3 id="reset-title">重置统计</h3>
            <p>只删除历史记录，监测目标与配置都会保留</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="关闭" disabled={busy}>
            <IconClose />
          </button>
        </div>

        <div className="field">
          <label htmlFor="reset-range">清除多久以前的数据</label>
          <select
            id="reset-range"
            value={range}
            onChange={(e) => setRange(Number(e.target.value))}
            disabled={busy}
          >
            {RANGE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <p className="field-hint">
            {range === 0
              ? '将清空全部探测记录，并让所有目标从同一时刻重新开始采集。'
              : `将删除 ${rangeLabel} 的探测记录，最近 ${range} 天的数据保留，正在进行的采集不受影响。`}
          </p>
        </div>

        <div className="field">
          <label htmlFor="reset-retention">自动保留数据时间</label>
          <select
            id="reset-retention"
            value={retention}
            onChange={(e) => changeRetention(Number(e.target.value))}
            disabled={busy || saving}
          >
            {RETENTION_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <p className="field-hint">
            {saving ? '正在保存…' : hint || '修改后立即生效，无需重启；超出保留期的数据会自动清理。'}
          </p>
        </div>

        {error && <div className="form-error">{error}</div>}

        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={busy}>
            取消
          </button>
          <button type="button" className="btn btn-danger" onClick={() => onConfirm(range)} disabled={busy || saving}>
            {busy ? '处理中…' : '重置'}
          </button>
        </div>
      </div>
    </div>
  )
}
