import { Alert, Anchor, Card, Group, Stack, Text, VisuallyHidden } from '@mantine/core';
import { BellOff } from 'lucide-react';
import { useNavigate } from '@tanstack/react-router';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useUnreadNotificationCount } from '../changes/DataChanges';
import { EmptyState, ProgramTitle, splitProgramMarks, zonedFormat } from '../futon';
import { notificationPath } from './link';
import { splitMessage } from './message';
import type { NotificationListQuery } from './__generated__/NotificationListQuery.graphql';
import type { NotificationListMarkAllMutation } from './__generated__/NotificationListMarkAllMutation.graphql';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

type Props = {
  // first は、新しいものから出す通知の数。
  first: number;
  // onNavigate は、通知が示す画面へ移るときに呼ぶ。ポップオーバーを閉じるのに使う。
  onNavigate?: () => void;
};

// NotificationList は、通知の一覧 (設計書 6.2)。通知のページと、ヘッダーのベルのポップオーバー (#1086) で使う。
// 1 件ずつ既読にする手間を省くため、読み込んだら、未読の通知をすべて既読にする。
// 開いたときに未読だった通知は、既読にした後に一覧を読み直しても、開いている間は新着として示す。
export function NotificationList({ first, onNavigate }: Props) {
  const { t } = useTranslation();
  // 未読の数が変わったら、一覧を読み直す (#867)。開いている間に届いた通知を出し、既読にした結果も反映する。
  const unreadChanges = useUnreadChanges();
  // markedCount は、開いたときの既読の処理を終えた回数。購読が最初の未読の数を送る前に既読にし終えると、
  // 未読の数の変化に気づけないので、終えたときにも読み直す。
  const [markedCount, setMarkedCount] = useState(0);
  const fetchKey = unreadChanges + markedCount;
  const data = useLazyLoadQuery<NotificationListQuery>(
    graphql`
      query NotificationListQuery($first: Int!) {
        unreadNotificationCount
        notifications(first: $first) {
          edges {
            node {
              id
              kind
              occurredAt
              message
              payload
              url
              readAt
            }
          }
        }
      }
    `,
    { first },
    // 開くたびに既読にするので、前に開いたときの一覧を使い回さず、取り直してから出す。
    // 開いた後に読み直すときは、読み直す間も一覧を出したままにする。
    { fetchKey, fetchPolicy: fetchKey === 0 ? 'network-only' : 'store-and-network' },
  );
  const [markAll] = useMutation<NotificationListMarkAllMutation>(graphql`
    mutation NotificationListMarkAllMutation {
      markAllNotificationsRead
    }
  `);
  const [error, setError] = useState(false);
  // marked は、開いたときの既読の処理を済ませたか。開発中の StrictMode で効果が二度走っても、一度だけ送る。
  const marked = useRef(false);
  // 未読の数は、読み込んだ分だけでなく、すべての通知について数えたものを使う。
  const unread = data.unreadNotificationCount;
  // newIds は、開いたときに未読だった通知。既読にすると、変更の通知で一覧が読み直され、readAt が埋まるので、最初の分を覚えておく。
  const [newIds] = useState(
    () =>
      new Set(
        data.notifications.edges
          .filter(({ node }) => node.readAt == null)
          .map(({ node }) => node.id),
      ),
  );

  // 既読にするのは、開いたときの 1 回だけにする。開いている間に届いた通知は、次に開くまで未読のまま残す。
  useEffect(() => {
    if (marked.current) {
      return;
    }
    marked.current = true;
    if (unread > 0) {
      markAll({
        variables: {},
        onCompleted: () => setMarkedCount((n) => n + 1),
        onError: () => setError(true),
      });
    }
  }, [markAll, unread]);

  return (
    <Stack gap="xs">
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
          // 開いている間に届いた通知は、まだ未読なので、新着として示す。
          const isNew = newIds.has(node.id) || node.readAt == null;

          return (
            <Card key={node.id} withBorder padding="xs" data-new={isNew}>
              <Group gap="xs" wrap="nowrap" align="flex-start">
                <Text
                  size="xs"
                  c={isNew ? 'mikan' : 'dimmed'}
                  aria-hidden
                  style={{ flexShrink: 0 }}
                >
                  {isNew ? '●' : '○'}
                </Text>
                <Stack gap={2}>
                  <Text size="sm" fw={isNew ? 600 : undefined}>
                    {isNew && <VisuallyHidden>{t('notifications.new')}</VisuallyHidden>}
                    <NotificationMessage message={node.message} payload={node.payload} />
                  </Text>
                  <Group gap="xs">
                    <Text size="xs" c="dimmed">
                      {dateTimeFormat.format(Date.parse(node.occurredAt))}
                    </Text>
                    <NotificationLink url={node.url} kind={node.kind} onNavigate={onNavigate} />
                  </Group>
                </Stack>
              </Group>
            </Card>
          );
        })
      )}
    </Stack>
  );
}

// NotificationLink は、通知が示す画面 (視聴予約の通知では、そのサービスのライブ視聴) への入り口。示す画面がなければ出さない。
function NotificationLink({
  url,
  kind,
  onNavigate,
}: {
  url: string | null | undefined;
  kind: string;
  onNavigate: (() => void) | undefined;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const path = notificationPath(url);
  if (path === null) {
    return null;
  }

  return (
    <Anchor
      href={path}
      size="xs"
      onClick={(event) => {
        // 修飾キーを押したときや中クリックは、ブラウザに任せて新しいタブなどで開かせる。
        if (
          event.button !== 0 ||
          event.metaKey ||
          event.ctrlKey ||
          event.shiftKey ||
          event.altKey
        ) {
          return;
        }
        event.preventDefault();
        onNavigate?.();
        void navigate({ href: path });
      }}
    >
      {kind === 'WATCH_REMINDER' ? t('notifications.watchLive') : t('notifications.open')}
    </Anchor>
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

// useUnreadChanges は、画面を開いてから未読の通知の数が変わった回数を返す。分からない間 (切れている間) は数えない。
function useUnreadChanges(): number {
  const unread = useUnreadNotificationCount();
  const [state, setState] = useState({ last: unread, changes: 0 });
  if (unread !== null && unread !== state.last) {
    setState({ last: unread, changes: state.last === null ? state.changes : state.changes + 1 });
  }

  return state.changes;
}
