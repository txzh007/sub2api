export type ModelPricingLifecycleFilter = 'active' | 'all' | 'deprecated'

export interface ModelPricingLifecycleEntry {
  deprecated: boolean
}

export function matchesModelPricingLifecycle(
  item: ModelPricingLifecycleEntry,
  filter: ModelPricingLifecycleFilter
): boolean {
  if (filter === 'active') return !item.deprecated
  if (filter === 'deprecated') return item.deprecated
  return true
}
