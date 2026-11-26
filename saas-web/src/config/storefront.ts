export function parseStorefrontFeatureFlag(value: unknown, defaultValue = false) {
  if (typeof value !== 'string') {
    return defaultValue
  }
  return value.trim().toLowerCase() === 'true'
}

export type ReservationMode = 'disabled' | 'anonymous' | 'otp'

export function parseReservationMode(value: unknown): ReservationMode {
  if (typeof value !== 'string') {
    return 'disabled'
  }
  const normalized = value.trim().toLowerCase()
  return normalized === 'anonymous' || normalized === 'otp' ? normalized : 'disabled'
}

const storefrontEnvironment = typeof import.meta.env === 'object' ? import.meta.env : undefined

export const storefrontPriceVisible = parseStorefrontFeatureFlag(
  storefrontEnvironment?.VITE_STOREFRONT_PRICE_VISIBLE,
)

export const reservationMode = parseReservationMode(storefrontEnvironment?.VITE_RESERVATION_MODE)
export const reservationEnabled = reservationMode !== 'disabled'
export const anonymousReservationEnabled = reservationMode === 'anonymous'
export const otpReservationEnabled = reservationMode === 'otp'
