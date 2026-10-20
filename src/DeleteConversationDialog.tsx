import { Trash2, X } from 'lucide-react';
import { useDialogFocus } from './useDialogFocus';
import type { Task } from './api';

export default function DeleteConversationDialog({
  task,
  busy,
  onCancel,
  onDelete,
}: {
  task: Task;
  busy: boolean;
  onCancel: () => void;
  onDelete: () => void;
}) {
  useDialogFocus(true);
  return (
    <div className="module-overlay">
      <section
        className="module-dialog delete-conversation-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="删除会话"
        onKeyDown={(e) => {
          if (e.key === 'Escape' && !busy) onCancel();
        }}
      >
        <header>
          <h2>删除这段会话？</h2>
          <button className="icon-button" aria-label="取消删除" disabled={busy} onClick={onCancel}>
            <X size={18} />
          </button>
        </header>
        <p className="delete-conversation-title">{task.title}</p>
        <p>
          删除此会话的全部消息及关联子任务记录，无法撤销。已导入和生成的文件会保留在项目文件中。
        </p>
        <footer>
          <button className="button" disabled={busy} onClick={onCancel}>
            取消
          </button>
          <button className="button danger" disabled={busy} onClick={onDelete}>
            <Trash2 size={15} />
            删除会话
          </button>
        </footer>
      </section>
    </div>
  );
}
