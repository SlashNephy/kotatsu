import { Button, Group, Menu, Text } from '@mantine/core';
import { Check, ListChecks, ListPlus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import type { DramaWatchControlsStatusMutation } from './__generated__/DramaWatchControlsStatusMutation.graphql';
import type { DramaWatchControlsWatchedMutation } from './__generated__/DramaWatchControlsWatchedMutation.graphql';

// DramaWatchStatus は、見ている作品の状態 (#1652)。
export type DramaWatchStatus = 'WATCHING' | 'WANNA_WATCH';

const statuses: readonly DramaWatchStatus[] = ['WATCHING', 'WANNA_WATCH'];

// watchStatusOf は、サーバーが返した状態を、画面で扱う状態にする。知らない値と null は、入れていないものとする。
export function watchStatusOf(value: string | null | undefined): DramaWatchStatus | null {
  return statuses.find((status) => status === value) ?? null;
}

// useSetDramaWatchStatus は、ログイン中のユーザーの見ている作品を変える関数を返す (#1652)。status が null なら外す。
export function useSetDramaWatchStatus() {
  const environment = useRelayEnvironment();

  return (seriesId: number, status: DramaWatchStatus | null) =>
    mutate<DramaWatchControlsStatusMutation>(
      environment,
      graphql`
        mutation DramaWatchControlsStatusMutation($seriesId: Int!, $status: DramaWatchStatus) {
          setDramaWatchStatus(seriesId: $seriesId, status: $status) {
            tmdbId
            watchStatus
          }
        }
      `,
      { seriesId, status },
    );
}

// useSetDramaEpisodeWatched は、ログイン中のユーザーが話を見たことを、手で付けるか外す関数を返す (#1652)。
export function useSetDramaEpisodeWatched() {
  const environment = useRelayEnvironment();

  return (
    episode: { seriesId: number; seasonNumber: number; episodeNumber: number },
    watched: boolean,
  ) =>
    mutate<DramaWatchControlsWatchedMutation>(
      environment,
      graphql`
        mutation DramaWatchControlsWatchedMutation(
          $seriesId: Int!
          $seasonNumber: Int!
          $episodeNumber: Int!
          $watched: Boolean!
        ) {
          setDramaEpisodeWatched(
            seriesId: $seriesId
            seasonNumber: $seasonNumber
            episodeNumber: $episodeNumber
            watched: $watched
          )
        }
      `,
      { ...episode, watched },
    );
}

// DramaWatchStatusMenu は、作品を見ている作品に足し、状態を変え、外すメニュー (#1652)。変えたら onChange を呼ぶ。
export function DramaWatchStatusMenu({
  seriesId,
  status,
  onChange,
}: {
  seriesId: number;
  status: DramaWatchStatus | null;
  onChange: (status: DramaWatchStatus | null) => void;
}) {
  const { t } = useTranslation();
  const setStatus = useSetDramaWatchStatus();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const change = async (next: DramaWatchStatus | null) => {
    setBusy(true);
    setError(null);
    try {
      await setStatus(seriesId, next);
      onChange(next);
    } catch (caught) {
      setError(settingsErrorMessage(t, caught));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Group gap={4}>
      <Menu position="bottom-start" withinPortal>
        <Menu.Target>
          <Button
            size="compact-xs"
            variant={status === null ? 'default' : 'light'}
            loading={busy}
            leftSection={
              status === null ? (
                <ListPlus size={12} aria-hidden />
              ) : (
                <ListChecks size={12} aria-hidden />
              )
            }
          >
            {status === null ? t('drama.watch.add') : t(`drama.watch.status.${status}`)}
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          {statuses.map((value) => (
            <Menu.Item key={value} disabled={value === status} onClick={() => void change(value)}>
              {t(`drama.watch.status.${value}`)}
            </Menu.Item>
          ))}
          {status !== null && (
            <>
              <Menu.Divider />
              <Menu.Item color="red" onClick={() => void change(null)}>
                {t('drama.watch.remove')}
              </Menu.Item>
            </>
          )}
        </Menu.Dropdown>
      </Menu>
      {error !== null && (
        <Text size="xs" c="red" role="alert">
          {error}
        </Text>
      )}
    </Group>
  );
}

// DramaWatchedButton は、話を見たことを、手で付けるか外すボタン (#1652)。見た話なら「見た」と示し、押すと取り消す。
export function DramaWatchedButton({
  episode,
  watched,
  onChange,
  label,
}: {
  episode: { seriesId: number; seasonNumber: number; episodeNumber: number };
  watched: boolean;
  onChange: (watched: boolean) => void;
  // label は、まだ見ていないときのボタンの文字。既定は「見たことにする」。
  label?: string;
}) {
  const { t } = useTranslation();
  const setWatched = useSetDramaEpisodeWatched();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const toggle = async () => {
    setBusy(true);
    setError(null);
    try {
      await setWatched(episode, !watched);
      onChange(!watched);
    } catch (caught) {
      setError(settingsErrorMessage(t, caught));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Group gap={4}>
      <Button
        size="compact-xs"
        variant={watched ? 'light' : 'default'}
        color={watched ? 'teal' : undefined}
        loading={busy}
        leftSection={<Check size={12} aria-hidden />}
        title={watched ? t('drama.watch.unmarkHint') : undefined}
        onClick={() => void toggle()}
      >
        {watched ? t('drama.watch.watched') : (label ?? t('drama.watch.markWatched'))}
      </Button>
      {error !== null && (
        <Text size="xs" c="red" role="alert">
          {error}
        </Text>
      )}
    </Group>
  );
}
