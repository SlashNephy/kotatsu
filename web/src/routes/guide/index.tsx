import {
  Alert,
  Anchor,
  Box,
  Button,
  Checkbox,
  Group,
  LoadingOverlay,
  Menu,
  SegmentedControl,
  Select,
  Stack,
  Title,
} from '@mantine/core';
import { CalendarDays, CalendarX2, ChevronLeft, ChevronRight, Clock, Sunrise } from 'lucide-react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense, useDeferredValue, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { useDataChanges } from '../../changes/DataChanges';
import type { ChangeChannel } from '../../changes/dataChanges';
import { EmptyState, ErrorBoundary, fromWallClock, PageLoader } from '../../futon';
import { loadExtendedHours, saveExtendedHours } from '../../guide/hourNotation';
import { GuideGrid, type GuideColumn } from '../../guide/GuideGrid';
import { GuideViewEditor, type EditableGuideView } from '../../guide/GuideViewEditor';
import { broadcastDay, dateOf, receivedUntil } from '../../guide/layout';
import { PinButton } from '../../guide/PinButton';
import { ProgramDetail } from '../../guide/ProgramDetail';
import { collapseSharedColumns } from '../../guide/sharedEvent';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { guideQuery } from './__generated__/guideQuery.graphql';
import classes from './index.module.css';
import { zonedFormat } from '../../futon';

type GuideSearch = {
  // date は、表示する放送日 (YYYY-MM-DD)。省くと、いまの放送日。
  date?: string;
  // type は、表示するチャンネル種別 (GR など)。省くと、最初の種別。
  type?: string;
  // program は、詳細を開いている番組の ID。
  program?: string;
  // view は、適用する番組表ビューの ID。省くと、絞り込まない。
  view?: string;
  // reservation は、詳細を開きたい番組の予約の ID。番組表を読んだら、その予約の番組を program にして外す。
  // ホームのこれからの予約から開くときに使う。予約からは番組の ID が分からないので、番組表の中から探す。
  reservation?: string;
};

const datePattern = /^\d{4}-\d{2}-\d{2}$/;
const day = 24 * 60 * 60_000;
// pinnedTab は、ピン留めしたサービスを出すタブの値 (#485)。チャンネル種別とぶつからないよう、小文字にする。
const pinnedTab = 'pinned';
// guideDays は、番組表で移れる、いまの放送日からの日数。
// 放送の EPG (EIT) は、今日の 0 時から 8 日分が届く。放送日は 4 時に切り替わるので、8 日目の放送日も 4 時から 24 時までは番組がある。
const guideDays = 8;

// guideChannels は、番組表に関わるデータの種類。番組と、番組に付ける予約の印。
// 番組表ビューは、ログイン中のユーザーの設定なので、ほかの端末で保存したら読み直す。
const guideChannels: readonly ChangeChannel[] = ['EPG', 'RESERVATIONS', 'VIEWER_SETTINGS'];
// jumpHours は、番組表で移れる時間帯の始まり (時)。放送日は 4 時に始まるので、24 時以降は翌日の深夜を表す。
const jumpHours = [5, 12, 17, 20, 24] as const;

export const Route = createFileRoute('/guide/')({
  validateSearch: (search: Record<string, unknown>): GuideSearch => {
    const text = (key: string) =>
      typeof search[key] === 'string' && search[key] !== '' ? search[key] : undefined;
    const date = text('date');
    const type = text('type');
    const program = text('program');
    const view = text('view');
    const reservation = text('reservation');

    return {
      ...(date !== undefined && datePattern.test(date) ? { date } : {}),
      ...(type !== undefined ? { type } : {}),
      ...(program !== undefined ? { program } : {}),
      ...(view !== undefined ? { view } : {}),
      ...(reservation !== undefined ? { reservation } : {}),
    };
  },
  component: GuidePage,
  staticData: { fluid: true },
});

