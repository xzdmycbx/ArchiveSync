<script setup>
import Icon from '../Icon.vue'
defineProps({
  checked: { type: Boolean, default: false },
  title: { type: String, default: '' },
  sub: { type: String, default: '' },
  icon: { type: String, default: '' },
})
const emit = defineEmits(['toggle'])
</script>

<template>
  <button type="button" class="checkcard" :class="{ on: checked }" @click="emit('toggle')">
    <span v-if="icon" class="cc-ico"><Icon :name="icon" :size="18" /></span>
    <span class="cc-body">
      <span class="cc-title">{{ title }}</span>
      <span v-if="sub" class="cc-sub">{{ sub }}</span>
    </span>
    <span class="cc-check"><Icon v-if="checked" name="check" :size="14" :stroke="2.6" /></span>
  </button>
</template>

<style scoped>
.checkcard {
  display: flex; align-items: center; gap: 11px; padding: 11px 13px; text-align: left;
  border: 1px solid var(--border-strong); border-radius: var(--r-md); background: var(--bg);
  color: var(--text); font: inherit; cursor: pointer; min-width: 190px;
  transition: border-color var(--dur-fast), background var(--dur-fast), box-shadow var(--dur-fast);
}
.checkcard:hover { border-color: var(--text-faint); }
.checkcard.on { border-color: var(--accent); background: var(--accent-soft); box-shadow: var(--shadow-focus); }
.cc-ico { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 9px; background: var(--bg-sunken); color: var(--text-muted); flex: 0 0 34px; }
.checkcard.on .cc-ico { background: color-mix(in srgb, var(--accent) 16%, transparent); color: var(--accent); }
.cc-body { display: flex; flex-direction: column; min-width: 0; flex: 1; }
.cc-title { font-weight: 560; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cc-sub { font-size: 12px; color: var(--text-faint); }
.cc-check {
  width: 20px; height: 20px; border-radius: 6px; border: 1.6px solid var(--border-strong);
  display: grid; place-items: center; color: #fff; flex: 0 0 20px; transition: all var(--dur-fast);
}
.checkcard.on .cc-check { background: var(--accent); border-color: var(--accent); }
</style>
