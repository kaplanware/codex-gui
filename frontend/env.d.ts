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
          GetRateLimits(): Promise<unknown>
          ListModels(): Promise<unknown>
          ListAgentProviders(): Promise<unknown>
          ListProviderModels(providerId: string): Promise<unknown>
          ListProviderUsage(): Promise<unknown>
          BeginProviderLogin(providerId: string): Promise<unknown>
          StartProviderConsole(
            providerId: string,
            purpose: string,
            projectId: string,
            cols: number,
            rows: number,
          ): Promise<unknown>
          WriteProviderConsole(sessionId: string, data: string): Promise<void>
          ResizeProviderConsole(sessionId: string, cols: number, rows: number): Promise<void>
          StopProviderConsole(sessionId: string): Promise<void>
          LogoutProvider(providerId: string): Promise<void>
          ListAgentTasks(projectId: string): Promise<unknown>
          CreateAgentTask(request: unknown): Promise<unknown>
          GetTaskChanges(taskId: string): Promise<unknown>
          PreflightProject(projectId: string): Promise<unknown>
          InspectProjectContext(projectId: string): Promise<unknown>
          AssessCommandRisk(projectId: string, command: string): Promise<unknown>
          ListProjectMemory(projectId: string): Promise<unknown>
          ProposeProjectMemory(
            projectId: string,
            kind: string,
            title: string,
            content: string,
          ): Promise<unknown>
          ReviewProjectMemory(id: string, decision: string): Promise<void>
          CreateTaskSnapshot(taskId: string): Promise<unknown>
          CreateTaskWorktree(taskId: string): Promise<unknown>
          RunTaskVerification(taskId: string): Promise<unknown>
          ApproveAgentTaskStage(taskId: string): Promise<void>
          ContinueAgentTask(taskId: string): Promise<void>
          RetryAgentTaskRun(taskId: string, runId: string): Promise<void>
          CancelAgentTask(taskId: string): Promise<void>
          RestartAgentTask(taskId: string): Promise<void>
          ListCoordinationMessages(taskId: string): Promise<unknown>
          PostCoordinationMessage(
            taskId: string,
            runId: string,
            kind: string,
            message: string,
          ): Promise<unknown>
          LoginWithChatGPT(): Promise<unknown>
          LoginWithDeviceCode(): Promise<unknown>
          Logout(): Promise<void>
          SelectProject(): Promise<unknown>
          AddProject(path: string): Promise<unknown>
          ListProjects(): Promise<unknown>
          RemoveProject(id: string): Promise<void>
          ListThreads(projectId: string): Promise<unknown>
          StartThread(projectId: string): Promise<unknown>
          StartAgentThread(projectId: string, providerId: string, model: string): Promise<unknown>
          ResumeThread(threadId: string): Promise<unknown>
          SendMessage(threadId: string, text: string): Promise<unknown>
          SendMessageWithOptions(threadId: string, text: string, options: unknown): Promise<unknown>
          SearchProjectFiles(projectId: string, query: string): Promise<unknown>
          OpenFile(path: string): Promise<void>
          OpenProjectFile(projectId: string, relativePath: string): Promise<void>
          ExportThread(threadId: string): Promise<string>
          ImportThread(projectId: string): Promise<unknown>
          InterruptTurn(threadId: string, turnId: string): Promise<void>
          RespondToApproval(decision: unknown): Promise<void>
          RespondToPermission(requestId: string, permissions: unknown, scope: string): Promise<void>
          RespondToUserInput(requestId: string, answers: unknown): Promise<void>
          RespondToElicitation(requestId: string, action: string, content: unknown): Promise<void>
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
