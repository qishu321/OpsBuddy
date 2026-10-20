import { useDialogFocus } from './useDialogFocus';
import { useEffect, useState } from 'react';
import {
  CalendarClock,
  Plus,
  Clock,
  Bell,
  Play,
  Pause,
  Pencil,
  Trash2,
  X,
  FolderOpen,
  History,
  ChevronRight,
  Square,
  Check,
} from 'lucide-react';
import {
  api,
  post,
  kinds,
  statusLabel,
  type State,
  type Task,
  type Schedule,
  type ScheduleRun,
  type DirectoryGrant,
} from './api';
import { desktop } from './desktop';

const weekNames = ['日', '一', '二', '三', '四', '五', '六'];
const displayTime = (at?: string, timezone = 'Asia/Shanghai') =>
  at
    ? new Date(at).toLocaleString('zh-CN', {
        timeZone: timezone,
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    : '—';
const runLabels: Record<string, string> = {
  missed: '已错过',
  skipped_overlap: '避免重叠 · 已跳过',
  preparing: '准备材料',
  ...statusLabel,
};
function newSchedule(projectId: string): Partial<Schedule> {
  const date = new Date(Date.now() + 3600000);
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
    .format(date)
    .split(' ');
  return {
    id: '',
    projectId,
    name: '',
    prompt: '',
    mode: 'task',
    frequency: 'once',
    timezone: 'Asia/Shanghai',
    startDate: parts[0],
    clock: parts[1],
    weekdays: [1, 2, 3, 4, 5],
    missedPolicy: 'skip',
    roleId: 'auto',
    kind: 'docx',
    modelId: '',
    allowRemote: false,
    useMemory: true,
    inputIds: [],
    skillIds: [],
    knowledgeIds: [],
    directoryId: '',
    version: 0,
  };
}
export default function SchedulePage({
  state,
  projectId,
  onRefresh,
  onTask,
}: {
  state: State;
  projectId: string;
  onRefresh: () => Promise<void>;
  onTask: (task: Task) => void;
}) {
  const [editing, setEditing] = useState<Partial<Schedule> | null>(null),
    [selected, setSelected] = useState(''),
    [tab, setTab] = useState('active'),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [picked, setPicked] = useState<DirectoryGrant | null>(null);
  useDialogFocus(!!editing);
  const [preview, setPreview] = useState<string[]>([]),
    [previewError, setPreviewError] = useState('');
  const weekdaysKey = editing?.weekdays?.join(',');
  useEffect(() => {
    if (!editing) return;
    setPreview([]);
    setPreviewError('');
    const abort = new AbortController();
    const timer = setTimeout(
      () =>
        api<{ times: string[] }>('/schedules/preview', {
          method: 'POST',
          body: JSON.stringify(editing),
          signal: abort.signal,
        })
          .then((result) => {
            setPreview(result.times);
            if (!result.times.length) setPreviewError('此时间已过去，请选择未来的执行时间。');
          })
          .catch((e) => {
            if (!abort.signal.aborted) setPreviewError(e.message);
          }),
      220,
    );
    return () => {
      clearTimeout(timer);
      abort.abort();
    };
  }, [
    !!editing,
    editing?.frequency,
    editing?.timezone,
    editing?.startDate,
    editing?.clock,
    weekdaysKey,
  ]);
  const perform = async (action: () => Promise<unknown>, message = '') => {
    setBusy(true);
    setError('');
    try {
      await action();
      await onRefresh();
      if (message) setNotice(message);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const open = (plan?: Schedule) => {
    setError('');
    setPicked(null);
    const draft = plan ? { ...plan } : newSchedule(projectId);
    if (!state.models.some((m) => m.id === draft.modelId)) {
      draft.modelId =
        state.models.find((m) => m.id === localStorage.getItem('opsbuddy-selected-model'))?.id ||
        state.models[0]?.id ||
        '';
      draft.allowRemote = false;
    }
    setEditing(draft);
  };
  const plans = state.schedules
    .filter((p) => p.projectId === projectId && (tab === 'all' || p.status === tab))
    .slice()
    .sort((a, b) => (a.nextAt || 'z').localeCompare(b.nextAt || 'z'));
  const selectedPlan = state.schedules.find((p) => p.id === selected);
  const runs = state.scheduleRuns
    .filter((r) => r.definition.projectId === projectId && (!selected || r.scheduleId === selected))
    .slice()
    .reverse();
  const openRun = (run: ScheduleRun) => {
    const task = state.tasks.find((t) => t.id === run.taskId);
    if (task) onTask(task);
  };
  const grant = picked || state.directories.find((g) => g.id === editing?.directoryId);
  return (
    <div className="assistant-page schedule-page" data-testid="schedule-page">
      <div className="page-title">
        <div>
          <span className="eyebrow">SCHEDULES</span>
          <h1>把下一次交给日程。</h1>
          <p>安排任务或提醒，每次执行都会留下独立会话。</p>
        </div>
        <button className="button primary" onClick={() => open()}>
          <Plus size={16} />
          新建定时任务
        </button>
      </div>
      <div className="assistant-info">
        <Clock size={18} />
        <div>
          <strong>桌面程序打开时执行</strong>
          <p>
            关闭或休眠期间不执行；错过后按计划跳过或只补跑一次。提醒在应用内显示，不向消息渠道发送。
          </p>
        </div>
      </div>
      {error && (
        <div className="module-error" role="alert">
          {error}
        </div>
      )}
      {notice && (
        <div className="module-notice">
          <Check size={14} />
          {notice}
        </div>
      )}
      <div className="schedule-layout">
        <section>
          <div className="assistant-filter-row">
            <div className="assistant-segments">
              {[
                ['active', '进行中'],
                ['paused', '已暂停'],
                ['completed', '已结束'],
                ['all', '全部'],
              ].map(([id, label]) => (
                <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}>
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="schedule-list">
            {plans.map((p) => (
              <article key={p.id} className={selected === p.id ? 'selected' : ''}>
                <button
                  className="schedule-open"
                  onClick={() => setSelected(selected === p.id ? '' : p.id)}
                >
                  <div className="schedule-time">
                    <span>
                      {p.nextAt
                        ? new Date(p.nextAt).toLocaleTimeString('zh-CN', {
                            timeZone: p.timezone,
                            hour: '2-digit',
                            minute: '2-digit',
                            hour12: false,
                          })
                        : p.clock}
                    </span>
                    <small>
                      {p.nextAt
                        ? new Date(p.nextAt).toLocaleDateString('zh-CN', {
                            timeZone: p.timezone,
                            month: '2-digit',
                            day: '2-digit',
                          })
                        : p.status === 'completed'
                          ? '已结束'
                          : '已暂停'}
                    </small>
                  </div>
                  <div className="schedule-copy">
                    <h3>
                      {p.mode === 'reminder' ? <Bell size={15} /> : <CalendarClock size={15} />}{' '}
                      {p.name}
                    </h3>
                    <p>{p.prompt}</p>
                    <footer>
                      <span>
                        {p.frequency === 'once'
                          ? '仅一次'
                          : p.frequency === 'daily'
                            ? '每天'
                            : `每周${p.weekdays.map((day) => weekNames[day]).join('、')}`}
                      </span>
                      <span>{p.timezone}</span>
                      <span>{p.missedPolicy === 'skip' ? '错过跳过' : '错过补跑一次'}</span>
                    </footer>
                  </div>
                  <ChevronRight size={15} />
                </button>
                <div className="schedule-actions">
                  <button
                    disabled={busy}
                    onClick={() =>
                      perform(() => post(`/schedules/${p.id}/run`), '已创建独立执行会话')
                    }
                  >
                    <Play size={13} />
                    立即运行
                  </button>
                  {p.status !== 'completed' && (
                    <button
                      disabled={busy}
                      onClick={() =>
                        perform(
                          () =>
                            post(
                              `/schedules/${p.id}/${p.status === 'paused' ? 'resume' : 'pause'}`,
                            ),
                          p.status === 'paused'
                            ? '计划已恢复'
                            : '已暂停后续触发，当前执行可在记录中停止',
                        )
                      }
                    >
                      {p.status === 'paused' ? <Play size={13} /> : <Pause size={13} />}{' '}
                      {p.status === 'paused' ? '恢复' : '暂停'}
                    </button>
                  )}
                  <button onClick={() => open(p)}>
                    <Pencil size={13} />
                    编辑
                  </button>
                  <button
                    aria-label={`删除计划 ${p.name}`}
                    disabled={busy}
                    onClick={() =>
                      perform(async () => {
                        await api(`/schedules/${p.id}`, { method: 'DELETE' });
                        if (selected === p.id) setSelected('');
                      }, '计划已删除；历史执行与成果保留')
                    }
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              </article>
            ))}
          </div>
          {!plans.length && (
            <div className="assistant-empty">
              <CalendarClock size={32} />
              <h3>为下一次工作留个时间</h3>
              <p>可以每天整理项目材料，也可以设置一条一次性提醒。</p>
              <button className="button" onClick={() => open()}>
                新建定时任务
              </button>
            </div>
          )}
        </section>
        <aside className="schedule-history">
          <header>
            <History size={16} />
            <strong>{selectedPlan ? selectedPlan.name : '最近执行'}</strong>
            {selected && (
              <button aria-label="查看全部执行记录" onClick={() => setSelected('')}>
                <X size={14} />
              </button>
            )}
          </header>
          <p>每个执行记录关联独立会话；跳过的触发不发起模型请求。</p>
          <div>
            {runs.slice(0, 60).map((r) => (
              <article key={r.id}>
                <div className={`schedule-run-dot ${r.status}`} />
                <div>
                  <strong>{r.definition.name}</strong>
                  <span>
                    {displayTime(r.scheduledAt, r.definition.timezone)} ·{' '}
                    {r.trigger === 'manual' ? '手动' : r.trigger === 'catch_up' ? '补跑' : '定时'}
                  </span>
                  <small>{runLabels[r.status] || r.status}</small>
                  {r.error && <p>{r.error}</p>}
                  <footer>
                    {r.taskId && (
                      <button
                        disabled={!state.tasks.some((t) => t.id === r.taskId)}
                        onClick={() => openRun(r)}
                      >
                        {state.tasks.some((t) => t.id === r.taskId) ? '查看会话' : '会话已删除'}
                        <ChevronRight size={12} />
                      </button>
                    )}
                    {['queued', 'preparing', 'running'].includes(r.status) && (
                      <button
                        disabled={busy}
                        onClick={() =>
                          perform(() => post(`/schedule-runs/${r.id}/cancel`), '已停止本次执行')
                        }
                      >
                        <Square size={11} />
                        停止
                      </button>
                    )}
                  </footer>
                </div>
              </article>
            ))}
          </div>
          {!runs.length && <div className="schedule-no-runs">执行后，过程和成果会出现在这里。</div>}
        </aside>
      </div>
      {editing && (
        <div className="module-overlay">
          <form
            className="module-dialog assistant-dialog schedule-dialog"
            role="dialog"
            aria-modal="true"
            aria-label="编辑定时任务"
            onKeyDown={(e) => {
              if (e.key === 'Escape' && !busy) setEditing(null);
            }}
            onSubmit={(e) => {
              e.preventDefault();
              perform(async () => {
                await post('/schedules', { ...editing, projectId });
                setEditing(null);
              }, '定时任务已保存');
            }}
          >
            <header>
              <div>
                <span className="eyebrow">SCHEDULE</span>
                <h2>{editing.id ? '编辑定时任务' : '安排下一次执行'}</h2>
              </div>
              <button
                type="button"
                className="icon-button"
                aria-label="关闭定时任务编辑"
                onClick={() => setEditing(null)}
              >
                <X size={18} />
              </button>
            </header>
            <label>
              计划名称
              <input
                required
                maxLength={80}
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                placeholder="例如：每天整理项目日报"
              />
            </label>
            <div className="assistant-segments schedule-mode">
              {[
                ['task', '执行任务'],
                ['reminder', '到时提醒'],
              ].map(([id, label]) => (
                <button
                  type="button"
                  key={id}
                  className={editing.mode === id ? 'active' : ''}
                  onClick={() => setEditing({ ...editing, mode: id as Schedule['mode'] })}
                >
                  {label}
                </button>
              ))}
            </div>
            <label>
              {editing.mode === 'reminder' ? '提醒内容' : '任务要求'}
              <textarea
                required
                rows={3}
                maxLength={10000}
                value={editing.prompt}
                onChange={(e) => setEditing({ ...editing, prompt: e.target.value })}
                placeholder={
                  editing.mode === 'reminder'
                    ? '例如：检查周报并补充本周风险。'
                    : '描述任务、交付要求和需要遵守的约束。'
                }
              />
            </label>
            <div className="assistant-form-row">
              <label>
                执行频率
                <select
                  value={editing.frequency}
                  onChange={(e) =>
                    setEditing({ ...editing, frequency: e.target.value as Schedule['frequency'] })
                  }
                >
                  <option value="once">仅一次</option>
                  <option value="daily">每天</option>
                  <option value="weekly">每周</option>
                </select>
              </label>
              <label>
                时区
                <select
                  value={editing.timezone}
                  onChange={(e) => setEditing({ ...editing, timezone: e.target.value })}
                >
                  {['Asia/Shanghai', 'Asia/Tokyo', 'UTC', 'America/New_York', 'Europe/London'].map(
                    (zone) => (
                      <option key={zone}>{zone}</option>
                    ),
                  )}
                </select>
              </label>
            </div>
            <div className="assistant-form-row">
              <label>
                {editing.frequency === 'once' ? '执行日期' : '开始日期'}
                <input
                  required
                  type="date"
                  value={editing.startDate}
                  onChange={(e) => setEditing({ ...editing, startDate: e.target.value })}
                />
              </label>
              <label>
                当地时间
                <input
                  required
                  type="time"
                  value={editing.clock}
                  onChange={(e) => setEditing({ ...editing, clock: e.target.value })}
                />
              </label>
            </div>
            {editing.frequency === 'weekly' && (
              <div className="schedule-weekdays" aria-label="执行星期">
                {weekNames.map((label, day) => (
                  <button
                    key={day}
                    type="button"
                    aria-pressed={editing.weekdays?.includes(day)}
                    onClick={() =>
                      setEditing({
                        ...editing,
                        weekdays: editing.weekdays?.includes(day)
                          ? editing.weekdays.filter((d) => d !== day)
                          : [...(editing.weekdays || []), day].sort(),
                      })
                    }
                  >
                    周{label}
                  </button>
                ))}
              </div>
            )}
            <div className="schedule-preview">
              <strong>接下来执行 · {editing.timezone}</strong>
              {previewError ? (
                <p>{previewError}</p>
              ) : preview.length ? (
                <div>
                  {preview.map((at) => (
                    <span key={at}>{displayTime(at, editing.timezone)}</span>
                  ))}
                </div>
              ) : (
                <p>正在计算执行时间…</p>
              )}
            </div>
            <label>
              错过执行时
              <select
                value={editing.missedPolicy}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    missedPolicy: e.target.value as Schedule['missedPolicy'],
                  })
                }
              >
                <option value="skip">跳过，并保留错过记录</option>
                <option value="catch_up_once">程序恢复后只补跑一次</option>
              </select>
            </label>
            {editing.mode === 'task' && (
              <>
                <div className="assistant-form-row">
                  <label>
                    成果类型
                    <select
                      value={editing.kind}
                      onChange={(e) => setEditing({ ...editing, kind: e.target.value })}
                    >
                      {Object.entries(kinds).map(([id, label]) => (
                        <option key={id} value={id}>
                          {label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    使用 Skills（可选）
                    <select
                      multiple
                      aria-label="定时任务技能"
                      value={[
                        ...(editing.skillIds || []),
                        ...state.roles
                          .filter((r) => r.id === editing.roleId && r.migratedSkillId)
                          .map((r) => r.migratedSkillId!),
                      ]}
                      onChange={(e) =>
                        setEditing({
                          ...editing,
                          roleId: 'auto',
                          skillIds: Array.from(e.target.selectedOptions, (o) => o.value),
                          kind:
                            editing.kind === 'auto'
                              ? state.roles.find((r) => r.id === editing.roleId)?.kind || 'auto'
                              : editing.kind,
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
                    <small>不选则自动匹配；多选使用 Ctrl。可在扩展中管理。</small>
                  </label>
                </div>
                <label>
                  模型
                  <select
                    value={editing.modelId}
                    onChange={(e) =>
                      setEditing({ ...editing, modelId: e.target.value, allowRemote: false })
                    }
                  >
                    {!editing.modelId && (
                      <option value="" disabled>
                        请先添加模型
                      </option>
                    )}
                    {state.models.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.name}
                      </option>
                    ))}
                  </select>
                </label>
                {editing.modelId && (
                  <label className="assistant-check">
                    <input
                      required
                      type="checkbox"
                      checked={editing.allowRemote}
                      onChange={(e) => setEditing({ ...editing, allowRemote: e.target.checked })}
                    />
                    允许此计划每次向所选服务发送任务、材料及引用记忆
                  </label>
                )}
                <label className="assistant-check">
                  <input
                    type="checkbox"
                    checked={editing.useMemory !== false}
                    onChange={(e) => setEditing({ ...editing, useMemory: e.target.checked })}
                  />
                  按需使用已启用记忆
                </label>
                <div className="schedule-directory">
                  <label>工作目录（可选）</label>
                  <div>
                    <FolderOpen size={17} />
                    <span>{grant?.path || '未选择；仅使用项目内勾选的文件'}</span>
                    {editing.directoryId && (
                      <button
                        type="button"
                        onClick={() => {
                          setPicked(null);
                          setEditing({ ...editing, directoryId: '' });
                        }}
                      >
                        <X size={13} />
                      </button>
                    )}
                    <button
                      className="text-button"
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        perform(async () => {
                          if (!desktop()) throw new Error('请在桌面版选择工作目录');
                          const directory = await desktop()!.PickScheduleDirectory(projectId);
                          if (directory?.id) {
                            setPicked(directory);
                            setEditing({ ...editing, directoryId: directory.id });
                          }
                        })
                      }
                    >
                      选择文件夹
                    </button>
                  </div>
                  <small>
                    每次读取并复制目录内材料（最多 16 个文件、32
                    MB）；原目录保留，成果存入应用，支持另存。
                  </small>
                </div>
                {state.assets.some((a) => a.projectId === projectId) && (
                  <details className="schedule-resources">
                    <summary>固定引用项目文件 · 已选 {editing.inputIds?.length || 0} 个</summary>
                    <div>
                      {state.assets
                        .filter((a) => a.projectId === projectId)
                        .slice(0, 100)
                        .map((a) => (
                          <label className="assistant-check" key={a.id}>
                            <input
                              type="checkbox"
                              checked={editing.inputIds?.includes(a.id)}
                              onChange={(e) =>
                                setEditing({
                                  ...editing,
                                  inputIds: e.target.checked
                                    ? [...(editing.inputIds || []), a.id]
                                    : editing.inputIds?.filter((id) => id !== a.id),
                                })
                              }
                            />
                            {a.name}
                          </label>
                        ))}
                    </div>
                  </details>
                )}
                {state.knowledgeBases.some((b) => b.projectId === projectId) && (
                  <details className="schedule-resources">
                    <summary>引用知识库 · 已选 {editing.knowledgeIds?.length || 0} 个</summary>
                    <div>
                      {state.knowledgeBases
                        .filter((b) => b.projectId === projectId)
                        .map((b) => (
                          <label className="assistant-check" key={b.id}>
                            <input
                              type="checkbox"
                              checked={editing.knowledgeIds?.includes(b.id)}
                              onChange={(e) =>
                                setEditing({
                                  ...editing,
                                  knowledgeIds: e.target.checked
                                    ? [...(editing.knowledgeIds || []), b.id]
                                    : editing.knowledgeIds?.filter((id) => id !== b.id),
                                })
                              }
                            />
                            {b.name}
                          </label>
                        ))}
                    </div>
                  </details>
                )}
              </>
            )}
            <div className="schedule-form-note">
              <Clock size={14} />
              <span>
                程序打开时到点执行。每次使用当时的技能、项目说明、知识库与记忆，并记录快照；模型服务地址改变后须重新确认计划授权。
              </span>
            </div>
            {error && (
              <div className="module-error" role="alert">
                {error}
              </div>
            )}
            <footer>
              <button className="button" type="button" onClick={() => setEditing(null)}>
                取消
              </button>
              <button
                className="button primary"
                type="submit"
                disabled={
                  busy || (editing.mode === 'task' && (!editing.modelId || !editing.allowRemote))
                }
              >
                保存定时任务
              </button>
            </footer>
          </form>
        </div>
      )}
    </div>
  );
}
