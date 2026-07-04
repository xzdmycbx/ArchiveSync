<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { scheduleDesc, retentionDesc } from '../utils/describe'
import PageHeader from '../components/PageHeader.vue'
import HelpNote from '../components/HelpNote.vue'
import UiModal from '../components/UiModal.vue'
import FileBrowser from '../components/FileBrowser.vue'
import Icon from '../components/Icon.vue'
import RetentionPreview from '../components/RetentionPreview.vue'
import UiField from '../components/ui/UiField.vue'
import UiSelect from '../components/ui/UiSelect.vue'
import UiSegmented from '../components/ui/UiSegmented.vue'
import UiRadioCards from '../components/ui/UiRadioCards.vue'
import UiToggle from '../components/ui/UiToggle.vue'
import UiCheckCard from '../components/ui/UiCheckCard.vue'

const ui = useUI()
const items = ref([])
const channels = ref([])
const notifiers = ref([])
const loading = ref(true)
const showModal = ref(false)
const editing = ref(false)
const saving = ref(false)
const keyTouched = ref(false)
const showBrowser = ref(false)

const TZ = ['Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Tokyo', 'Asia/Singapore', 'UTC', 'Europe/London', 'America/New_York', 'America/Los_Angeles'].map((v) => ({ value: v, label: v }))
const MODES = [{ value: 'times', label: '每日定时' }, { value: 'cron', label: 'Cron' }, { value: 'interval', label: '固定间隔' }]
const FORMATS = [{ value: 'tar.gz', label: 'tar.gz' }, { value: 'zip', label: 'zip' }]
const RET_MODES = [
  { value: 'tiered', label: '分层保留（推荐）', desc: '今天全部保留，历史每天各留几份' },
  { value: 'days', label: '按天数', desc: '保留最近 N 天的备份' },
  { value: 'simple', label: '按份数', desc: '只保留最近 N 份备份' },
]
const DAYKEEP = [
  { value: 'all', label: '当天全部', desc: '这些天里每一次备份都保留' },
  { value: 'one', label: '仅最新一份', desc: '每天只保留最新的一份' },
]
const ANCHOR_MODES = [
  { value: 'one-midnight', label: '每天 00:00', desc: '保留最接近午夜的一份' },
  { value: 'two', label: '每天 12:00、24:00', desc: '中午与午夜各一份' },
  { value: 'latest', label: '每天最新一份', desc: '当天最后一次备份' },
  { value: 'custom', label: '自定义时刻', desc: '手动指定保留时刻' },
]
const chanIcon = (t) => (t === 'local' ? 'folder' : 'cloud')
const notiIcon = { discord: 'discord', telegram: 'telegram', smtp: 'mail', webhook: 'webhook' }

const blank = () => ({
  id: '', key: '', name: '', source_path: '', enabled: true,
  schedule: { mode: 'times', cron: '0 3 * * *', times: [], times_per_day: 24, interval_min: 60, timezone: 'Asia/Shanghai' },
  retention: { timezone: 'Asia/Shanghai', keep_all_days: 1, daily_anchors: ['00:00'], keep_days: 7, max_versions: 0, min_keep: 1 },
  channel_ids: [], notifier_ids: [],
  archive: { format: 'tar.gz', compression: 6, include: [], exclude: [] },
})
const form = reactive(blank())

// Friendly retention state that derives form.retention.
const ret = reactive({ mode: 'tiered', timezone: 'Asia/Shanghai', count: 7, days: 7, dayKeep: 'all', keepAllDays: 1, anchorMode: 'one-midnight', anchorsCustom: '00:00' })

