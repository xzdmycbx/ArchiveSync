import { defineStore } from 'pinia'
import api from '../api/client'

// auth store holds the current admin session, populated from /api/auth/me.
export const useAuth = defineStore('auth', {
  state: () => ({
    user: null,
    loaded: false,
  }),
  getters: {
    isAuthed: (s) => !!s.user,
  },
  actions: {
    async fetchMe() {
      try {
        const { data } = await api.get('/auth/me')
        this.user = data
      } catch {
        this.user = null
      } finally {
        this.loaded = true
      }
    },
    // Full-page navigation to the IAM login flow (not an XHR).
    login(returnTo) {
      const rt = returnTo || window.location.pathname
      window.location.href = `/api/auth/login?return_to=${encodeURIComponent(rt)}`
    },
    async logout() {
      try {
        await api.post('/auth/logout')
      } catch {
        /* ignore */
      }
      this.user = null
      window.location.href = '/login'
    },
  },
})
