import { defineStore } from 'pinia'

import { backend, subscribe } from '../backend'
import type { Account, Approval, ConversationItem, Project, Thread, TurnStart } from '../types'

interface ItemEnvelope {
  event: string
  payload: {
    item: ConversationItem
    threadId: string
    turnId: string
  }
}

export const useWorkspaceStore = defineStore('workspace', {
  state: () => ({
    account: null as Account | null,
    projects: [] as Project[],
    threads: [] as Thread[],
    activeProject: null as Project | null,
    activeThread: null as Thread | null,
    activeTurn: null as TurnStart | null,
    items: [] as ConversationItem[],
    diff: '',
    approvals: [] as Approval[],
    deviceLogin: null as { verificationUrl: string; userCode: string } | null,
    busy: false,
    sending: false,
    error: '',
    unsubscribers: [] as Array<() => void>,
  }),
  getters: {
    currentApproval: (state) => state.approvals[0] ?? null,
  },
  actions: {
    async initialize() {
      this.connectEvents()
      await Promise.all([this.loadAccount(), this.loadProjects()])
    },
    connectEvents() {
      if (this.unsubscribers.length) return
      this.unsubscribers.push(
        subscribe('codex:account-updated', () => void this.loadAccount()),
        subscribe('codex:login-completed', (payload) => {
          const result = payload as { success: boolean; error?: string }
          if (result.success) {
            this.deviceLogin = null
            void this.loadAccount()
          } else if (result.error) {
            this.error = result.error
          }
        }),
        subscribe('codex:agent-message-delta', (payload) => {
          const event = payload as { itemId: string; delta: string }
          let item = this.items.find((candidate) => candidate.id === event.itemId)
          if (!item) {
            item = { id: event.itemId, type: 'agentMessage', text: '' }
            this.items.push(item)
          }
          item.text = `${item.text ?? ''}${event.delta}`
        }),
        subscribe('codex:item', (payload) => this.applyItem(payload as ItemEnvelope)),
        subscribe('codex:command-output-delta', (payload) => {
          const event = payload as { itemId: string; delta: string }
          const item = this.items.find((candidate) => candidate.id === event.itemId)
          if (item) item.aggregatedOutput = `${item.aggregatedOutput ?? ''}${event.delta}`
        }),
        subscribe('codex:turn-completed', () => {
          this.activeTurn = null
          this.sending = false
          if (this.activeProject) void this.loadThreads(this.activeProject.id)
        }),
        subscribe('codex:diff-updated', (payload) => {
          this.diff = (payload as { diff: string }).diff
        }),
        subscribe('codex:approval-requested', (payload) => {
          this.approvals.push(payload as Approval)
        }),
      )
    },
    disconnectEvents() {
      this.unsubscribers.splice(0).forEach((unsubscribe) => unsubscribe())
    },
    async loadAccount() {
      try {
        this.account = await backend.getAccount()
      } catch (error) {
        this.error = message(error)
      }
    },
    async login() {
      await this.run(async () => {
        await backend.loginWithChatGPT()
      })
    },
    async loginWithDeviceCode() {
      await this.run(async () => {
        const result = await backend.loginWithDeviceCode()
        this.deviceLogin = {
          verificationUrl: result.verificationUrl ?? '',
          userCode: result.userCode ?? '',
        }
      })
    },
    async logout() {
      await this.run(async () => {
        await backend.logout()
        this.account = { signedIn: false, requiresOpenaiAuth: true }
      })
    },
    async loadProjects() {
      try {
        this.projects = await backend.listProjects()
        if (!this.activeProject && this.projects[0]) await this.selectProject(this.projects[0])
      } catch (error) {
        this.error = message(error)
      }
    },
    async addProject() {
      await this.run(async () => {
        const project = await backend.selectProject()
        await this.loadProjects()
        await this.selectProject(project)
      })
    },
    async removeProject(project: Project) {
      await this.run(async () => {
        await backend.removeProject(project.id)
        if (this.activeProject?.id === project.id) {
          this.activeProject = null
          this.activeThread = null
          this.threads = []
          this.items = []
        }
        await this.loadProjects()
      })
    },
    async selectProject(project: Project) {
      this.activeProject = project
      this.activeThread = null
      this.items = []
      this.diff = ''
      await this.loadThreads(project.id)
    },
    async loadThreads(projectId: string) {
      this.threads = await backend.listThreads(projectId)
      const updatedActive = this.threads.find((thread) => thread.id === this.activeThread?.id)
      if (updatedActive) this.activeThread = updatedActive
    },
    async newThread() {
      if (!this.activeProject) return
      await this.run(async () => {
        const thread = await backend.startThread(this.activeProject!.id)
        await this.loadThreads(this.activeProject!.id)
        await this.selectThread(thread, false)
      })
    },
    async selectThread(thread: Thread, resume = true) {
      await this.run(async () => {
        this.activeThread = resume ? await backend.resumeThread(thread.id) : thread
        this.items = this.activeThread.items ?? []
        this.diff = ''
        this.activeTurn = null
      })
    },
    async send(text: string) {
      if (!this.activeThread || !text.trim()) return
      this.sending = true
      this.error = ''
      this.items.push({
        id: `local-${Date.now()}`,
        type: 'userMessage',
        text: text.trim(),
      })
      try {
        this.activeTurn = await backend.sendMessage(this.activeThread.id, text)
      } catch (error) {
        this.error = message(error)
        this.sending = false
      }
    },
    async interrupt() {
      if (!this.activeTurn) return
      await this.run(async () => {
        await backend.interruptTurn(this.activeTurn!.threadId, this.activeTurn!.turnId)
      })
    },
    async answerApproval(decision: string) {
      const approval = this.currentApproval
      if (!approval) return
      await this.run(async () => {
        await backend.respondToApproval(approval.requestId, approval.kind, decision)
        this.approvals.shift()
      })
    },
    applyItem(envelope: ItemEnvelope) {
      const item = envelope.payload.item
      const index = this.items.findIndex((candidate) => candidate.id === item.id)
      if (index === -1) {
        if (item.type === 'userMessage') return
        this.items.push(item)
      } else {
        const existing = this.items[index]!
        this.items[index] = {
          ...existing,
          ...item,
          text: item.text ?? existing.text,
          aggregatedOutput: item.aggregatedOutput ?? existing.aggregatedOutput,
        }
      }
    },
    async run(action: () => Promise<void>) {
      this.busy = true
      this.error = ''
      try {
        await action()
      } catch (error) {
        const text = message(error)
        if (!text.toLowerCase().includes('cancel')) this.error = text
      } finally {
        this.busy = false
      }
    },
  },
})

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
