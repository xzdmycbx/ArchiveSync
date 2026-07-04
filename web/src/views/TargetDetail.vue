<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { humanBytes, fmtTime, fmtDuration } from '../utils/format'
import { scheduleDesc, retentionDesc } from '../utils/describe'
import Icon from '../components/Icon.vue'
import StatusBadge from '../components/StatusBadge.vue'
import UiModal from '../components/UiModal.vue'
import UiSegmented from '../components/ui/UiSegmented.vue'
import ObjectBrowser from '../components/ObjectBrowser.vue'

const route = useRoute()
const ui = useUI()
const id = route.params.id
const loading = ref(true)
const target = ref(null)
const channels = ref([])
const notifiers = ref([])
const runs = ref([])
const selectedChannel = ref('')
const selectedRun = ref(null)
const showRun = ref(false)

const myChannels = computed(() => (target.value?.channel_ids || []).map((cid) => channels.value.find((c) => c.id === cid)).filter(Boolean))
const myNotifiers = computed(() => (target.value?.notifier_ids || []).map((nid) => notifiers.value.find((n) => n.id === nid)).filter(Boolean))
const channelTabs = computed(() => myChannels.value.map((c) => ({ value: c.id, label: c.name })))

async function load() {
  loading.value = true
  try {
    const [t, c, n, r] = await Promise.all([
      api.get(`/targets/${id}`),
      api.get('/channels'),
      api.get('/notifiers'),
      api.get('/runs', { params: { target: id, limit: 50 } }),
    ])
    target.value = t.data
    channels.value = c.data || []
    notifiers.value = n.data || []
    runs.value = r.data || []
    if (myChannels.value.length) selectedChannel.value = myChannels.value[0].id
  } catch (e) { ui.err(errMsg(e, '加载目标失败')) }
  finally { loading.value = false }
}
onMounted(load)

async function runNow() {
  try { await api.post(`/targets/${id}/run`); ui.ok('已触发备份'); setTimeout(load, 1500) }
  catch (e) { e?.response?.status === 409 ? ui.err('该目标正在备份中') : ui.err(errMsg(e, '触发失败')) }
}
function openRun(r) { selectedRun.value = r; showRun.value = true }
const triggerLabel = (t) => (t === 'manual' ? '手动' : '定时')
</script>

