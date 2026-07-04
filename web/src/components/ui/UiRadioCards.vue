<script setup>
defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, default: () => [] }, // [{value,label,desc?}]
  min: { type: Number, default: 168 }, // min card width for the responsive grid
})
const emit = defineEmits(['update:modelValue'])
</script>

<template>
  <div class="radio-cards" :style="{ '--min': min + 'px' }" role="radiogroup">
    <button
      v-for="o in options" :key="o.value" type="button" class="rc"
      :class="{ on: o.value === modelValue }" role="radio" :aria-checked="o.value === modelValue"
      @click="emit('update:modelValue', o.value)"
    >
      <span class="rc-dot"><span class="rc-inner" /></span>
      <span class="rc-body">
        <span class="rc-label">{{ o.label }}</span>
        <span v-if="o.desc" class="rc-desc">{{ o.desc }}</span>
      </span>
    </button>
  </div>
</template>

<style scoped>
.radio-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(var(--min), 1fr)); gap: 10px; }
.rc {
  display: flex; align-items: flex-start; gap: 10px; padding: 12px 13px; text-align: left;
  border: 1px solid var(--border-strong); border-radius: var(--r-md); background: var(--bg);
  color: var(--text); font: inherit; cursor: pointer; min-width: 0;
  transition: border-color var(--dur-fast), background var(--dur-fast), box-shadow var(--dur-fast);
}
.rc:hover { border-color: var(--text-faint); }
.rc.on { border-color: var(--accent); background: var(--accent-soft); box-shadow: var(--shadow-focus); }
.rc-dot { flex: 0 0 auto; width: 18px; height: 18px; border-radius: 50%; border: 1.6px solid var(--border-strong); display: grid; place-items: center; margin-top: 1px; transition: border-color var(--dur-fast); }
.rc.on .rc-dot { border-color: var(--accent); }
.rc-inner { width: 9px; height: 9px; border-radius: 50%; background: var(--accent); transform: scale(0); transition: transform var(--dur) var(--ease); }
.rc.on .rc-inner { transform: scale(1); }
.rc-body { display: flex; flex-direction: column; min-width: 0; }
.rc-label { font-weight: 560; font-size: 13.5px; }
.rc.on .rc-label { color: var(--accent); }
.rc-desc { font-size: 12px; color: var(--text-faint); margin-top: 2px; line-height: 1.45; }
</style>
