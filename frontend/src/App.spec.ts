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
          GetRateLimits: vi.fn().mockResolvedValue({}),
          ListModels: vi.fn().mockResolvedValue([]),
          ListAgentProviders: vi.fn().mockResolvedValue([]),
          ListProviderModels: vi.fn().mockResolvedValue([]),
          ListProviderUsage: vi.fn().mockResolvedValue([]),
          BeginProviderLogin: vi.fn(),
          StartProviderConsole: vi.fn(),
          WriteProviderConsole: vi.fn(),
          ResizeProviderConsole: vi.fn(),
          StopProviderConsole: vi.fn(),
          LogoutProvider: vi.fn(),
          ListAgentTasks: vi.fn().mockResolvedValue([]),
          CreateAgentTask: vi.fn(),
          PreflightProject: vi.fn().mockResolvedValue([]),
          InspectProjectContext: vi.fn().mockResolvedValue([]),
          AssessCommandRisk: vi.fn(),
          ListProjectMemory: vi.fn().mockResolvedValue([]),
          ProposeProjectMemory: vi.fn(),
          ReviewProjectMemory: vi.fn(),
          CreateTaskSnapshot: vi.fn(),
          CreateTaskWorktree: vi.fn(),
          RunTaskVerification: vi.fn(),
          ApproveAgentTaskStage: vi.fn(),
          ListCoordinationMessages: vi.fn().mockResolvedValue([]),
          PostCoordinationMessage: vi.fn(),
          LoginWithChatGPT: vi.fn(),
          LoginWithDeviceCode: vi.fn(),
          Logout: vi.fn(),
          SelectProject: vi.fn(),
          AddProject: vi.fn(),
          ListProjects: vi.fn().mockResolvedValue([]),
          RemoveProject: vi.fn(),
          ListThreads: vi.fn().mockResolvedValue([]),
          StartThread: vi.fn(),
          StartAgentThread: vi.fn(),
          ResumeThread: vi.fn(),
          SendMessage: vi.fn(),
          SendMessageWithOptions: vi.fn(),
          SearchProjectFiles: vi.fn().mockResolvedValue([]),
          OpenFile: vi.fn(),
          OpenProjectFile: vi.fn(),
          ExportThread: vi.fn(),
          ImportThread: vi.fn(),
          InterruptTurn: vi.fn(),
          RespondToApproval: vi.fn(),
          RespondToPermission: vi.fn(),
          RespondToUserInput: vi.fn(),
          RespondToElicitation: vi.fn(),
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
    expect(wrapper.text()).toContain('Settings')
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
