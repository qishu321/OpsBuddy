import { useDialogFocus } from './useDialogFocus';
import { useEffect, useState } from 'react';
import {
  ArrowLeft,
  BookOpen,
  Plus,
  Search,
  Pencil,
  Trash2,
  X,
  Check,
  Link2,
  Pause,
  Play,
} from 'lucide-react';
import { api, post, type State, type Task, type Memory } from './api';

export default function MemoryPage({
  state,
  projectId,
  onRefresh,
  onTask,
  onBack,
  seed,
  onClearSeed,
}: {
  state: State;
  projectId: string;
  onRefresh: () => Promise<void>;
  onTask: (task: Task) => void;
  onBack: () => void;
  seed?: Task;
  onClearSeed: () => void;
}) {
  const [query, setQuery] = useState(''),
    [filter, setFilter] = useState('all'),
    [editing, setEditing] = useState<Partial<Memory> | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState('');
  const create = (source?: Task) =>
    setEditing({
      id: '',
      scope: 'project',
      projectId,
      kind: 'preference',
      title: source ? source.title.slice(0, 60) : '',
      content: source?.prompt || '',
      keywords: '',
      enabled: true,
      sourceTaskId: source?.id,
      version: 0,
    });
  useEffect(() => {
    if (seed) {
      create(seed);
      onClearSeed();
    }
  }, [seed]);
  useDialogFocus(!!editing);
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
  const memories = state.memories.filter(
    (m) =>
      (m.scope === 'personal' || m.projectId === projectId) &&
      (filter === 'all' || (filter === 'disabled' && !m.enabled) || m.scope === filter) &&
      (!query || (m.title + m.content + m.keywords).toLowerCase().includes(query.toLowerCase())),
  );
  return (
    <div className="assistant-page memory-page" data-testid="memory-page">
      <button className="text-button assistant-back" onClick={onBack}>
        <ArrowLeft size={14} />
        返回设置
      </button>
      <div className="page-title">
        <div>
          <span className="eyebrow">PERSONAL MEMORY</span>
          <h1>让熟悉的偏好，留在身边。</h1>
          <p>你决定记住什么、在哪里使用，也可以随时停用。</p>
        </div>
        <button className="button primary" onClick={() => create()}>
          <Plus size={16} />
          添加记忆
        </button>
      </div>
      <div className="assistant-info">
        <BookOpen size={18} />
        <div>
          <strong>当前要求始终优先</strong>
          <p>
            任务按相关性读取最多 6
            条已启用记忆。项目偏好优先于同名个人偏好，历史任务保留当时的引用。
          </p>
        </div>
      </div>
      <div className="assistant-filter-row">
        <div className="assistant-segments">
          {[
            ['all', '全部'],
            ['personal', '个人'],
            ['project', '当前项目'],
            ['disabled', '已停用'],
          ].map(([id, label]) => (
            <button
              key={id}
              className={filter === id ? 'active' : ''}
              onClick={() => setFilter(id)}
            >
              {label}
            </button>
          ))}
        </div>
        <label className="assistant-search">
          <Search size={15} />
          <input
            value={query}
            aria-label="搜索记忆"
            placeholder="搜索标题、内容或关键词"
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
      </div>
      {notice && (
        <div className="module-notice">
          <Check size={14} />
          {notice}
        </div>
      )}
      {error && (
        <div className="module-error" role="alert">
          {error}
        </div>
      )}
      <div className="memory-list">
        {memories.map((m) => (
          <article key={m.id} className={!m.enabled ? 'disabled' : ''}>
            <div className={`memory-scope-icon ${m.scope}`}>
              <BookOpen size={19} />
            </div>
            <div className="memory-copy">
              <div>
                <h3>{m.title}</h3>
                <span>
                  {m.scope === 'personal' ? '个人' : '当前项目'} ·{' '}
                  {m.kind === 'preference' ? '偏好' : '经验'}
                </span>
              </div>
              <p>{m.content}</p>
              <footer>
                {m.keywords && <span>匹配：{m.keywords}</span>}
                <small>
                  v{m.version} ·{' '}
                  {new Date(m.updatedAt).toLocaleDateString('zh-CN', { timeZone: 'Asia/Shanghai' })}
                </small>
                {m.sourceTaskId && (
                  <button
                    disabled={!state.tasks.some((t) => t.id === m.sourceTaskId)}
                    onClick={() => {
                      const t = state.tasks.find((t) => t.id === m.sourceTaskId);
                      if (t) onTask(t);
                    }}
                  >
                    <Link2 size={12} />
                    {state.tasks.some((t) => t.id === m.sourceTaskId)
                      ? '来源会话'
                      : '来源会话已删除'}
                  </button>
                )}
              </footer>
            </div>
            <div className="memory-actions">
              <button
                className="icon-button"
                title={m.enabled ? '停用记忆' : '启用记忆'}
                aria-label={`${m.enabled ? '停用' : '启用'} ${m.title}`}
                disabled={busy}
                onClick={() =>
                  perform(
                    () => post('/memories', { ...m, enabled: !m.enabled }),
                    m.enabled ? '已停用，新任务不再读取' : '已启用',
                  )
                }
              >
                {m.enabled ? <Pause size={15} /> : <Play size={15} />}
              </button>
              <button
                className="icon-button"
                title="编辑记忆"
                aria-label={`编辑 ${m.title}`}
                onClick={() => setEditing({ ...m })}
              >
                <Pencil size={15} />
              </button>
              <button
                className="icon-button"
                title="删除记忆"
                aria-label={`删除 ${m.title}`}
                disabled={busy}
                onClick={() =>
                  perform(
                    () => api(`/memories/${m.id}`, { method: 'DELETE' }),
                    '已删除；历史任务中的引用快照保留',
                  )
                }
              >
                <Trash2 size={15} />
              </button>
            </div>
          </article>
        ))}
      </div>
      {!memories.length && (
        <div className="assistant-empty">
          <BookOpen size={32} />
          <h3>从一条明确的偏好开始</h3>
          <p>例如“汇报先写结论，使用简洁中文”。记忆由你保存，不自动提取附件里的要求。</p>
          <button className="button" onClick={() => create()}>
            添加第一条记忆
          </button>
        </div>
      )}
      {editing && (
        <div className="module-overlay">
          <form
            className="module-dialog assistant-dialog"
            role="dialog"
            aria-modal="true"
            aria-label="编辑记忆"
            onKeyDown={(e) => {
              if (e.key === 'Escape' && !busy) setEditing(null);
            }}
            onSubmit={(e) => {
              e.preventDefault();
              perform(async () => {
                await post('/memories', {
                  ...editing,
                  projectId: editing.scope === 'project' ? projectId : '',
                });
                setEditing(null);
              }, '记忆已保存，可在此停用或删除');
            }}
          >
            <header>
              <div>
                <span className="eyebrow">MEMORY</span>
                <h2>{editing.id ? '编辑记忆' : '保存一条偏好或经验'}</h2>
              </div>
              <button
                type="button"
                className="icon-button"
                aria-label="关闭记忆编辑"
                onClick={() => setEditing(null)}
              >
                <X size={18} />
              </button>
            </header>
            <label>
              标题
              <input
                required
                autoFocus
                maxLength={60}
                value={editing.title}
                onChange={(e) => setEditing({ ...editing, title: e.target.value })}
                placeholder="例如：汇报写作方式"
              />
            </label>
            <div className="assistant-form-row">
              <label>
                使用范围
                <select
                  value={editing.scope}
                  onChange={(e) =>
                    setEditing({ ...editing, scope: e.target.value as Memory['scope'] })
                  }
                >
                  <option value="project">当前项目</option>
                  <option value="personal">个人 · 所有项目</option>
                </select>
              </label>
              <label>
                类型
                <select
                  value={editing.kind}
                  onChange={(e) =>
                    setEditing({ ...editing, kind: e.target.value as Memory['kind'] })
                  }
                >
                  <option value="preference">偏好</option>
                  <option value="experience">项目经验</option>
                </select>
              </label>
            </div>
            <label>
              需要记住的内容
              <textarea
                required
                maxLength={2000}
                rows={5}
                value={editing.content}
                onChange={(e) => setEditing({ ...editing, content: e.target.value })}
                placeholder="只写你希望日后复用的偏好或经验，不保存密码与密钥。"
              />
            </label>
            <label>
              匹配关键词（可选）
              <input
                maxLength={200}
                value={editing.keywords}
                onChange={(e) => setEditing({ ...editing, keywords: e.target.value })}
                placeholder="PPT, 汇报, 演示"
              />
              <small>
                填写后仅在任务包含关键词时读取；不填写的偏好可用于所有相关任务，经验按内容匹配。
              </small>
            </label>
            <label className="assistant-check">
              <input
                type="checkbox"
                checked={editing.enabled}
                onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })}
              />
              保存后启用
            </label>
            {error && (
              <div className="module-error" role="alert">
                {error}
              </div>
            )}
            <footer>
              <button type="button" className="button" onClick={() => setEditing(null)}>
                取消
              </button>
              <button type="submit" className="button primary" disabled={busy}>
                保存记忆
              </button>
            </footer>
          </form>
        </div>
      )}
    </div>
  );
}
