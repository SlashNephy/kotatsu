import { Anchor, Stack, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { GuideAppearanceSettings } from '../../guide/GuideAppearanceSettings';

export const Route = createFileRoute('/settings/guide')({
  component: GuideSettingsPage,
});

// GuideSettingsPage は、番組表の見た目の設定 (設計書 10.3、#980)。
function GuideSettingsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.guide.title')}</Title>
      <GuideAppearanceSettings />
    </Stack>
  );
}
