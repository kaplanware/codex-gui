<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'

import { backend, subscribe } from './backend'
import { useDiagnosticsStore } from './stores/diagnostics'
import { useWorkspaceStore } from './stores/workspace'
import type { AgentProvider, ConversationItem, FileMatch, ProviderConsoleEvent } from './types'

const diagnostics = useDiagnosticsStore()
const workspace = useWorkspaceStore()
const prompt = ref('')
const conversation = ref<HTMLElement | null>(null)
const showDiagnostics = ref(false)
const showModelMenu = ref(false)
const showAgentMenu = ref(false)
const showThreadMenu = ref(false)
const currentView = ref<'chat' | 'agents' | 'settings'>('chat')
const taskTitle = ref('')
const taskPrompt = ref('')
const taskWorkflow = ref<'delivery' | 'parallel'>('delivery')
const taskWorkspaceMode = ref<'current' | 'new-branch' | 'existing-branch'>('new-branch')
const taskBaseBranch = ref('')
const taskNewBranch = ref('')
const taskExistingBranch = ref('')
const taskApprovalMode = ref<'manual' | 'auto'>('manual')
const taskQueueMerge = ref(false)
const taskMergeTarget = ref('')
const taskParentId = ref('')
const taskDetailTabs = ['evidence', 'changes', 'activity'] as const
const selectedRoles = ref(['planner', 'implementer', 'reviewer'])
const roleProviders = ref<Record<string, string>>({})
const memoryTitle = ref('')
const memoryContent = ref('')
const coordinationTaskId = ref('')
const coordinationDraft = ref('')
const selectedTaskId = ref('')
const selectedArtifactId = ref('')
const selectedTaskTab = ref<'evidence' | 'changes' | 'activity'>('evidence')
const selectedChangePath = ref('')
const objectiveExpanded = ref(false)
const rollbackTaskId = ref('')
const rollbackConfirmation = ref('')
const setupProvider = ref<AgentProvider | null>(null)
const consoleProvider = ref<AgentProvider | null>(null)
const consoleElement = ref<HTMLElement | null>(null)
const consoleSessionId = ref('')
const consoleStatus = ref('Starting…')
const planFeedback = ref('')
const userAnswers = ref<Record<string, string[]>>({})
const elicitationValues = ref<Record<string, unknown>>({})
let fileSearchTimer: number | undefined
let providerTerminal: Terminal | null = null
let terminalFit: FitAddon | null = null
let consoleResizeObserver: { observe(element: HTMLElement): void; disconnect(): void } | null = null
let unsubscribeConsole: (() => void) | null = null

const connected = computed(() => diagnostics.status?.state === 'ready')
const needsLogin = computed(
  () => connected.value && workspace.account !== null && !workspace.account.signedIn,
)
const statusText = computed(() => diagnostics.status?.state?.replace(/_/g, ' ') ?? 'starting')
onMounted(async () => {
  unsubscribeConsole = subscribe('provider:console', (payload) => {
    const event = payload as ProviderConsoleEvent
    if (
      !consoleSessionId.value &&
      consoleProvider.value?.id === event.providerId &&
      event.sessionId
    ) {
      consoleSessionId.value = event.sessionId
    }
    if (event.sessionId !== consoleSessionId.value) return
    if (event.data) providerTerminal?.write(event.data)
    if (event.exitCode !== undefined) {
      consoleStatus.value = event.exitCode === 0 ? 'Session finished' : `Exited (${event.exitCode})`
      if (event.error) providerTerminal?.writeln(`\r\n\x1b[31m${event.error}\x1b[0m`)
      globalThis.setTimeout(() => {
        void workspace.loadProviders()
        void workspace.loadProviderUsage()
      }, 500)
    }
  })
  diagnostics.connectEvents()
  await diagnostics.load()
  if (connected.value) await workspace.initialize()
})

onBeforeUnmount(() => {
  if (fileSearchTimer) globalThis.clearTimeout(fileSearchTimer)
  unsubscribeConsole?.()
  closeProviderConsole()
  diagnostics.disconnectEvents()
  workspace.disconnectEvents()
})

watch(
  () => diagnostics.status?.state,
  async (state, previous) => {
    if (state === 'ready' && previous !== 'ready') await workspace.initialize()
  },
)

watch(
  () => workspace.selectedProvider,
  () => {
    showAgentMenu.value = false
    showModelMenu.value = false
  },
)

watch(
  () => workspace.activeThread?.id,
  () => {
    showThreadMenu.value = false
    showAgentMenu.value = false
    showModelMenu.value = false
  },
)

watch(
  () =>
    workspace.projectBranches
      .map(
        (branch) =>
          `${branch.name}:${branch.current}:${branch.remote}:${branch.checkedOut}:${branch.worktree ?? ''}`,
      )
      .join('|'),
  () => {
    const current =
      workspace.projectBranches.find((branch) => branch.current)?.name ??
      workspace.projectBranches[0]?.name ??
      ''
    if (!taskBaseBranch.value || !workspace.projectBranches.some((branch) => branch.name === taskBaseBranch.value)) {
      taskBaseBranch.value = current
    }
    if (!taskMergeTarget.value || !workspace.projectBranches.some((branch) => branch.name === taskMergeTarget.value)) {
      taskMergeTarget.value = current
    }
    const availableExisting = workspace.projectBranches.find(
      (branch) => !branch.remote && !branch.checkedOut,
    )
    if (
      !taskExistingBranch.value ||
      !workspace.projectBranches.some(
        (branch) =>
          branch.name === taskExistingBranch.value && !branch.remote && !branch.checkedOut,
      )
    ) {
      taskExistingBranch.value = availableExisting?.name ?? ''
    }
    if (!workspace.activeProject?.isGitRepository) {
      taskWorkspaceMode.value = 'current'
      taskQueueMerge.value = false
    }
  },
  { immediate: true },
)

watch(taskWorkspaceMode, (mode) => {
  if (mode === 'current') {
    taskQueueMerge.value = false
    taskMergeTarget.value = ''
  } else if (!taskMergeTarget.value) {
    taskMergeTarget.value = currentProjectBranch.value
  }
})

watch(
  () => workspace.items.map((item) => `${item.id}:${item.text?.length ?? 0}`).join('|'),
  async () => {
    await nextTick()
    conversation.value?.scrollTo({ top: conversation.value.scrollHeight, behavior: 'smooth' })
  },
)

async function submit() {
  const text = prompt.value
  if (!text.trim()) return
  prompt.value = ''
  await workspace.send(text)
}

async function selectWorkspaceAgent(providerId: string) {
  showAgentMenu.value = false
  showModelMenu.value = false
  await workspace.selectProvider(providerId)
}

function clearActiveThread() {
  workspace.items = []
  workspace.queue = []
  workspace.planArtifact = null
  showThreadMenu.value = false
}

function handlePromptInput() {
  const match = prompt.value.match(/(?:^|\s)@([^\s@]*)$/)
  if (!match) {
    workspace.fileMatches = []
    return
  }
  if (fileSearchTimer) globalThis.clearTimeout(fileSearchTimer)
  fileSearchTimer = globalThis.setTimeout(() => void workspace.searchFiles(match[1] ?? ''), 120)
}

function selectFile(match: FileMatch) {
  prompt.value = prompt.value.replace(/(?:^|\s)@[^\s@]*$/, (value) => {
    const prefix = value.startsWith(' ') ? ' ' : ''
    return `${prefix}@${match.relativePath} `
  })
  workspace.addMention(match)
}

