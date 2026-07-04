import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '../stores/auth'
import AppShell from '../components/AppShell.vue'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/Login.vue'), meta: { public: true } },
  {
    path: '/',
    component: AppShell,
    children: [
      { path: '', name: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '仪表盘' } },
      { path: 'channels', name: 'channels', component: () => import('../views/Channels.vue'), meta: { title: '备份渠道' } },
      { path: 'channels/:id', name: 'channel-detail', component: () => import('../views/ChannelDetail.vue'), meta: { title: '渠道详情' } },
      { path: 'notifiers', name: 'notifiers', component: () => import('../views/Notifiers.vue'), meta: { title: '通知渠道' } },
      { path: 'notifiers/:id', name: 'notifier-detail', component: () => import('../views/NotifierDetail.vue'), meta: { title: '通知详情' } },
      { path: 'targets', name: 'targets', component: () => import('../views/Targets.vue'), meta: { title: '备份目标' } },
      { path: 'targets/:id', name: 'target-detail', component: () => import('../views/TargetDetail.vue'), meta: { title: '目标详情' } },
      { path: 'history', name: 'history', component: () => import('../views/History.vue'), meta: { title: '备份历史' } },
    ],
  },
  { path: '/:pathMatch(.*)*', component: () => import('../views/NotFound.vue'), meta: { public: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  if (!auth.loaded) await auth.fetchMe()
  if (to.meta.public) {
    if (to.name === 'login' && auth.isAuthed) return { path: '/' }
    return true
  }
  if (!auth.isAuthed) return { path: '/login', query: { return_to: to.fullPath } }
  return true
})

export default router
