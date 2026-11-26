<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DEFAULT_HOLIDAY_COUNTRY,
  HOLIDAY_BUSINESS_TIMEZONE,
  getStorefrontHolidays,
  type StorefrontHolidayCalendar,
  type StorefrontHolidayItem,
} from '../../api/holidays'
import {
  buildHolidayCalendarDays,
  groupHolidayItemsByDate,
  primaryHolidayType,
  shiftHolidayMonth,
  type HolidayCalendarDay,
} from '../../utils/holidayCalendar'

const { locale, t } = useI18n()
const calendar = ref<StorefrontHolidayCalendar | null>(null)
const loading = ref(true)
const error = ref(false)
const selectedDate = ref<string | null>(null)
let loadSequence = 0

const activeLocale = computed<'en' | 'zh'>(() => (locale.value === 'zh' ? 'zh' : 'en'))
const calendarCells = computed(() =>
  calendar.value ? buildHolidayCalendarDays(calendar.value.year, calendar.value.month) : [],
)
const holidaysByDate = computed(() => groupHolidayItemsByDate(calendar.value?.items || []))
const selectedItems = computed(() =>
  selectedDate.value ? holidaysByDate.value.get(selectedDate.value) || [] : [],
)
const monthTitle = computed(() => {
  if (!calendar.value) return ''
  return new Intl.DateTimeFormat(activeLocale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: 'long',
    year: 'numeric',
    timeZone: HOLIDAY_BUSINESS_TIMEZONE,
  }).format(new Date(Date.UTC(calendar.value.year, calendar.value.month - 1, 1)))
})

onMounted(() => {
  void loadCalendar()
})

watch(activeLocale, () => {
  void loadCalendar(calendar.value ? { year: calendar.value.year, month: calendar.value.month } : undefined)
})

