import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Category, Group, Settings as SettingsT, User } from '../lib/types'
import { Confirm, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, copyText, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Avatar } from '../components/people'
import { CurrencySelect } from '../components/Money'
import { Select } from '../components/Select'
import { DATE_FORMATS, TIME_FORMATS, fmtDateTime, fmtYMD, fmtTime } from '../lib/money'

type TabId = 'general' | 'categories' | 'users' | 'profile' | 'api' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'Your default currency, date and time formats; the app title and exchange-rate lookups for the box.' },
  { id: 'categories', label: 'Categories', icon: 'tag', sub: 'What an expense is for: the built-in list and your own.' },
  { id: 'users', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'api', label: 'API tokens', icon: 'key', sub: 'Personal tokens for widgets, scripts and iOS Shortcuts.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Exports, backups and importing a group.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  // the old /settings?tab=users links land on /settings/users
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  useEffect(() => { if (!raw && legacy && TABS.some(t => t.id === legacy)) navigate(`/settings/${legacy}`, { replace: true }) }, [raw, legacy, navigate])
  const want = raw || legacy
  const tab: TabId = TABS.find(t => t.id === want)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })

  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to your groups" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'categories' && <Categories />}
            {tab === 'users' && <Users />}
            {tab === 'profile' && <Profile />}
            {tab === 'api' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function General() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!f) return <Loading />
  const isAdmin = user?.role === 'admin'
  const save = async () => {
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, f); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const now = new Date()
  return (
    <div className="stack loose">
      <Section title="Yours" sub="Everyone on the box picks their own.">
        <SettingRow title="Default currency for new groups" help="Every group can pick its own."><CurrencySelect value={f.default_currency} onChange={c => setF({ ...f, default_currency: c })} /></SettingRow>
        <SettingRow title="Date format" help={`Today: ${fmtYMD(now.getFullYear(), now.getMonth() + 1, now.getDate(), true, f.date_format)}`}>
          <Select value={f.date_format || 'auto'} onChange={v => setF({ ...f, date_format: v as SettingsT['date_format'] })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} ariaLabel="Date format" />
        </SettingRow>
        <SettingRow title="Time format" help={`Now: ${fmtTime(now, f.time_format)}`}>
          <Select value={f.time_format || 'auto'} onChange={v => setF({ ...f, time_format: v as SettingsT['time_format'] })} options={TIME_FORMATS.map(t => ({ value: t.id, label: t.label }))} ariaLabel="Time format" />
        </SettingRow>
      </Section>
      <Section title="This box" sub={isAdmin ? 'These apply to everyone on this box.' : 'Set by an admin for everyone.'}>
        <SettingRow title="App title" help="Shown in the sidebar and the browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} style={{ width: 220 }} aria-label="App title" /></SettingRow>
        <SettingRow title="Look exchange rates up automatically" help={!f.fx_enabled ? 'Lookups are currently off on the server.' : 'The day\u2019s rate is filled in when an expense is in another currency.'}>
          <Switch checked={f.fx_auto} onChange={v => setF({ ...f, fx_auto: v })} disabled={!isAdmin} label="Look exchange rates up automatically" />
        </SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button></div>
      <Section title="Exchange rates" icon="coins" sub="When an expense is paid in a currency other than the group's, the app looks the day's rate up and stores it on the expense, so balances never shift afterwards. You can always type a different rate (what your bank actually charged).">
        <dl className="set-dl">
          <div><dt>Frankfurter</dt><dd className="muted">European Central Bank reference rates, ~30 major currencies, with history: the rate for the expense's date.</dd></div>
          <div><dt>open.er-api.com</dt><dd className="muted">~160 currencies (BAM, RSD, …), today's rate only. Used when Frankfurter does not cover a pair.</dd></div>
          <div><dt>On this server</dt><dd><code>{f.fx_provider}</code>{!f.fx_enabled && <span className="muted"> (lookups are currently off)</span>}<div className="muted small">Rates are cached once fetched, one per currency pair and day.</div></dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title={`${users.length} ${users.length === 1 ? 'account' : 'accounts'}`} sub="Everyone signs in with their own account and sees only the groups they created or are linked to. Link a group member to an account under the group's Settings → People. Admins manage users and box-wide settings; they do not see other people's groups."
        actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="user-plus" size={14} />Add user</button>}>
        <div className="set-people">
          {users.map(u => (
            <div key={u.id} className="set-person">
              <Avatar user={u} size="lg" />
              <div className="grow">
                <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}<span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></div>
                <div className="muted small ellipsis">{u.email}</div>
              </div>
              {isAdmin && <span className="row"><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
            </div>
          ))}
        </div>
      </Section>
      {status?.auth_mode !== 'local' && <div className="callout info"><Icon name="info" size={15} /><span>Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</span></div>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account is deleted. Group members linked to it stay in their groups, unlinked, so no expense or balance is lost." confirmLabel="Remove" onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </div>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f)
      toast(user.id ? 'User saved' : 'User added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters. Tell them to change it under Settings → Profile."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <Section title="You">
        <div className="set-profile">
          {user && <Avatar user={user} size="xl" />}
          <div className="stack grow">
            <Field label="Name" help="Shown to others; group members you are linked to keep their own name."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
            <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
          </div>
        </div>
      </Section>
      {status?.auth_mode === 'local' && (
        <Section title="Password" sub="At least 8 characters. Leave both empty to keep the current one.">
          <div className="stack">
            <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
            <Field label="New password"><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
          </div>
        </Section>
      )}
      <div className="row"><button type="submit" className="btn primary" disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="New token" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you and sees your groups. Good for a Homepage widget (<code>GET /api/v1/dashboard</code>) or adding an expense from an iOS Shortcut (<code>POST /api/v1/groups/{'{id}'}/expenses</code>). See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" aria-label="Token name" /><button type="submit" className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && (
          <div className="set-fresh">
            <div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div>
            <div className="copy-field"><input readOnly value={fresh} onFocus={e => e.target.select()} aria-label="New token" /><button type="button" className="btn sm" onClick={async () => toast(await copyText(fresh) ? 'Copied' : 'Could not copy')}><Icon name="copy" size={14} />Copy</button></div>
          </div>
        )}
      </Section>
      <Section title="Your tokens">
        {data && data.length > 0 ? (
          <div className="set-people">
            {data.map(t => (
              <div key={t.id} className="set-person">
                <span className="set-token-ico"><Icon name="key" size={16} /></span>
                <div className="grow">
                  <div className="set-person-name">{t.name}<span className="mono muted small">{t.prefix}…</span></div>
                  <div className="muted small">Last used {t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'} · created {fmtDateTime(t.created_at)}</div>
                </div>
                <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button>
              </div>
            ))}
          </div>
        ) : <p className="muted">No tokens yet.</p>}
      </Section>
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." confirmLabel="Revoke" onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </div>
  )
}

