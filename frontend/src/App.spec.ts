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
          GetAccount: vi.fn().mockResolvedValue({
            signedIn: true,
            type: 'chatgpt',
            email: 'developer@example.com',
            planType: 'plus',
            requiresOpenaiAuth: true,
          }),
          LoginWithChatGPT: vi.fn(),
          LoginWithDeviceCode: vi.fn(),
          Logout: vi.fn(),
          SelectProject: vi.fn(),
          AddProject: vi.fn(),
          ListProjects: vi.fn().mockResolvedValue([]),
          RemoveProject: vi.fn(),
          ListThreads: vi.fn().mockResolvedValue([]),
          StartThread: vi.fn(),
          ResumeThread: vi.fn(),
          SendMessage: vi.fn(),
          InterruptTurn: vi.fn(),
          RespondToApproval: vi.fn(),
          OpenInFileManager: vi.fn(),
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
    expect(wrapper.text()).toContain('Add your first project')
    expect(wrapper.text()).toContain('developer@example.com')
    expect(window.runtime?.EventsOn).toHaveBeenCalled()
  })

  it('unsubscribes from runtime events when unmounted', async () => {
    const unsubscribe = vi.fn()
    window.runtime = { EventsOn: vi.fn().mockReturnValue(unsubscribe) }
    const wrapper = mount(App, { global: { plugins: [createPinia()] } })
    wrapper.unmount()
    expect(unsubscribe).toHaveBeenCalled()
  })
})
