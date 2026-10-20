import {
  useEffect,
  useRef,
  useState,
  lazy,
  Suspense,
  type FormEvent,
  type ReactNode,
  type ClipboardEvent,
} from 'react';
import {
  ArrowUp,
  ArrowUpRight,
  ArrowLeft,
  Plus,
  Search,
  PanelLeftClose,
  PanelLeftOpen,
  LayoutGrid,
  FolderOpen,
  Bot,
  Workflow,
  Puzzle,
  Settings,
  Sparkles,
  FileText,
  Code,
  PanelRight,
  Play,
  MessageSquare,
  Presentation,
  Table2,
  Files,
  ChevronDown,
  ChevronRight,
  Paperclip,
  X,
  Check,
  CheckCircle2,
  Circle,
  Loader2,
  Download,
  Sun,
  Moon,
  ShieldCheck,
  Cable,
  BookOpen,
  Trash2,
  Copy,
  RefreshCw,
  Square,
  Image,
  Film,
  Scissors,
  AlertCircle,
  Archive,
  Pencil,
  Zap,
  CalendarClock,
  Bell,
  SlidersHorizontal,
} from 'lucide-react';
import {
  api,
  post,
  fileURL,
  size,
  kinds,
  statusLabel,
  type State,
  type Task,
  type Asset,
} from './api';
import { desktop, isDesktop } from './desktop';
import { MaterialPicker, ConversationHistory } from './SessionTools';
import { AgentDispatcher, AgentGroupPanel } from './AgentTeam';
import DeleteConversationDialog from './DeleteConversationDialog';
import MarkdownContent from './MarkdownContent';
import ArtifactSidebar from './ArtifactSidebar';
import ProjectSessions from './ProjectSessions';
import DesktopPetSettings from './DesktopPetSettings';
import { version as appVersion } from '../package.json';
const WorkflowPage = lazy(() => import('./WorkflowPage'));
const KnowledgePage = lazy(() => import('./KnowledgePage'));
const MemoryPage = lazy(() => import('./MemoryPage'));
const SchedulePage = lazy(() => import('./SchedulePage'));
const ReminderInbox = lazy(() => import('./ReminderInbox'));

type Page =
  | 'workspace'
  | 'projects'
  | 'workflows'
  | 'knowledge'
  | 'extensions'
  | 'settings'
  | 'schedules'
  | 'memory';
