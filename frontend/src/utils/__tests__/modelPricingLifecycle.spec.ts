import { describe, expect, it } from 'vitest'
import { matchesModelPricingLifecycle } from '@/utils/modelPricingLifecycle'

describe('matchesModelPricingLifecycle', () => {
  const active = { deprecated: false }
  const deprecated = { deprecated: true }

  it('defaults the pricing workflow to current active models', () => {
    expect(matchesModelPricingLifecycle(active, 'active')).toBe(true)
    expect(matchesModelPricingLifecycle(deprecated, 'active')).toBe(false)
  })

  it('can show all models or only deprecated models', () => {
    expect(matchesModelPricingLifecycle(active, 'all')).toBe(true)
    expect(matchesModelPricingLifecycle(deprecated, 'all')).toBe(true)
    expect(matchesModelPricingLifecycle(active, 'deprecated')).toBe(false)
    expect(matchesModelPricingLifecycle(deprecated, 'deprecated')).toBe(true)
  })
})
