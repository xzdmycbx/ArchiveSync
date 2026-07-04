// Shared formatting helpers used across views.

export function humanBytes(n) {
  if (n == null || n === 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let i = 0
  let v = Math.abs(n)
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtTime(t) {
  if (!t) return '—'
  const d = new Date(t)
  if (isNaN(d)) return '—'
  return d.toLocaleString(undefined, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

export function fmtRelative(t) {
  if (!t) return '—'
  const d = new Date(t)
  if (isNaN(d)) return '—'
  const diff = (Date.now() - d.getTime()) / 1000
  const abs = Math.abs(diff)
  const fut = diff < 0
  const pick = (v, u) => `${Math.round(v)} ${u}${fut ? '后' : '前'}`
  if (abs < 60) return fut ? '即将' : '刚刚'
  if (abs < 3600) return pick(abs / 60, '分钟')
  if (abs < 86400) return pick(abs / 3600, '小时')
  if (abs < 2592000) return pick(abs / 86400, '天')
  return fmtTime(t)
}

export function fmtDuration(ms) {
  if (ms == null) return '—'
  if (ms < 1000) return `${ms} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)} s`
  const m = Math.floor(s / 60)
  const rs = Math.round(s % 60)
  if (m < 60) return `${m}m ${rs}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export const statusMeta = {
  success: { label: '成功', cls: 'badge-ok', icon: 'checkCircle' },
  partial: { label: '部分成功', cls: 'badge-warn', icon: 'alert' },
  failed: { label: '失败', cls: 'badge-err', icon: 'xCircle' },
  running: { label: '运行中', cls: 'badge-info', icon: 'refresh' },
  pending: { label: '等待中', cls: 'badge-neutral', icon: 'clock' },
}