function Data() {
  const { toast } = useToast()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<{ group: Group; created: number; skipped: number }>(`${V1}/import`, JSON.parse(await file.text())); toast(`Imported ${r.created} expenses${r.skipped ? `, ${r.skipped} skipped` : ''}`); navigate(`/groups/${r.group.id}`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      <Section title="Backup" icon="database" sub={<>Export any of your groups as JSON or CSV under its Settings → Export. The whole database holds everyone's groups, so it is not downloadable here: whoever runs the box backs up the data directory (<code>pocket-split backup</code>).</>} />
      <Section title="Import a group" icon="upload" sub="A JSON export from any group's Settings → Export becomes a new group here, with its people and expenses, at the exchange rates it was saved with.">
        <label className="btn"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Import group JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
      </Section>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <div className="stack loose">
      <Section title="Pocket Split">
        <div className="set-about-hero">
          <img src="/favicon.svg" alt="" width={48} height={48} />
          <div>
            <h3>Pocket Split {data?.version ? <span className="badge">{String(data.version)}</span> : null}</h3>
            <p className="muted">Shared costs for trips, flats and dinners: who paid what, in any currency, and who owes whom. One binary, one SQLite file, one volume.</p>
          </div>
        </div>
        <p className="muted small mt">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-cost-split" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-cost-split</a>. Sibling projects: Pocket Habits, Pocket Inventory, Pocket Bookshelf and Pocket Invoicing. Exchange rates by Frankfurter (ECB) and ExchangeRate-API.</p>
      </Section>
      {data && <>
        <div className="set-stats">
          <div className="set-stat"><Icon name="users" size={16} /><span className="set-stat-n">{counts.groups ?? 0}</span><span className="set-stat-l">groups</span></div>
          <div className="set-stat"><Icon name="user" size={16} /><span className="set-stat-n">{counts.members ?? 0}</span><span className="set-stat-l">members</span></div>
          <div className="set-stat"><Icon name="receipt" size={16} /><span className="set-stat-n">{counts.expenses ?? 0}</span><span className="set-stat-l">expenses</span></div>
          <div className="set-stat"><Icon name="key" size={16} /><span className="set-stat-n">{counts.users ?? 0}</span><span className="set-stat-l">users</span></div>
        </div>
        <Section title="Server">
          <dl className="set-dl">
            <div><dt>Version</dt><dd className="mono">{String(data.version)}</dd></div>
            <div><dt>Runtime</dt><dd className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</dd></div>
            <div><dt>Auth mode</dt><dd className="mono">{String(data.auth_mode)}</dd></div>
            <div><dt>Exchange rates</dt><dd className="mono">{String(data.fx_provider)}</dd></div>
            <div><dt>Database</dt><dd className="mono">{String(data.db_path)} · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</dd></div>
            <div><dt>Timezone</dt><dd className="mono">{String(data.timezone)}</dd></div>
            <div><dt>Uptime</dt><dd className="mono">{String(data.uptime)}</dd></div>
          </dl>
        </Section>
      </>}
    </div>
  )
}

function Categories() {
  const { user, categories, reloadMeta } = useApp()
  const { toast } = useToast()
  const isAdmin = user?.role === 'admin'
  // Your own categories are yours to change; the built-in list is box-wide and an admin's.
  const canEdit = (c: Category) => c.mine || (c.builtin && isAdmin)
  const [draft, setDraft] = useState<Record<string, { label: string; icon: string }>>({})
  const [add, setAdd] = useState({ label: '', icon: '' })
  const [del, setDel] = useState<Category | null>(null)
  const [busy, setBusy] = useState(false)
  const run = async (fn: () => Promise<unknown>, msg: string) => { setBusy(true); try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  const d = (c: Category) => draft[c.id] || { label: c.label, icon: c.icon }
  const changed = (c: Category) => draft[c.id] && (draft[c.id].label !== c.label || draft[c.id].icon !== c.icon)
  const save = (c: Category) => run(async () => { await api.put(`${V1}/categories/${c.id}`, d(c)); setDraft(x => { const y = { ...x }; delete y[c.id]; return y }) }, 'Saved')
  const move = (c: Category, dir: -1 | 1) => {
    const i = categories.findIndex(x => x.id === c.id), j = i + dir
    if (j < 0 || j >= categories.length) return
    const o = categories[j]
    if (!canEdit(o)) return
    run(async () => { await api.put(`${V1}/categories/${c.id}`, { sort_order: j + 1 }); await api.put(`${V1}/categories/${o.id}`, { sort_order: i + 1 }) }, 'Moved')
  }
  const create = (e: React.FormEvent) => { e.preventDefault(); if (!add.label.trim()) return; run(async () => { await api.post(`${V1}/categories`, add); setAdd({ label: '', icon: '' }) }, 'Category added') }
  return (
    <div className="stack loose">
      <Section title="Categories" sub={<>Add your own here or straight from the expense form by typing a name that is not in the list; they are yours (others see one only where it is used in a group they share with you). Rename, reorder or remove your own here; expenses of a removed category move to General. {isAdmin ? 'As an admin you also edit the built-in ones, for everyone.' : 'The built-in ones are set by an admin.'}</>}>
        <div className="cat-list">
          {categories.map((c, i) => (
            <div key={c.id} className="cat-row">
              <input className="cat-icon" value={d(c).icon} onChange={e => setDraft({ ...draft, [c.id]: { ...d(c), icon: e.target.value } })} maxLength={8} disabled={!canEdit(c)} aria-label={`Icon for ${c.label}`} />
              <input className="cat-name" value={d(c).label} onChange={e => setDraft({ ...draft, [c.id]: { ...d(c), label: e.target.value } })} maxLength={60} disabled={!canEdit(c)} aria-label={`Name of ${c.label}`} />
              <span className="cat-used muted small">{c.count} {c.count === 1 ? 'expense' : 'expenses'}{c.builtin && <span className="badge">built-in</span>}{!c.builtin && !c.mine && <span className="badge">shared</span>}</span>
              <span className="cat-acts">
                {canEdit(c) && <>
                  {changed(c) && <button type="button" className="btn primary sm" onClick={() => save(c)} disabled={busy}>Save</button>}
                  <button type="button" className="btn ghost icon-only sm" onClick={() => move(c, -1)} disabled={busy || i === 0 || !canEdit(categories[i - 1])} title="Move up" aria-label={`Move ${c.label} up`}><Icon name="arrow-up" size={15} /></button>
                  <button type="button" className="btn ghost icon-only sm" onClick={() => move(c, 1)} disabled={busy || i === categories.length - 1 || !canEdit(categories[i + 1])} title="Move down" aria-label={`Move ${c.label} down`}><Icon name="arrow-down" size={15} /></button>
                  {c.id !== 'general' && <button type="button" className="btn ghost icon-only sm danger-text" onClick={() => setDel(c)} disabled={busy} title="Remove" aria-label={`Remove ${c.label}`}><Icon name="trash" size={15} /></button>}
                </>}
              </span>
            </div>
          ))}
        </div>
      </Section>
      <Section title="Add a category" sub="The icon is any emoji; leave it empty for 🏷️.">
        <form onSubmit={create} className="row wrap">
          <input className="cat-icon" value={add.icon} onChange={e => setAdd({ ...add, icon: e.target.value })} placeholder="🏷️" maxLength={8} aria-label="Icon" />
          <input value={add.label} onChange={e => setAdd({ ...add, label: e.target.value })} placeholder="New category, e.g. Ski passes" maxLength={60} style={{ flex: '1 1 200px', width: 'auto' }} aria-label="New category name" />
          <button type="submit" className="btn primary" disabled={busy || !add.label.trim()}><Icon name="plus" size={15} />Add</button>
        </form>
      </Section>
      {del && <Confirm title={`Remove “${del.label}”?`} message={`${del.count} ${del.count === 1 ? 'expense moves' : 'expenses move'} to General. The category is gone from every group that used it.`} confirmLabel="Remove" onCancel={() => setDel(null)} onConfirm={() => { const c = del; setDel(null); run(() => api.del(`${V1}/categories/${c.id}`), 'Category removed') }} />}
    </div>
  )
}
