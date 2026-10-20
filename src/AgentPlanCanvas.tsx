import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  MarkerType,
  type Node,
  type NodeProps,
  type Edge,
  type Connection,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { X, Bot, Play, Flag, Pause, Plus, Trash2, Save, ArrowUpRight } from 'lucide-react';
import {
  post,
  statusLabel,
  kinds,
  type AgentGroup,
  type AgentNodeSpec,
  type State,
  type Task,
} from './api';
import { FitWorkflowView } from './GraphViewport';
import './agent-canvas.css';

type Data = Record<string, unknown> & {
  title: string;
  role: string;
  model: string;
  status: string;
  terminal?: boolean;
  entry?: boolean;
};
type FlowNode = Node<Data, 'agentPlan'>;
function PlanNode({ data, selected }: NodeProps<FlowNode>) {
  return (
    <div className={`agent-plan-node ${selected ? 'selected' : ''} ${data.status}`}>
      {!data.entry && <Handle type="target" position={Position.Left} id="in" />}
      <div className="agent-plan-node-heading">
        <span>
          {data.entry ? <Play size={17} /> : data.terminal ? <Flag size={17} /> : <Bot size={17} />}
        </span>
        <strong>{data.title}</strong>
      </div>
      <p>{data.role}</p>
      <small>{data.model}</small>
      <footer>
        <i />
        {statusLabel[data.status] || data.status}
      </footer>
      {!data.terminal && <Handle type="source" position={Position.Right} id="out" />}
    </div>
  );
}
const nodeTypes = { agentPlan: PlanNode };
const roles: Record<string, string> = {
  planner: '方案协调',
  explorer: '资料探查',
  researcher: '资料研究',
  worker: '成果制作',
  reviewer: '独立审阅',
};
const clone = (g: AgentGroup) => structuredClone(g.plan?.members || []);

