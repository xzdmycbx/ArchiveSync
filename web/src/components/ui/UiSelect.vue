<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import Icon from '../Icon.vue'

const props = defineProps({
  modelValue: { type: [String, Number, Boolean, null], default: null },
  options: { type: Array, default: () => [] }, // [{value,label,hint?,icon?}] or ["a","b"]
  placeholder: { type: String, default: '请选择' },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const root = ref(null)
const menu = ref(null)
const open = ref(false)
const active = ref(-1)
const menuStyle = ref({})

const norm = computed(() =>
  props.options.map((o) => (o && typeof o === 'object' ? o : { value: o, label: String(o) })),
)
const selected = computed(() => norm.value.find((o) => o.value === props.modelValue))

function place() {
  const el = root.value
  if (!el) return
  const r = el.getBoundingClientRect()
  const spaceBelow = window.innerHeight - r.bottom
  const flipUp = spaceBelow < 220 && r.top > spaceBelow
  const maxH = Math.min(300, (flipUp ? r.top : spaceBelow) - 14)
  menuStyle.value = {
    position: 'fixed',
    left: r.left + 'px',
    width: r.width + 'px',
    maxHeight: maxH + 'px',
    ...(flipUp ? { bottom: window.innerHeight - r.top + 6 + 'px' } : { top: r.bottom + 6 + 'px' }),
  }
}
function toggle() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    active.value = norm.value.findIndex((o) => o.value === props.modelValue)
    nextTick(() => { place(); scrollActive() })
  }
}
function choose(o) { emit('update:modelValue', o.value); open.value = false }
function scrollActive() { menu.value?.querySelector('.sel-opt.active')?.scrollIntoView({ block: 'nearest' }) }

function onKey(e) {
  if (props.disabled) return
  if (!open.value && (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); toggle(); return }
  if (!open.value) return
  if (e.key === 'Escape') { open.value = false }
  else if (e.key === 'ArrowDown') { e.preventDefault(); active.value = Math.min(active.value + 1, norm.value.length - 1); scrollActive() }
  else if (e.key === 'ArrowUp') { e.preventDefault(); active.value = Math.max(active.value - 1, 0); scrollActive() }
  else if (e.key === 'Enter') { e.preventDefault(); if (norm.value[active.value]) choose(norm.value[active.value]) }
}
function onDoc(e) {
  if (root.value?.contains(e.target)) return
  if (menu.value?.contains(e.target)) return
  open.value = false
}
function onScrollOrResize() { if (open.value) open.value = false }

onMounted(() => {
  document.addEventListener('mousedown', onDoc)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDoc)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})
</script>

<template>
  <div ref="root" class="sel" :class="{ open, disabled }">
    <button type="button" class="sel-trigger" :disabled="disabled" @click="toggle" @keydown="onKey">
      <span v-if="selected" class="sel-value">
        <Icon v-if="selected.icon" :name="selected.icon" :size="16" />
        <span>{{ selected.label }}</span>
      </span>
      <span v-else class="sel-value placeholder">{{ placeholder }}</span>
      <Icon class="sel-caret" name="chevronDown" :size="16" />
    </button>

    <Teleport to="body">
      <Transition name="pop">
        <div v-if="open" ref="menu" class="sel-menu" role="listbox" :style="menuStyle">
          <button
            v-for="(o, i) in norm" :key="o.value"
            type="button" class="sel-opt"
            :class="{ active: i === active, selected: o.value === modelValue }"
            role="option" :aria-selected="o.value === modelValue"
            @mouseenter="active = i" @click="choose(o)"
          >
            <Icon v-if="o.icon" :name="o.icon" :size="16" class="opt-ico" />
            <span class="opt-text">
              <span class="opt-label">{{ o.label }}</span>
              <span v-if="o.hint" class="opt-hint">{{ o.hint }}</span>
            </span>
            <Icon v-if="o.value === modelValue" name="check" :size="15" :stroke="2.4" class="opt-check" />
          </button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.sel { position: relative; }
.sel-trigger {
  width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 10px 12px; border: 1px solid var(--border-strong); border-radius: var(--r-sm);
  background: var(--bg); color: var(--text); font: inherit; cursor: pointer; text-align: left;
  transition: border-color var(--dur-fast), box-shadow var(--dur-fast), background var(--dur-fast);
}
.sel-trigger:hover { border-color: var(--text-faint); }
.sel.open .sel-trigger { border-color: var(--accent); box-shadow: var(--shadow-focus); background: var(--bg-elev); }
.sel-trigger:focus-visible { outline: none; border-color: var(--accent); box-shadow: var(--shadow-focus); }
.sel.disabled .sel-trigger { opacity: 0.55; cursor: not-allowed; }
.sel-value { display: inline-flex; align-items: center; gap: 8px; min-width: 0; }
.sel-value span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sel-value.placeholder { color: var(--text-faint); }
.sel-caret { color: var(--text-faint); transition: transform var(--dur) var(--ease); }
.sel.open .sel-caret { transform: rotate(180deg); }
</style>

<style>
/* Teleported to body — must be global, not scoped. */
.sel-menu {
  z-index: 200; background: var(--bg-elev); border: 1px solid var(--border);
  border-radius: var(--r-md); box-shadow: var(--shadow-lg); padding: 6px; overflow-y: auto;
}
.sel-opt {
  width: 100%; display: flex; align-items: center; gap: 10px; padding: 9px 10px;
  border: none; background: transparent; border-radius: 8px; color: var(--text);
  font: inherit; cursor: pointer; text-align: left;
}
.sel-opt.active { background: var(--bg-hover); }
.sel-opt.selected { color: var(--accent); }
.sel-opt .opt-ico { color: var(--text-muted); }
.sel-opt.selected .opt-ico { color: var(--accent); }
.sel-opt .opt-text { display: flex; flex-direction: column; min-width: 0; flex: 1; }
.sel-opt .opt-label { font-weight: 520; }
.sel-opt .opt-hint { font-size: 12px; color: var(--text-faint); }
.sel-opt .opt-check { margin-left: auto; color: var(--accent); }
</style>
