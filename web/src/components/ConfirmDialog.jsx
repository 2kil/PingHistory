import { useEffect } from 'react'

export default function ConfirmDialog({
  open,
  title,
  message,
  confirmText = '确认',
  busy = false,
  onCancel,
  onConfirm,
}) {
  useEffect(() => {
    if (!open) return undefined
    const onKey = (event) => {
      if (event.key === 'Escape' && !busy) onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, busy, onCancel])

  if (!open) return null

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && !busy && onCancel()}>
      <div className="modal modal-sm" role="alertdialog" aria-modal="true">
        <div className="modal-head">
          <div>
            <h3>{title}</h3>
            <p>{message}</p>
          </div>
        </div>
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={busy}>
            取消
          </button>
          <button type="button" className="btn btn-danger" onClick={onConfirm} disabled={busy}>
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  )
}
