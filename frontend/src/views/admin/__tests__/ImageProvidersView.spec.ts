import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImageProvidersView from '../ImageProvidersView.vue'

const api = vi.hoisted(() => ({ groups: vi.fn(), list: vi.fn(), get: vi.fn(), create: vi.fn(), copy: vi.fn(), update: vi.fn(), remove: vi.fn(), setSchedulable: vi.fn(), preview: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: api.groups }, accounts: { list: api.list, getById: api.get, create: api.create, copyToImageProvider: api.copy, update: api.update, delete: api.remove, setSchedulable: api.setSchedulable, previewImageModels: api.preview } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: api.success, showError: api.error }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
const SelectStub = { name: 'Select', props: ['modelValue', 'options', 'disabled'], template: '<div />' }
const render = () => mount(ImageProvidersView, { global: { stubs: {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  DataTable: { props: ['data'], template: '<div><slot v-for="row in data" name="cell-actions" :row="row" /></div>' },
  Pagination: true, RouterLink: true, Select: SelectStub
} } })

describe('independent image providers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.groups.mockResolvedValue([{ id: 24, name: '生图', status: 'active', allow_image_generation: true }])
    api.list.mockResolvedValue({ items: [], total: 0 })
    api.create.mockResolvedValue({ id: 100 })
    api.copy.mockResolvedValue({ id: 102, name: 'Source (Copy)' })
    api.update.mockResolvedValue({ id: 100 })
    api.remove.mockReset().mockResolvedValue({ message: 'Account deleted successfully' })
    api.preview.mockReset().mockResolvedValue({ models: ['gpt-image-2', 'gemini-3.1-flash-image', 'gpt-5.5', 'custom-model'] })
  })

  it('defaults to OpenAI and binds the new image provider to the image group', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.create')!.trigger('click')
    expect(wrapper.findComponent(SelectStub).props('modelValue')).toBe('openai')
    expect(wrapper.findComponent(SelectStub).props('options')).toEqual([
      { value: 'openai', label: 'OpenAI' },
      { value: 'gemini', label: 'Gemini' },
      { value: 'grok', label: 'Grok / xAI' }
    ])
    await wrapper.get('#image-provider-name').setValue('Images')
    await wrapper.get('#image-provider-url').setValue('https://images.example/v1')
    await wrapper.get('#image-provider-key').setValue('test-secret')
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.fetchModels')!.trigger('click')
    await flushPromises()
    await wrapper.get('input[value="gemini-3.1-flash-image"]').setValue(true)
    await wrapper.get('input[value="gpt-image-2"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ platform: 'openai', type: 'apikey', group_ids: [24],
      credentials: { base_url: 'https://images.example/v1', api_key: 'test-secret', model_mapping: {
        'gemini-3.1-flash-image': 'gemini-3.1-flash-image', 'gpt-image-2': 'gpt-image-2'
      } }
    }))
  })

  it('keeps Grok image accounts visible in the provider list', async () => {
    api.list.mockResolvedValue({ items: [{ id: 101, name: 'Grok Images', platform: 'grok', status: 'active', schedulable: true }], total: 1 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('common.edit')
  })

  it('copies a supported existing API-key account into the image group', async () => {
    api.list.mockImplementation((_page, _pageSize, filters) => {
      if (filters?.group) return Promise.resolve({ items: [], total: 0 })
      return Promise.resolve({ items: [
        { id: 51, name: 'Claude OAuth', platform: 'anthropic', type: 'oauth' },
        { id: 52, name: 'Grok Source', platform: 'grok', type: 'apikey', credentials: { base_url: 'https://api.x.ai/v1' } },
        { id: 53, name: 'Gemini Source', platform: 'gemini', type: 'apikey' }
      ], total: 3 })
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.copyExisting')!.trigger('click')
    await flushPromises()
    const sourceSelect = wrapper.findComponent(SelectStub)
    expect(sourceSelect.props('options')).toEqual([
      { value: 52, label: 'Grok Source · Grok / xAI', description: 'https://api.x.ai/v1' },
      { value: 53, label: 'Gemini Source · Gemini', description: '' }
    ])
    expect(sourceSelect.props('modelValue')).toBe(52)
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.copyConfirm')!.trigger('click')
    await flushPromises()
    expect(api.copy).toHaveBeenCalledWith(52, 24)
    expect(api.success).toHaveBeenCalledWith('imageProviders.copySuccess')
  })

  it('preserves existing credentials and settings when editing without a new key', async () => {
    const account = { id: 100, name: 'Images', platform: 'openai', type: 'apikey', concurrency: 3,
      credentials: { base_url: 'https://old.example', model_mapping: { 'gpt-image-2': 'gpt-image-2' }, custom_setting: true } }
    api.list.mockResolvedValue({ items: [account], total: 1 })
    api.get.mockResolvedValue(account)
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith({ account_id: 100, platform: 'openai', base_url: 'https://old.example' })
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect((wrapper.get('input[value="gpt-image-2"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.findComponent(SelectStub).props('disabled')).toBe(true)
    await wrapper.get('#image-provider-url').setValue('https://new.example')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.update.mock.calls[0][1].credentials).toEqual({ base_url: 'https://new.example', custom_setting: true, model_mapping: { 'gpt-image-2': 'gpt-image-2' } })
    expect(api.update.mock.calls[0][1]).not.toHaveProperty('group_ids')
  })

  it('enables scheduling through the dedicated endpoint', async () => {
    api.list.mockResolvedValue({ items: [{ id: 100, name: 'Images', platform: 'openai', status: 'inactive', schedulable: false }], total: 1 })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.enable')!.trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(100, { status: 'active' })
    expect(api.setSchedulable).toHaveBeenCalledWith(100, true)
  })

  it('fetches draft models, filters and selects them while preserving existing aliases', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.create')!.trigger('click')
    const fetchButton = wrapper.findAll('button').find(button => button.text() === 'imageProviders.fetchModels')!
    expect(fetchButton.attributes('disabled')).toBeDefined()
    await wrapper.get('#image-provider-url').setValue('https://images.example/v1')
    await wrapper.get('#image-provider-key').setValue('test-key')
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.advancedModels')!.trigger('click')
    await wrapper.get('#image-provider-models').setValue('gpt-image-alias=gemini-3.1-flash-image')
    await fetchButton.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith({ platform: 'openai', base_url: 'https://images.example/v1', api_key: 'test-key' })
    expect(wrapper.find('input[value="gpt-5.5"]').exists()).toBe(false)
    expect((wrapper.get('input[value="gemini-3.1-flash-image"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('input[value="gpt-image-2"]').setValue(true)
    expect((wrapper.get('#image-provider-models').element as HTMLTextAreaElement).value).toBe('gpt-image-alias=gemini-3.1-flash-image\ngpt-image-2')
    await wrapper.get('input[type="checkbox"]').setValue(false)
    await wrapper.get('input[type="search"]').setValue('custom')
    expect(wrapper.find('input[value="custom-model"]').exists()).toBe(true)
    expect(wrapper.find('input[value="gpt-image-2"]').exists()).toBe(false)
    expect(api.create).not.toHaveBeenCalled()
    expect(api.update).not.toHaveBeenCalled()
  })

  it('uses the saved secret with an edited URL and reports fetch errors without losing mappings', async () => {
    const account = { id: 100, name: 'Images', platform: 'openai', concurrency: 3, credentials: { base_url: 'https://old.example', model_mapping: { 'gpt-image-2': 'gpt-image-2' } } }
    api.list.mockResolvedValue({ items: [account], total: 1 })
    api.get.mockResolvedValue(account)
    api.preview.mockRejectedValue({ status: 502, message: 'Upstream HTTP 401' })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()
    await wrapper.get('#image-provider-url').setValue('https://new.example/v1')
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.fetchModels')!.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenLastCalledWith({ account_id: 100, platform: 'openai', base_url: 'https://new.example/v1' })
    expect(wrapper.get('[role="alert"]').text()).toBe('Upstream HTTP 401')
    expect(wrapper.get('[role="group"]').text()).toContain('gpt-image-2')
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(api.update).not.toHaveBeenCalled()
  })

  it('discards results if connection settings change while fetching', async () => {
    let resolve!: (value: { models: string[] }) => void
    api.preview.mockImplementation(() => new Promise(done => { resolve = done }))
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.create')!.trigger('click')
    await wrapper.get('#image-provider-url').setValue('https://first.example')
    await wrapper.get('#image-provider-key').setValue('test-key')
    await wrapper.findAll('button').find(button => button.text() === 'imageProviders.fetchModels')!.trigger('click')
    await wrapper.get('#image-provider-url').setValue('https://second.example')
    resolve({ models: ['gpt-image-stale'] })
    await flushPromises()
    expect(wrapper.find('input[value="gpt-image-stale"]').exists()).toBe(false)
    expect(wrapper.findAll('button').some(button => button.text() === 'imageProviders.fetchingModels')).toBe(false)
  })

  it('requires confirmation before deleting and refreshes the list after success', async () => {
    const account = { id: 100, name: 'Images', platform: 'openai', status: 'active', schedulable: true }
    api.list.mockResolvedValue({ items: [account], total: 1 })
    const wrapper = render()
    await flushPromises()
    const deleteButton = () => wrapper.findAll('button').find(button => button.text() === 'common.delete')!
    await deleteButton().trigger('click')
    expect(api.remove).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('imageProviders.deleteImpact')
    await wrapper.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    expect(api.remove).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('imageProviders.deleteImpact')
    await deleteButton().trigger('click')
    api.list.mockResolvedValue({ items: [], total: 0 })
    await wrapper.findAll('button').filter(button => button.text() === 'common.delete').pop()!.trigger('click')
    await flushPromises()
    expect(api.remove).toHaveBeenCalledTimes(1)
    expect(api.remove).toHaveBeenCalledWith(100)
    expect(api.success).toHaveBeenCalledWith('imageProviders.deleted')
    expect(wrapper.text()).not.toContain('imageProviders.deleteImpact')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })

  it('retains the confirmation and shows an error when deletion fails', async () => {
    api.list.mockResolvedValue({ items: [{ id: 100, name: 'Images', platform: 'openai', status: 'active', schedulable: true }], total: 1 })
    api.remove.mockRejectedValue({ message: 'Delete failed' })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.delete')!.trigger('click')
    await wrapper.findAll('button').filter(button => button.text() === 'common.delete').pop()!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Delete failed')
    expect(wrapper.text()).toContain('imageProviders.deleteImpact')
    expect(api.list).toHaveBeenCalledTimes(1)
    expect(api.success).not.toHaveBeenCalled()
  })
})
