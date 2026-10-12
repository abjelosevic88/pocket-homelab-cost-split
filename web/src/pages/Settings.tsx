import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Category, Group, Settings as SettingsT, User } from '../lib/types'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Avatar } from '../components/people'
import { CurrencySelect } from '../components/Money'
import { Select } from '../components/Select'
import { DATE_FORMATS, TIME_FORMATS, fmtDateTime, fmtYMD, fmtTime } from '../lib/money'

type Tab = 'general' | 'categories' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>(() => (new URLSearchParams(window.location.search).get('tab') as Tab) || 'general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'categories', label: 'Categories' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API tokens' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'categories' && <Categories />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
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
  return (
    <div className="two-col">
      <Card title="Preferences">
        <div className="grid" style={{ gap: 14 }}>
          <Field label="App title" help="Shown in the sidebar and browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} /></Field>
          <Field group label="Default currency for new groups" help="Every group can pick its own."><CurrencySelect value={f.default_currency} onChange={c => setF({ ...f, default_currency: c })} disabled={!isAdmin} /></Field>
          <label className="row"><input type="checkbox" checked={f.fx_auto} onChange={e => setF({ ...f, fx_auto: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Look exchange rates up automatically</label>
          <div className="two-col tight">
            <Field group label="Date format" help={`Today: ${fmtYMD(new Date().getFullYear(), new Date().getMonth() + 1, new Date().getDate(), true, f.date_format)}`}>
              <Select value={f.date_format || 'auto'} onChange={v => setF({ ...f, date_format: v as SettingsT['date_format'] })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} disabled={!isAdmin} ariaLabel="Date format" />
            </Field>
            <Field group label="Time format" help={`Now: ${fmtTime(new Date(), f.time_format)}`}>
              <Select value={f.time_format || 'auto'} onChange={v => setF({ ...f, time_format: v as SettingsT['time_format'] })} options={TIME_FORMATS.map(t => ({ value: t.id, label: t.label }))} disabled={!isAdmin} ariaLabel="Time format" />
            </Field>
          </div>
          {isAdmin ? <div><button className="btn primary" onClick={save} disabled={busy}>Save</button></div> : <p className="muted small">Only an admin can change these.</p>}
        </div>
      </Card>
      <Card title="Exchange rates">
        <p className="muted">When an expense is paid in a currency other than the group's, the app looks the day's rate up and stores it on the expense, so balances never shift afterwards. You can always type a different rate (what your bank actually charged).</p>
        <table className="table compact">
          <tbody>
            <tr><th>Frankfurter</th><td className="muted small">European Central Bank reference rates, ~30 major currencies, with history: the rate for the expense's date.</td></tr>
            <tr><th>open.er-api.com</th><td className="muted small">~160 currencies (BAM, RSD, …), today's rate only. Used when Frankfurter does not cover a pair.</td></tr>
          </tbody>
        </table>
        <p className="muted small mt">Provider set on the server: <code>{f.fx_provider}</code>{!f.fx_enabled && ' (lookups are currently off)'}. Rates are cached once fetched, one per currency pair and day.</p>
      </Card>
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
    <Card title="Users" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone signs in with their own account and sees the groups they are a member of. Link a group member to an account under the group's Settings → People. Admins manage users and see every group.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account is deleted. Group members linked to it stay in their groups, unlinked, so no expense or balance is lost." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
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
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
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
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name" help="Shown to others; group members you are linked to keep their own name."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
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
    <Card title="API tokens">
      <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you and sees your groups. Good for a Homepage widget (<code>GET /api/v1/dashboard</code>) or adding an expense from an iOS Shortcut (<code>POST /api/v1/groups/{'{id}'}/expenses</code>). See <code>docs/api.md</code>.</p>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary">Create</button></form>
      {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Card>
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
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>) with every group, expense and user. Restore by stopping the app and replacing <code>pocket-split.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Import a group">
        <p className="muted">A JSON export from any group's Settings → Export becomes a new group here, with its people and expenses, at the exchange rates it was saved with.</p>
        <label className="btn">{busy ? 'Importing…' : 'Import group JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Split</b> keeps track of shared costs for trips, flats and dinners: who paid what, in any currency, and who owes whom. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-cost-split" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-cost-split</a>. Sibling projects: Pocket Habits, Pocket Inventory, Pocket Bookshelf and Pocket Invoicing. Exchange rates by Frankfurter (ECB) and ExchangeRate-API.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Exchange rates</th><td className="mono">{String(data.fx_provider)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.groups} groups · {counts.members} members · {counts.expenses} expenses · {counts.users} users</td></tr>
          </tbody>
        </table>
      )}
    </Card>
  )
}

function Categories() {
  const { user, categories, reloadMeta } = useApp()
  const { toast } = useToast()
  const isAdmin = user?.role === 'admin'
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
    run(async () => { await api.put(`${V1}/categories/${c.id}`, { sort_order: j + 1 }); await api.put(`${V1}/categories/${o.id}`, { sort_order: i + 1 }) }, 'Moved')
  }
  const create = (e: React.FormEvent) => { e.preventDefault(); if (!add.label.trim()) return; run(async () => { await api.post(`${V1}/categories`, add); setAdd({ label: '', icon: '' }) }, 'Category added') }
  return (
    <Card title="Categories">
      <p className="muted">What an expense is for. Everyone can add a category, also straight from the expense form by typing a name that is not in the list. {isAdmin ? 'Rename, reorder or remove them here; expenses of a removed category move to General.' : 'An admin can rename, reorder or remove them.'}</p>
      <table className="table compact cat-table">
        <thead><tr><th style={{ width: 70 }}>Icon</th><th>Name</th><th>Used</th><th /></tr></thead>
        <tbody>
          {categories.map((c, i) => (
            <tr key={c.id}>
              <td><input value={d(c).icon} onChange={e => setDraft({ ...draft, [c.id]: { ...d(c), icon: e.target.value } })} maxLength={8} disabled={!isAdmin} aria-label={`Icon for ${c.label}`} style={{ width: 56, textAlign: 'center' }} /></td>
              <td><input value={d(c).label} onChange={e => setDraft({ ...draft, [c.id]: { ...d(c), label: e.target.value } })} maxLength={60} disabled={!isAdmin} aria-label={`Name of ${c.label}`} /></td>
              <td className="muted" style={{ whiteSpace: 'nowrap' }}>{c.count} {c.count === 1 ? 'expense' : 'expenses'}{c.builtin && <span className="badge" style={{ marginLeft: 6 }}>built-in</span>}</td>
              <td className="right" style={{ whiteSpace: 'nowrap' }}>
                {isAdmin && <>
                  {changed(c) && <button className="btn primary sm" onClick={() => save(c)} disabled={busy}>Save</button>}
                  <button className="btn ghost sm" onClick={() => move(c, -1)} disabled={busy || i === 0} title="Move up">↑</button>
                  <button className="btn ghost sm" onClick={() => move(c, 1)} disabled={busy || i === categories.length - 1} title="Move down">↓</button>
                  {c.id !== 'general' && <button className="btn ghost sm danger-text" onClick={() => setDel(c)} disabled={busy}>Remove</button>}
                </>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <form onSubmit={create} className="row wrap mt">
        <input value={add.icon} onChange={e => setAdd({ ...add, icon: e.target.value })} placeholder="🏷️" maxLength={8} style={{ width: 64, textAlign: 'center' }} aria-label="Icon" />
        <input value={add.label} onChange={e => setAdd({ ...add, label: e.target.value })} placeholder="New category, e.g. Ski passes" maxLength={60} style={{ flex: '1 1 200px' }} aria-label="New category name" />
        <button className="btn primary" disabled={busy || !add.label.trim()}>Add</button>
      </form>
      <p className="muted small mt-sm">The icon is any emoji; leave it empty for 🏷️.</p>
      {del && <Confirm title={`Remove “${del.label}”?`} message={`${del.count} ${del.count === 1 ? 'expense moves' : 'expenses move'} to General. The category is gone for every group.`} onCancel={() => setDel(null)} onConfirm={() => { const c = del; setDel(null); run(() => api.del(`${V1}/categories/${c.id}`), 'Category removed') }} />}
    </Card>
  )
}
