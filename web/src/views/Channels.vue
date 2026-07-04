<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { fmtTime } from '../utils/format'
import PageHeader from '../components/PageHeader.vue'
import HelpNote from '../components/HelpNote.vue'
import UiModal from '../components/UiModal.vue'
import FileBrowser from '../components/FileBrowser.vue'
import Icon from '../components/Icon.vue'
import UiField from '../components/ui/UiField.vue'
import UiSelect from '../components/ui/UiSelect.vue'
import UiToggle from '../components/ui/UiToggle.vue'

const ui = useUI()
const items = ref([])
const loading = ref(true)
const showModal = ref(false)
const editing = ref(false)
const saving = ref(false)
const testing = ref(false)
const showBrowser = ref(false)

const TYPES = [
  { value: 's3', label: 'S3 / Cloudflare R2 / MinIO', icon: 'cloud', hint: '任意 S3 兼容对象存储' },
  { value: 'local', label: '本地目录', icon: 'folder', hint: '写入服务器本地磁盘' },
]
const typeMeta = { s3: { label: 'S3 / R2', icon: 'cloud' }, local: { label: '本地目录', icon: 'folder' } }

const blank = () => ({
  id: '', name: '', type: 's3',
  config: { endpoint: '', region: 'auto', bucket: '', access_key_id: '', secret_access_key: '', prefix: '', force_path_style: false, base_path: '' },
})
const form = reactive(blank())
const canSave = computed(() => form.name.trim().length > 0)

async function load() {
  loading.value = true
  try { items.value = (await api.get('/channels')).data || [] }
  catch (e) { ui.err(errMsg(e)) }
  finally { loading.value = false }
}
onMounted(load)

function openNew() { Object.assign(form, blank()); editing.value = false; showModal.value = true }
function openEdit(ch) {
  Object.assign(form, blank())
  form.id = ch.id; form.name = ch.name; form.type = ch.type
  Object.assign(form.config, ch.config || {})
  form.config.secret_access_key = ''
  editing.value = true; showModal.value = true
}

async function save() {
  saving.value = true
  try {
    const payload = { name: form.name, type: form.type, config: { ...form.config } }
    if (editing.value) await api.put(`/channels/${form.id}`, payload)
    else await api.post('/channels', payload)
    ui.ok(editing.value ? '渠道已更新' : '渠道已创建')
    showModal.value = false; load()
  } catch (e) { ui.err(errMsg(e, '保存失败')) }
  finally { saving.value = false }
}
async function test() {
  testing.value = true
  try {
    const { data } = await api.post('/channels/test', { id: form.id, name: form.name, type: form.type, config: { ...form.config } })
    data.ok ? ui.ok('连接成功') : ui.err(`连接失败：${data.error || '未知错误'}`)
  } catch (e) { ui.err(errMsg(e, '测试失败')) }
  finally { testing.value = false }
}
async function testExisting(ch) {
  try { const { data } = await api.post(`/channels/${ch.id}/test`); data.ok ? ui.ok(`${ch.name}：连接成功`) : ui.err(`${ch.name}：${data.error || '连接失败'}`) }
  catch (e) { ui.err(errMsg(e)) }
}
async function remove(ch) {
  if (!(await ui.confirm({ title: '删除渠道', message: `确认删除渠道「${ch.name}」？该操作不可撤销。`, confirmText: '删除', danger: true }))) return
  try { await api.delete(`/channels/${ch.id}`); ui.ok('渠道已删除'); load() }
  catch (e) { ui.err(errMsg(e, '删除失败')) }
}
</script>

