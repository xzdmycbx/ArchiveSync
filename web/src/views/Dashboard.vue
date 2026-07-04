<script setup>
import { ref, onMounted, computed } from 'vue'
import { RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { humanBytes, fmtRelative, fmtTime } from '../utils/format'
import { scheduleDesc } from '../utils/describe'
import PageHeader from '../components/PageHeader.vue'
import Icon from '../components/Icon.vue'
import StatusBadge from '../components/StatusBadge.vue'

const ui = useUI()
const loading = ref(true)
const data = ref(null)

async function load() {
  loading.value = true
  try { data.value = (await api.get('/status')).data }
  catch (e) { ui.err(errMsg(e, '加载状态失败')) }
  finally { loading.value = false }
}
onMounted(load)

const stats = computed(() => data.value?.stats || {})
const successRate = computed(() => {
  const s = stats.value
  return s.runs_total ? `${Math.round((s.success_rate || 0) * 100)}%` : '—'
})
const tiles = computed(() => {
  const s = stats.value
  return [
    { icon: 'target', label: '备份目标', value: s.enabled_targets ?? 0, sub: `共 ${s.targets ?? 0} 个，已启用` },
    { icon: 'checkCircle', label: '成功率', value: successRate.value, sub: `共 ${s.runs_total ?? 0} 次备份` },
    { icon: 'clock', label: '最近备份', value: s.last_backup_at ? fmtRelative(s.last_backup_at) : '—', sub: s.last_backup_at ? fmtTime(s.last_backup_at) : '暂无记录', small: true },
    { icon: 'archive', label: '总备份体积', value: humanBytes(s.total_size || 0), sub: `${s.channels ?? 0} 渠道 · ${s.notifiers ?? 0} 通知` },
  ]
})

async function runTarget(t) {
  try { await api.post(`/targets/${t.id}/run`); ui.ok(`已触发备份：${t.name}`); setTimeout(load, 1500) }
  catch (e) { ui.err(errMsg(e, '触发失败')) }
}
</script>

<template>
  <PageHeader eyebrow="总览" title="仪表盘" lede="一眼掌握所有备份目标的运行状况、下次调度与最近的备份结果。" />

  <div v-if="loading" class="empty"><span class="spinner" /></div>

  <template v-else-if="data">
    <div class="grid grid-4" style="margin-bottom:20px">
      <div v-for="(t, i) in tiles" :key="t.label" class="card stat rise" :style="{ animationDelay: i * 60 + 'ms' }">
        <div class="stat-top">
          <span class="label">{{ t.label }}</span>
          <span class="stat-ico"><Icon :name="t.icon" :size="18" /></span>
        </div>
        <div class="value" :style="t.small ? 'font-size:19px' : ''">{{ t.value }}</div>
        <div class="sub">{{ t.sub }}</div>
      </div>
    </div>

    <div class="card" style="margin-bottom:20px">
      <div class="card-head">
        <div><h3>备份目标</h3><div class="sub">按调度自动运行，或点击立即备份</div></div>
        <RouterLink to="/targets" class="btn btn-sm">管理目标 <Icon name="chevronRight" :size="15" /></RouterLink>
      </div>
      <div v-if="!data.targets?.length" class="empty">
        <span class="empty-ico"><Icon name="target" :size="24" /></span>
        <h3>还没有备份目标</h3>
        <p>创建第一个目标，把某个目录按计划备份到渠道。</p>
        <RouterLink to="/targets" class="btn btn-primary btn-sm" style="margin-top:14px"><Icon name="plus" :size="16" /> 新建目标</RouterLink>
      </div>
      <div v-else class="table-wrap">
        <table class="table">
          <thead><tr><th>名称</th><th>调度</th><th>下次运行</th><th>最近结果</th><th /></tr></thead>
          <tbody>
            <tr v-for="t in data.targets" :key="t.id">
              <td><div class="stack"><span class="cell-title">{{ t.name }}</span><span class="cell-sub mono">{{ t.source_path }}</span></div></td>
              <td class="muted">{{ scheduleDesc(t.schedule) }}</td>
              <td>
                <span v-if="!t.enabled" class="badge badge-neutral">已停用</span>
                <span v-else class="muted">{{ t.next_run ? fmtRelative(t.next_run) : '—' }}</span>
              </td>
              <td><StatusBadge v-if="t.last_run" :status="t.last_run.status" /><span v-else class="faint">从未运行</span></td>
              <td class="table-actions">
                <button class="btn btn-sm" :disabled="!t.enabled" @click="runTarget(t)"><Icon name="play" :size="14" /> 立即备份</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="card">
      <div class="card-head">
        <div><h3>最近备份</h3><div class="sub">最新的备份运行记录</div></div>
        <RouterLink to="/history" class="btn btn-sm">查看全部 <Icon name="chevronRight" :size="15" /></RouterLink>
      </div>
      <div v-if="!data.recent?.length" class="empty"><span class="empty-ico"><Icon name="history" :size="24" /></span><h3>暂无备份记录</h3></div>
      <div v-else class="table-wrap">
        <table class="table">
          <thead><tr><th>时间</th><th>目标</th><th>状态</th><th>大小</th><th>文件</th></tr></thead>
          <tbody>
            <tr v-for="r in data.recent" :key="r.id">
              <td class="muted">{{ fmtTime(r.started_at) }}</td>
              <td class="cell-title">{{ r.target_name }}</td>
              <td><StatusBadge :status="r.status" /></td>
              <td class="mono tnum">{{ humanBytes(r.size_bytes) }}</td>
              <td class="muted tnum">{{ r.file_count }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <p class="faint" style="text-align:center;font-size:12px;margin-top:22px">{{ data.version }}</p>
  </template>
</template>
