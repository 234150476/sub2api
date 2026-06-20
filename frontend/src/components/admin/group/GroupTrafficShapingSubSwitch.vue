<template>
  <div class="flex items-start justify-between gap-3">
    <div class="flex-1">
      <div class="flex items-center gap-2">
        <span class="text-sm font-medium text-gray-900 dark:text-white">{{ label }}</span>
        <span
          v-if="comingSoon"
          class="rounded bg-gray-200 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
        >
          {{ t('admin.groups.trafficShaping.comingSoonTag') }}
        </span>
        <span
          v-if="autoOn"
          class="rounded bg-green-100 px-1.5 py-0.5 text-xs text-green-700 dark:bg-green-900/30 dark:text-green-300"
        >
          {{ t('admin.groups.trafficShaping.autoOnTag') }}
        </span>
      </div>
      <p v-if="hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ hint }}
      </p>
    </div>
    <button
      type="button"
      :disabled="comingSoon"
      @click="onToggle"
      :class="[
        'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors',
        comingSoon
          ? 'cursor-not-allowed bg-gray-200 dark:bg-dark-600'
          : modelValue
            ? 'bg-amber-500'
            : 'bg-gray-300 dark:bg-dark-600'
      ]"
    >
      <span
        :class="[
          'inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform',
          modelValue ? 'translate-x-[18px]' : 'translate-x-0.5'
        ]"
      />
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelValue: boolean
  label: string
  hint?: string
  comingSoon?: boolean
  autoOn?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()

const { t } = useI18n()

function onToggle() {
  if (props.comingSoon) return
  emit('update:modelValue', !props.modelValue)
}
</script>
