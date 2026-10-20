// Headless verification of OUR generated demo only. No user profile or desktop UI.
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const { spawn, execFileSync } = require('node:child_process');
const assert = require('node:assert/strict');
const { chromium } = require('playwright');

async function freePort() {
  const listener = net.createServer();
  await new Promise((resolve, reject) => listener.once('error', reject).listen(0, '127.0.0.1', resolve));
  const port = listener.address().port;
  await new Promise((resolve) => listener.close(resolve));
  return port;
}
async function stop(proc) {
  if (!proc || proc.exitCode !== null) return;
  const done = new Promise((resolve) => proc.once('exit', resolve));
  proc.kill();
  await done;
}
async function main() {
  const run = path.resolve(process.argv[2]);
  if (!fs.existsSync(path.join(run, 'project', 'go.mod'))) throw new Error('Provide a completed demo run directory');
  const qa = path.join(run, 'qa');
  const work = path.join(qa, `work-${Date.now()}`);
  fs.mkdirSync(work, { recursive: true });
  fs.copyFileSync(path.join(run, 'project', 'index.html'), path.join(work, 'index.html'));
  const serverExe = path.join(qa, 'todo-demo-check.exe');
  execFileSync('go', ['build', '-o', serverExe, '.'], { cwd: path.join(run, 'project'), windowsHide: true, timeout: 60000 });
  let child, browser;
  let serverLog = '';
  const start = async () => {
    const port = await freePort();
    const env = { PORT: String(port) };
    for (const key of ['PATH', 'SystemRoot', 'WINDIR', 'TMP', 'TEMP', 'USERPROFILE']) if (process.env[key]) env[key] = process.env[key];
    child = spawn(serverExe, [], { cwd: work, env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
    child.stdout.on('data', (b) => { serverLog += b.toString(); });
    child.stderr.on('data', (b) => { serverLog += b.toString(); });
    const base = `http://127.0.0.1:${port}`;
    for (let i = 0; i < 100; i++) {
      try { if ((await fetch(`${base}/api/health`)).ok) return base; } catch {}
      if (child.exitCode !== null) throw new Error(`Demo exited: ${serverLog}`);
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error(`Server not ready: ${serverLog}`);
  };
  const evidence = { passed: false, headless: true, tests: [], externalPageRequests: [] };
  try {
    let base = await start();
    browser = await chromium.launch({ channel: 'msedge', headless: true });
    const page = await browser.newPage({ viewport: { width: 1100, height: 860 } });
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.route('**/*', async (route) => {
      const url = route.request().url();
      if (url.startsWith(base + '/') || url.startsWith('data:')) return route.continue();
      evidence.externalPageRequests.push(url);
      return route.abort();
    });
    await page.goto(base, { waitUntil: 'networkidle' });
    await page.locator('#emptyState').waitFor({ state: 'visible' });
    evidence.tests.push('首次打开空列表正常');
    const input = page.locator('#titleInput');
    await input.fill('整理本周会议纪要'); await input.press('Enter');
    await page.getByText('整理本周会议纪要', { exact: true }).waitFor();
    await input.fill('核对项目预算'); await page.locator('#addBtn').click();
    await page.getByText('核对项目预算', { exact: true }).waitFor();
    assert.equal((await page.locator('#statTotal').textContent()).trim(), '2');
    evidence.tests.push('回车/按钮新增，统计为2');
    const row = page.locator('#todoList li').filter({ hasText: '整理本周会议纪要' });
    await row.locator('.todo-toggle').click();
    await page.waitForFunction(() => document.querySelector('#statDone').textContent.trim() === '1');
    evidence.tests.push('完成切换实际保存，完成数为1');
    await page.locator('[data-filter="active"]').click();
    assert.equal(await page.getByText('整理本周会议纪要', { exact: true }).count(), 0);
    assert.equal(await page.getByText('核对项目预算', { exact: true }).count(), 1);
    await page.locator('[data-filter="done"]').click();
    assert.equal(await page.getByText('核对项目预算', { exact: true }).count(), 0);
    await page.locator('[data-filter="all"]').click();
    await page.locator('#searchInput').fill('预算');
    assert.equal(await page.getByText('核对项目预算', { exact: true }).count(), 1);
    assert.equal(await page.getByText('整理本周会议纪要', { exact: true }).count(), 0);
    await page.locator('#searchInput').fill('');
    evidence.tests.push('进行中/完成筛选与搜索正常');
    // Delivery screenshots show ordinary demo content, before security-test data.
    await page.screenshot({ path: path.join(run, 'preview-desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 420, height: 860 });
    await page.screenshot({ path: path.join(run, 'preview-compact.png'), fullPage: true });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
    evidence.tests.push('1100和420宽度布局，无横向溢出');
    await page.setViewportSize({ width: 1100, height: 860 });
    const hostile = '<img src=x onerror="window.demoInjected=true">';
    await input.fill(hostile); await input.press('Enter');
    await page.getByText(hostile, { exact: true }).waitFor();
    assert.equal(await page.locator('#todoList img').count(), 0);
    assert.equal(await page.evaluate(() => Boolean(window.demoInjected)), false);
    evidence.tests.push('HTML样式标题按纯文本显示，未执行');
    await input.fill('   '); await input.press('Enter');
    await page.locator('#formError').waitFor({ state: 'visible' });
    evidence.tests.push('空白标题在界面明确报错');
    await input.fill('');
    await stop(child); child = null;
    base = await start();
    await page.goto(base, { waitUntil: 'networkidle' });
    await page.getByText('整理本周会议纪要', { exact: true }).waitFor();
    await page.waitForFunction(() => document.querySelector('#statDone').textContent.trim() === '1');
    assert.equal((await page.locator('#statTotal').textContent()).trim(), '3');
    evidence.tests.push('真实服务停止重开，三条数据与完成状态保留');
    assert.equal(errors.length, 0, JSON.stringify(errors));
    assert.equal(evidence.externalPageRequests.length, 0);
    evidence.passed = true;
    console.log(JSON.stringify(evidence, null, 2));
  } catch (error) {
    evidence.error = error.message;
    throw error;
  } finally {
    fs.writeFileSync(path.join(run, 'browser-verification.json'), JSON.stringify(evidence, null, 2));
    fs.writeFileSync(path.join(qa, 'server.log'), serverLog);
    if (browser) await browser.close();
    await stop(child);
  }
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
