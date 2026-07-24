import { defineStore } from 'pinia'

import { backend, subscribe } from '../backend'
import type { Activity, ConnectionEvent, StartupStatus } from '../types'

const activityLimit = 40

export const useDiagnosticsStore = defineStore('diagnostics', {
  state: () => ({
    status: null as StartupStatus | null,
    activity: [] as Activity[],
    busy: false,
    actionError: '',
    unsubscribers: [] as Array<() => void>,
  }),
  actions: {
    async load() {
      try {
        this.status = await backend.getStartupStatus()
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error)
      }
    },
    connectEvents() {
      if (this.unsubscribers.length) return
      this.unsubscribers.push(
        subscribe('codex:connection', (payload) => {
          const event = payload as ConnectionEvent
          if (this.status) {
            this.status.state = event.state
            this.status.installation = event.installation
            this.status.lastError = event.error
            this.status.updatedAt = event.timestamp
          } else {
            void this.load()
          }
        }),
        subscribe('codex:activity', (payload) => {
          this.activity.unshift(payload as Activity)
          this.activity = this.activity.slice(0, activityLimit)
        }),
      )
    },
    disconnectEvents() {
      this.unsubscribers.splice(0).forEach((unsubscribe) => unsubscribe())
    },
    async retry() {
      await this.runAction(async () => {
        await backend.startCodex()
        await this.load()
      })
    },
    async restart() {
      await this.runAction(async () => {
        await backend.restartCodex()
        await this.load()
      })
    },
    async runAction(action: () => Promise<void>) {
      this.busy = true
      this.actionError = ''
      try {
        await action()
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error)
        await this.load()
      } finally {
        this.busy = false
      }
    },
  },
})
