<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import { fmtTime } from '../utils/format'
import Icon from '../components/Icon.vue'
import ObjectBrowser from '../components/ObjectBrowser.vue'

const route = useRoute()
const ui = useUI()
const id = route.params.id
const loading = ref(true)
const channel = ref(null)
const testing = ref(false)

const typeMeta = { s3: { label: 'S3 / R2', icon: 'cloud' }, local: { label: '本地目录', icon: 'folder' } }

async function load() {
  loading.value = true
  try { channel.value = (await api.get(`/channels/${id}`)).data }
  catch (e) { ui.err(errMsg(e, '加载渠道失败')) }
  finally { loading.value = false }
}
onMounted(load)

async function test() {
  testing.value = true
  try { const { data } = await api.post(`/channels/${id}/test`); data.ok ? ui.ok('连接成功') : ui.err(`连接失败：${data.error || '未知错误'}`) }
  catch (e) { ui.err(errMsg(e)) }
  finally { testing.value = false }
}
</script>

<template>
  <div v-if="loading" class="empty"><span class="spinner" /></div>
  <template v-else-if="channel">
    <div class="detail-head rise">
      <RouterLink to="/channels" class="btn btn-ghost btn-sm"><Icon name="back" :size="16" /> 备份渠道</RouterLink>
      <div class="dh-main">
        <span class="type-ico"><Icon :name="typeMeta[channel.type]?.icon || 'cloud'" :size="18" /></span>
        <h1>{{ channel.name }}</h1>
        <span class="badge badge-accent">{{ typeMeta[channel.type]?.label || channel.type }}</span>
      </div>
      <div class="spacer" />
      <button class="btn" :disabled="testing" @click="test"><span v-if="testing" class="spinner" /><Icon v-else name="activity" :size="16" /> 测试连接</button>
    </div>

    <div class="card card-pad" style="margin-bottom:18px">
      <div class="section-title"><Icon name="server" :size="15" /> 配置</div>
      <dl class="kv">
        <template v-if="channel.type === 's3'">
          <dt>Endpoint</dt><dd class="mono">{{ channel.config.endpoint || '（AWS S3 默认）' }}</dd>
          <dt>Region</dt><dd class="mono">{{ channel.config.region || '—' }}</dd>
          <dt>Bucket</dt><dd class="mono">{{ channel.config.bucket }}</dd>
          <dt>Key 前缀</dt><dd class="mono">{{ channel.config.prefix || '（无）' }}</dd>
          <dt>Path-Style</dt><dd>{{ channel.config.force_path_style ? '开启' : '关闭' }}</dd>
        </template>
        <template v-else>
          <dt>本地路径</dt><dd class="mono">{{ channel.config.base_path }}</dd>
        </template>
        <dt>创建时间</dt><dd>{{ fmtTime(channel.created_at) }}</dd>
      </dl>
    </div>

    <div class="card">
      <div class="card-head">
        <div><h3>渠道内的备份</h3><div class="sub">以文件夹形式浏览该渠道下所有目标的归档，可直接下载</div></div>
      </div>
      <div class="card-pad">
        <ObjectBrowser :channel-id="id" root-prefix="" root-label="全部备份" />
      </div>
    </div>
  </template>

  <div v-else class="empty"><h3>渠道不存在</h3><RouterLink to="/channels" class="btn btn-primary btn-sm" style="margin-top:12px">返回列表</RouterLink></div>
</template>

<style scoped>
.detail-head { display: flex; align-items: center; gap: 16px; margin-bottom: 22px; flex-wrap: wrap; }
.dh-main { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.dh-main h1 { font-size: 22px; letter-spacing: -0.02em; }
.type-ico { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 10px; background: var(--accent-soft); color: var(--accent); }
</style>
