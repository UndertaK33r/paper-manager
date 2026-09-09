// 端到端冒烟：覆盖最容易被改坏、又最依赖手工验证的链路。
// 数据落在临时 DATA_DIR 的隔离实例里，不碰真实论文库。
const { test, expect } = require('@playwright/test');
const path = require('path');

const SAMPLE_PDF = path.join(__dirname, '..', 'fixtures', 'sample.pdf');
const TITLE = 'E2E Smoke Test Paper'; // 来自 fixture 的 PDF Info 字典

// 上传重复标题会弹「检测到重复论文」确认框，删除会弹确认框：一律确认
test.beforeEach(async ({ page }) => {
  page.on('dialog', (d) => d.accept());
});

async function uploadSample(page) {
  await page.goto('/#/upload');
  await page.setInputFiles('.drop-zone input[type=file]', SAMPLE_PDF);
  await expect(page.locator('.upload-page .p3r-h3').first()).toContainText(TITLE, { timeout: 20_000 });
  return page;
}

async function openFirstDetail(page) {
  await page.goto('/');
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("详情")').click();
  await expect(page.locator('.detail-title')).toContainText(TITLE);
}

test('首页可加载，健康检查返回注入的版本号', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('.topbar')).toBeVisible();
  await expect(page.locator('.toolbar')).toBeVisible();
  const health = await page.request.get('/api/health');
  expect(health.ok()).toBeTruthy();
  expect((await health.json()).version).toBeTruthy();
});

test('上传 PDF → 出现在列表 → 打开详情看到原生阅读器', async ({ page }) => {
  await uploadSample(page);
  await page.goto('/');
  await expect(page.locator('.p3r-table tbody tr').first()).toContainText(TITLE);

  await openFirstDetail(page);
  await expect(page.locator('.pdf-panel iframe')).toBeVisible();
});

test('笔记：浮窗自动保存 + 详情页手动保存，刷新后都还在', async ({ page }) => {
  await uploadSample(page);
  await openFirstDetail(page);

  // 全屏浮窗：停止输入后自动保存
  await page.click('text=全宽阅读');
  await expect(page.locator('.fs-notes')).toBeVisible();
  await page.locator('.fs-notes__input').fill('自动保存的笔记内容');
  await expect(page.locator('.fs-notes__status')).toContainText('已保存', { timeout: 20_000 });

  await page.reload();
  await expect(page.locator('textarea.p3r-input').first()).toHaveValue('自动保存的笔记内容');

  // 详情页手动保存
  await page.locator('textarea.p3r-input').first().fill('手动保存的笔记内容');
  await page.click('button:has-text("保存笔记")');
  await page.reload();
  await expect(page.locator('textarea.p3r-input').first()).toHaveValue('手动保存的笔记内容');
});

test('全屏阅读笔记浮窗：可拖动、可固定、固定后拖不动', async ({ page }) => {
  await uploadSample(page);
  await openFirstDetail(page);
  await page.click('text=全宽阅读');

  const panel = page.locator('.fs-notes');
  await expect(panel).toBeVisible();
  const before = await panel.boundingBox();
  const head = await page.locator('.fs-notes__head').boundingBox();
  await page.mouse.move(head.x + 30, head.y + head.height / 2);
  await page.mouse.down();
  await page.mouse.move(head.x + 30 - 160, head.y + head.height / 2 - 120, { steps: 8 });
  await page.mouse.up();
  const after = await panel.boundingBox();
  expect(Math.abs(after.x - (before.x - 160))).toBeLessThan(12);

  await page.click('.fs-notes__pin');
  await expect(panel).toHaveClass(/fs-notes--pinned/);
  await expect(page.locator('.fs-notes__resize')).toHaveCount(0);

  const pinned = await panel.boundingBox();
  const head2 = await page.locator('.fs-notes__head').boundingBox();
  await page.mouse.move(head2.x + 30, head2.y + head2.height / 2);
  await page.mouse.down();
  await page.mouse.move(head2.x + 30 + 120, head2.y + head2.height / 2 + 80, { steps: 6 });
  await page.mouse.up();
  const still = await panel.boundingBox();
  expect(Math.abs(still.x - pinned.x)).toBeLessThan(2);
});

test('删除论文后从列表移除', async ({ page }) => {
  await uploadSample(page);
  await page.goto('/');
  const before = await page.locator('.p3r-table tbody tr').count();
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("删除")').click();
  await expect(page.locator('.p3r-table tbody tr')).toHaveCount(before - 1, { timeout: 15_000 });
});

test('笔记支持 Markdown 预览，且不执行注入代码', async ({ page }) => {
  await uploadSample(page);
  await openFirstDetail(page);

  await page.fill('textarea.p3r-input', '# 标题\n\n- **加粗**\n\n<script>window.__xss=1</script>');
  await page.click('.notes-head button:has-text("预览")');
  const preview = page.locator('.notes-preview');
  await expect(preview).toBeVisible();
  await expect(preview.locator('h1')).toHaveText('标题');
  await expect(preview.locator('strong')).toHaveText('加粗');
  // 注入内容必须被转义，且不执行
  expect(await preview.innerHTML()).not.toContain('<script>');
  expect(await page.evaluate(() => !window.__xss)).toBeTruthy();

  // 切回编辑，内容仍在
  await page.click('.notes-head button:has-text("编辑")');
  await expect(page.locator('textarea.p3r-input')).toHaveValue(/# 标题/);
});

test('删除进回收站、可恢复、可彻底删除', async ({ page }) => {
  await uploadSample(page);
  await page.goto('/');
  const before = await page.locator('.p3r-table tbody tr').count();
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("删除")').click();
  await expect(page.locator('.p3r-table tbody tr')).toHaveCount(before - 1, { timeout: 15_000 });

  // 管理 → 回收站（注意：其他用例的删除也会留在这里，因此用相对计数断言）
  await page.click('text=管理');
  await page.click('.dialog-head button:has-text("回收站")');
  await expect(page.locator('.trash-list li').first()).toContainText('删除于');
  const trashed = await page.locator('.trash-list li').count();

  // 恢复一条后回到列表
  await page.locator('.trash-list li button:has-text("恢复")').first().click();
  await expect(page.locator('.trash-list li')).toHaveCount(trashed - 1, { timeout: 15_000 });
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(page.locator('.p3r-table tbody tr')).toHaveCount(before, { timeout: 15_000 });

  // 再次删除并彻底删除，回收站少一条
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("删除")').click();
  await expect(page.locator('.p3r-table tbody tr')).toHaveCount(before - 1, { timeout: 15_000 });
  await page.click('text=管理');
  await page.click('.dialog-head button:has-text("回收站")');
  await page.locator('.trash-list li button:has-text("彻底删除")').first().click();
  await expect(page.locator('.trash-list li')).toHaveCount(trashed - 1, { timeout: 15_000 });
});

test('备份接口返回可解析的 zip，且包含数据库快照', async ({ page }) => {
  await uploadSample(page);
  const res = await page.request.get('/api/backup');
  expect(res.ok()).toBeTruthy();
  expect(res.headers()['content-type']).toContain('application/zip');
  const body = await res.body();
  expect(body.slice(0, 4).toString('hex')).toBe('504b0304'); // zip 魔数
  expect(body.includes(Buffer.from('data/paper-manager.db'))).toBeTruthy();
  expect(body.includes(Buffer.from('data/uploads/'))).toBeTruthy();
});
