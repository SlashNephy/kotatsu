import {
  Alert,
  Anchor,
  Badge,
  Box,
  Button,
  Card,
  Group,
  Modal,
  SegmentedControl,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
} from 'react-relay';

import { ErrorBoundary } from '../futon';
import { isUnauthenticated } from '../relay/environment';
import { groupByDay, needsAttention, timelineBar } from '../reservations/timeline';
import type { reservationsDeleteMutation } from './__generated__/reservationsDeleteMutation.graphql';
import type { reservationsEnableMutation } from './__generated__/reservationsEnableMutation.graphql';
import type {
  reservationsQuery,
  reservationsQuery$data,
} from './__generated__/reservationsQuery.graphql';

// query は、時間帯に重なる予約を取る。続きは、同じ時間帯と状態で、カーソルから読む。
const query = graphql`
  query reservationsQuery(
    $from: Time!
    $to: Time!
    $states: [AssignmentState!]
    $first: Int!
    $after: String
  ) {
    viewer {
      role
    }
    reservations(from: $from, to: $to, states: $states, first: $first, after: $after) {
      edges {
        node {
          id
          kind
          name
          startAt
          endAt
          enabled
          service {
            name
          }
          assignment {
            state
            tuner
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
`;

type ReservationsSearch = {
  // problems は、録画が欠ける見込みの予約だけを出すかどうか。
  problems?: boolean;
};

export const Route = createFileRoute('/reservations')({
  validateSearch: (search: Record<string, unknown>): ReservationsSearch =>
    search['problems'] === true ? { problems: true } : {},
  component: ReservationsPage,
});

const hour = 3_600_000;
// rangeDays は、一覧に出す、いまからの日数。
const rangeDays = 8;
// pageSize は、1 回に取る件数。tenban は 100 件まで返す。
const pageSize = 100;

const dayFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  timeZone: 'Asia/Tokyo',
});
const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

const kindKeys: Record<
  string,
  'reservations.kind.PROGRAM' | 'reservations.kind.TIME' | 'reservations.kind.RULE'
> = {
  PROGRAM: 'reservations.kind.PROGRAM',
  TIME: 'reservations.kind.TIME',
  RULE: 'reservations.kind.RULE',
};
const stateKeys: Record<
  string,
  | 'reservations.state.PENDING'
  | 'reservations.state.OK'
  | 'reservations.state.PARTIAL'
  | 'reservations.state.CONFLICT'
> = {
  PENDING: 'reservations.state.PENDING',
  OK: 'reservations.state.OK',
  PARTIAL: 'reservations.state.PARTIAL',
  CONFLICT: 'reservations.state.CONFLICT',
};

type Node = reservationsQuery$data['reservations']['edges'][number]['node'];
type Reservation = Node & { readonly startMs: number };

