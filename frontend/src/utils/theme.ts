/** Dark is the default on every route; only an explicit light choice overrides it. */
export function initializeTheme(): boolean {
  let savedTheme: string | null = null
  try {
    savedTheme = localStorage.getItem('theme')
  } catch {
    // Storage may be unavailable; keep the same dark default, not the OS theme.
  }
  const isDark = savedTheme !== 'light'
  document.documentElement.classList.toggle('dark', isDark)
  return isDark
}
