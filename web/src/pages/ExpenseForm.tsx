import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Expense, FxAnswer, Group, SplitMode } from '../lib/types'
import { decimalsOf, fmtMoney, SPLIT_LABEL, todayISO } from '../lib/money'
import { Card, Confirm, Empty, Field, Loading, PageHeader, Segmented, useAsync, useToast } from '../components/ui'
import { Money, MoneyInput, CurrencySelect } from '../components/Money'
import { MemberAvatar } from '../components/people'
import { Select } from '../components/Select'

interface Row { member_id: number; on: boolean; share: number | null }

/** Largest-remainder apportioning, same as the server, for the live preview. */
function apportion(total: number, weights: number[]): number[] {
  const sum = weights.reduce((a, b) => a + b, 0)
  if (sum <= 0) return weights.map(() => 0)
  const neg = total < 0
  const t = Math.abs(total)
  const parts = weights.map(w => Math.floor((t * w) / sum))
  const rems = weights.map((w, i) => ({ i, r: (t * w) % sum })).sort((a, b) => b.r - a.r || a.i - b.i)
  let left = t - parts.reduce((a, b) => a + b, 0)
  for (let k = 0; left > 0; k = (k + 1) % rems.length, left--) parts[rems[k].i]++
  return neg ? parts.map(p => -p) : parts
}

