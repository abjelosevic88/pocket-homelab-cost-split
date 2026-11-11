import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Activity, Balances, CostCenterPosition, Expense, Group as GroupT, GroupStats, Member } from '../lib/types'
import { ago, categoriesFor, decimalsOf, fmtDate, fmtMonth, isCC, listNames, memberName, peopleOf, SPLIT_LABEL, todayISO, useDebounceValue } from '../lib/money'
import { Card, Confirm, Empty, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Money, CurrencySelect } from '../components/Money'
import { GroupIcon, MemberAvatar, MemberStack } from '../components/people'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'

type Tab = 'expenses' | 'balances' | 'stats' | 'activity' | 'settings'

export default function Group() {
  const { id, tab = 'expenses' } = useParams()
  const navigate = useNavigate()
  const { data: g, loading, error, reload } = useAsync(() => api.get<GroupT>(`${V1}/groups/${id}`), [id])
  if (loading && !g) return <Loading />
  if (error || !g) return <Empty icon="users" title="Group not found"><Link to="/groups">Back to groups</Link></Empty>
  const t = (['expenses', 'balances', 'stats', 'activity', 'settings'].includes(tab) ? tab : 'expenses') as Tab
  return (
    <>
      <PageHeader title={<span className="grp-title"><GroupIcon group={g} size={40} /><span className="grow"><span className="row wrap">{g.name}<span className="badge cat">{g.currency}</span>{g.archived && <span className="badge">archived</span>}</span></span></span>}
        sub={<span className="row wrap grp-sub"><MemberStack members={g.members} myId={g.my_member_id} /><span>{g.members.map(m => m.name).join(', ')}</span>{g.description && <span className="muted">· {g.description}</span>}</span>}
        actions={<><Link className="btn" to={`/reports?group=${g.id}`}><Icon name="chart" size={15} />Report</Link>{!g.archived && <button type="button" className="btn primary" onClick={() => navigate(`/groups/${g.id}/expenses/new`)}><Icon name="plus" size={15} strokeWidth={2.2} />Add expense</button>}</>} />
      <div className="grp-tabs"><Tabs tabs={[{ id: 'expenses', label: `Expenses${g.expense_count ? ` (${g.expense_count})` : ''}`, icon: 'receipt' }, { id: 'balances', label: 'Balances', icon: 'scale' }, { id: 'stats', label: 'Stats', icon: 'pie' }, { id: 'activity', label: 'Activity', icon: 'activity' }, { id: 'settings', label: 'Settings', icon: 'settings' }]} value={t} onChange={x => navigate(`/groups/${g.id}${x === 'expenses' ? '' : '/' + x}`)} /></div>
      <div className="mt">
        {t === 'expenses' && <Expenses g={g} />}
        {t === 'balances' && <BalancesTab g={g} onChanged={reload} />}
        {t === 'stats' && <Stats g={g} />}
        {t === 'activity' && <ActivityTab g={g} />}
        {t === 'settings' && <GroupSettings g={g} onChanged={reload} />}
      </div>
    </>
  )
}

// ---------- expenses ----------

