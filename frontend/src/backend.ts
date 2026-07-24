import type { Account, LoginStart, Project, StartupStatus, Thread, TurnStart } from './types'

const app = () => {
  const binding = window.go?.main?.App
  if (!binding)
    throw new Error('Wails bindings are unavailable. Run the application through Wails.')
  return binding
}

export const backend = {
  getStartupStatus: () => app().GetStartupStatus() as Promise<StartupStatus>,
  detectCodex: () => app().DetectCodex(),
  startCodex: () => app().StartCodex(),
  restartCodex: () => app().RestartCodex(),
  getAccount: () => app().GetAccount() as Promise<Account>,
  loginWithChatGPT: () => app().LoginWithChatGPT() as Promise<LoginStart>,
  loginWithDeviceCode: () => app().LoginWithDeviceCode() as Promise<LoginStart>,
  logout: () => app().Logout(),
  selectProject: () => app().SelectProject() as Promise<Project>,
  addProject: (path: string) => app().AddProject(path) as Promise<Project>,
  listProjects: () => app().ListProjects() as Promise<Project[]>,
  removeProject: (id: string) => app().RemoveProject(id),
  listThreads: (projectId: string) => app().ListThreads(projectId) as Promise<Thread[]>,
  startThread: (projectId: string) => app().StartThread(projectId) as Promise<Thread>,
  resumeThread: (threadId: string) => app().ResumeThread(threadId) as Promise<Thread>,
  sendMessage: (threadId: string, text: string) =>
    app().SendMessage(threadId, text) as Promise<TurnStart>,
  interruptTurn: (threadId: string, turnId: string) => app().InterruptTurn(threadId, turnId),
  respondToApproval: (requestId: string, kind: string, decision: string) =>
    app().RespondToApproval({ requestId, kind, decision }),
  openInFileManager: (path: string) => app().OpenInFileManager(path),
}

export function subscribe(name: string, callback: (payload: unknown) => void): () => void {
  const eventsOn = window.runtime?.EventsOn
  return eventsOn ? eventsOn(name, callback) : () => undefined
}
