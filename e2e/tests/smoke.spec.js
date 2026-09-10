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

test('阅读模式：重排全文 + 选中高亮 + 刷新后仍在 + 删除', async ({ page }) => {
  // 建一篇带全文的论文（全文由 PATCH 写入，模拟已提取的状态）
  const created = await page.request.post('/api/papers', {
    data: { title: '阅读模式 E2E 论文', authors: 'Tester', year: 2026 },
  });
  expect(created.ok()).toBeTruthy();
  const paper = await created.json();
  const fulltext = [
    '1',
    'Distilling Textual Priors from LLM to Efficient Image Fusion',
    'Ran Zhang, Xuanhua He',
    'Abstract',
    '—Multi-modality image fusion aims to synthesize a single, comprehensive image from',
    'multiple source inputs. Traditional approaches offer efficiency but struggle with low-quality',
    'inputs.',
    '1 Introduction',
    'We propose a novel framework for distilling large model priors into a compact network.',
  ].join('\n');
  const patched = await page.request.patch(`/api/papers/${paper.id}`, { data: { fulltext } });
  expect(patched.ok()).toBeTruthy();

  await page.goto(`/#/papers/${paper.id}`);
  await page.waitForTimeout(500);
  await page.click('button:has-text("阅读模式")');

  // 服务端已做断行重排与标题识别
  await expect(page.locator('.read-body')).toBeVisible();
  // 服务端识别出小节标题（Abstract / 1 Introduction）
  expect(await page.locator('.read-h').count()).toBeGreaterThanOrEqual(2);
  await expect(page.locator('.read-h').first()).toHaveText(/Abstract|Distilling/);
  // 断行已被合并（原文里 "…image from" 与 "multiple source inputs." 分属两行）
  await expect(page.locator('.read-body')).toContainText('comprehensive image from multiple source inputs');

  // 选中一段文字 → 高亮
  const selected = await page.evaluate(() => {
    const root = document.querySelector('.read-body');
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node;
    while ((node = walker.nextNode())) {
      const idx = node.textContent.indexOf('Multi-modality');
      if (idx < 0 || !node.parentElement.closest('.read-seg')) continue;
      const r = document.createRange();
      r.setStart(node, idx);
      r.setEnd(node, idx + 20);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(r);
      document.querySelector('.read-wrap').dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
      return true;
    }
    return false;
  });
  expect(selected).toBeTruthy();
  await expect(page.locator('.anno-popup')).toBeVisible();
  await page.locator('.anno-popup .anno-dot--green').click();
  await expect(page.locator('mark.anno--green')).toHaveCount(1);
  await expect(page.locator('mark.anno').first()).toHaveText(/Multi-modality/);

  // 刷新后按偏移复原
  await page.reload();
  await page.waitForTimeout(800);
  await expect(page.locator('mark.anno')).toHaveCount(1);
  await expect(page.locator('.anno-item')).toHaveCount(1);

  // 标注列表 → 删除
  await page.locator('.anno-item button:has-text("删除")').first().click();
  await expect(page.locator('.anno-item')).toHaveCount(0);
  await expect(page.locator('mark.anno')).toHaveCount(0);
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