function Expenses({ g }: { g: GroupT }) {
  const { categories } = useApp()
  const [q, setQ] = useState('')
  const [member, setMember] = useState('')
  const [cat, setCat] = useState('')
  const dq = useDebounceValue(q, 250)
  const { data, loading } = useAsync(() => api.get<{ expenses: Expense[]; total: number }>(`${V1}/groups/${g.id}/expenses?q=${encodeURIComponent(dq)}&member=${member}&category=${cat}`), [g.id, dq, member, cat, g.updated_at])
  const byMonth = useMemo(() => {
    const out: { month: string; items: Expense[]; total: number }[] = []
    for (const e of data?.expenses || []) {
      const m = e.date.slice(0, 7)
      let grp = out[out.length - 1]
      if (!grp || grp.month !== m) { grp = { month: m, items: [], total: 0 }; out.push(grp) }
      grp.items.push(e)
      if (!e.reimbursement) grp.total += e.amount_group
    }
    return out
  }, [data])
  const icon = (c: string) => categories.find(x => x.id === c)?.icon || '🧾'
  return (
    <>
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search expenses…" aria-label="Search expenses" /></div>
        <Select value={member} onChange={setMember} placeholder="Everyone" clearLabel="Everyone" options={g.members.map(m => ({ value: String(m.id), label: m.name }))} ariaLabel="Filter by member" />
        <Select value={cat} onChange={setCat} placeholder="All categories" clearLabel="All categories" options={categoriesFor(categories, g.id).map(c => ({ value: c.id, label: `${c.icon} ${c.label}` }))} ariaLabel="Filter by category" />
      </div>
      {loading && !data ? <Loading /> : !data || data.expenses.length === 0 ? (
        <Empty icon={q || member || cat ? 'search' : 'receipt'} title={q || member || cat ? 'Nothing matches' : 'No expenses yet'}>{!(q || member || cat) && !g.archived && <>Add the first one with <b>Add expense</b>.</>}</Empty>
      ) : byMonth.map(grp => (
        <div key={grp.month} className="month-group">
          <h3><span>{fmtMonth(grp.month)}</span><span className="spacer" /><span className="muted"><Money minor={grp.total} code={g.currency} /></span></h3>
          <div className="expense-list">
            {grp.items.map(e => <ExpenseRow key={e.id} e={e} g={g} icon={icon(e.category)} />)}
          </div>
        </div>
      ))}
    </>
  )
}

function ExpenseRow({ e, g, icon }: { e: Expense; g: GroupT; icon: string }) {
  const mine = e.shares.find(s => s.member_id === g.my_member_id)
  const paidByMe = e.paid_by === g.my_member_id
  const myNet = (paidByMe ? e.amount_group : 0) - (mine ? mine.amount_group : 0)
  const foreign = e.currency !== g.currency
  const forCenter = !e.reimbursement && e.shares.length === 1 ? g.members.find(m => m.id === e.shares[0].member_id && isCC(m)) : undefined
  if (e.reimbursement) {
    const to = e.shares[0]?.member_id
    return (
      <Link to={`/groups/${g.id}/expenses/${e.id}`} className="expense-row reimb">
        <span className="ico"><Icon name={e.cost_center_id ? 'briefcase' : 'transfer'} size={17} /></span>
        <span className="info">
          <span className="nm">{memberName(g.members, e.paid_by)} paid {memberName(g.members, to)}{e.cost_center_id > 0 && <span className="badge cat cc-badge">buys into {memberName(g.members, e.cost_center_id)}</span>}</span>
          <span className="meta">{fmtDate(e.date, false)} · {e.cost_center_id ? `${memberName(g.members, e.cost_center_id)} now owes ${memberName(g.members, e.paid_by)} this part instead of ${memberName(g.members, to)}` : 'payment'}{e.title && e.title !== 'Reimbursement' ? ` · ${e.title}` : ''}{e.notes && ` · ${e.notes}`}</span>
        </span>
        <span className="side">
          <b><Money minor={e.amount} code={e.currency} /></b>
          {foreign && <span className="muted small">= <Money minor={e.amount_group} code={g.currency} /></span>}
        </span>
      </Link>
    )
  }
  return (
    <Link to={`/groups/${g.id}/expenses/${e.id}`} className="expense-row">
      <span className="ico" title={e.category}>{icon}</span>
      <span className="info">
        <span className="nm">{e.title}</span>
        <span className="meta">{fmtDate(e.date, false)} · <b>{memberName(g.members, e.paid_by)}</b> {forCenter ? <>fronted for <b>{forCenter.name}</b></> : <>paid for {listNames(g.members, e.shares.map(s => s.member_id))}</>}{e.split_mode !== 'even' && <span className="badge split-badge">{SPLIT_LABEL[e.split_mode]}</span>}{e.source && <span className="muted"> · via Pocket {e.source.charAt(0).toUpperCase() + e.source.slice(1)}</span>}</span>
      </span>
      <span className="side">
        <b><Money minor={e.amount} code={e.currency} /></b>
        {foreign && <span className="muted small">= <Money minor={e.amount_group} code={g.currency} /></span>}
        {forCenter ? (paidByMe && <span className="small ok">{forCenter.name} owes you</span>)
          : g.my_member_id > 0 && (mine || paidByMe) && <span className={`small ${myNet > 0 ? 'ok' : myNet < 0 ? 'bad' : 'muted'}`}>{myNet > 0 ? 'you lent' : myNet < 0 ? 'you owe' : 'even'} {myNet !== 0 && <Money minor={Math.abs(myNet)} code={g.currency} />}</span>}
      </span>
    </Link>
  )
}

