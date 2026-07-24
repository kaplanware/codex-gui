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

export interface RateLimitWindow {
  usedPercent: number
  remainingPercent: number
  resetsAt?: number
  windowDurationMins?: number
}

export interface RateLimits {
  limitName?: string
  primary?: RateLimitWindow
  secondary?: RateLimitWindow
}

export interface Model {
  id: string
  displayName: string
  description: string
  isDefault: boolean
  defaultReasoningEffort: string
  supportedReasoningEfforts: string[]
}

export interface FileMatch {
  name: string
  path: string
  relativePath: string
  score: number
}

export interface MentionInput {
  name: string
  path: string
}

export interface TurnOptions {
  model?: string
  effort?: string
  mode?: 'default' | 'plan'
  mentions?: MentionInput[]
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
  kind: 'command' | 'file' | 'permissions'
  details: {
    itemId: string
    threadId: string
    turnId: string
    command?: string
    cwd?: string
    reason?: string
    grantRoot?: string
    permissions?: Record<string, unknown>
  }
}

export interface UserInputQuestion {
  id: string
  header: string
  question: string
  isOther: boolean
  isSecret: boolean
  options: Array<{ label: string; description: string }> | null
}

export interface UserInputRequest {
  requestId: string
  details: {
    questions: UserInputQuestion[]
  }
}

export interface ElicitationRequest {
  requestId: string
  details: {
    mode: string
    message: string
    url?: string
    requestedSchema?: {
      properties?: Record<string, { title?: string; description?: string; type?: string }>
      required?: string[]
    }
  }
}
