<script setup>
import { useAuth } from '../stores/auth'
import { useRoute } from 'vue-router'
import Icon from '../components/Icon.vue'

const auth = useAuth()
const route = useRoute()

function login() {
  auth.login(route.query.return_to || '/')
}

const points = [
  { icon: 'cloud', text: 'S3 / R2 / MinIO 与本地目录，多渠道分发' },
  { icon: 'layers', text: '分层保留策略，精确到每日快照' },
  { icon: 'bell', text: 'Discord / Telegram / 邮件 / Webhook 通知' },
]
</script>

<template>
  <div class="login-wrap">
    <span class="blob blob-a" />
    <span class="blob blob-b" />

    <div class="card login-card rise">
      <div class="login-logo"><Icon name="archive" :size="30" /></div>
      <h1 class="login-title">ArchiveSync</h1>
      <p class="login-tag">备份同步控制台</p>

      <ul class="login-points">
        <li v-for="p in points" :key="p.text">
          <span class="lp-ico"><Icon :name="p.icon" :size="16" /></span>
          <span>{{ p.text }}</span>
        </li>
      </ul>

      <button class="btn btn-primary btn-lg btn-block" @click="login">
        <Icon name="shield" :size="18" /> 使用 TransCircle IAM 登录
      </button>
      <p class="login-foot">通过统一身份认证 · OIDC + PKCE 安全登录</p>
    </div>
  </div>
</template>

<style scoped>
.login-title { font-size: 26px; letter-spacing: -0.03em; margin-bottom: 4px; }
.login-tag { color: var(--text-muted); margin-bottom: 26px; }
.login-points { list-style: none; padding: 0; margin: 0 0 26px; display: flex; flex-direction: column; gap: 12px; text-align: left; }
.login-points li { display: flex; align-items: center; gap: 11px; font-size: 13.5px; color: var(--text-muted); }
.lp-ico { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; background: var(--accent-soft); color: var(--accent); flex: 0 0 32px; }
.login-foot { font-size: 12px; color: var(--text-faint); margin-top: 18px; }

.blob { position: absolute; border-radius: 50%; filter: blur(72px); opacity: 0.5; z-index: 0; pointer-events: none; }
.blob-a { width: 460px; height: 460px; background: radial-gradient(circle, #5661e8, transparent 70%); top: -140px; left: -120px; animation: floaty 9s var(--ease) infinite; }
.blob-b { width: 420px; height: 420px; background: radial-gradient(circle, #22b8cf, transparent 70%); bottom: -160px; right: -120px; animation: floaty 11s var(--ease) infinite reverse; }
</style>
