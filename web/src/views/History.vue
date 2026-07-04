<script setup>
import { ref, onMounted } from 'vue'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { humanBytes, fmtTime, fmtDuration } from '../utils/format'
import PageHeader from '../components/PageHeader.vue'
import HelpNote from '../components/HelpNote.vue'
import UiModal from '../components/UiModal.vue'
import Icon from '../components/Icon.vue'
import StatusBadge from '../components/StatusBadge.vue'

const ui = useUI()
const runs = ref([])
const loading = ref(true)
const selected = ref(null)
const showDetail = ref(false)

async function load() {
  loading.value = true
  try { runs.value = (await api.get('/runs', { params: { limit: 200 } })).data || [] }
  catch (e) { ui.err(errMsg(e)) }
  finally { loading.value = false }
}
onMounted(load)

function open(r) { selected.value = r; showDetail.value = true }
const triggerLabel = (t) => (t === 'manual' ? '手动' : '定时')
</script>

<template>
  <PageHeader eyebrow="记录" title="备份历史" lede="每一次备份运行的结果、体积、耗时与逐渠道详情，最近 200 条。">
    <template #actions>
      <button class="btn" @click="load"><Icon name="refresh" :size="16" /> 刷新</button>
    </template>
  </PageHeader>

  <HelpNote title="如何阅读记录？">
    <b>状态</b>反映整体结果：全部渠道成功为「成功」，部分成功为「部分成功」，全部失败为「失败」。点击「详情」可查看每个渠道的上传结果、归档 Key 与清理数量。
  </HelpNote>

  <div class="card">
    <div v-if="loading" class="empty"><span class="spinner" /></div>
    <div v-else-if="!runs.length" class="empty">
      <span class="empty-ico"><Icon name="history" :size="24" /></span>
      <h3>暂无备份记录</h3>
      <p>运行一次备份后，记录会出现在这里。</p>
    </div>
    <div v-else class="table-wrap">
      <table class="table">
        <thead><tr><th>时间</th><th>目标</th><th>状态</th><th>大小</th><th>文件</th><th>耗时</th><th>触发</th><th /></tr></thead>
        <TransitionGroup tag="tbody" name="list">
          <tr v-for="r in runs" :key="r.id">
            <td class="muted">{{ fmtTime(r.started_at) }}</td>
            <td class="cell-title">{{ r.target_name }}</td>
            <td><StatusBadge :status="r.status" /></td>
            <td class="mono tnum">{{ humanBytes(r.size_bytes) }}</td>
            <td class="muted tnum">{{ r.file_count }}</td>
            <td class="muted tnum">{{ fmtDuration(r.duration_ms) }}</td>
            <td><span class="badge badge-neutral">{{ triggerLabel(r.trigger) }}</span></td>
            <td class="table-actions"><button class="btn btn-sm" @click="open(r)">详情</button></td>
          </tr>
        </TransitionGroup>
      </table>
    </div>
  </div>

  <UiModal :show="showDetail" title="备份详情" :subtitle="selected?.target_name" dismissible @close="showDetail = false">
    <template v-if="selected">
      <dl class="kv">
        <dt>状态</dt><dd><StatusBadge :status="selected.status" /></dd>
        <dt>开始 / 结束</dt><dd>{{ fmtTime(selected.started_at) }} → {{ fmtTime(selected.finished_at) }}</dd>
        <dt>耗时</dt><dd>{{ fmtDuration(selected.duration_ms) }}</dd>
        <dt>大小 / 文件</dt><dd>{{ humanBytes(selected.size_bytes) }} · {{ selected.file_count }} 个文件</dd>
        <dt>归档 Key</dt><dd class="mono">{{ selected.archive_key || '—' }}</dd>
        <dt>触发方式</dt><dd>{{ triggerLabel(selected.trigger) }}</dd>
        <dt>信息</dt><dd>{{ selected.message || '—' }}</dd>
      </dl>

      <div class="section-title" style="margin-top:20px"><span class="num">·</span> 目的渠道结果</div>
      <div v-if="!selected.destinations?.length" class="hint">无</div>
      <div v-else class="dest-list">
        <div v-for="(d, i) in selected.destinations" :key="i" class="dest">
          <span class="dest-ico" :class="d.success ? 'ok' : 'err'"><Icon :name="d.success ? 'checkCircle' : 'xCircle'" :size="18" /></span>
          <div class="dest-body">
            <div class="dest-name">{{ d.channel_name }}</div>
            <div class="dest-sub" :class="{ err: !d.success }">{{ d.success ? `已上传 · 清理 ${d.pruned || 0} 个旧版本` : (d.error || '失败') }}</div>
          </div>
        </div>
      </div>
    </template>
    <template #footer><button class="btn btn-ghost" @click="showDetail = false">关闭</button></template>
  </UiModal>
</template>

<style scoped>
.dest-list { display: flex; flex-direction: column; gap: 8px; }
.dest { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg-sunken); }
.dest-ico.ok { color: var(--ok); }
.dest-ico.err { color: var(--danger); }
.dest-name { font-weight: 560; }
.dest-sub { font-size: 12.5px; color: var(--text-muted); margin-top: 1px; }
.dest-sub.err { color: var(--danger); }
</style>