const blank: State = {
  memories: [],
  schedules: [],
  scheduleRuns: [],
  directories: [],
  notifications: [],
  agentGroups: [],
  projects: [],
  roles: [],
  models: [],
  extensions: [],
  assets: [],
  tasks: [],
  knowledgeBases: [],
  knowledgeDocuments: [],
  workflows: [],
  workflowRuns: [],
};
const icons: Record<string, typeof FileText> = {
  chat: MessageSquare,
  docx: FileText,
  pptx: Presentation,
  xlsx: Table2,
  pdf: Files,
  organize: FolderOpen,
  auto: Sparkles,
  html: Code,
  code: Code,
  md: FileText,
  png: Image,
  jpg: Image,
  jpeg: Image,
};
const navs: { id: Page; label: string; icon: typeof FileText }[] = [
  { id: 'workspace', label: '工作台', icon: LayoutGrid },
  { id: 'projects', label: '项目', icon: FolderOpen },
  { id: 'knowledge', label: '知识库', icon: BookOpen },
  { id: 'workflows', label: '工作流', icon: Workflow },
  { id: 'schedules', label: '定时任务', icon: CalendarClock },
  { id: 'extensions', label: '扩展', icon: Puzzle },
];
const navigationGroups = [
  { label: '工作空间', items: navs.slice(0, 3) },
  { label: '工具', items: navs.slice(3) },
];
const templates = [
  {
    kind: 'pptx',
    title: '制作演示文稿',
    desc: '从想法到一份清晰的汇报',
    prompt: '请根据附件制作一份项目汇报 PPT，突出目标、进展和下一步计划。',
  },
  {
    kind: 'docx',
    title: '撰写与编辑文档',
    desc: '让报告、方案和纪要更有条理',
    prompt: '请根据以下内容整理一份会议纪要，包含讨论事项、结论和行动计划。\n\n',
  },
  {
    kind: 'xlsx',
    title: '整理数据表格',
    desc: '合并数据，保留每一处细节',
    prompt: '合并上传的表格，并按完整行去重。',
  },
  {
    kind: 'pdf',
    title: '处理 PDF 文件',
    desc: '提取文字、合并或提取页面',
    prompt: '提取上传 PDF 的文字，并标注页码。',
  },
  {
    kind: 'html',
    title: '制作网页',
    desc: '生成网页，在右侧交互预览',
    prompt: '制作一个单文件 HTML 计算器网页，包含可交互的按钮。',
  },
  {
    kind: 'md',
    title: 'Markdown 文档',
    desc: '整理笔记、说明与项目文档',
    prompt: '将材料整理成 Markdown 文档，包含清晰的标题和要点。',
  },
];
function Mark({ kind = 'docx', small = false }: { kind?: string; small?: boolean }) {
  const Icon = icons[kind] || FileText;
  return (
    <span className={`file-mark ${kind} ${small ? 'small' : ''}`}>
      <Icon size={small ? 17 : 23} />
    </span>
  );
}
function Badge({ status }: { status: string }) {
  return (
    <span className={`status ${status}`}>
      {status === 'running' || status === 'queued' ? (
        <Loader2 size={12} className="spin" />
      ) : (
        <span className="status-dot" />
      )}
      {statusLabel[status] || status}
    </span>
  );
}
function Empty({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof FileText;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Icon size={26} />
      </div>
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}

export default function App() {
  const [nativePlatform, setNativePlatform] = useState('');
  const [state, setState] = useState<State>(blank),
    [page, setPage] = useState<Page>('workspace'),
    [projectId, setProjectId] = useState('inbox'),
    [taskId, setTaskId] = useState(''),
    [assetId, setSelectedAssetId] = useState('');
  const [rightOpen, setRightOpen] = useState(false);
  const setAssetId = (id: string) => {
    setSelectedAssetId(id);
    setRightOpen(Boolean(id));
  };
  const [prompt, setPrompt] = useState(''),
    [kind, setKind] = useState('auto'),
    [modelId, setModelId] = useState(''),
    [attachments, setAttachments] = useState<string[]>([]),
    [parentId, setParentId] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Task | null>(null);
  const [knowledgeIds, setKnowledgeIds] = useState<string[]>([]);
  const [skillIds, setSkillIds] = useState<string[]>([]),
    [sidebarTab, setSidebarTab] = useState<'sessions' | 'files'>('sessions'),
    [dispatchOpen, setDispatchOpen] = useState(false);
  const [search, setSearch] = useState(''),
    [extTab, setExtTab] = useState('skill'),
    [extensionSearch, setExtensionSearch] = useState(''),
    [modal, setModal] = useState(''),
    [editing, setEditing] = useState<any>(null),
    [busy, setBusy] = useState(false),
    [toast, setToast] = useState(''),
    [error, setError] = useState(''),
    [online, setOnline] = useState(false),
    [allowRemote, setAllowRemote] = useState(true),
    [collapsed, setCollapsed] = useState(false),
    [dark, setDark] = useState(localStorage.getItem('opsbuddy-theme') === 'dark');
  const [useMemory, setUseMemory] = useState(true),
    [memorySeed, setMemorySeed] = useState<Task | undefined>(),
    [inboxOpen, setInboxOpen] = useState(false);
  // Invalidates pending UI callbacks even when switching A → B → A.
  const contextEpoch = useRef(0);
  const viewEpoch = contextEpoch.current;
  const clipboardBusy = useRef(false);
  const uploadRef = useRef<HTMLInputElement>(null),
    textarea = useRef<HTMLTextAreaElement>(null),
    bottom = useRef<HTMLDivElement>(null);
  const task = state.tasks.find((t) => t.id === taskId && t.projectId === projectId),
    asset = state.assets.find((a) => a.id === assetId && a.projectId === projectId),
    project = state.projects.find((p) => p.id === projectId);
  const toggleRightSidebar = () => {
    if (page !== 'workspace') setPage('workspace');
    if (!rightOpen && !asset) {
      const conversation = task?.conversationId || task?.id;
      const related = conversation
        ? state.tasks
            .filter((t) => t.projectId === projectId && (t.conversationId || t.id) === conversation)
            .flatMap((t) => [...(t.outputIds || []), ...(t.inputIds || [])])
        : [];
      const recent = state.assets
        .filter((a) => a.projectId === projectId && (!conversation || related.includes(a.id)))
        .slice(-1)[0];
      if (recent) setSelectedAssetId(recent.id);
    }
    setRightOpen((v) => !v);
  };
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.code === 'KeyB') {
        e.preventDefault();
        toggleRightSidebar();
      }
    };
    window.addEventListener('keydown', key);
    return () => window.removeEventListener('keydown', key);
  }, [rightOpen, assetId, taskId, projectId, page, state.assets, state.tasks]);
  const active =
    state.tasks.some((t) => ['queued', 'running', 'waiting'].includes(t.status)) ||
    state.workflowRuns.some((r) => ['queued', 'running'].includes(r.status)) ||
    state.agentGroups.some(
      (g) => g.settling || ['planning', 'running', 'aggregating'].includes(g.status),
    ) ||
    state.schedules.some((p) => p.status === 'active') ||
    state.scheduleRuns.some((r) => ['queued', 'preparing', 'running'].includes(r.status));
  const refresh = async () => {
    try {
      setState(await api<State>('/state'));
      setOnline(true);
    } catch (e) {
      setOnline(false);
      throw e;
    }
  };
  const startupImportOpened = useRef(false);
  useEffect(() => {
    const imported = state.startupImport;
    if (!imported || startupImportOpened.current) return;
    const entry = state.tasks.find(
      (t) => t.id === imported.taskId && t.projectId === imported.projectId,
    );
    if (!entry) return;
    startupImportOpened.current = true;
    openTask(entry);
    setToast('已导入演示项目，可查看协作画布与成果');
  }, [state.startupImport?.taskId, state.tasks]);
  useEffect(() => {
    const native = desktop();
    (native ? native.Ready() : Promise.resolve())
      .then(async () => {
        if (native) setNativePlatform((await native.Info()).platform || 'windows');
        await refresh();
      })
      .then(async () => {
        if (native && (await native.Info()).smokeTest === 'true') {
          const { runDesktopSmoke } = await import('./desktop-smoke');
          await runDesktopSmoke({ refresh, navigate: (next) => setPage(next) });
        }
      })
      .catch((e) => setError(e.message));
  }, []);
  useEffect(() => {
    if (!state.models.length) {
      if (modelId) setModelId('');
      return;
    }
    const saved = localStorage.getItem('opsbuddy-selected-model');
    const selected =
      state.models.find((m) => m.id === modelId) ||
      state.models.find((m) => m.id === saved) ||
      state.models[0];
    if (selected.id !== modelId) setModelId(selected.id);
    localStorage.setItem('opsbuddy-selected-model', selected.id);
  }, [state.models, modelId]);
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => refresh().catch(() => {}), 1500);
    return () => clearInterval(timer);
  }, [active]);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    localStorage.setItem('opsbuddy-theme', dark ? 'dark' : 'light');
  }, [dark]);
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(''), 4500);
    return () => clearTimeout(t);
  }, [toast]);
  useEffect(() => {
    if (task?.outputIds?.length) setAssetId(task.outputIds[0]);
  }, [task?.id, task?.outputIds?.join(',')]);
  useEffect(() => {
    if (
      !task ||
      task.status !== 'succeeded' ||
      task.outputIds?.length ||
      !/<html[\s>]/i.test(task.result || '') ||
      !/<\/html>/i.test(task.result || '')
    )
      return;
    let current = true;
    post<Asset>(`/tasks/${task.id}/html`)
      .then(async (next) => {
        await refresh();
        if (current) setAssetId(next.id);
      })
      .catch((e) => {
        if (current) setError(e.message);
      });
    return () => {
      current = false;
    };
  }, [task?.id, task?.status, task?.result]);
  useEffect(() => {
    if (task) bottom.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }, [task?.steps.length, task?.status]);
  useEffect(() => {
    if (!modal) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) setModal('');
      if (event.key !== 'Tab') return;
      const dialog = document.querySelector('[role="dialog"]');
      const items = Array.from(
        dialog?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)',
        ) || [],
      ).filter((el) => el.offsetParent !== null);
      const first = items[0],
        last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      }
      if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [modal, busy]);
  const notify = async (action: () => Promise<unknown>, success?: string) => {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
      if (success) setToast(success);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    const prevent = (e: DragEvent) => e.preventDefault();
    window.addEventListener('dragover', prevent);
    window.addEventListener('drop', prevent);
    return () => {
      window.removeEventListener('dragover', prevent);
      window.removeEventListener('drop', prevent);
    };
  }, []);
  const newTask = () => {
    contextEpoch.current++;
    setDispatchOpen(false);
    setSkillIds([]);
    setPage('workspace');
    setTaskId('');
    setAssetId('');
    setPrompt('');
    setAttachments([]);
    setParentId('');
    setKnowledgeIds([]);
    setKind('auto');
    setError('');
    setTimeout(() => textarea.current?.focus(), 30);
  };
  const switchProject = (id: string) => {
    setProjectId(id);
    newTask();
  };
  const openTask = (t: Task) => {
    contextEpoch.current++;
    setDispatchOpen(false);
    setError('');
    setKind(t.kind === 'chat' ? 'chat' : 'auto');
    if (state.models.some((m) => m.id === t.modelId)) setModelId(t.modelId);
    setAllowRemote(true);
    setSkillIds([]);
    setKnowledgeIds(t.configSnapshot?.knowledgeIds || []);
    setPage('workspace');
    setProjectId(t.projectId);
    setTaskId(t.id);
    setAssetId(t.outputIds?.[0] || '');
    setAttachments([]);
    setParentId('');
    setPrompt('');
  };
  const chooseTemplate = (type: string, text: string) => {
    newTask();
    setKind(type);
    setPrompt(text);
    setTimeout(() => textarea.current?.focus(), 50);
  };
  const upload = async (files: FileList | File[] | null, epoch = contextEpoch.current) => {
    if (!files?.length) return;
    if (attachments.length + files.length > 16) {
      setError('每轮最多引用 16 个文件，请先移除部分材料');
      return;
    }
    const form = new FormData();
    form.append('projectId', projectId);
    Array.from(files).forEach((f) => form.append('files', f));
    await notify(async () => {
      const list = await api<Asset[]>('/assets', { method: 'POST', body: form });
      if (contextEpoch.current === epoch)
        setAttachments((prev) => [...new Set([...prev, ...list.map((a) => a.id)])]);
    }, '文件已添加');
    if (uploadRef.current) uploadRef.current.value = '';
  };
  const pasteScreenshots = async (event: ClipboardEvent<HTMLFormElement>) => {
    const items = Array.from(event.clipboardData.items);
    const images = items
      .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
      .map((item) => item.getAsFile())
      .filter((file): file is File => !!file);
    if (!images.length)
      images.push(
        ...Array.from(event.clipboardData.files).filter((file) => file.type.startsWith('image/')),
      );
    if (!images.length) return; // Plain text keeps the editor's native paste behavior.
    event.preventDefault();
    if (busy || clipboardBusy.current) {
      setError('正在处理文件，请稍后再粘贴截图');
      return;
    }
    const pastedText = event.clipboardData.getData('text/plain');
    if (pastedText && event.target === textarea.current) {
      const field = textarea.current!;
      const start = field.selectionStart,
        end = field.selectionEnd;
      setPrompt((value) => value.slice(0, start) + pastedText + value.slice(end));
      requestAnimationFrame(() =>
        field.setSelectionRange(start + pastedText.length, start + pastedText.length),
      );
    }
    const existingImages = attachments.filter((id) =>
      state.assets.some((a) => a.id === id && ['png', 'jpg', 'jpeg'].includes(a.kind)),
    ).length;
    if (existingImages + images.length > 4) {
      setError('每轮最多添加 4 张截图或图片，请先移除部分图片');
      return;
    }
    clipboardBusy.current = true;
    const epoch = contextEpoch.current;
    setBusy(true);
    setError('');
    try {
      const files: File[] = [];
      for (const [index, image] of images.entries()) {
        if (image.size > 8 * 1024 * 1024) throw new Error('单张截图最多 8 MB，请缩小截图后重试');
        const bitmap = await createImageBitmap(image);
        try {
          if (bitmap.width * bitmap.height > 20000000)
            throw new Error('截图超过 2000 万像素，请裁剪后重试');
          let blob: Blob = image;
          let ext = image.type === 'image/jpeg' ? 'jpg' : 'png';
          if (!['image/png', 'image/jpeg'].includes(image.type)) {
            const canvas = document.createElement('canvas');
            canvas.width = bitmap.width;
            canvas.height = bitmap.height;
            canvas.getContext('2d')!.drawImage(bitmap, 0, 0);
            blob = await new Promise<Blob>((resolve, reject) =>
              canvas.toBlob(
                (value) =>
                  value ? resolve(value) : reject(new Error('截图转换失败，请保存为 PNG 后添加')),
                'image/png',
              ),
            );
            ext = 'png';
          }
          if (blob.size > 8 * 1024 * 1024) throw new Error('截图转换后超过 8 MB，请裁剪后重试');
          files.push(
            new File([blob], `Clipboard_Screenshot_${Date.now()}_${index + 1}.${ext}`, {
              type: ext === 'jpg' ? 'image/jpeg' : 'image/png',
            }),
          );
        } finally {
          bitmap.close();
        }
      }
      await upload(files, epoch);
    } catch (e) {
      setError((e as Error).message || '无法读取截图，请保存图片后添加');
    } finally {
      clipboardBusy.current = false;
      setBusy(false);
    }
  };
  const pickFiles = async () => {
    const native = desktop();
    if (!native) {
      uploadRef.current?.click();
      return;
    }
    const epoch = contextEpoch.current;
    await notify(async () => {
      const list = await native.PickFiles(projectId);
      if (list?.length) {
        if (contextEpoch.current === epoch)
          setAttachments((prev) => [...new Set([...prev, ...list.map((a) => a.id)])]);
        setToast(`已添加 ${list.length} 个本机文件，原文件保留`);
      }
    });
  };
  const pickFolder = async () => {
    if (!desktop()) {
      setError('文件夹引用仅在桌面版中可用');
      return;
    }
    const epoch = contextEpoch.current;
    await notify(async () => {
      const list = await desktop()!.PickFolder(projectId);
      if (contextEpoch.current === epoch)
        setAttachments((prev) => [...new Set([...prev, ...list.map((a) => a.id)])]);
    });
  };
  const saveAsset = async (asset: Asset) => {
    const native = desktop();
    if (!native) return;
    await notify(async () => {
      const saved = await native.SaveAsset(asset.id);
      if (saved) setToast('已另存成果：' + saved);
    });
  };
  const hasImageAttachments = attachments.some((id) =>
    state.assets.some((a) => a.id === id && ['png', 'jpg', 'jpeg'].includes(a.kind)),
  );
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if ((!prompt.trim() && !hasImageAttachments) || busy || clipboardBusy.current) return;
    if (!modelId || !state.models.some((m) => m.id === modelId)) {
      setPage('settings');
      setError('请先添加并选择模型');
      return;
    }
    const epoch = contextEpoch.current;
    await notify(async () => {
      const t = await post<Task>('/tasks', {
        prompt:
          prompt.trim() ||
          '请查看我提供的截图，说明主要内容；如果需要进一步操作，请先问清我的目标。',
        projectId,
        kind: prompt.trim() ? kind : 'chat',
        roleId: 'auto',
        modelId,
        inputIds: attachments,
        parentId: parentId || (task?.projectId === projectId ? task.id : ''),
        skillIds,
        allowRemote,
        knowledgeIds,
        useMemory,
      });
      if (contextEpoch.current !== epoch) return;
      setTaskId(t.id);
      setAssetId('');
      setPrompt('');
      setAttachments([]);
      setParentId('');
    });
  };
  const continueAsset = (a: Asset) => {
    if (a.projectId !== projectId) return;
    contextEpoch.current++;
    setAttachments([a.id]);
    const source = state.tasks.find((t) => t.id === a.taskId && t.projectId === projectId);
    setTaskId(source?.id || '');
    setPage('workspace');
    setParentId(source?.id || '');
    setKind(a.kind);
    setPrompt('');
    setToast('已引用当前成果，请输入修改要求并选择 AI 模型');
    setTimeout(() => textarea.current?.focus(), 50);
  };
  const openModal = (name: string, value: any = null) => {
    setEditing(value);
    setModal(name);
    setError('');
  };
  const saveForm = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = e.currentTarget;
    const f = new FormData(form);
    const vals = Object.fromEntries(f);
    notify(async () => {
      if (modal === 'project') {
        const p = editing?.id
          ? await api<{ id: string }>(`/projects/${editing.id}`, {
              method: 'PUT',
              body: JSON.stringify(vals),
            })
          : await post<{ id: string }>('/projects', vals);
        if (!editing?.id) switchProject(p.id);
      } else if (modal === 'model') {
        await post('/models', { ...vals, id: editing?.id || '' });
      } else {
        await post('/extensions', {
          ...vals,
          id: editing?.id || '',
          kind: extTab,
          enabled: f.get('enabled') === 'on',
        });
      }
      setModal('');
    }, '已保存');
  };
  const latestConversations = new Map<string, Task>();
  // Newest turn per project/conversation; never merge two projects by conversation ID.
  [...state.tasks]
    .filter((t) => !t.archived && !t.agentMemberId)
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
    .forEach((t) => latestConversations.set(`${t.projectId}:${t.conversationId || t.id}`, t));
  const taskList = [...latestConversations.values()].sort((a, b) =>
    b.createdAt.localeCompare(a.createdAt),
  );
  const projectFiles = state.assets
    .filter(
      (a) =>
        a.projectId === projectId &&
        (!search || a.name.toLowerCase().includes(search.toLowerCase())),
    )
    .slice()
    .reverse();
  const previousTurns: Task[] = [];
  let predecessor = task?.parentId;
  const visited = new Set<string>();
  while (predecessor && !visited.has(predecessor) && previousTurns.length < 20) {
    visited.add(predecessor);
    const previous = state.tasks.find(
      (t) => t.id === predecessor && t.projectId === task?.projectId,
    );
    if (!previous) break;
    previousTurns.unshift(previous);
    predecessor = previous.parentId;
  }
  const agentGroup = state.agentGroups.find(
    (g) => g.id === task?.agentGroupId && g.projectId === projectId,
  );

  return (
    <div className={`app ${collapsed ? 'collapsed' : ''}`}>
      <aside className="sidebar">
        <div className="brand" onClick={newTask}>
          <div className="brand-symbol">
            <span />
            <span />
            <span />
          </div>
          <div className="brand-name">
            OpsBuddy<span>你的 AI 办公伙伴</span>
          </div>
          <button
            className="icon-button collapse-toggle"
            title="收起导航"
            onClick={(e) => {
              e.stopPropagation();
              setCollapsed(true);
            }}
          >
            <PanelLeftClose size={17} />
          </button>
        </div>
        <button className="new-task" aria-label="新建任务" onClick={newTask}>
          <Plus size={18} />
          <span>新建任务</span>
          <kbd>↗</kbd>
        </button>
        <nav aria-label="主导航">
          {navigationGroups.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-label">{group.label}</div>
              {group.items.map(({ id, label, icon: Icon }) => (
                <button
                  key={id}
                  onClick={() => {
                    setPage(id);
                    setError('');
                  }}
                  className={page === id ? 'active' : ''}
                  aria-current={page === id ? 'page' : undefined}
                  title={label}
                >
                  <Icon size={18} />
                  <span>{label}</span>
                  {id === 'workspace' && active && <span className="nav-live" />}
                </button>
              ))}
            </div>
          ))}
        </nav>
        <div className="sidebar-history">
          <div className="sidebar-mode-tabs" role="tablist" aria-label="会话与文件">
            <button
              role="tab"
              aria-selected={sidebarTab === 'sessions'}
              className={sidebarTab === 'sessions' ? 'active' : ''}
              onClick={() => {
                setSidebarTab('sessions');
                setSearch('');
              }}
            >
              会话
            </button>
            <button
              role="tab"
              aria-selected={sidebarTab === 'files'}
              className={sidebarTab === 'files' ? 'active' : ''}
              onClick={() => {
                setSidebarTab('files');
                setSearch('');
              }}
            >
              文件
            </button>
          </div>
          <div className="sidebar-label">
            {sidebarTab === 'sessions' ? '项目与会话' : (project?.name || '项目') + ' · 文件'}{' '}
            <span>{sidebarTab === 'sessions' ? taskList.length : projectFiles.length}</span>
          </div>
          <div className="history-search">
            <Search size={13} />
            <input
              aria-label="搜索任务"
              placeholder={sidebarTab === 'sessions' ? '搜索项目或会话…' : '搜索文件…'}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          {sidebarTab === 'files' ? (
            projectFiles.length ? (
              projectFiles.slice(0, 60).map((a) => (
                <button
                  key={a.id}
                  className="history-item sidebar-file"
                  title={a.name}
                  onClick={() => {
                    setPage('workspace');
                    setAssetId(a.id);
                  }}
                >
                  <Files size={14} />
                  <span>
                    {a.name}
                    <small>
                      {a.taskId ? 'Agent 成果' : '引用材料'} · {size(a.size)}
                    </small>
                  </span>
                </button>
              ))
            ) : (
              <p className="history-empty">当前项目还没有文件。点击输入框 + 添加材料。</p>
            )
          ) : (
            <ProjectSessions
              projects={state.projects}
              tasks={taskList}
              projectId={projectId}
              taskId={page === 'workspace' ? taskId : ''}
              search={search}
              onProject={switchProject}
              onNew={switchProject}
              onOpen={openTask}
              onDelete={setDeleteTarget}
            />
          )}
        </div>
        <div className="sidebar-bottom">
          <button
            className={page === 'settings' || page === 'memory' ? 'active' : ''}
            onClick={() => setPage('settings')}
            title="设置"
          >
            <Settings size={18} />
            <span>设置</span>
          </button>
          <div className="profile">
            <div className="avatar">O</div>
            <div>
              <strong>{isDesktop() ? '桌面工作空间' : '本地开发环境'}</strong>
              <span>
                <i className={online ? 'connected' : ''} />
                {online ? '已就绪' : '正在连接'}
              </span>
            </div>
            <button className="icon-button" onClick={() => setDark(!dark)} aria-label="切换主题">
              {dark ? <Sun size={16} /> : <Moon size={16} />}
            </button>
          </div>
        </div>
      </aside>
      <section className="main">
        <header className="topbar">
          <div className="breadcrumbs">
            {collapsed && (
              <button className="icon-button" onClick={() => setCollapsed(false)} title="展开导航">
                <PanelLeftOpen size={18} />
              </button>
            )}
            <span>{navs.find((n) => n.id === page)?.label || '设置'}</span>
            <ChevronRight size={14} />
            <select
              aria-label="当前项目"
              value={projectId}
              onChange={(e) => {
                switchProject(e.target.value);
              }}
            >
              {state.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
          <div className="topbar-right">
            <button
              className="notification-bell"
              aria-label="打开提醒中心"
              onClick={() => setInboxOpen(true)}
            >
              <Bell size={17} />
              {state.notifications.some((n) => !n.readAt) && (
                <span>{state.notifications.filter((n) => !n.readAt).length}</span>
              )}
            </button>
            <button
              className={`right-sidebar-toggle${rightOpen && page === 'workspace' ? ' active' : ''}`}
              aria-label={rightOpen && page === 'workspace' ? '关闭右侧栏' : '打开右侧栏'}
              aria-expanded={rightOpen && page === 'workspace'}
              aria-keyshortcuts="Control+Shift+B"
              title={`${rightOpen && page === 'workspace' ? '关闭' : '打开'}右侧栏 · Ctrl+Shift+B`}
              onClick={toggleRightSidebar}
            >
              <PanelRight size={18} />
            </button>
          </div>
        </header>
        {error && (
          <div className="error-banner" role="alert">
            <AlertCircle size={17} />
            <span>{error}</span>
            <button onClick={() => setError('')} aria-label="关闭错误">
              <X size={15} />
            </button>
          </div>
        )}
        {!online && (
          <div className="connection-banner">
            {isDesktop() ? '桌面工作空间尚未就绪。' : '开发服务尚未连接。请启动 Go 服务后'}
            <button onClick={() => refresh().catch((e) => setError(e.message))}>重新连接</button>
          </div>
        )}

        {state.notifications.some((n) => !n.readAt) && (
          <div className="assistant-reminder-banner">
            <Bell size={15} />
            <button onClick={() => setInboxOpen(true)}>
              {state.notifications.filter((n) => !n.readAt).slice(-1)[0]?.title} ·
              查看提醒与计划结果
            </button>
            <button
              className="icon-button"
              aria-label="标记最新提醒已读"
              onClick={() =>
                notify(() =>
                  post(
                    `/notifications/${state.notifications.filter((n) => !n.readAt).slice(-1)[0].id}/read`,
                  ),
                )
              }
            >
              <X size={13} />
            </button>
          </div>
        )}
        {page === 'workspace' ? (
          <div
            className={`workspace ${task ? 'with-task' : ''} ${rightOpen ? 'with-artifact' : ''}`}
          >
            <div className="conversation">
              <div className="conversation-scroll">
                {!task ? (
                  <div className="welcome dashboard-welcome">
                    <div className="welcome-kicker">日常办公 · 轻量开发</div>
                    <h1>今天想完成什么？</h1>
                    <p className="welcome-sub">直接描述任务，或选一个常用场景开始。</p>
                    <div className="template-grid">
                      {templates.map((t) => (
                        <button
                          className={`template-card${kind === t.kind ? ' selected' : ''}`}
                          aria-pressed={kind === t.kind}
                          key={t.kind}
                          onClick={() => chooseTemplate(t.kind, t.prompt)}
                        >
                          <Mark kind={t.kind} />
                          <ArrowUpRight className="card-arrow" size={16} />
                          <strong>{t.title}</strong>
                          <span>{t.desc}</span>
                        </button>
                      ))}
                    </div>
                    <div className="workspace-secondary-actions">
                      <button
                        onClick={() =>
                          chooseTemplate(
                            'code',
                            '使用 Go 开发一个含网页界面和 JSON 接口的小工具项目，并在本机启动。',
                          )
                        }
                      >
                        <Code size={15} />
                        代码项目（含后端）
                        <ArrowUpRight size={13} />
                      </button>
                      <button
                        onClick={() => chooseTemplate('organize', '将上传的文件按类型分类打包。')}
                      >
                        <FolderOpen size={15} /> 整理文件 <ArrowUpRight size={13} />
                      </button>
                      <span>截图可直接粘贴，文件可拖入输入框</span>
                    </div>
                    {!state.models.length && (
                      <div className="model-setup-note">
                        <Zap size={18} />
                        <div>
                          <strong>连接你的模型</strong>
                          <p>添加模型后，即可开始对话和处理文件。</p>
                        </div>
                        <button
                          onClick={() => {
                            setPage('settings');
                            openModal('model');
                          }}
                        >
                          添加模型 <ArrowUpRight size={14} />
                        </button>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="task-content">
                    <div className="task-heading">
                      <button className="icon-button" onClick={newTask} title="返回工作台">
                        <ArrowLeft size={18} />
                      </button>
                      <span>
                        {task.configSnapshot?.executor === 'memory-v1'
                          ? '偏好已保存'
                          : state.scheduleRuns.find((run) => run.id === task.scheduleRunId)
                                ?.definition.mode === 'reminder'
                            ? '到时提醒'
                            : kinds[task.kind]}
                        {task.scheduleRunId && ' · 定时会话'}
                      </span>
                      <button
                        className="text-button"
                        onClick={() => {
                          setMemorySeed(task);
                          setPage('memory');
                        }}
                      >
                        <BookOpen size={13} />
                        记为偏好
                      </button>
                      <Badge status={task.status} />
                      <button
                        className="icon-button archive"
                        title="删除会话"
                        aria-label="删除当前会话"
                        onClick={() => setDeleteTarget(task)}
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                    <ConversationHistory
                      turns={previousTurns}
                      state={state}
                      onAsset={(a) => setAssetId(a.id)}
                    />
                    {agentGroup && (
                      <AgentGroupPanel
                        group={agentGroup}
                        state={state}
                        onRefresh={refresh}
                        onTask={openTask}
                      />
                    )}
                    {project?.importKey && (
                      <p className="imported-project-note">
                        这是导入的真实执行记录。历史回执保留；代码尚未在本次桌面启动，可在右侧查看源码并点击“本机运行”。
                      </p>
                    )}
                    <div className="user-message">
                      <div className="message-avatar">我</div>
                      <div>
                        <div className="message-meta">
                          你{' '}
                          <time>
                            {new Date(task.createdAt).toLocaleTimeString('zh-CN', {
                              hour: '2-digit',
                              minute: '2-digit',
                            })}
                          </time>
                        </div>
                        <div className="user-bubble">{task.prompt}</div>
                        <div className="input-files">
                          {task.inputIds
                            ?.map((id) => state.assets.find((a) => a.id === id))
                            .filter((a): a is Asset => !!a)
                            .map((a) => (
                              <a
                                key={a.id}
                                href={fileURL(a)}
                                onClick={(e) => {
                                  if (isDesktop()) {
                                    e.preventDefault();
                                    saveAsset(a);
                                  }
                                }}
                              >
                                <Paperclip size={13} />
                                {a.name}
                              </a>
                            ))}
                        </div>
                      </div>
                    </div>
                    <div className="assistant-message">
                      <div className="assistant-avatar">
                        <Sparkles size={18} />
                      </div>
                      <div className="assistant-body">
                        <div className="message-meta">
                          {task.configSnapshot?.mode === 'model' ? task.modelName : 'OpsBuddy'}{' '}
                          <span>
                            {task.configSnapshot?.mode === 'model'
                              ? task.configSnapshot.model?.modelId
                              : task.modelName}
                          </span>
                        </div>
                        {!!task.usedSkillIds?.length && (
                          <div className="used-skills" aria-label="已读取技能">
                            <BookOpen size={13} />
                            {task.usedSkillIds.map((id) => (
                              <span key={id}>
                                {task.configSnapshot?.extensions.find((x) => x.id === id)?.name ||
                                  state.extensions.find((x) => x.id === id)?.name ||
                                  id}
                              </span>
                            ))}
                          </div>
                        )}
                        <details className="task-config">
                          <summary>
                            本次任务配置{!task.configSnapshot && '（旧任务未记录）'}
                          </summary>
                          {task.configSnapshot ? (
                            <div>
                              <p>
                                {task.configSnapshot.mode === 'model' ? '模型生成' : '本地规则处理'}
                                {' · '}
                                {kinds[task.configSnapshot.outputKind]}
                                {' · '}
                                {new Date(task.configSnapshot.capturedAt).toLocaleString('zh-CN')}
                              </p>
                              {task.configSnapshot.role.instructions && (
                                <p className="config-instructions">
                                  {task.configSnapshot.role.instructions}
                                </p>
                              )}
                              {task.configSnapshot.model && (
                                <p>
                                  所选模型：{task.configSnapshot.model.name}
                                  {' · '}
                                  {task.configSnapshot.model.family}
                                  {' · '}
                                  {task.configSnapshot.model.modelId}
                                  <br />
                                  {task.configSnapshot.model.baseUrl}
                                  {task.configSnapshot.mode === 'local' && '（本任务未调用模型）'}
                                </p>
                              )}
                              <p>
                                发送授权：
                                {task.configSnapshot.allowRemote ? '提交时已确认' : '未授权'}
                                {' · '}执行器：{task.configSnapshot.executor}
                              </p>
                              {task.configSnapshot.projectInstructions && (
                                <details>
                                  <summary>本次项目说明</summary>
                                  <p>{task.configSnapshot.projectInstructions}</p>
                                </details>
                              )}
                              <p>
                                候选技能：{task.configSnapshot.extensions.length} 个
                                {task.configSnapshot.mode === 'local' &&
                                  '（本地模式不注入模型提示）'}
                              </p>
                              {!!task.configSnapshot.memoryRefs?.length && (
                                <details>
                                  <summary>
                                    本次引用记忆 · {task.configSnapshot.memoryRefs.length} 条
                                  </summary>
                                  {task.configSnapshot.memoryRefs.map((m) => (
                                    <div key={m.id}>
                                      <strong>
                                        {m.scope === 'project' ? '项目' : '个人'} · {m.title} · v
                                        {m.version}
                                      </strong>
                                      <p>{m.content}</p>
                                      <small>SHA-256：{m.contentHash}</small>
                                    </div>
                                  ))}
                                </details>
                              )}
                              {task.configSnapshot.extensions.map((extension) => (
                                <details key={extension.id}>
                                  <summary>
                                    {extension.name} · {extension.version || '未声明版本'}
                                    {task.configSnapshot?.lazySkills
                                      ? task.usedSkillIds?.includes(extension.id)
                                        ? ' · 已读取'
                                        : ' · 未读取'
                                      : ''}
                                  </summary>
                                  <p>内容 SHA-256：{extension.contentHash}</p>
                                  <pre>{extension.content}</pre>
                                </details>
                              ))}
                              <p>提交后配置保持不变；不包含密钥。重新填写任务将使用届时配置。</p>
                            </div>
                          ) : (
                            <p>此任务创建于配置快照功能上线前，无法还原当时配置。</p>
                          )}
                        </details>
                        {task.status !== 'succeeded' && (
                          <h3>
                            {task.status === 'failed'
                              ? '这个步骤需要处理'
                              : task.status === 'cancelled'
                                ? '任务已停止'
                                : task.status === 'interrupted'
                                  ? '任务已中断'
                                  : '正在处理你的任务'}
                          </h3>
                        )}
                        <details className="tool-process">
                          <summary>工具过程 · {task.steps.length} 条记录</summary>
                          <div className="steps">
                            {task.steps
                              .filter(
                                (s, i, all) =>
                                  s.status !== 'running' ||
                                  !all
                                    .slice(i + 1)
                                    .some((v) => v.name === s.name && v.status === 'completed'),
                              )
                              .map((s, i) => (
                                <div className="step" key={i}>
                                  {s.status === 'completed' ? (
                                    <CheckCircle2 size={16} />
                                  ) : ['running', 'queued'].includes(task.status) ? (
                                    <Loader2 size={16} className="spin" />
                                  ) : (
                                    <Circle size={16} />
                                  )}
                                  <div>
                                    <strong>{s.name}</strong>
                                    <span>{s.detail}</span>
                                  </div>
                                </div>
                              ))}
                          </div>
                        </details>
                        {!task.outputIds?.length &&
                          /<html[\s>]/i.test(task.result || '') &&
                          /<\/html>/i.test(task.result || '') && (
                            <button
                              className="button primary"
                              onClick={() =>
                                notify(async () => {
                                  const next = await post<Asset>(`/tasks/${task.id}/html`);
                                  await refresh();
                                  setAssetId(next.id);
                                }, '已打开网页预览')
                              }
                            >
                              <Play size={14} />
                              运行 / 预览网页
                            </button>
                          )}
                        {task.result &&
                        task.kind === 'chat' &&
                        task.outputIds?.some((id) =>
                          state.assets.some((a) => a.id === id && a.kind === 'html'),
                        ) &&
                        /<html[\s>]/i.test(task.result) ? (
                          <div className="recovered-web-result">
                            <p>网页已提取为文件，可在右侧直接交互预览。</p>
                            <details>
                              <summary>查看原始回复</summary>
                              <MarkdownContent className="result-text" content={task.result} />
                            </details>
                          </div>
                        ) : (
                          task.result && (
                            <MarkdownContent className="result-text" content={task.result} />
                          )
                        )}
                        {!!task.configSnapshot?.knowledgeRefs?.length && (
                          <div className="task-knowledge-citations">
                            <strong>
                              <BookOpen size={13} />
                              本次引用的知识来源
                            </strong>
                            {task.configSnapshot.knowledgeRefs.map((h, i) => (
                              <p key={i} title={h.text}>
                                {h.name} · 片段 {h.chunkIndex + 1}
                                {h.page ? ` · PDF 第 ${h.page} 页` : ''}
                              </p>
                            ))}
                            <button className="text-button" onClick={() => setPage('knowledge')}>
                              查看知识库
                            </button>
                          </div>
                        )}
                        {task.error && (
                          <p className="task-error">
                            <AlertCircle size={16} />
                            {task.error}
                          </p>
                        )}
                        <div className="output-files">
                          {task.outputIds
                            ?.map((id) => state.assets.find((a) => a.id === id))
                            .filter((a): a is Asset => !!a)
                            .map((a) => (
                              <button
                                className="output-card"
                                key={a.id}
                                onClick={() => setAssetId(a.id)}
                              >
                                <Mark kind={a.kind} />
                                <div>
                                  <strong>{a.name}</strong>
                                  <span>
                                    {a.kind === 'code'
                                      ? '源码项目 · 文件与运行预览'
                                      : `${a.kind.toUpperCase()} · ${size(a.size)} · 点击预览`}
                                  </span>
                                </div>
                                <ChevronRight size={17} />
                              </button>
                            ))}
                        </div>
                        {['failed', 'cancelled', 'interrupted'].includes(task.status) && (
                          <button
                            className="button subtle"
                            onClick={() => {
                              setPrompt(task.prompt);
                              setAttachments(task.inputIds || []);
                              setKind(task.kind);
                              if (state.models.some((m) => m.id === task.modelId))
                                setModelId(task.modelId);
                              setParentId('');
                              textarea.current?.focus();
                            }}
                          >
                            <RefreshCw size={14} /> 加载原需求重试
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                )}
                <div ref={bottom} />
              </div>
              <div className="composer-wrap">
                <form
                  className="composer compact-composer"
                  onSubmit={submit}
                  onPaste={pasteScreenshots}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault();
                    upload(e.dataTransfer.files);
                  }}
                >
                  {!!skillIds.length && (
                    <div className="attached-skills">
                      {skillIds.map((id) => {
                        const x = state.extensions.find((e) => e.id === id);
                        return (
                          x && (
                            <span key={id}>
                              <Puzzle size={12} />
                              {x.name}
                              <button
                                type="button"
                                aria-label={`移除技能 ${x.name}`}
                                onClick={() => setSkillIds((ids) => ids.filter((s) => s !== id))}
                              >
                                <X size={12} />
                              </button>
                            </span>
                          )
                        );
                      })}
                    </div>
                  )}
                  {parentId && (
                    <div className="followup-tag">
                      <Pencil size={13} /> 基于成果创建新版本
                      <button
                        type="button"
                        onClick={() => {
                          setParentId('');
                          setAttachments([]);
                        }}
                      >
                        <X size={13} />
                      </button>
                    </div>
                  )}
                  {attachments.length > 0 && (
                    <div className="attached-files">
                      {attachments
                        .map((id) => state.assets.find((a) => a.id === id))
                        .filter((a): a is Asset => !!a)
                        .map((a) => (
                          <span
                            key={a.id}
                            className={
                              ['png', 'jpg', 'jpeg'].includes(a.kind)
                                ? 'image-attachment'
                                : undefined
                            }
                          >
                            {['png', 'jpg', 'jpeg'].includes(a.kind) ? (
                              <button
                                type="button"
                                className="attachment-image"
                                aria-label={`预览图片 ${a.name}`}
                                title="点击预览截图"
                                onClick={() => setAssetId(a.id)}
                              >
                                <img src={fileURL(a, true)} alt={a.name} />
                              </button>
                            ) : (
                              <Mark kind={a.kind} small />
                            )}
                            <span className="attachment-name" title={a.name}>
                              {a.name}
                            </span>
                            <button
                              type="button"
                              onClick={() => setAttachments((v) => v.filter((x) => x !== a.id))}
                              aria-label={`移除 ${a.name}`}
                            >
                              <X size={13} />
                            </button>
                          </span>
                        ))}
                    </div>
                  )}
                  {!parentId && !['auto', 'chat'].includes(kind) && (
                    <div className="composer-format">
                      <span>输出为 {kinds[kind] || kind}</span>
                      <button
                        type="button"
                        aria-label="恢复自动选择格式"
                        title="恢复自动选择格式"
                        onClick={() => {
                          setKind('auto');
                        }}
                      >
                        <X size={13} />
                      </button>
                    </div>
                  )}
                  <textarea
                    ref={textarea}
                    value={prompt}
                    onChange={(e) => setPrompt(e.target.value)}
                    placeholder={
                      parentId
                        ? '告诉我这份成果需要怎样修改…'
                        : '今天帮你做些什么？可粘贴截图、拖入图片或添加文件'
                    }
                    aria-label="任务需求"
                    rows={3}
                    onKeyDown={(e) => {
                      if (
                        e.key === 'Enter' &&
                        !e.shiftKey &&
                        !e.nativeEvent.isComposing &&
                        e.nativeEvent.keyCode !== 229
                      ) {
                        e.preventDefault();
                        const form = e.currentTarget.form;
                        const send =
                          form?.querySelector<HTMLButtonElement>('button[type="submit"]');
                        if (!e.repeat && send && !send.disabled) form?.requestSubmit(send);
                      }
                    }}
                  />
                  <div className="composer-tools">
                    <div className="composer-toolbar-left">
                      <MaterialPicker
                        key={projectId}
                        state={state}
                        projectId={projectId}
                        assetIds={attachments}
                        skillIds={skillIds}
                        onAssets={setAttachments}
                        onSkills={setSkillIds}
                        onFiles={pickFiles}
                        onFolder={pickFolder}
                        disabled={busy}
                      />
                      <details
                        className="composer-menu permission-menu"
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') e.currentTarget.open = false;
                        }}
                      >
                        <summary aria-label="任务权限">
                          <ShieldCheck size={17} />
                          <span>{allowRemote ? '默认权限' : '暂停发送'}</span>
                          <ChevronDown size={12} />
                        </summary>
                        <div className="composer-popover permission-popover">
                          <strong>任务权限</strong>
                          <label>
                            <input
                              type="checkbox"
                              checked={allowRemote}
                              onChange={(e) => setAllowRemote(e.target.checked)}
                            />
                            <span>
                              允许所选模型处理任务<small>发送本次要求及明确引用的材料。</small>
                            </span>
                          </label>
                          <p>仅处理你添加的材料，原始文件保留。</p>
                        </div>
                      </details>
                    </div>
                    <div className="composer-toolbar-right">
                      <details
                        className="composer-menu advanced-menu"
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') e.currentTarget.open = false;
                        }}
                      >
                        <summary aria-label="更多任务选项" title="输出格式、知识库与子任务">
                          <SlidersHorizontal size={17} />
                        </summary>
                        <div className="composer-popover advanced-popover">
                          <strong>任务选项</strong>
                          <div className="advanced-selects">
                            <select
                              aria-label="任务类型"
                              value={kind}
                              onChange={(e) => setKind(e.target.value)}
                            >
                              {Object.entries(kinds).map(([k, v]) => (
                                <option key={k} value={k}>
                                  {v}
                                </option>
                              ))}
                            </select>
                          </div>
                          {state.knowledgeBases.some((b) => b.projectId === projectId) && (
                            <details className="task-knowledge-picker">
                              <summary>
                                <BookOpen size={14} />
                                引用知识库{' '}
                                <span>
                                  {knowledgeIds.length
                                    ? `已选 ${knowledgeIds.length} 个`
                                    : '按需选择资料来源'}
                                </span>
                              </summary>
                              <div>
                                {state.knowledgeBases
                                  .filter((b) => b.projectId === projectId)
                                  .map((b) => (
                                    <label key={b.id}>
                                      <input
                                        type="checkbox"
                                        checked={knowledgeIds.includes(b.id)}
                                        onChange={(e) =>
                                          setKnowledgeIds((prev) =>
                                            e.target.checked
                                              ? [...prev, b.id]
                                              : prev.filter((id) => id !== b.id),
                                          )
                                        }
                                      />
                                      {b.name}
                                    </label>
                                  ))}
                              </div>
                            </details>
                          )}
                          <div className="team-dispatch-action">
                            <button
                              type="button"
                              disabled={
                                busy ||
                                (!!task && ['running', 'queued', 'waiting'].includes(task.status))
                              }
                              onClick={(e) => {
                                const panel = e.currentTarget.closest('details');
                                if (panel) panel.open = false;
                                setDispatchOpen(true);
                              }}
                            >
                              <Workflow size={14} />
                              派发子任务
                              <ArrowUpRight size={12} />
                            </button>
                            <span>独立上下文 · 并行处理 · 统一汇总</span>
                          </div>
                          {state.memories.some(
                            (m) =>
                              m.enabled && (m.scope === 'personal' || m.projectId === projectId),
                          ) && (
                            <label className="composer-memory-choice">
                              <input
                                type="checkbox"
                                checked={useMemory}
                                onChange={(e) => setUseMemory(e.target.checked)}
                              />
                              <BookOpen size={12} />
                              按需使用已保存的偏好和项目经验
                            </label>
                          )}
                        </div>
                      </details>
                      <label className="inline-model-picker" title="选择模型">
                        <Bot size={18} />
                        {state.models.length ? (
                          <select
                            aria-label="模型"
                            value={modelId}
                            onChange={(e) => {
                              setModelId(e.target.value);
                              setAllowRemote(true);
                            }}
                          >
                            {state.models.map((m) => (
                              <option key={m.id} value={m.id}>
                                {m.modelId || m.name}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <button
                            type="button"
                            className="add-model-shortcut"
                            onClick={() => {
                              setPage('settings');
                              openModal('model');
                            }}
                          >
                            添加模型
                          </button>
                        )}
                        {!!state.models.length && <ChevronDown size={12} />}
                      </label>
                      {task && ['running', 'queued', 'waiting'].includes(task.status) ? (
                        <button
                          className="send-button stopping"
                          type="button"
                          onClick={() =>
                            notify(() => post(`/tasks/${task.id}/cancel`), '已请求停止')
                          }
                          title="停止任务"
                        >
                          <Square size={16} />
                        </button>
                      ) : (
                        <button
                          className="send-button"
                          type="submit"
                          disabled={
                            (!prompt.trim() && !hasImageAttachments) ||
                            busy ||
                            !online ||
                            !modelId ||
                            !allowRemote
                          }
                          aria-label="开始任务"
                        >
                          {busy ? <Loader2 size={20} className="spin" /> : <ArrowUp size={20} />}
                        </button>
                      )}
                    </div>
                  </div>
                </form>
                <div className="composer-foot">
                  <span className="composer-project" title="会话、文件和项目记忆按此项目使用">
                    <FolderOpen size={12} />
                    {project?.name || '当前项目'}
                  </span>
                  <span className="send-shortcut">
                    <kbd>Enter</kbd> 发送 · <kbd>Shift</kbd> + <kbd>Enter</kbd> 换行
                  </span>
                </div>
              </div>
            </div>
            {rightOpen && (
              <ArtifactSidebar
                asset={asset}
                files={state.assets
                  .filter((a) => a.projectId === projectId)
                  .slice()
                  .reverse()}
                hasConversation={!!task}
                conversationIDs={
                  new Set(
                    state.tasks
                      .filter(
                        (t) =>
                          task &&
                          t.projectId === projectId &&
                          (t.conversationId || t.id) === (task.conversationId || task.id),
                      )
                      .flatMap((t) => [
                        ...(t.outputIds || []),
                        ...(t.inputIds || []),
                        ...state.assets.filter((a) => a.taskId === t.id).map((a) => a.id),
                      ]),
                  )
                }
                onSelect={setAssetId}
                onClose={() => setRightOpen(false)}
                onSaved={async (next) => {
                  await refresh();
                  if (contextEpoch.current === viewEpoch) setAssetId(next.id);
                  setToast('已保存新版本，原文件保留');
                }}
                onSave={saveAsset}
                onContinue={continueAsset}
                onAction={(action) => {
                  void notify(action);
                }}
              />
            )}
          </div>
        ) : (
          <div className="page-scroll">
            <div className="page-content">
              {page === 'schedules' && (
                <Suspense fallback={<div className="module-loading">正在加载定时任务…</div>}>
                  <SchedulePage
                    state={state}
                    projectId={projectId}
                    onRefresh={refresh}
                    onTask={openTask}
                  />
                </Suspense>
              )}
              {page === 'memory' && (
                <Suspense fallback={<div className="module-loading">正在加载个人记忆…</div>}>
                  <MemoryPage
                    state={state}
                    projectId={projectId}
                    onRefresh={refresh}
                    onTask={openTask}
                    onBack={() => setPage('settings')}
                    seed={memorySeed}
                    onClearSeed={() => setMemorySeed(undefined)}
                  />
                </Suspense>
              )}
              {page === 'projects' && (
                <>
                  <PageTitle
                    eyebrow="PROJECTS"
                    title="让工作，各就其位。"
                    description="把同一件事的材料、任务与成果放在一起。"
                    action={
                      <div className="project-page-actions">
                        {isDesktop() && (
                          <button
                            className="button"
                            disabled={busy}
                            onClick={async () => {
                              setBusy(true);
                              setError('');
                              try {
                                const result = await desktop()!.ImportProject();
                                if (!result.projectId) return;
                                const next = await api<State>('/state');
                                setState(next);
                                const entry = next.tasks.find((t) => t.id === result.taskId);
                                if (entry) openTask(entry);
                                else switchProject(result.projectId);
                                setToast(
                                  result.alreadyImported
                                    ? '该项目已导入，已打开原记录'
                                    : '已导入项目、会话和协作成果',
                                );
                              } catch (e) {
                                setError((e as Error).message);
                              } finally {
                                setBusy(false);
                              }
                            }}
                          >
                            <Download size={16} />
                            导入项目
                          </button>
                        )}
                        <button className="button primary" onClick={() => openModal('project')}>
                          <Plus size={16} /> 新建项目
                        </button>
                      </div>
                    }
                  />
                  <div className="project-grid">
                    {state.projects.map((p) => {
                      const tasks = state.tasks.filter((t) => t.projectId === p.id);
                      return (
                        <article
                          key={p.id}
                          className={`project-card ${projectId === p.id ? 'current' : ''}`}
                        >
                          <button
                            className="project-open"
                            onClick={() => {
                              switchProject(p.id);
                            }}
                          >
                            <div className="project-top">
                              <div className="folder-art">
                                <FolderOpen size={27} />
                              </div>
                              {projectId === p.id ? (
                                <span className="small-pill">当前项目</span>
                              ) : (
                                <ArrowUpRight size={17} />
                              )}
                            </div>
                            <h3>{p.name}</h3>
                            {p.importKey && (
                              <span className="small-pill">已导入 · 含真实执行记录</span>
                            )}
                            <p>{p.description || '为新的工作留一个空间'}</p>
                            <div className="project-count">
                              <span>{tasks.length} 个任务</span>
                              <span>
                                {
                                  state.assets.filter((a) => a.projectId === p.id && a.taskId)
                                    .length
                                }{' '}
                                份成果
                              </span>
                            </div>
                          </button>
                          <button
                            className="icon-button project-edit"
                            aria-label={`编辑项目 ${p.name}`}
                            title="项目说明与偏好"
                            onClick={() => openModal('project', p)}
                          >
                            <Pencil size={15} />
                          </button>
                        </article>
                      );
                    })}
                  </div>
                  <div className="section-heading">
                    <h3>项目中的任务</h3>
                    <span>{project?.name}</span>
                  </div>
                  {taskList.filter((t) => t.projectId === projectId).length ? (
                    <div className="task-table">
                      {taskList
                        .filter((t) => t.projectId === projectId)
                        .map((t) => (
                          <button key={t.id} onClick={() => openTask(t)}>
                            <Mark kind={t.kind} small />
                            <strong>{t.title}</strong>
                            <Badge status={t.status} />
                            <time>{new Date(t.createdAt).toLocaleDateString('zh-CN')}</time>
                            <ChevronRight size={15} />
                          </button>
                        ))}
                    </div>
                  ) : (
                    <Empty icon={FolderOpen} title="项目已就绪">
                      在工作台创建一个任务，材料与成果会自动归入当前项目。
                    </Empty>
                  )}
                </>
              )}
              {page === 'workflows' && (
                <Suspense fallback={<div className="module-loading">正在加载编排画布…</div>}>
                  <WorkflowPage
                    state={state}
                    projectId={projectId}
                    onRefresh={refresh}
                    onTask={(id) => {
                      const t = state.tasks.find((t) => t.id === id);
                      if (t) openTask(t);
                    }}
                  />
                </Suspense>
              )}
              {page === 'knowledge' && (
                <Suspense fallback={<div className="module-loading">正在加载知识库…</div>}>
                  <KnowledgePage state={state} projectId={projectId} onRefresh={refresh} />
                </Suspense>
              )}
              {page === 'extensions' && (
                <>
                  <PageTitle
                    eyebrow="EXTENSIONS"
                    title="给助手，添一点本领。"
                    description="按需添加技能、连接工具，保留简单的工作方式。"
                    action={
                      <button className="button primary" onClick={() => openModal('extension')}>
                        <Plus size={16} />
                        {extTab === 'skill'
                          ? '添加 Skill'
                          : extTab === 'mcp'
                            ? '添加连接'
                            : '安装技能包'}
                      </button>
                    }
                  />
                  <div className="tabs">
                    {[
                      { id: 'skill', name: 'Skills', icon: BookOpen },
                      { id: 'mcp', name: 'MCP 连接', icon: Cable },
                      { id: 'plugin', name: '插件', icon: Puzzle },
                    ].map(({ id, name, icon: Icon }) => (
                      <button
                        className={extTab === id ? 'active' : ''}
                        key={id}
                        onClick={() => setExtTab(id)}
                      >
                        <Icon size={16} />
                        {name}
                        <span>{state.extensions.filter((x) => x.kind === id).length}</span>
                      </button>
                    ))}
                  </div>
                  <div className="info-strip">
                    <ShieldCheck size={16} />
                    {extTab === 'skill'
                      ? '内置技能按任务类型与关键词选用，手动引用优先。可停用、查看说明或复制修改，不执行技能脚本。'
                      : extTab === 'mcp'
                        ? '支持远程 Streamable HTTP 工具发现。认证、本地 stdio 与实际工具执行尚未接入。'
                        : '支持声明式技能包清单；不安装或执行第三方程序。'}
                  </div>
                  {extTab === 'skill' && (
                    <label className="skill-search">
                      <Search size={15} />
                      <input
                        aria-label="搜索技能"
                        value={extensionSearch}
                        onChange={(e) => setExtensionSearch(e.target.value)}
                        placeholder="搜索办公、开发或技能名称…"
                      />
                      <span>
                        {state.extensions.filter((x) => x.kind === 'skill' && x.builtin).length}{' '}
                        个内置
                      </span>
                    </label>
                  )}
                  {state.extensions.filter((x) => x.kind === extTab).length ? (
                    <div className="extension-list">
                      {state.extensions
                        .filter(
                          (x) =>
                            x.kind === extTab &&
                            (extTab !== 'skill' ||
                              !extensionSearch ||
                              `${x.name} ${x.description} ${x.category || ''}`
                                .toLowerCase()
                                .includes(extensionSearch.toLowerCase())),
                        )
                        .map((x) => (
                          <article key={x.id} data-skill-id={x.kind === 'skill' ? x.id : undefined}>
                            <div className="extension-symbol">
                              {extTab === 'skill' ? (
                                <BookOpen size={21} />
                              ) : extTab === 'mcp' ? (
                                <Cable size={21} />
                              ) : (
                                <Puzzle size={21} />
                              )}
                            </div>
                            <div className="extension-detail">
                              <h3>
                                {x.name}
                                <span>{x.version || '本地'}</span>
                                {x.builtin && (
                                  <span className="builtin-skill-badge">内置 · {x.category}</span>
                                )}
                                {x.sourceRoleId && (
                                  <span className="builtin-skill-badge">已迁移</span>
                                )}
                              </h3>
                              <p>{x.description || x.url || '自定义技能说明'}</p>
                              <small>
                                {x.enabled ? x.status : '已停用'}
                                {x.sourceRoleId &&
                                  ` · 关联 ${x.dependencies?.length || 0} 项技能 / ${x.knowledgeIds?.length || 0} 个知识库`}
                              </small>
                              {x.kind === 'skill' && (
                                <details className="skill-instructions">
                                  <summary>查看 SKILL.md</summary>
                                  <MarkdownContent
                                    content={(x.content || '').replace(
                                      /^---\r?\n[\s\S]*?\r?\n---\r?\n/,
                                      '',
                                    )}
                                  />
                                </details>
                              )}
                            </div>
                            <button
                              className={`toggle ${x.enabled ? 'on' : ''}`}
                              role="switch"
                              aria-checked={x.enabled}
                              aria-label={`启用 ${x.name}`}
                              onClick={() =>
                                notify(() => post('/extensions', { ...x, enabled: !x.enabled }))
                              }
                            >
                              <span />
                            </button>
                            {extTab === 'mcp' && (
                              <button
                                className="button"
                                disabled={busy}
                                onClick={() =>
                                  notify(async () => {
                                    const v = await post<{ message: string }>(
                                      `/extensions/${x.id}/test`,
                                    );
                                    setToast(v.message);
                                  })
                                }
                              >
                                测试连接
                              </button>
                            )}
                            <button
                              className="icon-button"
                              title={x.builtin ? '复制为自定义技能' : '编辑'}
                              onClick={() =>
                                openModal(
                                  'extension',
                                  x.builtin
                                    ? { ...x, id: '', builtin: false, name: `${x.name} 副本` }
                                    : x,
                                )
                              }
                            >
                              {x.builtin ? <Copy size={16} /> : <Pencil size={16} />}
                            </button>
                            {!x.builtin && (
                              <button
                                className="icon-button"
                                title="删除扩展"
                                onClick={() =>
                                  notify(
                                    () => api(`/extensions/${x.id}`, { method: 'DELETE' }),
                                    '已删除扩展',
                                  )
                                }
                              >
                                <Trash2 size={16} />
                              </button>
                            )}
                          </article>
                        ))}
                    </div>
                  ) : (
                    <Empty
                      icon={extTab === 'skill' ? BookOpen : extTab === 'mcp' ? Cable : Puzzle}
                      title={
                        extTab === 'skill'
                          ? '把熟悉的方法，变成可复用的技能'
                          : extTab === 'mcp'
                            ? '连接你的工具服务'
                            : '从一个轻量技能包开始'
                      }
                    >
                      点击右上角添加。所有扩展均可停用，不影响已经生成的文件。
                    </Empty>
                  )}
                </>
              )}
              {page === 'settings' && (
                <>
                  <PageTitle
                    eyebrow="PREFERENCES"
                    title="你的工具，你来选择。"
                    description="使用自己的模型服务。任务内容只会发送到你明确选择的提供方。"
                  />
                  <div className="settings-memory-entry">
                    <BookOpen size={22} />
                    <div>
                      <strong>偏好与项目记忆</strong>
                      <p>查看、编辑或停用已保存的内容，决定 Agent 日后如何使用。</p>
                    </div>
                    <button className="button" onClick={() => setPage('memory')}>
                      管理记忆
                      <ChevronRight size={14} />
                    </button>
                  </div>
                  <div className="settings-section">
                    <div className="section-heading">
                      <div>
                        <h3>模型连接</h3>
                        <p>OpenAI-compatible · Anthropic Messages · Gemini GenerateContent</p>
                      </div>
                      <button className="button primary" onClick={() => openModal('model')}>
                        <Plus size={15} /> 添加模型
                      </button>
                    </div>
                    {state.models.length ? (
                      <div className="model-list">
                        {state.models.map((m) => (
                          <article className="model-card" key={m.id}>
                            <div className="model-logo">
                              <Sparkles size={22} />
                            </div>
                            <div>
                              <h3>
                                {m.name}
                                <span className="small-pill">{m.family}</span>
                              </h3>
                              <p>{m.modelId}</p>
                              <small>{m.baseUrl}</small>
                              <span className="key-status">
                                <ShieldCheck size={12} />
                                {m.hasKey
                                  ? '密钥已在本机加密保存'
                                  : '未配置密钥 · 适用于无需认证的本地服务'}
                                {m.testedAt ? ' · 文本测试已通过' : ''}
                              </span>
                            </div>
                            <div className="model-actions">
                              <button
                                className="button"
                                disabled={busy}
                                onClick={() =>
                                  notify(async () => {
                                    const result = await post<{ message: string }>(
                                      `/models/${m.id}/test`,
                                    );
                                    setToast(result.message);
                                  })
                                }
                              >
                                {busy ? <Loader2 className="spin" size={14} /> : <Zap size={14} />}
                                测试
                              </button>
                              <button
                                className="icon-button"
                                title="编辑模型"
                                onClick={() => openModal('model', m)}
                              >
                                <Pencil size={16} />
                              </button>
                              <button
                                className="icon-button"
                                title="删除模型"
                                onClick={() =>
                                  notify(
                                    () => api(`/models/${m.id}`, { method: 'DELETE' }),
                                    '模型配置已删除',
                                  )
                                }
                              >
                                <Trash2 size={16} />
                              </button>
                            </div>
                          </article>
                        ))}
                      </div>
                    ) : (
                      <div className="model-empty">
                        <div>
                          <Sparkles size={24} />
                          <h3>连接一个模型，开启智能办公</h3>
                          <p>
                            本地模式可以整理材料和生成基础文件。
                            <br />
                            接入模型后，可以根据要求撰写内容、分析表格和修改成果。
                          </p>
                        </div>
                        <button className="button" onClick={() => openModal('model')}>
                          配置第一个模型 <ArrowUpRight size={15} />
                        </button>
                      </div>
                    )}
                  </div>
                  <div className="settings-section">
                    <div className="section-heading">
                      <h3>工作空间</h3>
                    </div>
                    {isDesktop() && nativePlatform !== 'linux' && <DesktopPetSettings />}
                    {nativePlatform === 'linux' && (
                      <div className="setting-row">
                        <div>
                          <strong>桌面小搭子</strong>
                          <p>原生桌宠暂仅支持 Windows；Linux 不创建桌宠进程或定时器。</p>
                        </div>
                      </div>
                    )}
                    <div className="setting-row">
                      <div>
                        <strong>外观</strong>
                        <p>跟随你的工作习惯切换明暗主题</p>
                      </div>
                      <button className="button" onClick={() => setDark(!dark)}>
                        {dark ? <Sun size={15} /> : <Moon size={15} />}切换为
                        {dark ? '浅色' : '深色'}
                      </button>
                    </div>
                    <div className="setting-row">
                      <div>
                        <strong>本地数据与文件</strong>
                        <p>
                          {isDesktop()
                            ? nativePlatform === 'linux'
                              ? '数据默认保存在用户配置目录的 opsbuddy 文件夹。'
                              : '数据保存在桌面程序旁的 .opsbuddy 文件夹。'
                            : '开发数据保存在启动目录下的 .opsbuddy。'}
                          原文件保留，成果另存。
                        </p>
                      </div>
                      {isDesktop() ? (
                        <button
                          className="button"
                          onClick={() => notify(() => desktop()!.OpenDataFolder())}
                        >
                          <FolderOpen size={14} />
                          打开数据目录
                        </button>
                      ) : (
                        <span className="small-pill">本地存储</span>
                      )}
                    </div>
                    <div className="setting-row">
                      <div>
                        <strong>当前版本能力</strong>
                        <p>基础办公文件生成与处理。内容预览不等同于 Office 排版渲染。</p>
                      </div>
                      <span className="small-pill">
                        {`${appVersion} / ${nativePlatform === 'linux' ? 'Linux' : 'Windows'} 桌面`}
                      </span>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </section>
      <input hidden type="file" multiple ref={uploadRef} onChange={(e) => upload(e.target.files)} />
      {toast && (
        <div className="toast" role="status">
          <CheckCircle2 size={17} />
          {toast}
        </div>
      )}
      {deleteTarget && (
        <DeleteConversationDialog
          task={deleteTarget}
          busy={busy}
          onCancel={() => setDeleteTarget(null)}
          onDelete={() =>
            notify(async () => {
              const deleting = deleteTarget;
              await api(`/tasks/${deleting.id}`, { method: 'DELETE' });
              const conversation = deleting.conversationId || deleting.id;
              if (
                task?.projectId === deleting.projectId &&
                (task.conversationId || task.id) === conversation
              )
                newTask();
              setDeleteTarget(null);
            }, '会话已删除，项目文件仍保留')
          }
        />
      )}
      {inboxOpen && (
        <Suspense fallback={null}>
          <ReminderInbox
            state={state}
            onRefresh={refresh}
            onTask={openTask}
            onClose={() => setInboxOpen(false)}
            onError={setError}
          />
        </Suspense>
      )}
      <AgentDispatcher
        open={dispatchOpen}
        onClose={() => setDispatchOpen(false)}
        state={state}
        projectId={projectId}
        prompt={prompt || task?.prompt || ''}
        sourceTaskId={task?.id}
        inputIds={attachments}
        skillIds={skillIds}
        useMemory={useMemory}
        knowledgeIds={knowledgeIds}
        roleId="auto"
        modelId={modelId}
        kind={kind}
        allowRemote={allowRemote}
        onDispatched={(group) => {
          if (contextEpoch.current !== viewEpoch || group.projectId !== projectId) {
            void refresh();
            return;
          }
          setTaskId(group.rootTaskId);
          setAssetId('');
          setPrompt('');
          setAttachments([]);
          setParentId('');
          refresh().catch((e) => setError(e.message));
        }}
      />
      {modal && (
        <div
          className="modal-backdrop"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !busy) setModal('');
          }}
        >
          <section className="modal" role="dialog" aria-modal="true" aria-labelledby="dialog-title">
            <div className="modal-heading">
              <h2 id="dialog-title">
                {modal === 'project'
                  ? editing?.id
                    ? '编辑项目'
                    : '新建项目'
                  : modal === 'model'
                    ? editing
                      ? '编辑模型'
                      : '添加模型'
                    : extTab === 'skill'
                      ? '添加 / 编辑 Skill'
                      : extTab === 'mcp'
                        ? '配置 MCP 连接'
                        : '安装 / 编辑技能包'}
              </h2>
              <button
                className="icon-button"
                disabled={busy}
                onClick={() => setModal('')}
                title="关闭"
              >
                <X size={18} />
              </button>
            </div>
            {error && <div className="modal-error">{error}</div>}
            <form onSubmit={saveForm} key={modal + extTab + (editing?.id || '')}>
              <div className="form-body">
                <label>
                  名称
                  <input
                    name="name"
                    required
                    autoFocus
                    maxLength={80}
                    defaultValue={editing?.name || ''}
                    placeholder={
                      modal === 'project'
                        ? '例如：产品发布计划'
                        : modal === 'model'
                          ? '例如：我的办公模型'
                          : '为它起一个容易找到的名字'
                    }
                  />
                </label>
                {modal === 'project' && (
                  <label>
                    项目说明与偏好
                    <textarea
                      name="description"
                      rows={5}
                      maxLength={3000}
                      defaultValue={editing?.description || ''}
                      placeholder="项目目标、背景、表达习惯或约束；会作为本项目新任务的参考。"
                    />
                    <small>用于本项目新任务，当前要求优先；已提交任务的说明快照保持不变。</small>
                  </label>
                )}
                {modal === 'model' && (
                  <>
                    <label>
                      接口类型
                      <select
                        name="family"
                        defaultValue={editing?.family || 'openai'}
                        onChange={(e) => {
                          const form = e.currentTarget.form;
                          const base = form?.elements.namedItem('baseUrl') as HTMLInputElement;
                          if (base)
                            base.value =
                              {
                                openai: 'https://api.openai.com/v1',
                                anthropic: 'https://api.anthropic.com/v1',
                                gemini: 'https://generativelanguage.googleapis.com/v1beta',
                              }[e.target.value] || '';
                        }}
                      >
                        <option value="openai">OpenAI-compatible · Chat Completions</option>
                        <option value="anthropic">Anthropic · Messages</option>
                        <option value="gemini">Gemini · GenerateContent</option>
                      </select>
                    </label>
                    <label>
                      Base URL
                      <input
                        required
                        type="url"
                        name="baseUrl"
                        defaultValue={editing?.baseUrl || 'https://api.openai.com/v1'}
                      />
                      <small>填写 API 基础地址，包含 /v1 或 /v1beta，不包含具体请求方法。</small>
                    </label>
                    <label>
                      模型 ID
                      <input
                        required
                        name="modelId"
                        defaultValue={editing?.modelId || ''}
                        placeholder="填写提供商的模型标识"
                      />
                    </label>
                    <label>
                      API Key
                      <input
                        type="password"
                        name="apiKey"
                        autoComplete="new-password"
                        placeholder={
                          editing?.hasKey ? '留空保留已保存的密钥' : '本地服务无需认证时可留空'
                        }
                      />
                      <small>
                        密钥仅保存在本机：Windows 使用 DPAPI，Linux 使用用户独立密钥进行 AES-GCM
                        加密。
                      </small>
                    </label>
                    <div className="form-note">
                      此版本实现非流式内容生成。连接测试只验证文本与 JSON
                      解析，不代表通用工具调用已通过。
                    </div>
                  </>
                )}
                {modal === 'extension' && (
                  <>
                    <label>
                      说明
                      <input
                        name="description"
                        defaultValue={editing?.description || ''}
                        placeholder="它能帮助完成哪些工作？"
                      />
                    </label>
                    {extTab === 'mcp' ? (
                      <>
                        <label>
                          服务地址
                          <input
                            required
                            type="url"
                            name="url"
                            defaultValue={editing?.url || ''}
                            placeholder="https://example.com/mcp"
                          />
                        </label>
                        <div className="form-note">
                          目前只支持无需认证的远程工具发现；不会自动执行发现的工具。
                        </div>
                      </>
                    ) : (
                      <>
                        <label>
                          {extTab === 'skill' ? 'SKILL.md 内容' : '插件清单 JSON'}
                          <textarea
                            className="code-input"
                            name="content"
                            required
                            rows={9}
                            defaultValue={
                              editing?.content ||
                              (extTab === 'plugin'
                                ? '{\n  "name": "我的办公技能包",\n  "version": "1.0.0",\n  "skills": [{\n    "name": "会议纪要",\n    "content": "区分讨论、决策与行动项，不补写未知信息。"\n  }]\n}'
                                : '')
                            }
                            placeholder={'# 技能名称\n\n说明工作步骤和输出要求。'}
                          />
                        </label>
                        <label className="import-file">
                          从文件导入
                          <input
                            type="file"
                            accept={extTab === 'skill' ? '.md,.txt' : '.json'}
                            onChange={async (e) => {
                              const file = e.target.files?.[0];
                              const form = e.currentTarget.form;
                              if (file && form) {
                                const control = form.elements.namedItem(
                                  'content',
                                ) as HTMLTextAreaElement;
                                control.value = await file.text();
                              }
                            }}
                          />
                        </label>
                      </>
                    )}
                    <label className="check-label">
                      <input
                        type="checkbox"
                        name="enabled"
                        defaultChecked={editing ? editing.enabled : true}
                      />
                      启用此扩展
                    </label>
                  </>
                )}
              </div>
              <div className="modal-actions">
                <button
                  className="button"
                  type="button"
                  disabled={busy}
                  onClick={() => setModal('')}
                >
                  取消
                </button>
                <button className="button primary" disabled={busy} type="submit">
                  {busy ? <Loader2 className="spin" size={15} /> : <Check size={15} />}保存
                  {modal === 'model' ? '配置' : ''}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
    </div>
  );
}
function PageTitle({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-title">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </div>
  );
}
