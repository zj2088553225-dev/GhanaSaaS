import type { ReservationHeatLevel } from '../api/reservations'

export type ReservationDialogState = 'contact' | 'challenge' | 'code' | 'success' | 'error'

export const RESERVATION_COUNTRIES = [
  { code: 'GH', callingCode: '+233', label: 'Ghana' },
  { code: 'CN', callingCode: '+86', label: 'China' },
  { code: 'US', callingCode: '+1', label: 'United States' },
  { code: 'GB', callingCode: '+44', label: 'United Kingdom' },
  { code: 'NG', callingCode: '+234', label: 'Nigeria' },
  { code: 'CI', callingCode: '+225', label: "Côte d’Ivoire" },
] as const

export function newReservationID(randomUUID?: () => string) {
  const generator = randomUUID ?? globalThis.crypto?.randomUUID?.bind(globalThis.crypto)
  if (!generator) {
    throw new Error('Secure UUID generation is unavailable')
  }
  return generator()
}

export function reservationHeatTranslationKey(level: ReservationHeatLevel) {
  return `reservations.heat.${level}` as const
}

export function reservationWindowLabel(
  startAt: string | null,
  endAt: string | null,
  locale: 'en' | 'zh',
  fallback: string,
) {
  if (!startAt && !endAt) {
    return fallback
  }
  const formatter = new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en-GH', {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
  const format = (value: string | null) => value ? formatter.format(new Date(value)) : ''
  if (startAt && endAt) {
    return `${format(startAt)} – ${format(endAt)}`
  }
  return format(startAt || endAt)
}

export function nextReservationDialogState(state: ReservationDialogState, event: 'request' | 'requested' | 'verified' | 'fail' | 'retry' | 'reset'): ReservationDialogState {
  if (event === 'reset') return 'contact'
  if (event === 'fail') return 'error'
  if (event === 'retry') return state === 'error' ? 'contact' : state
  if (event === 'request' && (state === 'contact' || state === 'code')) return 'challenge'
  if (event === 'requested' && state === 'challenge') return 'code'
  if (event === 'verified' && state === 'code') return 'success'
  return state
}

export function reservationManageTokenKey(productID: number) {
  return `gc_reservation_manage_${productID}`
}

export interface ReservationOperationGuard {
  capture(): number
  invalidate(): number
  isCurrent(sequence: number): boolean
}

// Components use this guard to prevent a response from an earlier dialog,
// product, locale, or route instance from mutating the current UI state.
export function createReservationOperationGuard(): ReservationOperationGuard {
  let sequence = 0
  return {
    capture: () => sequence,
    invalidate: () => ++sequence,
    isCurrent: (captured) => captured === sequence,
  }
}
