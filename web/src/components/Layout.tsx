// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useMatch, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Group } from '../lib/types'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { GroupPicker } from './people'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

const SIDEBAR_KEY = 'ps-sidebar'

export default function Layout() {
  const { groups, archived, reloadGroups } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  // balances move whenever something is added or edited: refresh the group list on every page change
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }); reloadGroups().catch(() => {}) }, [loc.pathname]) // eslint-disable-line react-hooks/exhaustive-deps

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [pick, setPick] = useState(false)

  // the group on screen
  const gm = useMatch('/groups/:id/*')
  const current = [...groups, ...archived].find(g => String(g.id) === gm?.params.id)

  /** Add an expense: to the given group, the one on screen, the only one, or pick. */
  const addExpense = (g?: Group) => {
    closeDrawer()
    const target = g || (current && !current.archived ? current : undefined) || (groups.length === 1 ? groups[0] : undefined)
    if (target) navigate(`/groups/${target.id}/expenses/new`)
    else if (groups.length === 0) navigate('/groups?new=1')
    else setPick(true)
  }
  const newGroup = () => { closeDrawer(); navigate('/groups?new=1') }
  const addRef = useRef(addExpense)
  addRef.current = addExpense

  useEffect(() => {
    const open = () => setShortcuts(true)
    window.addEventListener('ps:shortcuts', open)
    return () => window.removeEventListener('ps:shortcuts', open)
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t?.closest?.('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'e': e.preventDefault(); addRef.current(); break
        case 'n': e.preventDefault(); newGroup(); break
        case 'h': go('/home'); break
        case 'g': go('/groups'); break
        case 'r': go('/reports'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const target = current && !current.archived ? current : groups.length === 1 ? groups[0] : undefined
  const commands: PaletteCommand[] = [
    { id: 'expense', label: target ? `Add an expense — ${target.name}` : 'Add an expense', icon: 'plus', hint: 'E', keywords: 'new cost paid bill', run: () => addExpense() },
    ...(current && !current.archived ? [{ id: 'pay', label: `Settle up — ${current.name}`, icon: 'transfer', keywords: 'payment reimbursement pay back balances', run: () => navigate(`/groups/${current.id}/balances`) }] : []),
    { id: 'group', label: 'New group', icon: 'user-plus', hint: 'N', keywords: 'create trip flat', run: newGroup },
    { id: 'home', label: 'Home', icon: 'home', hint: 'H', run: () => navigate('/home') },
    { id: 'groups', label: 'Groups', icon: 'users', hint: 'G', run: () => navigate('/groups') },
    { id: 'reports', label: 'Reports', icon: 'chart', hint: 'R', keywords: 'pdf export', run: () => navigate('/reports') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'categories', label: 'Categories', icon: 'tag', keywords: 'settings', run: () => navigate('/settings/categories') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
      {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
      <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
        <Sidebar onAdd={addExpense} onNewGroup={newGroup} onNavigate={closeDrawer} current={current} />
      </div>
      <div className="main">
        <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} group={current} />
        <main className="content"><div className="content-inner"><Outlet /></div></main>
      </div>
      {mobile && !/\/expenses\//.test(loc.pathname) && <button type="button" className="fab" aria-label="Add an expense" title="Add an expense" onClick={() => addExpense()}><Icon name="plus" size={26} strokeWidth={2.2} /></button>}
      {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
      {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
      {pick && <GroupPicker groups={groups} onClose={() => setPick(false)} />}
    </div>
  )
}

const SECTION: Record<string, string> = { home: 'Home', groups: 'Groups', reports: 'Reports' }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, group }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; group?: Group }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const parts = loc.pathname.split('/')
  const section = SECTION[parts[1] || ''] || ''
  const leaf = parts[3] === 'expenses' ? (parts[4] === 'new' ? 'New expense' : 'Expense') : ''
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('ps:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {group ? <>
          {!mobile && <><button type="button" className="crumb" onClick={() => navigate('/groups')}>Groups</button><Icon name="chevron-right" size={16} className="crumb-sep" /></>}
          {leaf ? <><button type="button" className="crumb ellipsis" onClick={() => navigate(`/groups/${group.id}`)}>{group.name}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{leaf}</span></> : <span className="ellipsis">{group.name}</span>}
        </> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      {mobile && <AskButton />}
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