function messageSegments(text: string) {
  return text.split(/(`[^`\n]*[/.][^`\n]*`|@[A-Za-z0-9_./-]+)/g).map((value) => {
    const candidate = value.startsWith('@')
      ? value.slice(1)
      : value.startsWith('`')
        ? value.slice(1, -1)
        : ''
    const file = candidate && !candidate.includes('://') ? candidate.replace(/^\.\//, '') : ''
    return { value, file }
  })
}

function openLinkedFile(path: string) {
  if (workspace.activeProject) void backend.openProjectFile(workspace.activeProject.id, path)
}

function linkedAddress(path: string) {
  return workspace.activeProject ? `${workspace.activeProject.path}/${path}` : path
}

function formatReset(timestamp?: number) {
  if (!timestamp) return 'Not reported'
  return new Date(timestamp * 1000).toLocaleString()
}

async function requestPlanRevision() {
  await workspace.revisePlan(planFeedback.value)
  planFeedback.value = ''
}

function chooseAnswer(questionId: string, value: string) {
  userAnswers.value[questionId] = [value]
}

async function submitUserInput() {
  await workspace.answerUserInput(userAnswers.value)
  userAnswers.value = {}
}

async function submitElicitation() {
  await workspace.answerElicitation('accept', elicitationValues.value)
  elicitationValues.value = {}
}

async function openMissionControl() {
  currentView.value = 'agents'
  await workspace.loadAgentWorkspace()
}

function toggleRole(role: string) {
  if (role === 'planner') return
  selectedRoles.value = selectedRoles.value.includes(role)
    ? selectedRoles.value.filter((candidate) => candidate !== role)
    : [...selectedRoles.value, role]
}

async function createAgentTask() {
  if (!taskPrompt.value.trim()) return
  const branch =
    taskWorkspaceMode.value === 'current'
      ? currentProjectBranch.value
      : taskWorkspaceMode.value === 'existing-branch'
        ? taskExistingBranch.value
        : taskNewBranch.value
  const created = await workspace.createAgentTask({
    title: taskTitle.value,
    prompt: taskPrompt.value,
    workflow: taskWorkflow.value,
    roles: selectedRoles.value,
    providers: Object.fromEntries(
      selectedRoles.value.map((role) => [role, roleProviders.value[role] || 'auto']),
    ),
    workspaceMode: workspace.activeProject?.isGitRepository ? taskWorkspaceMode.value : 'current',
    baseBranch:
      taskWorkspaceMode.value === 'existing-branch'
        ? taskExistingBranch.value
        : taskWorkspaceMode.value === 'current'
          ? currentProjectBranch.value
          : taskBaseBranch.value,
    taskBranch: branch,
    approvalMode: taskApprovalMode.value,
    queueMerge: taskQueueMerge.value,
    mergeTarget: taskMergeTarget.value,
    parentTaskId: taskParentId.value,
  })
  if (!created) return
  taskTitle.value = ''
  taskPrompt.value = ''
  taskNewBranch.value = ''
}

async function openRunThread(threadId?: string) {
  if (!threadId || !workspace.activeProject) return
  await workspace.loadThreads(workspace.activeProject.id)
  const thread = workspace.threads.find((candidate) => candidate.id === threadId)
  if (thread) {
    await workspace.selectThread(thread)
    currentView.value = 'chat'
  }
}

async function cancelSelectedTask() {
  const task = selectedTask.value?.task
  if (!task || !globalThis.confirm('Cancel this workflow and stop its active agents?')) return
  await workspace.cancelTask(task.id)
}

async function restartSelectedTask() {
  const task = selectedTask.value?.task
  if (
    !task ||
    !globalThis.confirm(
      'Restart this workflow from the first stage? Existing project files will be preserved.',
    )
  )
    return
  await workspace.restartTask(task.id)
}

async function openRollback(taskId: string) {
  rollbackTaskId.value = taskId
  rollbackConfirmation.value = ''
  workspace.rollbackPreview = null
  await workspace.previewRollback(taskId)
}

async function executeRollback() {
  const task = workspace.agentTasks.find((bundle) => bundle.task.id === rollbackTaskId.value)?.task
  if (!task) return
  await workspace.executeRollback(task.id, rollbackConfirmation.value)
  rollbackTaskId.value = ''
}

async function proposeMemory() {
  await workspace.proposeMemory('constraint', memoryTitle.value, memoryContent.value)
  memoryTitle.value = ''
  memoryContent.value = ''
}

async function openCoordination(taskId: string) {
  selectedTaskId.value = ''
  coordinationTaskId.value = taskId
  await workspace.loadCoordination(taskId)
}

async function postCoordination() {
  await workspace.postCoordination(coordinationTaskId.value, coordinationDraft.value)
  coordinationDraft.value = ''
}

async function providerAction(provider: AgentProvider) {
  if (!provider.installed) {
    setupProvider.value = provider
    return
  }
  if (provider.id === 'codex') {
    await workspace.manageProviderLogin(provider.id)
    return
  }
  await openProviderConsole(provider)
}

async function openProviderConsole(provider: AgentProvider) {
  consoleProvider.value = provider
  consoleStatus.value = 'Starting…'
  await nextTick()
  if (!consoleElement.value) return
  providerTerminal = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontFamily: "'JetBrains Mono', 'Cascadia Code', monospace",
    fontSize: 13,
    theme: {
      background: '#0b0f14',
      foreground: '#dce4ed',
      cursor: '#72e6bd',
      selectionBackground: '#285747',
    },
  })
  terminalFit = new FitAddon()
  providerTerminal.loadAddon(terminalFit)
  providerTerminal.open(consoleElement.value)
  terminalFit.fit()
  try {
    const session = await backend.startProviderConsole(
      provider.id,
      provider.authStatus === 'authenticated' ? 'manage' : 'login',
      '',
      providerTerminal.cols,
      providerTerminal.rows,
    )
    consoleSessionId.value = session.id
    consoleStatus.value = 'Interactive session'
    providerTerminal.onData((data) => {
      if (consoleSessionId.value) void backend.writeProviderConsole(consoleSessionId.value, data)
    })
    consoleResizeObserver = new globalThis.ResizeObserver(() => {
      terminalFit?.fit()
      if (consoleSessionId.value && providerTerminal) {
        void backend.resizeProviderConsole(
          consoleSessionId.value,
          providerTerminal.cols,
          providerTerminal.rows,
        )
      }
    })
    consoleResizeObserver.observe(consoleElement.value)
    providerTerminal.focus()
  } catch (error) {
    consoleStatus.value = 'Could not start'
    providerTerminal.writeln(
      `\x1b[31m${error instanceof Error ? error.message : String(error)}\x1b[0m`,
    )
  }
}

function closeProviderConsole() {
  const sessionId = consoleSessionId.value
  consoleSessionId.value = ''
  if (sessionId) void backend.stopProviderConsole(sessionId).catch(() => undefined)
  consoleResizeObserver?.disconnect()
  consoleResizeObserver = null
  providerTerminal?.dispose()
  providerTerminal = null
  terminalFit = null
  consoleProvider.value = null
}

function providerInstallCommand(providerId: string) {
  switch (providerId) {
    case 'claude':
      return 'npm install -g @anthropic-ai/claude-code'
    case 'gemini':
      return 'npm install -g @google/gemini-cli'
    case 'opencode':
      return 'Visit https://opencode.ai/docs for the current Linux installer'
    case 'ollama':
      return 'Visit https://ollama.com/download/linux'
    case 'antigravity':
      return 'Install Antigravity using Google’s official distribution'
    default:
      return 'Install the provider CLI and refresh Mission Control'
  }
}

const coordinationTask = computed(() =>
  workspace.agentTasks.find((bundle) => bundle.task.id === coordinationTaskId.value),
)

const selectedTask = computed(() =>
  workspace.agentTasks.find((bundle) => bundle.task.id === selectedTaskId.value),
)

const selectedTaskArtifacts = computed(() => {
  const artifacts = selectedTask.value?.artifacts ?? []
  const singletonTypes = new Set(['verification', 'snapshot', 'preflight', 'plan'])
  const seen = new Set<string>()
  return artifacts.filter((artifact) => {
    if (!singletonTypes.has(artifact.type)) return true
    if (seen.has(artifact.type)) return false
    seen.add(artifact.type)
    return true
  })
})

const selectedArtifact = computed(() => {
  const artifacts = selectedTaskArtifacts.value
  return (
    artifacts.find((artifact) => artifact.id === selectedArtifactId.value) ??
    artifacts.find((artifact) => artifact.type === 'plan') ??
    artifacts[0]
  )
})

const selectedTaskChanges = computed(() =>
  selectedTask.value ? workspace.taskChangesByTask[selectedTask.value.task.id] : undefined,
)

const selectedChange = computed(() => {
  const changes = selectedTaskChanges.value?.changes ?? []
  return changes.find((change) => `${change.workspace}:${change.path}` === selectedChangePath.value) ?? changes[0]
})

const selectedTaskActivity = computed(() => {
  const bundle = selectedTask.value
  if (!bundle) return []
  const events: Array<{ id: string; title: string; detail: string; timestamp: string; status: string }> = [
    {
      id: `created-${bundle.task.id}`,
      title: 'Workflow created',
      detail: `Manual delegation from Mission Control · ${bundle.task.mode}`,
      timestamp: bundle.task.createdAt,
      status: 'created',
    },
  ]
  for (const run of bundle.runs) {
    if (run.startedAt) {
      events.push({
        id: `started-${run.id}`,
        title: `${run.role} started`,
        detail: `${run.providerId} · ${run.summary || 'Agent process started'}`,
        timestamp: run.startedAt,
        status: 'running',
      })
    }
    if (run.finishedAt) {
      events.push({
        id: `finished-${run.id}`,
        title: `${run.role} ${run.status}`,
        detail: `${run.providerId} · ${run.summary || run.status}`,
        timestamp: run.finishedAt,
        status: run.status,
      })
    }
  }
  for (const message of workspace.coordinationByTask[bundle.task.id] ?? []) {
    events.push({
      id: message.id,
      title: `${message.role || 'agent'} · ${message.kind}`,
      detail: message.message,
      timestamp: message.createdAt,
      status: message.kind,
    })
  }
  return events.sort(
    (left, right) => new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime(),
  )
})

const rollbackRootTask = computed(
  () => workspace.agentTasks.find((bundle) => bundle.task.id === rollbackTaskId.value)?.task,
)

const currentProjectBranch = computed(
  () =>
    workspace.projectBranches.find((branch) => branch.current)?.name ??
    workspace.activeProject?.defaultBranch ??
    'current branch',
)

const isolatedTaskCount = computed(
  () => workspace.agentTasks.filter((bundle) => Boolean(bundle.task.worktree)).length,
)

const queuedMergeCount = computed(
  () => workspace.agentTasks.filter((bundle) => bundle.task.queueMerge).length,
)

const baseBranchOptions = computed(() => workspace.projectBranches)

const existingBranchOptions = computed(() =>
  workspace.projectBranches.filter((branch) => !branch.remote),
)

