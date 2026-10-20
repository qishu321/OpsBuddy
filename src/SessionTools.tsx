import { useEffect, useRef, useState } from 'react';
import { Plus, Paperclip, FolderOpen, Files, Puzzle, BookOpen, X, Search } from 'lucide-react';
import MarkdownContent from './MarkdownContent';
import { fileURL, size, type State, type Task, type Asset } from './api';

export function MaterialPicker({
  state,
  projectId,
  assetIds,
  skillIds,
  onAssets,
  onSkills,
  onFiles,
  onFolder,
  disabled,
}: {
  state: State;
  projectId: string;
  assetIds: string[];
  skillIds: string[];
  onAssets: (ids: string[]) => void;
  onSkills: (ids: string[]) => void;
  onFiles: () => void;
  onFolder: () => void;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false),
    [tab, setTab] = useState('files'),
    [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('pointerdown', close);
    document.addEventListener('keydown', escape);
    const resize = () => setOpen(false);
    window.addEventListener('resize', resize);
    return () => {
      document.removeEventListener('pointerdown', close);
      document.removeEventListener('keydown', escape);
      window.removeEventListener('resize', resize);
    };
  }, [open]);
  const assets = state.assets.filter(
    (a) =>
      a.projectId === projectId &&
      (a.sourceName || a.name).toLowerCase().includes(query.toLowerCase()),
  );
  const extensions = state.extensions.filter(
    (e) =>
      e.enabled &&
      e.kind === (tab === 'skills' ? 'skill' : 'plugin') &&
      (e.name + e.description).toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className="material-picker" ref={root}>
      <button
        className="icon-button attach"
        type="button"
        title="添加材料"
        aria-label="添加材料"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen(!open)}
      >
        <Plus size={21} />
      </button>
      {open && (
        <div
          className="material-popover"
          role="dialog"
          aria-label="引用材料"
          style={{
            position: 'fixed',
            left: Math.max(
              12,
              Math.min(root.current?.getBoundingClientRect().left || 12, innerWidth - 376),
            ),
            bottom:
              innerHeight - (root.current?.getBoundingClientRect().top || innerHeight - 12) + 8,
            maxHeight: Math.min(440, (root.current?.getBoundingClientRect().top || 460) - 20),
            overflowY: 'auto',
          }}
        >
          <header>
            <strong>添加材料</strong>
            <button type="button" aria-label="关闭材料选择" onClick={() => setOpen(false)}>
              <X size={15} />
            </button>
          </header>
          <div className="material-native-actions">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onFiles();
              }}
            >
              <Paperclip size={17} />
              <span>
                本机文件<small>文档、表格、PNG / JPEG</small>
              </span>
            </button>
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onFolder();
              }}
            >
              <FolderOpen size={17} />
              <span>
                引用文件夹<small>复制材料，保留原目录</small>
              </span>
            </button>
          </div>
          <div className="material-tabs">
            {[
              ['files', '项目文件', Files],
              ['skills', 'Skill', BookOpen],
              ['plugins', '插件', Puzzle],
            ].map(([id, label, Icon]) => {
              const I = Icon as typeof Files;
              return (
                <button
                  type="button"
                  key={id as string}
                  className={tab === id ? 'active' : ''}
                  onClick={() => setTab(id as string)}
                >
                  <I size={13} />
                  {label as string}
                </button>
              );
            })}
          </div>
          <label className="material-search">
            <Search size={14} />
            <input
              aria-label="搜索可引用材料"
              placeholder="搜索名称或说明"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <div className="material-options">
            {tab === 'files'
              ? assets.map((a) => (
                  <label key={a.id}>
                    <input
                      type="checkbox"
                      checked={assetIds.includes(a.id)}
                      disabled={!assetIds.includes(a.id) && assetIds.length >= 16}
                      onChange={(e) =>
                        onAssets(
                          e.target.checked
                            ? [...assetIds, a.id]
                            : assetIds.filter((id) => id !== a.id),
                        )
                      }
                    />
                    <span>
                      {a.sourceName || a.name}
                      <small>
                        {a.kind.toUpperCase()} · {size(a.size)}
                      </small>
                    </span>
                  </label>
                ))
              : extensions.map((x) => (
                  <label key={x.id}>
                    <input
                      type="checkbox"
                      checked={skillIds.includes(x.id)}
                      disabled={!skillIds.includes(x.id) && skillIds.length >= 16}
                      onChange={(e) =>
                        onSkills(
                          e.target.checked
                            ? [...skillIds, x.id]
                            : skillIds.filter((id) => id !== x.id),
                        )
                      }
                    />
                    <span>
                      {x.name}
                      {x.builtin ? ' · 内置' : ''}
                      <small>{x.description || '已启用的技能包'}</small>
                    </span>
                  </label>
                ))}
            {!(tab === 'files' ? assets.length : extensions.length) && (
              <p className="material-empty">
                {tab === 'files'
                  ? '还没有项目文件，可先添加本机材料。'
                  : '没有匹配的已启用项目，请先在「扩展」中添加。'}
              </p>
            )}
          </div>
          <footer>
            {tab === 'files'
              ? '每轮最多 16 个文件，图片也可拖入输入框。'
              : '先提供候选名称与说明，模型按任务需要选择并读取正文。插件目前支持声明式技能包。'}
          </footer>
        </div>
      )}
    </div>
  );
}

export function ConversationHistory({
  turns,
  state,
  onAsset,
}: {
  turns: Task[];
  state: State;
  onAsset: (a: Asset) => void;
}) {
  if (!turns.length) return null;
  return (
    <div className="conversation-history" data-testid="conversation-history">
      {turns.map((t, i) => (
        <details key={t.id} open={i === turns.length - 1}>
          <summary>
            <span>第 {i + 1} 轮</span>
            {t.prompt}
            <small>
              {new Date(t.createdAt).toLocaleTimeString('zh-CN', {
                hour: '2-digit',
                minute: '2-digit',
              })}
            </small>
          </summary>
          <div>
            <p className="history-user-prompt">{t.prompt}</p>
            <MarkdownContent content={t.result || t.error || '本轮未交付结果'} />
            <div className="history-deliverables">
              {t.outputIds
                .map((id) => state.assets.find((a) => a.id === id && a.projectId === t.projectId))
                .filter((a): a is Asset => !!a)
                .map((a) => (
                  <button key={a.id} onClick={() => onAsset(a)}>
                    <Files size={13} />
                    {a.sourceName || a.name}
                  </button>
                ))}
            </div>
          </div>
        </details>
      ))}
    </div>
  );
}

export function ImagePreview({ asset }: { asset: Asset }) {
  return (
    <div className="asset-image-preview">
      <img src={fileURL(asset, true)} alt={asset.name} />
    </div>
  );
}
