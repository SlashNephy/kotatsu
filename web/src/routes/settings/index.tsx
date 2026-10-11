import { Card, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

export const Route = createFileRoute('/settings/')({
  component: SettingsPage,
});

// SettingsPage は、設定の入り口 (設計書 10.3)。
function SettingsPage() {
  const { t } = useTranslation();
  const links = [
    {
      to: '/settings/account',
      title: t('settings.account.title'),
      body: t('settings.account.summary'),
    },
    {
      to: '/settings/notifications',
      title: t('notifications.settingsTitle'),
      body: t('settings.notificationsSummary'),
    },
    {
      to: '/settings/password',
      title: t('viewer.changePassword'),
      body: t('settings.passwordSummary'),
    },
  ] as const;

  return (
    <Stack gap="sm">
      <Title order={2} size="h4">
        {t('settings.title')}
      </Title>
      {links.map((link) => (
        <Card
          key={link.to}
          withBorder
          padding="sm"
          renderRoot={(props) => <Link {...props} to={link.to} />}
        >
          <Text fw={600}>{link.title}</Text>
          <Text size="sm" c="dimmed">
            {link.body}
          </Text>
        </Card>
      ))}
    </Stack>
  );
}
