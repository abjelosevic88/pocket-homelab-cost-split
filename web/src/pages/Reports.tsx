import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Group } from '../lib/types'
import { fmtDate, fmtMonth, todayISO } from '../lib/money'
import { Card, Empty, Field, Loading, PageHeader, Segmented, useAsync } from '../components/ui'
import { Money } from '../components/Money'
import { MemberAvatar } from '../components/people'
import { Select } from '../components/Select'

interface Report {
  group: { id: number; name: string; currency: string }
  filter_text: string
  generated_at: string
  total: number; reimbursed: number; count: number; per_head: number; first_date: string; last_date: string
  members: { member_id: number; name: string; paid: number; share: number; net: number; count: number }[]
  categories: { id: string; label: string; icon: string; total: number; count: number; percent: number }[]
  months: { month: string; total: number; count: number }[]
  expenses: { id: number; date: string; title: string; category: string; category_label: string; paid_by: string; amount: number; currency: string; amount_group: number; reimbursement: boolean; for: string; notes: string }[]
  currencies: { currency: string; total: number; count: number }[]
}

type Preset = 'month' | 'last' | 'year' | 'all' | 'custom'

function range(p: Preset): [string, string] {
  const now = new Date(), y = now.getFullYear(), m = now.getMonth()
  const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  switch (p) {
    case 'month': return [iso(new Date(y, m, 1)), iso(new Date(y, m + 1, 0))]
    case 'last': return [iso(new Date(y, m - 1, 1)), iso(new Date(y, m, 0))]
    case 'year': return [`${y}-01-01`, `${y}-12-31`]
    default: return ['', '']
  }
}

