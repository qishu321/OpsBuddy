import { useEffect, useRef, useState } from 'react';
import {
  Code,
  Eye,
  FileCode,
  FolderOpen,
  Play,
  RefreshCw,
  Save,
  Square,
  Terminal,
} from 'lucide-react';
import { api, post, type Asset } from './api';

export interface CodeProject {
  title: string;
  runtime: 'go' | 'node' | 'python';
  mode: 'server' | 'command' | 'fragment';
  entry: string;
  files: { path: string; content: string }[];
}
interface CodeRun {
  status: string;
  url?: string;
  logs: string;
  error?: string;
  exitCode?: number;
}
const labels: Record<string, string> = {
  stopped: '未运行',
  starting: '启动中',
  running: '服务运行中',
  exited: '已退出',
  failed: '运行失败',
};

export default function CodeProjectPreview({
  asset,
  onSaved,
}: {
  asset: Asset;
  onSaved: (next: Asset) => Promise<void>;
}) {
  const [project, setProject] = useState<CodeProject>();
  const [original, setOriginal] = useState('');
  const [file, setFile] = useState('');
  const [tab, setTab] = useState<'preview' | 'source' | 'logs'>('preview');
  const [run, setRun] = useState<CodeRun>({ status: 'stopped', logs: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const [pageError, setPageError] = useState('');
  const iframe = useRef<HTMLIFrameElement>(null);
  useEffect(() => {
    let live = true;
    const controller = new AbortController();
    setProject(undefined);
    setOriginal('');
    setError('');
    setPageError('');
    setTab('preview');
    Promise.all([
      api<CodeProject>(`/assets/${asset.id}/code`, { signal: controller.signal }),
      api<CodeRun>(`/assets/${asset.id}/runtime`, { signal: controller.signal }),
    ])
      .then(async ([p, v]) => {
        if (!live) return;
        setProject(p);
        setOriginal(JSON.stringify(p));
        setFile(p.entry || p.files[0]?.path || '');
        if (p.mode === 'fragment' || asset.requireRunConfirmation) setTab('source');
        if (p.mode === 'command') setTab('logs');
        setRun(v);
        if (p.mode === 'server' && v.status === 'stopped' && !asset.requireRunConfirmation) {
          const next = await post<CodeRun>(`/assets/${asset.id}/runtime/start`);
          if (live) setRun(next);
        }
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [asset.id]);
  useEffect(() => {
    if (!['starting', 'running'].includes(run.status)) return;
    let live = true;
    const timer = setInterval(
      () =>
        api<CodeRun>(`/assets/${asset.id}/runtime`)
          .then((v) => {
            if (live) setRun(v);
          })
          .catch((e) => {
            if (live) setError(e.message);
          }),
      1200,
    );
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [asset.id, run.status]);
  useEffect(() => {
    const receive = (event: MessageEvent) => {
      if (
        event.source !== iframe.current?.contentWindow ||
        event.data?.type !== 'opsbuddy-preview-health'
      )
        return;
      if (event.data.kind === 'error')
        setPageError(String(event.data.message || '页面发生错误').slice(0, 600));
    };
    addEventListener('message', receive);
    return () => removeEventListener('message', receive);
  }, []);

  const dirty = project && JSON.stringify(project) !== original;
  const action = async (name: 'start' | 'stop') => {
    setBusy(true);
    setError('');
    try {
      setPageError('');
      setRun(await post<CodeRun>(`/assets/${asset.id}/runtime/${name}`));
      if (name === 'start') setTab(project?.mode === 'command' ? 'logs' : 'preview');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const save = async (start: boolean) => {
    setBusy(true);
    setError('');
    try {
      const next = await post<Asset>(`/assets/${asset.id}/code`, project);
      if (start) {
        await post(`/assets/${asset.id}/runtime/stop`);
        await post(`/assets/${next.id}/runtime/start`);
      }
      await onSaved(next);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const usesVue = project?.files.some((file) =>
    /__opsbuddy_runtime__\/vue|Vue\.createApp|from ['"]vue/.test(file.content),
  );
  const running = ['starting', 'running'].includes(run.status);
  return (
    <div className="code-project-preview">
      <div className="code-runtime-bar">
        <span className={`code-runtime-state ${run.status}`}>
          <i />
          {run.status === 'running' && pageError
            ? '页面运行异常'
            : labels[run.status] || run.status}
        </span>
        <span>
          {usesVue && 'Vue 3 + '}
          {project?.runtime === 'node'
            ? 'Node.js'
            : project?.runtime === 'go'
              ? 'Go'
              : project?.runtime === 'python'
                ? 'Python'
                : '代码项目'}
        </span>
        <button
          className="button"
          disabled={busy || !project || !!dirty || project.mode === 'fragment'}
          onClick={() => void action(running ? 'stop' : 'start')}
          title="使用本机语言环境执行项目代码"
        >
          {running ? <Square size={13} /> : <Play size={13} />}
          {project?.mode === 'fragment' ? '候选源码，等待集成' : running ? '停止' : '本机运行'}
        </button>
      </div>
      <div className="text-artifact-tabs" role="tablist" aria-label="代码项目视图">
        <button role="tab" aria-selected={tab === 'preview'} onClick={() => setTab('preview')}>
          <Eye size={14} />
          预览
        </button>
        <button role="tab" aria-selected={tab === 'source'} onClick={() => setTab('source')}>
          <Code size={14} />
          项目文件
        </button>
        <button role="tab" aria-selected={tab === 'logs'} onClick={() => setTab('logs')}>
          <Terminal size={14} />
          运行日志
        </button>
        <button
          className="text-artifact-refresh"
          aria-label="刷新项目预览"
          onClick={() => {
            setPageError('');
            setReload((v) => v + 1);
          }}
        >
          <RefreshCw size={14} />
        </button>
      </div>
      {(error || run.error) && (
        <p className="module-error" role="alert">
          {error || run.error}
        </p>
      )}
      {pageError && (
        <div className="code-page-error" role="alert">
          <strong>页面运行失败</strong>
          <span>{pageError}</span>
          <button className="text-button" onClick={() => setTab('source')}>
            查看源码
          </button>
          <button className="text-button" onClick={() => setTab('logs')}>
            查看日志
          </button>
        </div>
      )}
      {tab === 'source' && project ? (
        <div className="code-source-panel">
          <nav className="code-file-tree" aria-label="项目文件树">
            <header>
              <FolderOpen size={14} />
              <strong>{project.title}</strong>
              <small>{project.files.length}</small>
            </header>
            {[...project.files]
              .sort((a, b) => a.path.localeCompare(b.path))
              .map((source) => (
                <button
                  key={source.path}
                  type="button"
                  aria-current={file === source.path ? 'true' : undefined}
                  className={file === source.path ? 'selected' : ''}
                  title={source.path}
                  onClick={() => setFile(source.path)}
                  style={{ paddingLeft: 12 + source.path.split('/').length * 8 }}
                >
                  <FileCode size={14} />
                  <span>{source.path}</span>
                  {source.path === project.entry && <small>入口</small>}
                </button>
              ))}
            {usesVue && <small className="code-local-library">Vue 3 · 本地运行库</small>}
          </nav>
          <div className="code-editor-area">
            <header>
              {file}
              <small>UTF-8</small>
            </header>
            <textarea
              className="text-source-editor"
              aria-label="项目源码"
              spellCheck={false}
              value={project.files.find((f) => f.path === file)?.content || ''}
              onChange={(e) =>
                setProject({
                  ...project,
                  files: project.files.map((f) =>
                    f.path === file ? { ...f, content: e.target.value } : f,
                  ),
                })
              }
            />
          </div>
        </div>
      ) : tab === 'logs' ? (
        <pre className="code-runtime-logs" aria-label="项目运行日志">
          {run.logs || '启动项目后，这里显示程序的实际输出。'}
          {run.exitCode !== undefined ? `\n退出码：${run.exitCode}` : ''}
        </pre>
      ) : run.status === 'running' && run.url ? (
        <iframe
          ref={iframe}
          key={`${run.url}-${reload}`}
          title="项目运行预览"
          src={run.url}
          sandbox="allow-scripts allow-forms"
          referrerPolicy="no-referrer"
        />
      ) : (
        <div className="code-preview-empty">
          <Terminal size={30} />
          <h3>
            {run.status === 'starting'
              ? '正在启动项目…'
              : project?.mode === 'command'
                ? '命令行项目'
                : '运行项目，查看实际效果'}
          </h3>
          <p>
            {run.status === 'starting'
              ? '服务就绪后自动显示页面；编译信息可在运行日志查看。'
              : project?.mode === 'command'
                ? '程序输出与退出码显示在运行日志。'
                : '支持前端与后端组成的服务，也可预览接口返回的 JSON。'}
          </p>
          {run.status === 'failed' && (
            <button className="button" onClick={() => setTab('logs')}>
              查看运行日志
            </button>
          )}
        </div>
      )}
      {dirty && (
        <footer className="code-save-bar">
          <span>有未保存修改</span>
          <button className="button" disabled={busy} onClick={() => void save(false)}>
            <Save size={13} />
            保存新版本
          </button>
          <button
            className="button primary"
            disabled={busy || project?.mode === 'fragment'}
            onClick={() => void save(true)}
          >
            <Play size={13} />
            保存并运行
          </button>
        </footer>
      )}
    </div>
  );
}
