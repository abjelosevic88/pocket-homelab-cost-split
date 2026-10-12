import type { Currency, Member } from './types'

/** Decimals for a currency code; 2 when the list is not loaded yet. */
export function decimalsOf(code: string, currencies: Currency[]): number {
  const c = currencies.find(x => x.code === code)
  return c ? c.decimals : 2
}

const fmtCache = new Map<string, Intl.NumberFormat>()
function formatter(code: string, decimals: number, display: 'symbol' | 'code' = 'symbol'): Intl.NumberFormat {
  const key = `${code}:${decimals}:${display}`
  let f = fmtCache.get(key)
  if (!f) {
    try {
      f = new Intl.NumberFormat(undefined, { style: 'currency', currency: code, currencyDisplay: display === 'symbol' ? 'narrowSymbol' : 'code', minimumFractionDigits: decimals, maximumFractionDigits: decimals })
    } catch {
      f = new Intl.NumberFormat(undefined, { minimumFractionDigits: decimals, maximumFractionDigits: decimals })
    }
    fmtCache.set(key, f)
  }
  return f
}

/** Minor units → "12.50 €", "¥1,500", "1.250 BHD". Unknown codes fall back to "12.50 XYZ". */
export function fmtMoney(minor: number, code: string, decimals = 2, opts: { sign?: boolean; code?: boolean } = {}): string {
  const major = minor / Math.pow(10, decimals)
  let s: string
  try {
    s = formatter(code, decimals, opts.code ? 'code' : 'symbol').format(Math.abs(major))
    // Intl falls back to the plain number for codes it does not know: add the code ourselves.
    if (!/[^\d.,\s\u00a0]/.test(s)) s = `${s} ${code}`
  } catch {
    s = `${Math.abs(major).toFixed(decimals)} ${code}`
  }
  if (minor < 0) return `−${s}`
  if (opts.sign && minor > 0) return `+${s}`
  return s
}

/** Minor units → plain "12.50" for inputs. */
export function toMajor(minor: number, decimals: number): string {
  return (minor / Math.pow(10, decimals)).toFixed(decimals)
}

/** "12,50" / "12.5" / "1 250" → minor units, or null when not a number. */
export function toMinor(s: string, decimals: number): number | null {
  const t = s.replace(/\s/g, '').replace(',', '.')
  if (t === '' || t === '-' || t === '.') return null
  if (!/^-?\d*(\.\d*)?$/.test(t)) return null
  const n = Number(t)
  if (!Number.isFinite(n)) return null
  return Math.round(n * Math.pow(10, decimals))
}

export function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function ago(iso: string): string {
  const t = new Date(iso).getTime()
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  if (s < 86400 * 7) return `${Math.floor(s / 86400)} d ago`
  return new Date(iso).toLocaleDateString()
}

/** "2026-03-14" → "14 Mar 2026"; "" stays "". */
export function fmtDate(d: string, withYear = true): string {
  if (!d) return ''
  const [y, m, day] = d.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, day).toLocaleDateString(undefined, withYear ? { day: 'numeric', month: 'short', year: 'numeric' } : { day: 'numeric', month: 'short' })
}

/** "2026-11" → "November 2026". */
export function fmtMonth(ym: string): string {
  const [y, m] = ym.split('-').map(Number)
  return new Date(y, m - 1, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
}

export function initials(name: string): string {
  const parts = name.trim().split(/[\s@._-]+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}

const PALETTE = ['#2563eb', '#7c3aed', '#0891b2', '#059669', '#d97706', '#dc2626', '#db2777', '#4b5563', '#b45309', '#0d9488']
export function colorFor(id: number): string { return PALETTE[Math.abs(id) % PALETTE.length] }

export function memberName(members: Member[], id: number): string {
  return members.find(m => m.id === id)?.name || '?'
}

/** "Ana, Marko and Jelena" / "everyone" when all members are in. */
export function listNames(members: Member[], ids: number[]): string {
  if (ids.length === members.length && members.length > 1) return 'everyone'
  const names = ids.map(id => memberName(members, id))
  if (names.length <= 1) return names.join('')
  return names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1]
}

export const SPLIT_LABEL: Record<string, string> = { even: 'Evenly', shares: 'By shares', percent: 'By percentage', amount: 'By amount' }

import { useEffect, useState } from 'react'
export function useDebounceValue<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value)
  useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t) }, [value, ms])
  return v
}
