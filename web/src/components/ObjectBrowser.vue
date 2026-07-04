<script setup>
import { ref, watch, computed } from 'vue'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import Icon from './Icon.vue'
import { humanBytes, fmtTime } from '../utils/format'

// Folder-style browser for a channel's stored objects, with per-file download.
const props = defineProps({
  channelId: { type: String, required: true },
  rootPrefix: { type: String, default: '' },
  rootLabel: { type: String, default: '全部备份' },
})
const ui = useUI()
const prefix = ref(props.rootPrefix)
const entries = ref([])
const loading = ref(false)
const query = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q))
})

async function load(p) {
  loading.value = true
  query.value = ''
  try {
    const { data } = await api.get(`/channels/${props.channelId}/objects`, { params: { prefix: p ?? '' } })
    prefix.value = data.prefix
    entries.value = data.entries || []
  } catch (e) { ui.err(errMsg(e, '读取备份失败')); entries.value = [] }
  finally { loading.value = false }
}
watch(() => [props.channelId, props.rootPrefix], () => load(props.rootPrefix), { immediate: true })

const crumbs = computed(() => {
  const rp = props.rootPrefix
  const rel = prefix.value.startsWith(rp) ? prefix.value.slice(rp.length) : prefix.value
  const parts = rel.split('/').filter(Boolean)
  const out = [{ label: props.rootLabel, prefix: rp }]
  let acc = rp
  for (const seg of parts) { acc += seg + '/'; out.push({ label: seg, prefix: acc }) }
  return out
})

function downloadUrl(key) { return `/api/channels/${props.channelId}/download?key=${encodeURIComponent(key)}` }
</script>

<template>
  <div class="ob">
    <div class="ob-bar">
      <div class="crumbs">
        <template v-for="(c, i) in crumbs" :key="i">
          <button class="crumb" :class="{ cur: i === crumbs.length - 1 }" @click="load(c.prefix)">{{ c.label }}</button>
          <Icon v-if="i < crumbs.length - 1" name="chevronRight" :size="13" class="crumb-sep faint" />
        </template>
      </div>
      <div class="ob-tools">
        <div class="ob-search">
          <Icon name="search" :size="15" />
          <input v-model="query" type="text" placeholder="搜索名称…" />
          <button v-if="query" class="ob-clear" title="清除" @click="query = ''"><Icon name="close" :size="13" /></button>
        </div>
        <button class="btn btn-sm btn-ghost icon-btn" title="刷新" @click="load(prefix)"><Icon name="refresh" :size="15" /></button>
      </div>
    </div>

    <div class="ob-list">
      <div v-if="loading" class="empty"><span class="spinner" /></div>
      <template v-else>
        <div v-if="!entries.length" class="empty faint" style="padding:30px">此位置暂无备份文件</div>
        <div v-else-if="!filtered.length" class="empty faint" style="padding:30px">没有匹配“{{ query }}”的项</div>
        <div v-for="e in filtered" :key="e.key" class="ob-item">
          <span class="ob-ico" :class="{ dir: e.is_dir }"><Icon :name="e.is_dir ? 'folderOpen' : 'file'" :size="17" /></span>
          <button v-if="e.is_dir" class="ob-name link" @click="load(e.key)">{{ e.name }}</button>
          <span v-else class="ob-name">{{ e.name }}</span>
          <span v-if="!e.is_dir" class="ob-meta faint">{{ humanBytes(e.size) }}<template v-if="e.last_modified"> · {{ fmtTime(e.last_modified) }}</template></span>
          <span class="spacer" />
          <a v-if="!e.is_dir" class="btn btn-sm" :href="downloadUrl(e.key)"><Icon name="download" :size="15" /> 下载</a>
          <Icon v-else name="chevronRight" :size="16" class="faint" />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.ob { border: 1px solid var(--border); border-radius: var(--r-md); overflow: hidden; }
.ob-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--border); background: var(--bg-sunken); flex-wrap: wrap; }
.crumbs { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; min-width: 0; }
.ob-tools { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; }
.ob-search { display: flex; align-items: center; gap: 6px; padding: 5px 10px; border: 1px solid var(--border-strong); border-radius: var(--r-pill); background: var(--bg); color: var(--text-faint); }
.ob-search:focus-within { border-color: var(--accent); box-shadow: var(--shadow-focus); }
.ob-search input { border: none; background: transparent; outline: none; font: inherit; font-size: 13px; color: var(--text); width: 130px; padding: 0; }
.ob-search input::placeholder { color: var(--text-faint); }
.ob-clear { border: none; background: transparent; color: var(--text-faint); cursor: pointer; display: grid; place-items: center; padding: 0; }
.ob-clear:hover { color: var(--text); }
.crumb { border: none; background: transparent; color: var(--text-muted); font: inherit; font-size: 13px; cursor: pointer; padding: 2px 4px; border-radius: 6px; }
.crumb:hover { color: var(--accent); }
.crumb.cur { color: var(--text); font-weight: 600; cursor: default; }
.crumb-sep { flex: 0 0 auto; }
.ob-list { max-height: 52vh; overflow-y: auto; }
.ob-item { display: flex; align-items: center; gap: 11px; padding: 11px 14px; border-bottom: 1px solid var(--border); }
.ob-item:last-child { border-bottom: none; }
.ob-item:hover { background: var(--bg-hover); }
.ob-ico { display: grid; place-items: center; color: var(--text-muted); flex: 0 0 auto; }
.ob-ico.dir { color: var(--accent); }
.ob-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13.5px; }
.ob-name.link { border: none; background: transparent; color: var(--text); font: inherit; font-weight: 550; cursor: pointer; text-align: left; padding: 0; }
.ob-name.link:hover { color: var(--accent); }
.ob-meta { font-size: 12px; white-space: nowrap; }
</style>
