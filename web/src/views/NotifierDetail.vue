<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { fmtTime } from '../utils/format'
import { eventLabel, notifierTypeLabel } from '../utils/describe'
import Icon from '../components/Icon.vue'

const route = useRoute()
const ui = useUI()
const id = route.params.id
const loading = ref(true)
const notifier = ref(null)
const targets = ref([])
const testing = ref(false)

const typeIcon = { discord: 'discord', telegram: 'telegram', smtp: 'mail', webhook: 'webhook' }
const ALL_EVENTS = ['start', 'success', 'failure']

const usedBy = computed(() => targets.value.filter((t) => (t.notifier_ids || []).includes(id)))
const summary = computed(() => {
  const n = notifier.value
  if (!n) return []
  const c = n.config || {}
  switch (n.type) {
    case 'discord': return [['群组 ID', c.guild_id || '—'], ['频道 ID', c.channel_id || '—']]
    case 'telegram': return [['Chat ID', c.tg_chat_id || '—']]
    case 'smtp': return [['SMTP 主机', c.smtp_host], ['端口', c.smtp_port], ['发件人', c.smtp_from], ['收件人', (c.smtp_to || []).join(', ')], ['隐式 TLS', c.smtp_tls ? '开启' : '关闭']]
    case 'webhook': return [['URL', c.webhook_url], ['方法', c.webhook_method || 'POST']]
    default: return []
  }
})

async function load() {
  loading.value = true
  try {
    const [n, t] = await Promise.all([api.get(`/notifiers/${id}`), api.get('/targets')])
    notifier.value = n.data
    targets.value = t.data || []
  } catch (e) { ui.err(errMsg(e, '加载通知渠道失败')) }
  finally { loading.value = false }
}
onMounted(load)

async function test() {
  testing.value = true
  try { const { data } = await api.post(`/notifiers/${id}/test`); data.ok ? ui.ok('测试通知已发送') : ui.err(`发送失败：${data.error || '未知错误'}`) }
  catch (e) { ui.err(errMsg(e)) }
  finally { testing.value = false }
}
</script>

<template>
  <div v-if="loading" class="empty"><span class="spinner" /></div>
  <template v-else-if="notifier">
    <div class="detail-head rise">
      <RouterLink to="/notifiers" class="btn btn-ghost btn-sm"><Icon name="back" :size="16" /> 通知渠道</RouterLink>
      <div class="dh-main">
        <span class="type-ico"><Icon :name="typeIcon[notifier.type] || 'bell'" :size="18" /></span>
        <h1>{{ notifier.name }}</h1>
        <span class="badge badge-accent">{{ notifierTypeLabel[notifier.type] || notifier.type }}</span>
        <span v-if="notifier.enabled" class="badge badge-ok"><span class="dot" /> 启用</span>
        <span v-else class="badge badge-neutral">停用</span>
      </div>
      <div class="spacer" />
      <button class="btn btn-primary" :disabled="testing" @click="test"><span v-if="testing" class="spinner" /><Icon v-else name="activity" :size="16" /> 发送测试</button>
    </div>

    <div class="grid grid-2" style="margin-bottom:18px">
      <div class="card card-pad">
        <div class="section-title"><Icon name="bell" :size="15" /> 配置</div>
        <dl class="kv">
          <template v-for="row in summary" :key="row[0]"><dt>{{ row[0] }}</dt><dd class="mono">{{ row[1] }}</dd></template>
          <dt>创建时间</dt><dd>{{ fmtTime(notifier.created_at) }}</dd>
        </dl>
      </div>
      <div class="card card-pad">
        <div class="section-title"><Icon name="zap" :size="15" /> 订阅事件</div>
        <div class="tag-list" style="margin-bottom:14px">
          <span v-for="e in (notifier.events && notifier.events.length ? notifier.events : ALL_EVENTS)" :key="e" class="badge badge-neutral">{{ eventLabel[e] || e }}</span>
        </div>
        <p class="muted" style="font-size:13px;line-height:1.6">备份触发所选事件时，会推送包含<b>目标名、状态、大小、耗时与逐渠道结果</b>的通知。</p>
      </div>
    </div>

    <div class="card">
      <div class="card-head"><div><h3>使用此通知的目标</h3><div class="sub">这些备份目标会向该渠道推送结果</div></div></div>
      <div v-if="!usedBy.length" class="empty faint" style="padding:32px">还没有目标使用它。可在「备份目标」编辑时勾选。</div>
      <div v-else class="table-wrap">
        <table class="table">
          <thead><tr><th>目标</th><th>唯一标识</th><th>状态</th><th /></tr></thead>
          <tbody>
            <tr v-for="t in usedBy" :key="t.id">
              <td><RouterLink :to="`/targets/${t.id}`" class="rowlink">{{ t.name }}</RouterLink></td>
              <td><span class="badge badge-neutral mono">{{ t.key }}</span></td>
              <td><span v-if="t.enabled" class="badge badge-ok"><span class="dot" /> 启用</span><span v-else class="badge badge-neutral">停用</span></td>
              <td class="table-actions"><RouterLink :to="`/targets/${t.id}`" class="btn btn-sm icon-btn" title="详情"><Icon name="eye" :size="16" /></RouterLink></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </template>

  <div v-else class="empty"><h3>通知渠道不存在</h3><RouterLink to="/notifiers" class="btn btn-primary btn-sm" style="margin-top:12px">返回列表</RouterLink></div>
</template>

<style scoped>
.detail-head { display: flex; align-items: center; gap: 16px; margin-bottom: 22px; flex-wrap: wrap; }
.dh-main { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.dh-main h1 { font-size: 22px; letter-spacing: -0.02em; }
.type-ico { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 10px; background: var(--accent-soft); color: var(--accent); }
</style>
