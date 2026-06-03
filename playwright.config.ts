import { defineConfig } from '@playwright/test';

export default defineConfig({
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  testMatch: '**/*.e2e.spec.ts',
});
