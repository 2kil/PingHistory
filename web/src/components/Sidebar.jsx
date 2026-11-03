import Sparkline from './Sparkline'
import { IconGrid, IconPause, IconPlay, IconPlus, IconTrash } from './Icons'
import { colorAt } from '../palette'
import { formatLoss, formatMs } from '../format'

function stop(handler) {
  return (event) => {
    event.stopPropagation()
    handler()
  }
}

export default function Sidebar({
  targets,
  view,
  onSelect,
  onSelectOverview,
  onAdd,
  onToggle,
  onDelete,
  health,
}) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
            <path
              d="M2 12.5h3.6l1.7-4.2 2.6 8.6 2-6 1.5 3.1H22"
              stroke="#fff"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>
        <div className="brand-text">
          <div className="brand-name">PingHistory</div>
          <div className="brand-sub">本地网络延迟监测</div>
        </div>
      </div>

      <button className="btn btn-primary btn-block" onClick={onAdd}>
        <IconPlus />
        添加监测目标
      </button>

      <button
        type="button"
        className={`nav-item ${view === 'overview' ? 'is-active' : ''}`}
        onClick={onSelectOverview}
      >
        <span className="nav-icon">
          <IconGrid />
        </span>
        <span className="nav-label">总览</span>
        <span className="nav-meta">{targets.length} 个目标</span>
      </button>

      <div className="section-label">
        监测目标
        <span className="section-count">{targets.length}</span>
      </div>

      <div className="target-list">
        {targets.length === 0 && <div className="target-list-empty">尚未添加目标</div>}
        {targets.map((t, index) => {
          const last = t.last
          const timedOut = t.enabled && last && !last.ok
          const dotClass = !t.enabled ? 'is-idle' : timedOut ? 'is-bad' : last ? 'is-good' : 'is-wait'
          const color = colorAt(index)
          const stroke = timedOut ? '#f04438' : !t.enabled ? '#98a2b3' : color

          return (
            <div
              key={t.id}
              className={`target ${view === t.id ? 'is-active' : ''} ${t.enabled ? '' : 'is-paused'}`}
              onClick={() => onSelect(t.id)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onSelect(t.id)
                }
              }}
            >
              <div className="target-head">
                <span className={`status-dot ${dotClass}`} />
                <span className="target-name" title={t.name}>
                  {t.name}
                </span>
                <span className="target-actions">
                  <button
                    type="button"
                    className="icon-btn"
                    title={t.enabled ? '暂停采集' : '恢复采集'}
                    onClick={stop(() => onToggle(t))}
                  >
                    {t.enabled ? <IconPause /> : <IconPlay />}
                  </button>
                  <button
                    type="button"
                    className="icon-btn danger"
                    title="删除目标"
                    onClick={stop(() => onDelete(t))}
                  >
                    <IconTrash />
                  </button>
                </span>
              </div>

              <div className="target-body">
                <span className="target-host" title={t.host}>
                  {t.host}
                </span>
                <span className="target-value">
                  {!t.enabled ? (
                    <span className="text-muted">已暂停</span>
                  ) : !last ? (
                    <span className="text-muted">采样中…</span>
                  ) : last.ok ? (
                    <>
                      {formatMs(last.rtt)}
                      <em>ms</em>
                    </>
                  ) : (
                    <span className="text-danger">超时</span>
                  )}
                </span>
              </div>

              <div className="target-spark">
                <Sparkline series={t.series} stroke={stroke} />
              </div>

              <div className="target-foot">
                <span className="target-foot-left">
                  <i className="target-color" style={{ background: color }} />
                  {t.interval_sec}s 间隔
                </span>
                <span className={t.stats.loss > 0 ? 'text-danger' : ''}>
                  {formatLoss(t.stats.loss)} 丢包
                </span>
              </div>
            </div>
          )
        })}
      </div>

      <div className="sidebar-foot">
        <div className="foot-line">
          <span className="foot-key">控制台</span>
          <span className="foot-val">127.0.0.1:{health?.port ?? '—'}</span>
        </div>
        <div className="foot-line">
          <span className="foot-key">版本</span>
          <span className="foot-val">v{health?.version ?? '1.0.0'}</span>
        </div>
        <div className="foot-hint">左键托盘图标打开本页面 · 右键可退出</div>
      </div>
    </aside>
  )
}
