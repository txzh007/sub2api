import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PelicanTestView from '../PelicanTestView.vue'

const api = vi.hoisted(() => ({
  options: vi.fn(),
  models: vi.fn(),
  listRuns: vi.fn(),
  getRun: vi.fn(),
  createRun: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin/pelicanTests', () => ({ default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

const run = (id: number) => ({
  id,
  admin_user_id: 1,
  api_key_id: 9,
  group_id: 3,
  group_name: 'Text group',
  model: 'gemini-2.5-flash',
  prompt: 'Generate an SVG of a pelican riding a bicycle',
  prompt_version: 'simon-2024-v1',
  request_id: `request-${id}`,
  usage_request_id: `client:request-${id}`,
  status: 'success',
  response_text: '<svg><circle r="10"/></svg>',
  svg: '<svg><circle r="10"/></svg>',
  error_message: '',
  duration_ms: 1200,
  input_tokens: 11,
  output_tokens: 200,
  total_cost: 0.002,
  actual_cost: 0.002,
  created_at: '2026-09-24T00:00:00Z',
  finished_at: '2026-09-24T00:00:01Z',
})

describe('PelicanTestView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.options.mockResolvedValue({
      keys: [{ id: 9, name: 'Dedicated test key', group_id: 3, group_name: 'Text group' }],
      prompt: 'Generate an SVG of a pelican riding a bicycle',
    })
    api.models.mockResolvedValue(['gemini-2.5-flash'])
    api.listRuns.mockResolvedValue({ items: [run(2), run(1)], next_before_id: 0 })
    api.getRun.mockImplementation(async (id: number) => run(id))
    api.createRun.mockResolvedValue({ id: 3, request_id: 'request-3' })
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: vi.fn(() => 'blob:pelican-test'),
      revokeObjectURL: vi.fn(),
    })
  })

  it('uses the selected key to discover models and launch a single test', async () => {
    const wrapper = mount(PelicanTestView)
    await flushPromises()
    expect(api.models).toHaveBeenCalledWith(9)
    expect(wrapper.text()).toContain('gemini-2.5-flash')

    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.createRun).toHaveBeenCalledOnce()
    expect(api.createRun).toHaveBeenCalledWith(9, 'gemini-2.5-flash')
    expect(wrapper.find('img[src="blob:pelican-test"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('loads older history through the cursor without losing comparison selections', async () => {
    api.listRuns.mockResolvedValueOnce({ items: [run(2)], next_before_id: 2 })
      .mockResolvedValueOnce({ items: [run(1)], next_before_id: 0 })
    const wrapper = mount(PelicanTestView)
    await flushPromises()
    await wrapper.find('button.w-full.rounded-xl').trigger('click')
    await flushPromises()
    await wrapper.find('button.btn-secondary.w-full').trigger('click')
    await flushPromises()
    expect(api.listRuns).toHaveBeenLastCalledWith(3, '', 2)
    expect(wrapper.text()).toContain('admin.pelicanTest.selected')
    expect(wrapper.findAll('button.w-full.rounded-xl')).toHaveLength(2)
    wrapper.unmount()
  })

  it('compares up to four results without accepting a fifth', async () => {
    api.listRuns.mockResolvedValue({ items: [run(5), run(4), run(3), run(2), run(1)], next_before_id: 0 })
    const wrapper = mount(PelicanTestView)
    await flushPromises()
    const historyButtons = wrapper.findAll('button.w-full.rounded-xl')
    for (const button of historyButtons.slice(0, 4)) {
      await button.trigger('click')
      await flushPromises()
    }
    expect(wrapper.findAll('article')).toHaveLength(4)
    expect(wrapper.findAll('div.grid').some(element => element.classes().includes('2xl:grid-cols-4'))).toBe(true)
    await historyButtons[4].trigger('click')
    expect(api.showError).toHaveBeenCalledWith('admin.pelicanTest.limit')
    expect(wrapper.findAll('article')).toHaveLength(4)
    wrapper.unmount()
  })
})
