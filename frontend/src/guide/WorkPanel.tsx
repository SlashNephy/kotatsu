import { Paper, Stack } from '@mantine/core';

import type { RecordAnimeEpisodesInput } from '../anime/__generated__/RecordAnimeButtonMutation.graphql';
import { AnimeEpisodes, type AnimeEpisodeItem } from '../anime/AnimeEpisodes';
import { DramaEpisode, type DramaEpisodeItem } from '../drama/DramaEpisode';

type WorkPanelProps = {
  animeEpisodes: readonly AnimeEpisodeItem[];
  // record を渡せば、アニメの話を手で記録するボタンを置く (AnimeEpisodes)。
  record?: RecordAnimeEpisodesInput;
  dramaEpisode: DramaEpisodeItem | null | undefined;
  programName: string;
  // canOverride なら、管理者が TMDB の作品を直す操作を置く (DramaEpisode)。
  canOverride: boolean;
};

// WorkPanel は、番組と録画の詳細に、番組が結びついた作品の情報を、枠で囲んだ 1 つの欄にまとめて出す。
// 番組そのものの情報 (局、日時、ジャンル) と見分けられるよう、作品の話、操作、作品のページへのリンクを、この欄に集める。
// 作品名でほかのサイトを探すリンクは、ここに置かず、番組名の横のメニュー (SearchElsewhereMenu) に置く。
// 結びついた作品がなければ、何も出さない。
export function WorkPanel({
  animeEpisodes,
  record,
  dramaEpisode,
  programName,
  canOverride,
}: WorkPanelProps) {
  if (animeEpisodes.length === 0 && dramaEpisode == null) {
    return null;
  }

  return (
    <Paper withBorder radius="md" p="sm">
      {/* アニメと TMDB の作品の両方に結びつくことはまずないが、あれば続けて出す。 */}
      <Stack gap="sm">
        <AnimeEpisodes episodes={animeEpisodes} record={record} />
        <DramaEpisode episode={dramaEpisode} programName={programName} canOverride={canOverride} />
      </Stack>
    </Paper>
  );
}
