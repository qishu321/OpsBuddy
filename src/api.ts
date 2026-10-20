export interface Project {
  importKey?: string;
  id: string;
  name: string;
  description: string;
  createdAt: string;
}
export interface Role {
  migratedSkillId?: string;
  knowledgeIds?: string[];
  id: string;
  name: string;
  description: string;
  instructions: string;
  kind: string;
  skillIds: string[];
  builtin: boolean;
}
export interface Model {
  id: string;
  name: string;
  family: string;
  baseUrl: string;
  modelId: string;
  hasKey: boolean;
  testedAt?: string;
}
export interface Extension {
  sourceRoleId?: string;
  dependencies?: string[];
  knowledgeIds?: string[];
  builtin?: boolean;
  category?: string;
  id: string;
  name: string;
  kind: string;
  description: string;
  content?: string;
  url?: string;
  enabled: boolean;
  version: string;
  status: string;
}
export interface Asset {
  requireRunConfirmation?: boolean;
  sourceName?: string;
  id: string;
  name: string;
  kind: string;
  size: number;
  projectId: string;
  taskId?: string;
  parentId?: string;
  preview?: string;
  validation: string;
  createdAt: string;
}
export interface Task {
  scheduleId?: string;
  scheduleRunId?: string;
  conversationId?: string;
  agentGroupId?: string;
  agentMemberId?: string;
  usedSkillIds?: string[];
  workflowRunId?: string;
  workflowNodeId?: string;
  configSnapshot?: {
    agentStage?: string;
    agentCandidateIds?: string[];
    ownedPaths?: string[];
    memoryRefs?: MemoryRef[];
    knowledgeIds?: string[];
    knowledgeRefs?: KnowledgeHit[];
    lazySkills?: boolean;
    conversationContext?: string;
    projectInstructions?: string;
    schemaVersion: number;
    capturedAt: string;
    executor: string;
    mode: 'local' | 'model';
    outputKind: string;
    allowRemote: boolean;
    role: Role;
    roleHash: string;
    model?: Omit<Model, 'hasKey' | 'testedAt'> & { credentialRef?: string };
    extensions: (Extension & { contentHash: string })[];
  };
  id: string;
  projectId: string;
  parentId?: string;
  title: string;
  prompt: string;
  kind: string;
  roleId: string;
  modelId: string;
  modelName: string;
  status: string;
  inputIds: string[];
  outputIds: string[];
  steps: { name: string; status: string; detail: string; at: string }[];
  result: string;
  error?: string;
  createdAt: string;
  updatedAt: string;
  archived: boolean;
}
export interface State {
  startupImport?: ProjectImportResult;
  memories: Memory[];
  schedules: Schedule[];
  scheduleRuns: ScheduleRun[];
  directories: DirectoryGrant[];
  notifications: Notification[];
  agentGroups: AgentGroup[];
  knowledgeBases: KnowledgeBase[];
  knowledgeDocuments: KnowledgeDocument[];
  workflows: WorkflowDefinition[];
  workflowRuns: WorkflowRun[];
  projects: Project[];
  roles: Role[];
  models: Model[];
  extensions: Extension[];
  assets: Asset[];
  tasks: Task[];
}

export interface MemoryRef {
  id: string;
  scope: 'personal' | 'project';
  title: string;
  content: string;
  version: number;
  contentHash: string;
}
export interface Memory {
  id: string;
  projectId?: string;
  scope: 'personal' | 'project';
  kind: 'preference' | 'experience';
  title: string;
  content: string;
  keywords: string;
  enabled: boolean;
  sourceTaskId?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}
export interface DirectoryGrant {
  id: string;
  projectId: string;
  name: string;
  path: string;
  createdAt: string;
}
export interface Schedule {
  id: string;
  name: string;
  projectId: string;
  mode: 'task' | 'reminder';
  prompt: string;
  kind: string;
  roleId: string;
  modelId: string;
  allowRemote: boolean;
  inputIds: string[];
  skillIds: string[];
  knowledgeIds: string[];
  useMemory?: boolean;
  frequency: 'once' | 'daily' | 'weekly';
  timezone: string;
  startDate: string;
  clock: string;
  weekdays: number[];
  missedPolicy: 'skip' | 'catch_up_once';
  directoryId?: string;
  status: 'active' | 'paused' | 'completed';
  nextAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}
