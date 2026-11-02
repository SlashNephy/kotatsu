import {
  Alert,
  Anchor,
  Badge,
  Button,
  Drawer,
  Group,
  Menu,
  Modal,
  Stack,
  Text,
  Title,
  Tooltip,
} from '@mantine/core';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  CalendarCheck,
  CalendarPlus,
  CalendarX2,
  ChevronDown,
  Film,
  History,
  ListPlus,
  Search,
  Play,
  SlidersHorizontal,
  TriangleAlert,
} from 'lucide-react';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useDataChanges } from '../changes/DataChanges';
import type { ChangeChannel } from '../changes/dataChanges';
import { conditionsFromProgram } from '../conditions/form';
import { ErrorBoundary, LinkifiedText, ProgramTitle, toHalfWidth } from '../futon';
import { programGenreLabels } from './genreOptions';
import type { ProgramDetailCancelMutation } from './__generated__/ProgramDetailCancelMutation.graphql';
import type { ProgramDetailQuery } from './__generated__/ProgramDetailQuery.graphql';
import type { ProgramDetailReserveMutation } from './__generated__/ProgramDetailReserveMutation.graphql';
import { ReservationSettingsDialog } from '../reservations/ReservationSettingsDialog';
import { AnimeEpisodes } from '../anime/AnimeEpisodes';
import { ServiceLabel } from './ServiceLabel';
import { WatchReserveButton } from '../watch/WatchReserveButton';
import { zonedFormat } from '../futon';

type Props = {
  programId: string | undefined;
  onClose: () => void;
};

// ProgramDetail は、番組の詳細を、番組表の横に開く。メンバーと管理者は、ここから予約できる。
export function ProgramDetail({ programId, onClose }: Props) {
  const { t } = useTranslation();

  return (
    <Drawer
      opened={programId !== undefined}
      onClose={onClose}
      position="right"
      size="md"
      title={t('guide.detail.title')}
    >
      {programId !== undefined && (
        <ErrorBoundary fallback={() => <Alert color="amber">{t('guide.detail.error')}</Alert>}>
          <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
            <Detail key={programId} programId={programId} onReserved={onClose} />
          </Suspense>
        </ErrorBoundary>
      )}
    </Drawer>
  );
}

// 検索から過去の番組も開くので、年も出す。
const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// onReserved は、録画予約を作れたときに呼ぶ。サイドバーを閉じ、番組表で予約した番組を見せる。
// detailChannels は、番組の詳細に関わるデータの種類。予約の割り当ての状態も出す。
const detailChannels: readonly ChangeChannel[] = ['EPG', 'RESERVATIONS', 'ASSIGNMENTS'];

