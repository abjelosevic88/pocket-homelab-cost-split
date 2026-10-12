import { useEffect, useState } from 'react'
import { useApp } from '../lib/app-context'
import { decimalsOf, fmtMoney, toMajor, toMinor } from '../lib/money'
import { Select } from './Select'

/** Formatted money that knows the currency list. */
export function Money({ minor, code, sign = false, className = '', showCode = false }: { minor: number; code: string; sign?: boolean; className?: string; showCode?: boolean }) {
  const { currencies } = useApp()
  const tone = sign ? (minor > 0 ? 'ok' : minor < 0 ? 'bad' : 'muted') : ''
  return <span className={`money ${tone} ${className}`}>{fmtMoney(minor, code, decimalsOf(code, currencies), { sign, code: showCode })}</span>
}

/** Text input for an amount in major units; reports minor units (or null). */
export function MoneyInput({ value, code, onChange, autoFocus, placeholder = '0.00', className = '', disabled, ariaLabel }: { value: number | null; code: string; onChange: (minor: number | null) => void; autoFocus?: boolean; placeholder?: string; className?: string; disabled?: boolean; ariaLabel?: string }) {
  const { currencies } = useApp()
  const decimals = decimalsOf(code, currencies)
  const [text, setText] = useState(value == null ? '' : toMajor(value, decimals))
  // Adopt outside changes (e.g. a recomputed split) unless the user is mid-edit on the same value.
  useEffect(() => { if (value == null) { if (text !== '' && toMinor(text, decimals) !== null) setText('') } else if (toMinor(text, decimals) !== value) setText(toMajor(value, decimals)) }, [value, decimals]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <input className={`money-input ${className}`} inputMode="decimal" value={text} placeholder={placeholder} autoFocus={autoFocus} disabled={disabled} aria-label={ariaLabel}
      onChange={e => { setText(e.target.value); onChange(toMinor(e.target.value, decimals)) }}
      onBlur={() => { const m = toMinor(text, decimals); if (m !== null) setText(toMajor(m, decimals)) }} />
  )
}

/** Searchable currency picker over the ISO list, the usual ones first. */
const COMMON = ['EUR', 'USD', 'GBP', 'CHF', 'BAM', 'RSD', 'JPY', 'CAD', 'AUD', 'PLN', 'CZK', 'HUF', 'TRY']
export function CurrencySelect({ value, onChange, className = '', disabled, ariaLabel = 'Currency' }: { value: string; onChange: (code: string) => void; className?: string; disabled?: boolean; ariaLabel?: string }) {
  const { currencies } = useApp()
  const isCommon = (c: string) => COMMON.includes(c)
  // Rows are keyed by value, so the common block gets a "c:" prefix and the full list stays plain.
  const options = [
    ...COMMON.map(code => currencies.find(x => x.code === code)).filter((c): c is NonNullable<typeof c> => Boolean(c)).map(c => ({ value: 'c:' + c.code, label: c.code, hint: c.name, group: 'Common' })),
    ...currencies.map(c => ({ value: c.code, label: c.code, hint: c.name, group: 'All currencies' })),
  ]
  const cur = value ? (isCommon(value) ? 'c:' + value : value) : ''
  return <Select className={`currency-select ${className}`} value={cur} onChange={v => onChange(v.replace(/^c:/, ''))} options={options} placeholder="Currency…" ariaLabel={ariaLabel} disabled={disabled} />
}