export interface ScheduleRun {
  id: string;
  scheduleId: string;
  taskId?: string;
  scheduledAt: string;
  trigger: 'manual' | 'timer' | 'catch_up';
  status: string;
  error?: string;
  createdAt: string;
  finishedAt?: string;
  definition: Schedule;
}
export interface Notification {
  id: string;
  taskId?: string;
  runId: string;
  title: string;
  body: string;
  createdAt: string;
  readAt?: string;
}
export interface KnowledgeBase {
  id: string;
  projectId: string;
  name: string;
  description: string;
  createdAt: string;
}
export interface KnowledgeChunk {
  index: number;
  text: string;
  page?: number;
}
export interface KnowledgeDocument {
  id: string;
  baseId: string;
  assetId: string;
  name: string;
  status: string;
  error?: string;
  chunkCount: number;
  contentHash: string;
  indexedAt: string;
  chunks?: KnowledgeChunk[];
}
export interface KnowledgeHit {
  baseId: string;
  documentId: string;
  assetId: string;
  name: string;
  chunkIndex: number;
  page?: number;
  text: string;
  score: number;
  contentHash: string;
}
export interface WorkflowNode {
  skillIds?: string[];
  joinMode?: string;
  id: string;
  type: string;
  label: string;
  x: number;
  y: number;
  prompt?: string;
  kind?: string;
  roleId?: string;
  modelId?: string;
  baseIds?: string[];
  query?: string;
  operator?: string;
  value?: string;
  field?: string;
}
export interface WorkflowEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
}
export interface WorkflowDefinition {
  id: string;
  projectId: string;
  name: string;
  description: string;
  version: number;
  enabled: boolean;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  updatedAt: string;
}
export interface WorkflowNodeRun {
  id: string;
  nodeId: string;
  label: string;
  status: string;
  taskId?: string;
  text?: string;
  assetIds: string[];
  knowledgeRefs?: KnowledgeHit[];
  branch?: string;
  error?: string;
  startedAt?: string;
  finishedAt?: string;
}
export interface WorkflowRun {
  id: string;
  workflowId: string;
  projectId: string;
  status: string;
  prompt: string;
  inputIds: string[];
  definition: WorkflowDefinition;
  nodes: WorkflowNodeRun[];
  outputIds: string[];
  error?: string;
  createdAt: string;
  finishedAt?: string;
}
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set('X-OpsBuddy-Request', '1');
  if (options.body && !(options.body instanceof FormData))
    headers.set('Content-Type', 'application/json');
  const response = await fetch('/api' + path, { ...options, headers });
  if (!response.ok) {
    let message = '请求失败，请稍后重试';
    try {
      message = (await response.json()).error || message;
    } catch {}
    throw new Error(message);
  }
  return response.json();
}
export const post = <T>(path: string, data: unknown = {}) =>
  api<T>(path, { method: 'POST', body: JSON.stringify(data) });
export const fileURL = (a: Asset, inline = false) =>
  `/api/assets/${a.id}/file${inline ? '?inline=1' : ''}`;
export const size = (n: number) =>
  n > 1024 * 1024
    ? (n / 1024 / 1024).toFixed(1) + ' MB'
    : Math.max(1, Math.round(n / 1024)) + ' KB';
export const kinds: Record<string, string> = {
  auto: '自动选择',
  chat: '对话问答',
  md: 'Markdown 文档',
  html: 'HTML 网页',
  code: '代码项目（含后端）',
  docx: 'Word 文档',
  pptx: 'PPT 演示',
  xlsx: 'Excel 表格',
  pdf: 'PDF 处理',
  organize: '文件整理',
};
export const statusLabel: Record<string, string> = {
  paused: '已暂停',
  planning: '制定计划中',
  blocked: '已阻塞',
  partially_succeeded: '部分完成',
  waiting: '等待子任务',
  pending: '待派发',
  aggregating: '汇总中',
  queued: '排队中',
  running: '处理中',
  succeeded: '已完成',
  failed: '未完成',
  cancelled: '已停止',
  interrupted: '已中断',
};

export interface ProjectImportResult {
  bundleId: string;
  projectId: string;
  taskId: string;
  alreadyImported: boolean;
}
export interface AgentNodeSpec {
  id: string;
  role: string;
  name: string;
  prompt: string;
  modelId?: string;
  kind?: string;
  dependsOn?: string[];
  inputIds?: string[];
  skillIds?: string[];
  ownedPaths?: string[];
  acceptance?: string;
  failurePolicy?: string;
}
export interface AgentGroup {
  id: string;
  projectId: string;
  rootTaskId: string;
  sourceTaskId?: string;
  prompt: string;
  status: string;
  createdAt: string;
  finishedAt?: string;
  error?: string;
  schemaVersion?: number;
  planVersion?: number;
  autoResume?: boolean;
  resumeCount?: number;
  settling?: boolean;
  modelCatalog?: { id: string; name: string; family: string; baseUrl: string; modelId: string }[];
  planHistory?: {
    version: number;
    at: string;
    plan: { summary: string; members: AgentNodeSpec[] };
  }[];
  plannerTaskId?: string;
  plan?: { summary: string; members: AgentNodeSpec[] };
  reworkRound?: number;
  limits?: {
    members: number;
    concurrency: number;
    depth: number;
    retries: number;
    reworkRounds: number;
  };
  reviews?: {
    status: 'pass' | 'rework' | 'blocked';
    summary: string;
    issues: { nodeId: string; reason: string }[];
    taskId: string;
    round: number;
    at: string;
  }[];
  members: {
    id: string;
    role: string;
    name: string;
    prompt: string;
    status: string;
    taskIds: string[];
    pending: string[];
    instructions: string[];
    dependsOn?: string[];
    acceptance?: string;
    error?: string;
    retries?: number;
    configSnapshot?: Task['configSnapshot'];
  }[];
}
