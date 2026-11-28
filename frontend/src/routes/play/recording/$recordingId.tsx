import { Alert, Anchor, Stack } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useListState } from '../../../recordings/listSearch';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle, useNow } from '../../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import { RuleRecordingList } from '../../../recordings/RuleRecordingList';
import { broadcastTime, recordingPosition, formatDuration } from '../../../yunomi/position';
import { toVideoFormat } from '../../../yunomi/route';
import { useWatchPositionSaver } from '../../../yunomi/useWatchPositionSaver';
import { usePlaybackReporter } from '../../../animeRecords/usePlaybackReporter';
import { averageBitrate } from '../../../yunomi/streamMeter';
import { kindBitrates } from '../../../recordings/streamQuality';
import { loadCmAutoSkip } from '../../../yunomi/cmAutoSkip';
import { cmMergeGap, cmSectionAt, mergeCmSections } from '../../../yunomi/cmSkip';
import { browserStorage } from '../../../yunomi/volume';
import { playableStart } from '../../../yunomi/reconnect';
import { resumePosition } from '../../../yunomi/resume';
import { parseStartSeconds } from '../../../yunomi/shareUrl';
import type { RecordingIdPlayQuery } from './__generated__/RecordingIdPlayQuery.graphql';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';
import { readableParams } from '../../../futon/readableId';
import { VodPlayer } from '../../../yunomi/VodPlayer';
import { autoCaptionPositions } from '../../../yunomi/AutoCaptionOverlay';
import type { PlayerMedia } from '../../../yunomi/playerMedia';

// PlaySearch は、再生の画面の URL のクエリ。t は、再生を始める位置 (録画の始まりからの秒)。キャプチャの履歴から撮った場面を開くのと、今の位置の URL の共有に使う (#457)。
type PlaySearch = { t?: number };

export const Route = createFileRoute('/play/recording/$recordingId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('recordingId', 'Recording'),
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = parseStartSeconds(search.t);

    return t === undefined ? {} : { t };
  },
  component: PlayPage,
});

// PlayPage は、録画を再生する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
// 画面は、アプリのルートで描く (#1602)。
function PlayPage() {
  const { recordingId } = Route.useParams();
  const { t: start } = Route.useSearch();

  return useHostedWatch(<PlayWatch recordingId={recordingId} start={start} />);
}

