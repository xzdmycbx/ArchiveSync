<script setup>
import Icon from '../components/Icon.vue'
</script>

<template>
  <div class="center-screen">
    <div class="nf rise">
      <span class="nf-ico"><Icon name="slash" :size="30" /></span>
      <h2>找不到该页面</h2>
      <p class="muted">你访问的地址不存在或已移动。</p>
      <router-link to="/" class="btn btn-primary">返回仪表盘</router-link>
    </div>
  </div>
</template>

<style scoped>
.nf { text-align: center; }
.nf-ico { display: grid; place-items: center; width: 60px; height: 60px; border-radius: 17px; margin: 0 auto 16px; background: var(--bg-sunken); color: var(--text-muted); }
.nf h2 { font-size: 20px; margin-bottom: 6px; }
.nf .btn { margin-top: 16px; }
</style>
