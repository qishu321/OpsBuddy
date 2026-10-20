import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import {
  Bot,
  X,
  Plus,
  ArrowUpRight,
  Square,
  Send,
  Users,
  Loader2,
  ChevronRight,
  Workflow,
  Pause,
  Play,
} from 'lucide-react';
import {
  post,
  statusLabel,
  kinds as kindLabel,
  type AgentGroup,
  type AgentNodeSpec,
  type State,
  type Task,
} from './api';
import MarkdownContent from './MarkdownContent';
import './agent-canvas.css';
const AgentPlanCanvas = lazy(() => import('./AgentPlanCanvas'));
export const agentRoles: Record<string, { name: string; description: string; prompt: string }> = {
  planner: {
    name: '方案协调',
    description: '明确本次目标、约束与验收方法',
    prompt: '依据明确引用的材料制定可验证的方案，说明输入、交付与约束。',
  },
  explorer: {
    name: '资料探查',
    description: '梳理所选文件与资料范围',
    prompt: '梳理已提供的材料，列出关键事实、资料结构和缺失信息。',
  },
  researcher: {
    name: '资料研究',
    description: '依据材料分析与归纳',
    prompt: '研究已提供的材料与知识片段，围绕主任务提出有依据的结论，标注假设和待核实事项。',
  },
  worker: {
    name: '成果制作',
    description: '完成主任务需要的交付',
    prompt: '按主任务要求制作内容，保留材料中的重要数字、事实与约束。',
  },
  reviewer: {
    name: '成果审阅',
    description: '等待其他成员完成后检查',
    prompt: '检查其他成员的成果，核对事实与用户约束，明确列出问题和修改建议。',
  },
};
export function AgentDispatcher({
  open,
  onClose,
  state,
  projectId,
  prompt,
  sourceTaskId,
  inputIds,
  skillIds,
  knowledgeIds,
  roleId,
  modelId,
  kind,
  allowRemote,
  useMemory,
  onDispatched,
}: {
  open: boolean;
  onClose: () => void;
  state: State;
  projectId: string;
  prompt: string;
  sourceTaskId?: string;
  inputIds: string[];
  skillIds: string[];
  knowledgeIds: string[];
  roleId: string;
  modelId: string;
  kind: string;
  allowRemote: boolean;
  useMemory: boolean;
  onDispatched: (group: AgentGroup) => void;
}) {
  const [goal, setGoal] = useState(''),
    [members, setMembers] = useState<AgentNodeSpec[]>([]),
    [autoPlan, setAutoPlan] = useState(true),
    [autoResume, setAutoResume] = useState(false),
    [outputKind, setOutputKind] = useState(kind),
    [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const initialFocus = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (open) {
      setGoal(prompt);
      setError('');
      setAutoPlan(true);
      setAutoResume(false);
      setOutputKind(['pdf', 'organize'].includes(kind) ? 'chat' : kind);
      setMembers(
        ['explorer', 'worker', 'reviewer'].map((role, i) => ({
          id: `node-${i + 1}`,
          role,
          name: agentRoles[role].name,
          prompt: agentRoles[role].prompt,
          modelId: '',
          kind:
            role === 'worker'
              ? ['docx', 'pptx', 'xlsx', 'code', 'md', 'html'].includes(kind)
                ? kind
                : 'md'
              : 'chat',
          dependsOn: i === 0 ? [] : [`node-${i}`],
          acceptance: '内容符合本节点要求，缺失和未验证事项明确说明',
        })),
      );
      requestAnimationFrame(() => initialFocus.current?.focus());
    }
  }, [open]);
  if (!open) return null;
  const send = async () => {
    setBusy(true);
    setError('');
    try {
      if (!modelId) throw new Error('请先添加并选择模型');
      const group = await post<AgentGroup>('/agent-groups', {
        projectId,
        prompt: goal,
        sourceTaskId,
        inputIds,
        skillIds,
        knowledgeIds,
        roleId,
        modelId,
        kind: outputKind,
        allowRemote,
        useMemory,
        autoPlan,
        autoResume,
        members: autoPlan ? [] : members,
      });
      onDispatched(group);
      onClose();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="module-overlay">
      <section
        className="module-dialog team-dispatcher"
        role="dialog"
        aria-modal="true"
        aria-label="派发子任务"
        onKeyDown={(e) => {
          if (e.key === 'Escape' && !busy) onClose();
          if (e.key === 'Tab') {
            const els = Array.from(
              e.currentTarget.querySelectorAll<HTMLElement>(
                'button:not(:disabled),textarea,select,input',
              ),
            ).filter((x) => x.offsetParent !== null);
            if (e.shiftKey && document.activeElement === els[0]) {
              e.preventDefault();
              els[els.length - 1]?.focus();
            } else if (!e.shiftKey && document.activeElement === els[els.length - 1]) {
              e.preventDefault();
              els[0]?.focus();
            }
          }
        }}
      >
        <header>
          <div>
            <span className="eyebrow">AGENT TEAM</span>
            <h2>把任务交给一支小队</h2>
            <p>制定计划 → 按依赖执行 → 审阅与有限返工 → 统一交付</p>
          </div>
          <button
            className="icon-button"
            aria-label="关闭子任务派发"
            disabled={busy}
            onClick={onClose}
          >
            <X size={19} />
          </button>
        </header>
        <label>
          主任务目标
          <textarea
            ref={initialFocus}
            value={goal}
            rows={3}
            maxLength={10000}
            onChange={(e) => setGoal(e.target.value)}
            placeholder="希望这支小队一起完成什么？"
          />
        </label>
        <div className="team-plan-options">
          <label>
            执行方式
            <select
              aria-label="协作规划方式"
              value={autoPlan ? 'auto' : 'manual'}
              onChange={(e) => setAutoPlan(e.target.value === 'auto')}
            >
              <option value="auto">主模型制定计划并执行</option>
              <option value="manual">手动编辑计划</option>
            </select>
          </label>
          <label>
            最终交付
            <select
              aria-label="协作最终交付"
              value={outputKind}
              onChange={(e) => setOutputKind(e.target.value)}
            >
              {['auto', 'chat', 'md', 'docx', 'pptx', 'xlsx', 'html', 'code'].map((k) => (
                <option key={k} value={k}>
                  {kindLabel[k] || '自动选择'}
                </option>
              ))}
            </select>
          </label>
        </div>
        {autoPlan ? (
          <p className="team-dispatch-context">
            主模型将根据目标制定结构化计划，从已配置连接中选择成员模型；校验依赖后立即执行。可在主会话查看真实计划与进度。若希望指定模型或修改依赖，请切换手动编辑。
          </p>
        ) : (
          <>
            <div className="team-members-heading">
              <strong>
                任务分工 <span>{members.length} / 6</span>
              </strong>
              <button
                disabled={busy || members.length >= 6}
                onClick={() =>
                  setMembers([
                    ...members,
                    {
                      id: `node-${Date.now()}`,
                      role: 'worker',
                      name: '成果制作',
                      prompt: agentRoles.worker.prompt,
                      kind: 'md',
                      dependsOn: [],
                    },
                  ])
                }
              >
                <Plus size={14} />
                添加成员
              </button>
            </div>
            <div className="team-member-editors">
              {members.map((m, i) => (
                <article key={i}>
                  <div className="team-member-editor-top">
                    <Bot size={17} />
                    <select
                      aria-label={`成员 ${i + 1} 分工`}
                      value={m.role}
                      onChange={(e) =>
                        setMembers(
                          members.map((v, n) =>
                            n === i
                              ? {
                                  ...v,
                                  role: e.target.value,
                                  kind: e.target.value === 'reviewer' ? 'chat' : v.kind,
                                  name: agentRoles[e.target.value].name,
                                  prompt: agentRoles[e.target.value].prompt,
                                }
                              : v,
                          ),
                        )
                      }
                    >
                      {Object.entries(agentRoles).map(([id, v]) => (
                        <option key={id} value={id}>
                          {id} · {v.name}
                        </option>
                      ))}
                    </select>
                    <button
                      title="移除此成员"
                      aria-label={`移除成员 ${i + 1}`}
                      disabled={members.length <= 1 || busy}
                      onClick={() =>
                        setMembers(
                          members
                            .filter((_, n) => n !== i)
                            .map((v) => ({
                              ...v,
                              dependsOn: v.dependsOn?.filter((id) => id !== m.id),
                            })),
                        )
                      }
                    >
                      <X size={14} />
                    </button>
                  </div>
                  <textarea
                    aria-label={`成员 ${i + 1} 任务`}
                    rows={2}
                    value={m.prompt}
                    maxLength={4000}
                    onChange={(e) =>
                      setMembers(
                        members.map((v, n) => (n === i ? { ...v, prompt: e.target.value } : v)),
                      )
                    }
                  />
                  <div className="team-plan-options">
                    <label>
                      模型
                      <select
                        aria-label={`成员 ${i + 1} 模型`}
                        value={m.modelId || ''}
                        onChange={(e) =>
                          setMembers(
                            members.map((v, n) =>
                              n === i ? { ...v, modelId: e.target.value } : v,
                            ),
                          )
                        }
                      >
                        <option value="">沿用主模型</option>
                        {state.models.map((model) => (
                          <option key={model.id} value={model.id}>
                            {model.name} · {model.modelId}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      交付
                      <select
                        aria-label={`成员 ${i + 1} 交付`}
                        value={m.kind || 'chat'}
                        onChange={(e) =>
                          setMembers(
                            members.map((v, n) => (n === i ? { ...v, kind: e.target.value } : v)),
                          )
                        }
                      >
                        {(m.role === 'reviewer'
                          ? ['chat']
                          : ['chat', 'md', 'docx', 'pptx', 'xlsx', 'html', 'code']
                        ).map((k) => (
                          <option key={k} value={k}>
                            {kindLabel[k]}
                          </option>
                        ))}
                      </select>
                    </label>
                  </div>
                  <label>
                    验收条件
                    <input
                      aria-label={`成员 ${i + 1} 验收条件`}
                      value={m.acceptance || ''}
                      maxLength={1000}
                      onChange={(e) =>
                        setMembers(
                          members.map((v, n) =>
                            n === i ? { ...v, acceptance: e.target.value } : v,
                          ),
                        )
                      }
                    />
                  </label>
                  {m.kind === 'code' && (
                    <label>
                      负责文件（每行一个相对路径）
                      <textarea
                        aria-label={`成员 ${i + 1} 文件职责`}
                        value={(m.ownedPaths || []).join('\n')}
                        onChange={(e) =>
                          setMembers(
                            members.map((v, n) =>
                              n === i ? { ...v, ownedPaths: e.target.value.split('\n') } : v,
                            ),
                          )
                        }
                      />
                    </label>
                  )}
                  <details>
                    <summary>依赖与材料范围 · {m.dependsOn?.length || 0} 个前置节点</summary>
                    <fieldset>
                      <legend>等待这些节点成功</legend>
                      {members
                        .filter((v) => v.id !== m.id)
                        .map((other) => (
                          <label key={other.id} className="team-check">
                            <input
                              type="checkbox"
                              checked={m.dependsOn?.includes(other.id) || false}
                              onChange={(e) =>
                                setMembers(
                                  members.map((v, n) =>
                                    n === i
                                      ? {
                                          ...v,
                                          dependsOn: e.target.checked
                                            ? [...(v.dependsOn || []), other.id]
                                            : v.dependsOn?.filter((id) => id !== other.id),
                                        }
                                      : v,
                                  ),
                                )
                              }
                            />
                            {other.name} ({other.id})
                          </label>
                        ))}
                    </fieldset>
                    <fieldset>
                      <legend>明确引用的文件（默认继承本次所选材料）</legend>
                      {inputIds.map((id) => (
                        <label key={id} className="team-check">
                          <input
                            type="checkbox"
                            checked={(m.inputIds || inputIds).includes(id)}
                            onChange={(e) =>
                              setMembers(
                                members.map((v, n) =>
                                  n === i
                                    ? {
                                        ...v,
                                        inputIds: e.target.checked
                                          ? [...(v.inputIds || inputIds), id]
                                          : (v.inputIds || inputIds).filter((x) => x !== id),
                                      }
                                    : v,
                                ),
                              )
                            }
                          />
                          {state.assets.find((a) => a.id === id)?.name || id}
                        </label>
                      ))}
                    </fieldset>
                    <fieldset>
                      <legend>候选技能（运行时按需读取）</legend>
                      {skillIds.map((id) => (
                        <label key={id} className="team-check">
                          <input
                            type="checkbox"
                            checked={(m.skillIds || skillIds).includes(id)}
                            onChange={(e) =>
                              setMembers(
                                members.map((v, n) =>
                                  n === i
                                    ? {
                                        ...v,
                                        skillIds: e.target.checked
                                          ? [...(v.skillIds || skillIds), id]
                                          : (v.skillIds || skillIds).filter((x) => x !== id),
                                      }
                                    : v,
                                ),
                              )
                            }
                          />
                          {state.extensions.find((x) => x.id === id)?.name || id}
                        </label>
                      ))}
                    </fieldset>
                  </details>
                </article>
              ))}
            </div>
          </>
        )}
        <div className="team-dispatch-context">
          <span>
            {inputIds.length} 个所选文件 · {knowledgeIds.length} 个知识库 · {skillIds.length}{' '}
            项候选技能
          </span>
          <p>
            {modelId
              ? `使用模型：${state.models.find((m) => m.id === modelId)?.name || '已选模型'}。子任务与最终汇总分别发起请求；如需技能选择，每个任务还会有一次目录选择请求。`
              : '请先添加并选择模型，再派发子任务。'}{' '}
            资料探查与研究仅访问引用材料，尚未提供联网搜索。
          </p>
        </div>
        <label className="team-recovery-choice">
          <input
            type="checkbox"
            checked={autoResume}
            onChange={(e) => setAutoResume(e.target.checked)}
          />
          重启后自动继续此协作（未完成请求会重新调用模型）
        </label>
        {error && (
          <div className="module-error" role="alert">
            {error}
          </div>
        )}
        <footer>
          <span>最多 6 成员 · 3 并发 · 2 轮返工 · 每节点重试 1 次</span>
          <button
            className="button primary"
            disabled={
              busy ||
              !goal.trim() ||
              (!autoPlan && members.some((m) => !m.prompt.trim())) ||
              !modelId ||
              !allowRemote
            }
            onClick={send}
          >
            {busy ? <Loader2 size={15} className="spin" /> : <Send size={15} />}协作并执行
          </button>
        </footer>
      </section>
    </div>
  );
}
export function AgentGroupPanel({
  group,
  state,
  onRefresh,
  onTask,
}: {
  group: AgentGroup;
  state: State;
  onRefresh: () => Promise<void>;
  onTask: (task: Task) => void;
}) {
  const [error, setError] = useState(''),
    [busy, setBusy] = useState(false),
    [canvasOpen, setCanvasOpen] = useState(false),
    [selected, setSelected] = useState(''),
    [instruction, setInstruction] = useState('');
  const active = ['planning', 'running', 'aggregating'].includes(group.status);
  const perform = async (action: () => Promise<unknown>) => {
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
  const root = state.tasks.find((t) => t.id === group.rootTaskId);
  return (
    <section className="agent-group-panel" data-testid="agent-group">
      <header>
        <span className="team-emblem">
          <Users size={18} />
        </span>
        <div>
          <strong>协作小队</strong>
          <span>
            {group.members.length} 位成员 · {statusLabel[group.status] || group.status}
          </span>
        </div>
        {root && (
          <button onClick={() => onTask(root)}>
            主任务
            <ArrowUpRight size={13} />
          </button>
        )}
        {(active || ['paused', 'interrupted'].includes(group.status)) && (
          <button
            className="team-stop"
            disabled={busy}
            onClick={() => perform(() => post(`/agent-groups/${group.id}/cancel`))}
          >
            <Square size={12} />
            整组停止
          </button>
        )}
      </header>
      <div className="team-controls">
        <button disabled={!group.plan?.members?.length} onClick={() => setCanvasOpen(true)}>
          <Workflow size={15} />
          编排画布
        </button>
        {active && (
          <button
            disabled={busy}
            onClick={() => perform(() => post(`/agent-groups/${group.id}/pause`))}
          >
            <Pause size={14} />
            暂停协作
          </button>
        )}
        {['paused', 'interrupted'].includes(group.status) && (
          <button
            disabled={busy || group.settling}
            onClick={() => perform(() => post(`/agent-groups/${group.id}/resume`))}
          >
            <Play size={14} />
            {group.settling ? '请求收尾中…' : '继续执行'}
          </button>
        )}
        <label title="仅在桌面重启导致中断时自动继续；手动暂停或停止不会重放。未完成请求会重新调用模型。">
          <input
            type="checkbox"
            disabled={busy}
            checked={group.autoResume || false}
            onChange={(e) =>
              perform(() =>
                post(`/agent-groups/${group.id}/recovery`, { autoResume: e.target.checked }),
              )
            }
          />
          重启后自动继续
        </label>
      </div>
      {['paused', 'interrupted'].includes(group.status) && (
        <p className="team-resume-note">
          已完成的独立分支保留。继续时仅重新执行未完成的请求，可能产生新的模型用量。
        </p>
      )}
      {canvasOpen && (
        <Suspense fallback={<p>正在打开编排画布…</p>}>
          <AgentPlanCanvas
            group={group}
            state={state}
            onRefresh={onRefresh}
            onTask={(task) => {
              setCanvasOpen(false);
              onTask(task);
            }}
            onClose={() => setCanvasOpen(false)}
          />
        </Suspense>
      )}
      {group.plan?.summary && (
        <div className="team-plan-summary">
          <strong>执行计划 v{group.planVersion || 1}</strong>
          <MarkdownContent content={group.plan.summary} />
          <span>
            返工 {group.reworkRound || 0} / 2 · 配置按计划版本保存
            {!group.members.some((m) => m.role === 'reviewer') && group.status !== 'planning'
              ? ' · 未设置独立审阅'
              : ''}
          </span>
        </div>
      )}
      {!!group.planHistory?.length && (
        <details className="team-plan-summary">
          <summary>历史计划 · {group.planHistory.length} 次调整</summary>
          {group.planHistory.map((v) => (
            <p key={v.version}>
              v{v.version} · {new Date(v.at).toLocaleString()} ·{' '}
              {v.plan.members.map((m) => m.name || m.role).join(' → ')}
            </p>
          ))}
        </details>
      )}
      {group.status === 'planning' && (
        <p className="team-aggregation">
          <Loader2 size={14} className="spin" />
          主模型正在制定并校验计划…
        </p>
      )}
      {group.plannerTaskId && (
        <button
          onClick={() => {
            const planner = state.tasks.find((t) => t.id === group.plannerTaskId);
            if (planner) onTask(planner);
          }}
        >
          查看规划任务
        </button>
      )}
      <div className="team-progress">
        {group.members.map((m) => {
          const latest = state.tasks.find((t) => t.id === m.taskIds[m.taskIds.length - 1]);
          return (
            <article key={m.id}>
              <div className={`team-role-icon ${m.role}`}>
                <Bot size={17} />
              </div>
              <div className="team-member-info">
                <strong>
                  {m.name}
                  <code>{m.role}</code>
                </strong>
                <span>
                  {statusLabel[m.status] || '等待派发'}
                  {m.role === 'reviewer' && m.taskIds.length === 0 ? ' · 等待其他成员成果' : ''}
                  {m.pending?.length ? ` · ${m.pending.length} 轮待执行` : ''}
                </span>
                <p>{m.instructions[m.instructions.length - 1]}</p>
                <p>
                  模型：{m.configSnapshot?.model?.name || latest?.modelName || '历史记录未保存'} ·{' '}
                  {m.configSnapshot?.model?.modelId || latest?.configSnapshot?.model?.modelId || ''}
                </p>
                <p>
                  依赖：
                  {m.dependsOn?.length
                    ? m.dependsOn
                        .map((id) => group.members.find((v) => v.id === id)?.name || id)
                        .join('、')
                    : '无，可并行'}{' '}
                  · 自动重试 {m.retries || 0} / 1
                </p>
                {m.acceptance && <p>验收：{m.acceptance}</p>}
                {(m.error || latest?.error) && (
                  <p className="module-error">{m.error || latest?.error}</p>
                )}
                {latest && (
                  <details>
                    <summary>真实任务、文件与模型回执</summary>
                    <code>{latest.id}</code>
                    <p>
                      {new Date(latest.createdAt).toLocaleString()} → {statusLabel[latest.status]}
                      {' · '}耗时{' '}
                      {Math.max(
                        0,
                        Math.round(
                          ((['queued', 'running'].includes(latest.status)
                            ? Date.now()
                            : Date.parse(latest.updatedAt)) -
                            Date.parse(latest.createdAt)) /
                            1000,
                        ),
                      )}{' '}
                      秒
                    </p>
                    {latest.steps
                      .filter((s) => s.name === '模型接口响应')
                      .map((s, i) => (
                        <p key={i}>{s.detail}</p>
                      ))}
                    <p>
                      {latest.outputIds
                        .map((id) => state.assets.find((a) => a.id === id)?.name || id)
                        .join('、') || '会话内容，无新增文件'}
                    </p>
                    {latest.result && <MarkdownContent content={latest.result} />}
                  </details>
                )}
                <div>
                  {m.taskIds.map((id, i) => {
                    const task = state.tasks.find((t) => t.id === id);
                    return (
                      task && (
                        <button key={id} onClick={() => onTask(task)}>
                          第 {i + 1} 轮<ChevronRight size={11} />
                        </button>
                      )
                    );
                  })}
                  {group.status === 'running' && (
                    <button
                      disabled={busy || m.instructions.length >= 8}
                      onClick={() => {
                        setSelected(selected === m.id ? '' : m.id);
                        setInstruction('');
                      }}
                    >
                      追加指令
                    </button>
                  )}
                </div>
                {selected === m.id && group.status === 'running' && (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      perform(async () => {
                        await post(`/agent-groups/${group.id}/members/${m.id}/instructions`, {
                          prompt: instruction,
                        });
                        setSelected('');
                        setInstruction('');
                      });
                    }}
                  >
                    <textarea
                      rows={2}
                      aria-label={`给 ${m.name} 追加指令`}
                      value={instruction}
                      onChange={(e) => setInstruction(e.target.value)}
                      placeholder="当前轮完成后，以本成员的结果继续处理…"
                    />
                    <button disabled={busy || !instruction.trim()} type="submit">
                      <Send size={12} />
                      加入下一轮
                    </button>
                  </form>
                )}
              </div>
            </article>
          );
        })}
      </div>
      {!!group.reviews?.length && (
        <details className="team-review-history" open>
          <summary>审阅与返工记录</summary>
          {group.reviews.map((review) => (
            <article key={review.taskId}>
              <strong>
                {review.round === 0 ? '首次审阅' : `返工第 ${review.round} 轮审阅`} ·{' '}
                {{ pass: '通过', rework: '返工', blocked: '阻塞' }[review.status]}
              </strong>
              <MarkdownContent content={review.summary} />
              {review.issues.map((issue, i) => (
                <p key={i}>
                  {group.members.find((m) => m.id === issue.nodeId)?.name || issue.nodeId}：
                  {issue.reason}
                </p>
              ))}
            </article>
          ))}
        </details>
      )}
      {group.status === 'aggregating' && (
        <div className="team-aggregation">
          <Loader2 size={14} className="spin" />
          主任务正在读取成员成果，生成最终交付…
        </div>
      )}
      {(error || group.error) && (
        <div className="module-error" role="alert">
          {error || group.error}
        </div>
      )}
    </section>
  );
}