export default function ExpenseForm() {
  const { id, eid } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { categories, currencies, settings, reloadMeta } = useApp()
  const { data: g } = useAsync(() => api.get<Group>(`${V1}/groups/${id}`), [id])
  const { data: existing, loading: loadingE } = useAsync(() => eid ? api.get<Expense>(`${V1}/groups/${id}/expenses/${eid}`) : Promise.resolve(null), [id, eid])
  const editing = Boolean(eid)

  const [title, setTitle] = useState('')
  const [amount, setAmount] = useState<number | null>(null)
  const [currency, setCurrency] = useState('')
  const [rate, setRate] = useState<string>('')
  const [rateInfo, setRateInfo] = useState<{ source: string; date: string } | null>(null)
  const [rateBusy, setRateBusy] = useState(false)
  const rateTouched = useRef(false)
  const [paidBy, setPaidBy] = useState('')
  const [date, setDate] = useState(todayISO())
  const [category, setCategory] = useState('general')
  const [notes, setNotes] = useState('')
  const [mode, setMode] = useState<SplitMode>('even')
  const [reimb, setReimb] = useState(false)
  const [rows, setRows] = useState<Row[]>([])
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const [ready, setReady] = useState(false)

  // Fill the form once the group (and, when editing, the expense) is here.
  useEffect(() => {
    if (!g || ready || (editing && loadingE)) return
    if (editing && existing) {
      setTitle(existing.title); setAmount(existing.amount); setCurrency(existing.currency); setRate(String(existing.fx_rate)); setPaidBy(String(existing.paid_by))
      setDate(existing.date); setCategory(existing.category); setNotes(existing.notes); setMode(existing.split_mode); setReimb(existing.reimbursement)
      setRows(g.members.map(m => { const s = existing.shares.find(x => x.member_id === m.id); return { member_id: m.id, on: Boolean(s), share: s ? s.share : null } }))
      rateTouched.current = true
    } else if (!editing) {
      setCurrency(g.currency); setPaidBy(String(g.my_member_id || g.members[0]?.id || ''))
      setRows(g.members.map(m => ({ member_id: m.id, on: true, share: null })))
    }
    setReady(true)
  }, [g, existing, loadingE, editing, ready])

  // Exchange rate: look it up when the currency or date changes, unless typed by hand.
  const foreign = Boolean(g && currency && currency !== g.currency)
  useEffect(() => {
    if (!g || !foreign || !ready) { setRateInfo(null); return }
    if (rateTouched.current) return
    let cancelled = false
    setRateBusy(true)
    api.get<FxAnswer>(`${V1}/fx?from=${currency}&to=${g.currency}&date=${date}`)
      .then(r => { if (!cancelled) { setRate(String(r.rate)); setRateInfo({ source: r.source, date: r.date }) } })
      .catch(() => { if (!cancelled) { setRateInfo({ source: '', date }); if (!rate) setRate('') } })
      .finally(() => { if (!cancelled) setRateBusy(false) })
    return () => { cancelled = true }
  }, [g, currency, date, foreign, ready]) // eslint-disable-line react-hooks/exhaustive-deps

  const rateNum = Number(rate.replace(',', '.'))
  const dExp = decimalsOf(currency, currencies), dGrp = g ? decimalsOf(g.currency, currencies) : 2
  const amountGroup = useMemo(() => {
    if (amount == null || !g) return null
    if (!foreign) return amount
    if (!(rateNum > 0)) return null
    return Math.round((amount / Math.pow(10, dExp)) * rateNum * Math.pow(10, dGrp))
  }, [amount, foreign, rateNum, dExp, dGrp, g])

  // Live preview of each person's part.
  const active = rows.filter(r => r.on)
  const preview = useMemo(() => {
    if (amount == null || active.length === 0) return null
    const weights = active.map(r => mode === 'even' ? 1 : (r.share ?? 0))
    if (mode === 'amount') return { parts: active.map(r => r.share ?? 0), ok: active.reduce((a, r) => a + (r.share ?? 0), 0) === amount }
    if (mode === 'percent') return { parts: apportion(amount, weights), ok: weights.reduce((a, b) => a + b, 0) === 10000 }
    return { parts: apportion(amount, weights), ok: weights.reduce((a, b) => a + b, 0) > 0 }
  }, [amount, active, mode])

  if (!g || !ready) return <Loading />
  if (editing && !existing) return <Empty title="Expense not found"><Link to={`/groups/${g.id}`}>Back to the group</Link></Empty>

  const setRow = (id: number, p: Partial<Row>) => setRows(rs => rs.map(r => r.member_id === id ? { ...r, ...p } : r))
  const changeMode = (m: SplitMode) => {
    setMode(m)
    setRows(rs => rs.map(r => ({ ...r, share: m === 'shares' ? (r.share && r.share > 0 && r.share < 1000 ? r.share : 1) : m === 'percent' ? (active.length ? Math.floor(10000 / active.length) : 0) : null })))
    if (m === 'percent') setRows(rs => { const on = rs.filter(r => r.on); const parts = apportion(10000, on.map(() => 1)); let i = 0; return rs.map(r => r.on ? { ...r, share: parts[i++] } : r) })
    if (m === 'amount' && amount != null) setRows(rs => { const on = rs.filter(r => r.on); const parts = apportion(amount, on.map(() => 1)); let i = 0; return rs.map(r => r.on ? { ...r, share: parts[i++] } : r) })
  }
  const toggleReimb = (on: boolean) => {
    setReimb(on); setMode('even')
    if (on) { setTitle(t => t || 'Reimbursement'); setCategory('general'); const other = g.members.find(m => String(m.id) !== paidBy); setRows(rs => rs.map(r => ({ ...r, on: other ? r.member_id === other.id : false, share: null }))) }
    else setRows(rs => rs.map(r => ({ ...r, on: true, share: null })))
  }
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (amount == null || amount === 0) { toast('Enter the amount', 'err'); return }
    if (foreign && !(rateNum > 0)) { toast('Enter the exchange rate', 'err'); return }
    if (active.length === 0) { toast('Pick who it was for', 'err'); return }
    if (preview && !preview.ok) { toast(mode === 'percent' ? 'Percentages must add up to 100' : mode === 'amount' ? 'The amounts must add up to the total' : 'Shares must be positive', 'err'); return }
    setBusy(true)
    const body = { title, amount, currency, fx_rate: foreign ? rateNum : 1, paid_by: Number(paidBy), date, category, notes, split_mode: mode, reimbursement: reimb,
      shares: active.map(r => ({ member_id: r.member_id, share: mode === 'even' ? 1 : (r.share ?? 0) })) }
    try {
      if (editing) await api.put(`${V1}/groups/${g.id}/expenses/${eid}`, body); else await api.post(`${V1}/groups/${g.id}/expenses`, body)
      toast(editing ? 'Saved' : 'Expense added'); navigate(`/groups/${g.id}`)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${V1}/groups/${g.id}/expenses/${eid}`); toast('Deleted'); navigate(`/groups/${g.id}`) } catch (e) { toast((e as Error).message, 'err') } }
  const memberOpts = g.members.map(m => ({ value: String(m.id), label: m.name }))
  const fmtPct = (bp: number | null) => bp == null ? '' : (bp / 100).toFixed(2).replace(/\.?0+$/, '')

  return (
    <>
      <PageHeader title={editing ? (reimb ? 'Payment' : 'Edit expense') : (reimb ? 'Record a payment' : 'New expense')} sub={<Link to={`/groups/${g.id}`}>← {g.name}</Link>}
        actions={editing && <button className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>} />
      <form onSubmit={submit} className="expense-form">
        <Card title="What">
          <div className="grid" style={{ gap: 14 }}>
            <label className="row"><input type="checkbox" checked={reimb} onChange={e => toggleReimb(e.target.checked)} style={{ width: 'auto' }} /> This is a payment between two people (a reimbursement), not a shared cost</label>
            <Field label="Title"><input value={title} onChange={e => setTitle(e.target.value)} placeholder={reimb ? 'Reimbursement' : 'Dinner, taxi, groceries…'} autoFocus={!editing} required={!reimb} maxLength={120} /></Field>
            <Field group label="Amount" help={foreign ? undefined : 'Paid in another currency? Pick it on the left; it is converted at the day\u2019s rate.'}>
              <div className="amount-field">
                <CurrencySelect value={currency} onChange={c => { rateTouched.current = false; setCurrency(c) }} />
                <MoneyInput value={amount} code={currency} onChange={setAmount} ariaLabel="Amount" />
              </div>
            </Field>
            {foreign && (
              <div className="fx-box">
                <Field label={`Exchange rate · 1 ${currency} =`} help={rateBusy ? 'Looking the rate up…' : rateInfo?.source ? `From ${rateInfo.source}${rateInfo.date !== date ? ` for ${rateInfo.date}` : ''}. Change it if your bank charged differently.` : settings?.fx_enabled ? 'No rate found online — enter it by hand.' : 'Automatic lookups are off (Settings) — enter the rate by hand.'}>
                  <span className="row"><input inputMode="decimal" value={rate} onChange={e => { rateTouched.current = true; setRate(e.target.value); setRateInfo(null) }} placeholder="0.00" style={{ maxWidth: 160 }} aria-label="Exchange rate" /><span className="muted">{g.currency}</span>
                    <button type="button" className="btn ghost sm" onClick={() => { rateTouched.current = false; setRate(''); setRateInfo(null); setCurrency(c => c) ; api.get<FxAnswer>(`${V1}/fx?from=${currency}&to=${g.currency}&date=${date}`).then(r => { setRate(String(r.rate)); setRateInfo({ source: r.source, date: r.date }) }).catch(e => toast((e as Error).message, 'err')) }} title="Look up again">↻</button></span>
                </Field>
                {amount != null && amountGroup != null && <div className="muted">= <b><Money minor={amountGroup} code={g.currency} /></b> counted in the group</div>}
              </div>
            )}
            <div className="two-col tight">
              <Field group label={reimb ? 'Paid by' : 'Who paid'}><Select value={paidBy} onChange={setPaidBy} options={memberOpts} ariaLabel="Paid by" /></Field>
              <Field label="Date"><input type="date" value={date} onChange={e => { setDate(e.target.value); if (!rateTouched.current) setRateInfo(null) }} required /></Field>
            </div>
            {!reimb && <Field group label="Category" help="Type a name that is not in the list to add it.">
              <Select value={category} onChange={setCategory} options={categories.map(c => ({ value: c.id, label: `${c.icon} ${c.label}` }))} ariaLabel="Category" createLabel="Add category"
                onCreate={async label => { const c = await api.post<{ id: string }>(`${V1}/categories`, { label }); await reloadMeta(); return c.id }} />
            </Field>}
            <Field label="Notes"><textarea value={notes} onChange={e => setNotes(e.target.value)} rows={2} placeholder={reimb ? 'Cash, bank transfer…' : 'Anything worth remembering'} /></Field>
          </div>
        </Card>
        <Card title={reimb ? 'To whom' : 'For whom'}>
          {!reimb && (
            <div className="row wrap mb">
              <Segmented size="sm" value={mode} onChange={changeMode} options={(['even', 'shares', 'percent', 'amount'] as SplitMode[]).map(m => ({ value: m, label: SPLIT_LABEL[m] }))} />
              <span className="spacer" />
              <button type="button" className="btn ghost sm" onClick={() => setRows(rs => rs.map(r => ({ ...r, on: true })))}>All</button>
              <button type="button" className="btn ghost sm" onClick={() => setRows(rs => rs.map(r => ({ ...r, on: false })))}>None</button>
            </div>
          )}
          <div className="split-list">
            {rows.map((r, idx) => {
              const m = g.members.find(x => x.id === r.member_id)!
              const i = active.findIndex(a => a.member_id === r.member_id)
              const part = preview && i >= 0 ? preview.parts[i] : null
              return (
                <label key={r.member_id} className={`split-row ${r.on ? 'on' : ''}`}>
                  <input type={reimb ? 'radio' : 'checkbox'} name={reimb ? 'to' : undefined} checked={r.on} onChange={e => reimb ? setRows(rs => rs.map(x => ({ ...x, on: x.member_id === r.member_id }))) : setRow(r.member_id, { on: e.target.checked })} style={{ width: 'auto' }} />
                  <MemberAvatar member={m} me={m.id === g.my_member_id} />
                  <span className="nm">{m.name}{m.id === g.my_member_id && <span className="muted"> (you)</span>}</span>
                  <span className="spacer" />
                  {r.on && mode === 'shares' && <span className="row"><input type="number" min={0} step={1} value={r.share ?? ''} onChange={e => setRow(r.member_id, { share: e.target.value === '' ? null : Math.max(0, Math.floor(Number(e.target.value))) })} className="num" aria-label={`Shares for ${m.name}`} /><span className="muted small">shares</span></span>}
                  {r.on && mode === 'percent' && <span className="row"><input inputMode="decimal" value={fmtPct(r.share)} onChange={e => { const v = e.target.value.replace(',', '.'); setRow(r.member_id, { share: v === '' ? null : Math.round(Number(v) * 100) }) }} className="num" aria-label={`Percent for ${m.name}`} /><span className="muted small">%</span></span>}
                  {r.on && mode === 'amount' && <MoneyInput value={r.share} code={currency} onChange={v => setRow(r.member_id, { share: v })} className="num" ariaLabel={`Amount for ${m.name}`} />}
                  <span className={`part ${r.on ? '' : 'muted'}`}>{r.on && part != null ? fmtMoney(part, currency, dExp) : <span className="muted">—</span>}</span>
                  {idx === -1 && null}
                </label>
              )
            })}
          </div>
          {preview && !preview.ok && <div className="callout danger mt-sm">{mode === 'percent' ? `Percentages add up to ${fmtPct(active.reduce((a, r) => a + (r.share ?? 0), 0))} %, not 100.` : mode === 'amount' ? `The amounts add up to ${fmtMoney(active.reduce((a, r) => a + (r.share ?? 0), 0), currency, dExp)}, not ${fmtMoney(amount ?? 0, currency, dExp)}.` : 'Give at least one person a share.'}</div>}
          {reimb && active.length === 1 && String(active[0].member_id) === paidBy && <div className="callout danger mt-sm">Pick someone other than the payer.</div>}
          <div className="form-actions mt">
            <Link className="btn" to={`/groups/${g.id}`}>Cancel</Link>
            <button className="btn primary lg" disabled={busy}>{busy ? 'Saving…' : editing ? 'Save' : reimb ? 'Record payment' : 'Add expense'}</button>
          </div>
        </Card>
      </form>
      {del && <Confirm title="Delete this expense?" message="It disappears from the group and every balance is recomputed." onCancel={() => setDel(false)} onConfirm={remove} />}
    </>
  )
}
