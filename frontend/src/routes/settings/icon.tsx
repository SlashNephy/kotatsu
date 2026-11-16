import { Anchor, Stack, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { useDocumentTitle } from '../../futon';
import { AppIconSettings } from '../../settings/AppIconSettings';

export const Route = createFileRoute('/settings/icon')({
  component: IconSettingsPage,
});

// IconSettingsPage は、ブラウザのタブとヘッダーに出すアイコンの設定 (設計書 10.3、#1665)。
function IconSettingsPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('settings.pages.icon.title')]);

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.icon.title')}</Title>
      <AppIconSettings />
    </Stack>
  );
}
