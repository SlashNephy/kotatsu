import { Stack, Text, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

export const Route = createFileRoute('/')({
  component: Home,
});

function Home() {
  const { t } = useTranslation();

  return (
    <Stack>
      <Title order={2}>{t('app.name')}</Title>
      <Text>{t('app.tagline')}</Text>
    </Stack>
  );
}
