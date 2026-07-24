<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { backend } from './backend'
import { useDiagnosticsStore } from './stores/diagnostics'
import { useWorkspaceStore } from './stores/workspace'
import type { ConversationItem, FileMatch } from './types'

const diagnostics = useDiagnosticsStore()
const workspace = useWorkspaceStore()
const prompt = ref('')
const conversation = ref<HTMLElement | null>(null)
const showDiagnostics = ref(false)
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
  return text.split(/(@[A-Za-z0-9_./-]+)/g).map((value) => ({
    value,
    file: value.startsWith('@') ? value.slice(1) : '',
  }))
}

function openLinkedFile(path: string) {
  if (workspace.activeProject) void backend.openProjectFile(workspace.activeProject.id, path)
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
          <strong>Linux Codex Desktop</strong>
          <span>{{ workspace.activeProject?.path ?? 'Choose a local project' }}</span>
        </div>
      </div>
      <div class="header-actions">
        <button
          v-if="usageLabel"
          class="usage-chip"
          type="button"
          @click="workspace.loadRateLimits"
        >
          {{ usageLabel }}
        </button>
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

    <section v-else class="workspace">
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
              <select
                v-model="workspace.selectedModel"
                @change="workspace.setModel(workspace.selectedModel)"
              >
                <option v-for="model in workspace.models" :key="model.id" :value="model.id">
                  {{ model.displayName }}
                </option>
              </select>
              <select v-model="workspace.selectedEffort">
                <option v-for="effort in workspace.supportedEfforts" :key="effort" :value="effort">
                  {{ effort }}
                </option>
              </select>
              <select v-model="workspace.selectedMode">
                <option value="default">Default mode</option>
                <option value="plan">Plan mode</option>
              </select>
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
