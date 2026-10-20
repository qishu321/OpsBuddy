import { useState } from 'react';
import { ChevronDown, ChevronRight, FolderOpen, Plus, Trash2 } from 'lucide-react';
import type { Project, Task } from './api';

export default function ProjectSessions({
  projects,
  tasks,
  projectId,
  taskId,
  search,
  onProject,
  onNew,
  onOpen,
  onDelete,
}: {
  projects: Project[];
  tasks: Task[];
  projectId: string;
  taskId: string;
  search: string;
  onProject: (id: string) => void;
  onNew: (id: string) => void;
  onOpen: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const query = search.trim().toLocaleLowerCase();
  const groups = projects
    .map((project) => {
      const all = tasks.filter((task) => task.projectId === project.id);
      const matches = project.name.toLocaleLowerCase().includes(query);
      return {
        project,
        all,
        visible: all.filter(
          (task) => !query || matches || task.title.toLocaleLowerCase().includes(query),
        ),
      };
    })
    .filter(
      ({ project, visible }) =>
        !query || visible.length || project.name.toLocaleLowerCase().includes(query),
    );
  return (
    <div className="project-session-tree" aria-label="按项目分组的会话">
      {groups.map(({ project, all, visible }) => {
        const current = project.id === projectId;
        const open = !!query || (expanded[project.id] ?? current);
        const regionId = `project-sessions-${project.id}`;
        return (
          <section
            className={`project-session-group${current ? ' current' : ''}`}
            data-project-id={project.id}
            key={project.id}
          >
            <div className="project-session-heading">
              <button
                className="project-session-fold"
                aria-label={`${open ? '收起' : '展开'} ${project.name} 的会话`}
                aria-expanded={open}
                aria-controls={regionId}
                onClick={() => setExpanded((prev) => ({ ...prev, [project.id]: !open }))}
              >
                {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              </button>
              <button
                className="project-session-name"
                aria-current={current ? 'true' : undefined}
                title={`切换到项目：${project.name}`}
                onClick={() => {
                  setExpanded((prev) => ({ ...prev, [project.id]: true }));
                  if (!current) onProject(project.id);
                }}
              >
                <FolderOpen size={16} />
                <span>{project.name}</span>
                <small>{all.length}</small>
              </button>
              <button
                className="project-session-new"
                aria-label={`在 ${project.name} 新建会话`}
                title="在此项目新建会话"
                onClick={() => {
                  setExpanded((prev) => ({ ...prev, [project.id]: true }));
                  onNew(project.id);
                }}
              >
                <Plus size={14} />
              </button>
            </div>
            {open && (
              <div className="project-session-items" id={regionId}>
                {visible.map((task) => (
                  <div className="history-row" key={task.id}>
                    <button
                      className={`history-item ${taskId === task.id && current ? 'selected' : ''}`}
                      title={`${project.name} · ${task.title}`}
                      onClick={() => onOpen(task)}
                    >
                      <span className={`tiny-dot ${task.status}`} />
                      <span>{task.title}</span>
                    </button>
                    <button
                      className="history-delete"
                      aria-label={`删除会话 ${task.title}`}
                      title="删除会话"
                      onClick={() => onDelete(task)}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
                {!visible.length && (
                  <p className="project-session-empty">
                    {query ? '没有匹配的会话' : '暂无会话，点击 + 开始'}
                  </p>
                )}
              </div>
            )}
          </section>
        );
      })}
      {!groups.length && <p className="history-empty">没有找到项目或会话</p>}
    </div>
  );
}
