import { expect, test } from '@playwright/test';

test.describe('テーマ', () => {
  test.use({ colorScheme: 'light' });

  test('ダークを選ぶと焦げ茶の背景になり、再読み込みしても残る', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Kotatsu');
    await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(251, 247, 239)');

    await page.getByText('ダーク', { exact: true }).click();
    await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(42, 34, 28)');

    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'dark');
  });
});
