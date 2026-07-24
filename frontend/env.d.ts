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
          GetAccount(): Promise<unknown>
          LoginWithChatGPT(): Promise<unknown>
          LoginWithDeviceCode(): Promise<unknown>
          Logout(): Promise<void>
          SelectProject(): Promise<unknown>
          AddProject(path: string): Promise<unknown>
          ListProjects(): Promise<unknown>
          RemoveProject(id: string): Promise<void>
          ListThreads(projectId: string): Promise<unknown>
          StartThread(projectId: string): Promise<unknown>
          ResumeThread(threadId: string): Promise<unknown>
          SendMessage(threadId: string, text: string): Promise<unknown>
          InterruptTurn(threadId: string, turnId: string): Promise<void>
          RespondToApproval(decision: unknown): Promise<void>
          OpenInFileManager(path: string): Promise<void>
        }
      }
    }
    runtime?: {
      EventsOn(name: string, callback: (payload: unknown) => void): () => void
    }
  }
}

export {}
