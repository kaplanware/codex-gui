export type ConnectionState =
  | 'detecting'
  | 'missing'
  | 'unsupported'
  | 'starting'
  | 'initializing'
  | 'ready'
  | 'stopping'
  | 'stopped'
  | 'error'

export interface AppError {
  code: string
  summary: string
  detail?: string
  retryable: boolean
}

export interface CodexInstallation {
  found: boolean
  path?: string
  version?: string
  rawVersion?: string
  minimumVersion: string
  supported: boolean
  newerThanTested: boolean
}

export interface DependencyStatus {
  name: string
  found: boolean
  path?: string
  version?: string
}

export interface StartupStatus {
  appName: string
  appVersion: string
  platform: string
  state: ConnectionState
  installation?: CodexInstallation
  git: DependencyStatus
  lastError?: AppError
  stderr: string[]
  updatedAt: string
}

export interface ConnectionEvent {
  state: ConnectionState
  installation?: CodexInstallation
  error?: AppError
  timestamp: string
}

export interface Activity {
  method?: string
  kind: string
  message?: string
  timestamp: string
}

export interface Account {
  signedIn: boolean
  type?: string
  email?: string
  planType?: string
  requiresOpenaiAuth: boolean
}

export interface LoginStart {
  type: string
  loginId: string
  authUrl?: string
  verificationUrl?: string
  userCode?: string
}

export interface Project {
  id: string
  name: string
  path: string
  canonicalPath: string
  isGitRepository: boolean
  defaultBranch?: string
  createdAt: string
  lastOpenedAt: string
}

export interface Thread {
  id: string
  projectId: string
  title: string
  status: string
  createdAt: string
  updatedAt: string
  items?: ConversationItem[]
}

export interface TurnStart {
  threadId: string
  turnId: string
  status: string
}

export interface ConversationItem {
  id: string
  type: string
  text?: string
  command?: string
  cwd?: string
  status?: string
  aggregatedOutput?: string | null
  exitCode?: number | null
  changes?: Array<Record<string, unknown>>
  [key: string]: unknown
}

export interface Approval {
  requestId: string
  kind: 'command' | 'file'
  details: {
    itemId: string
    threadId: string
    turnId: string
    command?: string
    cwd?: string
    reason?: string
    grantRoot?: string
  }
}
