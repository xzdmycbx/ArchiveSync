<script setup>
import { useUI } from '../stores/ui'
import Icon from './Icon.vue'

const ui = useUI()
const iconFor = { ok: 'checkCircle', err: 'xCircle', info: 'info' }
</script>

<template>
  <TransitionGroup tag="div" name="toast" class="toasts">
    <div v-for="t in ui.toasts" :key="t.id" class="toast" :class="t.type" @click="ui.dismiss(t.id)">
      <Icon :name="iconFor[t.type] || 'info'" :size="20" class="t-ico" />
      <span>{{ t.text }}</span>
    </div>
  </TransitionGroup>
</template>
