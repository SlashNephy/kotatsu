import {
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Box,
  Button,
  Checkbox,
  CloseButton,
  Group,
  Menu,
  Modal,
  SegmentedControl,
  Stack,
  Tabs,
  Text,
  Title,
  Tooltip,
} from '@mantine/core';
import {
  CalendarCheck,
  CalendarX,
  CirclePause,
  CirclePlay,
  Clock,
  EllipsisVertical,
  ListChecks,
  PencilLine,
  Plus,
  SlidersHorizontal,
  Trash2,
  type LucideIcon,
} from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { startTransition, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
} from 'react-relay';

import { useDataChanges } from '../changes/DataChanges';
import type { DataChannel } from '../changes/dataChanges';
import { EmptyState, ErrorBoundary, PageLoader, ProgramTitle, toHalfWidth } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { isUnauthenticated } from '../relay/environment';
import { RecordingStateBadge } from '../recordings/RecordingStateBadge';
import { groupByDay, needsAttention } from '../reservations/timeline';
import type { reservationsDeleteManyMutation } from './__generated__/reservationsDeleteManyMutation.graphql';
import type { reservationsDeleteMutation } from './__generated__/reservationsDeleteMutation.graphql';
import type { reservationsEnableMutation } from './__generated__/reservationsEnableMutation.graphql';
import type { reservationsSetManyEnabledMutation } from './__generated__/reservationsSetManyEnabledMutation.graphql';
import type {
  reservationsQuery,
  reservationsQuery$data,
} from './__generated__/reservationsQuery.graphql';
import { ReservationSettingsDialog } from '../reservations/ReservationSettingsDialog';
import { ServiceLabel } from '../guide/ServiceLabel';
import { WatchReservationList } from '../watch/WatchReservationList';
import { TimeReservationDialog } from '../reservations/TimeReservationDialog';
import classes from './reservations.module.css';
import { zonedFormat } from '../futon';

