<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { eventLabel } from '../utils/describe'
import PageHeader from '../components/PageHeader.vue'
import HelpNote from '../components/HelpNote.vue'
import UiModal from '../components/UiModal.vue'
import Icon from '../components/Icon.vue'
import UiField from '../components/ui/UiField.vue'
import UiSelect from '../components/ui/UiSelect.vue'
import UiSegmented from '../components/ui/UiSegmented.vue'
import UiToggle from '../components/ui/UiToggle.vue'

const ui = useUI()
const items = ref([])
const loading = ref(true)
const showModal = ref(false)
const editing = ref(false)
const saving = ref(false)
const testing = ref(false)

const ALL_EVENTS = ['start', 'success', 'failure']
const TYPES = [
  { value: 'discord', label: 'Discord 机器人', icon: 'discord' },
  { value: 'telegram', label: 'Telegram 机器人', icon: 'telegram' },
  { value: 'smtp', label: '邮件 (SMTP)', icon: 'mail' },
  { value: 'webhook', label: 'Webhook', icon: 'webhook' },
]
const typeMeta = {
  discord: { label: 'Discord', icon: 'discord' },
  telegram: { label: 'Telegram', icon: 'telegram' },
  smtp: { label: '邮件', icon: 'mail' },
  webhook: { label: 'Webhook', icon: 'webhook' },
}
const METHODS = [{ value: 'POST', label: 'POST' }, { value: 'PUT', label: 'PUT' }]

const blank = () => ({
  id: '', name: '', type: 'discord', enabled: true, events: ['success', 'failure'],
  config: {
    bot_token: '', guild_id: '', channel_id: '', tg_bot_token: '', tg_chat_id: '',
    smtp_host: '', smtp_port: 465, smtp_user: '', smtp_pass: '', smtp_from: '', smtp_to: [], smtp_tls: true,
    webhook_url: '', webhook_method: 'POST', webhook_headers: {},
  },
})
const form = reactive(blank())
const canSave = computed(() => form.name.trim().length > 0)
const showBody = ref(false)

const smtpToText = computed({
  get: () => (form.config.smtp_to || []).join(', '),
  set: (v) => { form.config.smtp_to = v.split(/[,\n]/).map((x) => x.trim()).filter(Boolean) },
})
const headersText = computed({
  get: () => Object.entries(form.config.webhook_headers || {}).map(([k, v]) => `${k}: ${v}`).join('\n'),
  set: (v) => {
    const o = {}
    v.split('\n').forEach((line) => { const i = line.indexOf(':'); if (i > 0) o[line.slice(0, i).trim()] = line.slice(i + 1).trim() })
    form.config.webhook_headers = o
  },
})
function toggleEvent(e) { const i = form.events.indexOf(e); i < 0 ? form.events.push(e) : form.events.splice(i, 1) }

async function load() {
  loading.value = true
  try { items.value = (await api.get('/notifiers')).data || [] }
  catch (e) { ui.err(errMsg(e)) }
  finally { loading.value = false }
}
onMounted(load)

function openNew() { Object.assign(form, blank()); editing.value = false; showModal.value = true }
function openEdit(n) {
  Object.assign(form, blank())
  form.id = n.id; form.name = n.name; form.type = n.type; form.enabled = n.enabled; form.events = [...(n.events || [])]
  Object.assign(form.config, n.config || {})
  form.config.smtp_to = n.config?.smtp_to || []
  form.config.webhook_headers = {}
  form.config.bot_token = ''; form.config.tg_bot_token = ''; form.config.smtp_pass = ''
  editing.value = true; showModal.value = true
}
function payload() { return { name: form.name, type: form.type, enabled: form.enabled, events: form.events, config: { ...form.config } } }

async function save() {
  saving.value = true
  try {
    if (editing.value) await api.put(`/notifiers/${form.id}`, payload())
    else await api.post('/notifiers', payload())
    ui.ok(editing.value ? '通知渠道已更新' : '通知渠道已创建')
    showModal.value = false; load()
  } catch (e) { ui.err(errMsg(e, '保存失败')) }
  finally { saving.value = false }
}
async function test() {
  testing.value = true
  try { const { data } = await api.post('/notifiers/test', { id: form.id, ...payload() }); data.ok ? ui.ok('测试通知已发送') : ui.err(`发送失败：${data.error || '未知错误'}`) }
  catch (e) { ui.err(errMsg(e, '测试失败')) }
  finally { testing.value = false }
}
async function testExisting(n) {
  try { const { data } = await api.post(`/notifiers/${n.id}/test`); data.ok ? ui.ok(`${n.name}：已发送`) : ui.err(`${n.name}：${data.error || '发送失败'}`) }
  catch (e) { ui.err(errMsg(e)) }
}
async function remove(n) {
  if (!(await ui.confirm({ title: '删除通知渠道', message: `确认删除通知渠道「${n.name}」？`, confirmText: '删除', danger: true }))) return
  try { await api.delete(`/notifiers/${n.id}`); ui.ok('已删除'); load() }
  catch (e) { ui.err(errMsg(e, '删除失败')) }
}
</script>

