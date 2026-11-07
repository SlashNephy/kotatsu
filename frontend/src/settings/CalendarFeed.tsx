import { Alert, Button, Card, Code, CopyButton, Group, Stack, Text, Title } from '@mantine/core';
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, type useRelayEnvironment } from 'react-relay';

import type { CalendarFeedCreateMutation } from './__generated__/CalendarFeedCreateMutation.graphql';
import type { CalendarFeedDeleteMutation } from './__generated__/CalendarFeedDeleteMutation.graphql';
import { zonedFormat } from '../futon';
import { mutate } from './mutate';
import type { useReauthGate } from './useReauthGate';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

export type CalendarFeedItem = {
  createdAt: string;
  lastUsedAt: string | null | undefined;
};

type Props = {
  // feed は、発行した購読用の URL の記録。発行していなければ null。
  feed: CalendarFeedItem | null | undefined;
  onChanged: () => void;
  run: ReturnType<typeof useReauthGate>['run'];
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// CalendarFeed は、予約を iCalendar で購読する URL (#1255、設計書 9.8) の発行、発行し直し、取り消し。
// URL を持つ人はそのユーザーとして予約を読めるので、発行は再認証してから行う。URL は、発行した直後にだけ出す。
export function CalendarFeed({ feed, onChanged, run, fail, environment }: Props) {
  const { t } = useTranslation();
  const [issued, setIssued] = useState<string | null>(null);

  const create = () =>
    run(async () => {
      const response = await mutate<CalendarFeedCreateMutation>(
        environment,
        graphql`
          mutation CalendarFeedCreateMutation {
            createCalendarFeed {
              path
            }
          }
        `,
        {},
      );
      setIssued(new URL(response.createCalendarFeed.path, window.location.origin).toString());
      onChanged();
    }, fail);

  const revoke = () =>
    run(async () => {
      await mutate<CalendarFeedDeleteMutation>(
        environment,
        graphql`
          mutation CalendarFeedDeleteMutation {
            deleteCalendarFeed
          }
        `,
        {},
      );
      // 取り消した URL を出したままにしない。
      setIssued(null);
      onChanged();
    }, fail);

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.calendarFeed')}
        </Title>
        <Text size="xs" c="dimmed">
          {t('settings.account.calendarFeedDescription')}
        </Text>
        {issued !== null && (
          <Alert color="green" role="status" withCloseButton onClose={() => setIssued(null)}>
            <Stack gap={4}>
              <Text size="sm">{t('settings.account.calendarFeedIssued')}</Text>
              <Group gap="xs" wrap="nowrap">
                <Code miw={0} style={{ overflowWrap: 'anywhere' }}>
                  {issued}
                </Code>
                <CopyButton value={issued}>
                  {({ copied, copy }) => (
                    <Button
                      size="compact-sm"
                      variant="default"
                      flex="none"
                      leftSection={
                        copied ? <Check size={14} aria-hidden /> : <Copy size={14} aria-hidden />
                      }
                      onClick={copy}
                    >
                      {copied ? t('settings.account.copied') : t('settings.account.copy')}
                    </Button>
                  )}
                </CopyButton>
              </Group>
            </Stack>
          </Alert>
        )}
        {feed == null ? (
          <Text size="sm" c="dimmed">
            {t('settings.account.noCalendarFeed')}
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
          <Button size="compact-sm" onClick={create}>
            {feed == null
              ? t('settings.account.createCalendarFeed')
              : t('settings.account.recreateCalendarFeed')}
          </Button>
          {feed != null && (
            <Button size="compact-sm" variant="default" color="red" onClick={revoke}>
              {t('settings.account.revokeCalendarFeed')}
            </Button>
          )}
        </Group>
      </Stack>
    </Card>
  );
}
