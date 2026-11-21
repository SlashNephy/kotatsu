import {
  Anchor,
  Badge,
  Button,
  Group,
  Image,
  Popover,
  Stack,
  Text,
  TextInput,
} from '@mantine/core';
import type { TFunction } from 'i18next';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import type { DramaEpisodeOverrideMutation } from './__generated__/DramaEpisodeOverrideMutation.graphql';
import { DramaWatchedButton, DramaWatchStatusMenu, watchStatusOf } from './DramaWatchControls';
import tmdbLogo from './tmdb-logo.svg';

// TMDB_NOTICE は、TMDB の API の利用規約が求める帰属の表示。英語の原文のまま出す。
export const TMDB_NOTICE =
  'This product uses the TMDB API but is not endorsed or certified by TMDB.';

// DramaEpisodeItem は、番組や録画に結びついた TMDB の作品と話 (#1651、設計書 3.9)。
export type DramaEpisodeItem = {
  readonly series: {
    readonly tmdbId: number;
    readonly name: string | null | undefined;
    readonly originalName: string | null | undefined;
    readonly url: string;
    // watchStatus は、ログイン中のユーザーが作品を見ている作品に入れた状態 (#1652)。引かなければ undefined。
    readonly watchStatus?: string | null | undefined;
  };
  readonly seriesSource: 'SEARCH' | 'OVERRIDE' | '%future added value';
  readonly seasonNumber: number | null | undefined;
  readonly episodeNumber: number | null | undefined;
  readonly episodeName: string | null | undefined;
  readonly url: string;
  // watched は、ログイン中のユーザーがこの話を見たかどうか (#1652)。引かなければ undefined。
  readonly watched?: boolean | undefined;
};

// dramaEpisodeLabel は、シーズンと話を「シーズン7 第7話」や「特別編 第1話」の形にする。話がなければ null を返す。
export function dramaEpisodeLabel(t: TFunction, episode: DramaEpisodeItem): string | null {
  if (episode.seasonNumber == null || episode.episodeNumber == null) {
    return null;
  }

  return episodeLabel(t, {
    seasonNumber: episode.seasonNumber,
    episodeNumber: episode.episodeNumber,
  });
}

// episodeLabel は、シーズンと話を「シーズン7 第7話」や「特別編 第1話」の形にする。
export function episodeLabel(
  t: TFunction,
  episode: { readonly seasonNumber: number; readonly episodeNumber: number },
): string {
  const season =
    episode.seasonNumber === 0
      ? t('drama.special')
      : t('drama.season', { number: episode.seasonNumber });

  return `${season} ${t('drama.episode', { number: episode.episodeNumber })}`;
}

// parseTMDbSeriesId は、TMDB の作品の ID か、作品のページの URL (https://www.themoviedb.org/tv/123-name) から ID を読む。
// 読めなければ null を返す。
export function parseTMDbSeriesId(value: string): number | null {
  const trimmed = value.trim();
  const match = /^(\d+)$/.exec(trimmed) ?? /themoviedb\.org\/tv\/(\d+)/.exec(trimmed);
  if (match === null) {
    return null;
  }
  const id = Number(match[1]);

  return Number.isSafeInteger(id) && id > 0 && id <= 2147483647 ? id : null;
}

// isPlaceholderEpisodeName は、話の名前 name が、TMDB が名前の決まっていない話に付ける仮の名前 (第6話、Episode 6) かどうかを返す。
export function isPlaceholderEpisodeName(
  name: string,
  episodeNumber: number | null | undefined,
): boolean {
  if (episodeNumber == null) {
    return false;
  }

  return name === `第${episodeNumber}話` || name === `Episode ${episodeNumber}`;
}

// TMDbAttribution は、TMDB の API の利用規約が求める帰属の表示 (TMDB のロゴと注記)。
// 規約は、帰属の表示をアプリの「About」か「Credits」の区画に置くよう求めるので、このアプリについての画面 (/about) にだけ出す。
// ロゴは、Kotatsu のロゴより目立たないよう、文字と同じくらいの高さにする。画像は同梱し、外部から読まない。
export function TMDbAttribution() {
  const { t } = useTranslation();

  return (
    <Group gap={6} wrap="nowrap" align="center" data-testid="tmdb-attribution">
      <Image src={tmdbLogo} alt="TMDB" h={10} w="auto" fit="contain" />
      <Text size="xs" c="dimmed" title={t('drama.attributionNote')}>
        {TMDB_NOTICE}
      </Text>
    </Group>
  );
}

