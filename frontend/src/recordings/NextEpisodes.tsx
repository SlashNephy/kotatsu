import { Alert, Badge, Button, Card, Group, Stack, Text } from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { CalendarCheck, CalendarPlus, ListPlus, Tv } from 'lucide-react';
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

// NextEpisode は、続きの放送の番組 (Recording.nextEpisodes の要素)。
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
  const navigate = useNavigate();
  const [reserve, reserving] = useMutation<NextEpisodesReserveMutation>(graphql`
    mutation NextEpisodesReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [busy, setBusy] = useState<string | null>(null);
  // reserved は、この画面で予約した放送。予約した放送は、読み直すとサーバーが外すので、ここで覚えて「予約済み」として出し続ける。
  const [reserved, setReserved] = useState<readonly NextEpisode[]>([]);
  const [failed, setFailed] = useState(false);

  const reservedIds = new Set(reserved.map((episode) => episode.id));
  const shown = [...episodes.filter((episode) => !reservedIds.has(episode.id)), ...reserved].sort(
    (a, b) => Date.parse(a.startAt) - Date.parse(b.startAt),
  );
  const first = shown[0];
  if (first === undefined) {
    return null;
  }
  // ルールは、番組名から読んだ作品名をキーワードにし、局を限って作る。番組の詳細の「この番組からルールを作成」と同じ条件 (#1240)。
  const base = shown.find((episode) => episode.service.id === serviceId) ?? first;
  const createRule = () =>
    void navigate({
      to: '/rules/$ruleId',
      params: { ruleId: 'new' },
      state: {
        ruleConditions: conditionsFromProgram(base.name, base.service.id, base.ruleKeyword),
      },
    });
  const onReserve = (episode: NextEpisode) => {
    setFailed(false);
    setBusy(episode.id);
    reserve({
      variables: { program: episode.id },
      onCompleted: (_, errors) => {
        setBusy(null);
        if (errors != null && errors.length > 0) {
          setFailed(true);

          return;
        }
        setReserved((current) => [...current, episode]);
      },
      onError: () => {
        setBusy(null);
        setFailed(true);
      },
    });
  };

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
              onClick={createRule}
            >
              {t('recordings.nextEpisodes.createRule')}
            </Button>
          )}
        </Group>
        {shown.map((episode) => (
          <Group key={episode.id} gap="sm" wrap="nowrap" justify="space-between">
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
            {reservedIds.has(episode.id) ? (
              <Badge
                color="teal"
                variant="light"
                tt="none"
                style={{ flexShrink: 0 }}
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
                  loading={reserving && busy === episode.id}
                  disabled={reserving && busy !== episode.id}
                  onClick={() => onReserve(episode)}
                  aria-label={t('recordings.nextEpisodes.reserveProgram', { name: episode.name })}
                >
                  {t('recordings.nextEpisodes.reserve')}
                </Button>
              )
            )}
          </Group>
        ))}
        {failed && (
          <Alert color="red" role="alert" withCloseButton onClose={() => setFailed(false)}>
            {t('recordings.nextEpisodes.reserveError')}
          </Alert>
        )}
      </Stack>
    </Card>
  );
}
