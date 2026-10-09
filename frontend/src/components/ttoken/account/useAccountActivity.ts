import { ref, watch, onMounted, onUnmounted, type Ref } from 'vue'
import { getAccountActivity, type AccountActivity } from '@/api/ttoken/accountActivity'

export function useAccountActivity(accounts: Ref<{ id: number }[]>) {
  const hours = ref<1 | 24>(1)
  const stats = ref<Record<string, AccountActivity>>({})
  const loading = ref(false)
  const error = ref(false)
  const monitoring = ref(true)
  const updatedAt = ref('')
  const autoRefresh = ref(true)
  let controller: AbortController | undefined
  let generation = 0
  let timer: ReturnType<typeof setInterval> | undefined

  async function refresh() {
    const token = ++generation
    controller?.abort()
    controller = new AbortController()
    const ids = accounts.value.map(a => a.id)
    if (!ids.length) { stats.value = {}; loading.value = false; return }
    loading.value = true
    error.value = false
    try {
      const result: Record<string, AccountActivity> = {}
      let enabled = true
      let time = ''
      for (let i = 0; i < ids.length; i += 100) {
        const data = await getAccountActivity(ids.slice(i, i + 100), hours.value, false, controller.signal)
        if (token !== generation) return
        Object.assign(result, data.accounts)
        enabled = data.monitoring_enabled
        time = data.end_time
      }
      stats.value = result
      monitoring.value = enabled
      updatedAt.value = time
    } catch {
      if (token === generation) error.value = true
    } finally { if (token === generation) loading.value = false }
  }
  watch([() => accounts.value.map(a => a.id).join(','), hours], () => {
    stats.value = {}; updatedAt.value = ''; void refresh()
  }, { immediate: true })
  onMounted(() => {
    timer = setInterval(() => {
      if (autoRefresh.value && !loading.value && document.visibilityState === 'visible') void refresh()
    }, 30_000)
  })
  onUnmounted(() => { generation++; controller?.abort(); clearInterval(timer) })
  return { hours, stats, loading, error, monitoring, updatedAt, autoRefresh, refresh }
}
