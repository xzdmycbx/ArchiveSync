<script setup>
import Icon from '../Icon.vue'
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])
</script>

<template>
  <button
    type="button" class="cbx" :class="{ on: modelValue }" role="checkbox"
    :aria-checked="modelValue" :disabled="disabled"
    @click="emit('update:modelValue', !modelValue)"
  >
    <Transition name="pop">
      <Icon v-if="modelValue" name="check" :size="13" :stroke="2.6" />
    </Transition>
  </button>
</template>

<style scoped>
.cbx {
  width: 20px; height: 20px; border-radius: 6px; border: 1.6px solid var(--border-strong);
  background: var(--bg); cursor: pointer; display: grid; place-items: center; padding: 0;
  color: #fff; flex: 0 0 auto; transition: background var(--dur-fast), border-color var(--dur-fast);
}
.cbx.on { background: var(--accent); border-color: var(--accent); }
.cbx:disabled { opacity: 0.5; cursor: not-allowed; }
.cbx:focus-visible { outline: none; box-shadow: var(--shadow-focus); }
</style>
