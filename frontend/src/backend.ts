import type { StartupStatus } from './types'

const unavailable = (): never => {
  throw new Error('Wails bindings are unavailable. Run the application through Wails.')
}

export const backend = {
  async getStartupStatus(): Promise<StartupStatus> {
    const method = window.go?.main?.App?.GetStartupStatus
    return method ? ((await method()) as StartupStatus) : unavailable()
  },
  async detectCodex(): Promise<void> {
    const method = window.go?.main?.App?.DetectCodex
    if (!method) return unavailable()
    await method()
  },
  async startCodex(): Promise<void> {
    const method = window.go?.main?.App?.StartCodex
    if (!method) return unavailable()
    await method()
  },
  async restartCodex(): Promise<void> {
    const method = window.go?.main?.App?.RestartCodex
    if (!method) return unavailable()
    await method()
  },
}

export function subscribe(name: string, callback: (payload: unknown) => void): () => void {
  const eventsOn = window.runtime?.EventsOn
  return eventsOn ? eventsOn(name, callback) : () => undefined
}
