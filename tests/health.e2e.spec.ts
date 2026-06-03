import { test, expect } from '@playwright/test';

test('health endpoint returns ok', async ({ page }) => {
  const base = process.env.DATEY_URL || 'http://localhost:8080';
  const resp = await page.request.get(`${base}/health`);
  expect(resp.status()).toBe(200);
  const body = await resp.json();
  expect(body.status).toBe('ok');
  expect(body.version).toBeDefined();
});
