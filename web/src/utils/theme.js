// Minimal theme controller. When the user picks a theme it is stamped on the
// root element (design.css reads [data-theme]) and persisted; otherwise the OS
// preference (prefers-color-scheme) applies.
const KEY = 'as-theme'

export function initTheme() {
  const t = localStorage.getItem(KEY)
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t
}

export function currentTheme() {
  return (
    document.documentElement.dataset.theme ||
    (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  )
}

export function toggleTheme() {
  const next = currentTheme() === 'dark' ? 'light' : 'dark'
  document.documentElement.dataset.theme = next
  localStorage.setItem(KEY, next)
  return next
}