// dayFrom は、URL の日付 (なければ今日) の放送日の始まりを返す。
function dayFrom(date: string | undefined, todayFrom: number): number {
  // その日の正午 (表示のタイムゾーン) を含む放送日にする。
  return date === undefined
    ? todayFrom
    : broadcastDay(fromWallClock(Date.parse(`${date}T12:00:00Z`))).from;
}

function GuidePage() {
  const { t } = useTranslation();
  const navigate = useNavigate({ from: '/guide/' });
  const { view } = Route.useSearch();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('guide.title')}</Title>
      {/* ビューを外したら、エラーの表示を消して読み直す。 */}
      <ErrorBoundary
        key={view ?? ''}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : view !== undefined && graphQLErrorDetails(error).code === 'NOT_FOUND' ? (
            // 消したビューのリンクを開いたときは、ビューを外して開き直せるようにする。
            <Alert color="amber">
              {t('guide.view.notFound')}{' '}
              <Anchor
                component="button"
                type="button"
                onClick={() => void navigate({ search: (prev) => ({ ...prev, view: undefined }) })}
              >
                {t('guide.view.showAll')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Guide />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Guide() {
  const { t } = useTranslation();
  const navigate = useNavigate({ from: '/guide/' });
  const search = Route.useSearch();
  // now は、画面を開いた (「現在時刻」を押した) 時刻。日付の移動の基準と、いまの時刻の線に使う。
  const [now, setNow] = useState(() => Date.now());
  const today = broadcastDay(now);
  // 日付やビューを変えても、新しい番組表を読み終えるまでは前の番組表を出したままにし、番組表の上に読み込み中を出す。
  // 画面全体を読み込み中にしないよう、読み込みに使う値だけを遅らせる。
  const date = useDeferredValue(search.date);
  const view = useDeferredValue(search.view);
  const loading = date !== search.date || view !== search.view;
  // window は、出している番組表の日。selectedFrom は、選んだ日 (読み込み中なら、これから出す日)。
  const window = useMemo(() => {
    const from = dayFrom(date, today.from);

    return { from, to: from + day };
  }, [date, today.from]);
  const selectedFrom = dayFrom(search.date, today.from);
  // fetchKey は、番組表ビューを保存したときに、番組表とビューの一覧を取り直すための合図。
  const [fetchKey, setFetchKey] = useState(0);
  // changes は、番組表と予約が、EPG の同期やほかの端末で変わるたびに変わる (#503)。変わったら取り直す。
  const changes = useDataChanges(guideChannels);
  // editing は、編集中の番組表ビュー。
  const [editing, setEditing] = useState<EditableGuideView | null>(null);
  const data = useLazyLoadQuery<guideQuery>(
    graphql`
      query guideQuery($from: Time!, $to: Time!, $view: ID) {
        channelTypes {
          type
          displayName
        }
        pinnedServices {
          id
        }
        services {
          id
          name
          logoUrl
          serviceId
          channelType {
            displayName
          }
        }
        guideViews {
          id
          name
          genres {
            lv1
            lv2
          }
          excludeGenres {
            lv1
            lv2
          }
          minDurationSeconds
          services {
            id
          }
          hideEmptyServices
          freeOnly
        }
        guide(from: $from, to: $to, view: $view) {
          service {
            id
            name
            logoUrl
            channelType {
              type
            }
          }
          programs {
            id
            startAt
            endAt
            endUndecided
            name
            genres {
              lv1
            }
            sharedFrom {
              id
            }
            reservation {
              id
              enabled
            }
            watchReserved
          }
        }
      }
    `,
    {
      from: new Date(window.from).toISOString(),
      to: new Date(window.to).toISOString(),
      view: view ?? null,
    },
    // store-or-network は、fetchKey を変えても、ストアにある番組表を取り直さない (#614)。保存した後は、
    // ストアの値を出しながら取り直す。ストアに残るほかの日の番組表も、保存する前の条件で絞ったものなので、
    // その日へ移ったときにも取り直す。
    {
      fetchKey: `${fetchKey}:${changes}`,
      fetchPolicy: fetchKey === 0 && changes === 0 ? 'store-or-network' : 'store-and-network',
    },
  );
  const currentView = data.guideViews.find((candidate) => candidate.id === view);

  const types = data.channelTypes.filter((channelType) =>
    data.guide.some((row) => row.service.channelType.type === channelType.type),
  );
  // pinned は、ピン留めしたサービスの ID を、ピン留めした順に並べたもの (#485)。
  const pinned = useMemo(
    () => data.pinnedServices.map((service) => service.id),
    [data.pinnedServices],
  );
  // URL の種別が、この日の番組表にないときは、最初の種別にする。ピン留めのタブは、ピン留めがあるときだけ選べる。
  const type =
    (search.type === pinnedTab && pinned.length > 0) ||
    types.some((channelType) => channelType.type === search.type)
      ? search.type
      : types[0]?.type;
  // サブチャンネルは、元のサービスと違う番組を流す時間がある日だけ列を出し、同じ番組の枠は薄くする。
  const columns = useMemo<GuideColumn[]>(
    () =>
      (type === pinnedTab
        ? // ピン留めのタブでは、ピン留めしたサービスを、ピン留めした順にそのまま並べる。
          pinned.flatMap((id) =>
            data.guide
              .filter((row) => row.service.id === id)
              // ピン留めしたサブチャンネルは、元のサービスと同じ番組の枠も、薄くせずに出す。
              .map((row) => ({
                ...row,
                id,
                programs: row.programs.map((program) => ({ ...program, shared: false })),
              })),
          )
        : collapseSharedColumns(data.guide.map((row) => ({ ...row, id: row.service.id }))).filter(
            (row) => row.service.channelType.type === type,
          )
      ).map((row) => ({
        id: row.service.id,
        name: row.service.name,
        logoUrl: row.service.logoUrl ?? null,
        programs: row.programs.map((program) => ({
          id: program.id,
          startAt: Date.parse(program.startAt),
          endAt: Date.parse(program.endAt),
          endUndecided: program.endUndecided,
          name: program.name,
          genre: program.genres[0]?.lv1,
          shared: program.shared,
          reserved: program.reservation?.enabled === true,
          watchReserved: program.watchReserved,
        })),
      })),
    [data.guide, type, pinned],
  );

  // focused は、URL で指した番組 (予約から開いたときは、その予約の番組)。開いたときに、その時刻へスクロールする。
  const focused = useMemo(() => {
    const programs = data.guide.flatMap((row) => row.programs);

    return search.reservation !== undefined
      ? programs.find((program) => program.reservation?.id === search.reservation)
      : programs.find((program) => program.id === search.program);
  }, [data.guide, search.reservation, search.program]);
  // 予約から開いたときは、その予約の番組の詳細を開き、URL を番組の ID に置き換える。番組が見つからなければ、予約を外すだけにする。
  useEffect(() => {
    if (search.reservation === undefined) {
      return;
    }
    void navigate({
      search: (prev) => ({ ...prev, reservation: undefined, program: focused?.id ?? prev.program }),
      replace: true,
    });
  }, [navigate, search.reservation, focused]);

  // offset は、選んだ日の、今日からの日数。日付の移動のボタンに使う。
  const offset = Math.round((selectedFrom - today.from) / day);
  // keepTime は、日付を移っても、見ていた時間帯のままにするか。前後の日へ移るときは保ち、今日へ戻るときはいまの時刻を出す。
  const [keepTime, setKeepTime] = useState(false);
  // scrollKey は、日付を変えずに、いまの時刻へスクロールし直すための合図。
  const [scrollKey, setScrollKey] = useState(0);
  // jumpHour は、時間帯のメニューで選んだ時刻 (時)。選んでいなければ null。日付を移ったら取り消す。
  // extendedHours は、深夜を 24 時以降 (28 時間表記) で表すかどうか。
  const [extendedHours, setExtendedHours] = useState(loadExtendedHours);
  const [jumpHour, setJumpHour] = useState<number | null>(null);
  const moveTo = (days: number, keep: boolean) => {
    setJumpHour(null);
    setKeepTime(keep);
    void navigate({
      search: (prev) => ({ ...prev, date: dateOf(today.from + days * day), program: undefined }),
    });
  };
  // moveToNow は、いまの時刻を取り直し、今日のいまの時刻へ移る。すでに今日なら、日付は変わらないので、スクロールだけし直す。
  const moveToNow = () => {
    setJumpHour(null);
    const time = Date.now();
    setNow(time);
    setKeepTime(false);
    if (selectedFrom === broadcastDay(time).from) {
      setScrollKey((key) => key + 1);
    }
    void navigate({
      search: (prev) => ({ ...prev, date: dateOf(broadcastDay(time).from), program: undefined }),
    });
  };
  // jumpTo は、出している日の、hour 時の位置へスクロールする。
  const jumpTo = (hour: number) => {
    setKeepTime(false);
    setJumpHour(hour);
    setScrollKey((key) => key + 1);
  };
  const dayFormat = zonedFormat({
    month: 'numeric',
    day: 'numeric',
    weekday: 'short',
  });

  return (
    <Stack gap="sm">
      <div className={classes.toolbar}>
        <Group gap="xs">
          <Button
            variant="default"
            size="xs"
            disabled={offset <= 0}
            onClick={() => moveTo(offset - 1, true)}
            leftSection={<ChevronLeft size={14} aria-hidden />}
          >
            {t('guide.previousDay')}
          </Button>
          {/* 日付を押すと、移れる日の一覧から選べる。 */}
          <Menu position="bottom" withinPortal>
            <Menu.Target>
              <Button
                variant="subtle"
                size="xs"
                fw={700}
                miw={96}
                leftSection={<CalendarDays size={14} aria-hidden />}
                aria-label={t('guide.pickDate', { date: dayFormat.format(selectedFrom) })}
              >
                {dayFormat.format(selectedFrom)}
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              {Array.from({ length: guideDays }, (_, days) => (
                <Menu.Item
                  key={days}
                  onClick={() => moveTo(days, true)}
                  fw={days === offset ? 700 : undefined}
                >
                  {dayFormat.format(today.from + days * day)}
                </Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
          <Button
            variant="default"
            size="xs"
            disabled={offset >= guideDays - 1}
            onClick={() => moveTo(offset + 1, true)}
            rightSection={<ChevronRight size={14} aria-hidden />}
          >
            {t('guide.nextDay')}
          </Button>
          <Button
            variant="default"
            size="xs"
            onClick={moveToNow}
            leftSection={<Clock size={14} aria-hidden />}
          >
            {t('guide.now')}
          </Button>
          <Menu position="bottom" withinPortal>
            <Menu.Target>
              <Button variant="default" size="xs" leftSection={<Sunrise size={14} aria-hidden />}>
                {t('guide.jump')}
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              {jumpHours.map((hour) => (
                <Menu.Item key={hour} onClick={() => jumpTo(hour)}>
                  {t(`guide.jumpTo.${hour}`)}
                </Menu.Item>
              ))}
              <Menu.Divider />
              {/* 深夜を 24 時以降で表すかどうか。端末ごとに覚える (#486)。 */}
              <Menu.Label>
                <Checkbox
                  size="xs"
                  label={t('guide.extendedHours')}
                  checked={extendedHours}
                  onChange={(event) => {
                    saveExtendedHours(event.currentTarget.checked);
                    setExtendedHours(event.currentTarget.checked);
                  }}
                />
              </Menu.Label>
            </Menu.Dropdown>
          </Menu>
        </Group>
        {types.length > 0 && (
          <SegmentedControl
            className={classes.types}
            size="xs"
            value={type ?? ''}
            onChange={(value) =>
              void navigate({ search: (prev) => ({ ...prev, type: value, program: undefined }) })
            }
            data={[
              ...(pinned.length > 0 ? [{ value: pinnedTab, label: t('pin.tab') }] : []),
              ...types.map((channelType) => ({
                value: channelType.type,
                label: channelType.displayName,
              })),
            ]}
          />
        )}
        <Group gap="xs" wrap="nowrap" className={classes.end}>
          <Select
            size="xs"
            w={160}
            aria-label={t('guide.view.label')}
            // URL のビューが消えていても、「すべての番組」を選び直して外せるよう、URL の値をそのまま使う。
            value={search.view ?? ''}
            onChange={(value) =>
              void navigate({
                search: (prev) => ({
                  ...prev,
                  view: value === '' ? undefined : (value ?? undefined),
                }),
              })
            }
            data={[
              { value: '', label: t('guide.view.all') },
              ...data.guideViews.map((view) => ({ value: view.id, label: view.name })),
            ]}
            allowDeselect={false}
          />
          <Button
            variant="default"
            size="xs"
            onClick={() =>
              setEditing(
                currentView === undefined
                  ? {
                      id: undefined,
                      name: '',
                      genres: [],
                      excludeGenres: [],
                      minDurationSeconds: null,
                      services: [],
                      hideEmptyServices: false,
                      freeOnly: false,
                    }
                  : {
                      id: currentView.id,
                      name: currentView.name,
                      genres: currentView.genres,
                      excludeGenres: currentView.excludeGenres,
                      minDurationSeconds: currentView.minDurationSeconds,
                      services: currentView.services.map((service) => service.id),
                      hideEmptyServices: currentView.hideEmptyServices,
                      freeOnly: currentView.freeOnly,
                    },
              )
            }
          >
            {currentView === undefined ? t('guide.view.create') : t('guide.view.edit')}
          </Button>
        </Group>
      </div>
      {/* 日付やビューを変えたときは、読み終えるまで前の番組表を出し、その上に読み込み中を重ねる。 */}
      <Box pos="relative">
        <LoadingOverlay visible={loading} zIndex={5} overlayProps={{ blur: 1 }} />
        {columns.length === 0 ? (
          <EmptyState icon={CalendarX2} title={t('guide.empty')} hint={t('guide.emptyHint')} />
        ) : (
          <GuideGrid
            window={window}
            columns={columns}
            headerAction={(column) => (
              <PinButton
                serviceId={column.id}
                serviceName={column.name}
                pinned={pinned.includes(column.id)}
                size="sm"
              />
            )}
            now={now}
            receivedUntil={
              window.from > today.from
                ? receivedUntil(
                    columns.flatMap((column) => column.programs.map((program) => program.endAt)),
                    window.to,
                  )
                : undefined
            }
            scrollKey={scrollKey}
            scrollProgram={keepTime ? undefined : focused?.id}
            scrollTime={
              jumpHour !== null
                ? window.from + (jumpHour - 4) * 3_600_000
                : keepTime
                  ? null
                  : focused !== undefined
                    ? Date.parse(focused.startAt)
                    : window.from === today.from
                      ? now
                      : window.from + 15 * 3_600_000
            }
            extendedHours={extendedHours}
            onSelect={(program) => void navigate({ search: (prev) => ({ ...prev, program }) })}
          />
        )}
      </Box>
      {editing !== null && (
        <GuideViewEditor
          view={editing}
          services={data.services}
          onClose={() => setEditing(null)}
          onSaved={(id) => {
            setEditing(null);
            setFetchKey((key) => key + 1);
            void navigate({ search: (prev) => ({ ...prev, view: id }) });
          }}
        />
      )}
      <ProgramDetail
        programId={search.program}
        onClose={() => void navigate({ search: (prev) => ({ ...prev, program: undefined }) })}
      />
    </Stack>
  );
}
