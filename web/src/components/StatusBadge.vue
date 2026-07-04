<script setup>
import Icon from './Icon.vue'
import { statusMeta } from '../utils/format'
defineProps({ status: { type: String, default: '' } })
</script>

<template>
  <span class="badge" :class="statusMeta[status]?.cls || 'badge-neutral'">
    <Icon :name="statusMeta[status]?.icon || 'clock'" :size="13" />
    {{ statusMeta[status]?.label || status }}
  </span>
</template>
