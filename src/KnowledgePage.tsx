import { useRef, useState, type FormEvent } from 'react';
import {
  BookOpen,
  Plus,
  Search,
  FilePlus2,
  FileText,
  RefreshCw,
  Trash2,
  X,
  FolderOpen,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Link2,
} from 'lucide-react';
import {
  api,
  post,
  type State,
  type Asset,
  type KnowledgeBase,
  type KnowledgeDocument,
  type KnowledgeHit,
} from './api';
import { desktop } from './desktop';

export default function KnowledgePage({
  state,
  projectId,
  onRefresh,
}: {
  state: State;
  projectId: string;
  onRefresh: () => Promise<void>;
}) {
  const bases = state.knowledgeBases.filter((b) => b.projectId === projectId);
  const [chosen, setChosen] = useState(''),
    [query, setQuery] = useState(''),
    [hits, setHits] = useState<KnowledgeHit[] | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [creating, setCreating] = useState(false),
    [importing, setImporting] = useState(false),
    [selected, setSelected] = useState<string[]>([]),
    [preview, setPreview] = useState<KnowledgeDocument | null>(null);
  const uploadRef = useRef<HTMLInputElement>(null);
  const base = bases.find((b) => b.id === chosen) || bases[0];
  const docs = state.knowledgeDocuments.filter((d) => d.baseId === base?.id);
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
  const index = async (ids: string[]) => {
    if (!base) return;
    const result = await post<KnowledgeDocument[]>(`/knowledge/${base.id}/documents`, {
      assetIds: ids,
    });
    const good = result.filter((d) => d.status === 'ready').length;
    setNotice(`索引完成：${good} 份可检索，${result.length - good} 份需要处理。`);
    setHits(null);
    setImporting(false);
    setSelected([]);
  };
  const pick = async () => {
    if (!base) return;
    const native = desktop();
    if (!native) {
      uploadRef.current?.click();
      return;
    }
    await perform(async () => {
      const assets = await native.PickFiles(projectId);
      if (assets?.length) await index(assets.map((a) => a.id));
    });
  };
  const upload = async (files: FileList | null) => {
    if (!files?.length) return;
    await perform(async () => {
      const form = new FormData();
      form.append('projectId', projectId);
      Array.from(files).forEach((f) => form.append('files', f));
      const assets = await api<Asset[]>('/assets', { method: 'POST', body: form });
      await index(assets.map((a) => a.id));
    });
  };
  const create = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    perform(async () => {
      const created = await post<KnowledgeBase>('/knowledge', {
        name: form.get('name'),
        description: form.get('description'),
        projectId,
      });
      setChosen(created.id);
      setCreating(false);
      setHits(null);
    });
  };
  const search = (e: FormEvent) => {
    e.preventDefault();
    if (!base || !query.trim()) return;
    perform(async () =>
      setHits(
        await post<KnowledgeHit[]>('/knowledge/search', {
          projectId,
          baseIds: [base.id],
          query,
          limit: 8,
        }),
      ),
    );
  };
  return (
    <div className="knowledge-page">
      <div className="page-title">
        <div>
          <span className="eyebrow">KNOWLEDGE LIBRARY</span>
          <h1>让资料，成为助手的依据。</h1>
          <p>本地文档索引与关键词检索。引用时保留文件、片段和 PDF 页码。</p>
        </div>
        <button className="button primary" onClick={() => setCreating(true)}>
          <Plus size={16} />
          新建知识库
        </button>
      </div>
      {error && (
        <div className="module-error" role="alert">
          <AlertCircle size={16} />
          {error}
          <button onClick={() => setError('')}>
            <X size={14} />
          </button>
        </div>
      )}
      {notice && (
        <div className="module-notice">
          <CheckCircle2 size={16} />
          {notice}
        </div>
      )}
      {!base ? (
        <div className="empty">
          <BookOpen size={36} />
          <h3>先建立一个知识库</h3>
          <p>导入 Word、PDF、PPT、Excel 或文本，再让任务按需检索。</p>
          <button className="button" onClick={() => setCreating(true)}>
            <Plus size={14} />
            创建知识库
          </button>
        </div>
      ) : (
        <div className="knowledge-layout">
          <aside className="library-list">
            {bases.map((b) => (
              <button
                key={b.id}
                className={base.id === b.id ? 'active' : ''}
                onClick={() => {
                  setChosen(b.id);
                  setHits(null);
                  setPreview(null);
                  setNotice('');
                }}
              >
                <BookOpen size={17} />
                <div>
                  <strong>{b.name}</strong>
                  <span>
                    {
                      state.knowledgeDocuments.filter(
                        (d) => d.baseId === b.id && d.status === 'ready',
                      ).length
                    }{' '}
                    份可检索资料
                  </span>
                </div>
              </button>
            ))}
          </aside>
          <section className="library-content">
            <div className="library-heading">
              <div>
                <h2>{base.name}</h2>
                <p>{base.description || '此资料库只在当前项目内可用'}</p>
              </div>
              <div className="library-actions">
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => {
                    setSelected([]);
                    setImporting(true);
                  }}
                >
                  <FolderOpen size={15} />
                  引用项目文件
                </button>
                <button className="button primary" disabled={busy} onClick={pick}>
                  {busy ? <Loader2 size={15} className="spin" /> : <FilePlus2 size={15} />}
                  导入本机资料
                </button>
              </div>
            </div>
            <form className="knowledge-search" onSubmit={search}>
              <Search size={18} />
              <input
                aria-label="检索知识库"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="输入要查找的主题、条款或关键词…"
              />
              <button className="button" disabled={busy || !query.trim()}>
                检索
              </button>
            </form>
            {hits !== null && (
              <div className="knowledge-results">
                <div className="section-heading">
                  <h3>检索结果 · {hits.length}</h3>
                  <button className="text-button" onClick={() => setHits(null)}>
                    返回资料列表
                  </button>
                </div>
                {hits.length === 0 ? (
                  <div className="empty">
                    <h3>没有命中相关片段</h3>
                    <p>尝试缩短关键词，或先导入包含相关内容的资料。</p>
                  </div>
                ) : (
                  hits.map((h) => (
                    <article className="knowledge-hit" key={h.documentId + '-' + h.chunkIndex}>
                      <div>
                        <Link2 size={14} />
                        <strong>{h.name}</strong>
                        <span>
                          片段 {h.chunkIndex + 1}
                          {h.page ? ` · 第 ${h.page} 页` : ''}
                        </span>
                      </div>
                      <p>{h.text}</p>
                      <button
                        className="text-button"
                        onClick={() =>
                          perform(async () => {
                            const d = await api<KnowledgeDocument>(
                              `/knowledge/documents/${h.documentId}`,
                            );
                            setPreview(d);
                          })
                        }
                      >
                        查看来源 <FileText size={12} />
                      </button>
                    </article>
                  ))
                )}
              </div>
            )}
            {hits === null && (
              <>
                <div className="section-heading">
                  <h3>资料 · {docs.length}</h3>
                  <span>600 字符切片 · 本地检索</span>
                </div>
                {docs.length === 0 ? (
                  <div className="empty">
                    <FilePlus2 size={30} />
                    <h3>添加第一份资料</h3>
                    <p>索引保存在本机。扫描 PDF 需先做 OCR，当前不会将图片识别成文字。</p>
                  </div>
                ) : (
                  <div className="knowledge-documents">
                    {docs.map((d) => (
                      <article key={d.id}>
                        <FileText size={21} />
                        <div>
                          <strong>{d.name}</strong>
                          <span className={d.status === 'failed' ? 'document-failed' : ''}>
                            {d.status === 'ready' ? `${d.chunkCount} 个片段 · 已建立索引` : d.error}
                          </span>
                        </div>
                        <button
                          className="button"
                          onClick={() =>
                            perform(async () =>
                              setPreview(
                                await api<KnowledgeDocument>(`/knowledge/documents/${d.id}`),
                              ),
                            )
                          }
                        >
                          查看
                        </button>
                        <button
                          className="icon-button"
                          title="重建索引"
                          disabled={busy}
                          onClick={() => perform(() => index([d.assetId]))}
                        >
                          <RefreshCw size={15} />
                        </button>
                        <button
                          className="icon-button"
                          title="移出索引（保留文件）"
                          disabled={busy}
                          onClick={() =>
                            perform(async () => {
                              await api(`/knowledge/documents/${d.id}`, { method: 'DELETE' });
                              setNotice('已移出知识库索引，原文件与历史引用保留。');
                            })
                          }
                        >
                          <Trash2 size={15} />
                        </button>
                      </article>
                    ))}
                  </div>
                )}
              </>
            )}
            <div className="library-footnote">
              <BookOpen size={14} />
              在任务输入区或工作流知识节点中选择资料库，即可使用检索命中的来源。当前采用关键词匹配，尚未接入向量语义检索。
            </div>
          </section>
        </div>
      )}
      <input hidden type="file" multiple ref={uploadRef} onChange={(e) => upload(e.target.files)} />
      {creating && (
        <div className="module-overlay">
          <form className="module-dialog" onSubmit={create}>
            <div className="module-dialog-title">
              <h2>新建知识库</h2>
              <button type="button" onClick={() => setCreating(false)} aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <label>
              名称
              <input required autoFocus name="name" placeholder="例如：产品资料、工作规范" />
            </label>
            <label>
              说明
              <textarea name="description" rows={3} />
            </label>
            {error && <p className="module-error">{error}</p>}
            <div className="module-dialog-actions">
              <button className="button" type="button" onClick={() => setCreating(false)}>
                取消
              </button>
              <button className="button primary" disabled={busy}>
                创建
              </button>
            </div>
          </form>
        </div>
      )}
      {importing && (
        <div className="module-overlay">
          <div className="module-dialog">
            <div className="module-dialog-title">
              <h2>选择项目资料</h2>
              <button onClick={() => setImporting(false)} aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="asset-chooser">
              {state.assets
                .filter((a) => a.projectId === projectId)
                .map((a) => (
                  <label key={a.id}>
                    <input
                      type="checkbox"
                      checked={selected.includes(a.id)}
                      onChange={(e) =>
                        setSelected((prev) =>
                          e.target.checked ? [...prev, a.id] : prev.filter((x) => x !== a.id),
                        )
                      }
                    />
                    {a.name}
                    <span>{a.kind.toUpperCase()}</span>
                  </label>
                ))}
            </div>
            <div className="module-dialog-actions">
              <button className="button" onClick={() => setImporting(false)}>
                取消
              </button>
              <button
                className="button primary"
                disabled={busy || !selected.length || selected.length > 16}
                onClick={() => perform(() => index(selected))}
              >
                建立索引 · {selected.length}
              </button>
            </div>
          </div>
        </div>
      )}
      {preview && (
        <div className="module-overlay">
          <section className="module-dialog document-dialog">
            <div className="module-dialog-title">
              <h2>{preview.name}</h2>
              <button onClick={() => setPreview(null)} aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <p className="document-meta">
              {preview.chunkCount} 个片段 · 显示前 20 个 · 内容哈希{' '}
              {preview.contentHash?.slice(0, 12) || '无'}
            </p>
            <div className="document-chunks">
              {preview.error && <p className="module-error">{preview.error}</p>}
              {preview.chunks?.map((ch) => (
                <article key={ch.index}>
                  <strong>
                    片段 {ch.index + 1}
                    {ch.page ? ` · PDF 第 ${ch.page} 页` : ''}
                  </strong>
                  <pre>{ch.text}</pre>
                </article>
              ))}
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
