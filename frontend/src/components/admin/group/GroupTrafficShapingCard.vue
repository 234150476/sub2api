<template>
  <div
    class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/40 dark:bg-amber-900/10"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="flex-1">
        <div class="flex items-center gap-2">
          <span class="text-base">⚡</span>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.groups.trafficShaping.title') }}
          </h3>
          <span
            class="rounded bg-amber-200 px-1.5 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-200"
          >
            {{ t('admin.groups.trafficShaping.advancedTag') }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
          {{ t('admin.groups.trafficShaping.description') }}
        </p>
      </div>
      <button
        type="button"
        @click="onToggleEnabled"
        :class="[
          'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors',
          modelValue.traffic_shaping_enabled ? 'bg-amber-500' : 'bg-gray-300 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
            modelValue.traffic_shaping_enabled ? 'translate-x-6' : 'translate-x-1'
          ]"
        />
      </button>
    </div>

    <div
      v-if="modelValue.traffic_shaping_enabled"
      class="mt-4 space-y-3 border-t border-amber-200/60 pt-3 dark:border-amber-900/30"
    >
      <SubSwitch
        :model-value="modelValue.sse_early_keepalive_enabled"
        @update:model-value="modelValue.sse_early_keepalive_enabled = $event"
        :label="t('admin.groups.trafficShaping.sseKeepalive.label')"
        :hint="t('admin.groups.trafficShaping.sseKeepalive.hint')"
      />
      <SubSwitch
        :model-value="modelValue.stream_smoothing_enabled"
        @update:model-value="modelValue.stream_smoothing_enabled = $event"
        :label="t('admin.groups.trafficShaping.streamSmoothing.label')"
        :hint="t('admin.groups.trafficShaping.streamSmoothing.hint')"
        :coming-soon="true"
      />
      <SubSwitch
        :model-value="modelValue.silent_retry_5xx_enabled"
        @update:model-value="modelValue.silent_retry_5xx_enabled = $event"
        :label="t('admin.groups.trafficShaping.silentRetry5xx.label')"
        :hint="t('admin.groups.trafficShaping.silentRetry5xx.hint')"
        :auto-on="true"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import SubSwitch from './GroupTrafficShapingSubSwitch.vue'

// 直接修改 props.modelValue 的字段。
// 父组件用 v-model="editForm" 绑定 reactive 对象 —— reactive 容器不能整体替换，
// 但其字段可以直接修改并被 Vue 响应式系统捕获。这是 Vue 3 reactive 对象作为 prop
// 的标准用法（不违反单向数据流，因为字段所有权仍在父组件）。
const { modelValue } = defineProps<{
  modelValue: {
    traffic_shaping_enabled: boolean
    sse_early_keepalive_enabled: boolean
    stream_smoothing_enabled: boolean
    silent_retry_5xx_enabled: boolean
  }
}>()

const { t } = useI18n()

function onToggleEnabled() {
  modelValue.traffic_shaping_enabled = !modelValue.traffic_shaping_enabled
}
</script>
