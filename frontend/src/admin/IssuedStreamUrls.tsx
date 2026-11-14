import { Badge, Button, Group, Stack, Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { zonedFormat } from '../futon';
import { Section } from './Section';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

export type IssuedStreamUrlItem = {
  id: string;
  userDisplayName: string;
  targetName: string;
  live: boolean;
  createdAt: string;
  expiresAt: string;
  lastUsedAt: string | null | undefined;
};

type Props = {
  urls: readonly IssuedStreamUrlItem[];
  busy: boolean;
  onRevoke: (id: string) => void;
};

// IssuedStreamUrls は、発行された外部プレイヤー用の URL の一覧と、1 本ずつの取り消し (#1757、設計書 7.6)。
// 期限の過ぎていない URL を、新しい順に並べる。取り消すと、その URL は期限の前でも使えなくなり、開いたままの配信も止まる。
export function IssuedStreamUrls({ urls, busy, onRevoke }: Props) {
  const { t } = useTranslation();

  return (
    <Section title={t('admin.issuedStreamUrls.title')}>
      <Text size="xs" c="dimmed">
        {t('admin.issuedStreamUrls.description')}
      </Text>
      {urls.length === 0 ? (
        <Text size="sm" c="dimmed">
          {t('admin.issuedStreamUrls.empty')}
        </Text>
      ) : (
        <Stack gap="xs">
          {urls.map((url) => (
            <Group
              key={url.id}
              justify="space-between"
              wrap="nowrap"
              align="flex-start"
              data-issued-stream-url={url.id}
            >
              <Stack gap={2} miw={0} flex={1}>
                <Group gap="xs" wrap="nowrap" miw={0}>
                  <Badge size="sm" variant="light" color={url.live ? 'red' : 'blue'} flex="none">
                    {url.live
                      ? t('admin.issuedStreamUrls.live')
                      : t('admin.issuedStreamUrls.recording')}
                  </Badge>
                  <Text fw={600} truncate>
                    {url.targetName}
                  </Text>
                </Group>
                <Text size="xs" c="dimmed">
                  {t('admin.issuedStreamUrls.detail', {
                    user: url.userDisplayName,
                    created: dateTimeFormat.format(Date.parse(url.createdAt)),
                    expires: dateTimeFormat.format(Date.parse(url.expiresAt)),
                    lastUsed:
                      url.lastUsedAt == null
                        ? '-'
                        : dateTimeFormat.format(Date.parse(url.lastUsedAt)),
                  })}
                </Text>
              </Stack>
              <Button
                size="compact-sm"
                variant="default"
                color="red"
                flex="none"
                disabled={busy}
                onClick={() => onRevoke(url.id)}
              >
                {t('admin.issuedStreamUrls.revoke')}
              </Button>
            </Group>
          ))}
        </Stack>
      )}
    </Section>
  );
}
