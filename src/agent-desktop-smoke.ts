// Explicit EXE self-check only: deterministic local models, real DOM buttons,
// real Go requests and real persisted artifacts. No configured provider is used.
import { api, post, type AgentGroup, type Model, type State } from './api';

const wait = async (predicate: () => boolean, timeout = 12000) => {
  const start = performance.now();
  while (!predicate()) {
    if (performance.now() - start > timeout) throw new Error('Agent desktop DOM timeout');
    await new Promise((r) => setTimeout(r, 60));
  }
};
const click = (selector: string) => {
  const el = document.querySelector<HTMLButtonElement>(selector);
  if (!el || el.disabled) throw new Error(`Agent control unavailable: ${selector}`);
  el.click();
};
const closeCanvas = async (stage: string) => {
  await wait(() => {
    const b = document.querySelector<HTMLButtonElement>('button[aria-label="关闭编排画布"]');
    return !b || !b.disabled;
  });
  const button = document.querySelector<HTMLButtonElement>('button[aria-label="关闭编排画布"]');
  if (button) button.click();
  await wait(() => !document.querySelector('.agent-canvas-dialog')).catch(() => {
    throw new Error(`Canvas close failed: ${stage}`);
  });
};
const set = (selector: string, value: string) => {
  const el = document.querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(
    selector,
  );
  if (!el) throw new Error(`Agent field unavailable: ${selector}`);
  const prototype =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : el instanceof HTMLSelectElement
        ? HTMLSelectElement.prototype
        : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, 'value')!.set!.call(el, value);
  el.dispatchEvent(
    new Event(el instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }),
  );
};

