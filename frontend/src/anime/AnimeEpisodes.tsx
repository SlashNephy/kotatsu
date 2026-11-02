import { Anchor, Badge, Group, Stack, Text } from '@mantine/core';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

// AnimeWorkItem は、TID に対応する作品のページ。
export type AnimeWorkItem = {
  readonly annictUrl: string | null | undefined;
  readonly anilistUrl: string | null | undefined;
  readonly malUrl: string | null | undefined;
};

// AnimeEpisodeItem は、番組や録画に結びついた話 (設計書 3.8)。
export type AnimeEpisodeItem = {
  readonly syobocalUrl: string | null | undefined;
  readonly countStart: number | null | undefined;
  readonly countEnd: number | null | undefined;
  readonly subtitle: string | null | undefined;
  readonly first: boolean;
  readonly last: boolean;
  readonly rerun: boolean;
  readonly works: readonly AnimeWorkItem[];
};

// episodeLabel は、話数を「第5話」や「第1〜2話」の形にする。話数がなければ null を返す。
export function episodeLabel(
  t: TFunction,
  countStart: number | null | undefined,
  countEnd: number | null | undefined,
): string | null {
  if (countStart == null) {
    return null;
  }
  if (countEnd == null || countEnd === countStart) {
    return t('anime.episode', { number: countStart });
  }

  return t('anime.episodeRange', { from: countStart, to: countEnd });
}

// episodeLinks は、話のリンクを、Annict、AniList、MyAnimeList、しょぼいカレンダーの順に、重ねずに並べる。
export function episodeLinks(
  t: TFunction,
  episode: AnimeEpisodeItem,
): { label: string; href: string }[] {
  const result: { label: string; href: string }[] = [];
  const add = (label: string, href: string | null | undefined) => {
    if (href != null && !result.some((link) => link.href === href)) {
      result.push({ label, href });
    }
  };
  for (const work of episode.works) {
    add(t('anime.annict'), work.annictUrl);
    add(t('anime.anilist'), work.anilistUrl);
    add(t('anime.myAnimeList'), work.malUrl);
  }
  add(t('anime.syobocal'), episode.syobocalUrl);

  return result;
}

// AnimeEpisodes は、番組と録画の詳細に、結びついた話の話数とサブタイトル、作品のページへのリンクを出す (設計書 3.8)。
// 2 話連続の放送のように、1 つの番組が複数の話に結びつけば、1 話ずつ並べる。結びついた話がなければ、何も出さない。
export function AnimeEpisodes({ episodes }: { episodes: readonly AnimeEpisodeItem[] }) {
  const { t } = useTranslation();
  if (episodes.length === 0) {
    return null;
  }

  return (
    <Stack gap={4} data-testid="anime-episodes">
      {episodes.map((episode) => {
        const label = episodeLabel(t, episode.countStart, episode.countEnd);

        return (
          // 話は ID を持たないので、作品と話数とサブタイトルで見分ける。
          <Group
            key={`${episode.syobocalUrl}/${episode.countStart}/${episode.countEnd}/${episode.subtitle}`}
            gap="xs"
            wrap="wrap"
          >
            {label !== null && (
              <Text size="sm" fw={600}>
                {label}
              </Text>
            )}
            {episode.subtitle != null && <Text size="sm">{`「${episode.subtitle}」`}</Text>}
            {episode.first && (
              <Badge size="sm" variant="light" color="teal">
                {t('anime.first')}
              </Badge>
            )}
            {episode.last && (
              <Badge size="sm" variant="light" color="grape">
                {t('anime.last')}
              </Badge>
            )}
            {episode.rerun && (
              <Badge size="sm" variant="light" color="gray">
                {t('anime.rerun')}
              </Badge>
            )}
            {episodeLinks(t, episode).map((link) => (
              <Anchor
                key={link.href}
                size="sm"
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
              >
                {link.label}
              </Anchor>
            ))}
          </Group>
        );
      })}
    </Stack>
  );
}
