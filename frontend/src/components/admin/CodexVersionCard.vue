<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.codexVersion.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.codexVersion.description') }}
      </p>
    </div>

    <div class="p-6 space-y-4">
      <!-- 状态网格 -->
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.codexVersion.currentVersion') }}
          </div>
          <div class="mt-1 font-mono text-base text-gray-900 dark:text-white">
            <span v-if="loading" class="text-gray-400">--</span>
            <span v-else>{{ status?.version || '--' }}</span>
          </div>
        </div>

        <div>
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.codexVersion.source') }}
          </div>
          <div class="mt-1 flex items-center gap-2">
            <span
              v-if="!loading && status"
              class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
              :class="sourceBadgeClass"
            >
              <span class="h-1.5 w-1.5 rounded-full" :class="sourceDotClass"></span>
              {{ sourceLabel }}
            </span>
            <span v-else class="text-sm text-gray-400">--</span>
          </div>
        </div>

        <div>
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.codexVersion.fetchedAt') }}
          </div>
          <div class="mt-1 text-sm text-gray-700 dark:text-gray-300">
            <span v-if="loading">--</span>
            <span v-else-if="!status?.fetched_at || isZeroTime(status.fetched_at)" class="text-gray-400">
              {{ t('admin.settings.codexVersion.neverFetched') }}
            </span>
            <span v-else>{{ formatTime(status.fetched_at) }}</span>
          </div>
        </div>

        <div>
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.codexVersion.refreshInterval') }}
          </div>
          <div class="mt-1 text-sm text-gray-700 dark:text-gray-300">
            <span v-if="loading">--</span>
            <span v-else-if="!status?.auto_update" class="text-gray-400">
              {{ t('admin.settings.codexVersion.autoUpdateOff') }}
            </span>
            <span v-else>{{ status?.refresh_interval || '--' }}</span>
          </div>
        </div>
      </div>

      <!-- User-Agent 全文 -->
      <div>
        <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.codexVersion.userAgent') }}
        </div>
        <div
          class="mt-1 break-all rounded-md bg-gray-50 px-3 py-2 font-mono text-xs text-gray-800 dark:bg-dark-800 dark:text-gray-200"
        >
          <span v-if="loading">--</span>
          <span v-else>{{ status?.user_agent || '--' }}</span>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          <span v-if="status && !status.auto_update">
            {{ t('admin.settings.codexVersion.autoUpdateOffHint') }}
          </span>
          <span v-else>
            {{ t('admin.settings.codexVersion.autoUpdateOnHint') }}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loading || refreshing"
            @click="loadStatus(true)"
          >
            {{ t('admin.settings.codexVersion.reload') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="loading || refreshing"
            @click="onRefresh"
          >
            <span v-if="refreshing">{{ t('admin.settings.codexVersion.refreshing') }}</span>
            <span v-else>{{ t('admin.settings.codexVersion.refreshNow') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { codexVersionAPI, type CodexVersionStatus } from '@/api/admin/codexVersion'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const status = ref<CodexVersionStatus | null>(null)
const loading = ref(false)
const refreshing = ref(false)

const sourceLabel = computed(() => {
  if (!status.value) return ''
  return t(`admin.settings.codexVersion.sources.${status.value.source}`)
})

const sourceBadgeClass = computed(() => {
  switch (status.value?.source) {
    case 'github':
      return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'cache':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'fallback':
      return 'bg-yellow-50 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300'
    case 'hardcoded':
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
})

const sourceDotClass = computed(() => {
  switch (status.value?.source) {
    case 'github':
      return 'bg-green-500'
    case 'cache':
      return 'bg-blue-500'
    case 'fallback':
      return 'bg-yellow-500'
    case 'hardcoded':
    default:
      return 'bg-gray-400'
  }
})

function isZeroTime(iso: string): boolean {
  if (!iso) return true
  // Go 的 time.Time{} 零值序列化为 "0001-01-01T00:00:00Z"
  return iso.startsWith('0001-01-01')
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

async function loadStatus(showSuccess = false) {
  loading.value = true
  try {
    status.value = await codexVersionAPI.getStatus()
    if (showSuccess) {
      appStore.showSuccess(t('admin.settings.codexVersion.reloadSuccess'))
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

async function onRefresh() {
  refreshing.value = true
  try {
    const result = await codexVersionAPI.refresh()
    status.value = result.status
    appStore.showSuccess(
      t('admin.settings.codexVersion.refreshSuccess', { version: result.version }),
    )
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.settings.codexVersion.refreshFailed')))
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  loadStatus()
})
</script>
