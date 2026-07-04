<script setup>
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])
function toggle() { if (!props.disabled) emit('update:modelValue', !props.modelValue) }
</script>

<template>
  <button
    type="button" class="toggle" :class="{ on: modelValue }" role="switch"
    :aria-checked="modelValue" :disabled="disabled" @click="toggle"
  >
    <span class="knob" />
  </button>
</template>

<style scoped>
.toggle {
  width: 40px; height: 23px; border-radius: 999px; border: none; padding: 0;
  background: var(--border-strong); cursor: pointer; position: relative;
  transition: background var(--dur) var(--ease); flex: 0 0 auto;
}
.toggle.on { background: var(--accent); }
.toggle:disabled { opacity: 0.5; cursor: not-allowed; }
.toggle:focus-visible { outline: none; box-shadow: var(--shadow-focus); }
.knob {
  position: absolute; top: 3px; left: 3px; width: 17px; height: 17px; border-radius: 50%;
  background: #fff; box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
  transition: transform var(--dur) var(--ease);
}
.toggle.on .knob { transform: translateX(17px); }
</style>
