import { ANALYTICS_TIMEZONE, type AnalyticsDateRange } from '../api/analytics'

export type AnalyticsDatePreset = 'today' | 'yesterday' | 'last7' | 'last30' | 'custom'

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/
export const MAX_ANALYTICS_RANGE_DAYS = 366

function calendarDateFromUTC(date: Date) {
  return date.toISOString().slice(0, 10)
}

function calendarDateParts(value: string) {
  if (!DATE_PATTERN.test(value)) {
    return null
  }
  const [year, month, day] = value.split('-').map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  return calendarDateFromUTC(date) === value ? { year, month, day } : null
}

export function analyticsToday(now = new Date()) {
  const formatter = new Intl.DateTimeFormat('en-CA', {
    timeZone: ANALYTICS_TIMEZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  const parts = formatter.formatToParts(now)
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]))
  return `${values.year}-${values.month}-${values.day}`
}

export function shiftAnalyticsDate(value: string, days: number) {
  const parts = calendarDateParts(value)
  if (!parts) {
    return ''
  }
  return calendarDateFromUTC(new Date(Date.UTC(parts.year, parts.month - 1, parts.day + days)))
}

export function analyticsPresetRange(preset: Exclude<AnalyticsDatePreset, 'custom'>, now = new Date()): AnalyticsDateRange {
  const today = analyticsToday(now)
  switch (preset) {
    case 'today':
      return { from: today, to: today, timezone: ANALYTICS_TIMEZONE }
    case 'yesterday': {
      const yesterday = shiftAnalyticsDate(today, -1)
      return { from: yesterday, to: yesterday, timezone: ANALYTICS_TIMEZONE }
    }
    case 'last30':
      return { from: shiftAnalyticsDate(today, -29), to: today, timezone: ANALYTICS_TIMEZONE }
    case 'last7':
      return { from: shiftAnalyticsDate(today, -6), to: today, timezone: ANALYTICS_TIMEZONE }
  }
}

export function analyticsInclusiveDays(from: string, to: string) {
  const fromParts = calendarDateParts(from)
  const toParts = calendarDateParts(to)
  if (!fromParts || !toParts) {
    return 0
  }
  const fromTime = Date.UTC(fromParts.year, fromParts.month - 1, fromParts.day)
  const toTime = Date.UTC(toParts.year, toParts.month - 1, toParts.day)
  return Math.floor((toTime - fromTime) / 86_400_000) + 1
}

export function validateAnalyticsDateRange(range: Pick<AnalyticsDateRange, 'from' | 'to'>) {
  const days = analyticsInclusiveDays(range.from, range.to)
  if (days <= 0) {
    return '开始日期不能晚于结束日期'
  }
  if (days > MAX_ANALYTICS_RANGE_DAYS) {
    return `查询范围不能超过 ${MAX_ANALYTICS_RANGE_DAYS} 天`
  }
  return ''
}

export function normalizedAnalyticsRange(from: unknown, to: unknown, now = new Date()): AnalyticsDateRange {
  const fallback = analyticsPresetRange('last7', now)
  const candidate = {
    from: typeof from === 'string' ? from.trim() : '',
    to: typeof to === 'string' ? to.trim() : '',
    timezone: ANALYTICS_TIMEZONE,
  }
  return validateAnalyticsDateRange(candidate) ? fallback : candidate
}
