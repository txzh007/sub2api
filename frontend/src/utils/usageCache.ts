interface CacheTokenUsage {
  input_tokens?: number | null
  cache_creation_tokens?: number | null
  cache_read_tokens?: number | null
}

export function formatCacheHitRate(usage: CacheTokenUsage): string {
  const cacheReadTokens = usage.cache_read_tokens ?? 0
  // Usage logs store uncached input, cache creation and cache reads separately.
  const totalInputTokens = (usage.input_tokens ?? 0)
    + (usage.cache_creation_tokens ?? 0)
    + cacheReadTokens

  return totalInputTokens > 0
    ? `${((cacheReadTokens / totalInputTokens) * 100).toFixed(1)}%`
    : '-'
}