// ---------- balances ----------

function BalancesTab({ g, onChanged }: { g: GroupT; onChanged: () => void }) {
  const { toast } = useToast()
  const { data, loading, reload } = useAsync(() => api.get<Balances>(`${V1}/groups/${g.id}/balances`), [g.id, g.updated_at])
  const [pay, setPay] = useState<PayInit | null>(null)
  if (loading && !data) return <Loading />
  if (!data) return null
  const max = Math.max(1, ...data.balances.map(b => Math.abs(b.balance)))
  const name = (id: number) => memberName(g.members, id)
  const settled = data.settlements.length === 0
  return (
    <div className="two-col">
      <Card title={<><Icon name="scale" size={16} />Balances</>}>
        <p className="muted small">What each person paid for the group minus their part of everything, in {g.currency}. Positive: the group owes them.{data.cost_centers.length > 0 && ' Money fronted for a project is not in here: it is under the project below.'}</p>
        <div className="balance-list">
          {data.balances.map(b => {
            const m = g.members.find(x => x.id === b.member_id)!
            const w = Math.round((Math.abs(b.balance) / max) * 100)
            return (
              <div key={b.member_id} className="balance-row">
                <span className="who"><MemberAvatar member={m} me={m.id === g.my_member_id} /><span className="nm">{m.name}{m.id === g.my_member_id && <span className="muted"> (you)</span>}</span></span>
                <span className="track">
                  <span className="half left">{b.balance < 0 && <span className="bar neg" style={{ width: `${w}%` }} />}</span>
                  <span className="half right">{b.balance > 0 && <span className="bar pos" style={{ width: `${w}%` }} />}</span>
                </span>
                <span className="amt"><Money minor={b.balance} code={g.currency} sign /></span>
                <span className="detail muted small">paid <Money minor={b.paid} code={g.currency} /> · share <Money minor={b.owed} code={g.currency} /></span>
              </div>
            )
          })}
        </div>
      </Card>
      <Card title={<><Icon name="transfer" size={16} />Settle up</>}>
        {settled ? <Empty icon="check-circle" title="All settled">Nobody owes anybody.</Empty> : (
          <>
            <p className="muted small">The fewest payments that bring everyone to zero. Mark one as paid once the money has moved: it is recorded as a reimbursement.</p>
            <ul className="settle-list">
              {data.settlements.map((t, i) => (
                <li key={i}>
                  <span className="row wrap grow"><MemberAvatar member={{ id: t.from, name: name(t.from) }} /><b>{name(t.from)}</b><Icon name="arrow-right" size={14} className="muted" /><MemberAvatar member={{ id: t.to, name: name(t.to) }} /><b>{name(t.to)}</b></span>
                  <b><Money minor={t.amount} code={g.currency} /></b>
                  {!g.archived && <button type="button" className="btn sm" onClick={() => setPay(t)}><Icon name="check" size={14} />Mark as paid</button>}
                </li>
              ))}
            </ul>
          </>
        )}
        {!g.archived && <div className="mt"><button type="button" className="btn ghost sm" onClick={() => { const people = peopleOf(g.members); const from = g.my_member_id || people[0].id; setPay({ from, to: people.find(m => m.id !== from)?.id || from, amount: 0 }) }}><Icon name="plus" size={14} />Record another payment…</button></div>}
      </Card>
      {data.cost_centers.map(cc => <CostCenterCard key={cc.member_id} g={g} cc={cc} onPay={setPay} />)}
      {pay && <PayModal g={g} initial={pay} onClose={() => setPay(null)} onSaved={() => { setPay(null); reload(); onChanged(); toast('Payment recorded') }} />}
    </div>
  )
}

