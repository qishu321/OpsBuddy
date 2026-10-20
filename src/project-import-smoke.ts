// Explicit hidden EXE self-check. Exercises the REAL imported collaboration,
// with no provider calls, no file dialogs and no user desktop manipulation.
import { api, type State } from './api';
const wait = async (test: () => boolean, timeout = 20000) => {
  const until = performance.now() + timeout;
  while (!test()) {
    if (performance.now() > until) throw new Error('Imported project DOM timed out');
    await new Promise((r) => setTimeout(r, 80));
  }
};
const click = (scope: string, text: string) => {
  const b = Array.from(document.querySelectorAll<HTMLButtonElement>(scope)).find((b) =>
    b.textContent?.includes(text),
  );
  if (!b || b.disabled) throw new Error(`Import control missing: ${text}`);
  b.click();
};
export async function runProjectImportSmoke(hooks: {
  refresh: () => Promise<void>;
  navigate: (page: 'projects' | 'workflows' | 'workspace') => void;
}) {
  const state = await api<State>('/state');
  const imported = state.startupImport;
  if (!imported) throw new Error('Startup import not exposed');
  const project = state.projects.find((p) => p.id === imported.projectId);
  const root = state.tasks.find((t) => t.id === imported.taskId);
  const group = state.agentGroups.find((g) => g.rootTaskId === root?.id);
  if (!project?.importKey || !root || !group || group.members.length !== 3 || state.models.length)
    throw new Error('Imported project/history incomplete');
  await wait(
    () =>
      !!document.querySelector(
        `.project-session-group[data-project-id="${project.id}"].current .history-item.selected`,
      ),
  );
  await wait(() => document.querySelectorAll('.code-file-tree button').length === 3);
  if (
    !document.querySelector('.imported-project-note') ||
    !document.querySelector('.agent-group-panel')
  )
    throw new Error('Imported history not in main conversation');
  const assetId = root.outputIds[0];
  const before = await api<{ status: string }>(`/assets/${assetId}/runtime`);
  if (before.status !== 'stopped' || document.querySelector('iframe[title="项目运行预览"]'))
    throw new Error('Import silently executed code');
  click('.team-controls button', '编排画布');
  await wait(
    () => document.querySelectorAll('.agent-canvas-dialog .react-flow__node').length === 5,
  );
  document.querySelector<HTMLButtonElement>('[aria-label="关闭编排画布"]')?.click();
  await wait(() => !document.querySelector('.agent-canvas-dialog'));
  hooks.navigate('projects');
  await wait(() => !!document.querySelector('.project-page-actions'));
  if (
    !Array.from(document.querySelectorAll('.project-card h3')).some(
      (el) => el.textContent === project.name,
    )
  )
    throw new Error('Project absent from projects page');
  if (
    !Array.from(document.querySelectorAll('.project-page-actions button')).some((b) =>
      b.textContent?.includes('导入项目'),
    )
  )
    throw new Error('Native import entry absent');
  hooks.navigate('workflows');
  await wait(() => !!document.querySelector('.agent-workflow-cards'));
  click('.agent-workflow-cards button', '查看编排');
  await wait(
    () => document.querySelectorAll('.agent-canvas-dialog .react-flow__node').length === 5,
  );
  document.querySelector<HTMLButtonElement>('[aria-label="关闭编排画布"]')!.click();
  await wait(() => !document.querySelector('.agent-canvas-dialog'));
  hooks.navigate('workspace');
  await wait(() => !!document.querySelector('.code-runtime-bar button'));
  click('.code-runtime-bar button', '本机运行');
  await wait(() => !!document.querySelector('iframe[title="项目运行预览"]'), 45000);
  const runtime = await api<{ status: string; url?: string }>(`/assets/${assetId}/runtime`);
  if (runtime.status !== 'running' || !runtime.url)
    throw new Error('Imported final code did not start');
  const health = await fetch(new URL('api/health', runtime.url)).then((r) => r.json());
  if (health.ok !== true) throw new Error('Imported Go backend health failed');
  click('.code-runtime-bar button', '停止');
  await wait(() => !document.querySelector('iframe[title="项目运行预览"]'));
  const receipts = state.tasks
    .filter((t) => t.projectId === project.id)
    .flatMap((t) => t.steps.filter((s) => s.name === '模型接口响应'));
  if (receipts.length !== 5) throw new Error('Original model receipts missing');
  await hooks.refresh();
  return {
    passed: true,
    projectName: project.name,
    projectId: project.id,
    taskId: root.id,
    sidebar: true,
    projectsPage: true,
    mainConversation: true,
    workflowCanvas: true,
    nodes: 5,
    historyTasks: state.tasks.filter((t) => t.projectId === project.id).length,
    originalReceipts: receipts.length,
    sourceFiles: 3,
    noAutomaticRun: true,
    explicitRun: true,
    goHealth: true,
    stop: true,
  };
}
