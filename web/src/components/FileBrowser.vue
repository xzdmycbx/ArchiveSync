<script setup>
import { ref, computed, watch } from 'vue'
import api, { errMsg } from '../api/client'
import { useUI } from '../stores/ui'
import UiModal from './UiModal.vue'
import Icon from './Icon.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '选择目录' },
  start: { type: String, default: '' },
})
const emit = defineEmits(['close', 'pick'])
const ui = useUI()

const path = ref('')
const parent = ref('')
const roots = ref([])
const entries = ref([])
const loading = ref(false)
const query = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q))
})

async function browse(p) {
  loading.value = true
  query.value = ''
  try {
    const { data } = await api.get('/fs', { params: { path: p || '' } })
    path.value = data.path
    parent.value = data.parent
    roots.value = data.roots || []
    entries.value = data.entries || []
  } catch (e) { ui.err(errMsg(e, '无法读取目录')) }
  finally { loading.value = false }
}
watch(() => props.show, (v) => { if (v) browse(props.start || '') })

function pick() { if (path.value) { emit('pick', path.value); emit('close') } }
</script>

<template>
  <UiModal :show="show" :title="title" :subtitle="path || '选择一个磁盘或目录'" @close="emit('close')">
    <div class="roots">
      <button v-for="r in roots" :key="r.path" class="chip" @click="browse(r.path)">
        <Icon :name="r.name === '主目录' ? 'home' : 'hardDrive'" :size="14" /> {{ r.name }}
      </button>
    </div>

    <div v-if="path" class="fb-search">
      <Icon name="search" :size="15" />
      <input v-model="query" type="text" placeholder="搜索当前目录…" />
      <button v-if="query" class="fb-clear" title="清除" @click="query = ''"><Icon name="close" :size="13" /></button>
    </div>

    <div class="fb-list">
      <button v-if="parent" class="fb-item" @click="browse(parent)">
        <span class="fb-ico"><Icon name="back" :size="16" /></span><span class="fb-name">上级目录</span>
      </button>
      <div v-if="loading" class="empty"><span class="spinner" /></div>
      <template v-else>
        <button
          v-for="e in filtered" :key="e.path" class="fb-item" :class="{ isfile: !e.is_dir }"
          :disabled="!e.is_dir" @click="e.is_dir && browse(e.path)"
        >
          <span class="fb-ico"><Icon :name="e.is_dir ? 'folder' : 'file'" :size="16" /></span>
          <span class="fb-name">{{ e.name }}</span>
          <Icon v-if="e.is_dir" name="chevronRight" :size="15" class="faint" />
        </button>
        <div v-if="!filtered.length && path" class="empty faint" style="padding:24px">{{ query ? `没有匹配“${query}”的项` : '（此目录为空）' }}</div>
      </template>
    </div>

    <template #footer>
      <div class="fb-current mono faint">{{ path || '未选择' }}</div>
      <div class="spacer" />
      <button class="btn btn-ghost" @click="emit('close')">取消</button>
      <button class="btn btn-primary" :disabled="!path" @click="pick"><Icon name="check" :size="16" /> 选择此目录</button>
    </template>
  </UiModal>
</template>

<style scoped>
.roots { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 14px; }
.chip { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: var(--r-pill); border: 1px solid var(--border-strong); background: var(--bg); color: var(--text-muted); font: inherit; font-size: 13px; cursor: pointer; transition: all var(--dur-fast); }
.chip:hover { border-color: var(--accent); color: var(--accent); }
.fb-search { display: flex; align-items: center; gap: 7px; padding: 8px 12px; margin-bottom: 10px; border: 1px solid var(--border-strong); border-radius: var(--r-sm); background: var(--bg); color: var(--text-faint); }
.fb-search:focus-within { border-color: var(--accent); box-shadow: var(--shadow-focus); }
.fb-search input { border: none; background: transparent; outline: none; font: inherit; font-size: 13.5px; color: var(--text); flex: 1; min-width: 0; padding: 0; }
.fb-search input::placeholder { color: var(--text-faint); }
.fb-clear { border: none; background: transparent; color: var(--text-faint); cursor: pointer; display: grid; place-items: center; padding: 0; }
.fb-clear:hover { color: var(--text); }
.fb-list { border: 1px solid var(--border); border-radius: var(--r-md); overflow: hidden; max-height: 42vh; overflow-y: auto; background: var(--bg); }
.fb-item { width: 100%; display: flex; align-items: center; gap: 11px; padding: 10px 14px; border: none; border-bottom: 1px solid var(--border); background: transparent; color: var(--text); font: inherit; cursor: pointer; text-align: left; }
.fb-item:last-child { border-bottom: none; }
.fb-item:hover:not(:disabled) { background: var(--bg-hover); }
.fb-item.isfile { color: var(--text-faint); cursor: default; }
.fb-ico { display: grid; place-items: center; color: var(--text-muted); flex: 0 0 auto; }
.fb-item.isfile .fb-ico { color: var(--text-faint); }
.fb-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fb-current { font-size: 12px; max-width: 60%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
