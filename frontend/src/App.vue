<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { backend } from './backend'
import { useDiagnosticsStore } from './stores/diagnostics'
import { useWorkspaceStore } from './stores/workspace'
import type { AgentProvider, ConversationItem, FileMatch } from './types'

const diagnostics = useDiagnosticsStore()
const workspace = useWorkspaceStore()
const prompt = ref('')
const conversation = ref<HTMLElement | null>(null)
const showDiagnostics = ref(false)
const showUsage = ref(false)
const showModelMenu = ref(false)
const currentView = ref<'chat' | 'agents'>('chat')
const taskTitle = ref('')
const taskPrompt = ref('')
const taskWorkflow = ref<'delivery' | 'parallel'>('delivery')
const selectedRoles = ref(['planner', 'implementer', 'reviewer'])
const roleProviders = ref<Record<string, string>>({})
const memoryTitle = ref('')
const memoryContent = ref('')
const coordinationTaskId = ref('')
const coordinationDraft = ref('')
const setupProvider = ref<AgentProvider | null>(null)
const planFeedback = ref('')
const userAnswers = ref<Record<string, string[]>>({})
const elicitationValues = ref<Record<string, unknown>>({})
let fileSearchTimer: number | undefined

const connected = computed(() => diagnostics.status?.state === 'ready')
const needsLogin = computed(
  () => connected.value && workspace.account !== null && !workspace.account.signedIn,
)
const statusText = computed(() => diagnostics.status?.state?.replace(/_/g, ' ') ?? 'starting')
const usageLabel = computed(() => {
  const limits = workspace.rateLimits
  if (!limits?.primary) return ''
  const primary = `${Math.round(limits.primary.remainingPercent)}% left`
  return limits.secondary
    ? `${primary} · ${Math.round(limits.secondary.remainingPercent)}% long-term`
    : primary
})

onMounted(async () => {
  diagnostics.connectEvents()
  await diagnostics.load()
  if (connected.value) await workspace.initialize()
})