async function loadCalendar(target?: { year: number; month: number }) {
  const sequence = ++loadSequence
  loading.value = true
  error.value = false
  selectedDate.value = null
  try {
    const result = await getStorefrontHolidays({
      country: DEFAULT_HOLIDAY_COUNTRY,
      locale: activeLocale.value,
      ...(target || {}),
    })
    if (sequence !== loadSequence) return
    calendar.value = result
  } catch {
    if (sequence !== loadSequence) return
    error.value = true
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

function navigateMonth(offset: number) {
  if (!calendar.value || loading.value) return
  const target = shiftHolidayMonth(calendar.value.year, calendar.value.month, offset)
  void loadCalendar(target)
}

function retry() {
  void loadCalendar(calendar.value ? { year: calendar.value.year, month: calendar.value.month } : undefined)
}

function itemsForDate(date: string) {
  return holidaysByDate.value.get(date) || []
}

function visualType(day: HolidayCalendarDay) {
  const items = itemsForDate(day.date)
  if (items.length > 0 && items.every((item) => item.day_type === 'working_day_override')) {
    return 'working_day_override'
  }
  return primaryHolidayType(items.filter((item) => item.day_type === 'public_holiday'))
}

function selectDate(day: HolidayCalendarDay) {
  if (itemsForDate(day.date).length === 0) return
  selectedDate.value = selectedDate.value === day.date ? null : day.date
}

function isToday(date: string) {
  return calendar.value?.today === date
}

function hasObservedHoliday(date: string) {
  return itemsForDate(date).some(
    (item) => item.holiday_type === 'observed' && item.day_type === 'public_holiday',
  )
}

function itemTypeLabel(item: StorefrontHolidayItem) {
  if (item.day_type === 'working_day_override') return t('holidayCalendar.workingDayOverride')
  return t(`holidayCalendar.types.${item.holiday_type}`)
}

function formatCalendarDate(value: string, options: Intl.DateTimeFormatOptions = {}) {
  const [year, month, day] = value.split('-').map(Number)
  if (!year || !month || !day) return value
  return new Intl.DateTimeFormat(activeLocale.value === 'zh' ? 'zh-CN' : 'en-US', {
    timeZone: HOLIDAY_BUSINESS_TIMEZONE,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    ...options,
  }).format(new Date(Date.UTC(year, month - 1, day)))
}
</script>

<template>
  <section
    class="overflow-hidden rounded-xl border border-slate-100 bg-white shadow-sm"
    :aria-label="t('holidayCalendar.title')"
  >
    <div v-if="loading && !calendar" class="animate-pulse p-4" data-testid="holiday-calendar-loading">
      <div class="h-5 w-36 rounded bg-slate-200" />
      <div class="mt-2 h-3 w-28 rounded bg-slate-100" />
      <div class="mt-4 grid grid-cols-7 gap-1">
        <div v-for="index in 35" :key="index" class="h-8 rounded-md bg-slate-100" />
      </div>
    </div>

    <div v-else-if="error" class="flex min-h-64 flex-col items-center justify-center px-5 py-8 text-center" data-testid="holiday-calendar-error">
      <span class="text-2xl" aria-hidden="true">📅</span>
      <p class="mt-3 text-sm font-bold text-slate-700">{{ t('holidayCalendar.loadError') }}</p>
      <button
        type="button"
        class="mt-3 rounded-lg bg-orange-500 px-4 py-2 text-xs font-black text-white hover:bg-orange-600"
        @click="retry"
      >
        {{ t('holidayCalendar.retry') }}
      </button>
    </div>

    <template v-else-if="calendar">
      <header class="flex items-start justify-between gap-3 px-4 pb-2 pt-3.5">
        <div class="min-w-0">
          <h2 class="truncate text-base font-black text-slate-950">{{ monthTitle }}</h2>
          <p class="mt-0.5 text-[11px] font-bold uppercase tracking-wide text-red-600">
            {{ t('holidayCalendar.title') }}
          </p>
        </div>
        <div class="flex shrink-0 gap-1">
          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-600 hover:border-orange-300 hover:bg-orange-50 hover:text-orange-700 disabled:cursor-wait disabled:opacity-50"
            :aria-label="t('holidayCalendar.previousMonth')"
            :disabled="loading"
            @click="navigateMonth(-1)"
          >
            <svg viewBox="0 0 20 20" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="m12.5 15-5-5 5-5" />
            </svg>
          </button>
          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-600 hover:border-orange-300 hover:bg-orange-50 hover:text-orange-700 disabled:cursor-wait disabled:opacity-50"
            :aria-label="t('holidayCalendar.nextMonth')"
            :disabled="loading"
            @click="navigateMonth(1)"
          >
            <svg viewBox="0 0 20 20" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="m7.5 5 5 5-5 5" />
            </svg>
          </button>
        </div>
      </header>

      <div class="mx-3 flex items-center justify-between rounded-lg bg-orange-50 px-3 py-2">
        <div>
          <span class="text-[10px] font-black uppercase tracking-wider text-orange-700">{{ t('holidayCalendar.today') }}</span>
          <p class="text-xs font-bold text-slate-800">{{ formatCalendarDate(calendar.today, { month: 'long' }) }}</p>
        </div>
        <span class="rounded-full bg-white px-2 py-1 text-[10px] font-bold text-slate-500 shadow-sm">
          {{ t('holidayCalendar.accraTime') }}
        </span>
      </div>

      <div class="px-3 pb-3 pt-2.5" :class="{ 'opacity-60': loading }">
        <div class="grid grid-cols-7 text-center">
          <span
            v-for="weekday in ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat']"
            :key="weekday"
            class="pb-1 text-[9px] font-black uppercase tracking-wide text-slate-400"
          >
            {{ t(`holidayCalendar.weekdays.${weekday}`) }}
          </span>
        </div>

        <div class="grid grid-cols-7 gap-1" data-testid="holiday-calendar-grid">
          <template v-for="(day, index) in calendarCells" :key="day?.key || `blank-${index}`">
            <span v-if="!day" class="h-8 sm:h-9" aria-hidden="true" />
            <button
              v-else
              type="button"
              class="relative flex h-8 min-w-0 flex-col items-center justify-center rounded-md text-xs font-bold transition sm:h-9"
              :class="{
                'cursor-default text-slate-800': !day.weekend && !visualType(day),
                'cursor-default bg-rose-50/50 text-slate-400': day.weekend && !visualType(day),
                'bg-red-50 text-red-700 hover:bg-red-100': visualType(day) === 'federal',
                'bg-orange-50 text-orange-700 hover:bg-orange-100': visualType(day) === 'observed',
                'bg-purple-50 text-purple-700 hover:bg-purple-100': visualType(day) === 'special',
                'bg-emerald-50 text-emerald-700 hover:bg-emerald-100': visualType(day) === 'working_day_override',
                'ring-2 ring-orange-400 ring-offset-1': selectedDate === day.date,
              }"
              :aria-label="day.date"
              :aria-pressed="selectedDate === day.date"
              @click="selectDate(day)"
            >
              <span
                class="flex h-5 w-5 items-center justify-center rounded-full"
                :class="{ 'bg-orange-500 font-black text-white shadow-sm': isToday(day.date) }"
              >
                {{ day.day }}
              </span>
              <span
                v-if="visualType(day) && !isToday(day.date)"
                class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full"
                :class="{
                  'bg-red-500': visualType(day) === 'federal',
                  'bg-orange-500': visualType(day) === 'observed',
                  'bg-purple-500': visualType(day) === 'special',
                  'bg-emerald-500': visualType(day) === 'working_day_override',
                }"
                aria-hidden="true"
              />
              <span
                v-if="hasObservedHoliday(day.date)"
                class="absolute inset-x-0 bottom-0 truncate px-0.5 text-[7px] font-black uppercase leading-none text-orange-700"
              >
                {{ t('holidayCalendar.observedShort') }}
              </span>
            </button>
          </template>
        </div>

        <p v-if="calendar.items.length === 0" class="mt-2 text-center text-[11px] font-semibold text-slate-400" data-testid="holiday-calendar-empty">
          {{ t('holidayCalendar.emptyMonth') }}
        </p>

        <div
          v-if="selectedItems.length > 0"
          class="mt-2 rounded-lg border border-slate-200 bg-white p-3 shadow-lg"
          role="dialog"
          aria-live="polite"
        >
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-black text-slate-900">{{ formatCalendarDate(selectedDate || '') }}</p>
            <button type="button" class="text-lg leading-none text-slate-400 hover:text-slate-700" :aria-label="t('holidayCalendar.close')" @click="selectedDate = null">×</button>
          </div>
          <div
            v-for="(item, index) in selectedItems"
            :key="`${item.date}-${item.name_en}-${index}`"
            class="mt-2 border-t border-slate-100 pt-2 first:mt-1 first:border-0 first:pt-0"
          >
            <span
              class="inline-flex rounded-full px-2 py-0.5 text-[9px] font-black uppercase tracking-wide"
              :class="{
                'bg-red-50 text-red-700': item.holiday_type === 'federal' && item.day_type === 'public_holiday',
                'bg-orange-50 text-orange-700': item.holiday_type === 'observed' && item.day_type === 'public_holiday',
                'bg-purple-50 text-purple-700': item.holiday_type === 'special' && item.day_type === 'public_holiday',
                'bg-emerald-50 text-emerald-700': item.day_type === 'working_day_override',
              }"
            >
              {{ itemTypeLabel(item) }}
            </span>
            <p class="mt-1.5 text-xs font-black text-slate-900">{{ item.name_en }}</p>
            <p class="mt-0.5 text-xs font-semibold text-slate-500">{{ item.name_zh || t('holidayCalendar.noChineseName') }}</p>
            <p v-if="item.observed_for_date" class="mt-1 text-[10px] font-bold text-orange-700">
              {{ t('holidayCalendar.observedFor', { date: formatCalendarDate(item.observed_for_date) }) }}
            </p>
          </div>
        </div>
      </div>

      <footer class="border-t border-slate-100 bg-slate-50 px-4 py-2.5">
        <p class="text-[9px] font-black uppercase tracking-wider text-slate-400">
          {{ t('holidayCalendar.nextHoliday') }}
        </p>
        <div v-if="calendar.next_holiday" class="mt-0.5 flex items-center justify-between gap-3">
          <p class="min-w-0 truncate text-xs font-black text-slate-800">{{ calendar.next_holiday.name }}</p>
          <time
            class="shrink-0 text-[10px] font-bold"
            :class="{
              'text-red-600': calendar.next_holiday.holiday_type === 'federal',
              'text-orange-600': calendar.next_holiday.holiday_type === 'observed',
              'text-purple-600': calendar.next_holiday.holiday_type === 'special',
            }"
            :datetime="calendar.next_holiday.date"
          >
            {{ formatCalendarDate(calendar.next_holiday.date, { year: undefined }) }}
          </time>
        </div>
        <p v-else class="mt-0.5 text-[11px] font-semibold text-slate-500">
          {{ t('holidayCalendar.noUpcoming') }}
        </p>
      </footer>
    </template>
  </section>
</template>