export default function AgentPlanCanvas({
  group,
  state,
  onRefresh,
  onTask,
  onClose,
}: {
  group: AgentGroup;
  state: State;
  onRefresh: () => Promise<void>;
  onTask: (task: Task) => void;
  onClose: () => void;
}) {
  const [draft, setDraft] = useState<AgentNodeSpec[] | null>(null);
  const [baseVersion, setBaseVersion] = useState(group.planVersion || 1);
  const [selection, setSelection] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const dialog = useRef<HTMLElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.querySelector<HTMLButtonElement>('[aria-label="关闭编排画布"]')?.focus();
    return () => previous?.focus();
  }, []);
  const specs = draft || group.plan?.members || [];
  const editable = ['paused', 'interrupted'].includes(group.status) && !group.settling;
  const patch = (id: string, change: Partial<AgentNodeSpec>) => {
    setDraft((prev) => (prev || clone(group)).map((n) => (n.id === id ? { ...n, ...change } : n)));
  };
  const member = group.members.find((m) => m.id === selection);
  const spec = specs.find((m) => m.id === selection);
  const latest = state.tasks.find((t) => t.id === member?.taskIds.at(-1));
  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
      await onRefresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const { nodes, edges } = useMemo(() => {
    const levels = new Map<string, number>();
    // Bounded iteration also displays an invalid draft cycle so it can be fixed;
    // the server remains authoritative and rejects it at save time.
    for (let pass = 0; pass < specs.length; pass++)
      for (const n of specs)
        levels.set(
          n.id,
          Math.min(
            specs.length,
            1 + Math.max(0, ...(n.dependsOn || []).map((id) => levels.get(id) || 0)),
          ),
        );
    const rows = new Map<number, number>();
    const node = (
      id: string,
      title: string,
      role: string,
      model: string,
      status: string,
      x: number,
      y: number,
      extra: Partial<Data> = {},
    ): FlowNode => ({
      id,
      type: 'agentPlan',
      position: { x, y },
      data: { title, role, model, status, ...extra },
    });
    const ns: FlowNode[] = [
      node('@start', '项目目标', '主 Agent 制定计划', '', 'succeeded', 0, 120, { entry: true }),
    ];
    for (const n of specs) {
      const level = levels.get(n.id) || 1,
        row = rows.get(level) || 0;
      rows.set(level, row + 1);
      const m = group.members.find((m) => m.id === n.id);
      const model = group.modelCatalog?.find((v) => v.id === n.modelId);
      ns.push(
        node(
          n.id,
          n.name || roles[n.role],
          roles[n.role] || n.role,
          model?.modelId || m?.configSnapshot?.model?.modelId || '沿用主模型',
          draft ? 'pending' : m?.status || 'pending',
          level * 285,
          row * 210,
        ),
      );
    }
    ns.push(
      node(
        '@end',
        '统一交付',
        '主 Agent 汇总成果',
        '',
        group.status,
        (Math.max(0, ...levels.values()) + 1) * 285,
        120,
        { terminal: true },
      ),
    );
    const es: Edge[] = [];
    const edge = (source: string, target: string) =>
      es.push({
        id: `${source}:${target}`,
        source,
        target,
        sourceHandle: 'out',
        targetHandle: 'in',
        type: 'smoothstep',
        deletable: !source.startsWith('@') && !target.startsWith('@'),
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: 'var(--secondary)', strokeWidth: 1.5 },
      });
    for (const n of specs) {
      if (!n.dependsOn?.length) edge('@start', n.id);
      else for (const dep of n.dependsOn) edge(dep, n.id);
      if (!specs.some((v) => v.dependsOn?.includes(n.id))) edge(n.id, '@end');
    }
    return { nodes: ns, edges: es };
  }, [specs, group.members, group.status, group.modelCatalog, !!draft]);
  const connect = (c: Connection) => {
    if (
      !editable ||
      !c.source ||
      !c.target ||
      c.source === c.target ||
      c.source.startsWith('@') ||
      c.target.startsWith('@')
    )
      return;
    const target = specs.find((m) => m.id === c.target);
    if (target)
      patch(target.id, { dependsOn: [...new Set([...(target.dependsOn || []), c.source])] });
  };
  const root = state.tasks.find((t) => t.id === group.rootTaskId);
  return (
    <div
      className="modal-backdrop agent-canvas-backdrop"
      onKeyDown={(e) => {
        if (e.key === 'Escape' && !draft) onClose();
      }}
    >
      <section
        ref={dialog}
        className="agent-canvas-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="协作编排画布"
        onKeyDown={(e) => {
          if (e.key !== 'Tab') return;
          const fields = Array.from(
            dialog.current?.querySelectorAll<HTMLElement>(
              'button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),[tabindex="0"]',
            ) || [],
          ).filter((el) => el.offsetParent !== null);
          const first = fields[0],
            last = fields.at(-1);
          if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last?.focus();
          } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first?.focus();
          }
        }}
      >
        <header className="agent-canvas-header">
          <div>
            <strong>协作编排</strong>
            <span>
              计划 v{group.planVersion || 1} · {statusLabel[group.status]} · {specs.length} 位成员
            </span>
          </div>
          {['planning', 'running', 'aggregating'].includes(group.status) && (
            <button
              disabled={busy}
              onClick={() =>
                act(async () => {
                  await post(`/agent-groups/${group.id}/pause`);
                  setBaseVersion(group.planVersion || 1);
                })
              }
            >
              <Pause size={15} />
              暂停并调整
            </button>
          )}
          {editable && (
            <button
              disabled={busy || specs.length >= 6}
              onClick={() => {
                const id = crypto.randomUUID();
                setBaseVersion(group.planVersion || 1);
                setDraft([
                  ...specs,
                  {
                    id,
                    role: 'worker',
                    name: '新成员',
                    prompt: '',
                    kind: 'chat',
                    dependsOn: [],
                    modelId: root?.modelId,
                    inputIds: [],
                    skillIds: [],
                  },
                ]);
                setSelection(id);
              }}
            >
              <Plus size={15} />
              添加成员
            </button>
          )}
          {draft && (
            <>
              <button
                disabled={busy}
                onClick={() => {
                  setDraft(null);
                  setError('');
                }}
              >
                放弃调整
              </button>
              <button
                className="primary"
                disabled={busy || !editable}
                onClick={() =>
                  act(async () => {
                    await post(`/agent-groups/${group.id}/plan`, {
                      expectedVersion: baseVersion,
                      members: draft,
                    });
                    setDraft(null);
                  })
                }
              >
                <Save size={15} />
                保存计划
              </button>
            </>
          )}
          {editable && !draft && (
            <button
              className="primary"
              disabled={busy}
              onClick={() =>
                act(async () => {
                  await post(`/agent-groups/${group.id}/resume`);
                })
              }
            >
              <Play size={15} />
              继续执行
            </button>
          )}
          <button
            aria-label="关闭编排画布"
            disabled={busy}
            onClick={() => {
              if (!draft || window.confirm('放弃尚未保存的计划调整？')) onClose();
            }}
          >
            <X size={19} />
          </button>
        </header>
        <div className="agent-canvas-notice">
          {group.settling && editable === false && ['paused', 'interrupted'].includes(group.status)
            ? '正在取消本轮请求，收尾完成后即可调整或继续。'
            : editable
              ? '可连线调整依赖，点击成员修改分工。保存后继续，仅重跑受影响的分支。'
              : '节点与连线来自实际执行计划。点击节点查看分工、模型和交付。'}
        </div>
        {error && (
          <p className="module-error" role="alert">
            {error}
          </p>
        )}
        <div className="agent-canvas-body">
          <div className="agent-canvas-flow" data-testid="agent-plan-flow">
            <ReactFlow<FlowNode>
              nodes={nodes.map((n) => ({ ...n, selected: n.id === selection }))}
              edges={edges}
              nodeTypes={nodeTypes}
              nodesDraggable={false}
              nodesConnectable={editable}
              edgesReconnectable={false}
              elementsSelectable
              fitView
              minZoom={0.25}
              maxZoom={1.5}
              onNodeClick={(_, n) => {
                setSelection(n.id);
                if (!draft) setBaseVersion(group.planVersion || 1);
              }}
              onConnect={connect}
              onEdgesDelete={(removed) => {
                if (!editable) return;
                setDraft(
                  specs.map((s) => ({
                    ...s,
                    dependsOn: (s.dependsOn || []).filter(
                      (d) => !removed.some((e) => e.source === d && e.target === s.id),
                    ),
                  })),
                );
              }}
              deleteKeyCode={editable ? ['Backspace', 'Delete'] : null}
            >
              <FitWorkflowView viewId={`${group.id}:${specs.length}`} />
              <Background gap={22} size={1} color="var(--line)" />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
          <aside className="agent-canvas-inspector">
            {spec ? (
              <>
                <div className="agent-inspector-title">
                  <strong>{spec.name || roles[spec.role]}</strong>
                  <span>{statusLabel[member?.status || 'pending']}</span>
                </div>
                <label>
                  名称
                  <input
                    aria-label="编排成员名称"
                    disabled={!editable}
                    value={spec.name}
                    onChange={(e) => patch(spec.id, { name: e.target.value })}
                  />
                </label>
                <label>
                  职责
                  <select
                    aria-label="编排成员职责"
                    disabled={!editable}
                    value={spec.role}
                    onChange={(e) =>
                      patch(spec.id, {
                        role: e.target.value,
                        kind: e.target.value === 'reviewer' ? 'chat' : spec.kind,
                      })
                    }
                  >
                    {Object.entries(roles).map(([id, name]) => (
                      <option key={id} value={id}>
                        {name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  模型
                  <select
                    aria-label="编排成员模型"
                    disabled={!editable}
                    value={spec.modelId || root?.modelId || ''}
                    onChange={(e) => patch(spec.id, { modelId: e.target.value })}
                  >
                    {group.modelCatalog?.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.name} · {m.modelId}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  交付
                  <select
                    disabled={!editable || spec.role === 'reviewer'}
                    value={spec.kind || 'chat'}
                    onChange={(e) => patch(spec.id, { kind: e.target.value })}
                  >
                    {['chat', 'md', 'html', 'code', 'docx', 'pptx', 'xlsx'].map((k) => (
                      <option key={k} value={k}>
                        {kinds[k]}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  任务
                  <textarea
                    aria-label="编排成员任务"
                    rows={4}
                    disabled={!editable}
                    value={spec.prompt}
                    onChange={(e) => patch(spec.id, { prompt: e.target.value })}
                  />
                </label>
                <label>
                  验收条件
                  <textarea
                    rows={2}
                    disabled={!editable}
                    value={spec.acceptance || ''}
                    onChange={(e) => patch(spec.id, { acceptance: e.target.value })}
                  />
                </label>
                {spec.kind === 'code' && (
                  <label>
                    负责文件（每行一个）
                    <textarea
                      rows={3}
                      disabled={!editable}
                      value={(spec.ownedPaths || []).join('\n')}
                      onChange={(e) =>
                        patch(spec.id, { ownedPaths: e.target.value.split('\n').filter(Boolean) })
                      }
                    />
                  </label>
                )}
                <fieldset>
                  <legend>等待这些成员完成</legend>
                  {specs
                    .filter((s) => s.id !== spec.id)
                    .map((s) => (
                      <label className="agent-dependency" key={s.id}>
                        <input
                          type="checkbox"
                          disabled={!editable}
                          checked={spec.dependsOn?.includes(s.id) || false}
                          onChange={(e) =>
                            patch(spec.id, {
                              dependsOn: e.target.checked
                                ? [...(spec.dependsOn || []), s.id]
                                : (spec.dependsOn || []).filter((id) => id !== s.id),
                            })
                          }
                        />
                        {s.name || roles[s.role]}
                      </label>
                    ))}
                </fieldset>
                {latest && (
                  <button onClick={() => onTask(latest)}>
                    <ArrowUpRight size={14} />
                    查看最近任务与成果
                  </button>
                )}
                {member?.error && <p className="module-error">{member.error}</p>}
                {editable && (
                  <button
                    className="agent-remove-member"
                    disabled={busy}
                    onClick={() => {
                      setDraft(
                        specs
                          .filter((s) => s.id !== spec.id)
                          .map((s) => ({
                            ...s,
                            dependsOn: (s.dependsOn || []).filter((id) => id !== spec.id),
                          })),
                      );
                      setSelection('');
                    }}
                  >
                    <Trash2 size={14} />
                    移除成员
                  </button>
                )}
              </>
            ) : (
              <div className="agent-canvas-empty">
                <Bot size={26} />
                <strong>选择一个成员</strong>
                <p>查看职责、模型、依赖与验收条件。运行中可先暂停，再调整分工。</p>
              </div>
            )}
          </aside>
        </div>
        <footer className="agent-canvas-footer">
          <span>最多 6 位成员 · 3 任务并发 · 返工 {group.reworkRound || 0} / 2</span>
          <span>{draft ? '有未保存调整' : '历史文件和已完成的独立分支保留'}</span>
        </footer>
      </section>
    </div>
  );
}
