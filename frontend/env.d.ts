/// <reference types="vite/client" />

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          GetStartupStatus(): Promise<unknown>
          DetectCodex(): Promise<unknown>
          StartCodex(): Promise<void>
          RestartCodex(): Promise<void>
        }
      }
    }
    runtime?: {
      EventsOn(name: string, callback: (payload: unknown) => void): () => void
    }
  }
}

export {}
