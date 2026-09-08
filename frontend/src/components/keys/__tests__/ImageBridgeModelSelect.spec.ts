import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImageBridgeModelSelect from '../ImageBridgeModelSelect.vue'

const { getModels } = vi.hoisted(() => ({ getModels: vi.fn() }))
vi.mock('@/api', () => ({ keysAPI: { getImageBridgeModels: getModels } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const SelectStub = { name: 'Select', props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue'], template: '<div />' }
const render = (value: string | null = '') => mount(ImageBridgeModelSelect, { props: { modelValue: value }, global: { stubs: { Select: SelectStub } } })

describe('image bridge model selection', () => {
  beforeEach(() => { getModels.mockReset() })

  it('loads model options from the server image group and emits the chosen model', async () => {
    getModels.mockResolvedValue({ group_id: 24, group_name: '生图', models: ['gemini-custom-image'] })
    const wrapper = render()
    await flushPromises()
    const select = wrapper.findComponent(SelectStub)
    expect(getModels).toHaveBeenCalledTimes(1)
    expect(select.props('options').map((option: { value: unknown }) => option.value)).toEqual(['', null, 'gemini-custom-image'])
    select.vm.$emit('update:modelValue', 'gemini-custom-image')
    expect(wrapper.emitted('update:modelValue')).toEqual([['gemini-custom-image']])
  })

  it('preserves an existing selection when its model becomes unavailable', async () => {
    getModels.mockResolvedValue({ group_name: '生图', models: [] })
    const wrapper = render('gemini-retired-image')
    await flushPromises()
    const option = wrapper.findComponent(SelectStub).props('options').find((entry: { value: unknown }) => entry.value === 'gemini-retired-image')
    expect(option.disabled).toBe(true)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('keys.imageBridgeEmpty')
  })

  it('shows a retry after a failed fetch and updates the available models', async () => {
    getModels.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ group_name: '生图', models: ['gemini-3-pro-image'] })
    const wrapper = render(null)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.findComponent(SelectStub).props('modelValue')).toBeNull()
    expect(getModels).toHaveBeenCalledTimes(2)
  })
})