function Detail({ programId, onReserved }: { programId: string; onReserved: () => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // 番組や予約が、EPG の同期やほかの端末、ルールで変わったら、読み直す (#867)。
  const changes = useDataChanges(detailChannels);
  const data = useLazyLoadQuery<ProgramDetailQuery>(
    graphql`
      query ProgramDetailQuery($id: ID!) {
        viewer {
          role
        }
        node(id: $id) {
          __typename
          ... on Program {
            id
            name
            ruleKeyword
            description
            startAt
            endAt
            endUndecided
            isFree
            archived
            eventId
            service {
              id
              name
              logoUrl
              # 放送が終わった番組を、タイムシフトのバッファから再生できるか (#1121)。
              timeshiftPrograms {
                eventId
                startAt
              }
            }
            # 放送が終わった番組の録画 (#1121)。
            recordings {
              id
            }
            genres {
              lv1
              lv2
            }
            # しょぼいカレンダーで結びつけた話 (設計書 3.8)。詳細でだけ引く。
            animeEpisodes {
              syobocalUrl
              countStart
              countEnd
              subtitle
              first
              last
              rerun
              works {
                annictUrl
                anilistUrl
                malUrl
              }
            }
            extended {
              heading
              text
            }
            watchReserved
            reservation {
              id
              enabled
              kind
              assignment {
                state
                plannedStartAt
                plannedEndAt
              }
              rules {
                id
                name
              }
            }
          }
        }
      }
    `,
    { id: programId },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const [commit, inFlight] = useMutation<ProgramDetailReserveMutation>(graphql`
    mutation ProgramDetailReserveMutation($program: ID!) {
      # 番組に結び付けた後に詳細が読む項目を、すべて返す。返さないと、ストアにない項目を読んで落ちる。
      createProgramReservation(input: { program: $program }) {
        id
        enabled
        kind
        assignment {
          state
          plannedStartAt
          plannedEndAt
        }
      }
    }
  `);
  const [result, setResult] = useState<'error' | null>(null);
  // cancelling は、録画予約を取り消す確認を出しているかどうか。
  const [cancelling, setCancelling] = useState(false);
  // editing は、予約の録画の設定を変えるダイアログを出しているかどうか。
  const [editing, setEditing] = useState(false);
  // configuring は、録画の設定をしてから予約するダイアログを出しているかどうか。
  const [configuring, setConfiguring] = useState(false);
  const program = data.node;
  if (program?.__typename !== 'Program') {
    return <Alert color="amber">{t('guide.detail.error')}</Alert>;
  }

  const reserve = () => {
    setResult(null);
    commit({
      variables: { program: program.id },
      // 作った予約を番組に結び付け、番組表の枠と、次に開いたときのボタンに、予約済みを出す。
      updater: (store) => {
        const reservation = store.getRootField('createProgramReservation');
        const record = store.get(program.id);
        if (reservation !== null && record !== undefined && record !== null) {
          record.setLinkedRecord(reservation, 'reservation');
        }
      },
      // Relay は、data とともに返ったエラーを、onError ではなく onCompleted に渡す。
      onCompleted: (_response, errors) => {
        if (errors != null && errors.length > 0) {
          setResult('error');

          return;
        }
        onReserved();
      },
      // 予約を作れないロールには、ボタンを出していないので、ここでは理由を出し分けない。
      onError: () => setResult('error'),
    });
  };
  const start = Date.parse(program.startAt);
  const ended =
    program.archived || (!program.endUndecided && Date.parse(program.endAt) <= Date.now());
  // reserved は、この番組の録画予約が (有効なまま) あるかどうか。
  const reserved = program.reservation?.enabled === true;
  // 放送中なら、その場でライブ視聴を開けるようにする。
  const onAir = !program.archived && !ended && start <= Date.now();
  // 視聴予約は、放送が始まる前の番組にだけ付けられる。付けてある番組では、外せるようにボタンを出し続ける。
  const canWatchReserve = !program.archived && (start > Date.now() || program.watchReserved);
  // canRecord は、録画予約のボタンを出すかどうか。検索から開いた、放送の終わった番組は予約できない。
  const canRecord = data.viewer.role !== 'VIEWER' && !ended;
  const canCreateRule = data.viewer.role !== 'VIEWER';
  // 放送が終わった番組は、録画か、タイムシフトのバッファから見られる (#1121)。バッファの番組は、mikan が番組の始まりに
  // 気づいた時刻で区切るので、同じイベント ID で、番組の放送時間の中に始まったものを探す。
  const recording = ended ? program.recordings[0] : undefined;
  const timeshift = ended
    ? program.service.timeshiftPrograms.find(
        (candidate) =>
          candidate.eventId === program.eventId &&
          Date.parse(candidate.startAt) >= start - 60_000 &&
          Date.parse(candidate.startAt) < Date.parse(program.endAt),
      )
    : undefined;
  // createRule は、番組名から読んだ作品名とサービスを条件に引き継いで、ルールの作成画面を開く。番組検索の「この条件でルールを作成」と
  // 同じ画面。話数やサブタイトルまで入れると、その話にしか一致しないため、作品名だけにする (#1240)。
  const createRule = () =>
    void navigate({
      to: '/rules/$ruleId',
      params: { ruleId: 'new' },
      state: {
        ruleConditions: conditionsFromProgram(
          program.name,
          program.service.id,
          program.ruleKeyword,
        ),
      },
    });
  // searchSameName は、番組名を条件に引き継いで、過去の放送も含めて番組を探す (#1121)。
  const searchSameName = () =>
    void navigate({
      to: '/search',
      state: {
        searchConditions: {
          ...conditionsFromProgram(program.name, program.service.id),
          services: [],
        },
      },
    });
  const genres = programGenreLabels(t, program.genres);

  return (
    <Stack>
      {/* 番組名、サービス、放送時間、ジャンルの順に、1 段ずつ並べる。 */}
      <Stack gap={4}>
        <Title order={3} size="h4">
          <ProgramTitle name={program.name} />
        </Title>
        <Text size="sm">
          <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
        </Text>
        <Text size="sm" c="dimmed">
          {t('range', {
            from: dateTimeFormat.format(start),
            to: program.endUndecided
              ? t('guide.detail.endUndecided')
              : timeFormat.format(Date.parse(program.endAt)),
          })}
        </Text>
        {(genres.length > 0 || !program.isFree) && (
          <Group gap="xs">
            {genres.map((genre) => (
              <Badge key={genre} variant="light">
                {genre}
              </Badge>
            ))}
            {!program.isFree && <Badge color="gray">{t('guide.detail.paid')}</Badge>}
          </Group>
        )}
        <AnimeEpisodes episodes={program.animeEpisodes} record={{ program: program.id }} />
      </Stack>
      {/* 操作は 1 段にまとめる。録画予約は、よく使うワンクリックの予約を前に出し、ほかの操作は横の ▾ のメニューに入れる。 */}
      {ended && (
        <Group gap="xs" wrap="nowrap">
          {recording !== undefined && (
            <Button
              variant="light"
              flex={1}
              leftSection={<Film size={16} aria-hidden />}
              renderRoot={(props) => (
                <Link
                  {...props}
                  to="/recordings/$recordingId"
                  params={{ recordingId: recording.id }}
                />
              )}
            >
              {t('guide.detail.openRecording')}
            </Button>
          )}
          <Button
            variant="default"
            flex={1}
            leftSection={<Search size={16} aria-hidden />}
            onClick={searchSameName}
          >
            {t('guide.detail.searchSameName')}
          </Button>
          {timeshift !== undefined && (
            <Button
              variant="light"
              flex={1}
              leftSection={<History size={16} aria-hidden />}
              renderRoot={(props) => (
                <Link
                  {...props}
                  to="/timeshift/$serviceId/play"
                  params={{ serviceId: program.service.id }}
                  search={{ program: timeshift.startAt }}
                />
              )}
            >
              {t('guide.detail.playTimeshift')}
            </Button>
          )}
        </Group>
      )}
      {(onAir || canRecord || canCreateRule || canWatchReserve) && (
        <Group gap="xs" wrap="nowrap">
          {onAir && (
            <Button
              variant="light"
              flex={1}
              leftSection={<Play size={16} aria-hidden />}
              renderRoot={(props) => (
                <Link {...props} to="/live/$serviceId" params={{ serviceId: program.service.id }} />
              )}
            >
              {t('guide.detail.watch')}
            </Button>
          )}
          {canRecord && (
            <Button.Group flex={1}>
              {reserved && program.reservation != null ? (
                // 録画予約済みのボタンを押すと、録画の設定を開く。取り消しは、間違えて押さないよう、メニューに入れる。
                <Tooltip label={t('reservations.settings.open')}>
                  <Button
                    variant="outline"
                    flex={1}
                    onClick={() => setEditing(true)}
                    leftSection={<CalendarCheck size={16} aria-hidden />}
                  >
                    {t('guide.detail.reserved')}
                  </Button>
                </Tooltip>
              ) : (
                <Button
                  flex={1}
                  onClick={reserve}
                  loading={inFlight}
                  leftSection={<CalendarPlus size={16} aria-hidden />}
                >
                  {t('guide.detail.reserve')}
                </Button>
              )}
              <Menu position="bottom-end">
                <Menu.Target>
                  <Button
                    variant={reserved ? 'outline' : 'filled'}
                    px={8}
                    disabled={inFlight}
                    aria-label={t('guide.detail.moreActions')}
                  >
                    <ChevronDown size={16} aria-hidden />
                  </Button>
                </Menu.Target>
                <Menu.Dropdown>
                  {reserved && program.reservation != null ? (
                    <Menu.Item
                      color="red"
                      leftSection={<CalendarX2 size={16} aria-hidden />}
                      onClick={() => setCancelling(true)}
                    >
                      {t('guide.detail.cancelReservation')}
                    </Menu.Item>
                  ) : (
                    // ワンクリックの予約とは別に、録画の設定をしてから予約する (#463)。
                    <Menu.Item
                      leftSection={<SlidersHorizontal size={16} aria-hidden />}
                      onClick={() => setConfiguring(true)}
                    >
                      {t('reservations.settings.openCreate')}
                    </Menu.Item>
                  )}
                  <Menu.Item leftSection={<ListPlus size={16} aria-hidden />} onClick={createRule}>
                    {t('guide.detail.createRule')}
                  </Menu.Item>
                </Menu.Dropdown>
              </Menu>
            </Button.Group>
          )}
          {/* 放送の終わった番組は予約できないが、同じ番組名のこれからの回を録るルールは作れる。 */}
          {!canRecord && canCreateRule && (
            <Button
              variant="default"
              flex={1}
              leftSection={<ListPlus size={16} aria-hidden />}
              onClick={createRule}
            >
              {t('guide.detail.createRule')}
            </Button>
          )}
          {/* 視聴予約は、放送が始まる前の番組にだけ付けられる (設計書 4.8)。 */}
          {canWatchReserve && (
            <WatchReserveButton programId={program.id} reserved={program.watchReserved} />
          )}
        </Group>
      )}
      {/* 一部のみ録画する予約は、録れない範囲が分かるよう、マージンを削った後の予定時刻を出す (#466)。 */}
      {reserved &&
        program.reservation?.assignment.state === 'PARTIAL' &&
        program.reservation.assignment.plannedStartAt != null &&
        program.reservation.assignment.plannedEndAt != null && (
          <Alert color="amber" variant="light" icon={<TriangleAlert size={16} aria-hidden />}>
            {t('guide.detail.partial', {
              start: timeFormat.format(Date.parse(program.reservation.assignment.plannedStartAt)),
              end: timeFormat.format(Date.parse(program.reservation.assignment.plannedEndAt)),
            })}
          </Alert>
        )}
      {configuring && !reserved && (
        <ReservationSettingsDialog
          id={program.id}
          onClose={() => setConfiguring(false)}
          onReserved={onReserved}
        />
      )}
      {editing && program.reservation != null && (
        <ReservationSettingsDialog id={program.reservation.id} onClose={() => setEditing(false)} />
      )}
      {reserved && program.reservation != null && program.reservation.kind === 'RULE' && (
        <ReservedByRules rules={program.reservation.rules} />
      )}
      {cancelling && program.reservation != null && (
        <CancelDialog
          reservationId={program.reservation.id}
          byRule={program.reservation.kind === 'RULE'}
          programName={program.name}
          onClose={() => setCancelling(false)}
        />
      )}
      {result === 'error' && (
        <Alert color="red" role="alert">
          {t('guide.detail.reserveError')}
        </Alert>
      )}
      {program.description !== '' && (
        <Text size="sm">
          <LinkifiedText text={toHalfWidth(program.description)} />
        </Text>
      )}
      {program.extended.map((item, index) => (
        <Stack key={index} gap={2}>
          <Text size="sm" fw={700}>
            {toHalfWidth(item.heading)}
          </Text>
          <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>
            <LinkifiedText text={toHalfWidth(item.text)} />
          </Text>
        </Stack>
      ))}
    </Stack>
  );
}

// ReservedByRules は、ルール予約のもとになったルールを並べ、ルールの画面へ移れるようにする。
// 複数のルールに一致したときは、録画の設定に使うのが最も古いルール (先頭) であることも示す。
function ReservedByRules({ rules }: { rules: ReadonlyArray<{ id: string; name: string }> }) {
  const { t } = useTranslation();
  if (rules.length === 0) {
    return null;
  }

  return (
    <Stack gap={2}>
      <Text size="sm">
        {t('guide.detail.reservedByRules')}
        {rules.map((rule, index) => (
          <span key={rule.id}>
            {index > 0 && '、'}
            <Anchor
              size="sm"
              renderRoot={(props) => (
                <Link {...props} to="/rules/$ruleId" params={{ ruleId: rule.id }} />
              )}
            >
              {rule.name}
            </Anchor>
          </span>
        ))}
      </Text>
      {rules.length > 1 && (
        <Text size="xs" c="dimmed">
          {t('guide.detail.reservedByRulesSettings', { name: rules[0]?.name })}
        </Text>
      )}
    </Stack>
  );
}

type CancelProps = {
  reservationId: string;
  // byRule は、ルールで入った予約かどうか。ルール予約は消さずに無効にするので、ルールは残る。
  byRule: boolean;
  programName: string;
  onClose: () => void;
};

// CancelDialog は、録画予約を取り消す前に確かめる。番組予約は消し、ルール予約は無効にする (deleteReservation)。
function CancelDialog({ reservationId, byRule, programName, onClose }: CancelProps) {
  const { t } = useTranslation();
  const [error, setError] = useState(false);
  const [commit, inFlight] = useMutation<ProgramDetailCancelMutation>(graphql`
    mutation ProgramDetailCancelMutation($id: ID!) {
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
    <Modal opened onClose={onClose} title={t('guide.detail.cancelReservation')}>
      <Stack>
        <Text size="sm" fw={700}>
          <ProgramTitle name={programName} />
        </Text>
        <Text size="sm" c="dimmed">
          {byRule ? t('guide.detail.cancelRuleDescription') : t('guide.detail.cancelDescription')}
        </Text>
        {error && (
          <Alert color="red" role="alert">
            {t('guide.detail.cancelError')}
          </Alert>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t('reservations.cancel')}
          </Button>
          <Button
            color="red"
            loading={inFlight}
            onClick={() => {
              // 押し直したときは、前の失敗の知らせを消してから取り消す。
              setError(false);
              commit({
                variables: { id: reservationId },
                // 消した番組予約は、番組から外す。ルール予約は、返った予約 (無効) がそのまま番組に残る。
                updater: (store) => {
                  const payload = store.getRootField('deleteReservation');
                  if (payload?.getLinkedRecord('reservation') == null) {
                    store.delete(reservationId);
                  }
                },
                onCompleted: (_response, errors) =>
                  errors != null && errors.length > 0 ? setError(true) : onClose(),
                onError: () => setError(true),
              });
            }}
          >
            {t('guide.detail.cancelConfirm')}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
