import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import {
  buildHolidayCalendarDays,
  groupHolidayItemsByDate,
  primaryHolidayType,
  shiftHolidayMonth,
} from '../src/utils/holidayCalendar.ts'

const cells = buildHolidayCalendarDays(2026, 7)
const julyFirst = cells.find((cell) => cell?.date === '2026-07-01')
const julyFourth = cells.find((cell) => cell?.date === '2026-07-04')
assert.equal(cells.indexOf(julyFirst), 3)
assert.equal(julyFourth?.weekend, true)
assert.deepEqual(shiftHolidayMonth(2026, 12, 1), { year: 2027, month: 1 })
assert.deepEqual(shiftHolidayMonth(2026, 1, -1), { year: 2025, month: 12 })

const federal = {
  date: '2026-07-04',
  name: 'Independence Day',
  name_en: 'Independence Day',
  name_zh: '美国独立日',
  holiday_type: 'federal',
  day_type: 'public_holiday',
  observed_for_date: null,
  is_official: true,
}
const observed = {
  ...federal,
  holiday_type: 'observed',
  observed_for_date: '2026-07-04',
}
const grouped = groupHolidayItemsByDate([federal, observed])
assert.equal(grouped.get('2026-07-04')?.length, 2)
assert.equal(primaryHolidayType(grouped.get('2026-07-04') || []), 'observed')

const [component, api, home, en, zh] = await Promise.all([
  readFile(new URL('../src/components/storefront/StorefrontHolidayCalendar.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/api/holidays.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/PlatformHome.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/locales/en.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/locales/zh.ts', import.meta.url), 'utf8'),
])

assert.match(api, /DEFAULT_HOLIDAY_COUNTRY = 'US'/)
assert.match(api, /HOLIDAY_BUSINESS_TIMEZONE = 'Africa\/Accra'/)
assert.match(component, /calendar\.today/)
assert.doesNotMatch(component, /new Date\(\)/)
assert.match(component, /holiday_type === 'federal'/)
assert.match(component, /holiday_type === 'observed'/)
assert.match(component, /observed_for_date/)
assert.match(component, /navigateMonth\(-1\)/)
assert.match(component, /navigateMonth\(1\)/)
assert.match(component, /holiday-calendar-loading/)
assert.match(component, /holiday-calendar-error/)
assert.match(component, /holiday-calendar-empty/)
assert.match(home, /StorefrontHolidayCalendar/)
assert.match(en, /U\.S\. Federal Holidays/)
assert.match(en, /Observed Holiday/)
assert.match(zh, /美国联邦节假日/)
assert.match(zh, /顺延假日/)

console.log('storefront holiday calendar tests passed')
