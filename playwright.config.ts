import { defineConfig } from '@playwright/test';

export default defineConfig({
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  testMatch: '**/*.e2e.spec.ts',
  webServer: {
    command:
      'mkdir -p /tmp/datey-test && CGO_ENABLED=1 go build -tags fts5 -o villum-server . && DATA_DIR=/tmp/datey-test ./villum-server',
    port: 8080,
    reuseExistingServer: true,
    timeout: 30000,
  },
});