<template>
  <div v-if="loading" class="empty"><span class="spinner" /></div>
  <template v-else-if="target">
    <div class="detail-head rise">
      <RouterLink to="/targets" class="btn btn-ghost btn-sm"><Icon name="back" :size="16" /> 备份目标</RouterLink>
      <div class="dh-main">
        <h1>{{ target.name }}</h1>
        <div class="dh-badges">
          <span class="badge badge-neutral mono">{{ target.key }}</span>
          <span v-if="target.enabled" class="badge badge-ok"><span class="dot" /> 启用</span>
          <span v-else class="badge badge-neutral">停用</span>
        </div>
      </div>
      <div class="spacer" />
      <button class="btn btn-primary" @click="runNow"><Icon name="play" :size="16" /> 立即备份</button>
    </div>

    <div class="grid grid-2" style="margin-bottom:18px">
      <div class="card card-pad">
        <div class="section-title"><Icon name="target" :size="15" /> 概览</div>
        <dl class="kv">
          <dt>源目录</dt><dd class="mono">{{ target.source_path }}</dd>
          <dt>调度</dt><dd>{{ scheduleDesc(target.schedule) }}</dd>
          <dt>保留策略</dt><dd>{{ retentionDesc(target.retention) }}</dd>
          <dt>存储路径</dt><dd class="mono">{{ target.key }}/&lt;日期&gt;/&lt;时间&gt;.{{ target.archive?.format || 'tar.gz' }}</dd>
        </dl>
      </div>
      <div class="card card-pad">
        <div class="section-title"><Icon name="cloud" :size="15" /> 分发</div>
        <div class="dl-block">
          <div class="dl-label">备份渠道</div>
          <div class="tag-list"><RouterLink v-for="c in myChannels" :key="c.id" :to="`/channels/${c.id}`" class="badge badge-accent">{{ c.name }}</RouterLink><span v-if="!myChannels.length" class="faint">无</span></div>
        </div>
        <div class="dl-block">
          <div class="dl-label">通知渠道</div>
          <div class="tag-list"><RouterLink v-for="n in myNotifiers" :key="n.id" :to="`/notifiers/${n.id}`" class="badge badge-neutral">{{ n.name }}</RouterLink><span v-if="!myNotifiers.length" class="faint">无</span></div>
        </div>
      </div>
    </div>

    <div class="card" style="margin-bottom:18px">
      <div class="card-head"><div><h3>备份文件</h3><div class="sub">按日期浏览并下载已存储的归档</div></div>
        <div v-if="channelTabs.length > 1"><UiSegmented v-model="selectedChannel" :options="channelTabs" /></div>
      </div>
      <div class="card-pad">
        <ObjectBrowser v-if="selectedChannel" :channel-id="selectedChannel" :root-prefix="`${target.key}/`" root-label="按日期" />
        <div v-else class="empty faint">该目标未配置渠道</div>
      </div>
    </div>

    <div class="card">
      <div class="card-head"><div><h3>最近执行结果</h3><div class="sub">点击查看每次运行的日志与逐渠道结果</div></div>
        <button class="btn btn-sm btn-ghost icon-btn" title="刷新" @click="load"><Icon name="refresh" :size="15" /></button>
      </div>
      <div v-if="!runs.length" class="empty"><span class="empty-ico"><Icon name="history" :size="22" /></span><h3>暂无执行记录</h3></div>
      <div v-else class="table-wrap">
        <table class="table">
          <thead><tr><th>时间</th><th>状态</th><th>大小</th><th>文件</th><th>耗时</th><th>触发</th><th /></tr></thead>
          <tbody>
            <tr v-for="r in runs" :key="r.id">
              <td class="muted">{{ fmtTime(r.started_at) }}</td>
              <td><StatusBadge :status="r.status" /></td>
              <td class="mono tnum">{{ humanBytes(r.size_bytes) }}</td>
              <td class="muted tnum">{{ r.file_count }}</td>
              <td class="muted tnum">{{ fmtDuration(r.duration_ms) }}</td>
              <td><span class="badge badge-neutral">{{ triggerLabel(r.trigger) }}</span></td>
              <td class="table-actions"><button class="btn btn-sm" @click="openRun(r)">日志</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <UiModal :show="showRun" title="执行日志" :subtitle="selectedRun ? fmtTime(selectedRun.started_at) : ''" dismissible @close="showRun = false">
      <template v-if="selectedRun">
        <dl class="kv">
          <dt>状态</dt><dd><StatusBadge :status="selectedRun.status" /></dd>
          <dt>开始 / 结束</dt><dd>{{ fmtTime(selectedRun.started_at) }} → {{ fmtTime(selectedRun.finished_at) }}</dd>
          <dt>耗时</dt><dd>{{ fmtDuration(selectedRun.duration_ms) }}</dd>
          <dt>大小 / 文件</dt><dd>{{ humanBytes(selectedRun.size_bytes) }} · {{ selectedRun.file_count }} 个文件</dd>
          <dt>归档 Key</dt><dd class="mono">{{ selectedRun.archive_key || '—' }}</dd>
          <dt>信息</dt><dd>{{ selectedRun.message || '—' }}</dd>
        </dl>
        <div class="section-title" style="margin-top:18px"><span class="num">·</span> 逐渠道结果</div>
        <div v-if="!selectedRun.destinations?.length" class="hint">无</div>
        <div v-else class="dest-list">
          <div v-for="(d, i) in selectedRun.destinations" :key="i" class="dest">
            <span class="dest-ico" :class="d.success ? 'ok' : 'err'"><Icon :name="d.success ? 'checkCircle' : 'xCircle'" :size="18" /></span>
            <div><div style="font-weight:560">{{ d.channel_name }}</div><div class="faint" style="font-size:12.5px" :class="{ 'err-text': !d.success }">{{ d.success ? `已上传 · 清理 ${d.pruned || 0} 个旧版本` : (d.error || '失败') }}</div></div>
          </div>
        </div>
      </template>
      <template #footer><button class="btn btn-ghost" @click="showRun = false">关闭</button></template>
    </UiModal>
  </template>

  <div v-else class="empty"><h3>目标不存在</h3><RouterLink to="/targets" class="btn btn-primary btn-sm" style="margin-top:12px">返回列表</RouterLink></div>
</template>

<style scoped>
.detail-head { display: flex; align-items: center; gap: 16px; margin-bottom: 22px; flex-wrap: wrap; }
.dh-main { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.dh-main h1 { font-size: 22px; letter-spacing: -0.02em; }
.dh-badges { display: flex; gap: 6px; align-items: center; }
.dl-block { margin-top: 12px; }
.dl-block:first-of-type { margin-top: 4px; }
.dl-label { font-size: 12px; color: var(--text-muted); margin-bottom: 6px; }
a.badge { text-decoration: none; }
.dest-list { display: flex; flex-direction: column; gap: 8px; }
.dest { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg-sunken); }
.dest-ico.ok { color: var(--ok); }
.dest-ico.err { color: var(--danger); }
.err-text { color: var(--danger) !important; }
</style>