// PlayWatch は、録画 recordingId を、位置 start (秒) から再生する画面の内容。
function PlayWatch({ recordingId, start }: { recordingId: string; start: number | undefined }) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();
  // 詳細に戻ったときに、開いた一覧に戻れるよう、一覧のクエリを引き継ぐ (#2146)。
  const listState = useListState();

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor
          size="sm"
          renderRoot={(props) => (
            <Link
              {...props}
              to="/recordings/$recordingId"
              params={{ recordingId }}
              state={listState}
            />
          )}
        >
          {t('player.backToRecording')}
        </Anchor>
      )}
      {/* 同じルートのまま別の録画へ移ったときは、前の録画のエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={recordingId}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor
                component={Link}
                to="/login"
                // 共有された URL をログインしていない端末で開いても、ログインの後に同じ位置から再生する。
                onClick={() => rememberReturnTo(window.location.pathname + window.location.search)}
              >
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('recordings.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player key={recordingId} recordingId={recordingId} start={start} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生する録画。録画中は、録画を終えた時刻 (recordedUntil) を知るために読み直す。
const playQuery = graphql`
  query RecordingIdPlayQuery($id: ID!) {
    node(id: $id) {
      __typename
      ... on Recording {
        name
        description
        extended {
          heading
          text
        }
        startAt
        endAt
        streamUrl
        videoFormat
        recordedFrom
        recordedUntil
        gaps {
          from
          to
        }
        cmSections {
          from
          to
        }
        autoCaptions {
          from
          to
          text
        }
        jikkyoCommentsUrl
        jikkyoOtherCommentsUrl
        jikkyoNicologCommentsUrl
        rule {
          id
          name
        }
        watchPositionMs
        thumbnailUrl
        previewUrl
        audios {
          componentType
          languages
          samplingRate
        }
        files {
          kind
          size
        }
        drops
        errors
        scrambled
        streamQuality {
          kind
          packets
        }
        service {
          name
          logoUrl
          jikkyoChannelId
          jikkyo {
            id
            name
          }
        }
      }
    }
    transcoding {
      available
      presets {
        name
        codec
        passthrough
        videoBitrate
      }
    }
  }
`;

// recordingRefreshMs は、録画中の録画を追っかけ再生している間に、録画を読み直す間隔 (#671)。
// 録画を終えた時刻が分かるまでは、録画の長さを今の時刻まで伸ばして扱うので、終えたら読み直して止める。
const recordingRefreshMs = 30_000;

function Player({ recordingId, start }: { recordingId: string; start: number | undefined }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RecordingIdPlayQuery>(playQuery, { id: recordingId });
  const navigate = useNavigate();
  const listState = useListState();
  const recording = data.node?.__typename === 'Recording' ? data.node : null;
  // 録画中なら、録画を終えるまで読み直し、終えた時刻を受け取ったら、録画の長さを伸ばすのをやめる (#671)。
  const environment = useRelayEnvironment();
  const stillRecording = recording !== null && recording.recordedUntil == null;
  useEffect(() => {
    if (!stillRecording) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<RecordingIdPlayQuery>(
        environment,
        playQuery,
        { id: recordingId },
        { fetchPolicy: 'network-only' },
      )
        .toPromise()
        .catch(() => {});
    }, recordingRefreshMs);

    return () => clearInterval(timer);
  }, [environment, stillRecording, recordingId]);
  // now は、今の時刻。録画中の録画の長さを、再生している間も 1 秒ごとに伸ばす。
  const now = useNow(1000);
  // duration は、録画の長さ (秒)。ファイルには途切れた区間の分の映像がないので、その分を差し引く。
  const duration =
    recording === null
      ? 0
      : recordingPosition(
          recording.recordedFrom,
          recording.gaps,
          recording.recordedUntil == null ? now : Date.parse(recording.recordedUntil),
        );
  // cmPositions は、CM とみなした区間 (#479) を、途切れた分を詰めた再生の位置にしたもの。録画の時間軸の日時で受け取る。
  const cmPositions =
    recording === null
      ? []
      : recording.cmSections.map((section) => ({
          from: recordingPosition(recording.recordedFrom, recording.gaps, Date.parse(section.from)),
          to: recordingPosition(recording.recordedFrom, recording.gaps, Date.parse(section.to)),
        }));
  // resumeAt は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ前回の続きから、続きもなければ 0 (設計書 7.5)。
  // Int64 は Relay の型では any になるので、数に直す。
  const savedMs = recording?.watchPositionMs == null ? null : Number(recording.watchPositionMs);
  // CM を自動で飛ばす設定で、URL で位置を渡されずに開いた位置が CM の中なら、最初から CM の終わりを求める (#1512)。
  // 開いてから飛ぶと、頭の CM を飛んでから続きへ移ったり、読み直すたびに通知が出たりする。CM の区間も開始位置も、
  // 開いた時点で揃っているので、ここで一度だけ決める。URL で渡された位置は、自分でシークしたときと同じく飛ばさない。
  const [opening] = useState(() => {
    if (start !== undefined) {
      return { at: Math.min(start, duration), skipped: false, resumed: false };
    }
    const at = resumePosition(savedMs, duration);
    const span = loadCmAutoSkip(browserStorage())
      ? cmSectionAt(mergeCmSections(cmPositions, cmMergeGap), at)
      : null;

    return span === null
      ? { at, skipped: false, resumed: at > 0 }
      : { at: Math.min(span.to, duration), skipped: true, resumed: at > 0 };
  });
  // 終わりの直前から読むと、映像のフレームが届かずに失敗するので、映像が入るだけ手前から始める (#1576)。
  const resumeAt = playableStart(opening.at, duration);

  if (recording === null) {
    return <Alert color="amber">{t('recordings.notFound')}</Alert>;
  }
  const { recordedFrom, gaps } = recording;
  // gapMarks は、録画が途切れた位置のマーク (#479)。途切れた区間はシークバーの長さに含めないので (#754)、長さのないマークにし、
  // カーソルを乗せたら、途切れていた長さを出す。
  const gapMarks = gaps.map((gap) => {
    const at = recordingPosition(recordedFrom, gaps, Date.parse(gap.from));
    const length = Math.max((Date.parse(gap.to) - Date.parse(gap.from)) / 1000, 0);

    return { from: at, to: at, label: t('player.gapMark', { duration: formatDuration(length) }) };
  });
  // channelId は、実況のチャンネルの ID。実況を切っているユーザーにもボタンを出すため、外部へ問い合わせずに返る値を使う (設計書 7.4)。
  const channelId = recording.service.jikkyoChannelId ?? null;

  return (
    <VodPlayer
      source={{
        kind: 'RECORDING',
        target: recordingId,
        program: { id: recordingId, name: recording.name },
        title: <ProgramTitle name={recording.name} />,
        serviceName: recording.service.name,
        serviceLogoUrl: recording.service.logoUrl ?? null,
        time: recording,
        description: { text: recording.description, extended: recording.extended },
        artwork: recording.thumbnailUrl ?? null,
        previewUrl: recording.previewUrl ?? null,
        // BS4K と 8K の録画では、経路の選び方が変わる (#806)。
        transcoding: data.transcoding,
        videoFormat: toVideoFormat(recording.videoFormat),
        // デコードが追いつかずにほかの画面で下げた段階の上限は、段階を下げる追っかけ再生でだけ引き継ぐ (#1513)。
        inheritCap: stillRecording,
        streamUrl: recording.streamUrl,
        // kyusu の HLS は、録画の全体を 1 つのプレイリストで表すので、video の時刻がそのまま録画の中の位置になる。
        hlsUrl: recording.streamUrl,
        // kyusu の経路では、録画を始めたときの番組の音声の構成から選ぶ。
        audios: recording.audios,
        duration,
        seekLimit: duration,
        opening,
        initialSeek: start === undefined ? null : resumeAt,
        initial: resumeAt,
        // 終わりの直前から読むと、映像のフレームが届かずに失敗するので、映像が入るだけ手前から読む (#1576)。
        // CM の区間が録画の終わりまで続く回で、CM を飛ばすと、ここに来る。
        restartFrom: (seconds) => playableStart(seconds, duration),
        growing: stillRecording,
        // 画質のメニューから、この端末でデコードする経路も選べる (#2166)。
        decode: true,
        clockAt: (seconds) => broadcastTime(recordedFrom, gaps, seconds),
        bands: gapMarks,
        cmSections: cmPositions,
        jikkyo:
          channelId === null
            ? null
            : {
                url: recording.jikkyoCommentsUrl,
                // 録画中の録画を追っかけ再生している間は、リアルタイムのコメントも流す (#667)。
                live: stillRecording ? { channelId, recordedFrom, gaps } : null,
                others: recording.jikkyoOtherCommentsUrl,
                // 上映会のコメント (NICOLOG) は、録画自身の過去ログの `OP` で合わせるので、CM の区間があるときだけ読む (#1363)。
                nicolog:
                  cmPositions.length === 0
                    ? null
                    : { url: recording.jikkyoNicologCommentsUrl, cmSections: cmPositions },
                growing: false,
                recordedFrom,
                gaps,
              },
        autoCaptions: autoCaptionPositions(recordedFrom, gaps, recording.autoCaptions),
        // 同じアカウントのほかの端末から、このプレイヤーを操作できるようにする (#459、設計書 7.7)。
        remote: true,
        // データ放送は、HD の映像なら、映像と同じ位置から読んで出す (#2270)。
        dataBroadcast: true,
        // 録画は先に読むので、受け取りの速さで余裕を確認して上げる。オリジナルのビットレートは、録画の平均で比較する。
        topBitrate: averageBitrate(recording, now),
        streamInfo: {
          average: averageBitrate(recording, now),
          recordedKinds: kindBitrates(recording.streamQuality, duration),
          transport: {
            dropped: Number(recording.drops),
            errors: Number(recording.errors),
            scrambled: Number(recording.scrambled),
          },
        },
        recordingId,
        stop: () =>
          void navigate({
            to: '/recordings/$recordingId',
            params: { recordingId },
            state: listState,
          }),
        side: recording.rule != null && (
          // 一覧は読み込みを待たず、映像を先に出す。
          <Suspense fallback={null}>
            <RuleRecordingList
              ruleId={recording.rule.id}
              ruleName={recording.rule.name}
              recordingId={recordingId}
            />
          </Suspense>
        ),
      }}
    >
      {(api) => (
        <RecordingPlaybackTracker
          recordingId={recordingId}
          media={api.media}
          currentPosition={api.currentPosition}
          playing={!api.paused}
          clockAt={(seconds) => broadcastTime(recordedFrom, gaps, seconds)}
        />
      )}
    </VodPlayer>
  );
}

// RecordingPlaybackTracker は、録画の再生位置を保存し (設計書 7.5)、実際に再生した区間を送って、見たアニメを記録させる (#927)。
function RecordingPlaybackTracker({
  recordingId,
  media,
  currentPosition,
  playing,
  clockAt,
}: {
  recordingId: string;
  media: PlayerMedia | null;
  currentPosition: () => number;
  playing: boolean;
  clockAt: (seconds: number) => number;
}) {
  useWatchPositionSaver(recordingId, currentPosition, playing);
  usePlaybackReporter({ recordingId }, media, () => clockAt(currentPosition()));

  return null;
}
