import {
  Alert,
  Anchor,
  Badge,
  Button,
  Drawer,
  Group,
  Modal,
  Stack,
  Text,
  Title,
  Tooltip,
} from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CalendarCheck, CalendarPlus, Play } from 'lucide-react';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary, ProgramTitle, toHalfWidth } from '../futon';
import { genreKey } from './genres';
import type { ProgramDetailCancelMutation } from './__generated__/ProgramDetailCancelMutation.graphql';
import type { ProgramDetailQuery } from './__generated__/ProgramDetailQuery.graphql';
import type { ProgramDetailReserveMutation } from './__generated__/ProgramDetailReserveMutation.graphql';
import { ServiceLabel } from './ServiceLabel';

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
const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// onReserved は、録画予約を作れたときに呼ぶ。サイドバーを閉じ、番組表で予約した番組を見せる。
function Detail({ programId, onReserved }: { programId: string; onReserved: () => void }) {
  const { t } = useTranslation();
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
            description
            startAt
            endAt
            endUndecided
            isFree
            archived
            service {
              id
              name
              logoUrl
            }
            genres {
              lv1
            }
            extended {
              heading
              text
            }
            reservation {
              id
              enabled
              kind
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
  );
  const [commit, inFlight] = useMutation<ProgramDetailReserveMutation>(graphql`
    mutation ProgramDetailReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
        enabled
      }
    }
  `);
  const [result, setResult] = useState<'error' | null>(null);
  // cancelling は、録画予約を取り消す確認を出しているかどうか。
  const [cancelling, setCancelling] = useState(false);
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

  return (
    <Stack>
      <Title order={3} size="h4">
        <ProgramTitle name={program.name} />
      </Title>
      <Group gap="xs">
        <Text size="sm">
          <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
        </Text>
        <Text size="sm" c="dimmed">
          {dateTimeFormat.format(start)}〜
          {program.endUndecided
            ? t('guide.detail.endUndecided')
            : timeFormat.format(Date.parse(program.endAt))}
        </Text>
        {!program.isFree && <Badge color="gray">{t('guide.detail.paid')}</Badge>}
        {program.genres[0] !== undefined && (
          <Badge variant="light">{t(genreKey(program.genres[0].lv1))}</Badge>
        )}
      </Group>
      {onAir && (
        <Button
          variant="light"
          leftSection={<Play size={16} aria-hidden />}
          renderRoot={(props) => (
            <Link {...props} to="/live/$serviceId" params={{ serviceId: program.service.id }} />
          )}
        >
          {t('guide.detail.watch')}
        </Button>
      )}
      {/* 検索から開いた、放送の終わった番組は予約できない。 */}
      {data.viewer.role !== 'VIEWER' && !ended && !reserved && (
        <Button
          onClick={reserve}
          loading={inFlight}
          leftSection={<CalendarPlus size={16} aria-hidden />}
        >
          {t('guide.detail.reserve')}
        </Button>
      )}
      {/* 録画予約済みのボタンを押すと、確かめてから取り消す。 */}
      {data.viewer.role !== 'VIEWER' && !ended && reserved && program.reservation != null && (
        <Tooltip label={t('guide.detail.cancelReservation')}>
          <Button
            variant="outline"
            onClick={() => setCancelling(true)}
            leftSection={<CalendarCheck size={16} aria-hidden />}
          >
            {t('guide.detail.reserved')}
          </Button>
        </Tooltip>
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
      {program.description !== '' && <Text size="sm">{toHalfWidth(program.description)}</Text>}
      {program.extended.map((item, index) => (
        <Stack key={index} gap={2}>
          <Text size="sm" fw={700}>
            {toHalfWidth(item.heading)}
          </Text>
          <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>
            {toHalfWidth(item.text)}
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
