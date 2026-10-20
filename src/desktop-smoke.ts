// Runs only when the EXE is explicitly launched with --smoke-test.
import {
  api,
  post,
  type State,
  type Model,
  type Task,
  type Asset,
  type KnowledgeBase,
  type KnowledgeDocument,
  type KnowledgeHit,
  type WorkflowDefinition,
  type WorkflowRun,
  type AgentGroup,
  type Memory,
  type Schedule,
  type ScheduleRun,
} from './api';
import { desktop } from './desktop';
import { runAgentDesktopSmoke } from './agent-desktop-smoke';

export async function runDesktopSmoke(hooks: {
  refresh: () => Promise<void>;
  navigate: (
    page:
      | 'knowledge'
      | 'workflows'
      | 'workspace'
      | 'memory'
      | 'schedules'
      | 'extensions'
      | 'projects'
      | 'settings',
  ) => void;
}) {
  const report: Record<string, unknown> = { mode: 'desktop', startedAt: new Date().toISOString() };
  try {
    const native = desktop();
    if (!native) throw new Error('Native bindings missing');
    if ((await native.Info()).smokeImportDemo === 'true') {
      report.phase = 'project-import';
      const { runProjectImportSmoke } = await import('./project-import-smoke');
      report.projectImport = await runProjectImportSmoke(hooks);
      report.passed = true;
      report.finishedAt = new Date().toISOString();
      await native.ReportSmoke(JSON.stringify(report, null, 2));
      return;
    }
    const smokeModelURL = (await native.Info()).smokeModelURL;
    if (!smokeModelURL)
      throw new Error(
        'Start the local agent-smoke-fixture and set OPSBUDDY_SMOKE_MODEL_URL for this isolated EXE self-check',
      );
    report.nativeBindings = true;
    report.embeddedUI = Boolean(
      document.querySelector('.app') && document.querySelector('.composer'),
    );
    const composerBox = document.querySelector('.compact-composer')!.getBoundingClientRect();
    if (
      document.querySelector('select[aria-label="任务角色"]') ||
      Array.from(document.querySelectorAll('.sidebar nav button')).some((b) =>
        b.textContent?.includes('专家库'),
      )
    )
      throw new Error('Duplicate expert entry remains');
    report.noExpertLibrary = true;
    if (composerBox.height > 220 || !document.querySelector('.inline-model-picker'))
      throw new Error('Compact composer layout missing');
    report.compactComposer = {
      height: composerBox.height,
      inlineModel: true,
      advancedClosed: !document.querySelector('.advanced-menu[open]'),
    };
    const homeCards = Array.from(document.querySelectorAll<HTMLButtonElement>('.template-card'));
    const homeKinds = ['pptx', 'docx', 'xlsx', 'pdf', 'html', 'md'];
    if (homeCards.length !== homeKinds.length) throw new Error('Workspace entry points missing');
    for (let i = 0; i < homeCards.length; i++) {
      homeCards[i].click();
      await new Promise((resolve) => requestAnimationFrame(resolve));
      const selectedKind = document.querySelector<HTMLSelectElement>(
        'select[aria-label="任务类型"]',
      )?.value;
      if (selectedKind !== homeKinds[i] || !document.querySelector('.composer-format'))
        throw new Error('Task entry did not expose selected output format');
    }
    document.querySelector<HTMLButtonElement>('button[aria-label="恢复自动选择格式"]')!.click();
    await new Promise((resolve) => requestAnimationFrame(resolve));
    if (
      document.querySelector<HTMLSelectElement>('select[aria-label="任务类型"]')?.value !== 'auto'
    )
      throw new Error('Output format reset failed');
    document.querySelector<HTMLButtonElement>('button[aria-label="新建任务"]')!.click();
    await new Promise((resolve) => requestAnimationFrame(resolve));
    report.workspaceEntryPoints = { formats: homeKinds, resetToAuto: true };
    const state = await api<State>('/state');
    report.apiWithoutTCPListener = state.projects.length > 0;
    const t = await post<Task>('/tasks', {
      projectId: state.projects[0].id,
      roleId: 'docx',
      kind: 'docx',
      prompt: '桌面集成自检\n验证原生应用中的文件生成。',
    });
    let task: Task | undefined;
    for (let i = 0; i < 100; i++) {
      const updated = await api<State>('/state');
      task = updated.tasks.find((x) => x.id === t.id);
      if (task && task.status !== 'queued' && task.status !== 'running') break;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    if (!task || task.status !== 'succeeded' || !task.outputIds.length)
      throw new Error(task?.error || 'Task did not finish');
    report.wordTask = { status: task.status, outputCount: task.outputIds.length };
    const response = await fetch(`/api/assets/${task.outputIds[0]}/file`);
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (!response.ok || bytes[0] !== 80 || bytes[1] !== 75)
      throw new Error('DOCX package unavailable');
    report.assetDeliveredInProcess = bytes.length > 0;
    const base = await post<KnowledgeBase>('/knowledge', {
      projectId: state.projects[0].id,
      name: '桌面自检知识库',
      description: '隔离自检资料',
    });
    const form = new FormData();
    form.append('projectId', state.projects[0].id);
    form.append(
      'files',
      new File(['差旅住宿报销标准：每晚不超过 450 元，需要发票。'], '自检制度.txt', {
        type: 'text/plain',
      }),
    );
    const files = await api<Asset[]>('/assets', { method: 'POST', body: form });
    const docs = await post<KnowledgeDocument[]>(`/knowledge/${base.id}/documents`, {
      assetIds: [files[0].id],
    });
    const hits = await post<KnowledgeHit[]>('/knowledge/search', {
      projectId: state.projects[0].id,
      baseIds: [base.id],
      query: '住宿报销标准',
    });
    if (docs[0].status !== 'ready' || !hits.some((h) => h.text.includes('450')))
      throw new Error('Knowledge index/search failed');
    report.knowledge = {
      indexed: docs.length,
      retrievalHits: hits.length,
      sourcePreserved: hits[0].assetId === files[0].id,
    };
    const flow = await post<WorkflowDefinition>('/workflows', {
      projectId: state.projects[0].id,
      name: '桌面自检：检索与撰写',
      enabled: true,
      nodes: [
        { id: 'start', type: 'start', label: '开始', x: 20, y: 100 },
        {
          id: 'kb',
          type: 'knowledge',
          label: '知识检索',
          x: 290,
          y: 100,
          baseIds: [base.id],
          query: '{{input}}',
        },
        {
          id: 'writer',
          type: 'agent',
          label: '根据资料撰写',
          x: 560,
          y: 100,
          kind: 'docx',
          roleId: 'docx',
          prompt: '{{input}}',
        },
        { id: 'end', type: 'end', label: '结束', x: 830, y: 100 },
      ],
      edges: [
        { id: 'e1', source: 'start', target: 'kb', sourceHandle: 'out' },
        { id: 'e2', source: 'kb', target: 'writer', sourceHandle: 'out' },
        { id: 'e3', source: 'writer', target: 'end', sourceHandle: 'out' },
      ],
    });
    const started = await post<WorkflowRun>(`/workflows/${flow.id}/runs`, {
      prompt: '住宿报销标准',
      inputIds: [],
    });
    let finished: WorkflowRun | undefined;
    for (let i = 0; i < 100; i++) {
      const s = await api<State>('/state');
      finished = s.workflowRuns.find((r) => r.id === started.id);
      if (finished && !['queued', 'running'].includes(finished.status)) break;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    if (!finished || finished.status !== 'succeeded' || finished.outputIds.length !== 1)
      throw new Error('Workflow did not produce output');
    const updated = await api<State>('/state');
    const artifact = updated.assets.find((a) => a.id === finished!.outputIds[0]);
    if (!artifact?.preview?.includes('450'))
      throw new Error('Knowledge was not handed to the real document task');
    report.workflow = {
      status: finished.status,
      nodeCount: finished.nodes.length,
      outputCount: finished.outputIds.length,
      knowledgeInArtifact: true,
    };
    await hooks.refresh();
    const waitDOM = async (test: () => boolean, timeout = 5000) => {
      for (let i = 0; i < timeout / 50; i++) {
        if (test()) return;
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      throw new Error('Desktop UI did not render expected module');
    };
    hooks.navigate('knowledge');
    await waitDOM(
      () => !!document.querySelector('.knowledge-documents')?.textContent?.includes('自检制度.txt'),
    );
    report.knowledgePageRendered = true;
    hooks.navigate('workflows');
    await waitDOM(
      () => !!document.querySelector('.saved-workflows')?.textContent?.includes(flow.name),
    );
    const card = Array.from(document.querySelectorAll('.saved-workflows article')).find((el) =>
      el.textContent?.includes(flow.name),
    );
    const editButton = Array.from(card?.querySelectorAll('button') || []).find(
      (b) => b.textContent?.trim() === '编辑画布',
    );
    if (!editButton) throw new Error('Workflow editor action missing');
    (editButton as HTMLButtonElement).click();
    await waitDOM(
      () =>
        document.querySelectorAll('.react-flow__node').length === 4 &&
        !!document.querySelector('[data-testid="workflow-canvas"]'),
    );
    report.canvas = {
      rendered: true,
      nodeCount: document.querySelectorAll('.react-flow__node').length,
      edgeCount: document.querySelectorAll('.react-flow__edge').length,
    };
    report.phase = 'workflow-snapshot';
    document.querySelector<HTMLButtonElement>('button[title="查看运行记录"]')!.click();
    await waitDOM(() => !!document.querySelector('.run-history-item'));
    document.querySelector<HTMLButtonElement>('.run-history-item')!.click();
    await waitDOM(
      () =>
        !!document.querySelector('[data-testid="workflow-results"]') &&
        document.querySelectorAll('.react-flow__node').length === 4,
    );
    await waitDOM(() => {
      const bounds = document
        .querySelector('[data-testid="workflow-canvas"]')!
        .getBoundingClientRect();
      return Array.from(document.querySelectorAll<HTMLElement>('.react-flow__node')).every((el) => {
        const box = el.getBoundingClientRect();
        const css = getComputedStyle(el);
        return (
          css.visibility !== 'hidden' &&
          css.opacity !== '0' &&
          box.width > 0 &&
          box.left >= bounds.left - 2 &&
          box.right <= bounds.right + 2 &&
          box.top >= bounds.top - 2 &&
          box.bottom <= bounds.bottom + 2
        );
      });
    });
    if (!document.querySelector('.workflow-result-content')?.textContent?.includes('450'))
      throw new Error('Workflow output preview missing');
    report.workflowSnapshot = {
      allNodesVisible: true,
      edgeCount: document.querySelectorAll('.react-flow__edge').length,
      resultsInline: true,
      artifactPreview: true,
    };
    const returnToEdit = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.flow-editor-bar button'),
    ).find((b) => b.textContent?.trim() === '返回编辑');
    returnToEdit!.click();
    await waitDOM(
      () =>
        !document.querySelector('.workflow-results') &&
        document.querySelectorAll('.react-flow__node').length === 4,
    );
    const node = Array.from(document.querySelectorAll('.react-flow__node')).find((el) =>
      el.textContent?.includes('根据资料撰写'),
    );
    (node as HTMLElement)?.click();
    await waitDOM(() => !!document.querySelector('.inspector-content textarea'));
    if (!document.querySelector('[aria-label="节点技能"]'))
      throw new Error('Workflow skill configuration missing');
    report.nodeConfigurationRendered = true;
    const button = (selector: string) => {
      const el = document.querySelector<HTMLButtonElement>(selector);
      if (!el) throw new Error(`Missing workflow control: ${selector}`);
      return el;
    };
    button('.canvas-arrange').click();
    await waitDOM(
      () =>
        document
          .querySelector<HTMLElement>('.react-flow__node[data-id="start"]')
          ?.style.transform.includes('40px') === true,
    );
    button('button[title="撤销"]').click();
    await waitDOM(
      () =>
        document
          .querySelector<HTMLElement>('.react-flow__node[data-id="start"]')
          ?.style.transform.includes('20px') === true,
    );
    button('button[title="重做"]').click();
    await waitDOM(
      () =>
        document
          .querySelector<HTMLElement>('.react-flow__node[data-id="start"]')
          ?.style.transform.includes('40px') === true,
    );
    const titleFont = parseFloat(
      getComputedStyle(document.querySelector('.node-title strong')!).fontSize,
    );
    if (titleFont < 14) throw new Error('Workflow node title is too small');
    const editor = document.querySelector('.flow-editor')!.getBoundingClientRect();
    if (editor.bottom > innerHeight + 1 || editor.right > innerWidth + 1)
      throw new Error('Workflow editor overflows the desktop viewport');
    button('button[aria-label="切换小地图"]').click();
    await waitDOM(() => !!document.querySelector('.react-flow__minimap'));
    button('button[aria-label="切换小地图"]').click();
    await waitDOM(() => !document.querySelector('.react-flow__minimap'));
    // Discard only this isolated smoke test's arranged draft.
    const discard = Array.from(document.querySelectorAll('.flow-editor-bar button')).find(
      (el) => el.textContent?.trim() === '放弃修改',
    );
    (discard as HTMLButtonElement).click();
    await waitDOM(() => !document.querySelector('button[title="撤销"]:not(:disabled)'));
    button('button[title="返回工作流列表"]').click();
    await waitDOM(() => !!document.querySelector('.flow-page'));
    button('.flow-page .page-title button').click();
    await waitDOM(() => document.querySelectorAll('.react-flow__node').length === 2);
    button('.palette-node.agent').click();
    await waitDOM(
      () =>
        document.querySelectorAll('.react-flow__node').length === 3 &&
        document.querySelectorAll('.react-flow__edge').length === 2,
    );
    const save = Array.from(document.querySelectorAll('.flow-editor-bar button')).find(
      (el) => el.textContent?.trim() === '保存',
    );
    (save as HTMLButtonElement).click();
    await waitDOM(
      () => document.querySelector('.flow-notice')?.textContent?.includes('已保存') === true,
    );
    const persisted = (await api<State>('/state')).workflows.find((f) => f.name === '新的工作流');
    if (!persisted || persisted.nodes.length !== 3 || persisted.edges.length !== 2)
      throw new Error('Inserted workflow node did not persist');
    report.workflowStudio = {
      titleFont,
      editorFitsViewport: true,
      arrangeUndoRedo: true,
      minimapToggle: true,
      firstNodeAutoConnected: true,
      graphSaved: true,
    };
    const agentSmokeModel = await post<Model>('/models', {
      name: 'fixture-main',
      family: 'openai',
      baseUrl: smokeModelURL + '/main',
      modelId: 'fixture-main',
      apiKey: '',
    });
    const group = await post<AgentGroup>('/agent-groups', {
      projectId: state.projects[0].id,
      sourceTaskId: t.id,
      prompt: '桌面子任务自检',
      kind: 'docx',
      modelId: agentSmokeModel.id,
      allowRemote: true,
      members: [
        { role: 'worker', name: '整理材料', prompt: '整理引用文档' },
        { role: 'reviewer', name: '检查成果', prompt: '检查其他成员的成果' },
      ],
    });
    let completed: AgentGroup | undefined;
    for (let i = 0; i < 100; i++) {
      completed = (await api<State>('/state')).agentGroups.find((g) => g.id === group.id);
      if (completed && !['running', 'aggregating'].includes(completed.status)) break;
      await new Promise((resolve) => setTimeout(resolve, 80));
    }
    if (completed?.status !== 'succeeded') throw new Error('Desktop agent group did not succeed');
    await api(`/models/${agentSmokeModel.id}`, { method: 'DELETE' });
    await hooks.refresh();
    hooks.navigate('workspace');
    await waitDOM(() =>
      Array.from(document.querySelectorAll('.sidebar-history .history-item')).some((el) =>
        el.textContent?.includes('桌面子任务自检'),
      ),
    );
    const session = Array.from(document.querySelectorAll('.sidebar-history .history-item')).find(
      (el) => el.textContent?.includes('桌面子任务自检'),
    );
    (session as HTMLButtonElement).click();
    await waitDOM(
      () =>
        !!document.querySelector('[data-testid="agent-group"]') &&
        !!document.querySelector('[data-testid="conversation-history"]'),
    );
    button('button[aria-label="添加材料"]').click();
    await waitDOM(() => !!document.querySelector('.material-popover'));
    const materialsBounds = document.querySelector('.material-popover')!.getBoundingClientRect();
    if (materialsBounds.top < 0 || materialsBounds.right > innerWidth)
      throw new Error('Materials picker overflows desktop viewport');
    button('button[aria-label="关闭材料选择"]').click();
    const fixtureCanvas = document.createElement('canvas');
    fixtureCanvas.width = 8;
    fixtureCanvas.height = 8;
    fixtureCanvas.getContext('2d')!.fillRect(0, 0, 8, 8);
    const fixtureBlob = await new Promise<Blob>((resolve, reject) =>
      fixtureCanvas.toBlob(
        (blob) => (blob ? resolve(blob) : reject(new Error('Image fixture failed'))),
        'image/png',
      ),
    );
    const transfer = new DataTransfer();
    transfer.items.add(new File([fixtureBlob], '桌面拖入自检.png', { type: 'image/png' }));
    document
      .querySelector('.composer')!
      .dispatchEvent(
        new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }),
      );
    await waitDOM(
      () =>
        document.querySelector('.attached-files')?.textContent?.includes('桌面拖入自检.png') ===
        true,
    );
    report.phase = 'clipboard-screenshot';
    const clipboard = new DataTransfer();
    clipboard.items.add(new File([fixtureBlob], 'image.png', { type: 'image/png' }));
    clipboard.setData('text/plain', '请分析截图');
    const input = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="任务需求"]')!;
    const previousInput = input.value;
    input.setSelectionRange(0, 0);
    input.dispatchEvent(
      new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: clipboard }),
    );
    await waitDOM(
      () => document.querySelectorAll('.attached-files .image-attachment').length === 2,
    );
    await waitDOM(() =>
      Array.from(document.querySelectorAll<HTMLImageElement>('.attachment-image img')).every(
        (img) => img.complete && img.naturalWidth > 0,
      ),
    );
    if (input.value !== '请分析截图' + previousInput)
      throw new Error('Mixed screenshot paste lost accompanying text');
    const screenshotButton = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.attachment-image'),
    ).find((el) => el.getAttribute('aria-label')?.includes('Clipboard_Screenshot'))!;
    screenshotButton.click();
    await waitDOM(() => !!document.querySelector('.asset-image-preview img'));
    button('button[aria-label="关闭成果预览"]').click();
    const plainClipboard = new DataTransfer();
    plainClipboard.setData('text/plain', '普通文字');
    const plainEvent = new ClipboardEvent('paste', {
      bubbles: true,
      cancelable: true,
      clipboardData: plainClipboard,
    });
    input.dispatchEvent(plainEvent);
    if (plainEvent.defaultPrevented) throw new Error('Text-only paste was intercepted');
    const pastedRemove = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.attached-files button'),
    ).find((el) => el.getAttribute('aria-label')?.startsWith('移除 Clipboard_Screenshot'))!;
    pastedRemove.click();
    await waitDOM(
      () => document.querySelectorAll('.attached-files .image-attachment').length === 1,
    );
    report.clipboardScreenshot = {
      pasted: true,
      thumbnailLoaded: true,
      mixedTextPreserved: true,
      previewOpened: true,
      removed: true,
      plainTextNativePaste: true,
    };
    button('summary[aria-label="更多任务选项"]').click();
    await waitDOM(() => !!document.querySelector('.advanced-menu[open]'));
    const dispatch = Array.from(document.querySelectorAll('.team-dispatch-action button'))[0];
    (dispatch as HTMLButtonElement).click();
    await waitDOM(() => !!document.querySelector('select[aria-label="协作规划方式"]'));
    const planningSelect = document.querySelector<HTMLSelectElement>(
      'select[aria-label="协作规划方式"]',
    )!;
    planningSelect.value = 'manual';
    planningSelect.dispatchEvent(new Event('change', { bubbles: true }));
    await waitDOM(() => document.querySelectorAll('.team-member-editors article').length === 3);
    button('button[aria-label="关闭子任务派发"]').click();
    const filesTab = Array.from(document.querySelectorAll('.sidebar-mode-tabs button')).find(
      (el) => el.textContent === '文件',
    );
    (filesTab as HTMLButtonElement).click();
    await waitDOM(() => !!document.querySelector('.sidebar-file'));
    report.sessionsAndAgents = {
      groupStatus: completed.status,
      memberCount: completed.members.length,
      independentTaskCount: completed.members.reduce((count, m) => count + m.taskIds.length, 0),
      groupPanelRendered: true,
      historyRendered: true,
      materialsPickerRendered: true,
      dispatchDialogRendered: true,
      filesTabRendered: true,
      imageDropUploaded: true,
      nativeFolderBinding: typeof native.PickFolder === 'function',
    };
    const preference = await post<Memory>('/memories', {
      projectId: state.projects[0].id,
      scope: 'project',
      kind: 'preference',
      title: '桌面偏好自检',
      content: '输出使用简洁中文，先结论后依据。',
      keywords: '',
      enabled: true,
    });
    await hooks.refresh();
    report.phase = 'memory-editor';
    hooks.navigate('memory');
    await waitDOM(
      () => document.querySelector('.memory-list')?.textContent?.includes('桌面偏好自检') === true,
    );
    button('button[aria-label="编辑 桌面偏好自检"]').click();
    await waitDOM(() => !!document.querySelector('[aria-label="编辑记忆"] textarea'));
    const setValue = (control: HTMLInputElement | HTMLTextAreaElement, value: string) => {
      const prototype =
        control instanceof HTMLTextAreaElement
          ? HTMLTextAreaElement.prototype
          : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(prototype, 'value')!.set!.call(control, value);
      control.dispatchEvent(new Event('input', { bubbles: true }));
    };
    setValue(
      document.querySelector<HTMLTextAreaElement>('[aria-label="编辑记忆"] textarea')!,
      '输出使用简洁中文，保留关键数字，先结论后依据。',
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    button('[aria-label="编辑记忆"] button[type="submit"]').click();
    await waitDOM(
      () =>
        !document.querySelector('[aria-label="编辑记忆"]') &&
        document.querySelector('.memory-list')?.textContent?.includes('保留关键数字') === true,
    );
    report.phase = 'schedule-form';
    hooks.navigate('schedules');
    await waitDOM(() => !!document.querySelector('[data-testid="schedule-page"]'));
    button('.schedule-page .page-title button').click();
    await waitDOM(() => !!document.querySelector('[aria-label="编辑定时任务"]'));
    const reminderMode = Array.from(document.querySelectorAll('.schedule-mode button')).find(
      (el) => el.textContent === '到时提醒',
    );
    (reminderMode as HTMLButtonElement).click();
    await new Promise((resolve) => requestAnimationFrame(resolve));
    setValue(
      document.querySelector<HTMLInputElement>('[aria-label="编辑定时任务"] input')!,
      '桌面提醒自检',
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    setValue(
      document.querySelector<HTMLTextAreaElement>('[aria-label="编辑定时任务"] textarea')!,
      '核对项目进展并检查周报。',
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    button('[aria-label="编辑定时任务"] button[type="submit"]').click();
    await waitDOM(
      () =>
        document.querySelector('.schedule-list')?.textContent?.includes('桌面提醒自检') === true,
    );
    const planned = (await api<State>('/state')).schedules.find((p) => p.name === '桌面提醒自检');
    if (!planned) throw new Error('Reminder schedule did not persist');
    button('.schedule-actions button').click();
    let reminderRun: ScheduleRun | undefined;
    for (let i = 0; i < 100; i++) {
      reminderRun = (await api<State>('/state')).scheduleRuns.find(
        (r) => r.scheduleId === planned.id,
      );
      if (reminderRun?.status === 'succeeded') break;
      await new Promise((resolve) => setTimeout(resolve, 70));
    }
    if (reminderRun?.status !== 'succeeded') throw new Error('Scheduled reminder did not execute');
    await hooks.refresh();
    report.phase = 'notification-inbox';
    button('button[aria-label="打开提醒中心"]').click();
    await waitDOM(
      () =>
        document.querySelector('.reminder-list')?.textContent?.includes('核对项目进展') === true,
    );
    const markAll = Array.from(document.querySelectorAll('.reminder-heading button')).find((el) =>
      el.textContent?.includes('全部已读'),
    );
    (markAll as HTMLButtonElement).click();
    await waitDOM(
      () => document.querySelectorAll('.reminder-list article:not(.read)').length === 0,
    );
    button('button[aria-label="关闭提醒中心"]').click();
    report.phase = 'scheduled-document';
    const documentPlan = await post<Schedule>('/schedules', {
      projectId: state.projects[0].id,
      name: '桌面计划文档自检',
      mode: 'task',
      prompt: '生成独立计划会话文档',
      kind: 'docx',
      roleId: 'docx',
      frequency: 'once',
      timezone: 'Asia/Shanghai',
      startDate: '2028-01-01',
      clock: '09:00',
      missedPolicy: 'skip',
      useMemory: true,
    });
    const executed = await post<ScheduleRun>(`/schedules/${documentPlan.id}/run`);
    let executedTask: Task | undefined;
    for (let i = 0; i < 100; i++) {
      executedTask = (await api<State>('/state')).tasks.find((t) => t.id === executed.taskId);
      if (executedTask?.status === 'succeeded') break;
      await new Promise((resolve) => setTimeout(resolve, 70));
    }
    if (
      executedTask?.status !== 'succeeded' ||
      executedTask.conversationId !== executedTask.id ||
      executedTask.parentId ||
      executedTask.configSnapshot?.memoryRefs?.[0]?.id !== preference.id
    )
      throw new Error('Scheduled task lost independent conversation or memory snapshot');
    await hooks.refresh();
    hooks.navigate('memory');
    await waitDOM(() => !!document.querySelector('.memory-list'));
    button('button[aria-label="停用 桌面偏好自检"]').click();
    await waitDOM(() => !!document.querySelector('button[aria-label="启用 桌面偏好自检"]'));
    report.schedulesAndMemory = {
      memoryEditorSaved: true,
      memoryDisabled: true,
      reminderFormSaved: true,
      reminderExecuted: true,
      notificationRenderedAndRead: true,
      independentConversation: true,
      memorySnapshot: true,
      directoryPickerBinding: typeof native.PickScheduleDirectory === 'function',
    };
    report.phase = 'conversation-delete';
    hooks.navigate('workspace');
    await waitDOM(() => !!document.querySelector('.compact-composer'));
    const sessionsTab = Array.from(document.querySelectorAll('.sidebar-mode-tabs button')).find(
      (el) => el.textContent === '会话',
    );
    (sessionsTab as HTMLButtonElement).click();
    await waitDOM(() =>
      Array.from(document.querySelectorAll('.history-delete')).some((el) =>
        el.getAttribute('aria-label')?.includes('桌面子任务自检'),
      ),
    );
    const deleteFixture = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.history-delete'),
    ).find((el) => el.getAttribute('aria-label')?.includes('桌面子任务自检'));
    if (!deleteFixture) throw new Error('Conversation delete action missing');
    deleteFixture.click();
    await waitDOM(() => !!document.querySelector('[aria-label="删除会话"]'));
    button('button[aria-label="取消删除"]').click();
    await waitDOM(() => !document.querySelector('[aria-label="删除会话"]'));
    if (!(await api<State>('/state')).tasks.some((item) => item.id === t.id))
      throw new Error('Cancel delete removed task');
    deleteFixture.click();
    await waitDOM(() => !!document.querySelector('.delete-conversation-dialog .danger'));
    button('.delete-conversation-dialog .danger').click();
    await waitDOM(() => !document.querySelector('.delete-conversation-dialog'));
    const afterDelete = await api<State>('/state');
    if (
      afterDelete.tasks.some((item) => item.id === t.id) ||
      !afterDelete.assets.some((a) => a.id === task.outputIds[0])
    )
      throw new Error('Conversation deletion did not preserve project file');
    report.conversationDelete = { cancelPreserved: true, deleted: true, filesPreserved: true };
    report.phase = 'text-artifacts';
    const markdown =
      '# 桌面 Markdown 自检\n\n| 项目 | 状态 |\n| --- | --- |\n| 预览 | 完成 |\n\n```js\nconsole.log("中文");\n```\n';
    const page = `旧版前置说明\n\`\`\`html\n<!doctype html><html><head><meta charset="utf-8"></head><body><button id="sum">2+3</button><output id="result"></output><script>
      document.getElementById('sum').onclick=()=>document.getElementById('result').textContent=String(2+3);
      document.getElementById('sum').click();
      let isolated=false;try{parent.document.body}catch(e){isolated=true}
      fetch('/api/state').then(()=>false).catch(()=>true).then(blocked=>parent.postMessage({channel:'opsbuddy-preview-smoke',result:document.getElementById('result').textContent,isolated,blocked,bridge:typeof window.go!=='undefined',clean:!document.body.innerText.includes('旧版前置说明')&&!document.body.innerText.includes('旧版后置说明')},'*'));
    </script></body></html>\n\`\`\`\n旧版后置说明`;
    let previewReceipt:
      | { result: string; isolated: boolean; blocked: boolean; bridge: boolean; clean: boolean }
      | undefined;
    const receive = (event: MessageEvent) => {
      const frame = document.querySelector<HTMLIFrameElement>('iframe[title="网页交互预览"]');
      if (event.source === frame?.contentWindow && event.data?.channel === 'opsbuddy-preview-smoke')
        previewReceipt = event.data;
    };
    window.addEventListener('message', receive);
    const textFiles = new FormData();
    textFiles.append('projectId', state.projects[0].id);
    textFiles.append('files', new File([markdown], '桌面预览自检.md', { type: 'text/markdown' }));
    textFiles.append('files', new File([page], '桌面交互自检.html', { type: 'text/html' }));
    const textAssets = await api<Asset[]>('/assets', { method: 'POST', body: textFiles });
    await hooks.refresh();
    const textFilesTab = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.sidebar-mode-tabs button'),
    ).find((el) => el.textContent === '文件')!;
    textFilesTab.click();
    await waitDOM(() => !!document.querySelector('.sidebar-file[title="桌面预览自检.md"]'));
    button('.sidebar-file[title="桌面预览自检.md"]').click();
    await waitDOM(
      () =>
        !!document.querySelector('.markdown-document table') &&
        !!document.querySelector('.markdown-document h1') &&
        !!document.querySelector('.markdown-document pre code'),
    );
    const edit = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.text-artifact-tabs button'),
    ).find((el) => el.textContent?.includes('源码编辑'))!;
    edit.click();
    await waitDOM(() => !!document.querySelector('[aria-label="文件源码"]'));
    setValue(
      document.querySelector<HTMLTextAreaElement>('[aria-label="文件源码"]')!,
      markdown + '\n新增修订内容。',
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    button('.text-artifact footer button').click();
    await waitDOM(
      () =>
        document.querySelector('.markdown-document')?.textContent?.includes('新增修订内容') ===
        true,
    );
    const mdAsset = textAssets.find((a) => a.kind === 'md')!;
    const versions = (await api<State>('/state')).assets.filter((a) => a.parentId === mdAsset.id);
    const original = await api<{ content: string }>(`/assets/${mdAsset.id}/text`);
    if (versions.length !== 1 || original.content !== markdown)
      throw new Error('Text version did not preserve original');
    button('.sidebar-file[title="桌面交互自检.html"]').click();
    await waitDOM(() => !!previewReceipt);
    report.htmlPreviewReceipt = previewReceipt;
    if (
      previewReceipt!.result !== '5' ||
      !previewReceipt!.isolated ||
      !previewReceipt!.blocked ||
      previewReceipt!.bridge ||
      !previewReceipt!.clean
    )
      throw new Error('HTML interaction or isolation failed');
    if (!previewReceipt!.clean) throw new Error('Legacy commentary leaked into page');
    const initialWidth = document.querySelector('.artifact-panel')!.getBoundingClientRect().width;
    button('button[aria-label="展开成果预览"]').click();
    await waitDOM(() => !!document.querySelector('.artifact-panel.expanded'));
    if (document.querySelector('.artifact-panel')!.getBoundingClientRect().width <= initialWidth)
      throw new Error('Preview did not expand');
    button('button[aria-label="收起成果预览"]').click();
    window.removeEventListener('message', receive);
    report.textArtifacts = {
      markdownRendered: true,
      sourceEditorSaved: true,
      originalPreserved: true,
      htmlCalculation: true,
      legacyHTMLCommentaryRemoved: true,
      previewExpanded: true,
      isolated: true,
      networkBlocked: true,
    };
    const frameRect = document
      .querySelector('iframe[title="网页交互预览"]')!
      .getBoundingClientRect();
    const studioRect = document.querySelector('.artifact-panel')!.getBoundingClientRect();
    if (frameRect.height < studioRect.height * 0.6)
      throw new Error('HTML preview lost too much height to controls');
    button('button[aria-label="关闭成果预览"]').click();
    await waitDOM(() => !document.querySelector('.artifact-panel'));
    button('button[aria-label="打开右侧栏"]').click();
    await waitDOM(() => !!document.querySelector('iframe[title="网页交互预览"]'));
    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        code: 'KeyB',
        key: 'B',
        ctrlKey: true,
        shiftKey: true,
        bubbles: true,
      }),
    );
    await waitDOM(() => !document.querySelector('.artifact-panel'));
    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        code: 'KeyB',
        key: 'B',
        ctrlKey: true,
        shiftKey: true,
        bubbles: true,
      }),
    );
    await waitDOM(() => !!document.querySelector('.artifact-panel'));
    const browser = document.querySelector<HTMLSelectElement>('[aria-label="右侧文件范围"]')!;
    browser.value = 'project';
    browser.dispatchEvent(new Event('change', { bubbles: true }));
    await new Promise((resolve) => requestAnimationFrame(resolve));
    await waitDOM(() => !!document.querySelector('button[aria-label="打开文件 桌面预览自检.md"]'));
    button('button[aria-label="打开文件 桌面预览自检.md"]').click();
    await waitDOM(() => !!document.querySelector('.markdown-document'));
    const filter = document.querySelector<HTMLInputElement>('[aria-label="搜索预览文件"]')!;
    setValue(filter, '桌面交互');
    await new Promise((resolve) => requestAnimationFrame(resolve));
    await waitDOM(() => document.querySelectorAll('.artifact-file-items > button').length === 1);
    button('button[aria-label="打开文件 桌面交互自检.html"]').click();
    await waitDOM(() => !!document.querySelector('iframe[title="网页交互预览"]'));
    const separator = document.querySelector('[aria-label="调整右侧栏宽度"]')!;
    separator.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    await waitDOM(() => !!localStorage.getItem('opsbuddy-artifact-width'));
    button('button[aria-label="隐藏预览文件列表"]').click();
    await waitDOM(() => !document.querySelector('.artifact-file-browser'));
    button('button[aria-label="显示预览文件列表"]').click();
    await waitDOM(() => !!document.querySelector('.artifact-file-browser'));
    report.rightSidebar = {
      toolbarToggle: true,
      restoresSelectedFile: true,
      shortcut: true,
      fileSwitching: true,
      fileSearch: true,
      widthSaved: true,
      fileListToggle: true,
      htmlHeightRatio: frameRect.height / studioRect.height,
    };
    button('button[aria-label="关闭成果预览"]').click();
    (sessionsTab as HTMLButtonElement).click();
    report.phase = 'model-default';
    const modelFixture = await post<Model>('/models', {
      name: '默认模型自检',
      family: 'openai',
      baseUrl: 'https://example.invalid',
      modelId: 'smoke-model',
      apiKey: '',
    });
    await hooks.refresh();
    await waitDOM(
      () =>
        document.querySelector<HTMLSelectElement>('select[aria-label="模型"]')?.value ===
        modelFixture.id,
    );
    if (document.querySelector('select[aria-label="模型"] option[value=""]'))
      throw new Error('Local mode remains in model picker');
    const existingSession = document.querySelector<HTMLButtonElement>(
      '.sidebar-history .history-item',
    );
    existingSession?.click();
    await new Promise((resolve) => requestAnimationFrame(resolve));
    if (
      document.querySelector<HTMLSelectElement>('select[aria-label="模型"]')?.value !==
      modelFixture.id
    )
      throw new Error('Opening local history reset selected model');
    report.modelDefault = {
      autoSelected: true,
      noLocalOption: true,
      preservedOnHistory: true,
      persisted: localStorage.getItem('opsbuddy-selected-model') === modelFixture.id,
    };
    const imageOnlyClipboard = new DataTransfer();
    imageOnlyClipboard.items.add(
      new File([fixtureBlob], 'only-screenshot.png', { type: 'image/png' }),
    );
    const finalInput = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="任务需求"]',
    )!;
    setValue(finalInput, '');
    await new Promise((resolve) => requestAnimationFrame(resolve));
    finalInput.dispatchEvent(
      new ClipboardEvent('paste', {
        bubbles: true,
        cancelable: true,
        clipboardData: imageOnlyClipboard,
      }),
    );
    await waitDOM(
      () =>
        document.querySelectorAll('.attachment-image img').length > 0 &&
        !document.querySelector<HTMLButtonElement>('button[aria-label="开始任务"]')?.disabled,
    );
    report.imageOnlyMessageReady = true;
    button('button[aria-label="新建任务"]').click();
    await waitDOM(() => !!document.querySelector('.dashboard-welcome'));
    if (document.querySelector('.model-setup-note'))
      throw new Error('Model setup prompt remains after configuration');
    report.configuredWorkspace = { setupPromptHidden: true };
    const keyboardInput = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="任务需求"]',
    )!;
    const keyboardForm = keyboardInput.form!;
    let keyboardSubmits = 0;
    const captureKeyboardSubmit = (event: Event) => {
      event.preventDefault();
      event.stopImmediatePropagation();
      keyboardSubmits++;
    };
    keyboardForm.addEventListener('submit', captureKeyboardSubmit, true);
    try {
      setValue(keyboardInput, '回车发送自检');
      await new Promise((resolve) => requestAnimationFrame(resolve));
      const key = (options: KeyboardEventInit = {}) => {
        const event = new KeyboardEvent('keydown', {
          key: 'Enter',
          bubbles: true,
          cancelable: true,
          ...options,
        });
        keyboardInput.dispatchEvent(event);
        return event;
      };
      if (!key().defaultPrevented || Number(keyboardSubmits) !== 1)
        throw new Error('Enter did not submit');
      if (key({ shiftKey: true }).defaultPrevented || Number(keyboardSubmits) !== 1)
        throw new Error('Shift Enter did not preserve newline');
      if (key({ isComposing: true }).defaultPrevented || Number(keyboardSubmits) !== 1)
        throw new Error('IME confirmation submitted');
      const ime229 = new KeyboardEvent('keydown', {
        key: 'Enter',
        bubbles: true,
        cancelable: true,
      });
      Object.defineProperty(ime229, 'keyCode', { value: 229 });
      keyboardInput.dispatchEvent(ime229);
      if (ime229.defaultPrevented || Number(keyboardSubmits) !== 1)
        throw new Error('IME keyCode 229 submitted');
      if (!key({ repeat: true }).defaultPrevented || Number(keyboardSubmits) !== 1)
        throw new Error('Held Enter repeated send');
      key({ ctrlKey: true });
      if (Number(keyboardSubmits) !== 2) throw new Error('Ctrl Enter compatibility lost');
      setValue(keyboardInput, '');
      await new Promise((resolve) => requestAnimationFrame(resolve));
      key();
      if (Number(keyboardSubmits) !== 2) throw new Error('Empty message submitted by Enter');
      report.enterToSend = {
        enter: true,
        shiftNewline: true,
        imeSafe: true,
        repeatIgnored: true,
        ctrlEnter: true,
        emptyBlocked: true,
      };
    } finally {
      keyboardForm.removeEventListener('submit', captureKeyboardSubmit, true);
    }
    report.phase = 'backend-project';
    const backendProject = await post<Asset>('/code-projects', {
      projectId: state.projects[0].id,
      project: {
        title: '桌面后端运行自检',
        runtime: 'node',
        mode: 'server',
        entry: 'server.mjs',
        files: [
          {
            path: 'server.mjs',
            content: `import http from 'node:http';
console.log('desktop-backend-ready');
http.createServer((req,res)=>{
if(req.url==='/api'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({value:42}));return;}
res.setHeader('Content-Type','text/html; charset=utf-8');
res.end('<!doctype html><html><head></head><body><button id="counter">0</button><script>parent.postMessage({opsbuddyBackendLoaded:true},"*");fetch("./api").then(r=>r.json()).then(d=>{document.querySelector("#counter").textContent=d.value;parent.postMessage({opsbuddyBackend:d.value,bridge:!!window.go},"*")}).catch(e=>parent.postMessage({opsbuddyBackendError:e.message},"*"));document.querySelector("#counter").onclick=e=>{e.target.textContent=Number(e.target.textContent)+1;parent.postMessage({opsbuddyBackendClick:Number(e.target.textContent)},"*")};addEventListener("message",e=>{if(e.source===parent&&e.data?.opsbuddyClick)document.querySelector("#counter").click()});</script></body></html>');
}).listen(Number(process.env.PORT),'127.0.0.1');`,
          },
        ],
      },
    });
    report.backendPhase = 'created';
    let backendMessage: { value: number; bridge: boolean } | undefined;
    let backendClick = 0;
    const backendListener = (e: MessageEvent) => {
      const frame = document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]');
      if (frame?.contentWindow !== e.source) return;
      report.backendLastMessage = e.data;
      if (e.data?.opsbuddyBackend)
        backendMessage = { value: e.data.opsbuddyBackend, bridge: e.data.bridge };
      if (e.data?.opsbuddyBackendClick) backendClick = e.data.opsbuddyBackendClick;
    };
    window.addEventListener('message', backendListener);
    await hooks.refresh();
    (filesTab as HTMLButtonElement).click();
    await waitDOM(() => !!document.querySelector('.sidebar-file[title="桌面后端运行自检"]'));
    button('.sidebar-file[title="桌面后端运行自检"]').click();
    await waitDOM(() => !!document.querySelector('.code-project-preview .code-runtime-bar button'));
    report.backendPhase = 'starting';
    await waitDOM(() => !!document.querySelector('iframe[title="项目运行预览"]'));
    report.backendPhase = 'iframe-created';
    report.backendFrameURL =
      document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]')?.src;
    await waitDOM(() => !!backendMessage);
    if (backendMessage!.value !== 42 || backendMessage!.bridge)
      throw new Error('Backend preview request or bridge isolation failed');
    // Ask the isolated page itself to click its button: cross-origin DOM access remains blocked.
    // The fixture listens only in explicit smoke mode, never in generated projects.
    const backendFrame = document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]')!;
    if (
      !backendFrame.getAttribute('sandbox')?.includes('allow-scripts') ||
      backendFrame.getAttribute('sandbox')?.includes('allow-same-origin')
    )
      throw new Error('Backend preview shares desktop origin');
    backendFrame.contentWindow!.postMessage({ opsbuddyClick: true }, '*');
    await waitDOM(() => backendClick === 43);
    const projectTabs = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.code-project-preview [role="tab"]'),
    );
    projectTabs.find((b) => b.textContent?.includes('项目文件'))!.click();
    await waitDOM(() => !!document.querySelector('[aria-label="项目源码"]'));
    if (
      !document
        .querySelector<HTMLTextAreaElement>('[aria-label="项目源码"]')
        ?.value.includes('createServer')
    )
      throw new Error('Backend project files unavailable');
    projectTabs.find((b) => b.textContent?.includes('运行日志'))!.click();
    await waitDOM(
      () =>
        document
          .querySelector('[aria-label="项目运行日志"]')
          ?.textContent?.includes('desktop-backend-ready') === true,
    );
    button('.code-runtime-bar button').click();
    await waitDOM(() => document.querySelector('.code-runtime-state')?.textContent === '未运行');
    const stopped = await api<{ status: string; url?: string }>(
      `/assets/${backendProject.id}/runtime`,
    );
    if (stopped.status !== 'stopped' || stopped.url) throw new Error('Backend stop failed');
    window.removeEventListener('message', backendListener);
    report.backendProject = {
      serviceStarted: true,
      iframeRendered: true,
      realAPIValue: backendMessage!.value,
      clickResult: backendClick,
      desktopBridgeAbsent: true,
      sourceFiles: true,
      logs: true,
      stopped: true,
    };
    report.phase = 'vue-offline-project';
    const vueDefinition = {
      title: 'Vue 与 Go 离线运行自检',
      runtime: 'go',
      mode: 'server',
      entry: 'main.go',
      files: [
        { path: 'go.mod', content: 'module vuecheck\n\ngo 1.22\n' },
        {
          path: 'main.go',
          content:
            'package main\nimport("net/http";"os")\nfunc main(){http.HandleFunc("/api",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");w.Write([]byte(`{"value":42}`))});http.Handle("/",http.FileServer(http.Dir(".")));http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"),nil)}',
        },
        {
          path: 'index.html',
          content: `<!doctype html><html><head><meta charset="utf-8"><script src="https://unpkg.com/vue@3.4.27/dist/vue.global.prod.js"></script></head><body><div id="app"><button id="vue-counter" @click="count++;report()">{{count}}</button></div><script>Vue.createApp({data(){return {count:0}},methods:{report(){this.$nextTick(()=>parent.postMessage({opsbuddyVue:this.count,bridge:!!window.go},'*'))}},mounted(){fetch('./api').then(r=>r.json()).then(d=>{this.count=d.value;this.report()})}}).mount('#app');addEventListener('message',e=>{if(e.source===parent&&e.data?.opsbuddyVueClick)document.querySelector('#vue-counter').click()});</script></body></html>`,
        },
      ],
    };
    const vueAsset = await post<Asset>('/code-projects', {
      projectId: state.projects[0].id,
      project: vueDefinition,
    });
    if (vueAsset.name.endsWith('.zip')) throw new Error('Code project still displayed as archive');
    let vueValue = 0,
      vueBridge = false;
    const vueListener = (event: MessageEvent) => {
      const frame = document.querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]');
      if (event.source === frame?.contentWindow && event.data?.opsbuddyVue) {
        vueValue = event.data.opsbuddyVue;
        vueBridge = event.data.bridge;
      }
    };
    window.addEventListener('message', vueListener);
    await hooks.refresh();
    await waitDOM(() => !!document.querySelector(`.sidebar-file[title="${vueAsset.name}"]`));
    button(`.sidebar-file[title="${vueAsset.name}"]`).click();
    await waitDOM(() => vueValue === 42, 30000);
    if (vueBridge || document.querySelector('.code-page-error'))
      throw new Error('Vue preview failed or exposed desktop bridge');
    document
      .querySelector<HTMLIFrameElement>('iframe[title="项目运行预览"]')!
      .contentWindow!.postMessage({ opsbuddyVueClick: true }, '*');
    await waitDOM(() => vueValue === 43);
    const vueClickResult = vueValue;
    const vueTabs = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.code-project-preview [role="tab"]'),
    );
    vueTabs.find((b) => b.textContent?.includes('项目文件'))!.click();
    await waitDOM(() => document.querySelectorAll('.code-file-tree button').length === 3);
    button('.code-file-tree button[title="index.html"]').click();
    await waitDOM(
      () =>
        document
          .querySelector<HTMLTextAreaElement>('[aria-label="项目源码"]')
          ?.value.includes('__opsbuddy_runtime__/vue.global.prod.js') === true,
    );
    const download = await fetch(`/api/assets/${vueAsset.id}/file`);
    const archive = new Uint8Array(await download.arrayBuffer());
    if (
      archive[0] !== 80 ||
      archive[1] !== 75 ||
      !download.headers.get('Content-Disposition')?.includes('.zip')
    )
      throw new Error('Explicit ZIP export failed');
    const badAsset = await post<Asset>(`/assets/${vueAsset.id}/code`, {
      ...vueDefinition,
      title: '页面错误提示自检',
      files: vueDefinition.files.map((f) =>
        f.path === 'index.html'
          ? {
              ...f,
              content: f.content.replace(
                '</body>',
                '<script>throw new Error("preview-error-fixture")</script></body>',
              ),
            }
          : f,
      ),
    });
    await hooks.refresh();
    await waitDOM(() => !!document.querySelector(`.sidebar-file[title="${badAsset.name}"]`));
    button(`.sidebar-file[title="${badAsset.name}"]`).click();
    await waitDOM(
      () =>
        document
          .querySelector('.code-page-error')
          ?.textContent?.includes('preview-error-fixture') === true,
      30000,
    );
    await post(`/assets/${badAsset.id}/runtime/stop`);
    await post(`/assets/${vueAsset.id}/runtime/stop`);
    window.removeEventListener('message', vueListener);
    report.vuePreview = {
      offlineRuntime: true,
      goAPIValue: 42,
      vueClickResult,
      sourceTree: true,
      noZipByDefault: true,
      explicitZipExport: true,
      pageErrorVisible: true,
      desktopBridgeAbsent: true,
    };
    report.phase = 'builtin-skills';
    hooks.navigate('extensions');
    await waitDOM(() => document.querySelectorAll('.extension-list [data-skill-id]').length === 12);
    setValue(document.querySelector<HTMLInputElement>('[aria-label="搜索技能"]')!, '后端');
    await waitDOM(() => document.querySelectorAll('.extension-list article').length === 1);
    const builtinRow = '[data-skill-id="builtin-skill-backend-development"]';
    button(`${builtinRow} .skill-instructions summary`).click();
    if (
      !document
        .querySelector(`${builtinRow} .skill-instructions[open]`)
        ?.textContent?.includes('PORT')
    )
      throw new Error('Builtin instructions unavailable');
    if (document.querySelector(`${builtinRow} button[title="删除扩展"]`))
      throw new Error('Builtin delete exposed');
    button(`${builtinRow} button[role="switch"]`).click();
    await waitDOM(
      () =>
        document
          .querySelector(`${builtinRow} button[role="switch"]`)
          ?.getAttribute('aria-checked') === 'false',
    );
    button(`${builtinRow} button[title="复制为自定义技能"]`).click();
    await waitDOM(() => !!document.querySelector('.modal input[name="name"]'));
    const copyName = document.querySelector<HTMLInputElement>('.modal input[name="name"]')!.value;
    if (
      !copyName.includes('副本') ||
      !document
        .querySelector<HTMLTextAreaElement>('.modal textarea[name="content"]')
        ?.value.includes('name: backend-development')
    )
      throw new Error('Builtin clone did not preserve SKILL.md');
    button('.modal button[type="submit"]').click();
    await waitDOM(() => !document.querySelector('.modal'));
    const skillsState = await api<State>('/state');
    const customCopy = skillsState.extensions.find((x) => x.name === copyName);
    if (
      !customCopy ||
      customCopy.builtin ||
      !customCopy.content?.includes('PORT') ||
      skillsState.extensions.find((x) => x.id === 'builtin-skill-backend-development')?.enabled !==
        false
    )
      throw new Error('Builtin toggle or editable copy failed');
    report.builtinSkills = {
      count: 12,
      searchable: true,
      instructions: true,
      disabled: true,
      protected: true,
      editableCopy: true,
    };
    report.phase = 'project-guidance';
    hooks.navigate('projects');
    await waitDOM(() => !!document.querySelector('.project-edit'));
    button('.project-edit').click();
    await waitDOM(() => !!document.querySelector('.modal textarea[name="description"]'));
    setValue(
      document.querySelector<HTMLTextAreaElement>('.modal textarea[name="description"]')!,
      '桌面项目说明自检：内容简洁，保留关键数字。',
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    button('.modal button[type="submit"]').click();
    await waitDOM(() => !document.querySelector('.modal'));
    const projectState = await api<State>('/state');
    if (!projectState.projects[0].description.includes('桌面项目说明自检'))
      throw new Error('Project guidance edit failed');
    report.projectGuidance = { editable: true, saved: true };
    report.phase = 'project-conversations';
    const projectB = await post<{ id: string }>('/projects', {
      name: '边界项目B',
      description: 'B 独立项目',
    });
    const projectA = state.projects[0];
    const fixtureTasks: Task[] = [];
    for (const [projectId, prompt] of [
      [projectA.id, '边界A会话'],
      [projectB.id, '边界B会话'],
    ]) {
      const created = await post<Task>('/tasks', {
        projectId,
        prompt,
        kind: 'docx',
        roleId: 'docx',
      });
      for (let i = 0; i < 100; i++) {
        const done = (await api<State>('/state')).tasks.find((item) => item.id === created.id);
        if (done?.status === 'succeeded') {
          fixtureTasks.push(done);
          break;
        }
        if (done?.status === 'failed') throw new Error(done.error);
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
    }
    if (fixtureTasks.length !== 2) throw new Error('Project conversation fixtures did not finish');
    await hooks.refresh();
    (sessionsTab as HTMLButtonElement).click();
    const groupA = `.project-session-group[data-project-id="${projectA.id}"]`;
    const groupB = `.project-session-group[data-project-id="${projectB.id}"]`;
    await waitDOM(() => !!document.querySelector(groupB));
    button(`${groupA} .project-session-name`).click();
    await waitDOM(() => !!document.querySelector(`${groupA} .history-item`));
    const sessionButton = (group: string, title: string) =>
      Array.from(document.querySelectorAll<HTMLButtonElement>(`${group} .history-item`)).find(
        (item) => item.textContent?.includes(title),
      )!;
    sessionButton(groupA, '边界A会话').click();
    await waitDOM(
      () =>
        !!document.querySelector('.task-content') && !!document.querySelector('.artifact-panel'),
    );
    setValue(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="任务需求"]')!,
      'A 项目未发送的草稿',
    );
    button(`${groupB} .project-session-name`).click();
    await waitDOM(
      () =>
        document.querySelector<HTMLSelectElement>('[aria-label="当前项目"]')?.value ===
          projectB.id && !document.querySelector('.task-content'),
    );
    if (
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="任务需求"]')!.value ||
      document.querySelector('.attached-files') ||
      document.querySelector('.artifact-panel')
    )
      throw new Error('Switching projects retained old draft or preview');
    if (
      !document
        .querySelector(`${groupA} .project-session-items`)
        ?.textContent?.includes('边界A会话') ||
      document.querySelector(`${groupB} .project-session-items`)?.textContent?.includes('边界A会话')
    )
      throw new Error('Project tree mixed conversations');
    sessionButton(groupB, '边界B会话').click();
    await waitDOM(() => !!document.querySelector('.task-content'));
    if (
      !document.querySelector('.task-content')?.textContent?.includes('边界B会话') ||
      document.querySelector('.task-content')?.textContent?.includes('边界A会话') ||
      document.querySelector('.conversation-history')
    )
      throw new Error('Opening B exposed A history');
    button(`${groupA} .project-session-new`).click();
    await waitDOM(
      () =>
        document.querySelector<HTMLSelectElement>('[aria-label="当前项目"]')?.value === projectA.id,
    );
    const racingFile = new DataTransfer();
    racingFile.items.add(
      new File(['A private upload'], '项目A异步上传.txt', { type: 'text/plain' }),
    );
    document
      .querySelector('.composer')!
      .dispatchEvent(
        new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: racingFile }),
      );
    // Switch before the pending upload resolves; the response must not attach to B.
    button(`${groupB} .project-session-new`).click();
    let uploadFinished = false;
    for (let i = 0; i < 100; i++) {
      const current = await api<State>('/state');
      uploadFinished = current.assets.some(
        (a) => a.projectId === projectA.id && (a.sourceName || a.name) === '项目A异步上传.txt',
      );
      if (uploadFinished) break;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    await waitDOM(() => !document.querySelector('.composer button[aria-label="开始任务"] .spin'));
    await new Promise((resolve) => setTimeout(resolve, 100));
    if (!uploadFinished || document.querySelector('.attached-files'))
      throw new Error('Late A upload attached to B or lost its project');
    (filesTab as HTMLButtonElement).click();
    await waitDOM(() => !!document.querySelector('.sidebar-file'));
    if (document.querySelector('.sidebar-history')?.textContent?.includes('项目A异步上传'))
      throw new Error('B file list contains A files');
    (sessionsTab as HTMLButtonElement).click();
    setValue(document.querySelector<HTMLInputElement>('[aria-label="搜索任务"]')!, '边界A会话');
    await waitDOM(
      () => !!document.querySelector(`${groupA} .history-item`) && !document.querySelector(groupB),
    );
    setValue(document.querySelector<HTMLInputElement>('[aria-label="搜索任务"]')!, '');
    report.projectConversations = {
      groupedByProject: true,
      openSwitchesProject: true,
      newInProject: true,
      draftAndPreviewCleared: true,
      lateUploadIsolated: true,
      fileListScoped: true,
      searchKeepsProject: true,
    };
    report.phase = 'desktop-companion';
    hooks.navigate('settings');
    await waitDOM(() => !!document.querySelector('[aria-label="显示桌面宠物"]:not(:disabled)'));
    const petBefore = await native.PetSettings();
    if (!petBefore.enabled || !(await native.PetDiagnostics()).nativeWindow)
      throw new Error('Native companion did not start');
    button('[aria-label="显示桌面宠物"]').click();
    await waitDOM(
      () =>
        document.querySelector('[aria-label="显示桌面宠物"]')?.getAttribute('aria-checked') ===
        'false',
    );
    if ((await native.PetDiagnostics()).visible) throw new Error('Pet disable retained a window');
    button('[aria-label="显示桌面宠物"]').click();
    await waitDOM(() => !!document.querySelector('[aria-label="播放桌宠动画"]'));
    button('[aria-label="播放桌宠动画"]').click();
    await waitDOM(
      () =>
        document.querySelector<HTMLInputElement>('[aria-label="播放桌宠动画"]')?.checked === false,
    );
    const petTask = await post<Task>('/tasks', {
      projectId: projectB.id,
      kind: 'docx',
      roleId: 'docx',
      prompt: '桌宠状态联动自检',
    });
    for (let i = 0; i < 100; i++) {
      const done = (await api<State>('/state')).tasks.find((task) => task.id === petTask.id);
      if (done?.status === 'succeeded') break;
      if (done?.status === 'failed') throw new Error(done.error);
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    let petStatus = await native.PetDiagnostics();
    for (let i = 0; i < 30 && petStatus.phase !== 'done'; i++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      petStatus = await native.PetDiagnostics();
    }
    if (petStatus.phase !== 'done') throw new Error('Real task did not reach companion');
    const settleStarted = performance.now();
    let petIdle = await native.PetDiagnostics();
    while (petIdle.timerActive && performance.now() - settleStarted < 10000) {
      await new Promise((resolve) => setTimeout(resolve, 200));
      petIdle = await native.PetDiagnostics();
    }
    await new Promise((resolve) => setTimeout(resolve, 700));
    const petAfterIdle = await native.PetDiagnostics();
    report.petIdleDiagnostics = {
      before: petIdle,
      after: petAfterIdle,
      settleMs: performance.now() - settleStarted,
    };
    if (petIdle.timerActive || petAfterIdle.timerActive || petAfterIdle.renders !== petIdle.renders)
      throw new Error('Companion kept rendering while static');
    await native.ConfigurePet(petBefore);
    await new Promise((resolve) => setTimeout(resolve, 3600));
    const calmBefore = await native.PetDiagnostics();
    await new Promise((resolve) => setTimeout(resolve, 1100));
    const calmAfter = await native.PetDiagnostics();
    if (
      petBefore.animate &&
      (!calmAfter.timerActive ||
        calmAfter.renders <= calmBefore.renders ||
        calmAfter.fontPixels < 16)
    )
      throw new Error('Active companion or readable font missing');
    report.desktopCompanion = {
      nativeWindow: true,
      settingsToggle: true,
      reducedMotion: true,
      taskEvent: 'done',
      idleRepaints: 0,
      timerStopped: true,
      activeIdle: calmAfter.renders > calmBefore.renders,
      fontPixels: calmAfter.fontPixels,
      dpi: calmAfter.dpi,
    };
    report.phase = 'project-agent-collaboration';
    report.projectAgentCollaboration = await runAgentDesktopSmoke(smokeModelURL, hooks);
    report.phase = 'complete';
    report.passed = report.embeddedUI === true;
  } catch (e) {
    report.passed = false;
    report.error = (e as Error).message;
    if (report.phase === 'backend-project')
      report.backendDOM = document
        .querySelector('.code-project-preview')
        ?.textContent?.slice(0, 3000);
  }
  report.finishedAt = new Date().toISOString();
  await desktop()!.ReportSmoke(JSON.stringify(report, null, 2));
}
