import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { Category, Currency, Group, Settings, User } from './types'
import { setDateFormats } from './money'

export type ThemeMode = 'system' | 'light' | 'dark'

interface AuthStatus { needs_setup: boolean; auth_mode: string; authenticated: boolean; user: User | null; version: string; title: string; oidc?: boolean; oidc_name?: string }

interface AppState {
  status: AuthStatus | null
  user: User | null
  settings: Settings | null
  users: User[]
  currencies: Currency[]
  categories: Category[]
  /** Your active groups, for the sidebar, the palette and the "add expense" picker. */
  groups: Group[]
  /** Your archived groups. */
  archived: Group[]
  themeMode: ThemeMode
  theme: 'light' | 'dark'
  setThemeMode: (m: ThemeMode) => void
  toggleTheme: () => void
  refresh: () => Promise<void>
  reloadMeta: () => Promise<void>
  reloadGroups: () => Promise<void>
  setUser: (u: User | null) => void
  setSettings: (s: Settings) => void
}

const Ctx = createContext<AppState>(null!)
export function useApp() { return useContext(Ctx) }

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [user, setUser] = useState<User | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [currencies, setCurrencies] = useState<Currency[]>([])
  const [categories, setCategories] = useState<Category[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [archived, setArchived] = useState<Group[]>([])
  // Follows the system unless set to light or dark (ps-theme-mode keeps an explicit choice; the old ps-theme was written on every load, so it is ignored).
  const [themeMode, setThemeModeRaw] = useState<ThemeMode>(() => (localStorage.getItem('ps-theme-mode') as ThemeMode) || 'system')
  const [systemDark, setSystemDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const h = () => setSystemDark(mq.matches)
    mq.addEventListener('change', h)
    return () => mq.removeEventListener('change', h)
  }, [])
  const theme: 'light' | 'dark' = themeMode === 'system' ? (systemDark ? 'dark' : 'light') : themeMode
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    document.querySelectorAll('meta[name="theme-color"]').forEach(m => m.setAttribute('content', theme === 'dark' ? '#111113' : '#ffffff'))
  }, [theme])
  const setThemeMode = useCallback((m: ThemeMode) => { setThemeModeRaw(m); if (m === 'system') localStorage.removeItem('ps-theme-mode'); else localStorage.setItem('ps-theme-mode', m) }, [])

  useEffect(() => { if (settings?.title) document.title = settings.title }, [settings?.title])
  useEffect(() => { if (settings) setDateFormats(settings.date_format || 'auto', settings.time_format || 'auto') }, [settings?.date_format, settings?.time_format]) // eslint-disable-line react-hooks/exhaustive-deps

  const reloadGroups = useCallback(async () => {
    const [a, b] = await Promise.all([api.get<Group[]>(`${V1}/groups?archived=0`), api.get<Group[]>(`${V1}/groups?archived=1`).catch(() => [] as Group[])])
    setGroups(a || []); setArchived(b || [])
  }, [])

  const reloadMeta = useCallback(async () => {
    const [u, c, k] = await Promise.all([api.get<User[]>(`${V1}/users`), api.get<Currency[]>(`${V1}/currencies`), api.get<Category[]>(`${V1}/categories`)])
    setUsers(u); setCurrencies(c); setCategories(k)
  }, [])

  const refresh = useCallback(async () => {
    const st = await api.get<AuthStatus>(`${V1}/auth/status`)
    setStatus(st)
    setUser(st.user)
    if (st.authenticated) {
      setSettings(await api.get<Settings>(`${V1}/settings`))
      await Promise.all([reloadMeta(), reloadGroups().catch(() => {})])
    }
  }, [reloadMeta, reloadGroups])

  useEffect(() => { refresh().catch(() => setStatus({ needs_setup: false, auth_mode: 'local', authenticated: false, user: null, version: '', title: 'Pocket Split' })) }, [refresh])
  useEffect(() => {
    const h = () => { setUser(null); setStatus(s => s ? { ...s, authenticated: false, user: null } : s) }
    window.addEventListener('ps:unauthorized', h)
    return () => window.removeEventListener('ps:unauthorized', h)
  }, [])

  return <Ctx.Provider value={{ status, user, settings, users, currencies, categories, groups, archived, themeMode, theme, setThemeMode, toggleTheme: () => setThemeMode(theme === 'dark' ? 'light' : 'dark'), refresh, reloadMeta, reloadGroups, setUser, setSettings }}>{children}</Ctx.Provider>
}
