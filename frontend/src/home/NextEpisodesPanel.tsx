import { ActionIcon, Card, Stack, Title, Tooltip } from '@mantine/core';
import { ListPlus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  NextEpisodeItem,
  NextEpisodesFailure,
  useCreateSeriesRule,
  useReserveNextEpisode,
  type NextEpisode,
} from '../recordings/NextEpisodes';

// SeriesNextEpisode は、ホームの「続きの放送」の 1 行 (Query.nextEpisodes の要素)。
export type SeriesNextEpisode = {
  // watched は、シリーズの見た回のうち、話数の最も大きい録画。シリーズを見分けるのに使う。
  readonly watched: { readonly id: string };
  readonly program: NextEpisode;
};

// NextEpisodesPanel は、ホームの「続きの放送」(設計書 4.4、#1564)。ユーザーが見たシリーズごとに、次の放送を 1 件ずつ、
// 「予約」と「このシリーズのルールを作る」と一緒に出す。出すものがなければ何も出さない。視聴者には、ボタンを出さない (設計書 9.7)。
export function NextEpisodesPanel({
  suggestions,
  canEdit,
}: {
  suggestions: readonly SeriesNextEpisode[];
  canEdit: boolean;
}) {
  const { t } = useTranslation();
  const createRule = useCreateSeriesRule();
  const reservation = useReserveNextEpisode();
  // seriesOf は、この画面で予約を頼んだ放送の、シリーズ (見た録画の ID)。予約した放送は、読み直すとサーバーが外し、
  // 同じシリーズの次の回に入れ替わるので、画面を離れるまで、そのシリーズの行は予約した放送のまま出す。
  const [seriesOf, setSeriesOf] = useState<Readonly<Record<string, string>>>({});

  const reservedSeries = new Set(
    reservation.reserved.flatMap((episode) => {
      const series = seriesOf[episode.id];

      return series === undefined ? [] : [series];
    }),
  );
  const rows = [
    ...suggestions
      .filter((suggestion) => !reservedSeries.has(suggestion.watched.id))
      .map((suggestion) => suggestion.program),
    ...reservation.reserved,
  ].sort((a, b) => Date.parse(a.startAt) - Date.parse(b.startAt));
  if (rows.length === 0) {
    return null;
  }
  const onReserve = (episode: NextEpisode) => {
    const series = suggestions.find((suggestion) => suggestion.program.id === episode.id)?.watched
      .id;
    if (series !== undefined) {
      setSeriesOf((current) => ({ ...current, [episode.id]: series }));
    }
    reservation.reserve(episode);
  };

  return (
    <Stack gap="xs" component="section" aria-label={t('recordings.nextEpisodes.title')}>
      <Title order={3} size="h5">
        {t('recordings.nextEpisodes.title')}
      </Title>
      <Card withBorder>
        <Stack gap="xs">
          {rows.map((episode) => (
            <NextEpisodeItem
              key={episode.id}
              episode={episode}
              reservation={{ ...reservation, reserve: onReserve }}
              canEdit={canEdit}
              actions={
                <Tooltip label={t('recordings.nextEpisodes.createRule')}>
                  <ActionIcon
                    variant="subtle"
                    size="md"
                    onClick={() => createRule(episode)}
                    aria-label={t('recordings.nextEpisodes.createSeriesRule', {
                      name: episode.name,
                    })}
                  >
                    <ListPlus size={16} aria-hidden />
                  </ActionIcon>
                </Tooltip>
              }
            />
          ))}
          <NextEpisodesFailure reservation={reservation} />
        </Stack>
      </Card>
    </Stack>
  );
}
