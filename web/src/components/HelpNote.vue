<script setup>
import Icon from './Icon.vue'
defineProps({
  title: { type: String, default: '' },
  icon: { type: String, default: 'info' },
})
</script>

<template>
  <div class="helpnote">
    <span class="hn-ico"><Icon :name="icon" :size="16" /></span>
    <div class="hn-body">
      <div v-if="title" class="hn-title">{{ title }}</div>
      <div class="hn-text"><slot /></div>
    </div>
  </div>
</template>

<style scoped>
.helpnote { display: flex; gap: 12px; padding: 14px 16px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--brand-grad-soft); margin-bottom: 20px; }
.hn-ico { color: var(--accent); flex: 0 0 auto; margin-top: 1px; }
.hn-title { font-weight: 620; font-size: 13px; margin-bottom: 3px; }
.hn-text { color: var(--text-muted); font-size: 13px; line-height: 1.65; }
.hn-text :deep(b) { color: var(--text); font-weight: 600; }
.hn-text :deep(code) { background: var(--bg-sunken); padding: 1px 5px; border-radius: 5px; }
</style>