/** A payment to record: from → to, optionally buying into a cost center's debt to "to". */
type PayInit = { from: number; to: number; amount: number; via?: number }

function CostCenterCard({ g, cc, onPay }: { g: GroupT; cc: CostCenterPosition; onPay: (p: PayInit) => void }) {
  const m = g.members.find(x => x.id === cc.member_id)!
  const name = (id: number) => memberName(g.members, id)
  const people = peopleOf(g.members)
  return (
    <Card title={<><Icon name="briefcase" size={16} />{m.name}<span className="badge cat cc-badge">project</span></>}>
      <p className="muted small">What {m.name} owes back to whoever fronted money for it, in {g.currency}. It is not a person: it has no share of its own and stays out of who owes whom.</p>
      <div className="stats compact">
        <div className="stat"><div className="n"><Money minor={cc.owes} code={g.currency} /></div><div className="l">owes in total</div></div>
        <div className="stat"><div className="n"><Money minor={cc.charged} code={g.currency} /></div><div className="l">fronted for it</div></div>
        <div className="stat"><div className="n"><Money minor={cc.repaid} code={g.currency} /></div><div className="l">paid back</div></div>
      </div>
      {cc.claims.length === 0 ? <Empty icon="check-circle" title="Owes nobody">Nothing fronted for {m.name} is open.</Empty> : (
        <ul className="settle-list">
          {cc.claims.map(c => {
            const other = people.find(p => p.id !== c.member_id)
            return (
              <li key={c.member_id}>
                <span className="row wrap grow">{c.amount > 0 ? <><b>{m.name}</b> owes <MemberAvatar member={{ id: c.member_id, name: name(c.member_id) }} me={c.member_id === g.my_member_id} /><b>{name(c.member_id)}</b></> : <><MemberAvatar member={{ id: c.member_id, name: name(c.member_id) }} /><b>{name(c.member_id)}</b> owes <b>{m.name}</b></>}</span>
                <b><Money minor={Math.abs(c.amount)} code={g.currency} /></b>
                {!g.archived && c.amount > 0 && (
                  <span className="row">
                    <button type="button" className="btn sm" onClick={() => onPay({ from: m.id, to: c.member_id, amount: c.amount })} title={`${m.name} pays ${name(c.member_id)} back`}><Icon name="check" size={14} />Paid back</button>
                    {other && <button type="button" className="btn ghost sm" onClick={() => onPay({ from: other.id, to: c.member_id, amount: 0, via: m.id })} title={`Someone pays ${name(c.member_id)} and takes over part of this claim`}><Icon name="transfer" size={14} />Buy in…</button>}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function PayModal({ g, initial, onClose, onSaved }: { g: GroupT; initial: PayInit; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const { currencies } = useApp()
  const [from, setFrom] = useState(String(initial.from))
  const [to, setTo] = useState(String(initial.to))
  const [via, setVia] = useState(initial.via || 0)
  const [currency, setCurrency] = useState(g.currency)
  const [amount, setAmount] = useState<number | null>(initial.amount || null)
  const [date, setDate] = useState(todayISO())
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const opts = g.members.map(m => ({ value: String(m.id), label: isCC(m) ? `${m.name} (project)` : m.name }))
  const centers = g.members.filter(m => isCC(m))
  const fromM = g.members.find(m => String(m.id) === from), toM = g.members.find(m => String(m.id) === to)
  const canBuyIn = centers.length > 0 && fromM && toM && !isCC(fromM) && !isCC(toM) && from !== to
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!amount || from === to) { toast(from === to ? 'Pick two different people' : 'Enter the amount', 'err'); return }
    setBusy(true)
    try {
      await api.post(`${V1}/groups/${g.id}/expenses`, { title: 'Reimbursement', amount, currency, paid_by: Number(from), date, notes, split_mode: 'even', reimbursement: true, cost_center_id: canBuyIn ? via : 0, shares: [{ member_id: Number(to), share: 1 }] })
      onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Record a payment" onClose={onClose}>
      <form onSubmit={submit} className="stack">
        <div className="two-col tight">
          <Field group label="From"><Select value={from} onChange={setFrom} options={opts} /></Field>
          <Field group label="To"><Select value={to} onChange={setTo} options={opts} /></Field>
        </div>
        <Field group label="Amount" help={currency !== g.currency ? `Converted to ${g.currency} at the day's rate.` : undefined}>
          <div className="amount-field"><CurrencySelect value={currency} onChange={setCurrency} /><MoneyInputLazy value={amount} code={currency} onChange={setAmount} autoFocus /></div>
        </Field>
        {canBuyIn && (
          <Field group label="What is it?" help={via ? `${fromM!.name} takes over this much of what ${memberName(g.members, via)} owes ${toM!.name}: afterwards ${memberName(g.members, via)} owes ${fromM!.name} that part. Nobody owes anybody personally.` : `${fromM!.name} pays back a personal debt to ${toM!.name}.`}>
            <Segmented size="sm" value={via} onChange={setVia} options={[{ value: 0, label: 'Personal payment' }, ...centers.map(c => ({ value: c.id, label: `Buys into ${c.name}` }))]} />
          </Field>
        )}
        <Field label="Date"><input type="date" value={date} onChange={e => setDate(e.target.value)} max={todayISO()} /></Field>
        <Field label="Note"><input value={notes} onChange={e => setNotes(e.target.value)} placeholder="Cash, bank transfer…" /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy}>{busy ? 'Saving…' : 'Record payment'}</button></div>
      </form>
    </Modal>
  )
}
// MoneyInput lives in Money.tsx; re-exported here to keep imports short.
import { MoneyInput as MoneyInputLazy } from '../components/Money'

// ---------- stats ----------

function Stats({ g }: { g: GroupT }) {
  const { categories } = useApp()
  const { data: st, loading } = useAsync(() => api.get<GroupStats>(`${V1}/groups/${g.id}/stats`), [g.id, g.updated_at])
  if (loading && !st) return <Loading />
  if (!st) return null
  if (st.count === 0) return <Empty icon="pie" title="No expenses yet">Stats appear once there is something to count.</Empty>
  const maxMonth = Math.max(1, ...st.months.map(m => m.total))
  const maxCat = Math.max(1, ...st.categories.map(c => c.total))
  const maxMem = Math.max(1, ...st.members.map(m => Math.max(m.paid, m.share)))
  const cat = (id: string) => categories.find(c => c.id === id) || { icon: '🧾', label: id }
  const perHead = Math.round(st.total / Math.max(1, peopleOf(g.members).length))
  const mine = st.members.find(m => m.member_id === g.my_member_id)
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="n"><Money minor={st.total} code={g.currency} /></div><div className="l">Total spent · {st.count} {st.count === 1 ? 'expense' : 'expenses'}</div></div>
        <div className="stat"><div className="n"><Money minor={perHead} code={g.currency} /></div><div className="l">Per person, if split evenly</div></div>
        {mine && <div className="stat"><div className="n"><Money minor={mine.share} code={g.currency} /></div><div className="l">Your share · you paid <Money minor={mine.paid} code={g.currency} /></div></div>}
      </div>
      <div className="two-col">
        <Card title="By month">
          <div className="month-bars" style={{ gridTemplateColumns: `repeat(${Math.max(1, st.months.length)}, 1fr)` }}>
            {st.months.map(m => (
              <div key={m.month} className="mb-col" title={`${fmtMonth(m.month)}: ${m.count} expenses`}>
                <span className="v"><Money minor={m.total} code={g.currency} /></span>
                <span className="b" style={{ height: `${Math.max(2, Math.round((m.total / maxMonth) * 120))}px` }} />
                <span className="l">{fmtMonth(m.month).slice(0, 3)}{st.months.length > 1 && st.months[0].month.slice(0, 4) !== st.months[st.months.length - 1].month.slice(0, 4) ? ` ’${m.month.slice(2, 4)}` : ''}</span>
              </div>
            ))}
          </div>
          {st.first_date && <p className="muted small mt">From {fmtDate(st.first_date)} to {fmtDate(st.last_date)}.</p>}
        </Card>
        <Card title="By category">
          <div className="rating-bars">
            {st.categories.map(c => (
              <div key={c.category} className="rb-row cat">
                <span className="l">{cat(c.category).icon} {cat(c.category).label}</span>
                <span className="track"><span className="b" style={{ width: `${Math.round((c.total / maxCat) * 100)}%` }} /></span>
                <span className="n"><Money minor={c.total} code={g.currency} /></span>
              </div>
            ))}
          </div>
        </Card>
        <Card title="Who paid, whose share">
          <div className="rating-bars">
            {st.members.map(m => {
              const mem = g.members.find(x => x.id === m.member_id)!
              return (
                <div key={m.member_id} className="member-bars">
                  <span className="row"><MemberAvatar member={mem} /><b>{mem.name}</b>{isCC(mem) ? <span className="badge cat cc-badge">project</span> : <span className="muted small">{m.count} paid</span>}</span>
                  <div className="rb-row"><span className="l">paid</span><span className="track"><span className="b paid" style={{ width: `${Math.round((m.paid / maxMem) * 100)}%` }} /></span><span className="n"><Money minor={m.paid} code={g.currency} /></span></div>
                  <div className="rb-row"><span className="l">share</span><span className="track"><span className="b" style={{ width: `${Math.round((m.share / maxMem) * 100)}%` }} /></span><span className="n"><Money minor={m.share} code={g.currency} /></span></div>
                </div>
              )
            })}
          </div>
        </Card>
        <Card title="Currencies used">
          <table className="table compact">
            <thead><tr><th>Currency</th><th>Expenses</th><th className="right">Paid</th></tr></thead>
            <tbody>{st.currencies.map(c => <tr key={c.currency}><td><span className="badge cat">{c.currency}</span></td><td className="muted">{c.count}</td><td className="right"><Money minor={c.total} code={c.currency} /></td></tr>)}</tbody>
          </table>
          <p className="muted small mt">Everything above is in {g.currency}, converted at the rate saved on each expense.</p>
        </Card>
      </div>
    </>
  )
}

// ---------- activity ----------

export function ActivityKind({ kind }: { kind: string }) {
  const map: Record<string, [string, string]> = {
    'expense.added': ['plus', 'added'], 'expense.updated': ['edit', 'edited'], 'expense.deleted': ['trash', 'deleted'], 'reimbursement.added': ['transfer', 'payment'],
    'member.added': ['user-plus', 'joined'], 'member.removed': ['user-minus', 'left'], 'group.created': ['users', 'created'], 'group.updated': ['settings', 'settings'], 'group.imported': ['upload', 'imported'],
  }
  const [ico, label] = map[kind] || ['dot', kind]
  return <span className={`badge act ${kind.split('.')[1]}`} title={kind}><Icon name={ico} size={12} strokeWidth={2} />{label}</span>
}

function ActivityTab({ g }: { g: GroupT }) {
  const { data, loading } = useAsync(() => api.get<Activity[]>(`${V1}/groups/${g.id}/activity?limit=100`), [g.id, g.updated_at])
  if (loading && !data) return <Loading />
  if (!data || data.length === 0) return <Empty icon="activity" title="Nothing yet" />
  return (
    <Card flush>
      <ul className="mini-list pad-x">
        {data.map(a => (
          <li key={a.id}>
            <ActivityKind kind={a.kind} />
            <span className="grow">{a.expense_id && !a.kind.endsWith('deleted') ? <Link to={`/groups/${g.id}/expenses/${a.expense_id}`}>{a.summary}</Link> : a.summary}<div className="muted small">{a.user_name || 'someone'} · {ago(a.created_at)}</div></span>
          </li>
        ))}
      </ul>
    </Card>
  )
}

// ---------- settings ----------

function GroupSettings({ g, onChanged }: { g: GroupT; onChanged: () => void }) {
  const { user, users, reloadGroups } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [f, setF] = useState({ name: g.name, description: g.description, currency: g.currency })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const [newName, setNewName] = useState('')
  const [rename, setRename] = useState<Member | null>(null)
  // The creator decides who is in the group: deletes it, unlinks or removes other people's accounts.
  const canManage = !g.created_by || user?.id === g.created_by
  const othersAccount = (m: Member) => !!m.user_id && m.user_id !== user?.id && !canManage
  const run = async (fn: () => Promise<unknown>, msg: string) => { setBusy(true); try { await fn(); onChanged(); reloadGroups().catch(() => {}); toast(msg) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  const save = (e: React.FormEvent) => { e.preventDefault(); run(() => api.put(`${V1}/groups/${g.id}`, f), 'Saved') }
  const addName = (kind: 'person' | 'cost_center' = 'person') => { const n = newName.trim(); if (!n) return; run(async () => { await api.post(`${V1}/groups/${g.id}/members`, { name: n, kind }); setNewName('') }, kind === 'cost_center' ? `Project ${n} added` : `${n} added`) }
  const setKind = (m: Member, kind: 'person' | 'cost_center') => run(() => api.put(`${V1}/groups/${g.id}/members/${m.id}`, { kind }), kind === 'cost_center' ? `${m.name} is a project now` : `${m.name} is a person now`)
  const addUser = (id: string) => { if (!id) return; run(() => api.post(`${V1}/groups/${g.id}/members`, { user_id: Number(id) }), 'Added') }
  const link = (m: Member, id: string) => run(() => api.put(`${V1}/groups/${g.id}/members/${m.id}`, { user_id: id ? Number(id) : 0 }), id ? 'Linked' : 'Unlinked')
  const remove = (m: Member) => run(() => api.del(`${V1}/groups/${g.id}/members/${m.id}`), `${m.name} removed`)
  const userOf = (m: Member) => users.find(u => u.id === m.user_id)
  const freeUsers = users.filter(u => !g.members.some(m => m.user_id === u.id))
  return (
    <div className="two-col">
      <div className="stack">
        <Card title="Group">
          <form onSubmit={save} className="stack">
            <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={80} /></Field>
            <Field label="Description"><input value={f.description} onChange={e => setF({ ...f, description: e.target.value })} maxLength={200} /></Field>
            <Field group label="Main currency" help={g.expense_count > 0 ? 'Locked: the group already has expenses in it.' : 'Balances and totals are counted in it.'}><CurrencySelect value={f.currency} onChange={c => setF({ ...f, currency: c })} disabled={g.expense_count > 0} /></Field>
            <div className="row"><button type="submit" className="btn primary" disabled={busy}>Save</button></div>
          </form>
        </Card>
        <Card title="Archive & delete">
          <p className="muted small">Archiving hides the group from the list and stops new expenses; everything stays and can be brought back. Deleting removes the group with all its expenses for everyone.</p>
          <div className="row wrap">
            <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.put(`${V1}/groups/${g.id}`, { ...f, archived: !g.archived }), g.archived ? 'Group restored' : 'Group archived')}><Icon name="archive" size={15} />{g.archived ? 'Restore group' : 'Archive group'}</button>
            {canManage && <button type="button" className="btn danger" disabled={busy} onClick={() => setDel(true)}><Icon name="trash" size={15} />Delete group…</button>}
          </div>
          {del && <Confirm confirmLabel="Delete" title={`Delete “${g.name}”?`} message={`${g.expense_count} expenses and every balance in it are deleted for everyone. There is no undo — export it first if in doubt.`} onCancel={() => setDel(false)} onConfirm={() => { setDel(false); run(async () => { await api.del(`${V1}/groups/${g.id}`); navigate('/groups') }, 'Group deleted') }} />}
        </Card>
        <Card title="Export">
          <p className="muted small">JSON is complete and can be imported again under Settings → Backup &amp; data (as a new group). CSV is one row per expense with a column per person.</p>
          <div className="row wrap"><a className="btn" href={`${V1}/groups/${g.id}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/groups/${g.id}/export.csv`}><Icon name="download" size={15} />Export CSV</a></div>
        </Card>
      </div>
      <Card title={<><Icon name="users" size={16} />People</>}>
        <p className="muted small">Anyone can be added by name. Only the people linked to an account here (and whoever created the group) see it: link a person to their account so they see the group, their balance and “you” in the lists. Someone who paid or shares an expense cannot be removed.</p>
        <p className="muted small">A <b>project</b> (cost center) is not a person: it never gets a share of a split and stays out of who owes whom. Costs “for the project” become claims of whoever paid, and it pays them back later. Turn a stand-in member you used for this into a project with <b>Make a project</b>.</p>
        <table className="table compact">
          <tbody>
            {g.members.map(m => {
              const u = userOf(m)
              return (
                <tr key={m.id}>
                  <td style={{ width: 32 }}><MemberAvatar member={m} me={m.id === g.my_member_id} /></td>
                  <td><b>{m.name}</b>{m.id === g.my_member_id && <span className="badge you-badge">you</span>}{isCC(m) && <span className="badge cat cc-badge">project</span>}<div className="muted small">{isCC(m) ? 'cost center: no share, only pays back' : u ? u.email : 'no account'}</div></td>
                  <td className="right" style={{ whiteSpace: 'nowrap' }}>
                    {isCC(m) ? <button type="button" className="btn ghost sm" disabled={busy} onClick={() => setKind(m, 'person')} title="Treat it as a person again">Make a person</button> : <>
                    <Select className="link-user" value={m.user_id ? String(m.user_id) : ''} onChange={v => link(m, v)} disabled={othersAccount(m)} placeholder="Link account…" clearLabel="Unlink" ariaLabel={`Link ${m.name} to an account`}
                      options={users.filter(u => u.id === m.user_id || !g.members.some(x => x.user_id === u.id)).map(u => ({ value: String(u.id), label: u.name || u.email, hint: u.email }))} />
                    {!m.user_id && <button type="button" className="btn ghost sm" disabled={busy} onClick={() => setKind(m, 'cost_center')} title="A project you fund up front: it never carries a share and only owes back what was fronted for it">Make a project</button>}
                    </>}
                  </td>
                  <td className="right" style={{ whiteSpace: 'nowrap' }}>
                    <button type="button" className="btn ghost icon-only sm" onClick={() => setRename(m)} title="Rename" aria-label={`Rename ${m.name}`}><Icon name="edit" size={15} /></button>
                    <button type="button" className="btn ghost icon-only sm danger-text" onClick={() => remove(m)} disabled={busy || othersAccount(m)} title="Remove" aria-label={`Remove ${m.name}`}><Icon name="trash" size={15} /></button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        <div className="row wrap mt">
          <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Add someone (or a project) by name…" onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addName() } }} style={{ flex: '1 1 160px' }} aria-label="New member name" />
          <button type="button" className="btn" onClick={() => addName()} disabled={busy || !newName.trim()}><Icon name="plus" size={15} />Add</button>
          <button type="button" className="btn ghost" onClick={() => addName('cost_center')} disabled={busy || !newName.trim()} title="A cost center: a project you fund up front that pays you back later"><Icon name="briefcase" size={15} />Add as project</button>
          {freeUsers.length > 0 && <Select className="user-add" value="" onChange={addUser} placeholder="Someone with an account…" options={freeUsers.map(u => ({ value: String(u.id), label: u.name || u.email, hint: u.email }))} ariaLabel="Add a user" />}
        </div>
        {rename && <RenameModal m={rename} onClose={() => setRename(null)} onSaved={name => { const m = rename; setRename(null); run(() => api.put(`${V1}/groups/${g.id}/members/${m.id}`, { name }), 'Renamed') }} />}
      </Card>
    </div>
  )
}

function RenameModal({ m, onClose, onSaved }: { m: Member; onClose: () => void; onSaved: (name: string) => void }) {
  const [name, setName] = useState(m.name)
  return (
    <Modal title={`Rename ${m.name}`} onClose={onClose}>
      <form onSubmit={e => { e.preventDefault(); if (name.trim()) onSaved(name.trim()) }} className="stack">
        <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} autoFocus required maxLength={60} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}

export { decimalsOf }
