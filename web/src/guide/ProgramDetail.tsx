import { Alert, Badge, Button, Drawer, Group, Stack, Text, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CalendarCheck, CalendarPlus, Play } from 'lucide-react';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary, ProgramTitle } from '../futon';
import { genreKey } from './genres';
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
            <Detail key={programId} programId={programId} />
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

function Detail({ programId }: { programId: string }) {
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
      }
    }
  `);
  const [result, setResult] = useState<'reserved' | 'error' | null>(null);
  const program = data.node;
  if (program?.__typename !== 'Program') {
    return <Alert color="amber">{t('guide.detail.error')}</Alert>;
  }

  const reserve = () => {
    setResult(null);
    commit({
      variables: { program: program.id },
      // Relay は、data とともに返ったエラーを、onError ではなく onCompleted に渡す。
      onCompleted: (_response, errors) =>
        setResult(errors != null && errors.length > 0 ? 'error' : 'reserved'),
      // 予約を作れないロールには、ボタンを出していないので、ここでは理由を出し分けない。
      onError: () => setResult('error'),
    });
  };
  const start = Date.parse(program.startAt);
  const ended =
    program.archived || (!program.endUndecided && Date.parse(program.endAt) <= Date.now());
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
      {data.viewer.role !== 'VIEWER' && !ended && (
        <Button
          onClick={reserve}
          loading={inFlight}
          disabled={result === 'reserved'}
          leftSection={
            result === 'reserved' ? (
              <CalendarCheck size={16} aria-hidden />
            ) : (
              <CalendarPlus size={16} aria-hidden />
            )
          }
        >
          {result === 'reserved' ? t('guide.detail.reserved') : t('guide.detail.reserve')}
        </Button>
      )}
      {result === 'error' && (
        <Alert color="red" role="alert">
          {t('guide.detail.reserveError')}
        </Alert>
      )}
      {program.description !== '' && <Text size="sm">{program.description}</Text>}
      {program.extended.map((item, index) => (
        <Stack key={index} gap={2}>
          <Text size="sm" fw={700}>
            {item.heading}
          </Text>
          <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>
            {item.text}
          </Text>
        </Stack>
      ))}
    </Stack>
  );
}
