import apiClient from '@/api/client'

export interface ActivityBucket { success: number; failed: number; recovered: number }
export interface AccountActivity extends ActivityBucket {
  account_id: number
  rate_limited: number
  cost: number
  avg_duration_ms: number | null
  avg_first_token_ms: number | null
  last_used_at: string | null
  trend: ActivityBucket[]
}
export interface ActivityRequest {
  id: string
  kind: 'success' | 'failed' | 'recovered'
  created_at: string
  model: string
  status_code: number | null
  duration_ms: number | null
  first_token_ms: number | null
  cost: number
  message: string
}
export interface ActivityResponse {
  start_time: string
  end_time: string
  monitoring_enabled: boolean
  accounts: Record<string, AccountActivity>
  recent: ActivityRequest[]
}
export async function getAccountActivity(ids: number[], hours: 1 | 24, recent = false, signal?: AbortSignal) {
  const { data } = await apiClient.post<ActivityResponse>('/admin/accounts/activity/batch', {
    account_ids: ids, hours, include_recent: recent
  }, { signal })
  return data
}
export const activityLatency = (ms: number | null | undefined) => ms == null ? '—' : ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`
export function activityRate(stats: AccountActivity | undefined, enabled = true) {
  if (!stats || !enabled || stats.success + stats.failed === 0) return '—'
  const rate = stats.success / (stats.success + stats.failed) * 100
  // Never round a nonzero failure count into a misleading 100%.
  return `${Math.floor(rate * 10) / 10}%`
}
