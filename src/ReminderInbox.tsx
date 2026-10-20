import { useDialogFocus } from './useDialogFocus';
import { Bell, Check, X, ChevronRight } from 'lucide-react';
import { post, type State, type Task } from './api';
export default function ReminderInbox({
  state,
  onRefresh,
  onTask,
  onClose,
  onError,
}: {
  state: State;
  onRefresh: () => Promise<void>;
  onTask: (t: Task) => void;
  onClose: () => void;
  onError: (message: string) => void;
}) {
  useDialogFocus(true);
  const read = async (id: string, taskId?: string) => {
    try {
      await post(`/notifications/${id}/read`);
      await onRefresh();
      if (taskId) {
        const task = state.tasks.find((t) => t.id === taskId);
        if (task) {
          onTask(task);
          onClose();
        }
      }
    } catch (e) {
      onError((e as Error).message);
    }
  };
  return (
    <div className="module-overlay">
      <section
        className="module-dialog reminder-inbox"
        role="dialog"
        aria-modal="true"
        aria-label="提醒中心"
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose();
        }}
      >
        <header>
          <div>
            <Bell size={21} />
            <h2>提醒与计划结果</h2>
          </div>
          <button className="icon-button" aria-label="关闭提醒中心" onClick={onClose}>
            <X size={19} />
          </button>
        </header>
        <div className="reminder-heading">
          <p>到时提醒、执行结果和错过记录保存在这里。</p>
          <button className="text-button" onClick={() => read('all')}>
            <Check size={13} />
            全部已读
          </button>
        </div>
        <div className="reminder-list">
          {state.notifications
            .slice()
            .reverse()
            .slice(0, 100)
            .map((n) => (
              <article key={n.id} className={n.readAt ? 'read' : ''}>
                <span className="reminder-unread-dot" />
                <div>
                  <strong>{n.title}</strong>
                  <time>
                    {new Date(n.createdAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}
                  </time>
                  <p>{n.body}</p>
                  <footer>
                    {!n.readAt && <button onClick={() => read(n.id)}>标为已读</button>}
                    {n.taskId && (
                      <button
                        disabled={!state.tasks.some((t) => t.id === n.taskId)}
                        onClick={() => read(n.id, n.taskId)}
                      >
                        {state.tasks.some((t) => t.id === n.taskId) ? '查看会话' : '会话已删除'}
                        <ChevronRight size={12} />
                      </button>
                    )}
                  </footer>
                </div>
              </article>
            ))}
        </div>
        {!state.notifications.length && (
          <div className="assistant-empty">
            <Bell size={30} />
            <h3>还没有提醒</h3>
            <p>在定时任务里创建提醒，或等待计划交付结果。</p>
          </div>
        )}
      </section>
    </div>
  );
}