onBeforeUnmount(() => {
  if (fileSearchTimer) globalThis.clearTimeout(fileSearchTimer)
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
  selectedRoles.value = selectedRoles.value.includes(role)
    ? selectedRoles.value.filter((candidate) => candidate !== role)
    : [...selectedRoles.value, role]
}

async function createAgentTask() {
  if (!taskPrompt.value.trim()) return
  await workspace.createAgentTask({
    title: taskTitle.value,
    prompt: taskPrompt.value,
    workflow: taskWorkflow.value,
    roles: selectedRoles.value,
    providers: Object.fromEntries(
      selectedRoles.value.map((role) => [role, roleProviders.value[role] || 'auto']),
    ),
  })
  taskTitle.value = ''
  taskPrompt.value = ''
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

async function proposeMemory() {
  await workspace.proposeMemory('constraint', memoryTitle.value, memoryContent.value)
  memoryTitle.value = ''
  memoryContent.value = ''
}

async function openCoordination(taskId: string) {
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
  await workspace.manageProviderLogin(provider.id)
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
    .filter((run) => run.status === 'failed' || run.status === 'unavailable'),
)

function itemTitle(item: ConversationItem): string {
  switch (item.type) {
    case 'agentMessage':
      return 'Codex'
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
      <nav v-if="connected && !needsLogin" class="view-switcher">
        <button type="button" :class="{ active: currentView === 'chat' }" @click="currentView = 'chat'">
          Workspace
        </button>
        <button type="button" :class="{ active: currentView === 'agents' }" @click="openMissionControl">
          Mission Control
        </button>
      </nav>
      <div class="header-actions">
        <button
          v-if="usageLabel"
          class="usage-chip"
          type="button"
          @click="showUsage = !showUsage"
        >
          {{ usageLabel }}
        </button>
        <div v-if="showUsage" class="usage-popover">
          <div class="popover-heading">
            <div><small>Usage limits</small><strong>{{ workspace.rateLimits?.limitName || 'Codex' }}</strong></div>
            <button type="button" @click="showUsage = false">×</button>
          </div>
          <div v-if="workspace.rateLimits?.primary" class="usage-window">
            <span>Short-term window</span>
            <strong>{{ Math.round(workspace.rateLimits.primary.remainingPercent) }}% left</strong>
            <progress :value="workspace.rateLimits.primary.remainingPercent" max="100"></progress>
            <small>Automatically resets: {{ formatReset(workspace.rateLimits.primary.resetsAt) }}</small>
          </div>
          <div v-if="workspace.rateLimits?.secondary" class="usage-window">
            <span>Long-term window</span>
            <strong>{{ Math.round(workspace.rateLimits.secondary.remainingPercent) }}% left</strong>
            <progress :value="workspace.rateLimits.secondary.remainingPercent" max="100"></progress>
            <small>Automatically resets: {{ formatReset(workspace.rateLimits.secondary.resetsAt) }}</small>
          </div>
          <p>Limits are reset by the provider automatically. This client cannot manually reset provider quota.</p>
          <button class="secondary-button" type="button" @click="workspace.loadRateLimits">Refresh usage</button>
        </div>
        <button class="connection-chip" type="button" @click="showDiagnostics = !showDiagnostics">
          <span class="status-dot" :data-ready="connected"></span>
          {{ statusText }}
        </button>
        <div v-if="workspace.account?.signedIn" class="account-chip">
          <span>{{ workspace.account.email || workspace.account.type }}</span>
          <small>{{ workspace.account.planType }}</small>
          <button type="button" title="Sign out" @click="workspace.logout">Sign out</button>
        </div>
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

    <section v-else-if="needsLogin" class="center-state login-state">
      <p class="eyebrow">Authentication</p>
      <h1>Continue with ChatGPT</h1>
      <p>Authentication is handled by your installed Codex runtime in the system browser.</p>
      <div class="login-actions">
        <button class="primary-button" type="button" @click="workspace.login">
          Continue with ChatGPT
        </button>
        <button class="secondary-button" type="button" @click="workspace.loginWithDeviceCode">
          Use device code
        </button>
      </div>
      <div v-if="workspace.deviceLogin" class="device-code">
        <span>Enter this code in the browser</span>
        <strong>{{ workspace.deviceLogin.userCode }}</strong>
        <small>{{ workspace.deviceLogin.verificationUrl }}</small>
      </div>
      <p v-if="workspace.error" class="inline-error">{{ workspace.error }}</p>
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
                <small>{{ new Date(thread.updatedAt).toLocaleDateString() }}</small>
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
          <p>Select a local repository or directory where Codex should work.</p>
          <button class="primary-button" type="button" @click="workspace.addProject">
            Add project
          </button>
        </div>
        <div v-else-if="!workspace.activeThread" class="pane-empty">
          <div class="empty-icon">✦</div>
          <h2>Start a Codex thread</h2>
          <p>
            Codex will work inside {{ workspace.activeProject.name }} with approval prompts enabled.
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
              <span v-if="workspace.activeTurn" class="working-label">Codex is working…</span>
              <button class="secondary-button" type="button" @click="workspace.exportActiveThread">
                Export
              </button>
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
              <h3>What should Codex do?</h3>
              <p>Ask about the codebase, request a change, or run tests.</p>
            </div>
          </div>
          <form class="composer" @submit.prevent="submit">
            <textarea
              v-model="prompt"
              rows="3"
              placeholder="Message Codex…"
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
              <div class="model-control">
                <button class="control-trigger" type="button" @click="showModelMenu = !showModelMenu">
                  <span>Model</span><strong>{{ workspace.currentModel?.displayName || 'Default' }}</strong><b>⌄</b>
                </button>
                <div v-if="showModelMenu" class="model-menu">
                  <button
                    v-for="model in workspace.models"
                    :key="model.id"
                    type="button"
                    :class="{ active: model.id === workspace.selectedModel }"
                    @click="workspace.setModel(model.id); showModelMenu = false"
                  >
                    <strong>{{ model.displayName }}</strong><small>{{ model.description }}</small>
                  </button>
                </div>
              </div>
              <div class="segmented-control" aria-label="Reasoning effort">
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
                <button type="button" :class="{ active: workspace.selectedMode === 'default' }" @click="workspace.selectedMode = 'default'">Fast</button>
                <button type="button" :class="{ active: workspace.selectedMode === 'plan' }" @click="workspace.selectedMode = 'plan'">Plan</button>
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
          <span>Plan, implement, review, and verify across installed coding agents.</span>
        </div>
        <button class="secondary-button" type="button" @click="workspace.loadAgentWorkspace">
          Refresh
        </button>
      </header>

      <div class="mission-stats">
        <article><span>Active agents</span><strong>{{ activeRuns.length }}</strong></article>
        <article><span>Waiting</span><strong>{{ waitingRuns.length }}</strong></article>
        <article><span>Failed / unavailable</span><strong>{{ failedRuns.length }}</strong></article>
        <article><span>Tasks</span><strong>{{ workspace.agentTasks.length }}</strong></article>
      </div>

      <div class="mission-layout">
        <section class="mission-main">
          <div class="provider-strip">
            <article
              v-for="provider in workspace.agentProviders"
              :key="provider.id"
              :class="{ unavailable: !provider.runnable }"
            >
              <span class="provider-mark">{{ provider.name.slice(0, 1) }}</span>
              <div>
                <strong>{{ provider.name }}</strong>
                <small>{{
                  !provider.installed
                    ? 'Not installed'
                    : provider.runnable
                      ? provider.version || 'Available'
                      : 'Detected · adapter pending'
                }}</small>
                <small class="auth-state">{{ provider.authStatus }}</small>
              </div>
              <i :data-ready="provider.runnable"></i>
              <button
                class="provider-auth"
                type="button"
                :disabled="provider.authStatus === 'not-required'"
                @click.stop="providerAction(provider)"
              >
                {{
                  !provider.installed
                    ? 'Set up'
                    : provider.authStatus === 'authenticated'
                      ? 'Manage'
                      : 'Login'
                }}
              </button>
            </article>
          </div>

          <div class="agent-table">
            <header>
              <span>Agent / role</span><span>Provider</span><span>Task</span><span>Status</span>
            </header>
            <template v-for="bundle in workspace.agentTasks" :key="bundle.task.id">
              <article
                v-for="run in bundle.runs"
                :key="run.id"
                :class="{ clickable: !!run.threadId }"
                @click="openRunThread(run.threadId)"
              >
                <div><strong>{{ run.role }}</strong><small>{{ run.summary || 'Waiting for activity' }}</small></div>
                <span>{{ workspace.agentProviders.find((provider) => provider.id === run.providerId)?.name || run.providerId }}</span>
                <div><strong>{{ bundle.task.title }}</strong><small>{{ bundle.task.mode }}</small></div>
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
                <span><strong>{{ entry.path }}</strong><small>{{ entry.reason }} · ~{{ entry.estimatedTokens.toLocaleString() }} tokens</small></span>
              </div>
            </article>
            <article class="memory-inspector">
              <h3>Living Project Memory</h3>
              <div v-for="memory in workspace.projectMemory" :key="memory.id">
                <i data-status="warning"></i>
                <span><strong>{{ memory.title }}</strong><small>{{ memory.status }} · {{ memory.content }}</small></span>
                <span v-if="memory.status === 'proposed'" class="memory-actions">
                  <button type="button" @click="workspace.reviewMemory(memory.id, 'rejected')">×</button>
                  <button type="button" @click="workspace.reviewMemory(memory.id, 'approved')">✓</button>
                </span>
              </div>
              <input v-model="memoryTitle" type="text" placeholder="Constraint title" />
              <textarea v-model="memoryContent" rows="2" placeholder="Propose project knowledge…" />
              <button type="button" :disabled="!memoryTitle.trim() || !memoryContent.trim()" @click="proposeMemory">
                Propose memory
              </button>
            </article>
          </div>

          <div class="task-capsules">
            <article v-for="bundle in workspace.agentTasks" :key="bundle.task.id">
              <header>
                <div>
                  <span>{{ bundle.task.mode }} workflow</span>
                  <h3>{{ bundle.task.title }}</h3>
                </div>
                <strong :data-status="bundle.task.status">{{ bundle.task.status }}</strong>
              </header>
              <p>{{ bundle.task.prompt }}</p>
              <div class="artifact-row">
                <span
                  v-for="artifact in bundle.artifacts"
                  :key="artifact.id"
                  :title="artifact.content"
                >{{ artifact.type }} · {{ artifact.status }}</span>
                <em v-if="!bundle.artifacts.length">No artifacts yet</em>
              </div>
              <footer>
                <button
                  v-if="bundle.task.status === 'waiting-approval'"
                  class="approve-stage"
                  type="button"
                  @click="workspace.approveTaskStage(bundle.task.id)"
                >
                  Approve next stage
                </button>
                <button type="button" @click="workspace.createTaskSnapshot(bundle.task.id)">Snapshot</button>
                <button type="button" @click="workspace.createTaskWorktree(bundle.task.id)">Isolated worktree</button>
                <button type="button" @click="workspace.verifyTask(bundle.task.id)">Run verification</button>
                <button type="button" @click="openCoordination(bundle.task.id)">Shared task</button>
              </footer>
            </article>
          </div>
        </section>

        <aside class="task-builder">
          <p class="eyebrow">New workflow</p>
          <h2>Delegate a task</h2>
          <label>
            <span>Project</span>
            <strong>{{ workspace.activeProject?.name || 'Select a project in Workspace' }}</strong>
          </label>
          <label>
            <span>Task title</span>
            <input v-model="taskTitle" type="text" placeholder="Optional short title" />
          </label>
          <label>
            <span>Objective</span>
            <textarea v-model="taskPrompt" rows="7" placeholder="Describe the outcome, constraints, and verification…" />
          </label>
          <div>
            <span class="builder-label">Workflow</span>
            <div class="segmented-control builder-segments">
              <button type="button" :class="{ active: taskWorkflow === 'delivery' }" @click="taskWorkflow = 'delivery'">Delivery pipeline</button>
              <button type="button" :class="{ active: taskWorkflow === 'parallel' }" @click="taskWorkflow = 'parallel'">Parallel jury</button>
            </div>
          </div>
          <div>
            <span class="builder-label">Agent roles</span>
            <div class="role-grid">
              <button
                v-for="role in ['planner', 'implementer', 'reviewer', 'security', 'browser-qa']"
                :key="role"
                type="button"
                :class="{ active: selectedRoles.includes(role) }"
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
                    v-for="provider in workspace.agentProviders"
                    :key="provider.id"
                    :value="provider.id"
                    :disabled="!provider.runnable"
                  >
                    {{ provider.name }}{{ provider.runnable ? '' : ` · ${provider.status}` }}
                  </option>
                </select>
              </label>
            </div>
          </div>
          <div class="router-note">
            <strong>Smart Router</strong>
            <p>Each role is assigned to the best installed provider. Missing providers are never simulated.</p>
          </div>
          <button
            class="primary-button create-workflow"
            type="button"
            :disabled="!workspace.activeProject || !taskPrompt.trim() || !selectedRoles.length"
            @click="createAgentTask"
          >
            Start agent workflow
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
        <small>After installation, return to Mission Control and press Refresh. The button will change from Set up to Login.</small>
        <div class="approval-actions">
          <button class="secondary-button" type="button" @click="setupProvider = null">Close</button>
          <button
            class="primary-button"
            type="button"
            @click="setupProvider = null; workspace.loadAgentWorkspace()"
          >
            Refresh detection
          </button>
        </div>
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
            <header><strong>{{ message.role || 'agent' }}</strong><span>{{ message.kind }}</span><time>{{ new Date(message.createdAt).toLocaleTimeString() }}</time></header>
            <p>{{ message.message }}</p>
          </article>
          <div v-if="!(workspace.coordinationByTask[coordinationTaskId] ?? []).length" class="mission-empty">
            No messages yet. Agents append findings, questions, and handoffs here.
          </div>
        </div>
        <footer>
          <textarea v-model="coordinationDraft" rows="3" placeholder="Broadcast guidance to every active agent on this task…" />
          <button class="primary-button" type="button" :disabled="!coordinationDraft.trim()" @click="postCoordination">
            Broadcast
          </button>
        </footer>
      </section>
    </article>

    <article
      v-if="workspace.planArtifact?.status === 'needs_review'"
      class="plan-review-overlay"
    >
      <section class="plan-review">
        <header>
          <div>
            <p class="eyebrow">Plan artifact · Needs review</p>
            <h2>Review before implementation</h2>
            <span>Codex stayed read-only during this planning turn.</span>
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
            <small>Approval switches the next turn to implementation mode. Revision keeps Codex in strict read-only planning mode.</small>
          </aside>
          <div class="plan-document">
            <pre>{{ workspace.planArtifact.text }}</pre>
          </div>
        </div>
        <footer>
          <textarea v-model="planFeedback" rows="3" placeholder="Request a specific revision…"></textarea>
          <div>
            <button class="secondary-button" type="button" :disabled="!planFeedback.trim()" @click="requestPlanRevision">Revise plan</button>
            <button class="primary-button" type="button" @click="workspace.approvePlan">Approve & implement</button>
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

    <p v-if="workspace.error && !needsLogin" class="toast" role="alert">{{ workspace.error }}</p>
  </main>
</template>