export async function runAgentDesktopSmoke(
  endpoint: string,
  hooks: { refresh: () => Promise<void>; navigate: (page: 'workspace' | 'workflows') => void },
) {
  const models: Model[] = [];
  for (const name of ['main', 'front', 'back'])
    models.push(
      await post<Model>('/models', {
        name: `fixture-${name}`,
        family: 'openai',
        baseUrl: `${endpoint}/${name}`,
        modelId: `fixture-${name}`,
        apiKey: '',
      }),
    );
  await hooks.refresh();
  hooks.navigate('workspace');
  const result: Record<string, unknown> = { mockOnly: true };
  for (const kind of ['docx', 'code']) {
    click('button[aria-label="新建任务"]');
    await new Promise((r) => requestAnimationFrame(r));
    set('select[aria-label="模型"]', models[0].id);
    set(
      'textarea[aria-label="任务需求"]',
      kind === 'code'
        ? 'CODE_COLLAB：前端与Go后端并行开发计算器并启动预览'
        : 'OFFICE_COLLAB：预算120元，数量3，资料分析后撰写并审阅报告',
    );
    await new Promise((r) => requestAnimationFrame(r));
    if (!document.querySelector('.advanced-menu[open]'))
      click('summary[aria-label="更多任务选项"]');
    click('.team-dispatch-action button');
    await wait(() => !!document.querySelector('.team-dispatcher'));
    set('select[aria-label="协作最终交付"]', kind);
    await new Promise((r) => requestAnimationFrame(r));
    click('.team-dispatcher footer .primary');
    await wait(() => !document.querySelector('.team-dispatcher'));
    const deadline = performance.now() + 110000;
    let group: AgentGroup | undefined;
    while (performance.now() < deadline) {
      const state = await api<State>('/state');
      group = [...state.agentGroups]
        .reverse()
        .find((g) => g.prompt.includes(kind === 'code' ? 'CODE_COLLAB' : 'OFFICE_COLLAB'));
      if (group && !['planning', 'running', 'aggregating'].includes(group.status)) break;
      await new Promise((r) => setTimeout(r, 100));
    }
    if (group?.status !== 'succeeded')
      throw new Error(`Agent ${kind} failed: ${group?.status}: ${group?.error}`);
    await hooks.refresh();
    await wait(
      () =>
        !!document.querySelector('.team-plan-summary') &&
        !!document.querySelector('.team-review-history'),
    );
    const state = await api<State>('/state');
    const root = state.tasks.find((t) => t.id === group!.rootTaskId)!;
    const asset = state.assets.find((a) => a.id === root.outputIds[0])!;
    if (
      !asset ||
      asset.kind !== kind ||
      !group.plannerTaskId ||
      group.reviews?.[0]?.status !== 'pass'
    )
      throw new Error('Agent plan/review/asset missing');
    const receipts = state.tasks
      .filter((t) => t.agentGroupId === group!.id)
      .flatMap((t) => t.steps.filter((s) => s.name === '模型接口响应').map((s) => s.detail));
    if (!receipts.length || receipts.some((s) => !s.includes('fixture-actual')))
      throw new Error('Model response receipts missing');
    const groupEvidence = {
      id: group.id,
      rootTaskId: root.id,
      assetId: asset.id,
      plan: group.plan?.summary,
      members: group.members.map((m) => ({
        id: m.id,
        model: m.configSnapshot?.model?.modelId,
        status: m.status,
        dependsOn: m.dependsOn,
        taskIds: m.taskIds,
      })),
      review: group.reviews?.[0]?.status,
      receipts,
    };
    const graphButton = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.team-controls button'),
    ).find((b) => b.textContent?.includes('编排画布'));
    if (!graphButton) throw new Error('Agent canvas entry missing');
    graphButton.click();
    await wait(
      () =>
        document.querySelectorAll('.agent-canvas-dialog .react-flow__node').length ===
        group!.members.length + 2,
    );
    const dialog = document.querySelector('.agent-canvas-dialog')!.getBoundingClientRect();
    const viewport = document.querySelector('.agent-canvas-flow')!.getBoundingClientRect();
    if (
      dialog.left < 0 ||
      dialog.right > innerWidth + 1 ||
      dialog.bottom > innerHeight + 1 ||
      viewport.width < 250 ||
      viewport.height < 250
    )
      throw new Error('Agent canvas is clipped or blank');
    result[`${kind}Canvas`] = {
      nodes: group.members.length + 2,
      width: viewport.width,
      height: viewport.height,
      insideWindow: true,
    };
    await closeCanvas(`${kind} completed graph`);
    if (kind === 'docx') {
      if (!asset.preview?.includes('120') || !asset.preview.includes('数量为3'))
        throw new Error('Office extracted content mismatch');
      const download = await fetch(`/api/assets/${asset.id}/file`);
      const bytes = new Uint8Array(await download.arrayBuffer());
      if (bytes[0] !== 80 || bytes[1] !== 75 || bytes.length < 500)
        throw new Error('Office file missing actual OpenXML package');
      result.office = { ...groupEvidence, extractedBudget: 120, extractedQuantity: 3 };
      continue;
    }
    let ready = false,
      calculation = 0,
      bridge = true,
      source = '';
    const receive = (e: MessageEvent) => {
      const frame = document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]');
      if (e.source !== frame?.contentWindow) return;
      if (e.data?.opsbuddyAgentReady) ready = true;
      if (e.data?.opsbuddyAgentCalculation !== undefined) {
        calculation = e.data.opsbuddyAgentCalculation;
        bridge = e.data.bridge;
        source = e.data.source;
      }
    };
    window.addEventListener('message', receive);
    try {
      // Open the actual produced asset through the file button, not a fixture URL.
      const filesTab = Array.from(
        document.querySelectorAll<HTMLButtonElement>('.sidebar-mode-tabs button'),
      ).find((b) => b.textContent === '文件')!;
      filesTab.click();
      await wait(() => !!document.querySelector(`.sidebar-file[title="${asset.name}"]`));
      click(`.sidebar-file[title="${asset.name}"]`);
      await wait(() => !!document.querySelector('iframe[title="项目运行预览"]'), 30000);
      const frame = document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]')!;
      // The preview may have loaded when root delivery selected it automatically.
      if (!ready) {
        frame.src = frame.src;
        await wait(() => ready);
      }
      frame.contentWindow!.postMessage({ opsbuddyAgentClick: true }, '*');
      await wait(() => calculation === 12);
      if (
        bridge ||
        source !== 'go-backend' ||
        frame.getAttribute('sandbox')?.includes('allow-same-origin')
      )
        throw new Error('Go preview isolation or API response incorrect');
      const tabs = Array.from(
        document.querySelectorAll<HTMLButtonElement>('.code-project-preview [role="tab"]'),
      );
      tabs.find((b) => b.textContent?.includes('项目文件'))!.click();
      await wait(() => document.querySelectorAll('.code-file-tree button').length === 3);
      click('.code-file-tree button[title="main.go"]');
      await wait(
        () =>
          document
            .querySelector<HTMLTextAreaElement>('[aria-label="项目源码"]')
            ?.value.includes('http.ListenAndServe') === true,
      );
      click('.code-file-tree button[title="index.html"]');
      await wait(
        () =>
          document
            .querySelector<HTMLTextAreaElement>('[aria-label="项目源码"]')
            ?.value.includes('/api/add') === true,
      );
      tabs.find((b) => b.textContent?.includes('运行日志'))!.click();
      await wait(
        () =>
          document
            .querySelector('[aria-label="项目运行日志"]')
            ?.textContent?.includes('agent-go-backend-ready') === true,
      );
      click('.code-runtime-bar button');
      await wait(() => document.querySelector('.code-runtime-state')?.textContent === '未运行');
      const runtime = await api<{ status: string }>(`/assets/${asset.id}/runtime`);
      if (runtime.status !== 'stopped') throw new Error('Go runtime stop button failed');
      result.code = {
        ...groupEvidence,
        buttonInvokedGoAPI: true,
        calculation,
        source,
        bridgeAbsent: !bridge,
        sourceTree: ['index.html', 'main.go', 'go.mod'],
        startupLog: true,
        stopButton: true,
      };
    } finally {
      window.removeEventListener('message', receive);
    }
  }
  click('button[aria-label="新建任务"]');
  await new Promise((r) => requestAnimationFrame(r));
  set('select[aria-label="模型"]', models[0].id);
  set('textarea[aria-label="任务需求"]', 'CANCEL_COLLAB：检验追加要求与整组停止按钮');
  await new Promise((r) => requestAnimationFrame(r));
  if (!document.querySelector('.advanced-menu[open]')) click('summary[aria-label="更多任务选项"]');
  click('.team-dispatch-action button');
  await wait(() => !!document.querySelector('.team-dispatcher'));
  set('select[aria-label="协作规划方式"]', 'manual');
  await wait(() => document.querySelectorAll('.team-member-editors article').length === 3);
  set('select[aria-label="成员 1 模型"]', models[1].id);
  set('select[aria-label="协作最终交付"]', 'chat');
  await new Promise((r) => requestAnimationFrame(r));
  click('.team-dispatcher footer .primary');
  await wait(
    () => !document.querySelector('.team-dispatcher') && !!document.querySelector('.team-stop'),
  );
  await wait(() =>
    Array.from(document.querySelectorAll<HTMLButtonElement>('.team-member-info button')).some(
      (b) => b.textContent === '追加指令',
    ),
  );
  Array.from(document.querySelectorAll<HTMLButtonElement>('.team-member-info button'))
    .find((b) => b.textContent === '追加指令')!
    .click();
  await wait(() => !!document.querySelector('.team-member-info form textarea'));
  set('.team-member-info form textarea', '补充一条可验证要求，并保留已有成果');
  await new Promise((r) => requestAnimationFrame(r));
  click('.team-member-info form button[type="submit"]');
  await wait(() => !document.querySelector('.team-member-info form'));
  const beforeCancel = (await api<State>('/state')).agentGroups.find((g) =>
    g.prompt.includes('CANCEL_COLLAB'),
  )!;
  if (
    beforeCancel.members[0].configSnapshot?.model?.id !== models[1].id ||
    beforeCancel.members[0].instructions.length !== 2
  )
    throw new Error('Manual model selection or append button did not persist');
  click('.team-stop');
  await wait(() => !document.querySelector('.team-stop'));
  const cancelled = (await api<State>('/state')).agentGroups.find((g) => g.id === beforeCancel.id)!;
  if (
    cancelled.status !== 'cancelled' ||
    cancelled.members.slice(1).some((m) => m.taskIds.length > 0)
  )
    throw new Error('Group cancel dispatched dependent work');
  result.controls = {
    manualModelSelection: true,
    appendInstruction: true,
    stopGroup: true,
    noDependentDispatch: true,
    groupId: cancelled.id,
  };
  // Pause an actual in-flight model request, edit the graph, then resume it.
  click('button[aria-label="新建任务"]');
  await new Promise((r) => requestAnimationFrame(r));
  set('textarea[aria-label="任务需求"]', 'RECOVER_COLLAB：暂停后调整分工并继续');
  await new Promise((r) => requestAnimationFrame(r));
  if (!document.querySelector('.advanced-menu[open]')) click('summary[aria-label="更多任务选项"]');
  click('.team-dispatch-action button');
  await wait(() => !!document.querySelector('.team-dispatcher'));
  set('select[aria-label="协作规划方式"]', 'manual');
  set('select[aria-label="协作最终交付"]', 'chat');
  await new Promise((r) => requestAnimationFrame(r));
  click('.team-dispatcher footer .primary');
  await wait(
    () => !document.querySelector('.team-dispatcher') && !!document.querySelector('.team-controls'),
  );
  const findButton = (scope: string, text: string) =>
    Array.from(document.querySelectorAll<HTMLButtonElement>(`${scope} button`)).find((b) =>
      b.textContent?.includes(text),
    );
  findButton('.team-controls', '编排画布')!.click();
  await wait(() => !!document.querySelector('.agent-canvas-dialog'));
  findButton('.agent-canvas-header', '暂停并调整')!.click();
  await wait(
    () =>
      !!findButton('.agent-canvas-header', '继续执行') &&
      !findButton('.agent-canvas-header', '继续执行')!.disabled,
  );
  click('.agent-canvas-dialog .react-flow__node[data-id="node-1"]');
  await wait(() => !!document.querySelector('textarea[aria-label="编排成员任务"]'));
  set('textarea[aria-label="编排成员任务"]', '资料探查：记录恢复后新的分工要求');
  await new Promise((r) => requestAnimationFrame(r));
  findButton('.agent-canvas-header', '保存计划')!.click();
  await wait(
    () =>
      !findButton('.agent-canvas-header', '保存计划') &&
      !!findButton('.agent-canvas-header', '继续执行') &&
      !findButton('.agent-canvas-header', '继续执行')!.disabled,
  );
  findButton('.agent-canvas-header', '继续执行')!.click();
  await wait(
    () => !document.querySelector<HTMLButtonElement>('button[aria-label="关闭编排画布"]')?.disabled,
  );
  await closeCanvas('resume graph');
  let resumed: AgentGroup | undefined;
  const recoveryDeadline = performance.now() + 15000;
  while (performance.now() < recoveryDeadline) {
    const snapshot = await api<State>('/state');
    resumed = snapshot.agentGroups.find((g) => g.prompt.includes('RECOVER_COLLAB'));
    if (resumed?.status === 'succeeded') break;
    await new Promise((r) => setTimeout(r, 100));
  }
  if (resumed?.status !== 'succeeded' || resumed.planVersion !== 2 || resumed.resumeCount !== 1)
    throw new Error(`Graph edit/resume failed: ${resumed?.status}`);
  await hooks.refresh();
  hooks.navigate('workflows');
  await wait(() => !!document.querySelector('.agent-workflow-cards'));
  const projectCards = document.querySelectorAll('.agent-workflow-cards article');
  if (!projectCards.length) throw new Error('Project collaboration not linked to workflows');
  findButton('.agent-workflow-cards', '查看编排')!.click();
  await wait(() => !!document.querySelector('.agent-canvas-dialog'));
  await closeCanvas('workflow entry');
  result.recoveryControls = {
    pause: true,
    editPlan: true,
    resume: true,
    planVersion: 2,
    workflowEntry: true,
    groupId: resumed.id,
  };
  // Keep the isolated fixture artifacts and their model snapshots for inspection.
  return result;
}
