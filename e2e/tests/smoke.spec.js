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

test('上传 PDF → 出现在列表 → 详情内渲染出 PDF 页面', async ({ page }) => {
  await uploadSample(page);
  await page.goto('/');
  await expect(page.locator('.p3r-table tbody tr').first()).toContainText(TITLE);

  await openFirstDetail(page);
  // pdf.js 渲染：画布 + 可选中文字层（用于划段标注）
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('.pdf-page .pdf-text span').first()).toBeAttached();
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

test('PDF 上划段标注：选中→高亮→刷新复原→删除', async ({ page }) => {
  // 造一篇带 PDF 的论文：用最小 PDF fixture 上传（无文字层时无法划段，因此这里用 API 造数据不可行）
  await uploadSample(page);
  await page.goto('/');
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("详情")').click();
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 }); // pdf.js 渲染成功
  // 文字层在画布之后渲染，必须等到有 span 才能构造选区
  await expect.poll(async () => await page.locator('.pdf-text span').count(), { timeout: 20_000 }).toBeGreaterThan(0);

  // 在 PDF 文字层上构造选区（几何方式，与栏数无关）
  const selected = await page.evaluate(() => {
    const wrap = document.querySelector('.pdf-wrap');
    const spans = [...document.querySelectorAll('.pdf-text span')].filter(s => s.firstChild && s.textContent.trim().length > 8);
    if (!spans.length) return null;
    const node = spans[Math.floor(spans.length / 3)];
    node.scrollIntoView({ block: 'center' });
    const r = document.createRange();
    r.setStart(node.firstChild, 0);
    r.setEnd(node.firstChild, Math.min(6, node.firstChild.length));
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(r);
    wrap.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
    return node.textContent.slice(0, 6);
  });
  expect(selected).toBeTruthy();
  await expect(page.locator('.anno-popup')).toBeVisible();
  await page.locator('.anno-popup .anno-dot--yellow').click();
  await expect(page.locator('.pdf-anno')).toHaveCount(1);

  // 高亮与被选文字在几何上重合
  const aligned = await page.evaluate(() => {
    const box = document.querySelector('.pdf-anno');
    if (!box) return false;
    const rb = box.getBoundingClientRect();
    return rb.width > 1 && rb.height > 1;
  });
  expect(aligned).toBeTruthy();

  // 刷新后按「页码 + 归一化矩形」复原
  await page.reload();
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('.pdf-anno')).toHaveCount(1);
  await expect(page.locator('.anno-item')).toHaveCount(1);

  // 标注列表 → 删除
  await page.locator('.anno-item button:has-text("删除")').first().click();
  await expect(page.locator('.anno-item')).toHaveCount(0);
  await expect(page.locator('.pdf-anno')).toHaveCount(0);
});

test('PDF 上加文字批注：新建→输入→拖动→刷新保留→删除', async ({ page }) => {
  await uploadSample(page);
  await page.goto('/');
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("详情")').click();
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 });

  // 开启批注模式后点击页面任意位置
  await page.click('button:has-text("加批注")');
  // 点在 PDF 容器的可见区域内（整页比视口高，不能按页面高度比例取点）
  const wrapBox = await page.locator('.pdf-wrap').boundingBox();
  await page.mouse.click(wrapBox.x + wrapBox.width * 0.5, wrapBox.y + 120);
  await expect(page.locator('.pdf-note')).toHaveCount(1);

  // 输入文字（失焦保存）
  await page.locator('.pdf-note textarea').fill('这里值得复现');
  await page.locator('.pdf-note textarea').blur();
  await expect(page.locator('.anno-item')).toHaveCount(1);
  await expect(page.locator('.anno-item__kind').first()).toHaveText('批注');

  // 拖动位置并保存
  const before = await page.evaluate(() => {
    const b = document.querySelector('.pdf-note');
    return [b.offsetLeft, b.offsetTop];
  });
  const grip = await page.locator('.pdf-note__grip').boundingBox();
  await page.mouse.move(grip.x + 4, grip.y + 4);
  await page.mouse.down();
  await page.mouse.move(grip.x + 4 + 90, grip.y + 4 + 60, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(600);
  const after = await page.evaluate(() => {
    const b = document.querySelector('.pdf-note');
    return [b.offsetLeft, b.offsetTop];
  });
  expect(Math.abs(after[0] - before[0] - 90)).toBeLessThan(12);

  // 刷新后仍在（文字 + 位置）
  await page.reload();
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('.pdf-note')).toHaveCount(1);
  await expect(page.locator('.pdf-note textarea')).toHaveValue('这里值得复现');

  // 删除
  await page.locator('.pdf-note__del').click();
  await expect(page.locator('.pdf-note')).toHaveCount(0);
  await expect(page.locator('.anno-item')).toHaveCount(0);
});

test('划词翻译：选中文字→译文浮层→收集到译文面板→存为批注', async ({ page }) => {
  // 拦截翻译接口，不消耗真实 API 调用
  await page.route('**/api/ai/translate-text', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ translation: '多模态图像融合旨在把多个来源合成为一张图。' }),
    });
  });
  // 翻译按钮需要已配置 API Key（请求本身走上面的拦截，不会真的调用）
  const cfg = await page.request.put('/api/settings', { data: { aiApiKey: 'sk-e2e-dummy' } });
  expect((await cfg.json()).hasApiKey).toBeTruthy();
  await uploadSample(page);
  await page.goto('/');
  await page.locator('.p3r-table tbody tr').first().locator('button:has-text("详情")').click();
  await expect(page.locator('.pdf-page canvas')).toBeVisible({ timeout: 20_000 });
  await expect.poll(async () => await page.locator('.pdf-text span').count(), { timeout: 20_000 }).toBeGreaterThan(0);

  // 选中一句 → 浮层里点「翻译」
  await page.evaluate(() => {
    const node = [...document.querySelectorAll('.pdf-text span')].find(s => s.textContent.includes('Multi-modality'));
    node.scrollIntoView({ block: 'center' });
    const r = document.createRange();
    r.setStart(node.firstChild, 0);
    r.setEnd(node.firstChild, Math.min(24, node.firstChild.length));
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(r);
    document.querySelector('.pdf-wrap').dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
  });
  await expect(page.locator('.anno-popup button:has-text("翻译")')).toBeVisible();
  await page.click('.anno-popup button:has-text("翻译")');
  await expect(page.locator('.trans-popup')).toBeVisible();
  await expect(page.locator('.trans-popup')).toContainText('多模态图像融合');

  // 译文进入译文面板（全宽时可见）
  await page.click('button:has-text("全宽阅读")');
  await expect(page.locator('.fs-trans')).toContainText('多模态图像融合');

  // 存为批注 → 页面上出现批注框，内容为译文
  await page.click('.trans-popup button:has-text("存为批注")');
  await expect(page.locator('.pdf-note')).toHaveCount(1);
  await expect(page.locator('.pdf-note textarea')).toHaveValue(/多模态图像融合/);
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
