import { useEffect, useState } from 'react'
import { IconClose } from './Icons'

const PRESETS = [
  { label: '阿里 DNS', host: '223.5.5.5' },
  { label: '腾讯 DNS', host: '119.29.29.29' },
  { label: 'Cloudflare', host: '1.1.1.1' },
  { label: 'Google DNS', host: '8.8.8.8' },
  { label: '百度', host: 'www.baidu.com' },
]

const INTERVALS = [1, 2, 3, 5, 10, 30, 60]
const SIZES = [32, 56, 64, 128, 512, 1000, 1400]

export default function TargetFormModal({
  open,
  mode = 'add',
  initial,
  onClose,
  onSubmit,
  submitting,
  error,
}) {
  const isEdit = mode === 'edit'
  const [name, setName] = useState('')
  const [host, setHost] = useState('')
  const [interval, setIntervalSec] = useState(5)
  const [size, setSize] = useState(32)
  const [localError, setLocalError] = useState('')

  useEffect(() => {
    if (!open) return
    if (isEdit && initial) {
      setName(initial.name ?? '')
      setHost(initial.host ?? '')
      setIntervalSec(initial.interval_sec ?? 5)
      setSize(initial.packet_size ?? 32)
    } else {
      setName('')
      setHost('')
      setIntervalSec(5)
      setSize(32)
    }
    setLocalError('')
  }, [open, isEdit, initial])

  useEffect(() => {
    if (!open) return undefined
    const onKey = (event) => {
      if (event.key === 'Escape' && !submitting) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, submitting, onClose])

  if (!open) return null

  const applyPreset = (preset) => {
    setHost(preset.host)
    if (!name.trim()) setName(preset.label)
  }

  const submit = (event) => {
    event.preventDefault()
    const hostValue = host.trim()
    if (!hostValue) {
      setLocalError('请填写要监测的主机')
      return
    }
    const sizeValue = Number(size)
    if (!Number.isFinite(sizeValue) || sizeValue < 1 || sizeValue > 65000) {
      setLocalError('数据大小需在 1–65000 字节之间')
      return
    }
    setLocalError('')
    onSubmit({
      name: name.trim() || hostValue,
      host: hostValue,
      interval_sec: Number(interval),
      packet_size: Math.round(sizeValue),
    })
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && !submitting && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="target-form-title">
        <div className="modal-head">
          <div>
            <h3 id="target-form-title">{isEdit ? '编辑监测目标' : '添加监测目标'}</h3>
            <p>
              {isEdit
                ? '修改保存后立即按新参数重新开始探测，历史记录保留'
                : '支持域名或 IP，探测间隔越短，曲线越细腻'}
            </p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="关闭">
            <IconClose />
          </button>
        </div>

        <form onSubmit={submit}>
          <div className="field">
            <label htmlFor="host">主机地址</label>
            <input
              id="host"
              autoFocus
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder="例如 223.5.5.5 或 www.baidu.com"
              spellCheck={false}
              autoComplete="off"
            />
            {!isEdit && (
              <div className="presets">
                {PRESETS.map((preset) => (
                  <button
                    type="button"
                    key={preset.host}
                    className={`preset ${host === preset.host ? 'is-active' : ''}`}
                    onClick={() => applyPreset(preset)}
                  >
                    {preset.label}
                    <span>{preset.host}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="field">
            <label htmlFor="name">显示名称</label>
            <input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="默认与主机地址一致"
              autoComplete="off"
            />
          </div>

          <div className="field-row">
            <div className="field">
              <label htmlFor="interval">探测间隔</label>
              <select id="interval" value={interval} onChange={(e) => setIntervalSec(e.target.value)}>
                {INTERVALS.map((sec) => (
                  <option key={sec} value={sec}>
                    {sec} 秒
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="packet-size">数据大小（-l）</label>
              <input
                id="packet-size"
                type="number"
                min="1"
                max="65000"
                step="1"
                value={size}
                onChange={(e) => setSize(e.target.value)}
              />
            </div>
          </div>

          <div className="size-presets">
            {SIZES.map((bytes) => (
              <button
                type="button"
                key={bytes}
                className={`preset ${Number(size) === bytes ? 'is-active' : ''}`}
                onClick={() => setSize(bytes)}
              >
                {bytes} B
              </button>
            ))}
          </div>
          <p className="field-hint">对应 ping 的 -l 参数（字节），默认 32；调大可测试 MTU 与分片表现。</p>

          {(error || localError) && <div className="form-error">{localError || error}</div>}

          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose} disabled={submitting}>
              取消
            </button>
            <button type="submit" className="btn btn-primary" disabled={submitting || !host.trim()}>
              {submitting ? (isEdit ? '保存中…' : '添加中…') : isEdit ? '保存修改' : '添加并开始监测'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