// DramaEpisode は、番組と録画の詳細に、結びついた TMDB の作品とシーズン、話と、TMDB のページへのリンクを出す (#1651、設計書 3.9)。
// 結びついていなければ、何も出さない。canOverride なら、管理者が作品を直す操作を置く (programName の作品名の上書き)。
// ログイン中のユーザーの状態を引いていれば (#1652)、作品を見ている作品に足す操作と、話を見たことにする操作も置く。
// 作品名とシーズンと話、操作、リンクの順に、1 段ずつ並べる。
export function DramaEpisode({
  episode,
  programName,
  canOverride,
}: {
  episode: DramaEpisodeItem | null | undefined;
  programName: string;
  canOverride: boolean;
}) {
  const { t } = useTranslation();
  if (episode == null) {
    return null;
  }
  const label = dramaEpisodeLabel(t, episode);
  const name = episode.series.name ?? episode.series.originalName ?? t('drama.unknownSeries');
  // TMDB は、名前の決まっていない話に「第6話」のような仮の名前を付ける。話数と同じことしか言わないので出さない。
  // 話の名前は、画面の言語によらず日本語 (ja-JP) で引くので、画面の言語の文言ではなく、TMDB の仮の名前の形と比べる。
  // 日本語の名前がない話は、英語の「Episode 6」になることがあるので、それも仮の名前とみなす。
  const episodeName =
    episode.episodeName != null &&
    !isPlaceholderEpisodeName(episode.episodeName, episode.episodeNumber)
      ? episode.episodeName
      : null;

  return (
    <Stack gap="xs" data-testid="drama-episode">
      <Stack gap={2}>
        <Group gap="xs" wrap="wrap">
          <Text size="sm" fw={600}>
            {name}
          </Text>
          {episode.seriesSource === 'OVERRIDE' && (
            <Badge size="sm" variant="light" color="gray">
              {t('drama.overridden')}
            </Badge>
          )}
          {canOverride && <DramaOverrideButton programName={programName} episode={episode} />}
        </Group>
        {(label !== null || episodeName !== null) && (
          <Group gap="xs" wrap="wrap">
            {label !== null && <Text size="sm">{label}</Text>}
            {episodeName !== null && <Text size="sm">{`「${episodeName}」`}</Text>}
          </Group>
        )}
      </Stack>
      {episode.watched !== undefined && (
        // 読み直して、状態や話が変わったら、手元の状態を作り直す。
        <DramaWatch
          key={`${episode.series.tmdbId}:${episode.seasonNumber}:${episode.episodeNumber}:${episode.series.watchStatus}:${episode.watched}`}
          episode={episode}
        />
      )}
      <Group gap="sm" wrap="wrap">
        <Anchor size="sm" href={episode.url} target="_blank" rel="noopener noreferrer">
          {t('drama.tmdb')}
        </Anchor>
      </Group>
    </Stack>
  );
}

// DramaWatch は、作品を見ている作品に足す操作と、話を見たことにする操作を並べる (#1652)。
function DramaWatch({ episode }: { episode: DramaEpisodeItem }) {
  const [status, setStatus] = useState(() => watchStatusOf(episode.series.watchStatus));
  const [watched, setWatched] = useState(episode.watched === true);
  const { seasonNumber, episodeNumber } = episode;

  return (
    <Group gap="xs" wrap="wrap">
      <DramaWatchStatusMenu seriesId={episode.series.tmdbId} status={status} onChange={setStatus} />
      {seasonNumber != null && episodeNumber != null && (
        <DramaWatchedButton
          episode={{ seriesId: episode.series.tmdbId, seasonNumber, episodeNumber }}
          watched={watched}
          onChange={setWatched}
        />
      )}
    </Group>
  );
}

// DramaOverrideButton は、管理者が、番組名から読んだ作品名の TMDB の作品を直す操作 (#1651)。
// 直すと、同じ作品名の番組をまとめて照合し直す。TMDB には問い合わせないので、反映は次の照合 (nekonote) を待つ。
function DramaOverrideButton({
  programName,
  episode,
}: {
  programName: string;
  episode: DramaEpisodeItem;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [opened, setOpened] = useState(false);
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const seriesId = parseTMDbSeriesId(value);
  const save = async (input: { tmdbSeriesId?: number; exclude?: boolean; reset?: boolean }) => {
    setBusy(true);
    try {
      await mutate<DramaEpisodeOverrideMutation>(
        environment,
        graphql`
          mutation DramaEpisodeOverrideMutation($input: DramaTitleOverrideInput!) {
            updateDramaTitleOverride(input: $input) {
              titleKey
            }
          }
        `,
        { input: { programName, ...input } },
      );
      setMessage({ ok: true, text: t('drama.override.saved') });
      setValue('');
    } catch (caught) {
      setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
    } finally {
      setBusy(false);
    }
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (seriesId !== null) {
      void save({ tmdbSeriesId: seriesId });
    }
  };

  return (
    <Popover opened={opened} onChange={setOpened} width={320} position="bottom-start" withArrow>
      <Popover.Target>
        <Button size="compact-xs" variant="subtle" onClick={() => setOpened((o) => !o)}>
          {t('drama.override.button')}
        </Button>
      </Popover.Target>
      <Popover.Dropdown>
        <form onSubmit={submit}>
          <Stack gap="xs">
            <Text size="xs" c="dimmed">
              {t('drama.override.help')}
            </Text>
            <TextInput
              size="xs"
              label={t('drama.override.series')}
              placeholder="https://www.themoviedb.org/tv/123"
              value={value}
              onChange={(event) => setValue(event.currentTarget.value)}
              error={value !== '' && seriesId === null ? t('drama.override.invalid') : undefined}
            />
            <Group gap="xs">
              <Button size="compact-xs" type="submit" disabled={busy || seriesId === null}>
                {t('drama.override.save')}
              </Button>
              <Button
                size="compact-xs"
                variant="light"
                color="red"
                disabled={busy}
                onClick={() => void save({ exclude: true })}
              >
                {t('drama.override.exclude')}
              </Button>
              {episode.seriesSource === 'OVERRIDE' && (
                <Button
                  size="compact-xs"
                  variant="light"
                  disabled={busy}
                  onClick={() => void save({ reset: true })}
                >
                  {t('drama.override.reset')}
                </Button>
              )}
            </Group>
            {message !== null && (
              <Text
                size="xs"
                c={message.ok ? 'teal' : 'red'}
                role={message.ok ? 'status' : 'alert'}
              >
                {message.text}
              </Text>
            )}
          </Stack>
        </form>
      </Popover.Dropdown>
    </Popover>
  );
}
