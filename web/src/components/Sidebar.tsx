// Left column: add button, sections, your groups, settings — the Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import type { Group } from '../lib/types'
import { decimalsOf, fmtMoney } from '../lib/money'
import { Icon } from './icons'
import { GroupIcon } from './people'
import { type Anchor, type MenuItem, Menu, rectOf } from './ui'

export function Sidebar({ onAdd, onNewGroup, onNavigate, current }: { onAdd: (g?: Group) => void; onNewGroup: () => void; onNavigate: () => void; current?: Group }) {
  const { groups, archived, status, settings } = useApp()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Split'
  const ordered = current && !current.archived ? [current, ...groups.filter(g => g.id !== current.id)] : groups
  const addItems: MenuItem[] = [
    ...(ordered.length ? [{ heading: 'Add an expense to' }] : []),
    ...ordered.slice(0, 8).map((g, i) => ({ label: g.name, icon: 'receipt', hint: i === 0 ? 'E' : undefined, onSelect: () => onAdd(g) })),
    ...(ordered.length > 8 ? [{ label: 'Another group…', icon: 'more', onSelect: () => onAdd() }] : []),
    ...(ordered.length ? ['sep' as const] : []),
    { label: 'New group', icon: 'user-plus', hint: 'N', onSelect: onNewGroup },
  ]
  const nav = [
    { to: '/home', label: 'Home', icon: 'home' },
    { to: '/groups', label: 'Groups', icon: 'users', badge: groups.length, end: true },
    { to: '/reports', label: 'Reports', icon: 'chart' },
  ]
  return (
    <aside className="sidebar" aria-label="Split">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Split' ? title : <>Pocket <b>Split</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
        <Icon name="plus" size={17} strokeWidth={2.2} />Add expense<Icon name="chevron-down" size={15} className="create-caret" />
      </button>
      {menu && <Menu anchor={menu} items={addItems} onClose={() => setMenu(null)} width={248} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}{!!n.badge && <span className="badge" title={`${n.badge} active ${n.badge === 1 ? 'group' : 'groups'}`}>{n.badge}</span>}
          </NavLink>
        ))}
      </nav>

      <GroupList title="My groups" list={groups} onAdd={onAdd} onNavigate={onNavigate} add={onNewGroup} />
      {archived.length > 0 && <GroupList title="Archived" list={archived} onAdd={onAdd} onNavigate={onNavigate} closed />}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('ps:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}

function GroupList({ title, list, onAdd, onNavigate, add, closed }: { title: string; list: Group[]; onAdd: (g?: Group) => void; onNavigate: () => void; add?: () => void; closed?: boolean }) {
  const [open, setOpen] = useState(!closed)
  return (
    <section className="sb-sec">
      <h4 className="sb-h">
        <button type="button" className="sb-h-btn" aria-expanded={open} onClick={() => setOpen(o => !o)}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} />{title}</button>
        {add && <button type="button" className="btn ghost icon-only xs" aria-label="New group" title="New group (n)" onClick={add}><Icon name="plus" size={14} /></button>}
      </h4>
      {open && (list.length === 0
        ? <p className="sb-empty">No groups yet. <button type="button" className="link-btn" onClick={add}>Create one</button></p>
        : <ul className="cal-list">{list.map(g => <GroupRow key={g.id} g={g} onAdd={onAdd} onNavigate={onNavigate} />)}</ul>)}
    </section>
  )
}

function GroupRow({ g, onAdd, onNavigate }: { g: Group; onAdd: (g?: Group) => void; onNavigate: () => void }) {
  const { currencies } = useApp()
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const go = (tab = '') => { onNavigate(); navigate(`/groups/${g.id}${tab ? `/${tab}` : ''}`) }
  const items: MenuItem[] = [
    { label: 'Open', icon: 'receipt', onSelect: () => go() },
    ...(!g.archived ? [{ label: 'Add an expense', icon: 'plus', onSelect: () => onAdd(g) }] : []),
    'sep',
    { label: 'Balances', icon: 'scale', onSelect: () => go('balances') },
    { label: 'Stats', icon: 'pie', onSelect: () => go('stats') },
    { label: 'Activity', icon: 'activity', onSelect: () => go('activity') },
    { label: 'Report', icon: 'chart', onSelect: () => { onNavigate(); navigate(`/reports?group=${g.id}`) } },
    'sep',
    { label: 'Group settings', icon: 'settings', onSelect: () => go('settings') },
  ]
  const bal = g.my_member_id ? g.my_balance : 0
  return (
    <li className={`cal-row grp-row ${g.archived ? 'off' : ''}`}>
      <NavLink to={`/groups/${g.id}`} onClick={onNavigate} className={({ isActive }) => `cal-check ${isActive ? 'is-active' : ''}`} title={`${g.name} · ${g.currency}`}>
        <GroupIcon group={g} />
        <span className="cal-name ellipsis">{g.name}</span>
        {bal !== 0
          ? <span className={`grp-meta ${bal > 0 ? 'ok' : 'bad'}`} title={bal > 0 ? 'You get back' : 'You owe'}>{fmtMoney(bal, g.currency, decimalsOf(g.currency, currencies), { sign: true })}</span>
          : <span className="grp-meta">{g.currency}</span>}
      </NavLink>
      <button type="button" className="btn ghost icon-only xs cal-more" aria-label={`Options for ${g.name}`} aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(rectOf(e.currentTarget))}><Icon name="more" size={15} /></button>
      {menu && <Menu anchor={menu} items={items} onClose={() => setMenu(null)} width={210} />}
    </li>
  )
}
