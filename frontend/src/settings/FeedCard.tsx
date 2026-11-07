import { Alert, Button, Card, Code, CopyButton, Group, Stack, Text, Title } from '@mantine/core';
import { Check, Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { zonedFormat } from '../futon';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

export type FeedItem = {
  createdAt: string;
  lastUsedAt: string | null | undefined;
};

export type IssuedUrl = {
  // label は、URL が何のためのものかの見出し。URL が 1 つなら省く。
  label?: string;
  url: string;
};

type Props = {
  title: string;
  description: string;
  // issuedMessage は、発行した直後に URL と一緒に出す説明。
  issuedMessage: string;
  emptyMessage: string;
  createLabel: string;
  // feed は、発行した URL の記録。発行していなければ null。
  feed: FeedItem | null | undefined;
  // issued は、発行した直後にだけ分かる URL。出していなければ null。
  issued: IssuedUrl[] | null;
  onCloseIssued: () => void;
  onCreate: () => void;
  onRevoke: () => void;
};

// FeedCard は、Cookie を送れないアプリに配信する URL (設計書 9.8) の発行、発行し直し、取り消しのカード。
// 予約の iCalendar (#1255) と、ライブ視聴の M3U と番組表の XMLTV (#1324) で使う。URL は、発行した直後にだけ出す。
export function FeedCard({
  title,
  description,
  issuedMessage,
  emptyMessage,
  createLabel,
  feed,
  issued,
  onCloseIssued,
  onCreate,
  onRevoke,
}: Props) {
  const { t } = useTranslation();

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {title}
        </Title>
        <Text size="xs" c="dimmed">
          {description}
        </Text>
        {issued !== null && (
          <Alert color="green" role="status" withCloseButton onClose={onCloseIssued}>
            <Stack gap={4}>
              <Text size="sm">{issuedMessage}</Text>
              {issued.map(({ label, url }) => (
                <Stack key={url} gap={2}>
                  {label !== undefined && (
                    <Text size="xs" fw={600}>
                      {label}
                    </Text>
                  )}
                  <Group gap="xs" wrap="nowrap">
                    <Code miw={0} style={{ overflowWrap: 'anywhere' }}>
                      {url}
                    </Code>
                    <CopyButton value={url}>
                      {({ copied, copy }) => (
                        <Button
                          size="compact-sm"
                          variant="default"
                          flex="none"
                          leftSection={
                            copied ? (
                              <Check size={14} aria-hidden />
                            ) : (
                              <Copy size={14} aria-hidden />
                            )
                          }
                          onClick={copy}
                        >
                          {copied ? t('settings.account.copied') : t('settings.account.copy')}
                        </Button>
                      )}
                    </CopyButton>
                  </Group>
                </Stack>
              ))}
            </Stack>
          </Alert>
        )}
        {feed == null ? (
          <Text size="sm" c="dimmed">
            {emptyMessage}
          </Text>
        ) : (
          <Text size="xs" c="dimmed">
            {t('settings.account.apiTokenTimes', {
              created: dateTimeFormat.format(Date.parse(feed.createdAt)),
              lastUsed:
                feed.lastUsedAt == null ? '-' : dateTimeFormat.format(Date.parse(feed.lastUsedAt)),
            })}
          </Text>
        )}
        <Group gap="xs">
          <Button size="compact-sm" onClick={onCreate}>
            {feed == null ? createLabel : t('settings.account.recreateFeed')}
          </Button>
          {feed != null && (
            <Button size="compact-sm" variant="default" color="red" onClick={onRevoke}>
              {t('settings.account.revokeFeed')}
            </Button>
          )}
        </Group>
      </Stack>
    </Card>
  );
}