<template>
  <PageHeader eyebrow="存储" title="备份渠道" lede="备份归档要上传到哪里。支持任意 S3 兼容对象存储与本地目录，一个目标可同时分发到多个渠道。">
    <template #actions>
      <button class="btn btn-primary" @click="openNew"><Icon name="plus" :size="17" /> 新建渠道</button>
    </template>
  </PageHeader>

  <HelpNote title="什么是备份渠道？">
    渠道是备份文件的<b>目的地</b>。<b>S3</b> 类型适配 AWS S3、<b>Cloudflare R2</b>、MinIO 等：填写 Endpoint、Bucket 与密钥即可；
    <b>本地目录</b>把归档写入服务器磁盘。归档以 <code>&lt;目标ID&gt;/&lt;名称&gt;-&lt;时间戳&gt;.tar.gz</code> 命名，每个目标相互隔离。
  </HelpNote>

  <div class="card">
    <div v-if="loading" class="empty"><span class="spinner" /></div>
    <div v-else-if="!items.length" class="empty">
      <span class="empty-ico"><Icon name="cloud" :size="24" /></span>
      <h3>还没有备份渠道</h3>
      <p>先创建一个渠道，稍后在备份目标里选用它。</p>
      <button class="btn btn-primary" style="margin-top:14px" @click="openNew"><Icon name="plus" :size="17" /> 新建渠道</button>
    </div>
    <div v-else class="table-wrap">
      <table class="table">
        <thead><tr><th>名称</th><th>类型</th><th>位置</th><th>创建时间</th><th /></tr></thead>
        <TransitionGroup tag="tbody" name="list">
          <tr v-for="ch in items" :key="ch.id">
            <td>
              <div class="inline">
                <span class="type-ico"><Icon :name="typeMeta[ch.type]?.icon || 'cloud'" :size="17" /></span>
                <RouterLink :to="`/channels/${ch.id}`" class="rowlink">{{ ch.name }}</RouterLink>
              </div>
            </td>
            <td><span class="badge badge-accent">{{ typeMeta[ch.type]?.label || ch.type }}</span></td>
            <td class="mono faint" style="font-size:12px">
              <template v-if="ch.type === 's3'">{{ ch.config.bucket }}<span v-if="ch.config.endpoint"> · {{ ch.config.endpoint.replace(/^https?:\/\//, '') }}</span></template>
              <template v-else>{{ ch.config.base_path }}</template>
            </td>
            <td class="muted">{{ fmtTime(ch.created_at) }}</td>
            <td class="table-actions">
              <RouterLink :to="`/channels/${ch.id}`" class="btn btn-sm icon-btn" title="详情 / 浏览备份"><Icon name="eye" :size="16" /></RouterLink>
              <button class="btn btn-sm icon-btn" title="测试连接" @click="testExisting(ch)"><Icon name="activity" :size="16" /></button>
              <button class="btn btn-sm icon-btn" title="编辑" @click="openEdit(ch)"><Icon name="edit" :size="16" /></button>
              <button class="btn btn-sm icon-btn btn-danger" title="删除" @click="remove(ch)"><Icon name="trash" :size="16" /></button>
            </td>
          </tr>
        </TransitionGroup>
      </table>
    </div>
  </div>

  <UiModal :show="showModal" :title="editing ? '编辑渠道' : '新建渠道'" :subtitle="editing ? form.name : '配置一个备份文件的存储目的地'" @close="showModal = false">
    <UiField label="名称" hint="用于在目标里识别该渠道，例如「R2-生产环境」。">
      <input class="input" v-model="form.name" type="text" placeholder="R2-生产环境" />
    </UiField>
    <UiField label="类型">
      <UiSelect v-model="form.type" :options="TYPES" />
    </UiField>

    <template v-if="form.type === 's3'">
      <div class="row">
        <UiField label="Endpoint" help="R2 形如 https://<账户>.r2.cloudflarestorage.com；AWS S3 留空即可。">
          <input class="input" v-model="form.config.endpoint" type="text" placeholder="留空 = AWS S3" />
        </UiField>
        <UiField label="Region" help="Cloudflare R2 使用 auto；AWS 使用具体区域如 ap-northeast-1。">
          <input class="input" v-model="form.config.region" type="text" placeholder="auto" />
        </UiField>
      </div>
      <div class="row">
        <UiField label="Bucket"><input class="input" v-model="form.config.bucket" type="text" placeholder="存储桶名称" /></UiField>
        <UiField label="Key 前缀" help="所有对象都会放在该前缀下，便于多项目共享一个桶。留空则放桶根。">
          <input class="input" v-model="form.config.prefix" type="text" placeholder="backups/（可选）" />
        </UiField>
      </div>
      <UiField label="Access Key ID"><input class="input" v-model="form.config.access_key_id" type="text" autocomplete="off" /></UiField>
      <UiField label="Secret Access Key" :hint="editing ? '出于安全，已保存的密钥不回显；留空表示保持不变。' : ''">
        <input class="input" v-model="form.config.secret_access_key" type="password" autocomplete="new-password" :placeholder="editing ? '留空保持不变' : ''" />
      </UiField>
      <label class="switch-row">
        <UiToggle v-model="form.config.force_path_style" />
        <span>强制 Path-Style 寻址<span class="faint"> · MinIO 通常需要开启</span></span>
      </label>
    </template>

    <template v-else>
      <UiField label="本地路径" help="归档会写入该目录，请确保运行 ArchiveSync 的用户对其有写权限。">
        <div class="path-input">
          <input class="input" v-model="form.config.base_path" type="text" placeholder="/var/backups/archivesync" />
          <button type="button" class="btn" @click="showBrowser = true"><Icon name="folderOpen" :size="16" /> 浏览</button>
        </div>
      </UiField>
    </template>

    <template #footer>
      <button class="btn" :disabled="testing" @click="test">
        <span v-if="testing" class="spinner" /><Icon v-else name="activity" :size="16" /> 测试连接
      </button>
      <div class="spacer" />
      <button class="btn btn-ghost" @click="showModal = false">取消</button>
      <button class="btn btn-primary" :disabled="saving || !canSave" @click="save">
        <span v-if="saving" class="spinner" /> 保存
      </button>
    </template>
  </UiModal>

  <FileBrowser :show="showBrowser" title="选择本地目录" :start="form.config.base_path" @close="showBrowser = false" @pick="(p) => (form.config.base_path = p)" />
</template>

<style scoped>
.type-ico { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; background: var(--accent-soft); color: var(--accent); flex: 0 0 32px; }
.switch-row { display: flex; align-items: center; gap: 11px; cursor: pointer; font-size: 13.5px; margin-top: 4px; }
</style>