// query は、時間帯に重なる予約を取る。続きは、同じ時間帯と状態で、カーソルから読む。
const query = graphql`
  query reservationsQuery(
    $from: Time!
    $to: Time!
    $states: [AssignmentState!]
    $rule: ID
    $ruleId: ID!
    $withRule: Boolean!
    $first: Int!
    $after: String
  ) {
    viewer {
      role
    }
    # 絞り込んでいるルールの名前を出す。
    ruleFilter: node(id: $ruleId) @include(if: $withRule) {
      __typename
      ... on Rule {
        name
      }
    }
    reservations(from: $from, to: $to, states: $states, rule: $rule, first: $first, after: $after) {
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
            logoUrl
            channelType {
              type
            }
          }
          assignment {
            state
            tuner
            plannedStartAt
            plannedEndAt
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
  // watch は、録画の予約の代わりに、視聴予約の一覧を出すかどうか (設計書 4.8)。
  watch?: boolean;
  // problems は、録画が欠ける見込みの予約だけを出すかどうか。
  problems?: boolean;
  // rule は、このルールに一致した予約だけを出すときの、ルールの ID (#468)。
  rule?: string;
};

export const Route = createFileRoute('/reservations')({
  validateSearch: (search: Record<string, unknown>): ReservationsSearch => ({
    ...(search['watch'] === true ? { watch: true } : {}),
    ...(search['problems'] === true ? { problems: true } : {}),
    ...(typeof search['rule'] === 'string' && search['rule'] !== ''
      ? { rule: search['rule'] }
      : {}),
  }),
  component: ReservationsPage,
});

const hour = 3_600_000;
// rangeDays は、一覧に出す、いまからの日数。
const rangeDays = 8;
// pageSize は、1 回に取る件数。一度に並べすぎないよう、続きは「さらに読み込む」で足す。
const pageSize = 25;

// reservationChannels は、予約の一覧に関わるデータの種類。変わったら、一覧を読み直す。
const reservationChannels: readonly DataChannel[] = [
  'RESERVATIONS',
  'ASSIGNMENTS',
  'RESERVATION_CONFLICTS',
];

const dayFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// kinds は、予約の種類ごとの、行の先頭のアイコンと名前。ルール予約と手動の予約を見分けられるようにする。
// ルール予約は、ルールの画面と同じアイコンにする。
const kinds: Record<
  string,
  {
    icon: LucideIcon;
    key: 'reservations.kind.PROGRAM' | 'reservations.kind.TIME' | 'reservations.kind.RULE';
  }
> = {
  PROGRAM: { icon: CalendarCheck, key: 'reservations.kind.PROGRAM' },
  TIME: { icon: Clock, key: 'reservations.kind.TIME' },
  RULE: { icon: ListChecks, key: 'reservations.kind.RULE' },
};

type Node = reservationsQuery$data['reservations']['edges'][number]['node'];
type Reservation = Node & { readonly startMs: number; readonly endMs: number };

// ReservationsPage は、終わっていない予約の一覧 (設計書 10.3)。放送中の予約を先頭に、続けて日ごとに並べる。
// 終わった予約は出さない。録画の結果は、録画の画面で見る。
function ReservationsPage() {
  const { t } = useTranslation();
  // 画面を開いた時刻から数える。放送中の予約も、終わる前なので時間帯に重なって入る。
  // 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。
  const [range] = useState(() => {
    const now = Date.now();

    return {
      now,
      from: new Date(now).toISOString(),
      to: new Date(now + rangeDays * 24 * hour).toISOString(),
    };
  });

  const { watch } = Route.useSearch();
  const navigate = Route.useNavigate();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('reservations.title')}</Title>
      {/* 録画の予約は家族で共有し、視聴予約はユーザーごとのものなので、タブで分ける (設計書 4.8)。 */}
      <Tabs
        value={watch === true ? 'watch' : 'recording'}
        onChange={(value) => void navigate({ search: value === 'watch' ? { watch: true } : {} })}
      >
        <Tabs.List>
          <Tabs.Tab value="recording">{t('reservations.tab')}</Tabs.Tab>
          <Tabs.Tab value="watch">{t('watch.tab')}</Tabs.Tab>
        </Tabs.List>
      </Tabs>
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
          {watch === true ? <WatchReservationList /> : <Reservations range={range} />}
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Reservations({ range }: { range: { now: number; from: string; to: string } }) {
  const { t } = useTranslation();
  const navigate = Route.useNavigate();
  const { problems = false, rule } = Route.useSearch();
  // patches は、この画面で変えた予約。一覧を取り直さずに反映し、読み込んだ続きを捨てない。
  const [patches, setPatches] = useState<Record<string, { enabled?: boolean; removed?: boolean }>>(
    {},
  );
  const patch = (id: string, change: { enabled?: boolean; removed?: boolean }) =>
    setPatches((prev) => ({ ...prev, [id]: { ...prev[id], ...change } }));
  const variables = {
    from: range.from,
    to: range.to,
    states: problems ? (['PARTIAL', 'CONFLICT'] as const) : null,
    rule: rule ?? null,
    ruleId: rule ?? '',
    withRule: rule !== undefined,
  };
  // changes は、予約、割り当て、競合が、ほかの端末や neko で変わるたびに変わる (#503)。変わったら読み直す。
  const changes = useDataChanges(reservationChannels);
  // filterKey は、絞り込みの組。変わったら、読み込んだ続きを捨てる。データが変わったときも、続きは古くなるので捨てる。
  const filterKey = `${problems}:${rule ?? ''}:${changes}`;
  // fetchKey を変えると、読み直す。時刻予約を作った後に、一覧に足すのに使う。読み直す間も一覧を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // creatingTime は、時刻予約を作るダイアログを出しているかどうか。
  const [creatingTime, setCreatingTime] = useState(false);
  const data = useLazyLoadQuery<reservationsQuery>(
    query,
    { ...variables, first: pageSize, after: null },
    { fetchKey: `${fetchKey}:${changes}`, fetchPolicy: 'store-and-network' },
  );
  const environment = useRelayEnvironment();
  // more は、続きの読み込みで足した予約と、その次のカーソル。絞り込みを変えたら捨てる。
  const [more, setMore] = useState<{ key: string; nodes: Node[]; next: string | null } | null>(
    null,
  );
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState(false);
  // reload は、一覧を読み直す。時刻予約を作ったり時刻を変えたりすると、並びと割り当てが変わる。
  // 読み込んだ続きは、読み直しで並びが変わるので捨てる。
  const reload = () => {
    setMore(null);
    startTransition(() => setFetchKey((key) => key + 1));
  };
  const extra = more?.key === filterKey ? more : null;
  const next = extra !== null ? extra.next : (data.reservations.pageInfo.endCursor ?? null);
  const hasMore = extra !== null ? extra.next !== null : data.reservations.pageInfo.hasNextPage;
  const loadMore = async () => {
    setLoadingMore(true);
    setMoreError(false);
    try {
      const page = await fetchQuery<reservationsQuery>(environment, query, {
        ...variables,
        first: pageSize,
        after: next,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          key: filterKey,
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
  // selecting は、まとめて操作する予約を選んでいるかどうか。selected は、選んだ予約の ID (#471)。
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [bulkResult, setBulkResult] = useState<{
    action: 'enabled' | 'disabled' | 'deleted';
    processed: number;
    skipped: number;
  } | null>(null);
  const [bulkError, setBulkError] = useState(false);
  const [setManyEnabled, settingMany] = useMutation<reservationsSetManyEnabledMutation>(graphql`
    mutation reservationsSetManyEnabledMutation($ids: [ID!]!, $enabled: Boolean!) {
      setReservationsEnabled(ids: $ids, enabled: $enabled) {
        processed
        skipped
      }
    }
  `);
  const [deleteMany, deletingMany] = useMutation<reservationsDeleteManyMutation>(graphql`
    mutation reservationsDeleteManyMutation($ids: [ID!]!) {
      deleteReservations(ids: $ids) {
        processed
        skipped
      }
    }
  `);
  const toggleSelected = (id: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (!next.delete(id)) {
        next.add(id);
      }

      return next;
    });
  // finishBulk は、まとめて行った操作を終える。ルール予約は消さずに無効にし、録画中の予約は飛ばすので、一覧は読み直す。
  const finishBulk = (
    action: 'enabled' | 'disabled' | 'deleted',
    result: { processed: number; skipped: number },
  ) => {
    setBulkResult({ action, ...result });
    setSelecting(false);
    setSelected(new Set());
    setConfirmingDelete(false);
    reload();
  };
  const bulkFailed = () => {
    setConfirmingDelete(false);
    setBulkError(true);
  };
  const setEnabledMany = (enabled: boolean) => {
    setBulkError(false);
    setManyEnabled({
      variables: { ids: [...selected], enabled },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0
          ? bulkFailed()
          : finishBulk(enabled ? 'enabled' : 'disabled', response.setReservationsEnabled),
      onError: bulkFailed,
    });
  };
  const removeMany = () => {
    setBulkError(false);
    deleteMany({
      variables: { ids: [...selected] },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0
          ? bulkFailed()
          : finishBulk('deleted', response.deleteReservations),
      onError: bulkFailed,
    });
  };
  const reservations: Reservation[] = [
    ...data.reservations.edges.map((edge) => edge.node),
    ...(extra?.nodes ?? []),
  ]
    .filter((node) => patches[node.id]?.removed !== true)
    .map((node) => ({
      ...node,
      enabled: patches[node.id]?.enabled ?? node.enabled,
      startMs: Date.parse(node.startAt),
      endMs: Date.parse(node.endAt),
    }));
  // 放送中の予約は、日付の見出しの前にまとめる。始まった日が前の日でも、いま録っていると分かるようにする。
  const onAir = reservations.filter((reservation) => reservation.startMs <= range.now);
  const groups = groupByDay(reservations.filter((reservation) => reservation.startMs > range.now));
  const attention = reservations.filter((reservation) =>
    needsAttention(reservation.assignment.state, reservation.enabled),
  ).length;
  const row = (reservation: Reservation) => (
    <ReservationRow
      key={reservation.id}
      reservation={reservation}
      onAir={reservation.startMs <= range.now}
      canEdit={canEdit}
      onChanged={(enabled) => patch(reservation.id, { enabled })}
      onRescheduled={reload}
      onDelete={() => setDeleting(reservation)}
      selection={
        selecting
          ? {
              checked: selected.has(reservation.id),
              onToggle: () => toggleSelected(reservation.id),
            }
          : undefined
      }
    />
  );

  return (
    <Stack>
      <Group justify="space-between" wrap="wrap">
        <SegmentedControl
          size="xs"
          value={problems ? 'problems' : 'all'}
          onChange={(value) =>
            void navigate({
              search: (prev) => ({ ...prev, problems: value === 'problems' ? true : undefined }),
            })
          }
          data={[
            { value: 'all', label: t('reservations.filter.all') },
            { value: 'problems', label: t('reservations.filter.problems') },
          ]}
        />
        {rule !== undefined && (
          // Pill の消すボタンは、キーボードと読み上げから隠れるので、CloseButton を添える。
          <Badge
            size="lg"
            variant="light"
            color="gray"
            tt="none"
            rightSection={
              <CloseButton
                size="xs"
                aria-label={t('reservations.filter.clearRule')}
                onClick={() => void navigate({ search: (prev) => ({ ...prev, rule: undefined }) })}
              />
            }
          >
            {t('reservations.filter.rule', {
              name:
                data.ruleFilter?.__typename === 'Rule'
                  ? data.ruleFilter.name
                  : t('reservations.filter.unknownRule'),
            })}
          </Badge>
        )}
        {!problems && attention > 0 && (
          <Badge color="amber" size="lg" variant="light">
            {t('reservations.attention', { count: attention })}
          </Badge>
        )}
        {canEdit && (
          <Group gap="xs">
            {reservations.length > 0 && (
              <Button
                size="xs"
                variant={selecting ? 'light' : 'default'}
                leftSection={<ListChecks size={14} aria-hidden />}
                onClick={() => {
                  setSelecting(!selecting);
                  setSelected(new Set());
                }}
              >
                {t(selecting ? 'reservations.selectDone' : 'reservations.select')}
              </Button>
            )}
            <Button
              size="xs"
              variant="default"
              leftSection={<Plus size={14} aria-hidden />}
              onClick={() => setCreatingTime(true)}
            >
              {t('reservations.time.open')}
            </Button>
          </Group>
        )}
      </Group>
      {selecting && (
        <Group gap="xs">
          <Button
            size="xs"
            variant="default"
            disabled={selected.size === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(true)}
          >
            {t('reservations.enableSelected', { count: selected.size })}
          </Button>
          <Button
            size="xs"
            variant="default"
            disabled={selected.size === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(false)}
          >
            {t('reservations.disableSelected', { count: selected.size })}
          </Button>
          <Button
            size="xs"
            color="red"
            variant="light"
            leftSection={<Trash2 size={14} aria-hidden />}
            disabled={selected.size === 0}
            onClick={() => setConfirmingDelete(true)}
          >
            {t('reservations.deleteSelected', { count: selected.size })}
          </Button>
        </Group>
      )}
      {bulkResult !== null && (
        <Alert color="teal" role="status" withCloseButton onClose={() => setBulkResult(null)}>
          {t(`reservations.bulk.${bulkResult.action}`, { count: bulkResult.processed })}
          {bulkResult.skipped > 0 && t('reservations.bulk.skipped', { count: bulkResult.skipped })}
        </Alert>
      )}
      {bulkError && (
        <Alert color="red" role="alert">
          {t('reservations.toggleError')}
        </Alert>
      )}
      <Modal
        opened={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        title={t('reservations.deleteSelectedTitle')}
      >
        <Stack>
          <Text size="sm">{t('reservations.deleteSelectedConfirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirmingDelete(false)}>
              {t('reservations.settings.cancel')}
            </Button>
            <Button color="red" loading={deletingMany} onClick={removeMany}>
              {t('reservations.deleteSelected', { count: selected.size })}
            </Button>
          </Group>
        </Stack>
      </Modal>
      {creatingTime && (
        <TimeReservationDialog
          onClose={() => setCreatingTime(false)}
          onSaved={() => {
            setCreatingTime(false);
            reload();
          }}
        />
      )}
      {reservations.length === 0 && (
        <EmptyState
          icon={CalendarX}
          title={problems ? t('reservations.noProblems') : t('reservations.empty')}
          hint={problems ? undefined : t('reservations.emptyHint')}
        />
      )}
      {reservations.length > 0 && (
        // 行をまたいで列を揃えるため、見出しも行も 1 つのグリッドに並べる。
        <Box className={classes.list}>
          {onAir.length > 0 && (
            <>
              <Text fw={700} className={classes.heading}>
                {t('reservations.onAir')}
              </Text>
              {onAir.map(row)}
            </>
          )}
          {groups.map((group) => (
            <Box key={group.from} className={classes.group}>
              <Text fw={700} className={classes.heading}>
                {dayFormat.format(group.from)}
              </Text>
              {group.items.map(row)}
            </Box>
          ))}
        </Box>
      )}
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
          onDeleted={() => {
            patch(deleting.id, { removed: true });
            setDeleting(null);
          }}
        />
      )}
    </Stack>
  );
}

type RowProps = {
  reservation: Reservation;
  onAir: boolean;
  canEdit: boolean;
  onChanged: (enabled: boolean) => void;
  // onRescheduled は、時刻予約の時刻を変えたときに呼ぶ。並びと割り当てが変わるので、一覧を読み直す。
  onRescheduled: () => void;
  onDelete: () => void;
  // selection は、まとめて操作する予約を選んでいるときの、この行の選択 (#471)。選んでいなければ undefined。
  selection?: { checked: boolean; onToggle: () => void };
};

// plannedRange は、一部のみ録画する予約の、マージンを削った後に録画する予定の時刻を返す (#466)。
// 録れない範囲が分かるよう、一部のみの有効な予約でだけ返す。
function plannedRange(reservation: Reservation): { start: number; end: number } | null {
  const { state, plannedStartAt, plannedEndAt } = reservation.assignment;
  if (
    state !== 'PARTIAL' ||
    !reservation.enabled ||
    plannedStartAt == null ||
    plannedEndAt == null
  ) {
    return null;
  }

  return { start: Date.parse(plannedStartAt), end: Date.parse(plannedEndAt) };
}

// ReservationRow は、予約の 1 行。番組名を押すと、番組表でその番組の詳細を開く。
// 状態は、目を留めたいもの (録画中、競合、一部のみ、無効) だけをバッジで出す。録画できる見込みは当たり前なので出さない。
function ReservationRow({
  reservation,
  onAir,
  canEdit,
  onChanged,
  onRescheduled,
  onDelete,
  selection,
}: RowProps) {
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
  const [editing, setEditing] = useState(false);
  // editingTime は、時刻予約の時刻と名前を変えるダイアログを出しているかどうか。
  const [editingTime, setEditingTime] = useState(false);
  const state = reservation.assignment.state;
  const attention = needsAttention(state, reservation.enabled);
  const planned = plannedRange(reservation);
  const recording = onAir && reservation.enabled && state !== 'CONFLICT';
  const kind = kinds[reservation.kind] ?? kinds['PROGRAM']!;
  const KindIcon = kind.icon;
  const toggle = () => {
    const enabled = !reservation.enabled;
    setToggleError(false);
    commit({
      variables: { id: reservation.id, enabled },
      onCompleted: (_response, errors) =>
        errors != null && errors.length > 0 ? setToggleError(true) : onChanged(enabled),
      onError: () => setToggleError(true),
    });
  };

  return (
    <Box
      className={classes.row}
      data-state={state}
      data-attention={attention || undefined}
      data-disabled={!reservation.enabled || undefined}
    >
      {/* 選んでいる間は、種類の印の代わりに、選ぶ欄を出す。録画中の予約は、録画を止めないよう選べない (#471)。 */}
      {selection !== undefined ? (
        <Box className={`${classes.kind} ${classes.select}`}>
          <Tooltip label={t('reservations.selectRecording')} disabled={!recording}>
            <Checkbox
              aria-label={reservation.name === '' ? t('reservations.untitled') : reservation.name}
              checked={selection.checked}
              disabled={recording}
              onChange={selection.onToggle}
            />
          </Tooltip>
        </Box>
      ) : (
        <Box className={classes.kind} title={t(kind.key)}>
          <KindIcon size={16} role="img" aria-label={t(kind.key)} />
        </Box>
      )}
      <Box className={classes.time}>
        <Text size="sm" c="dimmed">
          {timeFormat.format(reservation.startMs)}〜{timeFormat.format(reservation.endMs)}
        </Text>
        {planned !== null && (
          <Text size="xs" c="amber.8">
            {t('reservations.planned', {
              start: timeFormat.format(planned.start),
              end: timeFormat.format(planned.end),
            })}
          </Text>
        )}
      </Box>
      <Text size="sm" truncate className={classes.title}>
        <Link
          className={classes.link}
          to="/guide"
          search={{
            date: dateOf(broadcastDay(reservation.startMs).from),
            type: reservation.service.channelType.type,
            reservation: reservation.id,
          }}
        >
          {reservation.name === '' ? (
            t('reservations.untitled')
          ) : (
            <ProgramTitle name={reservation.name} />
          )}
        </Link>
      </Text>
      {/* バッジのない行でも、サービスを同じ列に置くよう、空の枠を残す。 */}
      <Group gap={4} wrap="nowrap" className={classes.badges}>
        {recording && <RecordingStateBadge state="RECORDING" size="xs" />}
        {!reservation.enabled ? (
          <Badge size="xs" color="gray">
            {t('reservations.disabled')}
          </Badge>
        ) : (
          attention && (
            // 行では短く出し、詳しい状態は、ホバーで見せる。
            <Badge
              size="xs"
              color="amber"
              variant="light"
              title={t(
                state === 'CONFLICT' ? 'reservations.state.CONFLICT' : 'reservations.state.PARTIAL',
              )}
            >
              {t(
                state === 'CONFLICT' ? 'reservations.badge.CONFLICT' : 'reservations.badge.PARTIAL',
              )}
            </Badge>
          )
        )}
      </Group>
      <Text
        size="xs"
        c="dimmed"
        truncate
        maw="14em"
        title={toHalfWidth(reservation.service.name)}
        className={classes.service}
      >
        <ServiceLabel
          name={reservation.service.name}
          logoUrl={reservation.service.logoUrl}
          logoOnlyNarrow
        />
      </Text>
      <Box className={classes.actions}>
        {canEdit && (
          <Menu position="bottom-end" withinPortal>
            <Menu.Target>
              <ActionIcon
                variant="subtle"
                color="gray"
                aria-label={t('reservations.actions')}
                loading={inFlight}
              >
                <EllipsisVertical size={16} aria-hidden />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item
                leftSection={<SlidersHorizontal size={14} aria-hidden />}
                onClick={() => setEditing(true)}
              >
                {t('reservations.settings.open')}
              </Menu.Item>
              {/* 番組予約とルール予約の時刻と名前は、EPG に従うので変えられない。 */}
              {reservation.kind === 'TIME' && (
                <Menu.Item
                  leftSection={<PencilLine size={14} aria-hidden />}
                  onClick={() => setEditingTime(true)}
                >
                  {t('reservations.time.edit')}
                </Menu.Item>
              )}
              <Menu.Item
                leftSection={
                  reservation.enabled ? (
                    <CirclePause size={14} aria-hidden />
                  ) : (
                    <CirclePlay size={14} aria-hidden />
                  )
                }
                onClick={toggle}
              >
                {t(reservation.enabled ? 'reservations.disable' : 'reservations.enable')}
              </Menu.Item>
              {/* ルール予約は、消しても次の照合でまた作られるので、消さずに無効にする (設計書 4.3)。 */}
              {reservation.kind !== 'RULE' && (
                <Menu.Item
                  color="red"
                  leftSection={<Trash2 size={14} aria-hidden />}
                  onClick={onDelete}
                >
                  {t('reservations.delete')}
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        )}
      </Box>
      {toggleError && (
        <Text size="xs" c="red" role="alert" className={classes.error}>
          {t('reservations.toggleError')}
        </Text>
      )}
      {editing && (
        <ReservationSettingsDialog id={reservation.id} onClose={() => setEditing(false)} />
      )}
      {editingTime && (
        <TimeReservationDialog
          reservation={reservation}
          onClose={() => setEditingTime(false)}
          onSaved={() => {
            setEditingTime(false);
            onRescheduled();
          }}
        />
      )}
    </Box>
  );
}

type DeleteProps = {
  reservation: Reservation;
  onClose: () => void;
  onDeleted: () => void;
};

// DeleteDialog は、番組予約や時刻予約を消す前に確かめる。ルール予約は、行のスイッチで無効にする。
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
          {reservation.name === '' ? (
            t('reservations.untitled')
          ) : (
            <ProgramTitle name={reservation.name} />
          )}
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
                onCompleted: (_response, errors) =>
                  errors != null && errors.length > 0 ? setError(true) : onDeleted(),
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