export default function Reports() {
  const { categories } = useApp()
  const [params, setParams] = useSearchParams()
  const { data: groups } = useAsync(() => api.get<Group[]>(`${V1}/groups`), [])
  const [groupId, setGroupId] = useState(params.get('group') || '')
  const [preset, setPreset] = useState<Preset>('all')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [cats, setCats] = useState<string[]>([])
  const [member, setMember] = useState('')
  const [payments, setPayments] = useState(false)

  useEffect(() => { if (groups && !groupId && groups.length) setGroupId(String(groups[0].id)) }, [groups, groupId])
  useEffect(() => { if (groupId) { params.set('group', groupId); setParams(params, { replace: true }) } }, [groupId]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setMember(''); setCats([]) }, [groupId])
  const pickPreset = (p: Preset) => { setPreset(p); if (p !== 'custom') { const [a, b] = range(p); setFrom(a); setTo(b) } }

  const g = groups?.find(x => String(x.id) === groupId)
  const query = useMemo(() => {
    const q = new URLSearchParams()
    if (from) q.set('from', from)
    if (to) q.set('to', to)
    if (cats.length) q.set('categories', cats.join(','))
    if (member) q.set('member', member)
    if (payments) q.set('reimbursements', 'include')
    return q.toString()
  }, [from, to, cats, member, payments])
  const { data: rep, loading, error } = useAsync(() => groupId ? api.get<Report>(`${V1}/groups/${groupId}/report?${query}`) : Promise.resolve(null), [groupId, query])

  if (!groups) return <Loading />
  if (groups.length === 0) return <><PageHeader title="Reports" /><Empty title="No groups yet"><Link to="/groups?new=1">Create a group</Link> first.</Empty></>
  const cur = g?.currency || 'EUR'
  const maxCat = Math.max(1, ...(rep?.categories.map(c => c.total) || [1]))
  const maxMonth = Math.max(1, ...(rep?.months.map(m => m.total) || [1]))
  const toggleCat = (id: string) => setCats(c => c.includes(id) ? c.filter(x => x !== id) : [...c, id])

  return (
    <>
      <PageHeader title="Reports" sub="Pick a group and a slice of its expenses; the PDF is exactly what you see here."
        actions={groupId && <a className="btn primary" href={`${V1}/groups/${groupId}/report.pdf?${query}`} download>Export PDF</a>} />
      <Card className="mb">
        <div className="report-filters">
          <Field group label="Group"><Select value={groupId} onChange={setGroupId} options={groups.map(x => ({ value: String(x.id), label: x.name, hint: x.currency }))} ariaLabel="Group" /></Field>
          <Field group label="Period">
            <div className="row wrap">
              <Segmented size="sm" value={preset} onChange={pickPreset} options={[{ value: 'month', label: 'This month' }, { value: 'last', label: 'Last month' }, { value: 'year', label: 'This year' }, { value: 'all', label: 'All time' }, { value: 'custom', label: 'Custom' }]} />
              {preset === 'custom' && <span className="row"><input type="date" value={from} onChange={e => setFrom(e.target.value)} max={to || todayISO()} aria-label="From" /><span className="muted">–</span><input type="date" value={to} onChange={e => setTo(e.target.value)} min={from} aria-label="To" /></span>}
            </div>
          </Field>
          <Field group label="Person" help="Expenses they paid or took part in."><Select value={member} onChange={setMember} placeholder="Everyone" clearLabel="Everyone" options={(g?.members || []).map(m => ({ value: String(m.id), label: m.name }))} ariaLabel="Person" /></Field>
          <Field group label="Categories" help={cats.length ? `${cats.length} selected` : 'All categories. Click to narrow down.'}>
            <div className="chips">
              {categories.map(c => <button key={c.id} type="button" className={`chip sm ${cats.includes(c.id) ? 'active' : ''}`} onClick={() => toggleCat(c.id)}>{c.icon} {c.label}</button>)}
              {cats.length > 0 && <button type="button" className="chip sm" onClick={() => setCats([])}>✕ Clear</button>}
            </div>
          </Field>
          <label className="row"><input type="checkbox" checked={payments} onChange={e => setPayments(e.target.checked)} style={{ width: 'auto' }} /> Include settle-up payments (they are never counted as spending)</label>
        </div>
      </Card>

      {error && <div className="callout danger mb">{error}</div>}
      {loading && !rep ? <Loading /> : rep && (
        <>
          <p className="muted small mb-sm">{rep.group.name} · {rep.filter_text}{rep.first_date && ` · ${fmtDate(rep.first_date)} – ${fmtDate(rep.last_date)}`}</p>
          <div className="stats">
            <div className="stat"><div className="n"><Money minor={rep.total} code={cur} /></div><div className="l">Total spent · {rep.count} {rep.count === 1 ? 'expense' : 'expenses'}</div></div>
            <div className="stat"><div className="n"><Money minor={rep.per_head} code={cur} /></div><div className="l">Per person, split evenly</div></div>
            <div className="stat"><div className="n">{rep.categories.length}</div><div className="l">{rep.categories.length === 1 ? 'Category' : 'Categories'}</div></div>
            <div className="stat"><div className="n"><Money minor={rep.reimbursed} code={cur} /></div><div className="l">Settle-up payments{!payments && ' (not shown)'}</div></div>
          </div>
          {rep.count === 0 && rep.expenses.length === 0 ? <Empty title="Nothing in this selection">Widen the period or clear the filters.</Empty> : (
            <>
              <div className="two-col">
                <Card title="Per person" flush>
                  <table className="table compact">
                    <thead><tr><th>Person</th><th className="right">Paid for</th><th className="right">Paid</th><th className="right">Share</th><th className="right">Net</th></tr></thead>
                    <tbody>
                      {rep.members.map(m => <tr key={m.member_id}><td><span className="row"><MemberAvatar member={{ id: m.member_id, name: m.name }} />{m.name}</span></td><td className="right muted">{m.count}</td><td className="right"><Money minor={m.paid} code={cur} /></td><td className="right"><Money minor={m.share} code={cur} /></td><td className="right"><Money minor={m.net} code={cur} sign /></td></tr>)}
                      <tr><td><b>Total</b></td><td className="right muted">{rep.count}</td><td className="right"><b><Money minor={rep.members.reduce((a, m) => a + m.paid, 0)} code={cur} /></b></td><td className="right"><b><Money minor={rep.members.reduce((a, m) => a + m.share, 0)} code={cur} /></b></td><td className="right muted">—</td></tr>
                    </tbody>
                  </table>
                </Card>
                <Card title="By category">
                  <div className="rating-bars">
                    {rep.categories.map(c => (
                      <div key={c.id} className="rb-row cat">
                        <span className="l">{c.icon} {c.label}</span>
                        <span className="track"><span className="b" style={{ width: `${Math.round((c.total / maxCat) * 100)}%` }} /></span>
                        <span className="n"><Money minor={c.total} code={cur} /> <span className="muted small">{c.percent.toFixed(0)}%</span></span>
                      </div>
                    ))}
                  </div>
                </Card>
              </div>
              {rep.months.length > 1 && (
                <Card title="By month" className="mt">
                  <div className="month-bars" style={{ gridTemplateColumns: `repeat(${rep.months.length}, 1fr)` }}>
                    {rep.months.map(m => <div key={m.month} className="mb-col" title={`${fmtMonth(m.month)}: ${m.count} expenses`}><span className="v"><Money minor={m.total} code={cur} /></span><span className="b" style={{ height: `${Math.max(2, Math.round((m.total / maxMonth) * 120))}px` }} /><span className="l">{fmtMonth(m.month).slice(0, 3)} ’{m.month.slice(2, 4)}</span></div>)}
                  </div>
                </Card>
              )}
              <Card title={`Expenses (${rep.expenses.length})`} className="mt" flush>
                <table className="table compact report-table">
                  <thead><tr><th>Date</th><th>Expense</th><th>Category</th><th>Paid by</th><th>For</th><th className="right">Amount</th></tr></thead>
                  <tbody>
                    {rep.expenses.map(e => (
                      <tr key={e.id} className={e.reimbursement ? 'muted' : ''}>
                        <td className="muted" style={{ whiteSpace: 'nowrap' }}>{fmtDate(e.date)}</td>
                        <td><Link to={`/groups/${rep.group.id}/expenses/${e.id}`}>{e.title}</Link>{e.notes && <div className="muted small">{e.notes}</div>}</td>
                        <td className="muted">{e.category_label}</td>
                        <td>{e.paid_by}</td>
                        <td className="muted">{e.for}</td>
                        <td className="right" style={{ whiteSpace: 'nowrap' }}><Money minor={e.amount_group} code={cur} />{e.currency !== cur && <div className="muted small"><Money minor={e.amount} code={e.currency} /></div>}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </Card>
              {rep.currencies.length > 1 && (
                <Card title="Currencies used" className="mt">
                  <table className="table compact"><thead><tr><th>Currency</th><th>Expenses</th><th className="right">Paid</th></tr></thead>
                    <tbody>{rep.currencies.map(c => <tr key={c.currency}><td><span className="badge cat">{c.currency}</span></td><td className="muted">{c.count}</td><td className="right"><Money minor={c.total} code={c.currency} /></td></tr>)}</tbody></table>
                </Card>
              )}
            </>
          )}
        </>
      )}
    </>
  )
}
