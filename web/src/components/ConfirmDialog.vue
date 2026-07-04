<script setup>
import { useUI } from '../stores/ui'
import UiModal from './UiModal.vue'
import Icon from './Icon.vue'

const ui = useUI()
</script>

<template>
  <UiModal
    :show="!!ui.dialog"
    :title="ui.dialog?.title || ''"
    dismissible
    @close="ui.resolveDialog(false)"
  >
    <div class="confirm-body">
      <span class="confirm-ico" :class="{ danger: ui.dialog?.danger }">
        <Icon :name="ui.dialog?.danger ? 'alert' : 'info'" :size="22" />
      </span>
      <p>{{ ui.dialog?.message }}</p>
    </div>
    <template #footer>
      <button class="btn btn-ghost" @click="ui.resolveDialog(false)">{{ ui.dialog?.cancelText }}</button>
      <button
        class="btn" :class="ui.dialog?.danger ? 'btn-danger' : 'btn-primary'"
        @click="ui.resolveDialog(true)"
      >{{ ui.dialog?.confirmText }}</button>
    </template>
  </UiModal>
</template>

<style scoped>
.confirm-body { display: flex; gap: 14px; align-items: flex-start; }
.confirm-ico { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 11px; background: var(--accent-soft); color: var(--accent); flex: 0 0 auto; }
.confirm-ico.danger { background: var(--danger-soft); color: var(--danger); }
.confirm-body p { margin: 0; padding-top: 9px; line-height: 1.65; color: var(--text); }
</style>
