import type { Member, User } from '../lib/types'
import { colorFor, initials } from '../lib/money'

export function Avatar({ user, size = '' }: { user: User; size?: '' | 'lg' }) {
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
      {shown.map(m => <MemberAvatar key={m.id} member={m} />)}
      {members.length > max && <span className="avatar more">+{members.length - max}</span>}
    </span>
  )
}
