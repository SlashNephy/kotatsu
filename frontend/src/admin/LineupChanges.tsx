import { Alert, Stack, Table, Text } from '@mantine/core';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { zonedFormat } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { LineupChangesMarkReadMutation } from './__generated__/LineupChangesMarkReadMutation.graphql';
import type { LineupChangesQuery } from './__generated__/LineupChangesQuery.graphql';
import { dismissLineupAlerts } from '../notifications/lineupAlerts';
import { Section } from './Section';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// LineupChanges は、衛星の局の顔ぶれがチューナーサーバーと食い違ったという知らせ (チャンネルの変更、#1519) を、新しい順に並べる。
// 開いたら、まだ読んでいない知らせを既読にし、画面の上のお知らせを消す。知らせがなければ、何も出さない。
export function LineupChanges() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<LineupChangesQuery>(
    graphql`
      query LineupChangesQuery {
        notifications(first: 10, kinds: [LINEUP_CHANGED]) {
          edges {
            node {
              id
              message
              occurredAt
              readAt
            }
          }
        }
      }
    `,
    {},
    { fetchPolicy: 'store-and-network' },
  );
  const [markRead] = useMutation<LineupChangesMarkReadMutation>(graphql`
    mutation LineupChangesMarkReadMutation($ids: [ID!]!) {
      markNotificationsRead(ids: $ids)
    }
  `);
  const [error, setError] = useState(false);
  // marked は、開いたときの既読の処理を済ませたか。開発中の StrictMode で効果が二度走っても、一度だけ送る。
  const marked = useRef(false);
  const alerts = data.notifications.edges.map(({ node }) => node);
  const unread = alerts.filter((alert) => alert.readAt == null).map((alert) => alert.id);

  useEffect(() => {
    if (marked.current || unread.length === 0) {
      return;
    }
    marked.current = true;
    // 画面の上のお知らせは、既読にし終えるのを待たずに外す。
    dismissLineupAlerts(unread);
    markRead({
      variables: { ids: unread },
      // 既読にしたことを手元のデータにも書き、画面の上のお知らせを消す。戻ってきたときに、既読にし直さない。
      updater: (store) => {
        const readAt = new Date().toISOString();
        for (const id of unread) {
          store.get(id)?.setValue(readAt, 'readAt');
        }
      },
      onError: () => setError(true),
    });
  }, [markRead, unread]);

  if (alerts.length === 0) {
    return null;
  }

  return (
    <Section title={t('admin.system.lineup.title')}>
      <Stack gap="xs">
        <Text size="sm" c="dimmed">
          {t('admin.system.lineup.description')}
        </Text>
        {error && (
          <Alert color="red" role="alert">
            {t('notifications.markError')}
          </Alert>
        )}
        <Table>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t('admin.system.lineup.occurredAt')}</Table.Th>
              <Table.Th>{t('admin.system.lineup.message')}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {alerts.map((alert) => (
              <Table.Tr key={alert.id}>
                <Table.Td style={{ whiteSpace: 'nowrap' }}>
                  {dateTimeFormat.format(Date.parse(alert.occurredAt))}
                </Table.Td>
                <Table.Td>{alert.message}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Stack>
    </Section>
  );
}
