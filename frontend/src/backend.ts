import type {
  Account,
  AgentProvider,
  AgentTaskBundle,
  AgentTaskRequest,
  ContextEntry,
  ElicitationRequest,
  FileMatch,
  LoginStart,
  Model,
  Project,
  ProjectMemory,
  PreflightCheck,
  RateLimits,
  StartupStatus,
  Thread,
  TurnOptions,
  TurnStart,
} from './types'

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
  getRateLimits: () => app().GetRateLimits() as Promise<RateLimits>,
  listModels: () => app().ListModels() as Promise<Model[]>,
  listAgentProviders: () => app().ListAgentProviders() as Promise<AgentProvider[]>,
  listAgentTasks: (projectId = '') =>
    app().ListAgentTasks(projectId) as Promise<AgentTaskBundle[]>,
  createAgentTask: (request: AgentTaskRequest) =>
    app().CreateAgentTask(request) as Promise<AgentTaskBundle>,
  preflightProject: (projectId: string) =>
    app().PreflightProject(projectId) as Promise<PreflightCheck[]>,
  inspectProjectContext: (projectId: string) =>
    app().InspectProjectContext(projectId) as Promise<ContextEntry[]>,
  createTaskSnapshot: (taskId: string) => app().CreateTaskSnapshot(taskId),
  createTaskWorktree: (taskId: string) => app().CreateTaskWorktree(taskId),
  runTaskVerification: (taskId: string) => app().RunTaskVerification(taskId),
  approveAgentTaskStage: (taskId: string) => app().ApproveAgentTaskStage(taskId),
  listProjectMemory: (projectId: string) =>
    app().ListProjectMemory(projectId) as Promise<ProjectMemory[]>,
  proposeProjectMemory: (projectId: string, kind: string, title: string, content: string) =>
    app().ProposeProjectMemory(projectId, kind, title, content) as Promise<ProjectMemory>,
  reviewProjectMemory: (id: string, decision: 'approved' | 'rejected') =>
    app().ReviewProjectMemory(id, decision),
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
  sendMessageWithOptions: (threadId: string, text: string, options: TurnOptions) =>
    app().SendMessageWithOptions(threadId, text, options) as Promise<TurnStart>,
  searchProjectFiles: (projectId: string, query: string) =>
    app().SearchProjectFiles(projectId, query) as Promise<FileMatch[]>,
  openFile: (path: string) => app().OpenFile(path),
  openProjectFile: (projectId: string, relativePath: string) =>
    app().OpenProjectFile(projectId, relativePath),
  exportThread: (threadId: string) => app().ExportThread(threadId),
  importThread: (projectId: string) => app().ImportThread(projectId) as Promise<Thread>,
  interruptTurn: (threadId: string, turnId: string) => app().InterruptTurn(threadId, turnId),
  respondToApproval: (requestId: string, kind: string, decision: string) =>
    app().RespondToApproval({ requestId, kind, decision }),
  respondToPermission: (
    requestId: string,
    permissions: Record<string, unknown>,
    scope: 'turn' | 'session',
  ) => app().RespondToPermission(requestId, permissions, scope),
  respondToUserInput: (requestId: string, answers: Record<string, string[]>) =>
    app().RespondToUserInput(requestId, answers),
  respondToElicitation: (
    request: ElicitationRequest,
    action: 'accept' | 'decline' | 'cancel',
    content: Record<string, unknown>,
  ) => app().RespondToElicitation(request.requestId, action, content),
  openInFileManager: (path: string) => app().OpenInFileManager(path),
}

export function subscribe(name: string, callback: (payload: unknown) => void): () => void {
  const eventsOn = window.runtime?.EventsOn
  return eventsOn ? eventsOn(name, callback) : () => undefined
}
