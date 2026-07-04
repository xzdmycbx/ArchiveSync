<script setup>
import { ref, computed } from 'vue'
import { RouterView, RouterLink, useRoute } from 'vue-router'
import { useAuth } from '../stores/auth'
import { currentTheme, toggleTheme } from '../utils/theme'
import Icon from './Icon.vue'

const auth = useAuth()
const route = useRoute()
const drawerOpen = ref(false)
const theme = ref(currentTheme())

const sections = [
  { title: '总览', items: [{ to: '/', label: '仪表盘', icon: 'dashboard', exact: true }] },
  {
    title: '配置',
    items: [
      { to: '/targets', label: '备份目标', icon: 'target' },
      { to: '/channels', label: '备份渠道', icon: 'cloud' },
      { to: '/notifiers', label: '通知渠道', icon: 'bell' },
    ],
  },
  { title: '记录', items: [{ to: '/history', label: '备份历史', icon: 'history' }] },
]

const title = computed(() => route.meta.title || 'ArchiveSync')
const initials = computed(() => (auth.user?.name || auth.user?.username || '?').slice(0, 1).toUpperCase())

function isActive(item) {
  return item.exact ? route.path === '/' : route.path.startsWith(item.to)
}
function flipTheme() { theme.value = toggleTheme() }
</script>

<template>
  <div class="layout">
    <Transition name="fade">
      <div v-if="drawerOpen" class="drawer-scrim" @click="drawerOpen = false" />
    </Transition>

    <aside class="sidebar" :class="{ open: drawerOpen }">
      <div class="brand">
        <span class="brand-mark"><Icon name="archive" :size="20" /></span>
        <span class="stack">
          <span class="brand-name">ArchiveSync</span>
          <span class="brand-sub">备份同步控制台</span>
        </span>
      </div>

      <nav class="nav">
        <template v-for="s in sections" :key="s.title">
          <div class="nav-label">{{ s.title }}</div>
          <RouterLink
            v-for="item in s.items" :key="item.to" :to="item.to"
            class="nav-item" :class="{ active: isActive(item) }" @click="drawerOpen = false"
          >
            <span class="nav-ico"><Icon :name="item.icon" :size="19" /></span>
            <span>{{ item.label }}</span>
          </RouterLink>
        </template>
      </nav>

      <div class="sidebar-foot">
        <div class="user-chip">
          <img v-if="auth.user?.picture" :src="auth.user.picture" class="avatar" alt="" />
          <span v-else class="avatar avatar-fallback">{{ initials }}</span>
          <span class="stack" style="min-width:0;flex:1">
            <span class="u-name">{{ auth.user?.name || auth.user?.username }}</span>
            <span class="u-mail faint">{{ auth.user?.email || '本地会话' }}</span>
          </span>
          <button class="btn btn-ghost icon-btn btn-sm" title="退出登录" @click="auth.logout()">
            <Icon name="logout" :size="17" />
          </button>
        </div>
      </div>
    </aside>

    <main class="content">
      <header class="topbar">
        <div class="tb-left">
          <button class="btn btn-ghost icon-btn tb-menu" aria-label="菜单" @click="drawerOpen = true">
            <Icon name="menu" :size="19" />
          </button>
          <span class="tb-title">{{ title }}</span>
        </div>
        <div class="tb-right">
          <span class="badge badge-neutral tb-iam">
            <span class="dot" style="color:var(--ok)" /> IAM 已登录
          </span>
          <button class="btn btn-ghost icon-btn" :title="theme === 'dark' ? '切换到浅色' : '切换到深色'" @click="flipTheme">
            <Icon :name="theme === 'dark' ? 'sun' : 'moon'" :size="18" />
          </button>
        </div>
      </header>

      <div class="page">
        <RouterView v-slot="{ Component }">
          <Transition name="route" mode="out-in">
            <div :key="route.path" class="view-scope">
              <component :is="Component" />
            </div>
          </Transition>
        </RouterView>
      </div>
    </main>
  </div>
</template>

<style scoped>
.u-name { font-weight: 560; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.u-mail { font-size: 11.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.user-chip { padding: 4px; }

.tb-left { display: flex; align-items: center; gap: 12px; }
.tb-title { font-weight: 620; font-size: 15px; letter-spacing: -0.01em; }
.tb-right { display: flex; align-items: center; gap: 10px; }
.tb-iam { font-weight: 540; }
.tb-menu { display: none; }

.drawer-scrim { position: fixed; inset: 0; z-index: 39; background: rgba(10, 12, 17, 0.5); }

@media (max-width: 820px) {
  .tb-menu { display: inline-grid; }
  .tb-iam { display: none; }
}
</style>