function buildRetention() {
  const tz = ret.timezone
  if (ret.mode === 'simple') {
    form.retention = { timezone: tz, keep_all_days: 0, daily_anchors: [], keep_days: 0, max_versions: Math.max(1, ret.count || 1), min_keep: 1 }
  } else if (ret.mode === 'days') {
    const d = Math.max(1, ret.days || 1)
    form.retention = { timezone: tz, keep_all_days: ret.dayKeep === 'all' ? d : 0, daily_anchors: [], keep_days: d, max_versions: 0, min_keep: 1 }
  } else {
    let anchors
    if (ret.anchorMode === 'one-midnight') anchors = ['00:00']
    else if (ret.anchorMode === 'two') anchors = ['12:00', '24:00']
    else if (ret.anchorMode === 'latest') anchors = []
    else anchors = ret.anchorsCustom.split(/[,\n]/).map((s) => s.trim()).filter(Boolean)
    form.retention = { timezone: tz, keep_all_days: Math.max(1, ret.keepAllDays || 1), daily_anchors: anchors, keep_days: Math.max(1, ret.days || 1), max_versions: 0, min_keep: 1 }
  }
}
watch(ret, buildRetention, { deep: true })

function inferRetention(p) {
  ret.timezone = p.timezone || 'Asia/Shanghai'
  const anchors = p.daily_anchors || []
  const ka = p.keep_all_days || 0, kd = p.keep_days || 0, mv = p.max_versions || 0
  if (ka === 0 && anchors.length === 0 && kd === 0 && mv > 0) {
    ret.mode = 'simple'; ret.count = mv
  } else if (ka > 0 && anchors.length === 0 && kd > 0 && ka === kd) {
    ret.mode = 'days'; ret.dayKeep = 'all'; ret.days = kd
  } else if (ka === 0 && anchors.length === 0 && kd > 0) {
    ret.mode = 'days'; ret.dayKeep = 'one'; ret.days = kd
  } else {
    ret.mode = 'tiered'; ret.days = kd || 7; ret.keepAllDays = ka || 1
    if (anchors.length === 0) ret.anchorMode = 'latest'
    else if (anchors.length === 1 && anchors[0] === '00:00') ret.anchorMode = 'one-midnight'
    else if (anchors.length === 2 && anchors.includes('12:00')) ret.anchorMode = 'two'
    else { ret.anchorMode = 'custom'; ret.anchorsCustom = anchors.join(', ') }
  }
}

const retExplain = computed(() => {
  if (ret.mode === 'simple') return `始终保留最近的 ${ret.count || 1} 份备份，更旧的自动删除。`
  if (ret.mode === 'days') return `保留最近 ${ret.days || 1} 天；${ret.dayKeep === 'all' ? '这些天里的每一次备份都保留' : '每天只保留最新的一份'}，超过后删除。`
  const a = ret.anchorMode === 'latest' ? '每天保留最新一份'
    : ret.anchorMode === 'custom' ? `每天保留最接近 ${ret.anchorsCustom || '?'} 的备份`
      : ret.anchorMode === 'two' ? '每天保留最接近 12:00 与 24:00 的两份' : '每天保留最接近 00:00 的一份'
  return `最近 ${ret.keepAllDays || 1} 天的备份全部保留；更早的日子${a}；总共保留 ${ret.days || 1} 天。`
})

const timesText = computed({
  get: () => (form.schedule.times || []).join(', '),
  set: (v) => { form.schedule.times = v.split(/[,\n]/).map((x) => x.trim()).filter(Boolean) },
})
const excludeText = computed({
  get: () => (form.archive.exclude || []).join('\n'),
  set: (v) => { form.archive.exclude = v.split('\n').map((x) => x.trim()).filter(Boolean) },
})
const includeText = computed({
  get: () => (form.archive.include || []).join('\n'),
  set: (v) => { form.archive.include = v.split('\n').map((x) => x.trim()).filter(Boolean) },
})

const perDay = computed(() => {
  const s = form.schedule
  if (s.mode === 'times') return s.times?.length || Number(s.times_per_day) || 24
  if (s.mode === 'interval') return s.interval_min > 0 ? Math.max(1, Math.round(1440 / s.interval_min)) : 24
  return 24
})
const canSave = computed(() => form.name.trim() && form.key.trim() && form.source_path.trim() && form.channel_ids.length)

