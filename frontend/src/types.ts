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
