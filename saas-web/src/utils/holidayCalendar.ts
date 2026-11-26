import type { StorefrontHolidayItem } from '../api/holidays'

export interface HolidayCalendarDay {
  key: string
  date: string
  day: number
  weekend: boolean
}

export type HolidayCalendarCell = HolidayCalendarDay | null

export function buildHolidayCalendarDays(year: number, month: number): HolidayCalendarCell[] {
  if (!Number.isInteger(year) || !Number.isInteger(month) || year < 1 || month < 1 || month > 12) {
    return []
  }
  const firstWeekday = new Date(Date.UTC(year, month - 1, 1)).getUTCDay()
  const daysInMonth = new Date(Date.UTC(year, month, 0)).getUTCDate()
  const cells: HolidayCalendarCell[] = Array.from({ length: firstWeekday }, () => null)

  for (let day = 1; day <= daysInMonth; day += 1) {
    const date = calendarDate(year, month, day)
    const weekday = new Date(Date.UTC(year, month - 1, day)).getUTCDay()
    cells.push({ key: date, date, day, weekend: weekday === 0 || weekday === 6 })
  }

  const visibleCellCount = Math.ceil(cells.length / 7) * 7
  while (cells.length < visibleCellCount) {
    cells.push(null)
  }
  return cells
}

export function shiftHolidayMonth(year: number, month: number, offset: number) {
  const zeroBased = year * 12 + month - 1 + offset
  return {
    year: Math.floor(zeroBased / 12),
    month: ((zeroBased % 12) + 12) % 12 + 1,
  }
}

export function groupHolidayItemsByDate(items: StorefrontHolidayItem[]) {
  const grouped = new Map<string, StorefrontHolidayItem[]>()
  for (const item of items) {
    const existing = grouped.get(item.date) || []
    existing.push(item)
    grouped.set(item.date, existing)
  }
  return grouped
}

export function primaryHolidayType(items: StorefrontHolidayItem[]): StorefrontHolidayItem['holiday_type'] | null {
  if (items.some((item) => item.holiday_type === 'observed')) {
    return 'observed'
  }
  if (items.some((item) => item.holiday_type === 'federal')) {
    return 'federal'
  }
  if (items.some((item) => item.holiday_type === 'special')) {
    return 'special'
  }
  return null
}

function calendarDate(year: number, month: number, day: number) {
  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`
}
