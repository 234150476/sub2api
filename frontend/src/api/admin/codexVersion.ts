/**
 * Codex CLI 指纹版本管理 API
 * 对应后端 backend/internal/handler/admin/codex_version_handler.go
 */

import { apiClient } from '../client'

export interface CodexVersionStatus {
  version: string
  user_agent: string
  source: 'github' | 'cache' | 'fallback' | 'hardcoded'
  fetched_at: string
  auto_update: boolean
  refresh_interval: string
  fallback_version: string
  user_agent_template: string
}

/**
 * 获取当前 Codex CLI 指纹状态
 */
export async function getCodexVersionStatus(): Promise<CodexVersionStatus> {
  const { data } = await apiClient.get<CodexVersionStatus>('/admin/codex-version')
  return data
}

export interface CodexVersionRefreshResult {
  version: string
  status: CodexVersionStatus
}

/**
 * 强制从 GitHub 拉取一次最新 Codex CLI 版本
 * 失败时后端返回 502；前端应捕获并提示用户当前版本不变
 */
export async function refreshCodexVersion(): Promise<CodexVersionRefreshResult> {
  const { data } = await apiClient.post<CodexVersionRefreshResult>('/admin/codex-version/refresh')
  return data
}

export const codexVersionAPI = {
  getStatus: getCodexVersionStatus,
  refresh: refreshCodexVersion
}

export default codexVersionAPI
