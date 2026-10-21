import { useNavigate } from 'react-router-dom'
import type { Group, Member, User } from '../lib/types'
import { colorFor, initials } from '../lib/money'
import { Modal } from './ui'
import { Money } from './Money'
import { Icon } from './icons'

export function Avatar({ user, size = '' }: { user: User; size?: '' | 'lg' | 'xl' }) {
  const name = user.name || user.email
  return <span className={`avatar ${size}`} style={{ background: colorFor(user.id) }} title={name}>{initials(name)}</span>
}

/** A member's avatar: coloured by member id so the same person looks the same across a group. */
export function MemberAvatar({ member, size = '', me = false }: { member: Pick<Member, 'id' | 'name'>; size?: '' | 'lg' | 'xl'; me?: boolean }) {
  return <span className={`avatar ${size} ${me ? 'me' : ''}`} style={{ background: colorFor(member.id * 7 + 3) }} title={member.name}>{initials(member.name)}</span>
}

/** A row of overlapping member avatars. */
export function MemberStack({ members, max = 5, myId = 0 }: { members: Member[]; max?: number; myId?: number }) {
  const shown = members.slice(0, max)
  return (
    <span className="avatar-stack">
      {shown.map(m => <MemberAvatar key={m.id} member={m} me={m.id === myId} />)}
      {members.length > max && <span className="avatar more">+{members.length - max}</span>}
    </span>
  )
}

/** A group's tile: its first letter on a colour that stays the same for the group. */
export function GroupIcon({ group, size = 22 }: { group: Pick<Group, 'id' | 'name'>; size?: number }) {
  const letter = (group.name.trim().match(/\p{L}|\p{N}/u)?.[0] || '·').toUpperCase()
  return <span className="grp-ico" style={{ ['--c' as string]: colorFor(group.id * 3 + 1), width: size, height: size, fontSize: Math.round(size * 0.48) }} aria-hidden="true">{letter}</span>
}

/** "Add an expense to…" — pick one of your groups. */
export function GroupPicker({ groups, onClose }: { groups: Group[]; onClose: () => void }) {
  const navigate = useNavigate()
  return (
    <Modal title="Add an expense to…" onClose={onClose}>
      <div className="pick-list">
        {groups.map(g => (
          <button type="button" key={g.id} className="pick-row" onClick={() => { onClose(); navigate(`/groups/${g.id}/expenses/new`) }}>
            <GroupIcon group={g} size={34} />
            <span className="grow"><b>{g.name}</b><span className="muted small block">{g.members.length} {g.members.length === 1 ? 'person' : 'people'} · {g.expense_count} {g.expense_count === 1 ? 'expense' : 'expenses'} · <Money minor={g.total} code={g.currency} /></span></span>
            <span className="badge cat">{g.currency}</span>
            <Icon name="chevron-right" size={16} className="muted" />
          </button>
        ))}
      </div>
    </Modal>
  )
}