<template>
  <PageHeader eyebrow="通知" title="通知渠道" lede="备份开始、成功或失败时把结果推送到你常用的地方——聊天机器人、邮箱或自建接口。">
    <template #actions>
      <button class="btn btn-primary" @click="openNew"><Icon name="plus" :size="17" /> 新建通知</button>
    </template>
  </PageHeader>

  <HelpNote title="会推送哪些内容？">
    通知包含<b>目标名、状态、耗时、归档大小、逐渠道结果</b>与失败时的错误详情。支持
    <b>Discord</b>（Bot Token + 群组/频道 ID）、<b>Telegram</b>、<b>SMTP 邮件</b>与 <b>Webhook</b>。可为每个渠道选择订阅哪些事件。
  </HelpNote>

  <div class="card">
    <div v-if="loading" class="empty"><span class="spinner" /></div>
    <div v-else-if="!items.length" class="empty">
      <span class="empty-ico"><Icon name="bell" :size="24" /></span>
      <h3>还没有通知渠道</h3>
      <p>添加一个渠道，及时获知每次备份的结果。</p>
      <button class="btn btn-primary" style="margin-top:14px" @click="openNew"><Icon name="plus" :size="17" /> 新建通知</button>
    </div>
    <div v-else class="table-wrap">
      <table class="table">
        <thead><tr><th>名称</th><th>类型</th><th>订阅事件</th><th>状态</th><th /></tr></thead>
        <TransitionGroup tag="tbody" name="list">
          <tr v-for="n in items" :key="n.id">
            <td><div class="inline"><span class="type-ico"><Icon :name="typeMeta[n.type]?.icon || 'bell'" :size="17" /></span><RouterLink :to="`/notifiers/${n.id}`" class="rowlink">{{ n.name }}</RouterLink></div></td>
            <td><span class="badge badge-accent">{{ typeMeta[n.type]?.label || n.type }}</span></td>
            <td><div class="tag-list"><span v-for="e in (n.events && n.events.length ? n.events : ALL_EVENTS)" :key="e" class="badge badge-neutral">{{ eventLabel[e] || e }}</span></div></td>
            <td><span v-if="n.enabled" class="badge badge-ok"><span class="dot dot-pulse" /> 启用</span><span v-else class="badge badge-neutral">停用</span></td>
            <td class="table-actions">
              <RouterLink :to="`/notifiers/${n.id}`" class="btn btn-sm icon-btn" title="详情"><Icon name="eye" :size="16" /></RouterLink>
              <button class="btn btn-sm icon-btn" title="发送测试" @click="testExisting(n)"><Icon name="activity" :size="16" /></button>
              <button class="btn btn-sm icon-btn" title="编辑" @click="openEdit(n)"><Icon name="edit" :size="16" /></button>
              <button class="btn btn-sm icon-btn btn-danger" title="删除" @click="remove(n)"><Icon name="trash" :size="16" /></button>
            </td>
          </tr>
        </TransitionGroup>
      </table>
    </div>
  </div>

  <UiModal :show="showModal" :title="editing ? '编辑通知渠道' : '新建通知渠道'" :subtitle="editing ? form.name : '选择平台并填写推送目标'" @close="showModal = false">
    <div class="row">
      <UiField label="名称"><input class="input" v-model="form.name" type="text" placeholder="例如：运维告警" /></UiField>
      <UiField label="类型"><UiSelect v-model="form.type" :options="TYPES" /></UiField>
    </div>

    <UiField label="订阅事件" hint="留空表示接收全部事件。">
      <div class="chips">
        <button v-for="e in ALL_EVENTS" :key="e" type="button" class="chip" :class="{ on: form.events.includes(e) }" @click="toggleEvent(e)">
          <Icon v-if="form.events.includes(e)" name="check" :size="13" :stroke="2.6" />{{ eventLabel[e] }}
        </button>
      </div>
    </UiField>

    <template v-if="form.type === 'discord'">
      <UiField label="Bot Token" :hint="editing ? '留空保持不变' : ''"><input class="input" v-model="form.config.bot_token" type="password" autocomplete="new-password" :placeholder="editing ? '留空保持不变' : ''" /></UiField>
      <div class="row">
        <UiField label="群组 ID (Guild)" help="服务器 ID，仅作记录。"><input class="input" v-model="form.config.guild_id" type="text" /></UiField>
        <UiField label="频道 ID (Channel)" help="机器人将消息发送到该频道，需有发言权限。"><input class="input" v-model="form.config.channel_id" type="text" /></UiField>
      </div>
    </template>

    <template v-else-if="form.type === 'telegram'">
      <UiField label="Bot Token" :hint="editing ? '留空保持不变' : ''"><input class="input" v-model="form.config.tg_bot_token" type="password" autocomplete="new-password" :placeholder="editing ? '留空保持不变' : ''" /></UiField>
      <UiField label="Chat ID" help="用户 / 群组 / 频道 ID。可向 @userinfobot 获取。"><input class="input" v-model="form.config.tg_chat_id" type="text" /></UiField>
    </template>

    <template v-else-if="form.type === 'smtp'">
      <div class="row">
        <UiField label="SMTP 主机"><input class="input" v-model="form.config.smtp_host" type="text" placeholder="smtp.example.com" /></UiField>
        <UiField label="端口" help="465 = 隐式 TLS；587/25 走 STARTTLS。">
          <input class="input" v-model.number="form.config.smtp_port" type="number" style="max-width:120px" />
        </UiField>
      </div>
      <div class="row">
        <UiField label="用户名"><input class="input" v-model="form.config.smtp_user" type="text" autocomplete="off" /></UiField>
        <UiField label="密码" :hint="editing ? '留空保持不变' : ''"><input class="input" v-model="form.config.smtp_pass" type="password" autocomplete="new-password" :placeholder="editing ? '留空保持不变' : ''" /></UiField>
      </div>
      <UiField label="发件人"><input class="input" v-model="form.config.smtp_from" type="text" placeholder="ArchiveSync <backup@example.com>" /></UiField>
      <UiField label="收件人" help="逗号分隔可填多个。"><input class="input" v-model="smtpToText" type="text" placeholder="ops@example.com, admin@example.com" /></UiField>
      <label class="switch-row"><UiToggle v-model="form.config.smtp_tls" /><span>使用隐式 TLS（端口 465）<span class="faint"> · 否则尝试 STARTTLS</span></span></label>
    </template>

    <template v-else>
      <div class="row">
        <UiField label="URL"><input class="input" v-model="form.config.webhook_url" type="url" placeholder="https://example.com/hook" /></UiField>
        <UiField label="方法"><UiSegmented v-model="form.config.webhook_method" :options="METHODS" /></UiField>
      </div>
      <UiField label="自定义 Header" help="每行 Key: Value。常用于携带鉴权令牌。" :hint="editing ? '出于安全，已保存的 Header 不回显；留空保持不变。' : ''">
        <textarea v-model="headersText" rows="3" :placeholder="editing ? '留空保持不变' : 'Authorization: Bearer xxx'" />
      </UiField>
      <div class="body-doc">
        <button type="button" class="btn btn-ghost btn-sm" @click="showBody = !showBody">
          <Icon :name="showBody ? 'chevronDown' : 'chevronRight'" :size="15" /> 查看请求体格式
        </button>
        <Transition name="expand">
          <div v-if="showBody">
            <p class="hint" style="margin:9px 0 8px">
              以 <b>{{ form.config.webhook_method || 'POST' }}</b> 发送下述 JSON。已兼容 Discord / Slack 的 Incoming Webhook（分别读取
              <code>content</code> / <code>text</code> 字段），也可对接自建接口读取结构化字段。
            </p>
            <pre class="code-block">{
  "type":      "success",           // start | success | failure
  "target":    "Nginx 配置",
  "title":     "备份成功：Nginx 配置",
  "message":   "已上传至 1 个渠道（1.2 MiB，24 个文件）",
  "content":   "…完整多行文本（供 Discord 使用）",
  "text":      "…同上（供 Slack 使用）",
  "timestamp": "2026-07-04T12:00:00Z",
  "fields":    { "触发方式": "手动触发" },
  "run": {
    "status": "success", "size_bytes": 1258291,
    "file_count": 24, "duration_ms": 850,
    "destinations": [
      { "channel_name": "R2-生产", "success": true, "pruned": 1 }
    ]
  }
}</pre>
          </div>
        </Transition>
      </div>
    </template>

    <hr class="divider" />
    <label class="switch-row"><UiToggle v-model="form.enabled" /><span>启用该通知渠道</span></label>

    <template #footer>
      <button class="btn" :disabled="testing" @click="test"><span v-if="testing" class="spinner" /><Icon v-else name="activity" :size="16" /> 发送测试</button>
      <div class="spacer" />
      <button class="btn btn-ghost" @click="showModal = false">取消</button>
      <button class="btn btn-primary" :disabled="saving || !canSave" @click="save"><span v-if="saving" class="spinner" /> 保存</button>
    </template>
  </UiModal>
</template>

<style scoped>
.type-ico { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; background: var(--accent-soft); color: var(--accent); flex: 0 0 32px; }
.switch-row { display: flex; align-items: center; gap: 11px; cursor: pointer; font-size: 13.5px; }
.chips { display: flex; gap: 8px; flex-wrap: wrap; }
.chip { display: inline-flex; align-items: center; gap: 5px; padding: 7px 13px; border-radius: var(--r-pill); border: 1px solid var(--border-strong); background: var(--bg); color: var(--text-muted); font: inherit; font-weight: 530; cursor: pointer; transition: all var(--dur-fast); }
.chip:hover { border-color: var(--text-faint); }
.chip.on { background: var(--accent-soft); border-color: var(--accent); color: var(--accent); }
.body-doc { margin-top: 4px; }
.code-block { margin: 0; padding: 14px 16px; border-radius: var(--r-md); background: var(--bg-sunken); border: 1px solid var(--border); font-family: var(--font-mono); font-size: 12px; line-height: 1.6; color: var(--text-muted); overflow-x: auto; white-space: pre; }
</style>
