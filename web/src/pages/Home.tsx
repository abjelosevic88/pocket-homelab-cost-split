import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Activity, Dashboard } from '../lib/types'
import { ago, fmtDate } from '../lib/money'
import { Card, Empty, Loading, PageHeader, useAsync } from '../components/ui'
import { Money } from '../components/Money'
import { ActivityKind } from './Group'

export default function Home() {
  const { user } = useApp()
  const navigate = useNavigate()
  const { data: d, loading } = useAsync(() => api.get<Dashboard>(`${V1}/dashboard`), [])
  const { data: act } = useAsync(() => api.get<Activity[]>(`${V1}/activity?limit=12`), [])
  if (loading || !d) return <Loading />
  const hour = new Date().getHours()
  const hello = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const owe = d.positions.filter(p => p.you_owe > 0)
  const owed = d.positions.filter(p => p.you_are_owed > 0)
  return (
    <>
      <PageHeader title={`${hello}, ${user?.name?.split(' ')[0] || 'there'}`} sub={d.groups === 0 ? 'No groups yet. Create one for a trip, a flat or a dinner.' : `${d.groups} active ${d.groups === 1 ? 'group' : 'groups'}${d.archived_groups ? `, ${d.archived_groups} archived` : ''}`}
        actions={<button className="btn primary" onClick={() => navigate('/groups?new=1')}>+ New group</button>} />
      <div className="stats">
        <div className="stat"><div className="l">You owe</div><div className="n bad">{owe.length ? owe.map(p => <div key={p.currency}><Money minor={p.you_owe} code={p.currency} /></div>) : <span className="muted">—</span>}</div></div>
        <div className="stat"><div className="l">You are owed</div><div className="n ok">{owed.length ? owed.map(p => <div key={p.currency}><Money minor={p.you_are_owed} code={p.currency} /></div>) : <span className="muted">—</span>}</div></div>
        <div className="stat"><div className="l">Your share this month</div><div className="n">{d.spent_this_month.length ? d.spent_this_month.map(c => <div key={c.currency}><Money minor={c.total} code={c.currency} /></div>) : <span className="muted">—</span>}</div></div>
      </div>
      <div className="two-col">
        <Card title="Your groups" actions={<Link className="btn ghost sm" to="/groups">All groups →</Link>}>
          {d.group_lines.length === 0 ? <Empty title="Nothing shared yet"><Link to="/groups?new=1">Create your first group</Link>.</Empty> : (
            <ul className="mini-list">
              {d.group_lines.map(g => (
                <li key={g.id}>
                  <Link to={`/groups/${g.id}`} className="row grow">
                    <span className="grow">
                      <b>{g.name}</b>
                      <div className="muted small">{g.members} {g.members === 1 ? 'person' : 'people'} · {g.expenses} {g.expenses === 1 ? 'expense' : 'expenses'} · <Money minor={g.total} code={g.currency} />{g.last_date && ` · last ${fmtDate(g.last_date, false)}`}</div>
                    </span>
                    <span className="right"><Money minor={g.balance} code={g.currency} sign /><div className="muted small">{g.balance > 0 ? 'you get back' : g.balance < 0 ? 'you owe' : 'settled'}</div></span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card title="Recent activity">
          {!act || act.length === 0 ? <p className="muted">Nothing happened yet.</p> : (
            <ul className="mini-list">
              {act.map(a => (
                <li key={a.id}>
                  <Link to={a.expense_id && !a.kind.endsWith('deleted') ? `/groups/${a.group_id}/expenses/${a.expense_id}` : `/groups/${a.group_id}`} className="row grow">
                    <span className="grow"><ActivityKind kind={a.kind} /> <span>{a.summary}</span><div className="muted small">{a.group_name} · {a.user_name || 'someone'} · {ago(a.created_at)}</div></span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </>
  )
}
