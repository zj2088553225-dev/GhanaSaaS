import request, { type ApiResponse } from '../utils/request'

export const DEFAULT_HOLIDAY_COUNTRY = 'US'
export const HOLIDAY_BUSINESS_TIMEZONE = 'Africa/Accra'

export type HolidayType = 'federal' | 'observed' | 'special'
export type HolidayDayType = 'public_holiday' | 'working_day_override'

export interface StorefrontHolidayItem {
  date: string
  name: string
  name_en: string
  name_zh: string | null
  holiday_type: HolidayType
  day_type: HolidayDayType
  observed_for_date: string | null
  is_official: boolean
}

export interface StorefrontHolidayCalendar {
  country_code: string
  country_name: string
  timezone: string
  year: number
  month: number
  today: string
  items: StorefrontHolidayItem[]
  next_holiday: StorefrontHolidayItem | null
}

export interface StorefrontHolidayQuery {
  country?: string
  year?: number
  month?: number
  locale?: 'en' | 'zh'
}

export async function getStorefrontHolidays(
  params: StorefrontHolidayQuery = {},
): Promise<StorefrontHolidayCalendar> {
  const response = await request.get<StorefrontHolidayCalendar | ApiResponse<StorefrontHolidayCalendar>>(
    '/storefront/holidays',
    {
      params: {
        ...params,
        country: params.country || DEFAULT_HOLIDAY_COUNTRY,
      },
    },
  )
  const body = response.data
  const calendar = 'country_code' in body ? body : body.data
  if (!calendar) {
    throw new Error('Holiday calendar data is unavailable')
  }
  return calendar
}
