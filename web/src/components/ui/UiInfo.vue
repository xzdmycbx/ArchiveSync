<script setup>
import { ref } from 'vue'
import Icon from '../Icon.vue'
defineProps({ text: { type: String, default: '' } })

const open = ref(false)
const trigger = ref(null)
const pos = ref({ top: 0, left: 0, placement: 'top' })

function show() {
  const el = trigger.value
  if (!el) return
  const r = el.getBoundingClientRect()
  const below = r.top < 130 // not enough room above → drop below
  const left = Math.min(Math.max(r.left + r.width / 2, 140), window.innerWidth - 140)
  pos.value = {
    left,
    top: below ? r.bottom + 9 : r.top - 9,
    placement: below ? 'bottom' : 'top',
  }
  open.value = true
}
function hide() { open.value = false }
</script>

<template>
  <span
    ref="trigger" class="uinfo" tabindex="0"
    @mouseenter="show" @mouseleave="hide" @focus="show" @blur="hide"
    @click.stop="open ? hide() : show()"
  >
    <Icon name="info" :size="14" />
    <Teleport to="body">
      <Transition name="pop">
        <span
          v-if="open" class="uinfo-pop" :class="pos.placement"
          :style="{ top: pos.top + 'px', left: pos.left + 'px' }"
        >{{ text }}</span>
      </Transition>
    </Teleport>
  </span>
</template>

<style scoped>
.uinfo { display: inline-grid; place-items: center; color: var(--text-faint); cursor: help; vertical-align: middle; }
.uinfo:hover, .uinfo:focus-visible { color: var(--accent); outline: none; }
</style>

<style>
/* Global (teleported to body, so not scoped). */
.uinfo-pop {
  position: fixed; z-index: 200; width: max-content; max-width: 260px;
  padding: 9px 12px; border-radius: 10px;
  background: var(--text); color: var(--bg-elev);
  font-size: 12px; font-weight: 460; line-height: 1.55; text-align: left;
  box-shadow: var(--shadow-md); pointer-events: none;
}
.uinfo-pop.top { transform: translate(-50%, -100%); }
.uinfo-pop.bottom { transform: translate(-50%, 0); }
.uinfo-pop::after {
  content: ""; position: absolute; left: 50%; margin-left: -5px; border: 5px solid transparent;
}
.uinfo-pop.top::after { top: 100%; border-top-color: var(--text); }
.uinfo-pop.bottom::after { bottom: 100%; border-bottom-color: var(--text); }
</style>