// ReservationsPage は、これからの予約を、日ごとの時間軸に並べる画面 (設計書 10.3)。
function ReservationsPage() {
  const { t } = useTranslation();
  // 画面を開いた時刻から数える。放送中の予約も出すよう、少し前から取る。
  // 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。
  const [range] = useState(() => {
    const now = Date.now();

    return {
      from: new Date(now - 6 * hour).toISOString(),
      to: new Date(now + rangeDays * 24 * hour).toISOString(),
    };
  });

  return (
    <Stack gap="sm">
      <Title order={2}>{t('reservations.title')}</Title>
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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Reservations range={range} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Reservations({ range }: { range: { from: string; to: string } }) {
  const { t } = useTranslation();
  const navigate = Route.useNavigate();
  const { problems = false } = Route.useSearch();
  // patches は、この画面で変えた予約。一覧を取り直さずに反映し、読み込んだ続きを捨てない。
  const [patches, setPatches] = useState<Record<string, { enabled?: boolean; removed?: boolean }>>(
    {},
  );
  const patch = (id: string, change: { enabled?: boolean; removed?: boolean }) =>
    setPatches((prev) => ({ ...prev, [id]: { ...prev[id], ...change } }));
  const data = useLazyLoadQuery<reservationsQuery>(
    query,
    {
      ...range,
      states: problems ? ['PARTIAL', 'CONFLICT'] : null,
      first: pageSize,
      after: null,
    },
    { fetchPolicy: 'store-and-network' },
  );
  const environment = useRelayEnvironment();
  // more は、続きの読み込みで足した予約と、その次のカーソル。絞り込みを変えたら捨てる。
  const [more, setMore] = useState<{ key: boolean; nodes: Node[]; next: string | null } | null>(
    null,
  );
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState(false);
  const extra = more?.key === problems ? more : null;
  const next = extra !== null ? extra.next : (data.reservations.pageInfo.endCursor ?? null);
  const hasMore = extra !== null ? extra.next !== null : data.reservations.pageInfo.hasNextPage;
  const loadMore = async () => {
    setLoadingMore(true);
    setMoreError(false);
    try {
      const page = await fetchQuery<reservationsQuery>(environment, query, {
        ...range,
        states: problems ? ['PARTIAL', 'CONFLICT'] : null,
        first: pageSize,
        after: next,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          key: problems,
          nodes: [...(extra?.nodes ?? []), ...page.reservations.edges.map((edge) => edge.node)],
          next: page.reservations.pageInfo.hasNextPage
            ? (page.reservations.pageInfo.endCursor ?? null)
            : null,
        });
      }
    } catch {
      setMoreError(true);
    } finally {
      setLoadingMore(false);
    }
  };
  const [deleting, setDeleting] = useState<Reservation | null>(null);
  const canEdit = data.viewer.role !== 'VIEWER';
  const reservations: Reservation[] = [
    ...data.reservations.edges.map((edge) => edge.node),
    ...(extra?.nodes ?? []),
  ]
    .filter((node) => patches[node.id]?.removed !== true)
    .map((node) => ({
      ...node,
      enabled: patches[node.id]?.enabled ?? node.enabled,
      startMs: Date.parse(node.startAt),
    }));
  const groups = groupByDay(reservations);
  const attention = reservations.filter((reservation) =>
    needsAttention(reservation.assignment.state, reservation.enabled),
  ).length;

  return (
    <Stack>
      <Group justify="space-between" wrap="wrap">
        <SegmentedControl
          size="xs"
          value={problems ? 'problems' : 'all'}
          onChange={(value) =>
            void navigate({ search: value === 'problems' ? { problems: true } : {} })
          }
          data={[
            { value: 'all', label: t('reservations.filter.all') },
            { value: 'problems', label: t('reservations.filter.problems') },
          ]}
        />
        {!problems && attention > 0 && (
          <Badge color="amber" size="lg" variant="light">
            {t('reservations.attention', { count: attention })}
          </Badge>
        )}
      </Group>
      {reservations.length === 0 && (
        <Alert color="gray">
          {problems ? t('reservations.noProblems') : t('reservations.empty')}
        </Alert>
      )}
      {groups.map((group) => (
        <Stack key={group.from} gap={4}>
          <Text fw={700}>{dayFormat.format(group.from)}</Text>
          {group.items.map((reservation) => (
            <ReservationRow
              key={reservation.id}
              reservation={reservation}
              dayFrom={group.from}
              canEdit={canEdit}
              onChanged={(enabled) => patch(reservation.id, { enabled })}
              onDelete={() => setDeleting(reservation)}
            />
          ))}
        </Stack>
      ))}
      {moreError && (
        <Alert color="red" role="alert">
          {t('reservations.moreError')}
        </Alert>
      )}
      {hasMore && (
        <Button variant="default" loading={loadingMore} onClick={() => void loadMore()}>
          {t('reservations.more')}
        </Button>
      )}
      {deleting !== null && (
        <DeleteDialog
          reservation={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={(disabled) => {
            // ルール予約は消さずに無効にするので、無効として残す。
            patch(deleting.id, disabled ? { enabled: false } : { removed: true });
            setDeleting(null);
          }}
        />
      )}
    </Stack>
  );
}

type RowProps = {
  reservation: Reservation;
  dayFrom: number;
  canEdit: boolean;
  onChanged: (enabled: boolean) => void;
  onDelete: () => void;
};

function ReservationRow({ reservation, dayFrom, canEdit, onChanged, onDelete }: RowProps) {
  const { t } = useTranslation();
  const [commit, inFlight] = useMutation<reservationsEnableMutation>(graphql`
    mutation reservationsEnableMutation($id: ID!, $enabled: Boolean!) {
      updateReservation(id: $id, input: { enabled: $enabled }) {
        id
        enabled
      }
    }
  `);
  const [toggleError, setToggleError] = useState(false);
  const end = Date.parse(reservation.endAt);
  const bar = timelineBar(dayFrom, reservation.startMs, end);
  const state = reservation.assignment.state;
  const attention = needsAttention(state, reservation.enabled);
  const barColor = !reservation.enabled
    ? 'var(--mantine-color-gray-4)'
    : attention
      ? 'var(--mantine-color-amber-5)'
      : 'var(--mantine-color-mikan-4)';

  return (
    <Card
      withBorder
      padding="xs"
      data-state={state}
      style={{
        opacity: reservation.enabled ? 1 : 0.6,
        // 録画が欠ける見込みの予約は、左の線でも示す。
        borderLeft: attention ? '4px solid var(--mantine-color-amber-5)' : undefined,
      }}
    >
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Stack gap={2} style={{ minWidth: 0 }}>
          <Group gap="xs" wrap="nowrap">
            <Text size="sm" c="dimmed" style={{ flexShrink: 0 }}>
              {timeFormat.format(reservation.startMs)}〜{timeFormat.format(end)}
            </Text>
            <Text size="sm" truncate>
              {reservation.service.name}
            </Text>
          </Group>
          <Text size="sm" fw={700} truncate>
            {reservation.name === '' ? t('reservations.untitled') : reservation.name}
          </Text>
          <Group gap={4}>
            <Badge size="sm" variant="outline" color="gray">
              {t(kindKeys[reservation.kind] ?? 'reservations.kind.PROGRAM')}
            </Badge>
            {!reservation.enabled ? (
              <Badge size="sm" color="gray">
                {t('reservations.disabled')}
              </Badge>
            ) : (
              <Badge size="sm" color={attention ? 'amber' : 'gray'} variant="light">
                {t(stateKeys[state] ?? 'reservations.state.PENDING')}
              </Badge>
            )}
          </Group>
        </Stack>
        {canEdit && (
          <Group gap="xs" wrap="nowrap" style={{ flexShrink: 0 }}>
            <Switch
              aria-label={t('reservations.enabled')}
              checked={reservation.enabled}
              disabled={inFlight}
              onChange={(e) => {
                const enabled = e.currentTarget.checked;
                setToggleError(false);
                commit({
                  variables: { id: reservation.id, enabled },
                  onCompleted: (_response, errors) =>
                    errors != null && errors.length > 0 ? setToggleError(true) : onChanged(enabled),
                  onError: () => setToggleError(true),
                });
              }}
            />
            {/* ルール予約は、消しても次の照合でまた作られるので、消さずに左のスイッチで無効にする (設計書 4.3)。 */}
            {/* 削除のない行でもスイッチの位置がそろうよう、削除の場所は空けておく。 */}
            <Box w={56}>
              {reservation.kind !== 'RULE' && (
                <Button size="xs" variant="subtle" color="red" onClick={onDelete}>
                  {t('reservations.delete')}
                </Button>
              )}
            </Box>
          </Group>
        )}
      </Group>
      <Box
        aria-hidden
        mt={6}
        style={{
          position: 'relative',
          height: 6,
          borderRadius: 3,
          background: 'var(--mantine-color-default-border)',
        }}
      >
        <Box
          style={{
            position: 'absolute',
            left: `${bar.left}%`,
            width: `${bar.width}%`,
            height: '100%',
            borderRadius: 3,
            background: barColor,
          }}
        />
      </Box>
      {toggleError && (
        <Text size="xs" c="red" role="alert" mt={4}>
          {t('reservations.toggleError')}
        </Text>
      )}
    </Card>
  );
}

type DeleteProps = {
  reservation: Reservation;
  onClose: () => void;
  // onDeleted は、予約を無効にしたなら true、消したなら false を受け取る。
  onDeleted: (disabled: boolean) => void;
};

// DeleteDialog は、予約を消すか、ルール予約なら無効にする前に確かめる。
function DeleteDialog({ reservation, onClose, onDeleted }: DeleteProps) {
  const { t } = useTranslation();
  const [error, setError] = useState(false);
  const [commit, inFlight] = useMutation<reservationsDeleteMutation>(graphql`
    mutation reservationsDeleteMutation($id: ID!) {
      deleteReservation(id: $id) {
        id
        reservation {
          id
          enabled
        }
      }
    }
  `);
  return (
    <Modal opened onClose={onClose} title={t('reservations.deleteTitle')}>
      <Stack>
        <Text size="sm">
          {reservation.name === '' ? t('reservations.untitled') : reservation.name}
        </Text>
        <Text size="sm" c="dimmed">
          {t('reservations.deleteDescription')}
        </Text>
        {error && (
          <Alert color="red" role="alert">
            {t('reservations.deleteError')}
          </Alert>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t('reservations.cancel')}
          </Button>
          <Button
            color="red"
            loading={inFlight}
            onClick={() =>
              commit({
                variables: { id: reservation.id },
                onCompleted: (response, errors) =>
                  errors != null && errors.length > 0
                    ? setError(true)
                    : onDeleted(response.deleteReservation.reservation !== null),
                onError: () => setError(true),
              })
            }
          >
            {t('reservations.delete')}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
