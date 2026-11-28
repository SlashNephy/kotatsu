import { Alert, Anchor, Stack } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { PageLoader, ProgramTitle } from '../../../futon';
import { readableParams } from '../../../futon/readableId';
import { setTimeZone } from '../../../futon/timeZone';
import { openOfflineRecording, type OfflineFiles } from '../../../offline/store';
import type { PlayerMedia } from '../../../yunomi/playerMedia';
import { broadcastTime, recordingPosition } from '../../../yunomi/position';
import { VodPlayer } from '../../../yunomi/VodPlayer';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';

export const Route = createFileRoute('/play/offline/$recordingId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('recordingId', 'Recording'),
  staticData: { fluid: true },
  component: OfflinePlayPage,
});

// OfflinePlayPage は、端末に保存した録画を再生する画面 (#480、設計書 7.8)。サーバーにつながらなくても見られるよう、
// GraphQL を読まず、保存した TS をブラウザの中で変換して流す (設計書 7.2)。プレイヤーは、録画と同じ VodPlayer を使う (#2244)。
// 画面は、アプリのルート要素に描く (#1602)。
function OfflinePlayPage() {
  const { recordingId } = Route.useParams();

  return useHostedWatch(<OfflinePlayWatch recordingId={recordingId} />);
}

// OfflinePlayWatch は、端末に保存した録画 recordingId を再生する画面の内容。
function OfflinePlayWatch({ recordingId }: { recordingId: string }) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();
  // files は、開いた録画のファイル。読み込む前は undefined、保存していなければ null。
  const [files, setFiles] = useState<OfflineFiles | null | undefined>(undefined);
  useEffect(() => {
    let active = true;
    // 別の録画に移ったときに、前の録画のファイルを新しい録画のプレイヤーに渡さないよう、先に消す。
    setFiles(undefined);
    openOfflineRecording(recordingId).then(
      (opened) => {
        if (active) {
          setFiles(opened);
        }
      },
      () => {
        if (active) {
          setFiles(null);
        }
      },
    );

    return () => {
      active = false;
    };
  }, [recordingId]);

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor size="sm" component={Link} to="/offline">
          {t('offline.back')}
        </Anchor>
      )}
      {files === undefined ? (
        <PageLoader />
      ) : files === null ? (
        <Alert color="amber">{t('offline.notFound')}</Alert>
      ) : (
        <OfflinePlayer key={recordingId} files={files} />
      )}
    </Stack>
  );
}

// useObjectURL は、file を読む blob: の URL を作り、使い終えたら手放す。
// 変換のプレイヤーは、この URL を Range で読むので、映像の途中へのシークもプレイヤーが受け持つ。
function useObjectURL(file: Blob | null): string | null {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    if (file === null) {
      setUrl(null);

      return;
    }
    const created = URL.createObjectURL(file);
    setUrl(created);

    return () => URL.revokeObjectURL(created);
  }, [file]);

  return url;
}

// OfflinePlayer は、保存した録画 files を、端末に保存した録画の再生元として VodPlayer に渡す。
function OfflinePlayer({ files }: { files: OfflineFiles }) {
  const navigate = useNavigate();
  const { meta } = files;
  // オフラインではサーバーのタイムゾーンを読めないので、保存したときのタイムゾーンで日時を書く。
  setTimeZone(meta.timeZone);
  const videoURL = useObjectURL(files.video);
  const jikkyoURL = useObjectURL(files.jikkyo);
  // 保存した TS は、配信が切り詰めた 1 本のファイルなので、映像の時刻が、そのまま録画の始まりからの位置になる。
  // 長さは、プレイヤーがファイルから求めて映像の要素に渡す。分かるまでは、録画の時間帯から求める。
  const [duration, setDuration] = useState(() =>
    recordingPosition(meta.recordedFrom, meta.gaps, Date.parse(meta.recordedUntil)),
  );
  const [opening] = useState({ at: 0, skipped: false, resumed: false });
  if (videoURL === null) {
    return <PageLoader />;
  }

  return (
    <VodPlayer
      source={{
        kind: 'RECORDING',
        target: meta.id,
        program: { id: meta.id, name: meta.name },
        title: <ProgramTitle name={meta.name} />,
        serviceName: meta.serviceName,
        serviceLogoUrl: null,
        time: meta,
        description: null,
        artwork: null,
        previewUrl: null,
        // サーバーの変換は使えないので、ブラウザの中で変換する経路だけにする (設計書 7.2)。
        transcoding: { available: false, presets: [] },
        videoFormat: 'HD',
        inheritCap: false,
        streamUrl: videoURL,
        hlsUrl: null,
        audios: null,
        duration,
        seekLimit: duration,
        opening,
        initialSeek: null,
        initial: 0,
        growing: false,
        clockAt: (seconds) => broadcastTime(meta.recordedFrom, meta.gaps, seconds),
        bands: [],
        cmSections: null,
        jikkyo:
          files.jikkyo === null
            ? null
            : {
                url: jikkyoURL,
                live: null,
                others: null,
                nicolog: null,
                growing: false,
                recordedFrom: meta.recordedFrom,
                gaps: meta.gaps,
              },
        autoCaptions: [],
        remote: false,
        topBitrate: null,
        streamInfo: {},
        recordingId: null,
        stop: () => void navigate({ to: '/offline' }),
        local: true,
      }}
    >
      {({ media }) => <OfflineDuration media={media} onDuration={setDuration} />}
    </VodPlayer>
  );
}

// OfflineDuration は、プレイヤーがファイルから求めた長さを、onDuration で画面に渡す。描くものはない。
function OfflineDuration({
  media,
  onDuration,
}: {
  media: PlayerMedia | null;
  onDuration: (seconds: number) => void;
}) {
  useEffect(() => {
    if (media === null) {
      return;
    }
    const update = () => {
      if (Number.isFinite(media.duration) && media.duration > 0) {
        onDuration(media.duration);
      }
    };
    media.addEventListener('durationchange', update);
    update();

    return () => media.removeEventListener('durationchange', update);
    // onDuration は、画面の setState なので、変わらない。
  }, [media]);

  return null;
}
