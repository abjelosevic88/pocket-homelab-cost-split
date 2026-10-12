export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export interface Currency { code: string; name: string; decimals: number }
export interface Category { id: string; label: string; icon: string; builtin: boolean; sort_order: number; count: number }

export interface Member { id: number; group_id: number; user_id: number | null; name: string; created_at: string }

export interface Group {
  id: number
  name: string
  description: string
  currency: string
  created_by: number
  archived: boolean
  created_at: string
  updated_at: string
  members: Member[]
  expense_count: number
  total: number
  my_member_id: number
  my_balance: number
  last_activity: string
}

export type SplitMode = 'even' | 'shares' | 'percent' | 'amount'

export interface Share { member_id: number; share: number; amount: number; amount_group: number }

export interface Expense {
  id: number
  group_id: number
  title: string
  amount: number
  currency: string
  fx_rate: number
  amount_group: number
  paid_by: number
  date: string
  category: string
  notes: string
  split_mode: SplitMode
  reimbursement: boolean
  created_by: number
  created_at: string
  updated_at: string
  shares: Share[]
}

export interface Balance { member_id: number; paid: number; owed: number; balance: number }
export interface Transfer { from: number; to: number; amount: number }
export interface Balances { currency: string; balances: Balance[]; settlements: Transfer[] }

export interface GroupStats {
  currency: string
  total: number
  reimbursed: number
  count: number
  months: { month: string; total: number; count: number }[]
  categories: { category: string; total: number; count: number }[]
  members: { member_id: number; paid: number; share: number; count: number }[]
  currencies: { currency: string; total: number; count: number }[]
  first_date: string
  last_date: string
}

export interface Activity {
  id: number
  group_id: number
  group_name: string
  user_id: number
  user_name: string
  kind: string
  expense_id: number
  summary: string
  created_at: string
}

export interface Dashboard {
  groups: number
  archived_groups: number
  positions: { currency: string; you_owe: number; you_are_owed: number; net: number }[]
  you_owe_text: string
  you_are_owed_text: string
  net_text: string
  spent_this_month: { currency: string; total: number; count: number }[]
  spent_text: string
  group_lines: { id: number; name: string; currency: string; balance: number; members: number; expenses: number; total: number; last_date: string; settlements: { from: string; to: string; amount: number; from_me: boolean; to_me: boolean }[] }[]
}

export interface Settings {
  title: string
  default_currency: string
  fx_auto: boolean
  fx_provider: string
  fx_enabled: boolean
  date_format: DateFormat
  time_format: TimeFormat
}
export type DateFormat = 'auto' | 'dmy-dot' | 'dmy-slash' | 'mdy-slash' | 'ymd' | 'dmmmy' | 'mmmdy'
export type TimeFormat = 'auto' | '24h' | '12h'

export interface FxAnswer { from: string; to: string; date: string; rate: number; source: string; example: string }
