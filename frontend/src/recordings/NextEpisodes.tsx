import { Alert, Badge, Button, Card, Group, Stack, Text } from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { CalendarCheck, CalendarPlus, ListPlus, Tv } from 'lucide-react';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { conditionsFromProgram } from '../conditions/form';
import { ProgramTitle, zonedFormat } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';
import type { NextEpisodesReserveMutation } from './__generated__/NextEpisodesReserveMutation.graphql';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// NextEpisode は、続きの放送の番組 (Recording.nextEpisodes の要素と、Query.nextEpisodes の program)。
export type NextEpisode = {
  readonly id: string;
  readonly name: string;
  readonly startAt: string;
  readonly ruleKeyword: string;
  readonly service: {
    readonly id: string;
    readonly name: string;
    readonly logoUrl: string | null | undefined;
  };
};

// useCreateSeriesRule は、続きの放送 episode の作品名と局を条件にして、ルールの作成の画面を開く関数を返す。
// 番組の詳細の「この番組からルールを作成」と同じ条件 (#1240)。
export function useCreateSeriesRule() {
  const navigate = useNavigate();

  return (episode: NextEpisode) =>
    void navigate({
      to: '/rules/$ruleId',
      params: { ruleId: 'new' },
      state: {
        ruleConditions: conditionsFromProgram(
          episode.name,
          episode.service.id,
          episode.ruleKeyword,
        ),
      },
    });
}

// useReserveNextEpisode は、続きの放送を予約する。予約した放送は、読み直すとサーバーが外すので、画面を離れるまで保持する。
export function useReserveNextEpisode() {
  const [commit, reserving] = useMutation<NextEpisodesReserveMutation>(graphql`
    mutation NextEpisodesReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [busy, setBusy] = useState<string | null>(null);
  const [reserved, setReserved] = useState<readonly NextEpisode[]>([]);
  const [failed, setFailed] = useState(false);
  const reserve = (episode: NextEpisode) => {
    setFailed(false);
    setBusy(episode.id);
    commit({
      variables: { program: episode.id },
      onCompleted: (_, errors) => {
        setBusy(null);
        if (errors != null && errors.length > 0) {
          setFailed(true);

          return;
        }
        setReserved((current) =>
          current.some((other) => other.id === episode.id) ? current : [...current, episode],
        );
      },
      onError: () => {
        setBusy(null);
        setFailed(true);
      },
    });
  };

  return {
    // reserved は、この画面で予約した放送。
    reserved,
    reserve,
    // busy は、予約を頼んでいる放送の ID。
    busy: reserving ? busy : null,
    failed,
    dismissFailure: () => setFailed(false),
  };
}

export type NextEpisodeReserve = ReturnType<typeof useReserveNextEpisode>;

// NextEpisodeItem は、続きの放送の 1 行。番組名、日時、局と、「予約」のボタン (予約したら「予約済み」) を出す。
// actions は、「予約」の後ろに並べる操作。視聴者には、予約できないので、ボタンを出さない (設計書 9.7)。
export function NextEpisodeItem({
  episode,
  reservation,
  canEdit,
  actions,
}: {
  episode: NextEpisode;
  reservation: NextEpisodeReserve;
  canEdit: boolean;
  actions?: ReactNode;
}) {
  const { t } = useTranslation();
  const reserved = reservation.reserved.some((other) => other.id === episode.id);

  return (
    <Group gap="sm" wrap="nowrap" justify="space-between">
      <Stack gap={0} style={{ minWidth: 0 }}>
        <Text size="sm" truncate>
          <ProgramTitle name={episode.name} />
        </Text>
        <Group gap="xs" wrap="nowrap">
          <Text size="xs" c="dimmed">
            {dateTimeFormat.format(Date.parse(episode.startAt))}
          </Text>
          <Text size="xs" c="dimmed" truncate>
            <ServiceLabel name={episode.service.name} logoUrl={episode.service.logoUrl} />
          </Text>
        </Group>
      </Stack>
      <Group gap={4} wrap="nowrap" style={{ flexShrink: 0 }}>
        {reserved ? (
          <Badge
            color="teal"
            variant="light"
            tt="none"
            leftSection={<CalendarCheck size={12} aria-hidden />}
          >
            {t('recordings.nextEpisodes.reserved')}
          </Badge>
        ) : (
          canEdit && (
            <Button
              size="xs"
              variant="light"
              leftSection={<CalendarPlus size={14} aria-hidden />}
              loading={reservation.busy === episode.id}
              disabled={reservation.busy !== null && reservation.busy !== episode.id}
              onClick={() => reservation.reserve(episode)}
              aria-label={t('recordings.nextEpisodes.reserveProgram', { name: episode.name })}
            >
              {t('recordings.nextEpisodes.reserve')}
            </Button>
          )
        )}
        {canEdit && actions}
      </Group>
    </Group>
  );
}

// NextEpisodesFailure は、予約できなかったことを通知する。
export function NextEpisodesFailure({ reservation }: { reservation: NextEpisodeReserve }) {
  const { t } = useTranslation();
  if (!reservation.failed) {
    return null;
  }

  return (
    <Alert color="red" role="alert" withCloseButton onClose={reservation.dismissFailure}>
      {t('recordings.nextEpisodes.reserveError')}
    </Alert>
  );
}

// NextEpisodes は、録画の詳細に出す、録画のシリーズの続きの放送 (設計書 4.4、#1564)。放送ごとの「予約」と、
// シリーズの「このシリーズのルールを作る」を置く。続きの放送がなければ何も出さない。視聴者には、予約もルールも作れないので、
// ボタンを出さない (設計書 9.7)。
export function NextEpisodes({
  episodes,
  serviceId,
  canEdit,
}: {
  episodes: readonly NextEpisode[];
  // serviceId は、録画のサービス。ルールの条件には、このサービスの放送があればそれを、なければ最初の放送を使う。
  serviceId: string;
  canEdit: boolean;
}) {
  const { t } = useTranslation();
  const createRule = useCreateSeriesRule();
  const reservation = useReserveNextEpisode();

  const reservedIds = new Set(reservation.reserved.map((episode) => episode.id));
  const shown = [
    ...episodes.filter((episode) => !reservedIds.has(episode.id)),
    ...reservation.reserved,
  ].sort((a, b) => Date.parse(a.startAt) - Date.parse(b.startAt));
  const first = shown[0];
  if (first === undefined) {
    return null;
  }
  // ルールは、番組名から読んだ作品名をキーワードにし、局を限って作る。
  const base = shown.find((episode) => episode.service.id === serviceId) ?? first;

  return (
    <Card withBorder component="section" aria-label={t('recordings.nextEpisodes.title')}>
      <Stack gap="xs">
        <Group gap={6} justify="space-between" wrap="wrap">
          <Group gap={6} wrap="nowrap">
            <Tv size={16} aria-hidden />
            <Text component="span" size="sm" fw={700}>
              {t('recordings.nextEpisodes.title')}
            </Text>
          </Group>
          {canEdit && (
            <Button
              size="xs"
              variant="subtle"
              leftSection={<ListPlus size={14} aria-hidden />}
              onClick={() => createRule(base)}
            >
              {t('recordings.nextEpisodes.createRule')}
            </Button>
          )}
        </Group>
        {shown.map((episode) => (
          <NextEpisodeItem
            key={episode.id}
            episode={episode}
            reservation={reservation}
            canEdit={canEdit}
          />
        ))}
        <NextEpisodesFailure reservation={reservation} />
      </Stack>
    </Card>
  );
}
