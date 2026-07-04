<script setup>
import UiInfo from './UiInfo.vue'
defineProps({
  label: { type: String, default: '' },
  hint: { type: String, default: '' },
  help: { type: String, default: '' },
})
</script>

<template>
  <div class="field">
    <div v-if="label" class="field-label">
      <span>{{ label }}</span>
      <UiInfo v-if="help" :text="help" />
    </div>
    <slot />
    <div v-if="hint" class="hint">{{ hint }}</div>
  </div>
</template>
