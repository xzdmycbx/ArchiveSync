<script setup>
import { computed } from 'vue'
import Icon from './Icon.vue'

// A visual explainer for the retention policy: a compact ladder of recent days
// showing, per day, how much of that day's backups survive (accent fill) vs is
// pruned. Illustrative — mirrors the backend semantics closely.
const props = defineProps({
  policy: { type: Object, default: () => ({}) },
  perDay: { type: Number, default: 24 },
})

const m = computed(() => {
  const p = props.policy || {}
  const perDay = Math.max(1, Number(props.perDay) || 24)
  const keepAll = Math.max(0, Number(p.keep_all_days) || 0)
  const anchors = (p.daily_anchors || []).filter(Boolean)
  const anchorN = anchors.length || 1
  const keepDays = Math.max(0, Number(p.keep_days) || 0)
  const maxV = Math.max(0, Number(p.max_versions) || 0)

  const totalDays = keepDays > 0 ? keepDays : Math.max(keepAll, 1)
  const showN = Math.min(totalDays, 6)
  const rows = []
  for (let d = 0; d < showN; d++) {
    const full = d < keepAll
    const kept = full ? perDay : anchorN
    rows.push({
      label: d === 0 ? '今天' : `${d} 天前`,
      full, kept, total: perDay,
      pct: full ? 100 : Math.min(100, Math.max(10, Math.round((kept / perDay) * 100))),
    })
  }
  const moreDays = totalDays > showN ? totalDays - showN : 0

  let est = keepAll * perDay + Math.max(0, totalDays - keepAll) * anchorN
  let capped = false
  if (maxV > 0 && est > maxV) { est = maxV; capped = true }

  const simple = keepAll === 0 && anchors.length === 0 && keepDays === 0 && maxV > 0

  return { perDay, keepAll, anchorN, keepDays, maxV, rows, moreDays, est, capped, simple, hasOlder: keepDays > 0 }
})
</script>

<template>
  <div class="rp">
    <div class="rp-head">
      <span class="rp-title"><Icon name="layers" :size="15" /> 保留效果预览</span>
      <span class="rp-est">共约 <b class="tnum">{{ m.est }}</b> 份<span v-if="m.capped" class="rp-cap"> · 受上限</span></span>
    </div>

    <div v-if="m.simple" class="rp-simple">
      <div class="rp-simple-bar"><span class="fill" /></div>
      <p>只按数量保留：始终保留最近的 <b class="tnum">{{ m.maxV }}</b> 份，更旧的自动删除。</p>
    </div>

    <div v-else class="rp-rows">
      <div v-for="(r, i) in m.rows" :key="i" class="rp-row">
        <span class="rp-day">{{ r.label }}</span>
        <span class="rp-bar"><span class="fill" :class="{ full: r.full }" :style="{ width: r.pct + '%' }" /></span>
        <span class="rp-num" :class="{ hl: r.full }">{{ r.full ? '全部 ' + r.total : r.kept }}</span>
      </div>
      <div v-if="m.moreDays" class="rp-row muted-row">
        <span class="rp-day">…</span>
        <span class="rp-bar"><span class="fill" style="width:10%" /></span>
        <span class="rp-num">再 {{ m.moreDays }} 天</span>
      </div>
      <div v-if="m.hasOlder" class="rp-row">
        <span class="rp-day">更早</span>
        <span class="rp-bar drop" />
        <span class="rp-num del">删除</span>
      </div>
    </div>

    <div class="rp-legend">
      <span><i class="lg kept" /> 保留</span>
      <span><i class="lg pruned" /> 删除</span>
      <span class="faint">按 {{ perDay }} 次/天估算</span>
    </div>
  </div>
</template>

<style scoped>
.rp { border: 1px solid var(--border); border-radius: var(--r-lg); background: var(--bg-elev); padding: 16px 18px; }
.rp-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 14px; }
.rp-title { display: inline-flex; align-items: center; gap: 7px; font-weight: 620; font-size: 13.5px; color: var(--text); }
.rp-title :deep(.ico) { color: var(--accent); }
.rp-est { font-size: 13px; color: var(--text-muted); white-space: nowrap; }
.rp-est b { color: var(--accent); font-size: 15px; }
.rp-cap { color: var(--warn); }

.rp-simple { display: flex; flex-direction: column; gap: 10px; }
.rp-simple-bar { height: 18px; border-radius: 999px; background: var(--bg-sunken); overflow: hidden; }
.rp-simple-bar .fill { display: block; height: 100%; width: 100%; background: var(--brand-grad); }
.rp-simple p { margin: 0; font-size: 13px; color: var(--text-muted); line-height: 1.6; }
.rp-simple b { color: var(--accent); }

.rp-rows { display: flex; flex-direction: column; gap: 9px; }
.rp-row { display: grid; grid-template-columns: 52px 1fr 62px; align-items: center; gap: 12px; }
.rp-day { font-size: 12px; color: var(--text-muted); white-space: nowrap; }
.rp-bar { position: relative; height: 18px; border-radius: 999px; background: var(--bg-sunken); overflow: hidden; }
.rp-bar .fill { position: absolute; inset: 0 auto 0 0; border-radius: 999px; background: color-mix(in srgb, var(--accent) 55%, transparent); transition: width var(--dur) var(--ease); }
.rp-bar .fill.full { background: var(--brand-grad); }
.rp-bar.drop { background: repeating-linear-gradient(45deg, var(--bg-sunken), var(--bg-sunken) 5px, transparent 5px, transparent 9px); border: 1px dashed var(--border-strong); }
.rp-num { font-size: 12px; color: var(--text-muted); text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.rp-num.hl { color: var(--accent); font-weight: 600; }
.rp-num.del { color: var(--text-faint); }
.muted-row .rp-day, .muted-row .rp-num { color: var(--text-faint); }

.rp-legend { display: flex; align-items: center; gap: 16px; margin-top: 14px; padding-top: 13px; border-top: 1px dashed var(--border); font-size: 11.5px; color: var(--text-muted); }
.rp-legend span { display: inline-flex; align-items: center; gap: 6px; }
.lg { width: 11px; height: 11px; border-radius: 4px; display: inline-block; }
.lg.kept { background: var(--brand-grad); }
.lg.pruned { background: var(--bg-sunken); border: 1px dashed var(--border-strong); }
</style>
