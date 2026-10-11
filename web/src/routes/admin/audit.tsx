import {
  Alert,
  Button,
  Card,
  Code,
  Group,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Fragment, startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { AdminPage } from '../../admin/AdminPage';
import type {
  AuditEventFilter,
  auditQuery,
  auditQuery$data,
} from './__generated__/auditQuery.graphql';

export const Route = createFileRoute('/admin/audit')({
  component: AuditPage,
});

const pageSize = 50;

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// AuditPage は、監査ログを新しい順に見る、管理者向けの画面 (設計書 8.5、10.3)。
function AuditPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.audit.title')}>
      <AuditLog />
    </AdminPage>
  );
}

const query = graphql`
  query auditQuery($first: Int!, $after: String, $filter: AuditEventFilter) {
    auditEvents(first: $first, after: $after, filter: $filter) {
      edges {
        node {
          id
          occurredAt
          actor {
            type
            user {
              username
              displayName
            }
            role
          }
          action
          targetType
          targetId
          summary
          sourceIp
          userAgent
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
    users {
      id
      username
      displayName
    }
  }
`;

type AuditEvent = auditQuery$data['auditEvents']['edges'][number]['node'];

function AuditLog() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // filter は、いま一覧に使っている絞り込み。入力の途中では変えず、絞り込むボタンで変える。
  const [filter, setFilter] = useState<AuditEventFilter>({});
  const [actionPrefix, setActionPrefix] = useState('');
  const [actor, setActor] = useState<string | null>(null);
  const data = useLazyLoadQuery<auditQuery>(
    query,
    { first: pageSize, after: null, filter },
    { fetchPolicy: 'network-only' },
  );
  // more は、続きの読み込みで足した記録と、その次のカーソル。絞り込みを変えたら捨てる。
  const [more, setMore] = useState<{
    filter: AuditEventFilter;
    nodes: AuditEvent[];
    next: string | null;
  } | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState(false);
  const extra = more?.filter === filter ? more : null;
  const next = extra !== null ? extra.next : (data.auditEvents.pageInfo.endCursor ?? null);
  const hasMore = extra !== null ? extra.next !== null : data.auditEvents.pageInfo.hasNextPage;
  const events = [...data.auditEvents.edges.map((edge) => edge.node), ...(extra?.nodes ?? [])];
  const [expanded, setExpanded] = useState<string | null>(null);

  const apply = (event: FormEvent) => {
    event.preventDefault();
    const prefix = actionPrefix.trim();
    // 読み直す間も、いまの一覧を出したままにする。
    startTransition(() =>
      setFilter({
        actionPrefix: prefix === '' ? null : prefix,
        actorUserId: actor,
      }),
    );
  };
  const loadMore = async () => {
    setLoadingMore(true);
    setMoreError(false);
    try {
      const page = await fetchQuery<auditQuery>(environment, query, {
        first: pageSize,
        after: next,
        filter,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          filter,
          nodes: [...(extra?.nodes ?? []), ...page.auditEvents.edges.map((edge) => edge.node)],
          next: page.auditEvents.pageInfo.hasNextPage
            ? (page.auditEvents.pageInfo.endCursor ?? null)
            : null,
        });
      }
    } catch {
      setMoreError(true);
    } finally {
      setLoadingMore(false);
    }
  };

  const actorName = (event: AuditEvent) => {
    if (event.actor.type === 'system') {
      return t('admin.audit.system', { role: event.actor.role ?? '-' });
    }
    if (event.actor.user == null) {
      return t('admin.audit.deletedUser');
    }

    return `${event.actor.user.displayName} (@${event.actor.user.username})`;
  };

  return (
    <Stack gap="md">
      <Card withBorder>
        <form onSubmit={apply}>
          <Group gap="xs" align="flex-end">
            <TextInput
              size="xs"
              label={t('admin.audit.actionPrefix')}
              placeholder={t('admin.audit.actionPrefixPlaceholder')}
              value={actionPrefix}
              onChange={(event) => setActionPrefix(event.currentTarget.value)}
            />
            <Select
              size="xs"
              w={220}
              label={t('admin.audit.actor')}
              placeholder={t('admin.audit.anyActor')}
              clearable
              data={data.users.map((user) => ({
                value: user.id,
                label: `${user.displayName} (@${user.username})`,
              }))}
              value={actor}
              onChange={setActor}
            />
            <Button type="submit" size="compact-sm">
              {t('admin.audit.filter')}
            </Button>
          </Group>
        </form>
      </Card>

      <Card withBorder>
        {events.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.audit.empty')}
          </Text>
        ) : (
          <Table.ScrollContainer minWidth={720}>
            <Table fz="sm" verticalSpacing={4} styles={{ td: { whiteSpace: 'nowrap' } }}>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>{t('admin.audit.occurredAt')}</Table.Th>
                  <Table.Th>{t('admin.audit.actor')}</Table.Th>
                  <Table.Th>{t('admin.audit.action')}</Table.Th>
                  <Table.Th>{t('admin.audit.target')}</Table.Th>
                  <Table.Th>{t('admin.audit.source')}</Table.Th>
                  <Table.Th />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {events.map((event) => (
                  <Fragment key={event.id}>
                    <Table.Tr data-audit={event.action}>
                      <Table.Td>{dateTimeFormat.format(Date.parse(event.occurredAt))}</Table.Td>
                      <Table.Td>{actorName(event)}</Table.Td>
                      <Table.Td>
                        <Text size="sm" ff="monospace">
                          {event.action}
                        </Text>
                      </Table.Td>
                      <Table.Td>
                        <Text size="xs" ff="monospace">
                          {event.targetId == null
                            ? event.targetType
                            : `${event.targetType}:${event.targetId}`}
                        </Text>
                      </Table.Td>
                      <Table.Td>{event.sourceIp ?? '-'}</Table.Td>
                      <Table.Td>
                        <Button
                          size="compact-xs"
                          variant="subtle"
                          aria-expanded={expanded === event.id}
                          onClick={() => setExpanded(expanded === event.id ? null : event.id)}
                        >
                          {t('admin.audit.details')}
                        </Button>
                      </Table.Td>
                    </Table.Tr>
                    {expanded === event.id && (
                      <Table.Tr>
                        <Table.Td colSpan={6}>
                          <Stack gap={4}>
                            <Code block>{formatSummary(event.summary)}</Code>
                            {event.userAgent != null && event.userAgent !== '' && (
                              <Text size="xs" c="dimmed">
                                {t('admin.audit.userAgent', { userAgent: event.userAgent })}
                              </Text>
                            )}
                          </Stack>
                        </Table.Td>
                      </Table.Tr>
                    )}
                  </Fragment>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Card>

      {moreError && (
        <Alert color="red" role="alert">
          {t('admin.audit.moreError')}
        </Alert>
      )}
      {hasMore && (
        <Group>
          <Button size="compact-sm" variant="default" loading={loadingMore} onClick={loadMore}>
            {t('admin.audit.more')}
          </Button>
        </Group>
      )}
    </Stack>
  );
}

// formatSummary は、変更の要約 (JSON の文字列) を、読みやすく字下げする。JSON でなければ、そのまま返す。
function formatSummary(summary: string): string {
  try {
    return JSON.stringify(JSON.parse(summary), null, 2);
  } catch {
    return summary;
  }
}
