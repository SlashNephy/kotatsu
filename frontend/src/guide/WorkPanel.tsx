import { Group, Paper, Stack } from '@mantine/core';

import type { RecordAnimeEpisodesInput } from '../anime/__generated__/RecordAnimeButtonMutation.graphql';
import { AnimeEpisodes, type AnimeEpisodeItem } from '../anime/AnimeEpisodes';
import { DramaEpisode, type DramaEpisodeItem } from '../drama/DramaEpisode';
import { filmarksSearchURL } from './filmarks';
import { FilmarksLink } from './FilmarksLink';
import { streamingSearch } from './streaming';
import { StreamingLink } from './StreamingLink';

type WorkPanelProps = {
  animeEpisodes: readonly AnimeEpisodeItem[];
  // record を渡せば、アニメの話を手で記録するボタンを置く (AnimeEpisodes)。
  record?: RecordAnimeEpisodesInput;
  dramaEpisode: DramaEpisodeItem | null | undefined;
  programName: string;
  // canOverride なら、管理者が TMDB の作品を直す操作を置く (DramaEpisode)。
  canOverride: boolean;
  genres: readonly { readonly lv1: number; readonly lv2: number }[];
  workTitle: string;
  service: { readonly name: string; readonly channelType: { readonly type: string } };
};

// WorkPanel は、番組と録画の詳細に、番組が結びついた作品の情報を、枠で囲んだ 1 つの欄にまとめて出す。
// 番組そのものの情報 (局、日時、ジャンル) と見分けられるよう、作品の話、操作、作品のページへのリンクを、この欄に集める。
// リンク (Annict などの記録先、TMDB、Filmarks、見逃し配信) は、ばらばらの行に出さず、1 行にまとめる。
// 作品に結びつかなければ、リンクだけを枠なしで出す。出すリンクもなければ、何も出さない。
export function WorkPanel({
  animeEpisodes,
  record,
  dramaEpisode,
  programName,
  canOverride,
  genres,
  workTitle,
  service,
}: WorkPanelProps) {
  const anime = animeEpisodes.length > 0;
  const hasFilmarks = filmarksSearchURL(genres, workTitle, anime) != null;
  const hasStreaming = streamingSearch(service, workTitle) !== null;
  if (!anime && dramaEpisode == null && !hasFilmarks && !hasStreaming) {
    return null;
  }
  const extraLinks = (
    <>
      <FilmarksLink genres={genres} workTitle={workTitle} anime={anime} />
      <StreamingLink service={service} workTitle={workTitle} />
    </>
  );

  // 作品に結びつかず、リンクだけを出すなら、枠で囲まずに 1 行で出す。
  if (!anime && dramaEpisode == null) {
    return (
      <Group gap="sm" wrap="wrap">
        {extraLinks}
      </Group>
    );
  }

  return (
    <Paper withBorder radius="md" p="sm">
      {anime ? (
        // アニメと TMDB の作品の両方に結びつくことはまずないが、あれば TMDB の作品を続けて出し、リンクはアニメの側に足す。
        <Stack gap="sm">
          <AnimeEpisodes episodes={animeEpisodes} record={record} extraLinks={extraLinks} />
          <DramaEpisode
            episode={dramaEpisode}
            programName={programName}
            canOverride={canOverride}
          />
        </Stack>
      ) : (
        <DramaEpisode
          episode={dramaEpisode}
          programName={programName}
          canOverride={canOverride}
          extraLinks={extraLinks}
        />
      )}
    </Paper>
  );
}
