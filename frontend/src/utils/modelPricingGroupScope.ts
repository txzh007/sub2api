import type { ModelPricingCatalogEntry } from '@/api/admin/channels'

export type ModelPricingScope = 'active-groups' | 'catalog' | 'unused' | `group:${number}`

export interface PricingGroupSource {
  id: number
  name: string
  platform: string
  model_allowlist?: {
    enabled: boolean
    models: string[]
  }
  model_pricing?: Array<{
    models: string[]
  }>
}

export interface PricingGroupModels {
  group: PricingGroupSource
  candidates: string[]
}

export interface ScopedModelPricingEntry extends ModelPricingCatalogEntry {
  group_ids: number[]
  group_names: string[]
  synthetic?: boolean
  inherited_from?: string
}

interface GroupModelReference {
  model: string
  groups: PricingGroupSource[]
}

function normalized(value: string): string {
  return value.trim().toLowerCase()
}

export function matchesModelPattern(model: string, pattern: string): boolean {
  const normalizedModel = normalized(model)
  const normalizedPattern = normalized(pattern)
  if (!normalizedModel || !normalizedPattern) return false
  if (normalizedPattern === '*') return true
  if (normalizedPattern.endsWith('*')) {
    return normalizedModel.startsWith(normalizedPattern.slice(0, -1))
  }
  return normalizedModel === normalizedPattern
}

export function resolveEffectiveGroupModels(
  group: PricingGroupSource,
  candidates: string[]
): string[] {
  const candidateMap = new Map<string, string>()
  for (const model of candidates) {
    const key = normalized(model)
    if (key && !candidateMap.has(key)) candidateMap.set(key, model.trim())
  }

  const allowlist = group.model_allowlist
  const resolved = new Map<string, string>()
  for (const [key, model] of candidateMap) {
    resolved.set(key, model)
  }

  // A manually configured per-group price can refer to a model that is not in
  // the platform catalog yet. Keep exact names and expand wildcard patterns
  // against the candidates so newly integrated providers are not hidden.
  for (const pricing of group.model_pricing || []) {
    for (const pattern of pricing.models || []) {
      const cleanPattern = pattern.trim()
      if (!cleanPattern) continue
      if (cleanPattern.includes('*')) {
        for (const [key, model] of candidateMap) {
          if (matchesModelPattern(model, cleanPattern)) resolved.set(key, model)
        }
      } else {
        resolved.set(normalized(cleanPattern), cleanPattern)
      }
    }
  }

  return [...resolved.values()]
    .filter(model => !allowlist?.enabled || allowlist.models.some(pattern => matchesModelPattern(model, pattern)))
    .sort((a, b) => a.localeCompare(b))
}

function buildGroupModelReferences(groups: PricingGroupModels[]): GroupModelReference[] {
  const references = new Map<string, GroupModelReference>()
  for (const { group, candidates } of groups) {
    for (const model of resolveEffectiveGroupModels(group, candidates)) {
      const key = normalized(model)
      const existing = references.get(key)
      if (existing) {
        if (!existing.groups.some(item => item.id === group.id)) existing.groups.push(group)
      } else {
        references.set(key, { model, groups: [group] })
      }
    }
  }
  return [...references.values()].sort((a, b) => a.model.localeCompare(b.model))
}

function emptyCatalogEntry(model: string, provider: string): ModelPricingCatalogEntry {
  return {
    model,
    litellm_provider: provider,
    mode: 'chat',
    deprecated: false,
    input_cost_per_token: 0,
    input_cost_per_token_priority: 0,
    output_cost_per_token: 0,
    output_cost_per_token_priority: 0,
    cache_creation_input_token_cost: 0,
    cache_creation_input_token_cost_priority: 0,
    cache_creation_input_token_cost_above_1hr: 0,
    cache_read_input_token_cost: 0,
    cache_read_input_token_cost_priority: 0,
    output_cost_per_image: 0,
    output_cost_per_video: 0,
    output_cost_per_image_token: 0,
    input_cost_per_image_token: 0,
    long_context_input_token_threshold: 0,
    long_context_input_cost_multiplier: 0,
    long_context_output_cost_multiplier: 0,
    supports_prompt_caching: false,
    supports_service_tier: false,
    token_pricing_absent: true,
    overridden: false,
    wildcard: false
  }
}

function groupMetadata(groups: PricingGroupSource[]) {
  return {
    group_ids: groups.map(group => group.id),
    group_names: groups.map(group => group.name)
  }
}

export function materializeGroupPricingEntries(
  catalogItems: ModelPricingCatalogEntry[],
  groups: PricingGroupModels[]
): ScopedModelPricingEntry[] {
  const exactEntries = new Map<string, ModelPricingCatalogEntry>()
  const wildcardEntries: ModelPricingCatalogEntry[] = []
  for (const item of catalogItems) {
    if (item.wildcard || item.model.endsWith('*')) wildcardEntries.push(item)
    else if (!exactEntries.has(normalized(item.model))) exactEntries.set(normalized(item.model), item)
  }
  wildcardEntries.sort((a, b) => b.model.length - a.model.length)

  return buildGroupModelReferences(groups).map(({ model, groups: modelGroups }) => {
    const metadata = groupMetadata(modelGroups)
    const exact = exactEntries.get(normalized(model))
    if (exact) return { ...exact, ...metadata }

    const inherited = wildcardEntries.find(item => matchesModelPattern(model, item.model))
    if (inherited) {
      return {
        ...inherited,
        model,
        wildcard: false,
        overridden: false,
        override: undefined,
        deprecated: false,
        deprecation_date: undefined,
        token_pricing_absent: false,
        ...metadata,
        synthetic: true,
        inherited_from: inherited.model
      }
    }

    return {
      ...emptyCatalogEntry(model, modelGroups[0]?.platform || ''),
      ...metadata,
      synthetic: true
    }
  })
}

export function filterPricingEntriesByScope(
  catalogItems: ModelPricingCatalogEntry[],
  groupEntries: ScopedModelPricingEntry[],
  scope: ModelPricingScope
): ScopedModelPricingEntry[] {
  if (scope === 'active-groups') return groupEntries
  if (scope.startsWith('group:')) {
    const groupID = Number(scope.slice('group:'.length))
    return groupEntries.filter(item => item.group_ids.includes(groupID))
  }

  const usedModels = groupEntries.map(item => item.model)
  const catalogWithMetadata = catalogItems.map(item => ({
    ...item,
    group_ids: [] as number[],
    group_names: [] as string[]
  }))
  if (scope === 'unused') {
    return catalogWithMetadata.filter(item => !usedModels.some(model => matchesModelPattern(model, item.model)))
  }
  return catalogWithMetadata
}
