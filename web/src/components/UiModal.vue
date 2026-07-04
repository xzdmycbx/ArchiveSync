<script setup>
import { watch, onBeforeUnmount } from 'vue'
import Icon from './Icon.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  wide: { type: Boolean, default: false },
  // When false (default), clicking the backdrop does NOT close the modal, so an
  // accidental click outside a form can't discard filled-in content. Read-only
  // dialogs can opt in to click-outside dismissal.
  dismissible: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])

// Lock body scroll while open.
watch(() => props.show, (v) => {
  document.body.style.overflow = v ? 'hidden' : ''
})
onBeforeUnmount(() => { document.body.style.overflow = '' })

function onBackdrop() { if (props.dismissible) emit('close') }
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-backdrop" @click.self="onBackdrop">
        <div class="modal" :class="{ wide }" role="dialog" aria-modal="true">
          <header class="modal-head">
            <div class="modal-head-text">
              <h3>{{ title }}</h3>
              <p v-if="subtitle" class="modal-sub">{{ subtitle }}</p>
            </div>
            <button type="button" class="btn btn-ghost icon-btn" aria-label="关闭" @click="emit('close')">
              <Icon name="close" :size="18" />
            </button>
          </header>
          <div class="modal-body"><slot /></div>
          <footer v-if="$slots.footer" class="modal-foot"><slot name="footer" /></footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0; z-index: 60; display: grid; place-items: center; padding: 24px;
  background: color-mix(in srgb, #0a0c11 55%, transparent); backdrop-filter: blur(3px);
}
.modal {
  width: 100%; max-width: 600px; max-height: 90vh; display: flex; flex-direction: column;
  background: var(--bg-elev); border: 1px solid var(--border); border-radius: var(--r-xl);
  box-shadow: var(--shadow-lg); overflow: hidden;
}
.modal.wide { max-width: 820px; }
.modal-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 20px 24px; border-bottom: 1px solid var(--border); }
.modal-head-text h3 { font-size: 17px; }
.modal-sub { color: var(--text-muted); font-size: 13px; margin-top: 3px; }
.modal-body { padding: 22px 24px; overflow-y: auto; overflow-x: hidden; }
.modal-foot { display: flex; align-items: center; justify-content: flex-end; gap: 10px; padding: 16px 24px; border-top: 1px solid var(--border); background: var(--bg-sunken); }

.modal-enter-active { transition: opacity var(--dur) var(--ease); }
.modal-leave-active { transition: opacity var(--dur-fast) ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active .modal { transition: transform var(--dur) var(--ease), opacity var(--dur) var(--ease); }
.modal-leave-active .modal { transition: transform var(--dur-fast) ease, opacity var(--dur-fast) ease; }
.modal-enter-from .modal, .modal-leave-to .modal { transform: scale(0.95) translateY(12px); opacity: 0; }
</style>
