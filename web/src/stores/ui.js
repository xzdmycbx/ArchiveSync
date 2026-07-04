import { defineStore } from 'pinia'

let seq = 0

// ui store drives the global toast notifications and the confirm dialog.
export const useUI = defineStore('ui', {
  state: () => ({ toasts: [], dialog: null }),
  actions: {
    push(type, text) {
      const t = { id: ++seq, type, text }
      this.toasts.push(t)
      setTimeout(() => this.dismiss(t.id), 4200)
    },
    ok(t) { this.push('ok', t) },
    err(t) { this.push('err', t) },
    info(t) { this.push('info', t) },
    dismiss(id) { this.toasts = this.toasts.filter((x) => x.id !== id) },

    // confirm opens the styled dialog and resolves true/false.
    confirm(opts = {}) {
      return new Promise((resolve) => {
        this.dialog = {
          title: opts.title || '确认操作',
          message: opts.message || '',
          confirmText: opts.confirmText || '确定',
          cancelText: opts.cancelText || '取消',
          danger: !!opts.danger,
          _resolve: resolve,
        }
      })
    },
    resolveDialog(value) {
      if (this.dialog) {
        this.dialog._resolve(value)
        this.dialog = null
      }
    },
  },
})
