import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import App from './App.vue'

const status = {
  appName: 'Linux Codex Desktop',
  appVersion: '0.1.0-alpha.1',
  platform: 'linux/amd64',
  state: 'ready',
  installation: {
    found: true,
    path: '/usr/bin/codex',
    version: '0.145.0',
    minimumVersion: '0.145.0',
    supported: true,
    newerThanTested: false,
  },
  git: { name: 'Git', found: true, path: '/usr/bin/git', version: '2.50.0' },
  stderr: [],
  updatedAt: new Date().toISOString(),
}

describe('App', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.go = {
      main: {
        App: {
          GetStartupStatus: vi.fn().mockResolvedValue(status),
          DetectCodex: vi.fn(),
          StartCodex: vi.fn(),
          RestartCodex: vi.fn(),
        },
      },
    }
    window.runtime = {
      EventsOn: vi.fn().mockReturnValue(vi.fn()),
    }
  })

  it('renders detected dependencies and connection state', async () => {
    const wrapper = mount(App, {
      global: { plugins: [createPinia()] },
    })
    await vi.waitFor(() => expect(wrapper.text()).toContain('ready'))
    expect(wrapper.text()).toContain('0.145.0')
    expect(wrapper.text()).toContain('2.50.0')
    expect(window.runtime?.EventsOn).toHaveBeenCalledTimes(2)
  })

  it('unsubscribes from runtime events when unmounted', async () => {
    const unsubscribe = vi.fn()
    window.runtime = { EventsOn: vi.fn().mockReturnValue(unsubscribe) }
    const wrapper = mount(App, { global: { plugins: [createPinia()] } })
    wrapper.unmount()
    expect(unsubscribe).toHaveBeenCalledTimes(2)
  })
})
