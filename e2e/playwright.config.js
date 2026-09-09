// paper-manager E2E 配置
//
// 测试自带一个隔离实例：编译当前源码到 e2e/.tmp-server，用临时 DATA_DIR 启动，
// 绝不触碰用户真实数据目录。本地若已有 Chromium，可用
//   PLAYWRIGHT_CHROMIUM_PATH=/path/to/chrome-headless-shell npm test
// 复用，避免重复下载浏览器。
const { defineConfig } = require('@playwright/test');
const path = require('path');
const os = require('os');
const fs = require('fs');

const PORT = process.env.E2E_PORT || '18080';
const DATA_DIR = process.env.E2E_DATA_DIR || fs.mkdtempSync(path.join(os.tmpdir(), 'pm-e2e-'));
const BIN = path.join(__dirname, '.tmp-server');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: 'retain-on-failure',
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH }
      : {},
  },
  webServer: {
    command: `go build -o "${BIN}" ../cmd/server && DATA_DIR="${DATA_DIR}" PORT=${PORT} HOST=127.0.0.1 NO_OPEN=1 "${BIN}"`,
    url: `http://127.0.0.1:${PORT}/api/health`,
    reuseExistingServer: false,
    timeout: 120_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
