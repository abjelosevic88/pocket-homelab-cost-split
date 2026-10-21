import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Group } from '../lib/types'
import { fmtDate } from '../lib/money'
import { Card, Empty, Field, Loading, Modal, PageHeader, Segmented, useAsync, useToast } from '../components/ui'
import { Money, CurrencySelect } from '../components/Money'
import { GroupIcon, MemberStack } from '../components/people'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'

export default function Groups() {
  const { settings, reloadGroups } = useApp()
  const [params, setParams] = useSearchParams()
  const [show, setShow] = useState<'active' | 'archived'>('active')
  const [creating, setCreating] = useState(params.get('new') === '1')
  const navigate = useNavigate()
  const { data, loading, reload } = useAsync(() => api.get<Group[]>(`${V1}/groups?archived=${show === 'archived' ? 1 : 0}`), [show])
  useEffect(() => { if (params.get('new') === '1') { setCreating(true); params.delete('new'); setParams(params, { replace: true }) } }, [params]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <>
      <PageHeader title="Groups" sub="A group is one pot of shared costs: a trip, a flat, a dinner. Balances are counted in its main currency; pay in any."
        actions={<button type="button" className="btn primary" onClick={() => setCreating(true)}><Icon name="plus" size={15} strokeWidth={2.2} />New group</button>} />
      <div className="toolbar">
        <Segmented value={show} onChange={setShow} options={[{ value: 'active', label: 'Active' }, { value: 'archived', label: 'Archived' }]} />
      </div>
      {loading && !data ? <Loading /> : !data || data.length === 0 ? (
        <Empty icon={show === 'archived' ? 'archive' : 'users'} title={show === 'archived' ? 'No archived groups' : 'No groups yet'}>{show === 'active' && <>Create one for the next trip or the flat.</>}</Empty>
      ) : (
        <div className="group-grid">
          {data.map(g => (
            <Link key={g.id} to={`/groups/${g.id}`} className="card group-card">
              <div className="group-card-head">
                <GroupIcon group={g} size={38} />
                <div className="grow">
                  <div className="nm">{g.name}{g.archived && <span className="badge">archived</span>}</div>
                  {g.description && <div className="muted small">{g.description}</div>}
                </div>
                <span className="badge cat">{g.currency}</span>
              </div>
              <div className="row" style={{ marginTop: 10 }}>
                <MemberStack members={g.members} myId={g.my_member_id} />
                <span className="muted small">{g.members.length} {g.members.length === 1 ? 'person' : 'people'}</span>
                <span className="spacer" />
                <span className="muted small">{g.expense_count} {g.expense_count === 1 ? 'expense' : 'expenses'}{g.last_activity && ` · ${fmtDate(g.last_activity, false)}`}</span>
              </div>
              <div className="group-card-foot">
                <span><span className="muted small">Total</span><br /><b><Money minor={g.total} code={g.currency} /></b></span>
                <span className="right">
                  {g.my_member_id ? <><span className="muted small">{g.my_balance > 0 ? 'You get back' : g.my_balance < 0 ? 'You owe' : 'You are'}</span><br /><b><Money minor={g.my_balance} code={g.currency} sign /></b>{g.my_balance === 0 && <b className="muted"> settled</b>}</> : <span className="muted small">not a member</span>}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
      {creating && <NewGroupModal defaultCurrency={settings?.default_currency || 'EUR'} onClose={() => setCreating(false)} onCreated={g => { setCreating(false); reload(); reloadGroups().catch(() => {}); navigate(`/groups/${g.id}`) }} />}
    </>
  )
}

interface Draft { name: string; description: string; currency: string; members: { name: string; user_id: number; me: boolean }[] }

export function NewGroupModal({ defaultCurrency, onClose, onCreated }: { defaultCurrency: string; onClose: () => void; onCreated: (g: Group) => void }) {
  const { user, users } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Draft>({ name: '', description: '', currency: defaultCurrency, members: [{ name: user?.name || user?.email.split('@')[0] || 'Me', user_id: user?.id || 0, me: true }] })
  const [newName, setNewName] = useState('')
  const [busy, setBusy] = useState(false)
  const addName = () => { const n = newName.trim(); if (!n) return; if (f.members.some(m => m.name.toLowerCase() === n.toLowerCase())) { toast('Already in the list', 'err'); return } setF({ ...f, members: [...f.members, { name: n, user_id: 0, me: false }] }); setNewName('') }
  const addUser = (id: string) => { const u = users.find(x => String(x.id) === id); if (!u || f.members.some(m => m.user_id === u.id)) return; setF({ ...f, members: [...f.members, { name: u.name || u.email.split('@')[0], user_id: u.id, me: false }] }) }
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { onCreated(await api.post<Group>(`${V1}/groups`, f)); toast('Group created') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const otherUsers = users.filter(u => !f.members.some(m => m.user_id === u.id))
  return (
    <Modal title="New group" onClose={onClose}>
      <form onSubmit={submit} className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} placeholder="Lisbon week, Flat, Friday dinner…" autoFocus required maxLength={80} /></Field>
        <Field label="Description" help="Optional."><input value={f.description} onChange={e => setF({ ...f, description: e.target.value })} maxLength={200} /></Field>
        <Field group label="Main currency" help="Balances and totals are counted in it. Expenses can be paid in any currency and are converted at the day's rate. Cannot change once there are expenses.">
          <CurrencySelect value={f.currency} onChange={c => setF({ ...f, currency: c })} />
        </Field>
        <Field group label="People" help="Add anyone by name; link people with an account so they see the group and their balance.">
          <div className="chips mb-sm">
            {f.members.map((m, i) => <span key={i} className={`chip ${m.user_id ? 'active' : ''}`}>{m.name}{m.me && ' (you)'}{!m.me && <button type="button" className="chip-x" onClick={() => setF({ ...f, members: f.members.filter((_, j) => j !== i) })} aria-label={`Remove ${m.name}`}><Icon name="x" size={13} /></button>}</span>)}
          </div>
          <div className="row wrap">
            <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Name…" onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addName() } }} style={{ flex: '1 1 140px' }} aria-label="Member name" />
            <button type="button" className="btn" onClick={addName}>Add name</button>
            {otherUsers.length > 0 && <Select className="user-add" value="" onChange={addUser} placeholder="Someone with an account…" options={otherUsers.map(u => ({ value: String(u.id), label: u.name || u.email, hint: u.email }))} ariaLabel="Add a user" />}
          </div>
        </Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy || !f.name.trim()}>{busy ? 'Creating…' : 'Create group'}</button></div>
      </form>
    </Modal>
  )
}

export { Card }