const taskGitConfigurationValid = computed(() => {
  if (!workspace.activeProject?.isGitRepository) return true
  if (taskWorkspaceMode.value === 'current') return Boolean(currentProjectBranch.value)
  if (taskWorkspaceMode.value === 'existing-branch') {
    const branch = existingBranchOptions.value.find(
      (candidate) => candidate.name === taskExistingBranch.value,
    )
    return Boolean(branch && !branch.checkedOut)
  }
  return Boolean(taskBaseBranch.value)
})

const suggestedTaskBranch = computed(() => {
  const source = (taskTitle.value || taskPrompt.value || 'task').toLocaleLowerCase('en-US')
  const slug = source
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 42)
  return `agent/${slug || 'task'}`
})

function resolvedTaskBaseBranch(task: { baseBranch?: string }) {
  return task.baseBranch || currentProjectBranch.value
}

function taskWorkingBranch(task: { taskBranch?: string; workspaceMode: string }) {
  if (task.taskBranch) return task.taskBranch
  return task.workspaceMode === 'current' ? currentProjectBranch.value : 'pending branch'
}

function taskWorkspaceLabel(task: { workspaceMode: string; worktree?: string }) {
  if (task.worktree) return 'Isolated worktree'
  if (task.workspaceMode === 'existing-branch') return 'Existing branch'
  if (task.workspaceMode === 'new-branch') return 'New isolated branch'
  return 'Shared workspace'
}

function openTaskDetail(taskId: string) {
  selectedTaskId.value = taskId
  selectedTaskTab.value = 'evidence'
  selectedChangePath.value = ''
  objectiveExpanded.value = false
  const bundle = workspace.agentTasks.find((candidate) => candidate.task.id === taskId)
  selectedArtifactId.value =
    bundle?.artifacts.find((artifact) => artifact.type === 'plan')?.id ??
    bundle?.artifacts[0]?.id ??
    ''
  void workspace.loadCoordination(taskId)
}

