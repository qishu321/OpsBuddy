import { useEffect, useRef, useState, type CSSProperties } from 'react';
import {
  ChevronLeft,
  ChevronRight,
  Code,
  Download,
  Files,
  FolderOpen,
  List,
  Maximize2,
  Minimize2,
  MoreHorizontal,
  Pencil,
  Search,
  ShieldCheck,
  X,
} from 'lucide-react';
import { fileURL, size, type Asset } from './api';
import { desktop, isDesktop } from './desktop';
import TextArtifactPreview from './TextArtifactPreview';
import CodeProjectPreview from './CodeProjectPreview';
import { ImagePreview } from './SessionTools';

export default function ArtifactSidebar({
  asset,
  files,
  conversationIDs,
  hasConversation,
  onSelect,
  onClose,
  onSaved,
  onSave,
  onContinue,
  onAction,
}: {
  asset?: Asset;
  files: Asset[];
  conversationIDs: Set<string>;
  hasConversation: boolean;
  onSelect: (id: string) => void;
  onClose: () => void;
  onSaved: (next: Asset) => Promise<void>;
  onSave: (asset: Asset) => void;
  onContinue: (asset: Asset) => void;
  onAction: (action: () => Promise<unknown>) => void;
}) {
  const [expanded, setExpanded] = useState(false),
    [showFiles, setShowFiles] = useState(true),
    [search, setSearch] = useState(''),
    [scope, setScope] = useState<'session' | 'project'>('session');
  const [width, setWidth] = useState(() => {
    const value = Number(localStorage.getItem('opsbuddy-artifact-width'));
    return value >= 480 && value <= 1600 ? value : 0;
  });
  const panel = useRef<HTMLElement>(null);
  const resizing = useRef(false);
  useEffect(() => {
    const move = (e: PointerEvent) => {
      if (!resizing.current || !panel.current) return;
      const container = panel.current.parentElement!.getBoundingClientRect();
      const limit = window.innerWidth <= 1250 ? window.innerWidth - 80 : container.width - 340;
      const next = Math.max(Math.min(480, limit), Math.min(limit, container.right - e.clientX));
      setWidth(next);
    };
    const stop = () => {
      resizing.current = false;
      document.body.classList.remove('resizing-artifact');
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop);
    window.addEventListener('pointercancel', stop);
    return () => {
      stop();
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
      window.removeEventListener('pointercancel', stop);
    };
  }, []);
  useEffect(() => {
    if (width) localStorage.setItem('opsbuddy-artifact-width', String(Math.round(width)));
  }, [width]);
  const scoped = files.filter(
    (f) =>
      scope === 'project' || !hasConversation || conversationIDs.has(f.id) || f.id === asset?.id,
  );
  const visible = scoped.filter((f) => f.name.toLowerCase().includes(search.toLowerCase()));
  const current = scoped.findIndex((f) => f.id === asset?.id);
  const changeWidth = (delta: number) => {
    const currentWidth = panel.current?.getBoundingClientRect().width || 600;
    const available =
      panel.current?.parentElement?.getBoundingClientRect().width || window.innerWidth;
    setWidth(Math.max(480, Math.min(available - 340, currentWidth + delta)));
  };
  return (
    <aside
      ref={panel}
      aria-label="文件与预览"
      className={`artifact-panel artifact-studio${expanded ? ' expanded' : ''}${showFiles ? '' : ' files-hidden'}`}
      style={width ? ({ '--artifact-width': `${width}px` } as CSSProperties) : undefined}
    >
      {!expanded && (
        <div
          className="artifact-resize"
          role="separator"
          tabIndex={0}
          aria-label="调整右侧栏宽度"
          aria-orientation="vertical"
          aria-valuenow={Math.round(width || panel.current?.getBoundingClientRect().width || 600)}
          onDoubleClick={() => {
            setWidth(0);
            localStorage.removeItem('opsbuddy-artifact-width');
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
              e.preventDefault();
              changeWidth(e.key === 'ArrowLeft' ? 40 : -40);
            }
          }}
          onPointerDown={(e) => {
            e.preventDefault();
            resizing.current = true;
            e.currentTarget.setPointerCapture(e.pointerId);
            document.body.classList.add('resizing-artifact');
          }}
        />
      )}
      <header className="artifact-studio-header">
        <div>
          <span>
            {scope === 'session' && hasConversation ? '此会话中的文件' : '项目文件'} ·{' '}
            {scoped.length}
          </span>
          <strong>{asset?.name || '文件与预览'}</strong>
          <small>
            {asset ? `${asset.kind.toUpperCase()} · ${size(asset.size)}` : '选择文件查看内容'}
          </small>
        </div>
        <nav aria-label="文件预览操作">
          <button
            className="icon-button"
            aria-label="上一个文件"
            disabled={current <= 0}
            onClick={() => onSelect(scoped[current - 1].id)}
          >
            <ChevronLeft size={16} />
          </button>
          <button
            className="icon-button"
            aria-label="下一个文件"
            disabled={current < 0 || current >= scoped.length - 1}
            onClick={() => onSelect(scoped[current + 1].id)}
          >
            <ChevronRight size={16} />
          </button>
          {asset && (
            <button
              className="icon-button"
              aria-label={asset.kind === 'code' ? '导出项目 ZIP' : '另存文件'}
              title={asset.kind === 'code' ? '导出项目 ZIP' : '另存文件'}
              onClick={() => onSave(asset)}
            >
              <Download size={16} />
            </button>
          )}
          <button
            className="icon-button"
            aria-label={expanded ? '收起成果预览' : '展开成果预览'}
            title={expanded ? '退出大预览' : '大预览'}
            onClick={() => setExpanded((v) => !v)}
          >
            {expanded ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
          </button>
          <button
            className="icon-button"
            aria-label="关闭成果预览"
            title="关闭右侧栏 · Ctrl+Shift+B"
            onClick={onClose}
          >
            <X size={17} />
          </button>
        </nav>
      </header>
      <div className="artifact-studio-body">
        {showFiles && (
          <section className="artifact-file-browser" aria-label="预览文件列表">
            <select
              aria-label="右侧文件范围"
              value={scope}
              onChange={(e) => setScope(e.target.value as 'session' | 'project')}
            >
              <option value="session">{hasConversation ? '当前会话' : '当前项目'}</option>
              <option value="project">全部项目文件</option>
            </select>
            <label className="artifact-file-search">
              <Search size={13} />
              <input
                aria-label="搜索预览文件"
                placeholder="搜索文件…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            <div className="artifact-file-items">
              {visible.map((f) => (
                <button
                  key={f.id}
                  title={f.name}
                  className={f.id === asset?.id ? 'selected' : ''}
                  aria-label={`打开文件 ${f.name}`}
                  aria-current={f.id === asset?.id ? 'true' : undefined}
                  onClick={() => onSelect(f.id)}
                >
                  {f.kind === 'html' || f.kind === 'code' ? (
                    <Code size={16} />
                  ) : (
                    <Files size={16} />
                  )}
                  <span>
                    {f.name}
                    <small>
                      {f.kind === 'code' ? '源码项目' : f.kind.toUpperCase()} · {size(f.size)}
                    </small>
                  </span>
                </button>
              ))}
              {!visible.length && <p>{search ? '没有匹配的文件' : '当前会话还没有文件'}</p>}
            </div>
          </section>
        )}
        <div className="artifact-viewer">
          <button
            className="artifact-list-toggle"
            aria-label={showFiles ? '隐藏预览文件列表' : '显示预览文件列表'}
            title={showFiles ? '收起文件列表' : '显示文件列表'}
            onClick={() => setShowFiles((v) => !v)}
          >
            <List size={14} />
          </button>
          <div className="artifact-preview">
            {!asset ? (
              <div className="artifact-empty">
                <Files size={30} />
                <h3>文件都在这里</h3>
                <p>选择左侧文件进行预览，或在会话中生成新的文件。</p>
              </div>
            ) : asset.kind === 'code' ? (
              <CodeProjectPreview key={asset.id} asset={asset} onSaved={onSaved} />
            ) : ['md', 'html'].includes(asset.kind) ? (
              <TextArtifactPreview key={asset.id} asset={asset} onSaved={onSaved} />
            ) : ['png', 'jpg', 'jpeg'].includes(asset.kind) ? (
              <ImagePreview asset={asset} />
            ) : asset.kind === 'pdf' ? (
              <iframe src={fileURL(asset, true)} title="PDF 预览" />
            ) : (
              <>
                <div className="preview-label">
                  {asset.kind === 'zip' ? '分类清单' : '内容预览 · 非排版渲染'}
                </div>
                <div className={`paper ${asset.kind === 'xlsx' ? 'sheet' : ''}`}>
                  <pre>{asset.preview || '此文件暂无内容预览，请另存查看。'}</pre>
                </div>
              </>
            )}
          </div>
          {asset && (
            <footer className="artifact-actionbar">
              <span title={asset.validation}>
                <ShieldCheck size={13} />
                {asset.kind === 'html'
                  ? '单文件网页预览'
                  : asset.kind === 'md'
                    ? 'Markdown'
                    : '文件预览'}
              </span>
              {['docx', 'pptx', 'xlsx', 'md', 'html', 'code'].includes(asset.kind) && (
                <button onClick={() => onContinue(asset)}>
                  <Pencil size={13} />
                  继续修改
                </button>
              )}
              {isDesktop() && (
                <details>
                  <summary aria-label="更多文件操作" title="更多文件操作">
                    <MoreHorizontal size={17} />
                  </summary>
                  <div>
                    {isDesktop() && (
                      <>
                        <button onClick={() => onAction(() => desktop()!.OpenAsset(asset.id))}>
                          <Files size={14} />
                          {asset.kind === 'code' ? '打开源码目录' : '用系统应用打开'}
                        </button>
                        <button onClick={() => onAction(() => desktop()!.RevealAsset(asset.id))}>
                          <FolderOpen size={14} />
                          打开所在位置
                        </button>
                      </>
                    )}
                  </div>
                </details>
              )}
            </footer>
          )}
        </div>
      </div>
    </aside>
  );
}
