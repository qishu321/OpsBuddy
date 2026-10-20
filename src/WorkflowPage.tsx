import { useState, useCallback, useRef, useMemo, useEffect } from 'react';
import {
  ReactFlow,
  Background,
  MiniMap,
  Handle,
  Position,
  addEdge,
  useNodesState,
  useEdgesState,
  MarkerType,
  type Node,
  type NodeProps,
  type Connection,
  type Edge,
  type ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {
  Plus,
  Play,
  Save,
  ArrowLeft,
  Workflow,
  GitBranch,
  Bot,
  BookOpen,
  Merge,
  Flag,
  Square,
  CheckCircle2,
  Loader2,
  AlertCircle,
  Copy,
  History,
  Undo2,
  Redo2,
  Trash2,
  X,
  Files,
  ChevronRight,
  Paperclip,
  Wrench,
  LayoutGrid,
  Maximize2,
  ZoomIn,
  ZoomOut,
  Map as MapIcon,
  MousePointer2,
} from 'lucide-react';
import {
  api,
  post,
  statusLabel,
  kinds,
  type State,
  type WorkflowDefinition,
  type WorkflowNode,
  type WorkflowRun,
  type WorkflowNodeRun,
  type Asset,
} from './api';
import { desktop } from './desktop';
import { FitWorkflowView } from './GraphViewport';
import AgentPlanCanvas from './AgentPlanCanvas';

type CanvasData = Record<string, unknown> & { spec: WorkflowNode; run?: WorkflowNodeRun };
type CanvasNode = Node<CanvasData, 'ops'>;
const uid = () => crypto.randomUUID();
const types = [
  { type: 'start', label: '开始', icon: Play },
  { type: 'agent', label: 'Agent', icon: Bot },
  { type: 'knowledge', label: '知识检索', icon: BookOpen },
  { type: 'condition', label: '条件分支', icon: GitBranch },
  { type: 'tool', label: '办公工具', icon: Wrench },
  { type: 'merge', label: '汇合', icon: Merge },
  { type: 'end', label: '结束', icon: Flag },
];
const labels: Record<string, string> = {
  pending: '待执行',
  running: '执行中',
  succeeded: '成功',
  skipped: '已跳过',
  failed: '失败',
  cancelled: '已停止',
  interrupted: '已中断',
};
const descriptions: Record<string, string> = {
  agent: '按需调用技能，执行 AI 任务',
  knowledge: '从项目资料中检索依据',
  condition: '按条件选择执行路径',
  tool: '生成文档、表格与演示',
  merge: '汇总多个分支的成果',
};
function GraphNode({ data, selected }: NodeProps<CanvasNode>) {
  const n = data.spec,
    info = types.find((t) => t.type === n.type)!,
    Icon = info?.icon || Bot;
  return (
    <div className={`ops-node ${selected ? 'selected' : ''} ${n.type} ${data.run?.status || ''}`}>
      {n.type !== 'start' && <Handle type="target" position={Position.Left} id="in" />}
      <div className="ops-node-heading">
        <span>
          <Icon size={17} />
        </span>
        <div className="node-title">
          <small>
            {n.type === 'start' ? '触发入口' : n.type === 'end' ? '结果交付' : info.label}
          </small>
          <strong>{n.label}</strong>
        </div>
        {data.run?.status === 'running' && <Loader2 size={13} className="spin" />}
        {data.run?.status === 'succeeded' && <CheckCircle2 size={13} />}
      </div>
      <div className="ops-node-body">
        <small>
          {n.kind
            ? kinds[n.kind]
            : n.type === 'start'
              ? '输入'
              : n.type === 'end'
                ? '输出'
                : '执行配置'}
        </small>
        <p>
          {data.run
            ? labels[data.run.status]
            : n.type === 'agent'
              ? n.modelId
                ? '已配置 AI 模型'
                : '未选择模型'
              : n.type === 'knowledge'
                ? `${n.baseIds?.length || 0} 个知识库`
                : n.type === 'condition'
                  ? `${n.field === 'context' ? '上游内容' : '任务输入'} ${n.operator === 'has_files' ? '包含文件' : n.operator === 'not_contains' ? '不包含' : '包含'} ${n.value || ''}`
                  : n.type === 'merge'
                    ? '等待所有有效上游完成'
                    : n.type === 'start'
                      ? '任务要求与附件'
                      : n.type === 'end'
                        ? '收集流程交付'
                        : '调用本地办公处理器'}
        </p>
      </div>
      <div className="node-foot">
        <span className="node-ready-dot" />
        {data.run
          ? labels[data.run.status]
          : n.type === 'start'
            ? '接收任务与附件'
            : n.type === 'end'
              ? '交付处理结果'
              : '点击配置节点'}
        <ChevronRight size={12} />
      </div>
      {n.type === 'condition' ? (
        <>
          <span className="branch-tag true">TRUE</span>
          <Handle
            type="source"
            position={Position.Right}
            id="true"
            style={{ top: '64%', background: '#57a58e' }}
          />
          <span className="branch-tag false">FALSE</span>
          <Handle
            type="source"
            position={Position.Right}
            id="false"
            style={{ top: '78%', background: '#b89a6a' }}
          />
        </>
      ) : (
        n.type !== 'end' && <Handle type="source" position={Position.Right} id="out" />
      )}
    </div>
  );
}
const nodeTypes = { ops: GraphNode };
function asCanvas(n: WorkflowNode): CanvasNode {
  return {
    id: n.id,
    type: 'ops',
    deletable: !['start', 'end'].includes(n.type),
    position: { x: n.x, y: n.y },
    data: { spec: n },
  };
}
function asEdges(f: WorkflowDefinition): Edge[] {
  return f.edges.map((e) => ({
    ...e,
    targetHandle: 'in',
    type: 'smoothstep',
    label: e.sourceHandle === 'true' ? 'TRUE' : e.sourceHandle === 'false' ? 'FALSE' : undefined,
    markerEnd: { type: MarkerType.ArrowClosed },
    style: { stroke: e.sourceHandle === 'false' ? '#c68a43' : '#7b8fae', strokeWidth: 2 },
  }));
}
function template(projectId: string, type: string, baseId?: string): WorkflowDefinition {
  const node = (
    id: string,
    type: string,
    label: string,
    x: number,
    y: number,
    extra: Partial<WorkflowNode> = {},
  ): WorkflowNode => ({ id, type, label, x, y, ...extra });
  let nodes = [node('start', 'start', '开始', 30, 140), node('end', 'end', '结束', 430, 140)],
    edges = [{ id: 'e1', source: 'start', target: 'end', sourceHandle: 'out' }];
  let name = '新的工作流';
  if (type === 'office') {
    nodes[1].x = 990;
    name = '资料 → 文档 → 演示';
    nodes = [
      nodes[0],
      node('write', 'agent', '整理文档', 350, 140, {
        kind: 'docx',
        roleId: 'docx',
        prompt: '{{input}}\n请整理附件材料，输出结构清晰的文档。',
      }),
      node('slides', 'agent', '制作演示', 670, 140, {
        kind: 'pptx',
        roleId: 'pptx',
        prompt: '根据上游文档制作演示。任务目标：{{input}}',
      }),
      nodes[1],
    ];
    edges = [
      { id: 'e1', source: 'start', target: 'write', sourceHandle: 'out' },
      { id: 'e2', source: 'write', target: 'slides', sourceHandle: 'out' },
      { id: 'e3', source: 'slides', target: 'end', sourceHandle: 'out' },
    ];
  }
  if (type === 'branch') {
    name = '按要求选择交付格式';
    nodes = [
      node('start', 'start', '开始', 20, 180),
      node('condition', 'condition', '是否需要汇报', 270, 180, {
        field: 'input',
        operator: 'contains',
        value: '汇报',
      }),
      node('slides', 'agent', '制作汇报', 540, 40, {
        kind: 'pptx',
        roleId: 'pptx',
        prompt: '{{input}}',
      }),
      node('write', 'agent', '生成文档', 540, 330, {
        kind: 'docx',
        roleId: 'docx',
        prompt: '{{input}}',
      }),
      node('merge', 'merge', '汇合成果', 810, 180),
      node('end', 'end', '结束', 1080, 180),
    ];
    edges = [
      { id: 'e1', source: 'start', target: 'condition', sourceHandle: 'out' },
      { id: 'e2', source: 'condition', target: 'slides', sourceHandle: 'true' },
      { id: 'e3', source: 'condition', target: 'write', sourceHandle: 'false' },
      { id: 'e4', source: 'slides', target: 'merge', sourceHandle: 'out' },
      { id: 'e5', source: 'write', target: 'merge', sourceHandle: 'out' },
      { id: 'e6', source: 'merge', target: 'end', sourceHandle: 'out' },
    ];
  }
  if (type === 'knowledge') {
    nodes[1].x = 990;
    name = '知识检索 → 撰写文档';
    nodes = [
      nodes[0],
      node('knowledge', 'knowledge', '检索项目资料', 350, 140, {
        baseIds: baseId ? [baseId] : [],
        query: '{{input}}',
      }),
      node('write', 'agent', '根据资料撰写', 670, 140, {
        kind: 'docx',
        roleId: 'docx',
        prompt: '{{input}}\n引用知识检索节点的来源，缺失信息标待补充。',
      }),
      nodes[1],
    ];
    edges = [
      { id: 'e1', source: 'start', target: 'knowledge', sourceHandle: 'out' },
      { id: 'e2', source: 'knowledge', target: 'write', sourceHandle: 'out' },
      { id: 'e3', source: 'write', target: 'end', sourceHandle: 'out' },
    ];
  }
  if (type === 'meeting' || type === 'review' || type === 'sheet') {
    name =
      type === 'meeting'
        ? '会议纪要与行动项'
        : type === 'review'
          ? '撰写与交叉审阅'
          : '表格整理与汇总';
    nodes = [
      node('start', 'start', '开始', 30, 140),
      node(
        'work',
        'agent',
        type === 'meeting' ? '提炼纪要' : type === 'review' ? '撰写初稿' : '整理表格',
        350,
        140,
        {
          kind: type === 'sheet' ? 'xlsx' : 'chat',
          roleId: 'auto',
          prompt:
            type === 'meeting'
              ? '{{input}}\n根据材料输出会议结论、行动项、负责人和时间；未知信息明确标待补充。直接输出文字。'
              : type === 'review'
                ? '{{input}}\n依据材料撰写初稿，标注事实依据与缺失信息。直接输出文字。'
                : '{{input}}\n整理附件表格，保留原始数字和前导零，输出汇总表。',
        },
      ),
      node('end', 'end', '结束', 990, 140),
    ];
    edges = [{ id: 'e1', source: 'start', target: 'work', sourceHandle: 'out' }];
    if (type === 'review') {
      nodes.splice(
        2,
        0,
        node('review', 'agent', '审阅与定稿', 670, 140, {
          kind: 'chat',
          roleId: 'auto',
          prompt: '{{input}}\n检查上游初稿的事实、数字和用户约束，输出修订后的正文和待确认事项。',
        }),
      );
      edges.push(
        { id: 'e2', source: 'work', target: 'review', sourceHandle: 'out' },
        { id: 'e3', source: 'review', target: 'end', sourceHandle: 'out' },
      );
    } else {
      nodes[2].x = 670;
      edges.push({ id: 'e2', source: 'work', target: 'end', sourceHandle: 'out' });
    }
  }
  return {
    id: '',
    projectId,
    name,
    description: '',
    version: 0,
    enabled: true,
    nodes,
    edges,
    updatedAt: '',
  };
}

export default function WorkflowPage({
  state,
  projectId,
  onRefresh,
  onTask,
}: {
  state: State;
  projectId: string;
  onRefresh: () => Promise<void>;
  onTask: (id: string) => void;
}) {
  const [draft, setDraft] = useState<WorkflowDefinition | null>(null),
    [agentGroupId, setAgentGroupId] = useState(''),
    [nodes, setNodes, onNodesChange] = useNodesState<CanvasNode>([]),
    [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]),
    [selection, setSelection] = useState(''),
    [runId, setRunId] = useState(''),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [dirty, setDirty] = useState(false),
    [runDialog, setRunDialog] = useState(false),
    [runPrompt, setRunPrompt] = useState(''),
    [inputIds, setInputIds] = useState<string[]>([]),
    [allowRemote, setAllowRemote] = useState(false),
    [canvas, setCanvas] = useState<ReactFlowInstance<CanvasNode> | null>(null),
    [zoom, setZoom] = useState(1),
    [showMap, setShowMap] = useState(false),
    [tab, setTab] = useState<'node' | 'runs'>('node');
  const undo = useRef<{ nodes: CanvasNode[]; edges: Edge[] }[]>([]),
    redo = useRef<{ nodes: CanvasNode[]; edges: Edge[] }[]>([]);
  const workflows = state.workflows.filter((f) => f.projectId === projectId);
  const bases = state.knowledgeBases.filter((b) => b.projectId === projectId);
  const selectedRun = state.workflowRuns.find((r) => r.id === runId);
  const viewNodes = useMemo(
    () =>
      selectedRun
        ? selectedRun.definition.nodes.map((n) => ({
            ...asCanvas(n),
            data: { spec: n, run: selectedRun.nodes.find((x) => x.nodeId === n.id) },
          }))
        : nodes,
    [selectedRun, nodes],
  );
  const viewEdges = useMemo(
    () => (selectedRun ? asEdges(selectedRun.definition) : edges),
    [selectedRun, edges],
  );
  const selectedSpec = viewNodes.find((n) => n.id === selection)?.data.spec;
  const nodeRun = selectedRun?.nodes.find((n) => n.nodeId === selection);
  const resultNode =
    selectedRun &&
    (nodeRun?.text && ['agent', 'tool', 'knowledge'].includes(selectedSpec?.type || '')
      ? nodeRun
      : [...selectedRun.nodes]
          .reverse()
          .find(
            (n) =>
              n.text &&
              ['agent', 'tool', 'knowledge'].includes(
                selectedRun.definition.nodes.find((spec) => spec.id === n.nodeId)?.type || '',
              ),
          ));
  const resultAssets = (
    resultNode?.assetIds?.length ? resultNode.assetIds : selectedRun?.outputIds || []
  )
    .map((id) => state.assets.find((a) => a.id === id))
    .filter((a): a is Asset => !!a);
  const runs = state.workflowRuns
    .filter((r) => r.workflowId === draft?.id)
    .slice()
    .reverse();
  const perform = async (action: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await action();
      await onRefresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const remember = () => {
    undo.current = [...undo.current.slice(-39), structuredClone({ nodes, edges })];
    redo.current = [];
    setDirty(true);
  };
  const open = (f: WorkflowDefinition) => {
    const opened = structuredClone(f);
    const preferred =
      state.models.find((m) => m.id === localStorage.getItem('opsbuddy-selected-model')) ||
      state.models[0];
    let configured = false;
    for (const n of opened.nodes)
      if (n.type === 'agent' && !n.modelId && preferred) {
        n.modelId = preferred.id;
        configured = true;
      }
    setDraft(opened);
    setNodes(opened.nodes.map(asCanvas));
    setEdges(asEdges(f));
    setSelection('');
    setRunId('');
    setError('');
    setNotice('');
    setDirty(!f.id || configured);
    undo.current = [];
    redo.current = [];
    setTab('node');
  };
  const connect = useCallback(
    (c: Connection) => {
      remember();
      setEdges((eds) =>
        addEdge(
          {
            ...c,
            id: uid(),
            type: 'smoothstep',
            label:
              c.sourceHandle === 'true' ? 'TRUE' : c.sourceHandle === 'false' ? 'FALSE' : undefined,
            markerEnd: { type: MarkerType.ArrowClosed },
          },
          eds,
        ),
      );
    },
    [nodes, edges],
  );
  const save = async () => {
    if (!draft) throw new Error('请先创建流程');
    const f = {
      ...draft,
      nodes: nodes.map((n) => ({ ...n.data.spec, x: n.position.x, y: n.position.y })),
      edges: edges.map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        sourceHandle: e.sourceHandle || 'out',
      })),
    };
    const saved = await post<WorkflowDefinition>('/workflows', f);
    setDraft(saved);
    setDirty(false);
    setNotice(`已保存 · 版本 ${saved.version}`);
    return saved;
  };
  const add = (type: string) => {
    if (type === 'start' || type === 'end') return;
    remember();
    const n: WorkflowNode = {
      id: uid(),
      type,
      label: types.find((t) => t.type === type)!.label,
      x: (nodes.find((n) => n.id === selection)?.position.x ?? 30) + 320,
      y: nodes.find((n) => n.id === selection)?.position.y ?? 140,
    };
    while (
      nodes.some(
        (other) => Math.abs(other.position.x - n.x) < 250 && Math.abs(other.position.y - n.y) < 200,
      )
    )
      n.y += 220;
    if (type === 'agent' || type === 'tool') {
      n.kind = type === 'agent' ? 'chat' : 'docx';
      n.roleId = 'auto';
      n.prompt = '{{input}}';
      if (type === 'agent')
        n.modelId =
          state.models.find((m) => m.id === localStorage.getItem('opsbuddy-selected-model'))?.id ||
          state.models[0]?.id ||
          '';
    }
    if (type === 'knowledge') {
      n.baseIds = bases[0] ? [bases[0].id] : [];
      n.query = '{{input}}';
    }
    if (type === 'condition') {
      n.field = 'input';
      n.operator = 'contains';
      n.value = '汇报';
    }
    if (
      nodes.length === 2 &&
      edges.length === 1 &&
      nodes.some((x) => x.id === edges[0].source && x.data.spec.type === 'start') &&
      nodes.some((x) => x.id === edges[0].target && x.data.spec.type === 'end')
    ) {
      const start = nodes.find((x) => x.data.spec.type === 'start')!;
      const end = nodes.find((x) => x.data.spec.type === 'end')!;
      n.x = start.position.x + 320;
      n.y = start.position.y;
      setNodes([start, asCanvas(n), { ...end, position: { x: n.x + 320, y: n.y } }]);
      setEdges(
        asEdges({
          ...draft!,
          edges: [
            { id: uid(), source: start.id, target: n.id, sourceHandle: 'out' },
            // A condition needs two explicitly configured branches.
            ...(type === 'condition'
              ? []
              : [{ id: uid(), source: n.id, target: end.id, sourceHandle: 'out' }]),
          ],
        }),
      );
    } else setNodes((prev) => [...prev, asCanvas(n)]);
    setSelection(n.id);
    requestAnimationFrame(() =>
      requestAnimationFrame(() => canvas?.fitView({ padding: 0.22, maxZoom: 1, duration: 280 })),
    );
  };
  const arrange = () => {
    if (selectedRun) return;
    remember();
    const remaining = new Set(nodes.map((n) => n.id));
    const levels = new Map<string, number>();
    while (remaining.size) {
      const ready = [...remaining].filter((id) =>
        edges.filter((e) => e.target === id).every((e) => !remaining.has(e.source)),
      );
      // Keep an editable graph recoverable even before DAG validation on save.
      if (!ready.length) ready.push(remaining.values().next().value!);
      for (const id of ready) {
        levels.set(
          id,
          Math.max(
            0,
            ...edges.filter((e) => e.target === id).map((e) => (levels.get(e.source) ?? -1) + 1),
          ),
        );
        remaining.delete(id);
      }
    }
    const counts = new Map<number, number>();
    setNodes(
      nodes.map((n) => {
        const level = levels.get(n.id) || 0;
        const row = counts.get(level) || 0;
        counts.set(level, row + 1);
        return { ...n, position: { x: 40 + level * 320, y: 100 + row * 240 } };
      }),
    );
    requestAnimationFrame(() =>
      requestAnimationFrame(() => canvas?.fitView({ padding: 0.22, maxZoom: 1, duration: 300 })),
    );
  };
  const edit = (values: Partial<WorkflowNode>) => {
    remember();
    setNodes((prev) =>
      prev.map((n) =>
        n.id === selection ? { ...n, data: { ...n.data, spec: { ...n.data.spec, ...values } } } : n,
      ),
    );
  };
  const history = (back: boolean) => {
    const src = back ? undo : redo,
      dst = back ? redo : undo;
    const entry = src.current.pop();
    if (!entry) return;
    dst.current.push(structuredClone({ nodes, edges }));
    setNodes(entry.nodes);
    setEdges(entry.edges);
    setDirty(true);
  };
  const execute = () =>
    perform(async () => {
      const saved = dirty || !draft?.id ? await save() : draft!;
      if (
        saved.nodes.some((n) => n.type === 'agent' && !state.models.some((m) => m.id === n.modelId))
      )
        throw new Error('请为每个 Agent 节点选择模型');
      const run = await post<WorkflowRun>(`/workflows/${saved.id}/runs`, {
        prompt: runPrompt,
        inputIds,
        allowRemote,
      });
      setRunId(run.id);
      setRunDialog(false);
      setTab('runs');
      setNotice('流程已提交，节点将按连线调度');
    });
  const pickRunInputs = () =>
    perform(async () => {
      const native = desktop();
      if (!native) return;
      const assets = await native.PickFiles(projectId);
      setInputIds((prev) => [...new Set([...prev, ...assets.map((a) => a.id)])]);
    });
  const saveAsset = (a: Asset) =>
    perform(async () => {
      const native = desktop();
      if (native) {
        const target = await native.SaveAsset(a.id);
        if (target) setNotice('已另存成果');
      } else {
        const link = document.createElement('a');
        link.href = `/api/assets/${a.id}/file`;
        link.click();
      }
    });
  if (!draft)
    return (
      <div className="flow-page">
        <div className="page-title">
          <div>
            <span className="eyebrow">WORKFLOWS</span>
            <h1>把分工，连成完整的工作。</h1>
            <p>连接 Agent、知识库与办公工具，保存后直接执行。</p>
          </div>
          <button className="button primary" onClick={() => open(template(projectId, 'blank'))}>
            <Plus size={16} />
            新建工作流
          </button>
        </div>
        <div className="flow-template-row">
          {[
            { id: 'meeting', name: '会议纪要', desc: '提炼结论与行动项 · 文字输出', icon: Bot },
            {
              id: 'review',
              name: '撰写与审阅',
              desc: '初稿 → 审阅 → 定稿 · 文字输出',
              icon: CheckCircle2,
            },
            { id: 'sheet', name: '表格汇总', desc: '整理材料 → 可编辑表格', icon: Files },
            { id: 'office', name: '资料到汇报', desc: '文档整理 → PPT 交付', icon: Bot },
            { id: 'knowledge', name: '从知识库撰写', desc: '检索资料 → 引用生成', icon: BookOpen },
            { id: 'branch', name: '条件分支', desc: '根据任务选择不同交付', icon: GitBranch },
          ].map(({ icon: Icon, ...t }) => (
            <button key={t.id} onClick={() => open(template(projectId, t.id, bases[0]?.id))}>
              <Icon size={22} />
              <strong>{t.name}</strong>
              <span>{t.desc}</span>
              <Plus size={16} />
            </button>
          ))}
        </div>
        {!!state.agentGroups.filter((g) => g.projectId === projectId).length && (
          <>
            <div className="section-heading">
              <h3>项目协作</h3>
              <span>主会话中的真实编排与进度</span>
            </div>
            <div className="saved-workflows agent-workflow-cards">
              {state.agentGroups
                .filter((g) => g.projectId === projectId)
                .slice()
                .reverse()
                .map((g) => (
                  <article key={g.id}>
                    <div className="workflow-card-icon">
                      <Workflow size={23} />
                    </div>
                    <h3>{g.prompt}</h3>
                    <p>
                      {g.members.length} 位成员 · 计划 v{g.planVersion || 1} ·{' '}
                      {statusLabel[g.status]}
                    </p>
                    <div className="card-actions">
                      <button
                        className="button primary"
                        disabled={!g.plan?.members.length}
                        onClick={() => setAgentGroupId(g.id)}
                      >
                        查看编排
                      </button>
                      <button className="button" onClick={() => onTask(g.rootTaskId)}>
                        打开会话
                      </button>
                    </div>
                  </article>
                ))}
            </div>
            {state.agentGroups
              .filter((g) => g.projectId === projectId && g.id === agentGroupId)
              .map((g) => (
                <AgentPlanCanvas
                  key={g.id}
                  group={g}
                  state={state}
                  onRefresh={onRefresh}
                  onTask={(t) => {
                    setAgentGroupId('');
                    onTask(t.id);
                  }}
                  onClose={() => setAgentGroupId('')}
                />
              ))}
          </>
        )}
        <div className="section-heading">
          <h3>我的工作流 · {workflows.length}</h3>
          <span>当前项目 · 本地保存</span>
        </div>
        {workflows.length ? (
          <div className="saved-workflows">
            {workflows.map((f) => (
              <article key={f.id}>
                <div className="workflow-card-icon">
                  <Workflow size={25} />
                </div>
                <h3>{f.name}</h3>
                <p>{f.description || `${f.nodes.length} 个节点 · 版本 ${f.version}`}</p>
                <div className="workflow-card-meta">
                  <span>
                    {state.workflowRuns.filter((r) => r.workflowId === f.id).length} 次运行
                  </span>
                  <label className="workflow-enabled">
                    <span>{f.enabled ? '已启用' : '已停用'}</span>
                    <button
                      className={`toggle ${f.enabled ? 'on' : ''}`}
                      role="switch"
                      aria-label={`启用 ${f.name}`}
                      aria-checked={f.enabled}
                      disabled={busy}
                      onClick={() =>
                        perform(async () => {
                          await post('/workflows', { ...f, enabled: !f.enabled });
                        })
                      }
                    >
                      <span />
                    </button>
                  </label>
                </div>
                <div className="card-actions">
                  <button className="button primary" onClick={() => open(f)}>
                    编辑画布
                  </button>
                  <button
                    className="button"
                    onClick={() => {
                      open(f);
                      setTab('runs');
                    }}
                  >
                    <History size={14} />
                    运行记录
                  </button>
                  <button
                    className="icon-button"
                    title="复制流程"
                    onClick={() =>
                      open({ ...structuredClone(f), id: '', version: 0, name: f.name + ' · 副本' })
                    }
                  >
                    <Copy size={15} />
                  </button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="empty">
            <Workflow size={32} />
            <h3>从模板或空白画布开始</h3>
            <p>拖动节点，连接输入输出，再为每个 Agent 选择技能、模型与任务。</p>
          </div>
        )}
      </div>
    );
  return (
    <div className="flow-editor">
      <div className="flow-editor-bar">
        <button
          className="icon-button"
          title="返回工作流列表"
          onClick={() => {
            if (dirty) {
              setError('请先保存当前画布，再返回列表；也可点击放弃修改。');
              return;
            }
            setDraft(null);
          }}
        >
          <ArrowLeft size={18} />
        </button>
        <span className="editor-workflow-icon">
          <Workflow size={21} />
        </span>
        <input
          aria-label="工作流名称"
          value={draft.name}
          disabled={!!selectedRun}
          onChange={(e) => {
            setDraft({ ...draft, name: e.target.value });
            setDirty(true);
          }}
        />
        <span className="small-pill">
          {selectedRun
            ? `运行快照 v${selectedRun.definition.version}`
            : dirty
              ? '未保存'
              : `v${draft.version}`}
        </span>
        <div className="flow-bar-spacer" />
        {!selectedRun && (
          <label className="workflow-enabled editor-enabled">
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(e) => {
                setDraft({ ...draft, enabled: e.target.checked });
                setDirty(true);
              }}
            />
            启用
          </label>
        )}
        <button
          className="icon-button"
          title="查看运行记录"
          onClick={() => {
            setTab(tab === 'runs' ? 'node' : 'runs');
            setSelection('');
          }}
        >
          <History size={17} />
        </button>
        {selectedRun ? (
          <button
            className="button"
            onClick={() => {
              setRunId('');
              setTab('node');
            }}
          >
            返回编辑
          </button>
        ) : (
          <>
            <button
              className="icon-button"
              title="撤销"
              disabled={!undo.current.length}
              onClick={() => history(true)}
            >
              <Undo2 size={17} />
            </button>
            <button
              className="icon-button"
              title="重做"
              disabled={!redo.current.length}
              onClick={() => history(false)}
            >
              <Redo2 size={17} />
            </button>
            {dirty && (
              <button
                className="text-button"
                onClick={() => {
                  const original = state.workflows.find((f) => f.id === draft.id);
                  if (original) open(original);
                  else setDraft(null);
                }}
              >
                放弃修改
              </button>
            )}
            <button
              className="button"
              disabled={busy}
              onClick={() =>
                perform(async () => {
                  await save();
                })
              }
            >
              <Save size={15} />
              保存
            </button>
            <button
              className="button primary"
              disabled={busy}
              onClick={() => {
                setRunDialog(true);
                setAllowRemote(false);
              }}
            >
              <Play size={15} />
              执行
            </button>
          </>
        )}
      </div>
      {error && (
        <div className="module-error" role="alert">
          <AlertCircle size={15} />
          {error}
          <button onClick={() => setError('')}>
            <X size={14} />
          </button>
        </div>
      )}
      {notice && <div className="flow-notice">{notice}</div>}
      <div className="flow-editor-body">
        {!selectedRun && (
          <aside className="node-palette">
            <div className="palette-heading">
              <h3>节点库</h3>
              <span>5</span>
            </div>
            <p>组合能力，构建你的流程</p>
            <div className="palette-section-label">处理与逻辑</div>
            {types
              .filter((t) => t.type !== 'start' && t.type !== 'end')
              .map(({ type, label, icon: Icon }) => (
                <button
                  key={type}
                  className={`palette-node ${type}`}
                  title={`${label} · ${descriptions[type]}`}
                  aria-label={`添加${label}节点`}
                  onClick={() => add(type)}
                >
                  <span className="palette-node-icon">
                    <Icon size={18} />
                  </span>
                  <span className="palette-node-copy">
                    <strong>{label}</strong>
                    <small>{descriptions[type]}</small>
                  </span>
                  <Plus size={14} />
                </button>
              ))}
            <div className="palette-note">
              <MousePointer2 size={17} />
              <strong>连接你的想法</strong>
              <p>点击添加节点，拖动圆点连接。选中节点可编辑配置。</p>
              <span>
                <kbd>Delete</kbd> 删除选中的连线
              </span>
            </div>
          </aside>
        )}
        <div className="workflow-stage">
          <div className="flow-canvas" data-testid="workflow-canvas">
            <div className="canvas-heading">
              <span className="canvas-mode-dot" />
              {selectedRun ? '运行快照' : '流程画布'}
              <span>
                {viewNodes.length} 个节点 · {viewEdges.length} 条连接
              </span>
            </div>
            {!selectedRun && nodes.length === 2 && (
              <div className="canvas-onboarding">
                <strong>从一个处理节点开始</strong>
                <span>点击左侧 Agent 或办公工具，自动接入开始与结束之间。</span>
              </div>
            )}
            <ReactFlow<CanvasNode>
              key={selectedRun ? `run-${selectedRun.id}` : `edit-${draft.id || 'new'}`}
              onInit={setCanvas}
              onMove={(_, viewport) => setZoom(viewport.zoom)}
              nodes={viewNodes}
              edges={viewEdges}
              nodeTypes={nodeTypes}
              onNodesChange={
                selectedRun
                  ? undefined
                  : (changes) => {
                      if (
                        changes.some((c) =>
                          ['position', 'add', 'remove', 'replace'].includes(c.type),
                        )
                      )
                        setDirty(true);
                      if (changes.some((c) => c.type === 'remove')) remember();
                      onNodesChange(changes);
                    }
              }
              onEdgesChange={
                selectedRun
                  ? undefined
                  : (changes) => {
                      if (changes.some((c) => c.type !== 'select')) {
                        if (changes.some((c) => c.type === 'remove')) remember();
                        setDirty(true);
                      }
                      onEdgesChange(changes);
                    }
              }
              onConnect={selectedRun ? undefined : connect}
              onNodeDragStart={() => remember()}
              onNodeClick={(_, n) => {
                setSelection(n.id);
                setTab('node');
              }}
              nodesDraggable={!selectedRun}
              nodesConnectable={!selectedRun}
              elementsSelectable
              fitView
              fitViewOptions={{ padding: 0.22, maxZoom: 1 }}
              minZoom={0.1}
              maxZoom={1.5}
              defaultEdgeOptions={{
                type: 'smoothstep',
                style: { stroke: '#7b8fae', strokeWidth: 2 },
                markerEnd: { type: MarkerType.ArrowClosed },
              }}
              deleteKeyCode={selectedRun ? null : ['Backspace', 'Delete']}
            >
              <FitWorkflowView viewId={selectedRun?.id || draft.id || 'new'} />
              <Background gap={24} size={1} color="#c7d1e0" />
              {showMap && (
                <MiniMap
                  nodeColor={(n) => (n.type === 'ops' ? '#6586c5' : '#c9d3e5')}
                  pannable
                  zoomable
                />
              )}
            </ReactFlow>
            <div className="canvas-tools" role="toolbar" aria-label="画布工具">
              <button
                title="缩小"
                aria-label="缩小画布"
                onClick={() => canvas?.zoomOut({ duration: 180 })}
              >
                <ZoomOut size={16} />
              </button>
              <span className="canvas-zoom">{Math.round(zoom * 100)}%</span>
              <button
                title="放大"
                aria-label="放大画布"
                onClick={() => canvas?.zoomIn({ duration: 180 })}
              >
                <ZoomIn size={16} />
              </button>
              <i />
              <button
                title="适应画布"
                aria-label="适应画布"
                onClick={() => canvas?.fitView({ padding: 0.22, maxZoom: 1, duration: 250 })}
              >
                <Maximize2 size={16} />
              </button>
              {!selectedRun && (
                <button className="canvas-arrange" onClick={arrange}>
                  <LayoutGrid size={16} />
                  整理布局
                </button>
              )}
              <i />
              <button
                title="切换小地图"
                aria-label="切换小地图"
                aria-pressed={showMap}
                onClick={() => setShowMap(!showMap)}
              >
                <MapIcon size={16} />
              </button>
            </div>
            <div className="canvas-gesture-hint">拖动画布平移 · 滚轮缩放</div>
          </div>
          {selectedRun && (
            <section className="workflow-results" data-testid="workflow-results">
              <header>
                <div>
                  <strong>执行结果</strong>
                  <span>{resultNode?.label || '等待节点输出'}</span>
                </div>
                <span className={`node-status ${selectedRun.status}`}>
                  {statusLabel[selectedRun.status] || selectedRun.status}
                </span>
              </header>
              <div className="workflow-result-content">
                {resultNode ? (
                  <>
                    <pre>{resultNode.text}</pre>
                    {resultAssets.map((a) => (
                      <article className="workflow-result-file" key={a.id}>
                        <header>
                          <strong>
                            <Files size={14} />
                            {a.name}
                          </strong>
                          <button className="text-button" onClick={() => saveAsset(a)}>
                            另存文件
                          </button>
                        </header>
                        {a.preview && <pre>{a.preview.slice(0, 8000)}</pre>}
                      </article>
                    ))}
                    {resultNode.taskId && state.tasks.some((t) => t.id === resultNode.taskId) && (
                      <button className="text-button" onClick={() => onTask(resultNode.taskId!)}>
                        查看完整任务与结果 <ChevronRight size={13} />
                      </button>
                    )}
                  </>
                ) : (
                  <p>
                    {selectedRun.error ||
                      (['queued', 'running'].includes(selectedRun.status)
                        ? '流程执行后，文字和文件预览会显示在这里。'
                        : '本次流程没有可展示的节点输出。点击节点查看执行信息。')}
                  </p>
                )}
              </div>
            </section>
          )}
        </div>
        {(selection || tab === 'runs' || selectedRun) && (
          <aside className="flow-inspector">
            <div className="inspector-tabs">
              {!selectedRun && (
                <button
                  title="关闭属性"
                  className="icon-button"
                  onClick={() => {
                    setSelection('');
                    setTab('node');
                  }}
                >
                  <X size={13} />
                </button>
              )}
              <button className={tab === 'node' ? 'active' : ''} onClick={() => setTab('node')}>
                节点配置
              </button>
              <button className={tab === 'runs' ? 'active' : ''} onClick={() => setTab('runs')}>
                运行记录 <span>{runs.length}</span>
              </button>
            </div>
            {tab === 'node' ? (
              selectedSpec ? (
                <div className="inspector-content">
                  <div className="inspector-node-type">
                    {types.find((t) => t.type === selectedSpec.type)?.label}
                    {nodeRun && (
                      <span className={`node-status ${nodeRun.status}`}>
                        {labels[nodeRun.status]}
                      </span>
                    )}
                  </div>
                  <fieldset disabled={!!selectedRun}>
                    <label>
                      节点名称
                      <input
                        value={selectedSpec.label}
                        onChange={(e) => edit({ label: e.target.value })}
                      />
                    </label>
                    {(selectedSpec.type === 'agent' || selectedSpec.type === 'tool') && (
                      <>
                        <label>
                          交付格式
                          <select
                            value={selectedSpec.kind || 'docx'}
                            onChange={(e) => edit({ kind: e.target.value })}
                          >
                            {Object.entries(kinds)
                              .filter(
                                ([id]) =>
                                  id !== 'auto' &&
                                  (!['chat', 'md', 'html', 'code'].includes(id) ||
                                    selectedSpec.type === 'agent'),
                              )
                              .map(([id, name]) => (
                                <option key={id} value={id}>
                                  {name}
                                </option>
                              ))}
                          </select>
                        </label>
                        {selectedSpec.type === 'agent' && (
                          <label>
                            使用 Skills（可选）
                            <select
                              multiple
                              aria-label="节点技能"
                              value={[
                                ...(selectedSpec.skillIds || []),
                                ...state.roles
                                  .filter((r) => r.id === selectedSpec.roleId && r.migratedSkillId)
                                  .map((r) => r.migratedSkillId!),
                              ]}
                              onChange={(e) =>
                                edit({
                                  roleId: 'auto',
                                  skillIds: Array.from(e.target.selectedOptions, (o) => o.value),
                                })
                              }
                            >
                              {state.extensions
                                .filter((x) => x.kind === 'skill')
                                .map((x) => (
                                  <option key={x.id} value={x.id}>
                                    {x.name}
                                    {!x.enabled ? '（已停用）' : ''}
                                  </option>
                                ))}
                            </select>
                            <small>不选则自动匹配；多选使用 Ctrl。</small>
                          </label>
                        )}
                        {selectedSpec.type === 'agent' && (
                          <label>
                            模型
                            <select
                              value={selectedSpec.modelId || ''}
                              onChange={(e) => edit({ modelId: e.target.value })}
                            >
                              <option value="" disabled>
                                请选择模型
                              </option>
                              {state.models.map((m) => (
                                <option key={m.id} value={m.id}>
                                  {m.name}
                                </option>
                              ))}
                            </select>
                          </label>
                        )}
                        <label>
                          任务要求
                          <textarea
                            rows={6}
                            value={selectedSpec.prompt || ''}
                            onChange={(e) => edit({ prompt: e.target.value })}
                          />
                          <small>
                            {'{{input}}'} 表示本次任务输入；{'{{context}}'}{' '}
                            表示上游文字。上游文件会自动传递。
                          </small>
                        </label>
                      </>
                    )}
                    {(selectedSpec.type === 'knowledge' || selectedSpec.type === 'agent') && (
                      <div className="inspector-bases">
                        <strong>引用知识库</strong>
                        {bases.length ? (
                          bases.map((b) => (
                            <label key={b.id} className="check-label">
                              <input
                                type="checkbox"
                                checked={selectedSpec.baseIds?.includes(b.id) || false}
                                onChange={(e) =>
                                  edit({
                                    baseIds: e.target.checked
                                      ? [...(selectedSpec.baseIds || []), b.id]
                                      : (selectedSpec.baseIds || []).filter((id) => id !== b.id),
                                  })
                                }
                              />
                              {b.name}
                            </label>
                          ))
                        ) : (
                          <small>当前项目还没有知识库，请先到知识库页面创建。</small>
                        )}
                      </div>
                    )}
                    {selectedSpec.type === 'knowledge' && (
                      <label>
                        检索词
                        <textarea
                          rows={3}
                          value={selectedSpec.query || ''}
                          onChange={(e) => edit({ query: e.target.value })}
                        />
                        <small>支持 {'{{input}}'}。每次运行检索前 6 个命中片段，并固定来源。</small>
                      </label>
                    )}
                    {selectedSpec.type === 'condition' && (
                      <>
                        <label>
                          判断对象
                          <select
                            value={selectedSpec.field || 'input'}
                            onChange={(e) => edit({ field: e.target.value })}
                          >
                            <option value="input">任务输入</option>
                            <option value="context">上游文字</option>
                          </select>
                        </label>
                        <label>
                          判断规则
                          <select
                            value={selectedSpec.operator || 'contains'}
                            onChange={(e) => edit({ operator: e.target.value })}
                          >
                            <option value="contains">包含文字</option>
                            <option value="not_contains">不包含文字</option>
                            <option value="has_files">存在上游文件</option>
                          </select>
                        </label>
                        {selectedSpec.operator !== 'has_files' && (
                          <label>
                            匹配文字
                            <input
                              value={selectedSpec.value || ''}
                              onChange={(e) => edit({ value: e.target.value })}
                            />
                          </label>
                        )}
                        <p className="inspector-help">
                          TRUE 和 FALSE 分别连接不同分支；未选中的路径会标记为跳过。
                        </p>
                      </>
                    )}
                    {selectedSpec.type === 'merge' && (
                      <>
                        <label>
                          汇合策略
                          <select
                            value={selectedSpec.joinMode || 'all_success'}
                            onChange={(e) => edit({ joinMode: e.target.value })}
                          >
                            <option value="all_success">有效上游全部成功</option>
                            <option value="allow_partial">允许部分成功，交付已有成果</option>
                          </select>
                        </label>
                        <p className="inspector-help">
                          等待所有上游结束。条件跳过的分支不会阻塞；部分成功模式会保留失败记录并汇集成功分支。
                        </p>
                      </>
                    )}
                    {selectedSpec.type === 'start' && (
                      <p className="inspector-help">从执行对话框取得任务要求与附件。</p>
                    )}
                    {selectedSpec.type === 'end' && (
                      <p className="inspector-help">收集有效路径的文件作为最终成果。</p>
                    )}
                  </fieldset>
                  {nodeRun && (
                    <div className="node-run-detail">
                      <h3>执行结果</h3>
                      {nodeRun.error && <p className="module-error">{nodeRun.error}</p>}
                      {nodeRun.branch && <p>选择分支：{nodeRun.branch.toUpperCase()}</p>}
                      {nodeRun.text && <pre>{nodeRun.text}</pre>}
                      {nodeRun.knowledgeRefs?.map((ref, i) => (
                        <p className="citation" key={i}>
                          <BookOpen size={12} />
                          {ref.name} · 片段 {ref.chunkIndex + 1}
                          {ref.page ? ` · 第 ${ref.page} 页` : ''}
                        </p>
                      ))}
                      {nodeRun.taskId && (
                        <button className="button full" onClick={() => onTask(nodeRun.taskId!)}>
                          查看任务与成果 <ChevronRight size={14} />
                        </button>
                      )}
                    </div>
                  )}
                  {!selectedRun && !['start', 'end'].includes(selectedSpec.type) && (
                    <button
                      className="button full remove-node"
                      onClick={() => {
                        remember();
                        setNodes((prev) => prev.filter((n) => n.id !== selection));
                        setEdges((prev) =>
                          prev.filter((e) => e.source !== selection && e.target !== selection),
                        );
                        setSelection('');
                      }}
                    >
                      <Trash2 size={14} />
                      删除此节点
                    </button>
                  )}
                </div>
              ) : (
                <div className="inspector-empty">
                  <Workflow size={27} />
                  <p>点击节点配置技能、模型、条件和知识来源。</p>
                </div>
              )
            ) : (
              <div className="inspector-runs">
                {selectedRun && (
                  <div className="selected-run-summary">
                    <span className={`node-status ${selectedRun.status}`}>
                      {statusLabel[selectedRun.status] || selectedRun.status}
                    </span>
                    <p>{selectedRun.prompt}</p>
                    {selectedRun.error && <p className="module-error">{selectedRun.error}</p>}
                    {['queued', 'running'].includes(selectedRun.status) && (
                      <button
                        className="button full"
                        onClick={() =>
                          perform(async () => {
                            await post(`/workflow-runs/${selectedRun.id}/cancel`);
                          })
                        }
                      >
                        <Square size={13} />
                        停止流程
                      </button>
                    )}
                    {selectedRun.nodes.map((n) => (
                      <button
                        className="run-node-row"
                        key={n.nodeId}
                        onClick={() => {
                          setSelection(n.nodeId);
                          setTab('node');
                        }}
                      >
                        <span className={`node-state-dot ${n.status}`} />
                        <strong>{n.label}</strong>
                        <span>{labels[n.status]}</span>
                      </button>
                    ))}
                    {selectedRun.outputIds
                      .map((id) => state.assets.find((a) => a.id === id))
                      .filter((a): a is Asset => !!a)
                      .map((a) => (
                        <button className="run-output" key={a.id} onClick={() => saveAsset(a)}>
                          <Files size={15} />
                          {a.name}
                        </button>
                      ))}
                  </div>
                )}
                {!runs.length ? (
                  <div className="inspector-empty">
                    <History size={25} />
                    <p>保存并执行后，会记录每个节点的状态、任务和成果。</p>
                  </div>
                ) : (
                  runs.map((r) => (
                    <button
                      className={`run-history-item ${r.id === runId ? 'selected' : ''}`}
                      key={r.id}
                      onClick={() => setRunId(r.id)}
                    >
                      <div>
                        <strong>{new Date(r.createdAt).toLocaleString('zh-CN')}</strong>
                        <span className={`node-status ${r.status}`}>
                          {statusLabel[r.status] || r.status}
                        </span>
                      </div>
                      <p>{r.prompt}</p>
                      <span>
                        版本 {r.definition.version} · {r.nodes.length} 个节点
                      </span>
                    </button>
                  ))
                )}
              </div>
            )}
          </aside>
        )}
      </div>
      {runDialog && (
        <div className="module-overlay">
          <form
            className="module-dialog"
            onSubmit={(e) => {
              e.preventDefault();
              execute();
            }}
          >
            <div className="module-dialog-title">
              <h2>执行工作流</h2>
              <button type="button" onClick={() => setRunDialog(false)} aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <label>
              本次任务要求
              <textarea
                required
                autoFocus
                rows={4}
                value={runPrompt}
                onChange={(e) => setRunPrompt(e.target.value)}
                placeholder="告诉这些节点，这次要完成什么…"
              />
            </label>
            <div className="run-attachments-title">
              <strong>任务附件 · {inputIds.length}</strong>
              {desktop() && (
                <button type="button" className="text-button" onClick={pickRunInputs}>
                  <Paperclip size={13} />
                  选择本机文件
                </button>
              )}
            </div>
            <div className="asset-chooser">
              {state.assets
                .filter((a) => a.projectId === projectId)
                .map((a) => (
                  <label key={a.id}>
                    <input
                      type="checkbox"
                      checked={inputIds.includes(a.id)}
                      onChange={(e) =>
                        setInputIds((prev) =>
                          e.target.checked ? [...prev, a.id] : prev.filter((id) => id !== a.id),
                        )
                      }
                    />
                    {a.name}
                  </label>
                ))}
            </div>
            <label className="check-label">
              <input
                type="checkbox"
                checked={allowRemote}
                onChange={(e) => setAllowRemote(e.target.checked)}
              />
              允许将本次内容与引用资料发送到已配置的节点模型
            </label>
            <p className="run-explanation">
              Agent 节点需选择模型。并行办公节点最多 3 个，整个流程最多运行 10 分钟。当前流程
              {draft.enabled ? '已启用' : '未启用'}。
            </p>
            {!draft.enabled && (
              <label className="check-label">
                <input
                  type="checkbox"
                  onChange={(e) => {
                    setDraft({ ...draft, enabled: e.target.checked });
                    setDirty(true);
                  }}
                />
                启用此流程
              </label>
            )}
            {error && <p className="module-error">{error}</p>}
            <div className="module-dialog-actions">
              <button className="button" type="button" onClick={() => setRunDialog(false)}>
                取消
              </button>
              <button
                className="button primary"
                disabled={busy || !runPrompt.trim() || inputIds.length > 16 || !draft.enabled}
              >
                <Play size={14} />
                {busy ? '正在提交' : '保存并执行'}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
