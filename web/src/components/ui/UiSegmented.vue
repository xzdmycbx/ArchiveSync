<script setup>
const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, default: () => [] }, // [{value,label}]
})
const emit = defineEmits(['update:modelValue'])
</script>

<template>
  <div class="seg" role="tablist">
    <button
      v-for="o in options" :key="o.value" type="button" class="seg-item"
      :class="{ on: o.value === modelValue }" role="tab" :aria-selected="o.value === modelValue"
      @click="emit('update:modelValue', o.value)"
    >
      {{ o.label }}
    </button>
  </div>
</template>

<style scoped>
.seg {
  display: inline-flex; flex-wrap: wrap; max-width: 100%; padding: 3px; gap: 2px;
  background: var(--bg-sunken); border-radius: var(--r-sm); border: 1px solid var(--border);
}
.seg-item {
  border: none; background: transparent; color: var(--text-muted); font: inherit; font-weight: 540;
  padding: 7px 14px; border-radius: 8px; cursor: pointer; white-space: nowrap;
  transition: color var(--dur-fast), background var(--dur-fast), box-shadow var(--dur-fast);
}
.seg-item:hover { color: var(--text); }
.seg-item.on { background: var(--bg-elev); color: var(--accent); box-shadow: var(--shadow-xs); }
.seg-item:focus-visible { outline: none; box-shadow: var(--shadow-focus); }
</style>