function slugify(s) {
  return (s || '').toLowerCase().trim().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64)
}
watch(() => form.name, (n) => { if (!editing.value && !keyTouched.value) form.key = slugify(n) })
function onKeyInput() { keyTouched.value = true; form.key = form.key.replace(/[^A-Za-z0-9._-]/g, '') }

async function load() {
  loading.value = true
  try {
    const [t, c, n] = await Promise.all([api.get('/targets'), api.get('/channels'), api.get('/notifiers')])
    items.value = t.data || []; channels.value = c.data || []; notifiers.value = n.data || []
  } catch (e) { ui.err(errMsg(e)) }
  finally { loading.value = false }
}
onMounted(load)
const chName = (id) => channels.value.find((c) => c.id === id)?.name || id

function openNew() {
  Object.assign(form, blank()); editing.value = false; keyTouched.value = false
  Object.assign(ret, { mode: 'tiered', timezone: 'Asia/Shanghai', count: 7, days: 7, dayKeep: 'all', keepAllDays: 1, anchorMode: 'one-midnight', anchorsCustom: '00:00' })
  buildRetention(); showModal.value = true
}
function openEdit(t) {
  Object.assign(form, blank())
  form.id = t.id; form.key = t.key || ''; form.name = t.name; form.source_path = t.source_path; form.enabled = t.enabled
  Object.assign(form.schedule, t.schedule || {}); form.schedule.times = t.schedule?.times || []
  form.channel_ids = [...(t.channel_ids || [])]; form.notifier_ids = [...(t.notifier_ids || [])]
  Object.assign(form.archive, t.archive || {}); form.archive.include = t.archive?.include || []; form.archive.exclude = t.archive?.exclude || []
  editing.value = true; keyTouched.value = true
  inferRetention(t.retention || {}); buildRetention()
  showModal.value = true
}
function toggleChan(id) { const i = form.channel_ids.indexOf(id); i < 0 ? form.channel_ids.push(id) : form.channel_ids.splice(i, 1) }
function toggleNoti(id) { const i = form.notifier_ids.indexOf(id); i < 0 ? form.notifier_ids.push(id) : form.notifier_ids.splice(i, 1) }

async function save() {
  if (!form.channel_ids.length) { ui.err('请至少选择一个备份渠道'); return }
  saving.value = true
  try {
    const body = JSON.parse(JSON.stringify(form))
    if (editing.value) await api.put(`/targets/${form.id}`, body)
    else await api.post('/targets', body)
    ui.ok(editing.value ? '目标已更新' : '目标已创建')
    showModal.value = false; load()
  } catch (e) { ui.err(errMsg(e, '保存失败')) }
  finally { saving.value = false }
}
async function runNow(t) {
  try { await api.post(`/targets/${t.id}/run`); ui.ok(`已触发备份：${t.name}`) }
  catch (e) { e?.response?.status === 409 ? ui.err('该目标正在备份中') : ui.err(errMsg(e, '触发失败')) }
}
async function remove(t) {
  if (!(await ui.confirm({ title: '删除备份目标', message: `确认删除目标「${t.name}」？其历史记录也会一并删除，且不可恢复。`, confirmText: '删除', danger: true }))) return
  try { await api.delete(`/targets/${t.id}`); ui.ok('目标已删除'); load() }
  catch (e) { ui.err(errMsg(e, '删除失败')) }
}
</script>

