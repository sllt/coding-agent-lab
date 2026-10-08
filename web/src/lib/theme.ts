export type Theme = 'light' | 'dark' | 'system'

const KEY = 'agentlab.theme'

export function storedTheme(): Theme {
  const v = typeof localStorage !== 'undefined' ? localStorage.getItem(KEY) : null
  return v === 'light' || v === 'dark' ? v : 'system'
}

export function resolvedDark(theme: Theme): boolean {
  if (theme === 'system') return typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches
  return theme === 'dark'
}

/** applyTheme sets the class on <html>. No inline script is needed (CSP). */
export function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', resolvedDark(theme))
  if (theme === 'system') localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, theme)
}
