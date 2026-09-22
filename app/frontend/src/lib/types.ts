// Mirrors internal/protocol (Go). Keep field names in sync with the JSON tags there.

export type Complexity = '' | 'auto' | 'quick' | 'standard' | 'deep';

export interface ModelSelection {
  provider?: string;
  model?: string;
  complexity?: Complexity;
  credentialId?: string;
}

export interface UsageTotals {
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  reasoningTokens: number;
  costUsd: number;
  requests: number;
  estimated?: boolean;
}

export interface Thread {
  id: string;
  title: string;
  channel: string;
  pinned: boolean;
  archived: boolean;
  settings: ModelSelection;
  forkedFrom?: string;
  usage: UsageTotals;
  createdAt: string;
  updatedAt: string;
}

export type TurnStatus = 'running' | 'completed' | 'failed' | 'interrupted';

export interface Turn {
  id: string;
  threadId: string;
  status: TurnStatus;
  selection: ModelSelection;
  resolved: ModelSelection;
  autoPicked?: boolean;
  error?: string;
  usage: UsageTotals;
  startedAt: string;
  finishedAt?: string;
}

export type ItemKind = 'userMessage' | 'agentMessage' | 'reasoning' | 'toolCall' | 'inboundEvent' | 'approval' | 'error';
export type ItemStatus = 'inProgress' | 'completed' | 'failed' | 'denied';

export interface ToolCallData {
  callId: string;
  name: string;
  args: unknown;
  output?: string;
  error?: string;
  risk?: string;
}

export interface Item {
  id: string;
  threadId: string;
  turnId: string;
  seq: number;
  kind: ItemKind;
  status: ItemStatus;
  text?: string;
  tool?: ToolCallData;
  data?: unknown;
  createdAt: string;
}

export interface Attachment {
  name: string;
  mimeType: string;
  dataB64: string;
}

export interface Approval {
  id: string;
  threadId: string;
  turnId: string;
  itemId: string;
  tool: string;
  args: unknown;
  risk: string;
  reason: string;
  actionSummary: string;
  status: 'pending' | 'approved' | 'denied' | 'expired';
  decidedBy?: string;
  createdAt: string;
  expiresAt: string;
  decidedAt?: string;
}

export interface Credential {
  id: string;
  provider: string;
  label: string;
  baseUrl?: string;
  last4: string;
  enabled: boolean;
  isDefault: boolean;
  fallback: boolean;
  monthlyBudgetUsd?: number;
  hardStop?: boolean;
  lastTestedAt?: string;
  lastTestOk?: boolean;
  createdAt: string;
  monthUsage?: UsageTotals;
}

export interface Model {
  provider: string;
  id: string;
  displayName: string;
  contextWindow?: number;
  supportsTools: boolean;
  supportsImages: boolean;
  supportsReasoning: boolean;
  inputPerMTok: number;
  cachedInputPerMTok: number;
  outputPerMTok: number;
  hidden: boolean;
  source: string;
}

export interface Provider {
  id: string;
  displayName: string;
  enabled: boolean;
  defaultModel: string;
  credentials: number;
}

export interface ComplexityPreset {
  level: Complexity;
  reasoning: string;
  maxToolSteps: number;
  multiAgent: string;
  maxOutputTokens: number;
}

export interface ComplexityDefaults {
  default: Complexity;
  presets: ComplexityPreset[];
}

export interface EngineStatus {
  engineVersion: string;
  protocolVersion: string;
  startedAt: string;
  clients: number;
  activeTurns: number;
  pendingApprovals: number;
  dbPath: string;
}

export interface SearchHit {
  threadId: string;
  title: string;
  itemId: string;
  snippet: string;
}

export interface UsageRow {
  key: string;
  label: string;
  usage: UsageTotals;
}

export interface UsageSummary {
  rows: UsageRow[];
  total: UsageTotals;
}

export interface CredentialTestResult {
  ok: boolean;
  latencyMs: number;
  error?: string;
  models?: string[];
}

export interface Schedule {
  run_at?: string;
  frequency?: string;
  time?: string;
  minute?: number;
  day_of_week?: string;
  cron?: string;
}

export interface Task {
  id: number;
  name: string;
  prompt: string;
  taskType: 'one_time' | 'periodic';
  schedule: Schedule;
  timezone: string;
  status: 'active' | 'completed' | 'cancelled';
  settings: ModelSelection;
  threadId?: string;
  nextRunAt?: string;
  lastRunAt?: string;
  lastResult?: string;
  lastError?: string;
  createdBy: string;
  createdAt: string;
}

export interface TaskRun {
  id: number;
  taskId: number;
  status: 'running' | 'success' | 'failed';
  turnId?: string;
  startedAt: string;
  finishedAt?: string;
  result?: string;
  error?: string;
}

export interface SkillInfo {
  name: string;
  description: string;
  version?: string;
  runtime: string;
  riskLevel: string;
  scripts: string[];
  dir: string;
  removable: boolean;
  error?: string;
}

export interface MCPServer {
  name: string;
  transport: string;
  status: string;
  error?: string;
  serverName?: string;
  serverVersion?: string;
  tools: string[];
}

export interface ToolInfo {
  name: string;
  description: string;
  schema: unknown;
  source: string;
}

export interface BudgetWarning {
  credentialId: string;
  label: string;
  spentUsd: number;
  budgetUsd: number;
  percent: number;
}