function selectTaskTab(tab: 'evidence' | 'changes' | 'activity') {
  selectedTaskTab.value = tab
  if (tab === 'changes' && selectedTask.value) {
    void workspace.loadTaskChanges(selectedTask.value.task.id)
  }
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function readableArtifact(content: string) {
  try {
    return JSON.stringify(JSON.parse(content), null, 2)
  } catch {
    return content
  }
}

const activeRuns = computed(() =>
  workspace.agentTasks.flatMap((bundle) => bundle.runs).filter((run) => run.status === 'running'),
)
const waitingRuns = computed(() =>
  workspace.agentTasks
    .flatMap((bundle) => bundle.runs)
    .filter((run) => run.status === 'queued' || run.status === 'blocked'),
)
const failedRuns = computed(() =>
  workspace.agentTasks
    .flatMap((bundle) => bundle.runs)
    .filter(
      (run) =>
        run.status === 'failed' ||
        run.status === 'unavailable' ||
        run.status === 'interrupted',
    ),
)

function itemTitle(item: ConversationItem): string {
  switch (item.type) {
    case 'agentMessage':
      return (
        workspace.agentProviders.find((provider) => provider.id === item.providerId)?.name ||
        workspace.agentProviders.find((provider) => provider.id === workspace.selectedProvider)
          ?.name ||
        'Agent'
      )
    case 'userMessage':
      return 'You'
    case 'commandExecution':
      return 'Command'
    case 'fileChange':
      return 'File changes'
    case 'reasoning':
      return 'Reasoning'
    default:
      return item.type
  }
}

function itemText(item: ConversationItem): string {
  if (item.text) return item.text
  if (item.type === 'userMessage' && Array.isArray(item.content)) {
    return (item.content as Array<{ type: string; text?: string }>)
      .filter((content) => content.type === 'text')
      .map((content) => content.text ?? '')
      .join('\n')
  }
  if (item.type === 'commandExecution') {
    const output = item.aggregatedOutput ? `\n\n${item.aggregatedOutput}` : ''
    return `${item.command ?? ''}${output}`
  }
  if (item.type === 'fileChange') {
    return `${item.changes?.length ?? 0} file change(s) · ${item.status ?? 'in progress'}`
  }
  if (Array.isArray(item.summary)) return (item.summary as string[]).join('\n')
  return ''
}

function backendOpen(path: string) {
  void backend.openInFileManager(path)
}
</script>

<template>
  <main class="app-shell">
    <header class="app-header">
      <div class="brand">
        <span class="brand-mark">C</span>
        <div>
          <strong>Agent Workspace</strong>
          <span>{{ workspace.activeProject?.path ?? 'Choose a local project' }}</span>
        </div>
      </div>
      <nav v-if="connected" class="view-switcher">
        <button
          type="button"
          :class="{ active: currentView === 'chat' }"
          @click="currentView = 'chat'"
        >
          Workspace
        </button>
        <button
          type="button"
          :class="{ active: currentView === 'agents' }"
          @click="openMissionControl"
        >
          Mission Control
        </button>
        <button
          type="button"
          :class="{ active: currentView === 'settings' }"
          @click="currentView = 'settings'; workspace.loadProviders(); workspace.loadProviderUsage()"
        >
          Settings
        </button>
      </nav>
      <div class="header-actions">
        <button class="connection-chip" type="button" @click="showDiagnostics = !showDiagnostics">
          <span class="status-dot" :data-ready="connected"></span>
          {{ statusText }}
        </button>
      </div>
    </header>

    <section v-if="!connected" class="center-state">
      <p class="eyebrow">Codex runtime</p>
      <h1>{{ diagnostics.status?.lastError?.summary ?? 'Connecting to app-server…' }}</h1>
      <p>{{ diagnostics.status?.lastError?.detail ?? 'Preparing the local Codex session.' }}</p>
      <button class="primary-button" type="button" @click="diagnostics.retry">
        Retry connection
      </button>
    </section>

    <section v-else-if="currentView === 'settings'" class="settings-view">
      <header class="settings-heading">
        <div>
          <p class="eyebrow">Agent accounts</p>
          <h1>Providers & authentication</h1>
          <span>Connect local coding-agent CLIs once, then choose them directly in Workspace.</span>
        </div>
        <button class="secondary-button" type="button" @click="workspace.loadProviders">
          Refresh detection
        </button>
      </header>
      <div class="settings-provider-grid">
        <article v-for="provider in workspace.agentProviders" :key="provider.id">
          <span class="provider-mark">{{ provider.name.slice(0, 1) }}</span>
          <div>
            <h3>{{ provider.name }}</h3>
            <p>{{ provider.version || provider.status }}</p>
            <small :data-auth="provider.authStatus">{{ provider.authStatus }}</small>
          </div>
          <div class="settings-provider-actions">
            <button
              class="primary-button"
              type="button"
              :disabled="provider.authStatus === 'not-required'"
              @click="providerAction(provider)"
            >
              {{ !provider.installed ? 'Set up' : provider.authStatus === 'authenticated' ? 'Manage account' : 'Log in' }}
            </button>
            <button
              v-if="provider.id === 'codex' && provider.authStatus === 'authenticated'"
              class="secondary-button"
              type="button"
              @click="workspace.logout(); workspace.loadProviders()"
            >
              Sign out
            </button>
          </div>
        </article>
      </div>
      <div v-if="workspace.deviceLogin" class="device-code">
        <span>Enter this code in the browser</span>
        <strong>{{ workspace.deviceLogin.userCode }}</strong>
        <small>{{ workspace.deviceLogin.verificationUrl }}</small>
      </div>
    </section>

    <section v-else-if="currentView === 'chat'" class="workspace">
      <aside class="sidebar">
        <div class="sidebar-heading">
          <span>Projects</span>
          <button type="button" title="Add project" @click="workspace.addProject">+</button>
        </div>
        <div class="project-list">
          <div v-for="project in workspace.projects" :key="project.id" class="project-group">
            <div
              class="project-row"
              :class="{ active: project.id === workspace.activeProject?.id }"
            >
              <button
                class="collapse-button"
                type="button"
                @click="workspace.toggleProject(project)"
              >
                {{ workspace.expandedProjects[project.id] ? '⌄' : '›' }}
              </button>
              <button class="project-main" type="button" @click="workspace.selectProject(project)">
                <span class="project-icon">{{ project.name.slice(0, 1).toUpperCase() }}</span>
                <span>
                  <strong>{{ project.name }}</strong>
                  <small>{{
                    project.defaultBranch || (project.isGitRepository ? 'Git repository' : 'Folder')
                  }}</small>
                </span>
              </button>
              <button
                class="project-action"
                type="button"
                title="New thread"
                @click="workspace.newThread(project)"
              >
                +
              </button>
            </div>
            <div v-if="workspace.expandedProjects[project.id]" class="nested-threads">
              <button
                v-for="thread in workspace.threadsByProject[project.id] ?? []"
                :key="thread.id"
                class="thread-row"
                :class="{ active: thread.id === workspace.activeThread?.id }"
                type="button"
                @click="workspace.selectThread(thread)"
              >
                <strong>{{ thread.title || 'New thread' }}</strong>
                <small>
                  {{ thread.providerId === 'mixed' ? 'Multi-agent' : workspace.agentProviders.find((provider) => provider.id === thread.providerId)?.name || thread.providerId }}
                  · {{ new Date(thread.updatedAt).toLocaleDateString() }}
                </small>
              </button>
              <button class="import-thread" type="button" @click="workspace.importThread(project)">
                Import chat…
              </button>
            </div>
          </div>
        </div>

        <button
          v-if="workspace.activeProject"
          class="open-folder"
          type="button"
          @click="backendOpen(workspace.activeProject.path)"
        >
          Open project folder
        </button>
      </aside>

      <section class="conversation-pane">
        <div v-if="!workspace.activeProject" class="pane-empty">
          <div class="empty-icon">+</div>
          <h2>Add your first project</h2>
          <p>Select a local repository or directory where your coding agents should work.</p>
          <button class="primary-button" type="button" @click="workspace.addProject">
            Add project
          </button>
        </div>
        <div v-else-if="!workspace.activeThread" class="pane-empty">
          <div class="empty-icon">✦</div>
          <h2>Start an agent thread</h2>
          <p>
            Choose a connected coding agent and work directly inside
            {{ workspace.activeProject.name }}.
          </p>
          <button class="primary-button" type="button" @click="workspace.newThread()">
            New thread
          </button>
        </div>
        <template v-else>
          <div class="conversation-title">
            <div>
              <p class="eyebrow">Thread</p>
              <h2>{{ workspace.activeThread.title }}</h2>
            </div>
            <div class="title-actions">
              <span v-if="workspace.activeTurn" class="working-label">
                {{ workspace.agentProviders.find((provider) => provider.id === workspace.selectedProvider)?.name || 'Agent' }} is working…
              </span>
              <div class="thread-more">
                <button
                  class="thread-more-trigger"
                  type="button"
                  aria-label="Thread actions"
                  @click="showThreadMenu = !showThreadMenu"
                >
                  •••
                </button>
                <div v-if="showThreadMenu" class="thread-menu">
                  <button
                    type="button"
                    @click="workspace.exportActiveThread(); showThreadMenu = false"
                  >
                    <span>Export chat</span><small>JSON</small>
                  </button>
                  <button type="button" @click="clearActiveThread">
                    <span>Clear view</span><small>Local</small>
                  </button>
                  <button type="button" disabled>
                    <span>Share thread</span><small class="coming-soon">◷ Coming soon</small>
                  </button>
                </div>
              </div>
            </div>
          </div>
          <div ref="conversation" class="conversation">
            <article
              v-for="item in workspace.items"
              :key="item.id"
              class="message"
              :data-type="item.type"
            >
              <header>
                <strong>{{ itemTitle(item) }}</strong>
                <span v-if="item.status">{{ item.status }}</span>
              </header>
              <pre><template v-for="(segment, index) in messageSegments(itemText(item))" :key="index"><button
                v-if="segment.file"
                class="file-link"
                type="button"
                :data-tooltip="linkedAddress(segment.file)"
                :title="linkedAddress(segment.file)"
                @click="openLinkedFile(segment.file)"
              >{{ segment.value }}</button><template v-else>{{ segment.value }}</template></template></pre>
            </article>
            <div v-if="!workspace.items.length" class="conversation-welcome">
              <span>✦</span>
              <h3>What should your agent do?</h3>
              <p>Ask about the codebase, request a change, or run tests.</p>
            </div>
          </div>
          <form class="composer" @submit.prevent="submit">
            <textarea
              v-model="prompt"
              rows="3"
              :placeholder="`Message ${workspace.agentProviders.find((provider) => provider.id === workspace.selectedProvider)?.name || 'agent'}…`"
              @input="handlePromptInput"
              @keydown.enter.exact.prevent="submit"
            ></textarea>
            <div v-if="workspace.fileMatches.length" class="file-picker">
              <button
                v-for="match in workspace.fileMatches"
                :key="match.path"
                type="button"
                @click="selectFile(match)"
              >
                <strong>{{ match.name }}</strong><small>{{ match.relativePath }}</small>
              </button>
            </div>
            <div v-if="workspace.mentions.length" class="mention-list">
              <button
                v-for="mention in workspace.mentions"
                :key="mention.path"
                type="button"
                @click="workspace.removeMention(mention.path)"
              >
                @{{ mention.name }} ×
              </button>
            </div>
            <div class="turn-controls">
              <div class="agent-control">
                <button
                  class="agent-trigger"
                  type="button"
                  :disabled="!!workspace.activeTurn || !workspace.availableProviders.length"
                  @click="showAgentMenu = !showAgentMenu; showModelMenu = false"
                >
                  <span
                    class="agent-avatar"
                    :data-provider="workspace.selectedProvider"
                  >{{ workspace.agentProviders.find((provider) => provider.id === workspace.selectedProvider)?.name.slice(0, 1) || '?' }}</span>
                  <span>
                    <small>Agent</small>
                    <strong>{{ workspace.agentProviders.find((provider) => provider.id === workspace.selectedProvider)?.name || 'Connect an agent' }}</strong>
                  </span>
                  <b>⌄</b>
                </button>
                <div v-if="showAgentMenu" class="agent-menu">
                  <button
                    v-for="provider in workspace.availableProviders"
                    :key="provider.id"
                    type="button"
                    :class="{ active: provider.id === workspace.selectedProvider }"
                    @click="selectWorkspaceAgent(provider.id)"
                  >
                    <span class="agent-avatar" :data-provider="provider.id">{{
                      provider.name.slice(0, 1)
                    }}</span>
                    <span>
                      <strong>{{ provider.name }}</strong>
                      <small>{{ provider.version || 'Connected' }}</small>
                    </span>
                    <i></i>
                  </button>
                </div>
              </div>
              <div class="model-control">
                <button
                  class="control-trigger"
                  type="button"
                  @click="showModelMenu = !showModelMenu; showAgentMenu = false"
                >
                  <span>Model</span><strong>{{ workspace.providerModels.find((model) => model.id === workspace.selectedModel)?.name || 'Default' }}</strong><b>⌄</b>
                </button>
                <div v-if="showModelMenu" class="model-menu">
                  <button
                    v-for="model in workspace.providerModels"
                    :key="model.id"
                    type="button"
                    :class="{ active: model.id === workspace.selectedModel }"
                    @click="
                      workspace.selectedModel = model.id;
                      showModelMenu = false
                    "
                  >
                    <strong>{{ model.name }}</strong><small>{{ model.providerId }}</small>
                  </button>
                </div>
              </div>
              <div v-if="workspace.selectedProvider === 'codex'" class="segmented-control" aria-label="Reasoning effort">
                <button
                  v-for="effort in workspace.supportedEfforts"
                  :key="effort"
                  type="button"
                  :class="{ active: effort === workspace.selectedEffort }"
                  @click="workspace.selectedEffort = effort"
                >
                  {{ effort }}
                </button>
              </div>
              <div class="segmented-control mode-control" aria-label="Workflow mode">
                <button
                  type="button"
                  :class="{ active: workspace.selectedMode === 'default' }"
                  @click="workspace.selectedMode = 'default'"
                >
                  Fast
                </button>
                <button
                  type="button"
                  :class="{ active: workspace.selectedMode === 'plan' }"
                  @click="workspace.selectedMode = 'plan'"
                >
                  Plan
                </button>
              </div>
            </div>
            <div class="composer-footer">
              <span>
                Enter to send · Shift+Enter for newline
                <strong v-if="workspace.queue.length" class="queue-count">
                  · {{ workspace.queue.length }} queued
                </strong>
              </span>
              <div class="composer-actions">
                <button
                  v-if="workspace.activeTurn"
                  class="stop-button"
                  type="button"
                  @click="workspace.interrupt"
                >
                  Stop
                </button>
                <button class="send-button" type="submit" :disabled="!prompt.trim()">
                  {{ workspace.activeTurn || workspace.sending ? 'Queue' : 'Send' }}
                </button>
              </div>
            </div>
          </form>
        </template>
      </section>

      <aside class="changes-pane">
        <div class="changes-heading">
          <div>
            <p class="eyebrow">Current turn</p>
            <h2>Changes</h2>
          </div>
          <span>{{ workspace.diff ? 'Updated' : 'Clean' }}</span>
        </div>
        <pre v-if="workspace.diff" class="diff">{{ workspace.diff }}</pre>
        <div v-else class="changes-empty">
          <span>±</span>
          <p>File changes and the unified diff will appear here.</p>
        </div>
      </aside>
    </section>

    <section v-else class="mission-control">
      <header class="mission-header">
        <div>
          <p class="eyebrow">Provider-independent orchestration</p>
          <h1>Agent Mission Control</h1>
          <span>Run parallel delivery pipelines, review evidence, and resolve blocked work.</span>
        </div>
        <button class="secondary-button" type="button" @click="workspace.loadAgentWorkspace">
          Refresh
        </button>
      </header>

      <div class="mission-stats">
        <article>
          <span>Active agents</span><strong>{{ activeRuns.length }}</strong>
        </article>
        <article>
          <span>Waiting</span><strong>{{ waitingRuns.length }}</strong>
        </article>
        <article>
          <span>Failed / unavailable</span><strong>{{ failedRuns.length }}</strong>
        </article>
        <article>
          <span>Tasks</span><strong>{{ workspace.agentTasks.length }}</strong>
        </article>
      </div>

      <section v-if="workspace.activeProject?.isGitRepository" class="git-workspace-board">
        <header>
          <div>
            <p class="eyebrow">Git workspaces</p>
            <h2>Branch lanes</h2>
            <span>Each isolated task keeps its branch checked out without switching the main project.</span>
          </div>
          <div class="git-workspace-summary">
            <span><b>Current</b>{{ currentProjectBranch }}</span>
            <span><b>Branches</b>{{ workspace.projectBranches.length }}</span>
            <span><b>Worktrees</b>{{ isolatedTaskCount }}</span>
            <span><b>Merge queue</b>{{ queuedMergeCount }}</span>
          </div>
        </header>
        <div class="git-lane-list">
          <button
            v-for="bundle in workspace.agentTasks"
            :key="`git-${bundle.task.id}`"
            type="button"
            @click="openTaskDetail(bundle.task.id)"
          >
            <span class="git-lane-mode" :data-isolated="Boolean(bundle.task.worktree)">
              {{ bundle.task.worktree ? 'WT' : 'MAIN' }}
            </span>
            <span class="git-lane-task">
              <strong>{{ bundle.task.title }}</strong>
              <small>{{ taskWorkspaceLabel(bundle.task) }}</small>
            </span>
            <span class="git-branch-flow">
              <code>{{ resolvedTaskBaseBranch(bundle.task) }}</code>
              <i>→</i>
              <code class="working">{{ taskWorkingBranch(bundle.task) }}</code>
              <template v-if="bundle.task.mergeTarget">
                <i>→</i>
                <code>{{ bundle.task.mergeTarget }}</code>
              </template>
            </span>
            <span class="git-lane-state" :data-status="bundle.task.status">
              {{ bundle.task.status }}
            </span>
          </button>
          <div v-if="!workspace.agentTasks.length" class="mission-empty">
            Branch lanes appear here after you create the first task.
          </div>
        </div>
      </section>

      <section class="pipeline-board">
        <header>
          <div>
            <p class="eyebrow">Workflow control</p>
            <h2>Task pipelines</h2>
          </div>
          <span>Click a pipeline to inspect agent output, artifacts, worktrees, and evidence.</span>
        </header>
        <div class="pipeline-explainer">
          <span><b>1</b> Objective</span><i>→</i>
          <span><b>2</b> Agent plan</span><i>→</i>
          <span><b>3</b> Approval</span><i>→</i>
          <span><b>4</b> Implementation</span><i>→</i>
          <span><b>5</b> Review & evidence</span>
        </div>
        <div class="pipeline-list">
          <button
            v-for="bundle in workspace.agentTasks"
            :key="bundle.task.id"
            type="button"
            @click="openTaskDetail(bundle.task.id)"
          >
            <span class="pipeline-state" :data-status="bundle.task.status"></span>
            <span class="pipeline-copy">
              <strong>{{ bundle.task.title }}</strong>
              <small>{{ bundle.task.mode }} · {{ bundle.artifacts.length }} artifacts</small>
              <span v-if="workspace.activeProject?.isGitRepository" class="pipeline-git-meta">
                <b>{{ bundle.task.worktree ? 'worktree' : 'shared' }}</b>
                <code>{{ taskWorkingBranch(bundle.task) }}</code>
                <i v-if="bundle.task.mergeTarget">→ {{ bundle.task.mergeTarget }}</i>
              </span>
            </span>
            <span class="pipeline-stages">
              <span
                v-for="run in bundle.runs"
                :key="run.id"
                :data-status="run.status"
                :title="`${run.role} · ${run.providerId} · ${run.status}`"
              >
                <b>{{ run.role.slice(0, 1).toUpperCase() }}</b>
                <small>{{ run.status }}</small>
              </span>
            </span>
            <strong class="pipeline-result" :data-status="bundle.task.status">
              {{ bundle.task.status }}
            </strong>
          </button>
          <div v-if="!workspace.agentTasks.length" class="mission-empty">
            No workflows yet. Configure roles and launch a pipeline from the task builder.
          </div>
        </div>
      </section>

      <div class="mission-layout">
        <section class="mission-main">
          <div class="agent-table">
            <header>
              <span>Agent / role</span><span>Provider</span><span>Task</span><span>Status</span>
            </header>
            <template v-for="bundle in workspace.agentTasks" :key="bundle.task.id">
              <article
                v-for="run in bundle.runs"
                :key="run.id"
                class="clickable"
                @click="openTaskDetail(bundle.task.id)"
              >
                <div>
                  <strong>{{ run.role }}</strong><small>{{ run.summary || 'Waiting for activity' }}</small>
                </div>
                <span>{{
                  workspace.agentProviders.find((provider) => provider.id === run.providerId)
                    ?.name || run.providerId
                }}</span>
                <div>
                  <strong>{{ bundle.task.title }}</strong><small>{{ bundle.task.mode }}</small>
                </div>
                <div class="run-status" :data-status="run.status">
                  <span>{{ run.status }}</span>
                  <progress :value="run.progress" max="100"></progress>
                </div>
              </article>
            </template>
            <div v-if="!workspace.agentTasks.length" class="mission-empty">
              No agent tasks yet. Create the first workflow on the right.
            </div>
          </div>

          <div class="mission-inspectors">
            <article>
              <h3>Preflight</h3>
              <div v-for="check in workspace.preflight" :key="check.id">
                <i :data-status="check.status"></i>
                <span><strong>{{ check.label }}</strong><small>{{ check.detail }}</small></span>
              </div>
            </article>
            <article>
              <h3>Context Inspector</h3>
              <div v-for="entry in workspace.contextEntries" :key="entry.path">
                <i :data-status="entry.kind === 'included' ? 'passed' : 'warning'"></i>
                <span><strong>{{ entry.path }}</strong><small>{{ entry.reason }} · ~{{
                  entry.estimatedTokens.toLocaleString()
                }}
                  tokens</small></span>
              </div>
            </article>
            <article class="memory-inspector">
              <h3>Living Project Memory</h3>
              <div v-for="memory in workspace.projectMemory" :key="memory.id">
                <i data-status="warning"></i>
                <span><strong>{{ memory.title }}</strong><small>{{ memory.status }} · {{ memory.content }}</small></span>
                <span v-if="memory.status === 'proposed'" class="memory-actions">
                  <button type="button" @click="workspace.reviewMemory(memory.id, 'rejected')">
                    ×
                  </button>
                  <button type="button" @click="workspace.reviewMemory(memory.id, 'approved')">
                    ✓
                  </button>
                </span>
              </div>
              <input v-model="memoryTitle" type="text" placeholder="Constraint title" />
              <textarea v-model="memoryContent" rows="2" placeholder="Propose project knowledge…" />
              <button
                type="button"
                :disabled="!memoryTitle.trim() || !memoryContent.trim()"
                @click="proposeMemory"
              >
                Propose memory
              </button>
            </article>
          </div>
        </section>

        <aside class="task-builder">
          <header class="builder-header">
            <div>
              <p class="eyebrow">New workflow</p>
              <h2>Delegate a task</h2>
            </div>
            <span>{{ workspace.activeProject?.name || 'No project' }}</span>
          </header>

          <section class="builder-section">
            <div class="builder-section-title"><b>1</b><span>Outcome</span></div>
            <label>
              <span>Task title</span>
              <input v-model="taskTitle" type="text" placeholder="Short, recognizable title" />
            </label>
            <label>
              <span>Objective</span>
              <textarea
                v-model="taskPrompt"
                rows="9"
                placeholder="Describe the outcome, constraints, acceptance criteria, and verification…"
              />
            </label>
          </section>

          <section v-if="workspace.activeProject?.isGitRepository" class="builder-section">
            <div class="builder-section-title"><b>2</b><span>Git workspace</span></div>
            <div class="workspace-choice">
              <label
                v-for="option in [
                  { id: 'new-branch', title: 'New isolated branch', detail: 'Recommended · linked worktree' },
                  { id: 'existing-branch', title: 'Existing branch', detail: 'Open in a separate worktree' },
                  { id: 'current', title: 'Current workspace', detail: 'Shared files · no isolation' },
                ]"
                :key="option.id"
                :class="{ active: taskWorkspaceMode === option.id }"
              >
                <input
                  v-model="taskWorkspaceMode"
                  type="radio"
                  name="task-workspace"
                  :value="option.id"
                />
                <span><strong>{{ option.title }}</strong><small>{{ option.detail }}</small></span>
              </label>
            </div>
            <div v-if="taskWorkspaceMode === 'current'" class="branch-selection-summary warning">
              <span>
                <small>Working directly on</small>
                <code>{{ currentProjectBranch }}</code>
              </span>
              <p>
                Agents share the project directory. Parallel tasks and automatic merge tasks are
                disabled for this workspace mode.
              </p>
            </div>
            <div v-else class="builder-two-column">
              <label>
                <span>{{ taskWorkspaceMode === 'new-branch' ? 'Base branch' : 'Task branch' }}</span>
                <select
                  v-if="taskWorkspaceMode === 'new-branch'"
                  v-model="taskBaseBranch"
                >
                  <option v-for="branch in baseBranchOptions" :key="branch.name" :value="branch.name">
                    {{ branch.name }}{{ branch.current ? ' · current' : branch.remote ? ' · remote' : '' }}
                  </option>
                </select>
                <select v-else v-model="taskExistingBranch">
                  <option value="">Select branch…</option>
                  <option
                    v-for="branch in existingBranchOptions"
                    :key="branch.name"
                    :value="branch.name"
                    :disabled="branch.checkedOut"
                  >
                    {{ branch.name }}{{ branch.checkedOut ? ' · already checked out' : '' }}
                  </option>
                </select>
              </label>
              <label v-if="taskWorkspaceMode === 'new-branch'">
                <span>New branch name</span>
                <div class="branch-name-input">
                  <input
                    v-model="taskNewBranch"
                    type="text"
                    :placeholder="`${suggestedTaskBranch}-…`"
                    spellcheck="false"
                  />
                  <button
                    v-if="!taskNewBranch"
                    type="button"
                    @click="taskNewBranch = suggestedTaskBranch"
                  >
                    Use suggestion
                  </button>
                </div>
              </label>
            </div>
            <div v-if="taskWorkspaceMode !== 'current'" class="branch-selection-summary">
              <span>
                <small>{{ taskWorkspaceMode === 'new-branch' ? 'Create from' : 'Open branch' }}</small>
                <code>{{
                  taskWorkspaceMode === 'new-branch' ? taskBaseBranch : taskExistingBranch || 'Select a branch'
                }}</code>
              </span>
              <i>→</i>
              <span>
                <small>Task workspace</small>
                <code>{{
                  taskWorkspaceMode === 'new-branch'
                    ? taskNewBranch || `${suggestedTaskBranch}-…`
                    : 'New linked worktree'
                }}</code>
              </span>
            </div>
            <p
              v-if="taskWorkspaceMode === 'existing-branch' && !existingBranchOptions.some((branch) => !branch.checkedOut)"
              class="builder-inline-warning"
            >
              Every local branch is already checked out. Create a new isolated branch or use the
              current workspace.
            </p>
            <small class="builder-help">
              Linked worktrees keep task branches checked out simultaneously without switching the
              project’s main working directory.
            </small>
          </section>

          <section class="builder-section">
            <div class="builder-section-title">
              <b>{{ workspace.activeProject?.isGitRepository ? 3 : 2 }}</b><span>Execution policy</span>
            </div>
            <span class="builder-label">Workflow</span>
            <div class="segmented-control builder-segments">
              <button
                type="button"
                :class="{ active: taskWorkflow === 'delivery' }"
                @click="taskWorkflow = 'delivery'"
              >
                Delivery pipeline
              </button>
              <button
                type="button"
                :class="{ active: taskWorkflow === 'parallel' }"
                @click="taskWorkflow = 'parallel'"
              >
                Parallel jury
              </button>
            </div>
            <div class="workflow-behavior" :data-mode="taskWorkflow">
              <strong>{{
                taskWorkflow === 'delivery'
                  ? 'Code starts after plan approval'
                  : 'Selected agents start immediately'
              }}</strong>
              <p v-if="taskWorkflow === 'delivery'">
                Planner runs first without editing. Review its plan in the pipeline, then press
                “Approve next stage”. The implementer starts writing code only after that approval.
              </p>
              <p v-else>
                The planner runs first. After approval, remaining roles launch together and produce
                independent implementation or review evidence.
              </p>
            </div>
            <label class="builder-check">
              <input
                type="checkbox"
                :checked="taskApprovalMode === 'auto'"
                @change="taskApprovalMode = taskApprovalMode === 'auto' ? 'manual' : 'auto'"
              />
              <span>
                <strong>Auto-approve plan</strong>
                <small>Planner still runs first; implementation starts without waiting.</small>
              </span>
            </label>
          </section>

          <section class="builder-section">
            <div class="builder-section-title">
              <b>{{ workspace.activeProject?.isGitRepository ? 4 : 3 }}</b><span>Agent team</span>
            </div>
            <span class="builder-label">Agent roles</span>
            <div class="role-grid">
              <button
                v-for="role in ['planner', 'implementer', 'reviewer', 'security', 'browser-qa']"
                :key="role"
                type="button"
                :class="{ active: selectedRoles.includes(role) }"
                :disabled="role === 'planner'"
                @click="toggleRole(role)"
              >
                {{ role }}
              </button>
            </div>
            <div class="role-assignments">
              <label v-for="role in selectedRoles" :key="role">
                <span>{{ role }}</span>
                <select v-model="roleProviders[role]">
                  <option value="">Smart Router</option>
                  <option
                    v-for="provider in workspace.availableProviders"
                    :key="provider.id"
                    :value="provider.id"
                    :disabled="!provider.runnable"
                  >
                    {{ provider.name }}{{ provider.runnable ? '' : ` · ${provider.status}` }}
                  </option>
                </select>
              </label>
            </div>
          </section>

          <section class="builder-section">
            <div class="builder-section-title">
              <b>{{ workspace.activeProject?.isGitRepository ? 5 : 4 }}</b><span>Chain & delivery</span>
            </div>
            <label>
              <span>Depends on task</span>
              <select v-model="taskParentId">
                <option value="">No dependency</option>
                <option v-for="bundle in workspace.agentTasks" :key="bundle.task.id" :value="bundle.task.id">
                  {{ bundle.task.title }} · {{ bundle.task.status }}
                </option>
              </select>
            </label>
            <label
              v-if="workspace.activeProject?.isGitRepository && taskWorkspaceMode !== 'current'"
              class="builder-check"
            >
              <input v-model="taskQueueMerge" type="checkbox" />
              <span>
                <strong>Queue merge task when complete</strong>
                <small>Create an approval-gated merge step after verification.</small>
              </span>
            </label>
            <label
              v-if="
                taskQueueMerge &&
                workspace.activeProject?.isGitRepository &&
                taskWorkspaceMode !== 'current'
              "
            >
              <span>Merge target</span>
              <select v-model="taskMergeTarget">
                <option
                  v-for="branch in existingBranchOptions"
                  :key="branch.name"
                  :value="branch.name"
                >
                  {{ branch.name }}
                </option>
              </select>
            </label>
          </section>

          <p v-if="workspace.error" class="builder-error" role="alert">{{ workspace.error }}</p>
          <div class="router-note">
            <strong>Smart Router</strong>
            <p>
              Each role is assigned to the best installed provider. Missing providers are never
              simulated.
            </p>
          </div>
          <button
            class="primary-button create-workflow"
            type="button"
            :disabled="
              workspace.busy ||
              !workspace.activeProject ||
              !taskPrompt.trim() ||
              !selectedRoles.length ||
              !taskGitConfigurationValid
            "
            @click="createAgentTask"
          >
            {{ workspace.busy ? 'Creating workspace…' : 'Create task & run planner' }}
          </button>
        </aside>
      </div>
    </section>

    <article v-if="setupProvider" class="approval-overlay">
      <section class="approval-card provider-setup-card">
        <p class="eyebrow">Provider setup</p>
        <h2>Install {{ setupProvider.name }}</h2>
        <p>
          The provider CLI must be installed locally before Agent Workspace can open its official
          authentication flow.
        </p>
        <pre>{{ providerInstallCommand(setupProvider.id) }}</pre>
        <small>After installation, return to Mission Control and press Refresh. The button will change
          from Set up to Login.</small>
        <div class="approval-actions">
          <button class="secondary-button" type="button" @click="setupProvider = null">
            Close
          </button>
          <button
            class="primary-button"
            type="button"
            @click="
              setupProvider = null;
              workspace.loadAgentWorkspace()
            "
          >
            Refresh detection
          </button>
        </div>
      </section>
    </article>

    <article v-if="consoleProvider" class="provider-console-overlay">
      <section class="provider-console-panel">
        <header>
          <div>
            <p class="eyebrow">Embedded provider console</p>
            <h2>{{ consoleProvider.name }}</h2>
            <span>{{ consoleStatus }} · runs locally in your user environment</span>
          </div>
          <button type="button" aria-label="Close provider console" @click="closeProviderConsole">
            ×
          </button>
        </header>
        <div ref="consoleElement" class="provider-terminal"></div>
        <footer>
          <span>Interactive input, browser links, and CLI output stay inside Agent Workspace.</span>
          <button class="secondary-button" type="button" @click="closeProviderConsole">
            Close console
          </button>
        </footer>
      </section>
    </article>

    <article v-if="selectedTask" class="task-detail-overlay">
      <section class="task-detail-panel">
        <header>
          <div>
            <p class="eyebrow">{{ selectedTask.task.mode }} pipeline</p>
            <h2>{{ selectedTask.task.title }}</h2>
            <span>{{ selectedTask.task.status }} · {{ selectedTask.runs.length }} agent stages</span>
          </div>
          <button type="button" aria-label="Close task detail" @click="selectedTaskId = ''">×</button>
        </header>

        <div class="task-detail-objective" :class="{ expanded: objectiveExpanded }">
          <div>
            <span>Objective</span>
            <button
              v-if="selectedTask.task.prompt.length > 600"
              type="button"
              @click="objectiveExpanded = !objectiveExpanded"
            >
              {{ objectiveExpanded ? 'Collapse' : 'Show full prompt' }}
            </button>
          </div>
          <p>{{ selectedTask.task.prompt }}</p>
        </div>

        <section
          v-if="workspace.activeProject?.isGitRepository"
          class="task-git-context"
          :data-isolated="Boolean(selectedTask.task.worktree)"
        >
          <div>
            <span class="git-context-icon">⑂</span>
            <span>
              <small>Git workspace</small>
              <strong>{{ taskWorkspaceLabel(selectedTask.task) }}</strong>
            </span>
          </div>
          <div class="task-git-flow">
            <span><small>Base</small><code>{{ resolvedTaskBaseBranch(selectedTask.task) }}</code></span>
            <i>→</i>
            <span><small>Working branch</small><code>{{ taskWorkingBranch(selectedTask.task) }}</code></span>
            <template v-if="selectedTask.task.mergeTarget">
              <i>→</i>
              <span><small>Merge target</small><code>{{ selectedTask.task.mergeTarget }}</code></span>
            </template>
          </div>
          <div class="task-worktree-path">
            <small>{{ selectedTask.task.worktree ? 'Linked worktree' : 'Project directory' }}</small>
            <code :title="selectedTask.task.worktree || workspace.activeProject?.path">
              {{ selectedTask.task.worktree || workspace.activeProject?.path }}
            </code>
          </div>
          <span v-if="selectedTask.task.queueMerge" class="merge-queue-badge">Merge task queued</span>
        </section>

        <div class="task-detail-body">
          <aside class="task-stage-list">
            <h3>Pipeline stages</h3>
            <article
              v-for="(run, index) in selectedTask.runs"
              :key="run.id"
              :data-status="run.status"
            >
              <span class="stage-index">{{ index + 1 }}</span>
              <div>
                <strong>{{ run.role }}</strong>
                <small>{{ workspace.agentProviders.find((provider) => provider.id === run.providerId)?.name || run.providerId }}</small>
                <p>{{ run.summary || 'Waiting for activity' }}</p>
                <em v-if="run.worktree">{{ run.worktree }}</em>
              </div>
              <div class="stage-state">
                <span>{{ run.status }}</span>
                <progress :value="run.progress" max="100"></progress>
                <button
                  v-if="['interrupted', 'failed', 'cancelled'].includes(run.status)"
                  type="button"
                  :disabled="workspace.busy"
                  @click="workspace.retryTaskRun(selectedTask.task.id, run.id)"
                >
                  Retry stage
                </button>
                <button
                  v-if="run.threadId"
                  type="button"
                  @click="openRunThread(run.threadId); selectedTaskId = ''"
                >
                  Open session
                </button>
              </div>
            </article>
          </aside>

          <section class="task-evidence">
            <div class="task-detail-tabs">
              <button
                v-for="tab in taskDetailTabs"
                :key="tab"
                type="button"
                :class="{ active: selectedTaskTab === tab }"
                @click="selectTaskTab(tab)"
              >
                {{ tab }}
                <span v-if="tab === 'changes'">{{ selectedTaskChanges?.changes.length ?? 0 }}</span>
              </button>
            </div>

            <template v-if="selectedTaskTab === 'evidence'">
              <header>
                <div>
                  <h3>Artifacts & evidence</h3>
                  <span>Plans, agent output, snapshots, diffs, and verification reports.</span>
                </div>
                <strong>{{ selectedTaskArtifacts.length }}</strong>
              </header>
              <div class="artifact-tabs">
                <button
                  v-for="artifact in selectedTaskArtifacts"
                  :key="artifact.id"
                  type="button"
                  :class="{ active: selectedArtifact?.id === artifact.id }"
                  :data-status="artifact.status"
                  @click="selectedArtifactId = artifact.id"
                >
                  <strong>{{ artifact.title }}</strong>
                  <small>{{ artifact.type }} · {{ artifact.status }}</small>
                </button>
              </div>
              <pre v-if="selectedArtifact" class="artifact-content">{{
                readableArtifact(selectedArtifact.content)
              }}</pre>
              <div v-else class="mission-empty">No evidence has been generated yet.</div>
            </template>

            <template v-else-if="selectedTaskTab === 'changes'">
              <header>
                <div>
                  <h3>Changes since task start</h3>
                  <span>
                    {{ selectedTaskChanges?.source === 'snapshot' ? 'Exact file baseline' : 'Inferred from file timestamps' }}
                    · Git is not required
                  </span>
                </div>
                <button type="button" @click="workspace.loadTaskChanges(selectedTask.task.id)">Refresh</button>
              </header>
              <div v-if="selectedTaskChanges?.changes.length" class="task-changes-layout">
                <div class="task-change-list">
                  <button
                    v-for="change in selectedTaskChanges.changes"
                    :key="`${change.workspace}:${change.path}`"
                    type="button"
                    :class="{ active: selectedChange === change }"
                    :data-kind="change.kind"
                    @click="selectedChangePath = `${change.workspace}:${change.path}`"
                  >
                    <span>{{ change.kind.slice(0, 1).toUpperCase() }}</span>
                    <div>
                      <strong>{{ change.path }}</strong>
                      <small>{{ change.workspace }} · {{ formatBytes(change.size) }}</small>
                    </div>
                    <em v-if="change.inferred">inferred</em>
                  </button>
                </div>
                <div class="task-change-preview">
                  <header v-if="selectedChange">
                    <strong>{{ selectedChange.path }}</strong>
                    <span>{{ selectedChange.kind }} · {{ selectedChange.workspace }}</span>
                  </header>
                  <pre>{{ selectedChange?.diff || 'Inline diff is unavailable for this file.' }}</pre>
                </div>
              </div>
              <div v-else class="mission-empty">
                No file changes detected since this task started.
              </div>
            </template>

            <template v-else>
              <header>
                <div>
                  <h3>Origin & activity</h3>
                  <span>Why this task exists and how the workflow progressed.</span>
                </div>
                <strong>{{ selectedTaskActivity.length }}</strong>
              </header>
              <div class="task-origin-card">
                <span>Triggered by</span>
                <strong>Manual delegation · Mission Control</strong>
                <p>{{ selectedTask.task.prompt }}</p>
                <div>
                  <span>{{ selectedTask.task.mode }} workflow</span>
                  <span>{{ selectedTask.task.approvalMode }} approval</span>
                  <span>{{ selectedTask.task.workspaceMode }}</span>
                  <span v-if="selectedTask.task.taskBranch">{{ selectedTask.task.taskBranch }}</span>
                  <span v-if="selectedTask.task.mergeTarget">merge → {{ selectedTask.task.mergeTarget }}</span>
                  <span>{{ new Date(selectedTask.task.createdAt).toLocaleString() }}</span>
                  <span>{{ selectedTask.runs.length }} configured stages</span>
                </div>
              </div>
              <div class="task-activity-feed">
                <article v-for="event in selectedTaskActivity" :key="event.id" :data-status="event.status">
                  <i></i>
                  <div>
                    <strong>{{ event.title }}</strong>
                    <p>{{ event.detail }}</p>
                  </div>
                  <time>{{ new Date(event.timestamp).toLocaleString() }}</time>
                </article>
              </div>
            </template>
          </section>
        </div>

        <footer>
          <div>
            <span>{{ (workspace.coordinationByTask[selectedTask.task.id] ?? []).length }} coordination events</span>
            <button type="button" @click="openCoordination(selectedTask.task.id)">
              Open shared task bus
            </button>
          </div>
          <div>
            <button
              v-if="selectedTask.task.status === 'interrupted'"
              class="primary-button"
              type="button"
              :disabled="workspace.busy"
              @click="workspace.continueTask(selectedTask.task.id)"
            >
              Continue
            </button>
            <button type="button" @click="workspace.createTaskSnapshot(selectedTask.task.id)">
              Snapshot
            </button>
            <button
              type="button"
              :disabled="workspace.busy"
              @click="workspace.verifyTask(selectedTask.task.id)"
            >
              {{ workspace.busy ? 'Verifying…' : 'Run verification' }}
            </button>
            <button
              v-if="selectedTask.task.status === 'waiting-approval'"
              class="primary-button"
              type="button"
              @click="workspace.approveTaskStage(selectedTask.task.id)"
            >
              Approve next stage
            </button>
            <button
              v-if="!['completed', 'completed-with-findings', 'cancelled'].includes(selectedTask.task.status)"
              class="danger-button"
              type="button"
              :disabled="workspace.busy"
              @click="cancelSelectedTask"
            >
              Cancel
            </button>
            <button
              class="secondary-button"
              type="button"
              :disabled="workspace.busy"
              @click="restartSelectedTask"
            >
              Restart from beginning
            </button>
            <button
              v-if="selectedTask.task.status !== 'rolled-back'"
              class="danger-button"
              type="button"
              :disabled="workspace.busy"
              @click="openRollback(selectedTask.task.id)"
            >
              Rollback…
            </button>
          </div>
        </footer>
      </section>
    </article>

    <article v-if="rollbackTaskId && rollbackRootTask" class="rollback-overlay">
      <section class="rollback-panel">
        <header>
          <div>
            <p class="eyebrow">Destructive operation</p>
            <h2>Rollback task chain</h2>
          </div>
          <button type="button" @click="rollbackTaskId = ''">×</button>
        </header>
        <div class="rollback-warning">
          <strong>⚠ This cannot be undone automatically.</strong>
          <p>{{ workspace.rollbackPreview?.warning || 'Calculating rollback impact…' }}</p>
        </div>
        <div class="rollback-impact-list">
          <article
            v-for="(impact, index) in workspace.rollbackPreview?.tasks ?? []"
            :key="impact.taskId"
            :data-ready="impact.canRollback"
          >
            <span>{{ index + 1 }}</span>
            <div>
              <strong>{{ impact.title }}</strong>
              <small>
                {{ impact.branch || impact.workspace }} · {{ impact.changeCount }} changes ·
                {{ impact.status }}
              </small>
              <p v-if="impact.blockReason">{{ impact.blockReason }}</p>
            </div>
            <b>{{ impact.canRollback ? 'will rollback' : 'blocked' }}</b>
          </article>
        </div>
        <label>
          <span>Type the task title to confirm</span>
          <code>{{ rollbackRootTask.title }}</code>
          <input v-model="rollbackConfirmation" type="text" autocomplete="off" />
        </label>
        <footer>
          <button type="button" @click="rollbackTaskId = ''">Keep changes</button>
          <button
            class="danger-button"
            type="button"
            :disabled="
              workspace.busy ||
              !workspace.rollbackPreview?.canRollback ||
              rollbackConfirmation !== rollbackRootTask.title
            "
            @click="executeRollback"
          >
            Rollback {{ workspace.rollbackPreview?.tasks.length ?? 0 }} task(s)
          </button>
        </footer>
      </section>
    </article>

    <article v-if="coordinationTaskId && coordinationTask" class="coordination-overlay">
      <section class="coordination-panel">
        <header>
          <div>
            <p class="eyebrow">Shared task bus</p>
            <h2>{{ coordinationTask.task.title }}</h2>
            <span>TASK.md · append-only MAILBOX.jsonl · live turn steering</span>
          </div>
          <button type="button" @click="coordinationTaskId = ''">×</button>
        </header>
        <div class="coordination-runs">
          <span v-for="run in coordinationTask.runs" :key="run.id" :data-status="run.status">
            {{ run.role }} · {{ run.providerId }} · {{ run.status }}
          </span>
        </div>
        <div class="coordination-feed">
          <article
            v-for="message in workspace.coordinationByTask[coordinationTaskId] ?? []"
            :key="message.id"
            :data-role="message.role"
          >
            <header>
              <strong>{{ message.role || 'agent' }}</strong><span>{{ message.kind }}</span><time>{{ new Date(message.createdAt).toLocaleTimeString() }}</time>
            </header>
            <p>{{ message.message }}</p>
          </article>
          <div
            v-if="!(workspace.coordinationByTask[coordinationTaskId] ?? []).length"
            class="mission-empty"
          >
            No messages yet. Agents append findings, questions, and handoffs here.
          </div>
        </div>
        <footer>
          <textarea
            v-model="coordinationDraft"
            rows="3"
            placeholder="Broadcast guidance to every active agent on this task…"
          />
          <button
            class="primary-button"
            type="button"
            :disabled="!coordinationDraft.trim()"
            @click="postCoordination"
          >
            Broadcast
          </button>
        </footer>
      </section>
    </article>

    <article v-if="workspace.planArtifact?.status === 'needs_review'" class="plan-review-overlay">
      <section class="plan-review">
        <header>
          <div>
            <p class="eyebrow">Plan artifact · Needs review</p>
            <h2>Review before implementation</h2>
            <span>The selected agent stayed read-only during this planning turn.</span>
          </div>
          <button type="button" @click="workspace.closePlanReview">×</button>
        </header>
        <div class="plan-review-body">
          <aside>
            <strong>Approval gate</strong>
            <ol>
              <li class="done">Repository analyzed</li>
              <li class="done">Plan generated</li>
              <li class="active">Human review</li>
              <li>Implementation</li>
              <li>Verification</li>
            </ol>
            <small>Approval switches the next turn to implementation mode. Revision keeps the agent in
              strict read-only planning mode.</small>
          </aside>
          <div class="plan-document">
            <pre>{{ workspace.planArtifact.text }}</pre>
          </div>
        </div>
        <footer>
          <textarea
            v-model="planFeedback"
            rows="3"
            placeholder="Request a specific revision…"
          ></textarea>
          <div>
            <button
              class="secondary-button"
              type="button"
              :disabled="!planFeedback.trim()"
              @click="requestPlanRevision"
            >
              Revise plan
            </button>
            <button class="primary-button" type="button" @click="workspace.approvePlan">
              Approve & implement
            </button>
          </div>
        </footer>
      </section>
    </article>

    <article v-if="workspace.currentApproval" class="approval-overlay">
      <div class="approval-card">
        <p class="eyebrow">Approval required</p>
        <h2>
          {{
            workspace.currentApproval.kind === 'command'
              ? 'Run this command?'
              : workspace.currentApproval.kind === 'permissions'
                ? 'Grant requested permissions?'
                : 'Apply file changes?'
          }}
        </h2>
        <p v-if="workspace.currentApproval.details.reason" class="approval-reason">
          {{ workspace.currentApproval.details.reason }}
        </p>
        <div
          v-if="workspace.currentApproval.details.risk"
          class="risk-summary"
          :data-level="workspace.currentApproval.details.risk.level"
        >
          <strong>Risk: {{ workspace.currentApproval.details.risk.level }}</strong>
          <span v-for="reason in workspace.currentApproval.details.risk.reasons" :key="reason">
            {{ reason }}
          </span>
          <small>
            Reversible: {{ workspace.currentApproval.details.risk.reversible ? 'Yes' : 'No' }}
            · Outside project:
            {{ workspace.currentApproval.details.risk.outsideProject ? 'Possible' : 'No' }}
          </small>
        </div>
        <pre>{{
          workspace.currentApproval.details.command ||
          workspace.currentApproval.details.grantRoot ||
          'Codex is requesting permission to change project files.'
        }}</pre>
        <small>{{ workspace.currentApproval.details.cwd }}</small>
        <div class="approval-actions">
          <button class="danger-button" type="button" @click="workspace.answerApproval('decline')">
            Reject
          </button>
          <button
            class="secondary-button"
            type="button"
            @click="workspace.answerApproval('acceptForSession')"
          >
            Approve for session
          </button>
          <button class="primary-button" type="button" @click="workspace.answerApproval('accept')">
            Approve once
          </button>
        </div>
      </div>
    </article>

    <article v-if="workspace.currentUserInput" class="approval-overlay">
      <form class="approval-card interaction-card" @submit.prevent="submitUserInput">
        <p class="eyebrow">Codex question</p>
        <div
          v-for="question in workspace.currentUserInput.details.questions"
          :key="question.id"
          class="question-block"
        >
          <h2>{{ question.header }}</h2>
          <p>{{ question.question }}</p>
          <label v-for="option in question.options ?? []" :key="option.label">
            <input
              type="radio"
              :name="question.id"
              :checked="userAnswers[question.id]?.[0] === option.label"
              @change="chooseAnswer(question.id, option.label)"
            />
            <span><strong>{{ option.label }}</strong><small>{{ option.description }}</small></span>
          </label>
          <input
            v-if="!question.options?.length || question.isOther"
            type="text"
            placeholder="Your answer"
            @input="chooseAnswer(question.id, ($event.target as HTMLInputElement).value)"
          />
        </div>
        <div class="approval-actions">
          <button class="primary-button" type="submit">Continue</button>
        </div>
      </form>
    </article>

    <article v-if="workspace.currentElicitation" class="approval-overlay">
      <form class="approval-card interaction-card" @submit.prevent="submitElicitation">
        <p class="eyebrow">Tool input</p>
        <h2>{{ workspace.currentElicitation.details.message }}</h2>
        <label
          v-for="(property, key) in workspace.currentElicitation.details.requestedSchema
            ?.properties ?? {}"
          :key="key"
          class="field-label"
        >
          <span>{{ property.title || key }}</span>
          <input
            v-model="elicitationValues[key]"
            :type="property.type === 'number' ? 'number' : 'text'"
          />
          <small>{{ property.description }}</small>
        </label>
        <div class="approval-actions">
          <button
            class="danger-button"
            type="button"
            @click="workspace.answerElicitation('decline')"
          >
            Decline
          </button>
          <button class="primary-button" type="submit">Continue</button>
        </div>
      </form>
    </article>

    <aside v-if="showDiagnostics" class="diagnostics-drawer">
      <button type="button" @click="showDiagnostics = false">×</button>
      <p class="eyebrow">Diagnostics</p>
      <h2>Runtime status</h2>
      <dl>
        <div>
          <dt>Connection</dt>
          <dd>{{ statusText }}</dd>
        </div>
        <div>
          <dt>Codex</dt>
          <dd>{{ diagnostics.status?.installation?.version }}</dd>
        </div>
        <div>
          <dt>Executable</dt>
          <dd>{{ diagnostics.status?.installation?.path }}</dd>
        </div>
        <div>
          <dt>Git</dt>
          <dd>{{ diagnostics.status?.git.version }}</dd>
        </div>
      </dl>
      <button class="secondary-button" type="button" @click="diagnostics.restart">
        Restart app-server
      </button>
    </aside>

    <footer
      v-if="connected && workspace.providerUsage.some((usage) => usage.authenticated)"
      class="provider-usage-footer"
    >
      <button
        v-for="usage in workspace.providerUsage.filter((candidate) => candidate.authenticated)"
        :key="usage.providerId"
        type="button"
        :title="`${usage.detail}${usage.resetsAt ? ` · resets ${formatReset(usage.resetsAt)}` : ''}`"
        @click="currentView = 'settings'"
      >
        <i :data-authenticated="usage.authenticated"></i>
        <strong>{{ usage.name }}</strong>
        <span v-if="usage.remainingPercent !== undefined">
          {{ Math.round(usage.remainingPercent) }}%
        </span>
        <span v-else>{{ usage.authenticated ? usage.detail : 'Sign in' }}</span>
      </button>
    </footer>

    <p v-if="workspace.error && !needsLogin" class="toast" role="alert">{{ workspace.error }}</p>
  </main>
</template>
