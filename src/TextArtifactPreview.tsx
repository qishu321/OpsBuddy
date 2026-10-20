import { useEffect, useMemo, useState } from 'react';
import { Code, Copy, Eye, RefreshCw, Save } from 'lucide-react';
import { api, post, type Asset } from './api';
import MarkdownContent from './MarkdownContent';

export default function TextArtifactPreview({
  asset,
  onSaved,
}: {
  asset: Asset;
  onSaved: (next: Asset) => Promise<void>;
}) {
  const [source, setSource] = useState(''),
    [original, setOriginal] = useState(''),
    [tab, setTab] = useState<'preview' | 'source'>('preview'),
    [reload, setReload] = useState(0),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(true),
    [saving, setSaving] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    api<{ content: string }>(`/assets/${asset.id}/text`, { signal: controller.signal })
      .then((value) => {
        setSource(value.content);
        setOriginal(value.content);
        setLoading(false);
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setError(e.message);
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [asset.id]);
  const document = useMemo(() => {
    if (asset.kind !== 'html') return '';
    const start = /<html(?:\s|>)/i.exec(source)?.index;
    const end = source.toLowerCase().lastIndexOf('</html>');
    // Also handle old files that contain model commentary around their page.
    const markup = start !== undefined && end > start ? source.slice(start, end + 7) : source;
    const parsed = new DOMParser().parseFromString(markup, 'text/html');
    const policy = parsed.createElement('meta');
    policy.httpEquiv = 'Content-Security-Policy';
    policy.content =
      "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'";
    parsed.head.prepend(policy);
    const charset = parsed.createElement('meta');
    charset.setAttribute('charset', 'utf-8');
    parsed.head.prepend(charset);
    return '<!doctype html>\n' + parsed.documentElement.outerHTML;
  }, [source, asset.kind]);
  const save = async () => {
    setSaving(true);
    setError('');
    try {
      const next = await post<Asset>(`/assets/${asset.id}/versions`, { content: source });
      await onSaved(next);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="text-artifact" data-testid="text-artifact-preview">
      <div className="text-artifact-tabs" role="tablist" aria-label="文件视图">
        <button role="tab" aria-selected={tab === 'preview'} onClick={() => setTab('preview')}>
          <Eye size={14} />
          预览
        </button>
        <button role="tab" aria-selected={tab === 'source'} onClick={() => setTab('source')}>
          <Code size={14} />
          源码编辑
        </button>
        <button
          className="text-artifact-refresh"
          aria-label="刷新文件预览"
          title="刷新预览"
          onClick={() => setReload((n) => n + 1)}
        >
          <RefreshCw size={14} />
        </button>
        <button
          aria-label="复制文件源码"
          title="复制文件源码"
          onClick={() => {
            navigator.clipboard
              .writeText(source)
              .catch(() => setError('复制失败，请在源码编辑中选择并复制。'));
          }}
          disabled={loading}
        >
          <Copy size={14} />
        </button>
      </div>
      {error && (
        <p className="module-error" role="alert">
          {error}
        </p>
      )}
      {loading ? (
        <p className="text-artifact-status">正在读取文件…</p>
      ) : (
        <>
          {tab === 'source' ? (
            <textarea
              className="text-source-editor"
              aria-label="文件源码"
              spellCheck={false}
              value={source}
              onChange={(e) => setSource(e.target.value)}
            />
          ) : asset.kind === 'html' ? (
            <iframe
              key={`${asset.id}-${reload}`}
              title="网页交互预览"
              sandbox="allow-scripts"
              referrerPolicy="no-referrer"
              srcDoc={document}
            />
          ) : (
            <div className="markdown-document">
              <MarkdownContent content={source} />
            </div>
          )}
          {source !== original && (
            <footer>
              <span>
                {source !== original
                  ? '有未保存修改'
                  : asset.kind === 'html'
                    ? '单文件交互预览 · 不加载外部依赖'
                    : 'Markdown · UTF-8'}
              </span>
              <button className="button" disabled={saving || source === original} onClick={save}>
                <Save size={13} />
                {saving ? '保存中…' : '保存新版本'}
              </button>
            </footer>
          )}
        </>
      )}
    </div>
  );
}
