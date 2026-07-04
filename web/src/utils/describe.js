// Human-readable descriptions of schedule and retention policy objects,
// computed client-side so the dashboard/tables need no extra backend fields.

export function scheduleDesc(s) {
  if (!s) return '—'
  const tz = s.timezone ? ` · ${s.timezone}` : ''
  if (s.mode === 'cron') return `Cron ${s.cron || '—'}${tz}`
  if (s.mode === 'interval') return `每 ${s.interval_min || '?'} 分钟`
  if (s.mode === 'times') {
    if (s.times && s.times.length) return `每天 ${s.times.join(' / ')}${tz}`
    if (s.times_per_day) return `每天 ${s.times_per_day} 次${tz}`
  }
  return '—'
}

export function retentionDesc(r) {
  if (!r) return '—'
  const parts = []
  if (r.keep_all_days) parts.push(`近 ${r.keep_all_days} 天全量`)
  if (r.daily_anchors && r.daily_anchors.length) parts.push(`每日 ${r.daily_anchors.join('/')}`)
  if (r.keep_days) parts.push(`共 ${r.keep_days} 天`)
  if (r.max_versions) parts.push(`≤ ${r.max_versions} 份`)
  return parts.length ? parts.join(' · ') : '保留全部'
}

export const channelTypeLabel = { s3: 'S3 / R2', local: '本地目录' }
export const notifierTypeLabel = {
  discord: 'Discord', telegram: 'Telegram', smtp: '邮件 (SMTP)', webhook: 'Webhook',
}
export const eventLabel = { start: '开始', success: '成功', failure: '失败' }
