<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ReservationHeatLevel } from '../../api/reservations'
import { reservationHeatTranslationKey } from '../../utils/reservationUi'

const props = defineProps<{ level: ReservationHeatLevel }>()
const { t } = useI18n()
const label = computed(() => t(reservationHeatTranslationKey(props.level)))
const classes = computed(() => ({
  new: 'bg-sky-50 text-sky-700 ring-sky-200',
  popular: 'bg-orange-50 text-orange-700 ring-orange-200',
  trending: 'bg-rose-50 text-rose-700 ring-rose-200',
  highly_requested: 'bg-violet-50 text-violet-700 ring-violet-200',
}[props.level]))
</script>

<template>
  <span class="inline-flex rounded-full px-3 py-1 text-xs font-black ring-1 ring-inset" :class="classes">{{ label }}</span>
</template>
