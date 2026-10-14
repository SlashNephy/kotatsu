import { Alert, Anchor, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import { BellOff } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { EmptyState, ErrorBoundary, PageLoader, ProgramTitle, splitProgramMarks } from '../futon';
import { splitMessage } from '../notifications/message';
import { isUnauthenticated } from '../relay/environment';
import type { notificationsListQuery } from './__generated__/notificationsListQuery.graphql';
import type { notificationsMarkAllMutation } from './__generated__/notificationsMarkAllMutation.graphql';
import type { notificationsMarkMutation } from './__generated__/notificationsMarkMutation.graphql';

export const Route = createFileRoute('/notifications')({
  component: NotificationsPage,
});

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// NotificationsPage は、画面内の通知の一覧 (設計書 6.2)。
function NotificationsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Title order={2}>{t('notifications.title')}</Title>
        <Anchor component={Link} to="/settings/notifications" size="sm">
          {t('notifications.settings')}
        </Anchor>
      </Group>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <NotificationList />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// pageSize は、一度に出す通知の数。
const pageSize = 100;

function NotificationList() {
  const { t } = useTranslation();
  // fetchKey を変えると、読み直す。既読にした後に、一覧を新しくするのに使う。
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<notificationsListQuery>(
    graphql`
      query notificationsListQuery($first: Int!) {
        unreadNotificationCount
        notifications(first: $first) {
          edges {
            node {
              id
              kind
              occurredAt
              message
              payload
              readAt
            }
          }
        }
      }
    `,
    { first: pageSize },
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const [markRead] = useMutation<notificationsMarkMutation>(graphql`
    mutation notificationsMarkMutation($ids: [ID!]!) {
      markNotificationsRead(ids: $ids)
    }
  `);
  const [markAll, markingAll] = useMutation<notificationsMarkAllMutation>(graphql`
    mutation notificationsMarkAllMutation {
      markAllNotificationsRead
    }
  `);
  const [error, setError] = useState(false);
  const refresh = () => setFetchKey((key) => key + 1);
  const read = (id: string) =>
    markRead({ variables: { ids: [id] }, onCompleted: refresh, onError: () => setError(true) });
  // 未読の数は、読み込んだ分だけでなく、すべての通知について数えたものを使う。
  const unread = data.unreadNotificationCount;

  return (
    <Stack gap="xs">
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {t('notifications.unread', { count: unread })}
        </Text>
        <Button
          size="compact-sm"
          variant="default"
          disabled={unread === 0}
          loading={markingAll}
          onClick={() =>
            markAll({ variables: {}, onCompleted: refresh, onError: () => setError(true) })
          }
        >
          {t('notifications.markAll')}
        </Button>
      </Group>
      {error && (
        <Alert color="red" role="alert">
          {t('notifications.markError')}
        </Alert>
      )}
      {data.notifications.edges.length === 0 ? (
        <EmptyState
          icon={BellOff}
          title={t('notifications.empty')}
          hint={t('notifications.emptyHint')}
        />
      ) : (
        data.notifications.edges.map(({ node }) => {
          const isUnread = node.readAt == null;

          return (
            <Card
              key={node.id}
              withBorder
              padding="xs"
              data-unread={isUnread}
              // 未読の通知は、押すか、Enter かスペースで、既読にする。
              role={isUnread ? 'button' : undefined}
              tabIndex={isUnread ? 0 : undefined}
              onClick={isUnread ? () => read(node.id) : undefined}
              onKeyDown={
                isUnread
                  ? (event: KeyboardEvent<HTMLDivElement>) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        read(node.id);
                      }
                    }
                  : undefined
              }
              style={{ cursor: isUnread ? 'pointer' : undefined }}
            >
              <Group gap="xs" wrap="nowrap" align="flex-start">
                <Text size="xs" c={isUnread ? 'mikan' : 'dimmed'} style={{ flexShrink: 0 }}>
                  {isUnread ? '●' : '○'}
                </Text>
                <Stack gap={2}>
                  <Text size="sm" fw={isUnread ? 600 : undefined}>
                    <NotificationMessage message={node.message} payload={node.payload} />
                  </Text>
                  <Text size="xs" c="dimmed">
                    {dateTimeFormat.format(Date.parse(node.occurredAt))}
                  </Text>
                </Stack>
              </Group>
            </Card>
          );
        })
      )}
    </Stack>
  );
}

// NotificationMessage は、通知の文。番組名は、ほかの画面と同じく、囲み文字を印にして描く。
function NotificationMessage({ message, payload }: { message: string; payload: string }) {
  const { before, name, after } = splitMessage(message, payload);
  // 印の後ろに文が続く (「」の録画が…」など) ときは、詰まって見えないよう、印の左と同じだけ空ける。
  const spaced = name !== undefined && after !== '' && splitProgramMarks(name).marks.length > 0;

  return (
    <>
      {before}
      {name !== undefined && <ProgramTitle name={name} />}
      {spaced && <span style={{ display: 'inline-block', width: 4 }} />}
      {after}
    </>
  );
}
