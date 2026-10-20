import { useEffect, useState } from 'react';
import { desktop, type PetSettings } from './desktop';

export default function DesktopPetSettings() {
  const [settings, setSettings] = useState<PetSettings>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    let current = true;
    const load = () =>
      desktop()
        ?.PetSettings()
        .then((value) => {
          if (current) setSettings(value);
        })
        .catch((e) => {
          if (current) setError(String(e));
        });
    load();
    window.addEventListener('focus', load);
    return () => {
      current = false;
      window.removeEventListener('focus', load);
    };
  }, []);
  const change = async (field: 'enabled' | 'animate' | 'topmost', value: boolean) => {
    if (!settings || busy) return;
    setBusy(true);
    setError('');
    try {
      setSettings(await desktop()!.ConfigurePet({ ...settings, [field]: value }));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="desktop-pet-settings" aria-label="桌面宠物设置">
      <div className="setting-row">
        <div>
          <strong>桌面小搭子</strong>
          <p>会呼吸、眨眼和伸懒腰的小猫。单击摸摸，拖动挪位置，悬停时显示快捷图标。</p>
        </div>
        <button
          className={`toggle ${settings?.enabled ? 'on' : ''}`}
          role="switch"
          aria-label="显示桌面宠物"
          aria-checked={!!settings?.enabled}
          disabled={!settings || busy}
          onClick={() => change('enabled', !settings?.enabled)}
        >
          <span />
        </button>
      </div>
      {settings?.enabled && (
        <div className="pet-preferences">
          <label>
            <input
              type="checkbox"
              aria-label="播放桌宠动画"
              checked={settings.animate}
              disabled={busy}
              onChange={(e) => change('animate', e.target.checked)}
            />
            活跃模式：空闲小动作与工作动画
          </label>
          <label>
            <input
              type="checkbox"
              aria-label="桌宠保持置顶"
              checked={settings.topmost}
              disabled={busy}
              onChange={(e) => change('topmost', e.target.checked)}
            />
            保持置顶
          </label>
          <small>原生小窗口 · 空闲低帧率 · 关闭动画后静止 · 无额外模型请求</small>
        </div>
      )}
      {error && (
        <p className="pet-settings-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
