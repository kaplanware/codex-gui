import { defineStore } from 'pinia'

import { backend, subscribe } from '../backend'
import type {
  Account,
  AgentProvider,
  AgentTaskBundle,
  AgentTaskRequest,
  Approval,
  ConversationItem,
  ContextEntry,
  CoordinationMessage,
  ElicitationRequest,
  FileMatch,
  MentionInput,
  Model,
  Project,
  ProjectMemory,
  PreflightCheck,
  ProviderModel,
  ProviderUsage,
  RateLimits,
  TaskChanges,
  Thread,
  TurnOptions,
  TurnStart,
  UserInputRequest,
} from '../types'

interface ItemEnvelope {
  event: string
  payload: {
    item: ConversationItem
    threadId: string
    turnId: string
  }
}

interface QueuedMessage {
  id: string
  text: string
  options: TurnOptions
}

interface PlanArtifact {
  text: string
  status: 'needs_review' | 'approved' | 'revision_requested'
  createdAt: string
}

export const useWorkspaceStore = defineStore('workspace', {
  state: () => ({
    account: null as Account | null,
    rateLimits: null as RateLimits | null,
    models: [] as Model[],
    agentProviders: [] as AgentProvider[],
    providerModels: [] as ProviderModel[],
    providerUsage: [] as ProviderUsage[],
    selectedProvider: 'codex',
    agentTasks: [] as AgentTaskBundle[],
    taskChangesByTask: {} as Record<string, TaskChanges>,
    preflight: [] as PreflightCheck[],
    contextEntries: [] as ContextEntry[],
    projectMemory: [] as ProjectMemory[],
    coordinationByTask: {} as Record<string, CoordinationMessage[]>,
    selectedModel: '',
    selectedEffort: '',
    selectedMode: 'default' as 'default' | 'plan',
    planArtifact: null as PlanArtifact | null,
    lastTurnMode: 'default' as 'default' | 'plan',
    projects: [] as Project[],
    threadsByProject: {} as Record<string, Thread[]>,
    expandedProjects: {} as Record<string, boolean>,
    activeProject: null as Project | null,
    activeThread: null as Thread | null,
    activeTurn: null as TurnStart | null,
    items: [] as ConversationItem[],
    diff: '',
    approvals: [] as Approval[],
    userInputRequests: [] as UserInputRequest[],
    elicitations: [] as ElicitationRequest[],
    queue: [] as QueuedMessage[],
    fileMatches: [] as FileMatch[],
    mentions: [] as MentionInput[],
    deviceLogin: null as { verificationUrl: string; userCode: string } | null,
    busy: false,
    sending: false,
    searchingFiles: false,
    error: '',
    unsubscribers: [] as Array<() => void>,
  }),
  getters: {
    currentApproval: (state) => state.approvals[0] ?? null,
    currentUserInput: (state) => state.userInputRequests[0] ?? null,
    currentElicitation: (state) => state.elicitations[0] ?? null,
    threads: (state) =>
      state.activeProject ? (state.threadsByProject[state.activeProject.id] ?? []) : [],
    currentModel: (state) => state.models.find((model) => model.id === state.selectedModel),
    availableProviders: (state) =>
      state.agentProviders.filter(
        (provider) =>
          provider.installed &&
          provider.runnable &&
          (provider.authStatus === 'authenticated' || provider.authStatus === 'not-required'),
      ),
    supportedEfforts(): string[] {
      return this.currentModel?.supportedReasoningEfforts ?? []
    },
  },
  actions: {
    async initialize() {
      this.connectEvents()
      await Promise.all([
        this.loadAccount(),
        this.loadProjects(),
        this.loadModels(),
        this.loadProviders(),
        this.loadProviderUsage(),
      ])
    },
    connectEvents() {
      if (this.unsubscribers.length) return
      this.unsubscribers.push(
        subscribe('codex:account-updated', () => void this.loadAccount()),
        subscribe('codex:rate-limits-updated', () => void this.loadRateLimits()),
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
          const event = payload as { itemId: string; delta: string; threadId?: string }
          if (event.threadId && event.threadId !== this.activeThread?.id) return
          let item = this.items.find((candidate) => candidate.id === event.itemId)
          if (!item) {
            item = { id: event.itemId, type: 'agentMessage', text: '' }
            this.items.push(item)
          }
          item.text = `${item.text ?? ''}${event.delta}`
        }),
        subscribe('codex:item', (payload) => {
          const envelope = payload as ItemEnvelope
          if (
            envelope.payload.threadId &&
            envelope.payload.threadId !== this.activeThread?.id
          )
            return
          this.applyItem(envelope)
        }),
        subscribe('codex:command-output-delta', (payload) => {
          const event = payload as { itemId: string; delta: string }
          const item = this.items.find((candidate) => candidate.id === event.itemId)
          if (item) item.aggregatedOutput = `${item.aggregatedOutput ?? ''}${event.delta}`
        }),
        subscribe('codex:turn-completed', (payload) => {
          const event = payload as { thread?: { id?: string }; threadId?: string }
          const threadId = event.threadId ?? event.thread?.id
          if (threadId && threadId !== this.activeThread?.id) return
          if (this.lastTurnMode === 'plan') {
            const plan = [...this.items]
              .reverse()
              .find((item) => item.type === 'agentMessage' && item.text?.trim())
            if (plan?.text) {
              this.planArtifact = {
                text: plan.text,
                status: 'needs_review',
                createdAt: new Date().toISOString(),
              }
            }
          }
          this.activeTurn = null
          this.sending = false
          if (this.activeProject) void this.loadThreads(this.activeProject.id)
          void this.drainQueue()
        }),
        subscribe('codex:diff-updated', (payload) => {
          this.diff = (payload as { diff: string }).diff
        }),
        subscribe('codex:approval-requested', (payload) => {
          this.approvals.push(payload as Approval)
        }),
        subscribe('codex:user-input-requested', (payload) => {
          this.userInputRequests.push(payload as UserInputRequest)
        }),
        subscribe('codex:elicitation-requested', (payload) => {
          this.elicitations.push(payload as ElicitationRequest)
        }),
        subscribe('agents:tasks-updated', () => void this.loadAgentTasks()),
        subscribe('agents:coordination-message', (payload) => {
          const message = payload as CoordinationMessage
          const messages = this.coordinationByTask[message.taskId] ?? []
          if (!messages.some((candidate) => candidate.id === message.id)) {
            this.coordinationByTask[message.taskId] = [...messages, message]
          }
        }),
      )
    },
    disconnectEvents() {
      this.unsubscribers.splice(0).forEach((unsubscribe) => unsubscribe())
    },
    async loadAccount() {
      try {
        this.account = await backend.getAccount()
        if (this.account.signedIn) await this.loadRateLimits()
      } catch (error) {
        this.error = message(error)
      }
    },
    async loadRateLimits() {
      try {
        this.rateLimits = await backend.getRateLimits()
      } catch {
        this.rateLimits = null
      }
    },
    async loadModels() {
      try {
        this.models = await backend.listModels()
        const defaultModel = this.models.find((model) => model.isDefault) ?? this.models[0]
        if (!this.selectedModel && defaultModel) this.setModel(defaultModel.id)
      } catch (error) {
        this.error = message(error)
      }
    },
    async loadProviders() {
      try {
        this.agentProviders = await backend.listAgentProviders()
        const available = this.agentProviders.filter(
          (provider) =>
            provider.installed &&
            provider.runnable &&
            (provider.authStatus === 'authenticated' || provider.authStatus === 'not-required'),
        )
        if (!available.some((provider) => provider.id === this.selectedProvider)) {
          this.selectedProvider = available[0]?.id ?? ''
        }
        await this.loadProviderModels()
      } catch (error) {
        this.error = message(error)
      }
    },
    async loadProviderModels() {
      try {
        this.providerModels = await backend.listProviderModels(this.selectedProvider)
        const current = this.providerModels.find((model) => model.id === this.selectedModel)
        if (!current) this.selectedModel = this.providerModels[0]?.id ?? ''
      } catch {
        this.providerModels = []
      }
    },
    async selectProvider(providerId: string) {
      this.selectedProvider = providerId
      this.selectedModel = ''
      await this.loadProviderModels()
    },
    async loadProviderUsage() {
      try {
        this.providerUsage = await backend.listProviderUsage()
      } catch {
        this.providerUsage = []
      }
    },
    async loadAgentWorkspace() {
      try {
        const projectId = this.activeProject?.id ?? ''
        const [providers, tasks] = await Promise.all([
          backend.listAgentProviders(),
          backend.listAgentTasks(projectId),
        ])
        this.agentProviders = providers
        this.agentTasks = tasks
        if (projectId) {
          const [preflight, contextEntries, projectMemory] = await Promise.all([
            backend.preflightProject(projectId),
            backend.inspectProjectContext(projectId),
            backend.listProjectMemory(projectId),
          ])
          this.preflight = preflight
          this.contextEntries = contextEntries
          this.projectMemory = projectMemory
        }
      } catch (error) {
        this.error = message(error)
      }
    },
    async manageProviderLogin(providerId: string) {
      await this.run(async () => {
        await backend.beginProviderLogin(providerId)
        globalThis.setTimeout(() => void this.loadAgentWorkspace(), 2500)
      })
    },
    async logoutProvider(providerId: string) {
      await this.run(async () => {
        await backend.logoutProvider(providerId)
        await this.loadAgentWorkspace()
      })
    },
    async loadAgentTasks() {
      try {
        this.agentTasks = await backend.listAgentTasks(this.activeProject?.id ?? '')
      } catch (error) {
        this.error = message(error)
      }
    },
    async loadTaskChanges(taskId: string) {
      try {
        this.taskChangesByTask[taskId] = await backend.getTaskChanges(taskId)
      } catch (error) {
        this.error = message(error)
      }
    },
    async createAgentTask(request: Omit<AgentTaskRequest, 'projectId'>) {
      if (!this.activeProject) return
      await this.run(async () => {
        await backend.createAgentTask({ ...request, projectId: this.activeProject!.id })
        await this.loadAgentTasks()
      })
    },
    async createTaskSnapshot(taskId: string) {
      await this.run(async () => {
        await backend.createTaskSnapshot(taskId)
        await this.loadAgentTasks()
      })
    },
    async createTaskWorktree(taskId: string) {
      await this.run(async () => {
        await backend.createTaskWorktree(taskId)
        await this.loadAgentTasks()
      })
    },
    async verifyTask(taskId: string) {
      await this.run(async () => {
        await backend.runTaskVerification(taskId)
        await this.loadAgentTasks()
      })
    },
    async approveTaskStage(taskId: string) {
      await this.run(async () => {
        await backend.approveAgentTaskStage(taskId)
        await this.loadAgentTasks()
      })
    },
    async continueTask(taskId: string) {
      await this.run(async () => {
        await backend.continueAgentTask(taskId)
        await this.loadAgentTasks()
      })
    },
    async retryTaskRun(taskId: string, runId: string) {
      await this.run(async () => {
        await backend.retryAgentTaskRun(taskId, runId)
        await this.loadAgentTasks()
      })
    },
    async cancelTask(taskId: string) {
      await this.run(async () => {
        await backend.cancelAgentTask(taskId)
        await this.loadAgentTasks()
      })
    },
    async restartTask(taskId: string) {
      await this.run(async () => {
        await backend.restartAgentTask(taskId)
        await this.loadAgentTasks()
      })
    },
    async loadCoordination(taskId: string) {
      try {
        this.coordinationByTask[taskId] = await backend.listCoordinationMessages(taskId)
      } catch (error) {
        this.error = message(error)
      }
    },
    async postCoordination(taskId: string, text: string) {
      if (!text.trim()) return
      await this.run(async () => {
        const posted = await backend.postCoordinationMessage(taskId, text)
        const messages = this.coordinationByTask[taskId] ?? []
        this.coordinationByTask[taskId] = [...messages, posted]
      })
    },
    async proposeMemory(kind: string, title: string, content: string) {
      if (!this.activeProject || !title.trim() || !content.trim()) return
      await this.run(async () => {
        await backend.proposeProjectMemory(this.activeProject!.id, kind, title, content)
        this.projectMemory = await backend.listProjectMemory(this.activeProject!.id)
      })
    },
    async reviewMemory(id: string, decision: 'approved' | 'rejected') {
      if (!this.activeProject) return
      await this.run(async () => {
        await backend.reviewProjectMemory(id, decision)
        this.projectMemory = await backend.listProjectMemory(this.activeProject!.id)
      })
    },
    setModel(id: string) {
      this.selectedModel = id
      const model = this.models.find((candidate) => candidate.id === id)
      if (model && !model.supportedReasoningEfforts.includes(this.selectedEffort)) {
        this.selectedEffort = model.defaultReasoningEffort
      }
    },
    async login() {
      await this.run(async () => void (await backend.loginWithChatGPT()))
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
        this.rateLimits = null
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
        delete this.threadsByProject[project.id]
        if (this.activeProject?.id === project.id) {
          this.activeProject = null
          this.activeThread = null
          this.items = []
        }
        await this.loadProjects()
      })
    },
    async toggleProject(project: Project) {
      const willExpand = !this.expandedProjects[project.id]
      this.expandedProjects[project.id] = willExpand
      if (willExpand) {
        this.activeProject = project
        await this.loadThreads(project.id)
      }
    },
    async selectProject(project: Project) {
      this.activeProject = project
      this.expandedProjects[project.id] = true
      this.activeThread = null
      this.items = []
      this.diff = ''
      this.queue = []
      await this.loadThreads(project.id)
      void this.loadAgentWorkspace()
    },
    async loadThreads(projectId: string) {
      this.threadsByProject[projectId] = await backend.listThreads(projectId)
      const updatedActive = this.threadsByProject[projectId].find(
        (thread) => thread.id === this.activeThread?.id,
      )
      if (updatedActive) this.activeThread = updatedActive
    },
    async newThread(project?: Project | null) {
      project ??= this.activeProject
      if (!project) return
      await this.run(async () => {
        this.activeProject = project
        this.expandedProjects[project.id] = true
        const thread = await backend.startAgentThread(
          project.id,
          this.selectedProvider,
          this.selectedModel,
        )
        await this.loadThreads(project.id)
        await this.selectThread(thread, false)
      })
    },
    async selectThread(thread: Thread, resume = true) {
      await this.run(async () => {
        const project = this.projects.find((candidate) => candidate.id === thread.projectId)
        if (project) {
          this.activeProject = project
          this.expandedProjects[project.id] = true
        }
        this.activeThread = resume ? await backend.resumeThread(thread.id) : thread
        if (!this.selectedProvider && this.availableProviders[0]) {
          this.selectedProvider = this.availableProviders[0].id
          await this.loadProviderModels()
        }
        this.items = this.activeThread.items ?? []
        this.diff = ''
        this.activeTurn = null
        this.queue = []
        this.mentions = []
      })
    },
    async exportActiveThread() {
      if (!this.activeThread) return
      await this.run(async () => {
        await backend.exportThread(this.activeThread!.id)
      })
    },
    async importThread(project?: Project | null) {
      project ??= this.activeProject
      if (!project) return
      await this.run(async () => {
        const thread = await backend.importThread(project.id)
        this.activeProject = project
        this.expandedProjects[project.id] = true
        await this.loadThreads(project.id)
        await this.selectThread(thread, false)
      })
    },
    async send(text: string) {
      if (!this.activeThread || !text.trim()) return
      const entry: QueuedMessage = {
        id: `local-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        text: text.trim(),
        options: {
          providerId: this.selectedProvider,
          model: this.selectedModel,
          effort: this.selectedEffort,
          mode: this.selectedMode,
          mentions: [...this.mentions],
        },
      }
      this.mentions = []
      this.items.push({
        id: entry.id,
        type: 'userMessage',
        text: entry.text,
        status: this.activeTurn || this.sending ? 'queued' : 'sending',
      })
      if (this.activeTurn || this.sending) {
        this.queue.push(entry)
        return
      }
      await this.sendNow(entry)
    },
    async sendNow(entry: QueuedMessage) {
      if (!this.activeThread) return
      this.sending = true
      this.error = ''
      const localItem = this.items.find((item) => item.id === entry.id)
      if (localItem) localItem.status = 'sending'
      try {
        this.lastTurnMode = entry.options.mode ?? 'default'
        this.activeTurn = await backend.sendMessageWithOptions(
          this.activeThread.id,
          entry.text,
          entry.options,
        )
        if (localItem) localItem.status = 'sent'
      } catch (error) {
        if (localItem) localItem.status = 'failed'
        this.error = message(error)
        this.sending = false
        await this.drainQueue()
      }
    },
    async drainQueue() {
      if (this.activeTurn || this.sending || !this.queue.length) return
      const next = this.queue.shift()
      if (next) await this.sendNow(next)
    },
    async approvePlan() {
      if (!this.planArtifact) return
      this.planArtifact.status = 'approved'
      this.selectedMode = 'default'
      await this.send(
        'Implement the approved plan now. Follow it step by step, preserve unrelated changes, and run every verification listed in the plan before reporting completion.',
      )
    },
    async revisePlan(feedback: string) {
      if (!this.planArtifact || !feedback.trim()) return
      this.planArtifact.status = 'revision_requested'
      this.selectedMode = 'plan'
      await this.send(`Revise the plan using this review feedback. Do not implement yet:\n\n${feedback}`)
    },
    closePlanReview() {
      this.planArtifact = null
    },
    async interrupt() {
      if (!this.activeTurn) return
      await this.run(async () => {
        await backend.interruptTurn(this.activeTurn!.threadId, this.activeTurn!.turnId)
      })
    },
    async searchFiles(query: string) {
      if (!this.activeProject) return
      this.searchingFiles = true
      try {
        this.fileMatches = await backend.searchProjectFiles(this.activeProject.id, query)
      } catch {
        this.fileMatches = []
      } finally {
        this.searchingFiles = false
      }
    },
    addMention(match: FileMatch) {
      if (!this.mentions.some((mention) => mention.path === match.path)) {
        this.mentions.push({ name: match.relativePath, path: match.path })
      }
      this.fileMatches = []
    },
    removeMention(path: string) {
      this.mentions = this.mentions.filter((mention) => mention.path !== path)
    },
    async answerApproval(decision: string) {
      const approval = this.currentApproval
      if (!approval) return
      await this.run(async () => {
        if (approval.kind === 'permissions') {
          if (decision === 'accept' || decision === 'acceptForSession') {
            await backend.respondToPermission(
              approval.requestId,
              approval.details.permissions ?? {},
              decision === 'acceptForSession' ? 'session' : 'turn',
            )
          } else {
            await backend.respondToApproval(approval.requestId, approval.kind, decision)
          }
        } else {
          await backend.respondToApproval(approval.requestId, approval.kind, decision)
        }
        this.approvals.shift()
      })
    },
    async answerUserInput(answers: Record<string, string[]>) {
      const request = this.currentUserInput
      if (!request) return
      await this.run(async () => {
        await backend.respondToUserInput(request.requestId, answers)
        this.userInputRequests.shift()
      })
    },
    async answerElicitation(
      action: 'accept' | 'decline' | 'cancel',
      content: Record<string, unknown> = {},
    ) {
      const request = this.currentElicitation
      if (!request) return
      await this.run(async () => {
        await backend.respondToElicitation(request, action, content)
        this.elicitations.shift()
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
