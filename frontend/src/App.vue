<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'

import { useDiagnosticsStore } from './stores/diagnostics'

const store = useDiagnosticsStore()

const stateLabel = computed(() => {
  const state = store.status?.state ?? 'detecting'
  return state.replace(/_/g, ' ')
})

const stateTone = computed(() => {
  switch (store.status?.state) {
    case 'ready':
      return 'success'
    case 'missing':
    case 'unsupported':
    case 'error':
      return 'danger'
    case 'starting':
    case 'initializing':
    case 'detecting':
    case 'stopping':
      return 'working'
    default:
      return 'neutral'
  }
})

const canRestart = computed(
  () => store.status?.state === 'ready' || store.status?.state === 'error',
)

onMounted(async () => {
  store.connectEvents()
  await store.load()
})

onBeforeUnmount(() => store.disconnectEvents())
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">Local workspace client</p>
        <h1>Linux Codex Desktop</h1>
      </div>
      <div class="status-pill" :data-tone="stateTone">
        <span class="status-dot" aria-hidden="true"></span>
        <span>{{ stateLabel }}</span>
      </div>
    </header>

    <section class="hero">
      <div>
        <p class="eyebrow">Startup diagnostics</p>
        <h2>Codex connection</h2>
        <p class="lede">
          The desktop host runs the official Codex runtime locally and communicates over stdio.
        </p>
      </div>
      <button
        v-if="canRestart"
        class="button"
        type="button"
        :disabled="store.busy"
        @click="store.restart"
      >
        {{ store.busy ? 'Working…' : 'Restart app-server' }}
      </button>
      <button
        v-else-if="store.status?.state === 'missing' || store.status?.state === 'unsupported'"
        class="button"
        type="button"
        :disabled="store.busy"
        @click="store.retry"
      >
        {{ store.busy ? 'Checking…' : 'Retry detection' }}
      </button>
    </section>

    <p v-if="store.actionError" class="alert" role="alert">{{ store.actionError }}</p>
    <article v-if="store.status?.lastError" class="alert" role="alert">
      <strong>{{ store.status.lastError.summary }}</strong>
      <span>{{ store.status.lastError.detail }}</span>
    </article>

    <section class="grid" aria-label="Dependency status">
      <article class="card">
        <div class="card-heading">
          <span>Codex CLI</span>
          <span class="tag" :data-ok="store.status?.installation?.supported">
            {{ store.status?.installation?.supported ? 'Supported' : 'Needs attention' }}
          </span>
        </div>
        <dl>
          <div>
            <dt>Version</dt>
            <dd>{{ store.status?.installation?.version ?? 'Not detected' }}</dd>
          </div>
          <div>
            <dt>Minimum</dt>
            <dd>{{ store.status?.installation?.minimumVersion ?? '0.145.0' }}</dd>
          </div>
          <div>
            <dt>Executable</dt>
            <dd class="path">{{ store.status?.installation?.path ?? 'Searches PATH' }}</dd>
          </div>
        </dl>
      </article>

      <article class="card">
        <div class="card-heading">
          <span>Host environment</span>
          <span class="tag" :data-ok="store.status?.git.found">
            {{ store.status?.git.found ? 'Ready' : 'Incomplete' }}
          </span>
        </div>
        <dl>
          <div>
            <dt>Application</dt>
            <dd>{{ store.status?.appVersion ?? 'Loading' }}</dd>
          </div>
          <div>
            <dt>Platform</dt>
            <dd>{{ store.status?.platform ?? 'Loading' }}</dd>
          </div>
          <div>
            <dt>Git</dt>
            <dd>{{ store.status?.git.version ?? 'Not found' }}</dd>
          </div>
        </dl>
      </article>
    </section>

    <section class="log-panel">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Sanitized</p>
          <h2>Protocol activity</h2>
        </div>
        <span>{{ store.activity.length }} events</span>
      </div>
      <ol v-if="store.activity.length" class="activity-list">
        <li v-for="entry in store.activity" :key="`${entry.timestamp}-${entry.method}`">
          <span class="activity-kind">{{ entry.kind }}</span>
          <code>{{ entry.method || entry.message }}</code>
          <time>{{ new Date(entry.timestamp).toLocaleTimeString() }}</time>
        </li>
      </ol>
      <p v-else class="empty">Waiting for app-server activity.</p>
    </section>

    <footer>
      Independent open-source client. Not an official OpenAI product. Authentication and agent
      execution remain owned by the installed Codex runtime.
    </footer>
  </main>
</template>
