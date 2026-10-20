import {
  ActionIcon,
  Alert,
  Checkbox,
  Anchor,
  Box,
  Button,
  Card,
  Drawer,
  Group,
  Indicator,
  Modal,
  MultiSelect,
  Pill,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core';
import {
  CalendarDays,
  Clapperboard,
  Info,
  ListChecks,
  ListFilter,
  Search,
  SearchX,
  Shapes,
  Tag,
  Trash2,
  Tv,
} from 'lucide-react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
} from 'react-relay';

import { useDataChanges } from '../../changes/DataChanges';
import type { ChangeChannel } from '../../changes/dataChanges';
import { EmptyState, ErrorBoundary, PageLoader } from '../../futon';
import { genreKey } from '../../guide/genres';
import { broadcastRange } from '../../recordings/filterRange';
import { RecordingCard } from '../../recordings/RecordingCard';
import { recordingStates, stateKey } from '../../recordings/states';
import { isUnauthenticated } from '../../relay/environment';
import type {
  recordingsListQuery,
  recordingsListQuery$data,
  recordingsListQuery$variables,
} from './__generated__/recordingsListQuery.graphql';
import type { recordingsDeleteMutation } from './__generated__/recordingsDeleteMutation.graphql';
import type { recordingsTagsQuery } from './__generated__/recordingsTagsQuery.graphql';
import { renderServiceOption, serviceGroups } from '../../guide/serviceOption';

type RecordingsSearch = {
  // rule は、このルールで録った録画だけを出すときの、ルールの ID (#468)。
  rule?: string;
};

export const Route = createFileRoute('/recordings/')({
  validateSearch: (search: Record<string, unknown>): RecordingsSearch =>
    typeof search['rule'] === 'string' && search['rule'] !== '' ? { rule: search['rule'] } : {},
  component: RecordingsPage,
});

// genreOptions は、絞り込みで選べるジャンルの大分類 (ニュース・報道〜福祉と、その他)。
const genreOptions = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15];

// pageSize は、1 回に取る録画の数。
const pageSize = 48;