<template>
  <PageHeader eyebrow="配置" title="备份目标" lede="每个目标把一个源目录按计划打包，分发到一个或多个渠道，并按你设定的策略保留历史版本。">
    <template #actions>
      <button class="btn btn-primary" @click="openNew"><Icon name="plus" :size="17" /> 新建目标</button>
    </template>
  </PageHeader>

  <HelpNote title="备份目标如何工作？">
    到达<b>调度</b>时刻，ArchiveSync 会把<b>源目录</b>打包上传到所选<b>渠道</b>，随后按<b>保留策略</b>清理旧版本并向<b>通知渠道</b>推送结果。
    归档按目标的<b>唯一标识</b>存放为 <code>&lt;唯一标识&gt;/&lt;日期&gt;/&lt;时间&gt;.tar.gz</code>。
  </HelpNote>

  <div class="card">
    <div v-if="loading" class="empty"><span class="spinner" /></div>
    <div v-else-if="!items.length" class="empty">
      <span class="empty-ico"><Icon name="target" :size="24" /></span>
      <h3>还没有备份目标</h3>
      <p>创建第一个目标，把某个目录按计划备份到渠道。</p>
      <button class="btn btn-primary" style="margin-top:14px" @click="openNew"><Icon name="plus" :size="17" /> 新建目标</button>
    </div>
    <div v-else class="table-wrap">
      <table class="table">
        <thead><tr><th>名称</th><th>唯一标识</th><th>调度</th><th>保留策略</th><th>状态</th><th /></tr></thead>
        <TransitionGroup tag="tbody" name="list">
          <tr v-for="t in items" :key="t.id">
            <td><div class="stack"><RouterLink :to="`/targets/${t.id}`" class="cell-title rowlink">{{ t.name }}</RouterLink><span class="cell-sub mono">{{ t.source_path }}</span></div></td>
            <td><span class="badge badge-neutral mono">{{ t.key || '—' }}</span></td>
            <td class="muted">{{ scheduleDesc(t.schedule) }}</td>
            <td class="muted" style="font-size:12.5px">{{ retentionDesc(t.retention) }}</td>
            <td>
              <span v-if="t.enabled" class="badge badge-ok"><span class="dot dot-pulse" /> 启用</span>
              <span v-else class="badge badge-neutral">停用</span>
            </td>
            <td class="table-actions">
              <RouterLink :to="`/targets/${t.id}`" class="btn btn-sm icon-btn" title="详情"><Icon name="eye" :size="16" /></RouterLink>
              <button class="btn btn-sm icon-btn btn-primary" title="立即备份" @click="runNow(t)"><Icon name="play" :size="15" /></button>
              <button class="btn btn-sm icon-btn" title="编辑" @click="openEdit(t)"><Icon name="edit" :size="16" /></button>
              <button class="btn btn-sm icon-btn btn-danger" title="删除" @click="remove(t)"><Icon name="trash" :size="16" /></button>
            </td>
          </tr>
        </TransitionGroup>
      </table>
    </div>
  </div>

  <UiModal :show="showModal" :title="editing ? '编辑备份目标' : '新建备份目标'" :subtitle="editing ? form.name : '配置源目录、调度、保留与分发'" wide @close="showModal = false">
    <!-- 1 基本 -->
    <div class="section-title"><span class="num">01</span> 基本信息</div>
    <div class="row">
      <UiField label="名称"><input class="input" v-model="form.name" type="text" placeholder="例如：Nginx 配置" /></UiField>
      <UiField label="源目录" help="要备份的目录绝对路径，需运行 ArchiveSync 的用户可读。">
        <div class="path-input">
          <input class="input" v-model="form.source_path" type="text" placeholder="/etc/nginx" />
          <button type="button" class="btn" @click="showBrowser = true"><Icon name="folderOpen" :size="16" /> 浏览</button>
        </div>
      </UiField>
    </div>
    <UiField
      label="唯一标识 (ID)"
      :help="'该目标的固定身份，创建后不可修改。它就是存储目录名：<唯一标识>/日期/时间.tar.gz'"
      :hint="editing ? '唯一标识创建后不可修改。' : '字母、数字、下划线、连字符、点，1-64 位。建议用简短英文，如 nginx-conf。'"
    >
      <input class="input mono" v-model="form.key" type="text" :disabled="editing" placeholder="nginx-conf" @input="onKeyInput" />
    </UiField>

    <hr class="divider" />
    <!-- 2 调度 -->
    <div class="section-title"><span class="num">02</span> 调度</div>
    <p class="subhelp">决定何时自动运行。所有时间按所选时区计算。</p>
    <div class="row" style="align-items:flex-end">
      <UiField label="模式"><UiSegmented v-model="form.schedule.mode" :options="MODES" /></UiField>
      <UiField label="时区"><UiSelect v-model="form.schedule.timezone" :options="TZ" /></UiField>
    </div>
    <template v-if="form.schedule.mode === 'times'">
      <div class="row">
        <UiField label="每天次数" help="从 00:00 起在一天内均匀分布。例如 24 表示每小时一次。">
          <input class="input" v-model.number="form.schedule.times_per_day" type="number" min="0" placeholder="24" />
        </UiField>
        <UiField label="或指定时刻" help="填写后覆盖“每天次数”。逗号分隔，例如 00:00, 12:00。">
          <input class="input" v-model="timesText" type="text" placeholder="00:00, 12:00（可选）" />
        </UiField>
      </div>
    </template>
    <UiField v-else-if="form.schedule.mode === 'cron'" label="Cron 表达式" help="5 段：分 时 日 月 周。也支持 6 段（含秒）。" hint="例如 0 3 * * * 表示每天 03:00。">
      <input class="input mono" v-model="form.schedule.cron" type="text" placeholder="0 3 * * *" />
    </UiField>
    <UiField v-else label="间隔（分钟）">
      <input class="input" v-model.number="form.schedule.interval_min" type="number" min="1" placeholder="60" style="max-width:220px" />
    </UiField>

    <hr class="divider" />
    <!-- 3 保留（友好版） -->
    <div class="section-title"><span class="num">03</span> 保留策略</div>
    <p class="subhelp">决定保留多少历史版本、删除哪些旧备份。先选一种方式，右侧会实时预览保留效果。</p>
    <UiField label="划分“每天”的时区" help="按此时区计算日历日。中国大陆 (UTC+8) 请选 Asia/Shanghai。">
      <div class="tz-field"><UiSelect v-model="ret.timezone" :options="TZ" /></div>
    </UiField>

    <UiField label="保留方式"><UiRadioCards v-model="ret.mode" :options="RET_MODES" :min="188" /></UiField>

    <template v-if="ret.mode === 'simple'">
      <UiField label="保留份数" help="无论时间长短，只按数量保留最新的这么多份。">
        <input class="input num-field" v-model.number="ret.count" type="number" min="1" />
      </UiField>
    </template>

    <template v-else-if="ret.mode === 'days'">
      <UiField label="保留天数" help="超过这么多天的备份全部删除。">
        <input class="input num-field" v-model.number="ret.days" type="number" min="1" />
      </UiField>
      <UiField label="每天保留"><UiRadioCards v-model="ret.dayKeep" :options="DAYKEEP" /></UiField>
    </template>

    <template v-else>
      <div class="dual">
        <UiField label="最近几天完整保留" help="这么多天内的每一次备份都留着，方便随时回滚。填 1 = 只有今天保留全部。">
          <input class="input" v-model.number="ret.keepAllDays" type="number" min="1" />
        </UiField>
        <UiField label="总共保留天数" help="超过这么多天的备份全部删除。">
          <input class="input" v-model.number="ret.days" type="number" min="1" />
        </UiField>
      </div>
      <UiField label="更早的日子，每天保留"><UiRadioCards v-model="ret.anchorMode" :options="ANCHOR_MODES" :min="188" /></UiField>
      <UiField label="自定义保留时刻" :hint="ret.anchorMode === 'custom' ? '逗号分隔，如 00:00, 12:00' : '选择上方“自定义时刻”后可编辑'">
        <input class="input" v-model="ret.anchorsCustom" type="text" placeholder="00:00, 12:00" :disabled="ret.anchorMode !== 'custom'" style="max-width:320px" />
      </UiField>
    </template>

    <p class="ret-explain"><Icon name="info" :size="14" /> {{ retExplain }}</p>
    <RetentionPreview :policy="form.retention" :per-day="perDay" style="margin-top:14px" />

    <hr class="divider" />
    <!-- 4 渠道 -->
    <div class="section-title"><span class="num">04</span> 备份渠道 <span class="faint" style="font-weight:400">· 可多选</span></div>
    <p v-if="!channels.length" class="subhelp">还没有渠道，请先到「备份渠道」创建。</p>
    <div v-else class="pick-grid">
      <UiCheckCard v-for="c in channels" :key="c.id" :checked="form.channel_ids.includes(c.id)" :title="c.name" :sub="c.type === 'local' ? '本地目录' : 'S3 / R2'" :icon="chanIcon(c.type)" @toggle="toggleChan(c.id)" />
    </div>

    <hr class="divider" />
    <!-- 5 通知 -->
    <div class="section-title"><span class="num">05</span> 通知渠道 <span class="faint" style="font-weight:400">· 可选、可多选</span></div>
    <p v-if="!notifiers.length" class="subhelp">还没有通知渠道（可选）。可到「通知渠道」创建。</p>
    <div v-else class="pick-grid">
      <UiCheckCard v-for="n in notifiers" :key="n.id" :checked="form.notifier_ids.includes(n.id)" :title="n.name" :sub="n.type" :icon="notiIcon[n.type] || 'bell'" @toggle="toggleNoti(n.id)" />
    </div>

    <hr class="divider" />
    <!-- 6 归档 -->
    <div class="section-title"><span class="num">06</span> 归档选项</div>
    <div class="row" style="align-items:flex-end">
      <UiField label="格式"><UiSegmented v-model="form.archive.format" :options="FORMATS" /></UiField>
      <UiField label="压缩级别 (0–9)" help="仅 tar.gz 生效。0 使用默认，越大压缩越强越慢。">
        <input class="input" v-model.number="form.archive.compression" type="number" min="0" max="9" style="max-width:120px" />
      </UiField>
    </div>
    <div class="row">
      <UiField label="排除模式" help="每行一个 glob，匹配到的文件/目录不打包。">
        <textarea v-model="excludeText" rows="3" placeholder="*.log&#10;node_modules" />
      </UiField>
      <UiField label="仅包含" help="留空表示全部；填写后只打包匹配的路径。每行一个 glob。">
        <textarea v-model="includeText" rows="3" placeholder="留空 = 全部" />
      </UiField>
    </div>

    <hr class="divider" />
    <label class="switch-row"><UiToggle v-model="form.enabled" /><span>启用该目标<span class="faint"> · 参与定时调度</span></span></label>

    <template #footer>
      <div class="foot-desc faint">{{ scheduleDesc(form.schedule) }} · {{ retentionDesc(form.retention) }}</div>
      <div class="spacer" />
      <button class="btn btn-ghost" @click="showModal = false">取消</button>
      <button class="btn btn-primary" :disabled="saving || !canSave" @click="save"><span v-if="saving" class="spinner" /> 保存目标</button>
    </template>
  </UiModal>

  <FileBrowser :show="showBrowser" title="选择源目录" :start="form.source_path" @close="showBrowser = false" @pick="(p) => (form.source_path = p)" />
</template>

<style scoped>
.tz-field { max-width: 300px; }
.num-field { max-width: 160px; }
.dual { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.ret-explain { display: flex; align-items: flex-start; gap: 7px; margin: 16px 0 0; padding: 12px 14px; border-radius: var(--r-md); background: var(--accent-soft); color: var(--accent); font-size: 13px; line-height: 1.6; }
.ret-explain :deep(.ico) { margin-top: 1px; flex: 0 0 auto; }
.pick-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 10px; }
.switch-row { display: flex; align-items: center; gap: 11px; cursor: pointer; font-size: 13.5px; }
.foot-desc { font-size: 12px; max-width: 58%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 560px) { .dual { grid-template-columns: 1fr; } }
</style>