const query = graphql`
  query recordingsListQuery(
    $first: Int!
    $after: String
    $filter: RecordingFilter
    $order: RecordingOrder!
  ) {
    recordings(first: $first, after: $after, filter: $filter, order: $order) {
      edges {
        node {
          id
          name
          startAt
          endAt
          state
          protected
          drops
          errors
          scrambled
          recordedFrom
          recordedUntil
          gaps {
            from
            to
          }
          watchPositionMs
          thumbnailUrl
          previewUrl
          watchFrame {
            url
            x
            y
            width
            height
            sheetWidth
            sheetHeight
          }
          service {
            name
            logoUrl
          }
          tags {
            id
            name
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

type Recording = recordingsListQuery$data['recordings']['edges'][number]['node'];
type Filter = NonNullable<recordingsListQuery$variables['filter']>;
type Order = recordingsListQuery$variables['order'];

// RecordingsPage は、サムネイル付きの録画のグリッドの画面 (設計書 10.3)。タグと条件で絞り込める。
// 狭い画面では、絞り込みのフォームを一覧の上に置かず、見出しの右のボタンから、下から出るシートで開く。
function RecordingsPage() {
  const { t } = useTranslation();
  const { rule } = Route.useSearch();
  // formFilter は、フォームで決めた絞り込み。最初は絞り込まずに、新しい録画から出す。
  const [formFilter, setFilter] = useState<Filter>({});
  // ルールでの絞り込みは URL の rule から決め、ルールの画面から来たときや、戻ったときにも、URL に合わせる (#468)。
  const filter: Filter = rule === undefined ? formFilter : { ...formFilter, ruleId: rule };
  // order は、並び順。絞り込みではないので、選んだらすぐに並べ替える。
  const [order, setOrder] = useState<Order>('NEWEST_FIRST');
  const [filtersOpened, setFiltersOpened] = useState(false);
  // 閉じていても絞り込み中だと分かるよう、ボタンに、使っている条件の数を出す。
  const activeFilters = Object.keys(filter).length;

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Title order={2}>{t('recordings.title')}</Title>
        <Indicator label={activeFilters} size={16} disabled={activeFilters === 0} hiddenFrom="sm">
          <ActionIcon
            variant="default"
            size="lg"
            aria-label={
              activeFilters === 0
                ? t('recordings.filters')
                : t('recordings.filtersActive', { count: activeFilters })
            }
            onClick={() => setFiltersOpened(true)}
          >
            <ListFilter size={18} aria-hidden />
          </ActionIcon>
        </Indicator>
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
          <Recordings
            filter={filter}
            onFilter={setFilter}
            order={order}
            onOrder={setOrder}
            filtersOpened={filtersOpened}
            onFiltersClose={() => setFiltersOpened(false)}
          />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// Recordings は、絞り込みのフォーム。結果は、絞り込みを決めてから RecordingGrid で読む。
// フォームは、広い画面では一覧の上のカードに、狭い画面では下から出るシートに出す。入力中の値は両方で共有する。
function Recordings({
  filter,
  onFilter,
  order,
  onOrder,
  filtersOpened,
  onFiltersClose,
}: {
  filter: Filter;
  onFilter: (filter: Filter) => void;
  order: Order;
  onOrder: (order: Order) => void;
  filtersOpened: boolean;
  onFiltersClose: () => void;
}) {
  const { t } = useTranslation();
  // タグごとの録画の数も、録画が変わったら読み直す (#868)。
  const changes = useDataChanges(recordingChannels);
  const tags = useLazyLoadQuery<recordingsTagsQuery>(
    graphql`
      query recordingsTagsQuery($ruleId: ID!, $withRule: Boolean!) {
        viewer {
          role
        }
        ruleFilter: node(id: $ruleId) @include(if: $withRule) {
          __typename
          ... on Rule {
            name
          }
        }
        tags {
          id
          name
          recordings
        }
        services {
          id
          name
          logoUrl
          channelType {
            displayName
          }
        }
      }
    `,
    { ruleId: filter.ruleId ?? '', withRule: filter.ruleId != null },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const navigate = useNavigate({ from: '/recordings/' });
  const ruleName = tags.ruleFilter?.__typename === 'Rule' ? tags.ruleFilter.name : null;
  // version を変えると、一覧を読み直す。まとめて消した後に使う (#471)。
  const [version, setVersion] = useState(0);
  // deleted は、まとめて消した結果。読み直しても消えないよう、一覧の外に持つ。
  const [deleted, setDeleted] = useState<{ processed: number; skipped: number } | null>(null);
  const [keyword, setKeyword] = useState('');
  const [tagIds, setTagIds] = useState<string[]>([]);
  const [states, setStates] = useState<string[]>([]);
  const [serviceIds, setServiceIds] = useState<string[]>([]);
  const [genres, setGenres] = useState<string[]>([]);
  // from と to は、放送日の期間 (YYYY-MM-DD)。両端を含む。
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');

  const submit = (event: FormEvent) => {
    event.preventDefault();
    onFilter({
      ...(keyword.trim() !== '' ? { keyword: keyword.trim() } : {}),
      ...(tagIds.length > 0 ? { tagIds } : {}),
      ...(states.length > 0 ? { states: states as Filter['states'] } : {}),
      ...(serviceIds.length > 0 ? { serviceIds } : {}),
      ...(genres.length > 0 ? { genres: genres.map(Number) } : {}),
      ...broadcastRange(from, to),
    });
    onFiltersClose();
  };

  // fields は、絞り込みの欄。横に並べるときだけ、欄の幅を伸び縮みさせる。
  const fields = (inline: boolean) => {
    const style = inline ? { flex: '1 1 200px' } : undefined;
    return (
      <>
        <TextInput
          label={t('recordings.keyword')}
          leftSection={<Search size={16} aria-hidden />}
          value={keyword}
          onChange={(e) => setKeyword(e.currentTarget.value)}
          style={style}
        />
        <MultiSelect
          label={t('recordings.tags')}
          leftSection={<Tag size={16} aria-hidden />}
          data={tags.tags.map((tag) => ({
            value: tag.id,
            label: `${tag.name}（${tag.recordings}）`,
          }))}
          value={tagIds}
          onChange={setTagIds}
          searchable
          clearable
          style={style}
        />
        <MultiSelect
          label={t('recordings.states')}
          leftSection={<Info size={16} aria-hidden />}
          data={recordingStates.map((state) => ({ value: state, label: t(stateKey(state)) }))}
          value={states}
          onChange={setStates}
          clearable
          style={style}
        />
        <MultiSelect
          label={t('recordings.services')}
          leftSection={<Tv size={16} aria-hidden />}
          data={serviceGroups(tags.services)}
          renderOption={renderServiceOption(tags.services)}
          value={serviceIds}
          onChange={setServiceIds}
          searchable
          clearable
          style={style}
        />
        <MultiSelect
          label={t('recordings.genres')}
          leftSection={<Shapes size={16} aria-hidden />}
          data={genreOptions.map((lv1) => ({ value: String(lv1), label: t(genreKey(lv1)) }))}
          value={genres}
          onChange={setGenres}
          clearable
          style={style}
        />
        <TextInput
          type="date"
          label={t('recordings.from')}
          leftSection={<CalendarDays size={16} aria-hidden />}
          value={from}
          onChange={(e) => setFrom(e.currentTarget.value)}
          style={style}
        />
        <TextInput
          type="date"
          label={t('recordings.to')}
          leftSection={<CalendarDays size={16} aria-hidden />}
          value={to}
          onChange={(e) => setTo(e.currentTarget.value)}
          style={style}
        />
        <Button type="submit" leftSection={<ListFilter size={16} aria-hidden />}>
          {t('recordings.filter')}
        </Button>
      </>
    );
  };

  return (
    <Stack>
      <Card withBorder component="form" onSubmit={submit} visibleFrom="sm">
        <Group align="flex-end" wrap="wrap">
          {fields(true)}
        </Group>
      </Card>
      <Drawer
        opened={filtersOpened}
        onClose={onFiltersClose}
        position="bottom"
        title={t('recordings.filters')}
        styles={{
          // シートの高さは中身に合わせ、一覧が後ろに見えるようにする。
          content: {
            height: 'auto',
            maxHeight: '85dvh',
            borderRadius: 'var(--mantine-radius-md) var(--mantine-radius-md) 0 0',
          },
        }}
      >
        <Stack component="form" onSubmit={submit}>
          {fields(false)}
        </Stack>
      </Drawer>
      <Group justify={filter.ruleId != null ? 'space-between' : 'flex-end'}>
        {/* ルールの画面から来たときは、どのルールで絞っているかを出し、外せるようにする (#468)。 */}
        {filter.ruleId != null && (
          <Pill
            withRemoveButton
            removeButtonProps={{ 'aria-label': t('recordings.ruleFilterClear') }}
            onRemove={() => void navigate({ search: {} })}
          >
            {t('recordings.ruleFilter', { name: ruleName ?? t('recordings.deletedRule') })}
          </Pill>
        )}
        <SegmentedControl
          aria-label={t('recordings.order')}
          size="xs"
          value={order}
          onChange={(value) => onOrder(value as Order)}
          data={[
            { value: 'NEWEST_FIRST', label: t('recordings.newestFirst') },
            { value: 'OLDEST_FIRST', label: t('recordings.oldestFirst') },
          ]}
        />
      </Group>
      {deleted !== null && (
        <Alert color="teal" role="status" withCloseButton onClose={() => setDeleted(null)}>
          {deleted.skipped === 0
            ? t('recordings.bulkDeleted', { count: deleted.processed })
            : t('recordings.bulkDeletedSkipped', {
                count: deleted.processed,
                skipped: deleted.skipped,
              })}
        </Alert>
      )}
      <Suspense fallback={<PageLoader />}>
        <RecordingGrid
          key={`${order}:${JSON.stringify(filter)}:${version}`}
          filter={filter}
          order={order}
          canEdit={tags.viewer.role !== 'VIEWER'}
          onDeleted={(result) => {
            setDeleted(result);
            setVersion((n) => n + 1);
          }}
        />
      </Suspense>
    </Stack>
  );
}

// recordingChannels は、録画の一覧に関わるデータの種類。
const recordingChannels: readonly ChangeChannel[] = ['RECORDINGS'];

// RecordingGrid は、絞り込んだ録画のグリッド。続きは、同じ絞り込みで、カーソルから読む。
function RecordingGrid({
  filter,
  order,
  canEdit,
  onDeleted,
}: {
  filter: Filter;
  order: Order;
  canEdit: boolean;
  onDeleted: (result: { processed: number; skipped: number }) => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // selecting は、まとめて消す録画を選んでいるかどうか。selected は、選んだ録画の ID (#471)。
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [confirming, setConfirming] = useState(false);
  const [deleteMany, deleting] = useMutation<recordingsDeleteMutation>(graphql`
    mutation recordingsDeleteMutation($ids: [ID!]!) {
      deleteRecordings(ids: $ids) {
        processed
        skipped
      }
    }
  `);
  const toggle = (id: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (!next.delete(id)) {
        next.add(id);
      }

      return next;
    });
  const remove = () =>
    deleteMany({
      variables: { ids: [...selected] },
      onCompleted: (response, errors) => {
        setConfirming(false);
        if (errors != null && errors.length > 0) {
          setError(true);

          return;
        }
        onDeleted(response.deleteRecordings);
      },
      onError: () => {
        setConfirming(false);
        setError(true);
      },
    });
  // 録画を始めた、終えた、消したなど、録画が変わったら、最初のページを読み直す (#868)。
  const changes = useDataChanges(recordingChannels);
  const data = useLazyLoadQuery<recordingsListQuery>(
    query,
    { first: pageSize, after: null, filter, order },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  // more は、続きの読み込みで足した録画と、その次のカーソル。changes は、読み込んだときの録画の変更の回数。
  // 録画が変わって最初のページを読み直したら、続きは捨てて、最初のページから出し直す。
  // 続きを残すと、録画が増えて最初のページからこぼれた録画が、どちらにも出なくなる (#868)。
  const [loaded, setMore] = useState<{
    changes: number;
    recordings: Recording[];
    next: string | null;
  } | null>(null);
  const more = loaded?.changes === changes ? loaded : null;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const recordings = [
    ...data.recordings.edges.map((edge) => edge.node),
    ...(more?.recordings ?? []),
  ];
  const next =
    more !== null
      ? more.next
      : data.recordings.pageInfo.hasNextPage
        ? (data.recordings.pageInfo.endCursor ?? null)
        : null;

  const loadMore = async () => {
    setLoading(true);
    setError(false);
    try {
      const page = await fetchQuery<recordingsListQuery>(environment, query, {
        first: pageSize,
        after: next,
        filter,
        order,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          changes,
          recordings: [
            ...(more?.recordings ?? []),
            ...page.recordings.edges.map((edge) => edge.node),
          ],
          next: page.recordings.pageInfo.hasNextPage
            ? (page.recordings.pageInfo.endCursor ?? null)
            : null,
        });
      }
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Stack>
      {/* 録画がまだないときと、絞り込んで 0 件になったときで、次にすることが違うので、見出しと手がかりを分ける。 */}
      {recordings.length === 0 &&
        (Object.keys(filter).length === 0 ? (
          <EmptyState
            icon={Clapperboard}
            title={t('recordings.empty')}
            hint={t('recordings.emptyHint')}
          />
        ) : (
          <EmptyState
            icon={SearchX}
            title={t('recordings.noMatch')}
            hint={t('recordings.noMatchHint')}
          />
        ))}
      {/* 視聴者でなければ、録画を選んでまとめて消せる。保護した録画と録画中の録画は選べない (#471)。 */}
      {canEdit && recordings.length > 0 && (
        <Group gap="xs">
          <Button
            size="xs"
            variant={selecting ? 'light' : 'default'}
            leftSection={<ListChecks size={14} aria-hidden />}
            onClick={() => {
              setSelecting(!selecting);
              setSelected(new Set());
            }}
          >
            {t(selecting ? 'recordings.selectDone' : 'recordings.select')}
          </Button>
          {selecting && (
            <Button
              size="xs"
              color="red"
              variant="light"
              leftSection={<Trash2 size={14} aria-hidden />}
              disabled={selected.size === 0}
              onClick={() => setConfirming(true)}
            >
              {t('recordings.deleteSelected', { count: selected.size })}
            </Button>
          )}
        </Group>
      )}
      {recordings.length > 0 && (
        <SimpleGrid cols={{ base: 2, sm: 3, lg: 4 }} spacing="sm">
          {recordings.map((recording) => {
            const locked = recording.protected || recording.state === 'RECORDING';

            return (
              <Box key={recording.id} pos="relative">
                {/* 選んでいる間は、カードのリンクにキーボードでも移れないようにし、選ぶ操作だけを受ける。 */}
                <Box inert={selecting}>
                  <RecordingCard recording={recording} />
                </Box>
                {selecting && (
                  <SelectionOverlay
                    label={recording.name}
                    checked={selected.has(recording.id)}
                    disabledReason={
                      locked
                        ? t(
                            recording.protected
                              ? 'recordings.selectProtected'
                              : 'recordings.selectRecording',
                          )
                        : null
                    }
                    onToggle={() => toggle(recording.id)}
                  />
                )}
              </Box>
            );
          })}
        </SimpleGrid>
      )}
      <Modal
        opened={confirming}
        onClose={() => setConfirming(false)}
        title={t('recordings.deleteSelectedTitle')}
      >
        <Stack>
          <Text size="sm">{t('recordings.deleteSelectedConfirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirming(false)}>
              {t('recordings.cancel')}
            </Button>
            <Button color="red" loading={deleting} onClick={remove}>
              {t('recordings.deleteSelected', { count: selected.size })}
            </Button>
          </Group>
        </Stack>
      </Modal>
      {error && (
        <Alert color="red" role="alert">
          {t('recordings.error')}
        </Alert>
      )}
      {next !== null && (
        <Box>
          <Button variant="default" loading={loading} onClick={() => void loadMore()}>
            {t('recordings.more')}
          </Button>
        </Box>
      )}
    </Stack>
  );
}

// SelectionOverlay は、録画を選ぶときに、カードの上に重ねる選択の印。カードを押しても、詳細へ移らずに選ぶ。
// 選べない録画は、理由を出して押せないようにする (#471)。
function SelectionOverlay({
  label,
  checked,
  disabledReason,
  onToggle,
}: {
  label: string;
  checked: boolean;
  disabledReason: string | null;
  onToggle: () => void;
}) {
  const disabled = disabledReason !== null;

  return (
    <Tooltip label={disabledReason} disabled={!disabled} withinPortal>
      <UnstyledButton
        pos="absolute"
        inset={0}
        role="checkbox"
        aria-checked={checked}
        aria-label={label}
        aria-disabled={disabled}
        onClick={disabled ? undefined : onToggle}
        style={{
          borderRadius: 'var(--mantine-radius-default)',
          cursor: disabled ? 'not-allowed' : 'pointer',
          background: disabled
            ? 'color-mix(in srgb, var(--mantine-color-body) 60%, transparent)'
            : checked
              ? 'color-mix(in srgb, var(--mantine-primary-color-filled) 18%, transparent)'
              : undefined,
          outline: checked ? '2px solid var(--mantine-primary-color-filled)' : undefined,
        }}
      >
        {/* ボタンの中に入力欄を入れないよう、見た目だけの印にする。 */}
        <Checkbox.Indicator
          checked={checked}
          disabled={disabled}
          aria-hidden
          pos="absolute"
          top={8}
          left={8}
        />
      </UnstyledButton>
    </Tooltip>
  );
}
