import { Alert, Anchor, Stack } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle, useNow } from '../../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import { broadcastTime, recordingPosition, formatDuration } from '../../../yunomi/position';
import { toVideoFormat } from '../../../yunomi/route';
import {
  parseStartSeconds,
  timeshiftShareSeconds,
  timeshiftStartPosition,
} from '../../../yunomi/shareUrl';
import type { ServiceIdPlayTimeshiftQuery } from './__generated__/ServiceIdPlayTimeshiftQuery.graphql';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';
import { VodPlayer } from '../../../yunomi/VodPlayer';
import { readableParams } from '../../../futon/readableId';

// PlaySearch は、再生の画面の URL のクエリ。program は、再生する番組の始まりの時刻 (RFC 3339)。t は、再生を始める位置 (番組の始まりからの放送の秒)。
type PlaySearch = { program: string; t?: number };

export const Route = createFileRoute('/play/timeshift/$serviceId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('serviceId', 'Service'),
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = parseStartSeconds(search.t);

    return {
      program: typeof search.program === 'string' ? search.program : '',
      ...(t === undefined ? {} : { t }),
    };
  },
  component: PlayPage,
});

// PlayPage は、タイムシフトのバッファにある番組を、頭から再生する画面 (設計書 5.7)。
// バッファは、ブラウザの中で変換する経路だけで再生する。画面は、アプリのルート要素に描く (#1602)。
function PlayPage() {
  const { serviceId } = Route.useParams();
  const { program, t: start } = Route.useSearch();

  return useHostedWatch(<PlayWatch serviceId={serviceId} program={program} start={start} />);
}

// PlayWatch は、サービス serviceId の、program に始まった番組を、位置 start (秒) から再生する画面の内容。
function PlayWatch({
  serviceId,
  program,
  start,
}: {
  serviceId: string;
  program: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor
          size="sm"
          renderRoot={(props) => (
            <Link {...props} to="/play/live/$serviceId" params={{ serviceId }} />
          )}
        >
          {t('timeshift.backToLive')}
        </Anchor>
      )}
      {/* 同じルートのまま別のサービスや番組へ移ったときは、前の番組のエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={`${serviceId}:${program}`}
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
                ? t('timeshift.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player
            key={`${serviceId}:${program}`}
            serviceId={serviceId}
            startAt={program}
            start={start}
          />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生する番組。放送中は、番組の終わりと途切れた区間を知るために読み直す。
const playQuery = graphql`
  query ServiceIdPlayTimeshiftQuery($serviceId: ID!, $startAt: Time!) {
    timeshiftProgram(serviceId: $serviceId, startAt: $startAt) {
      name
      startAt
      endAt
      playableFrom
      gaps {
        from
        to
      }
      streamUrl
      hlsUrl
      service {
        name
        logoUrl
        videoFormat
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

// refreshMs は、放送中の番組を再生している間に、番組を読み直す間隔。終わった時刻が分かったら、長さを伸ばすのをやめる。
const refreshMs = 30_000;

function Player({
  serviceId,
  startAt,
  start,
}: {
  serviceId: string;
  startAt: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const variables = { serviceId, startAt };
  const { timeshiftProgram: program, transcoding } = useLazyLoadQuery<ServiceIdPlayTimeshiftQuery>(
    playQuery,
    variables,
  );
  // 放送中なら、番組が終わるまで読み直す。
  const environment = useRelayEnvironment();
  const onAir = program.endAt == null;
  useEffect(() => {
    if (!onAir) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<ServiceIdPlayTimeshiftQuery>(environment, playQuery, variables, {
        fetchPolicy: 'network-only',
      })
        .toPromise()
        .catch(() => {});
    }, refreshMs);

    return () => clearInterval(timer);
    // variables は描画ごとに作り直すが、内容は serviceId と startAt だけで決まる。
  }, [environment, onAir, serviceId, startAt]);
  // now は、今の時刻。放送中の番組の長さを、再生している間も 1 秒ごとに伸ばす。
  const now = useNow(1000);
  // duration は、再生できる長さ (秒)。途切れた区間の分は、バッファに映像がないので差し引く。放送中は今まで伸ばす。
  const duration = recordingPosition(
    program.playableFrom,
    program.gaps,
    program.endAt == null ? now : Date.parse(program.endAt),
  );
  // initialPosition は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ再生できる範囲の始まりから。
  const [initialPosition] = useState(() =>
    start === undefined ? 0 : Math.min(timeshiftStartPosition(program, start), duration),
  );
  // gapMarks は、バッファが途切れた位置のマーク。途切れた区間はシークバーの長さに含めないので、長さのないマークにする。
  const gapMarks = program.gaps.map((gap) => {
    const at = recordingPosition(program.playableFrom, program.gaps, Date.parse(gap.from));
    const length = Math.max((Date.parse(gap.to) - Date.parse(gap.from)) / 1000, 0);

    return { from: at, to: at, label: t('player.gapMark', { duration: formatDuration(length) }) };
  });

  return (
    <VodPlayer
      source={{
        kind: 'TIMESHIFT',
        target: serviceId,
        // バッファの番組には ID がないので、局と始まりの時刻で見分ける。
        program: { id: `${serviceId}:${startAt}`, name: program.name },
        title: program.name === '' ? t('timeshift.untitled') : <ProgramTitle name={program.name} />,
        serviceName: program.service.name,
        serviceLogoUrl: program.service.logoUrl ?? null,
        time: {
          startAt: program.startAt,
          endAt: program.endAt ?? program.startAt,
          endUndecided: program.endAt == null,
        },
        description: null,
        artwork: null,
        previewUrl: null,
        transcoding,
        videoFormat: toVideoFormat(program.service.videoFormat),
        // デコードが追いつかずにほかの画面で下げた段階の上限は、段階を下げる放送中の番組でだけ引き継ぐ (#1513)。
        inheritCap: onAir,
        streamUrl: program.streamUrl,
        hlsUrl: program.hlsUrl,
        // バッファの番組の音声の構成は分からないので、プレイヤーに任せる。
        audios: null,
        duration,
        seekLimit: duration,
        opening: { at: initialPosition, skipped: false, resumed: false },
        initialSeek: start === undefined ? null : initialPosition,
        initial: initialPosition,
        growing: onAir,
        pauseAtEnd: true,
        clockAt: (seconds) => broadcastTime(program.playableFrom, program.gaps, seconds),
        // 共有する位置は、番組の始まりからの放送の秒にする。
        shareSeconds: (seconds) => timeshiftShareSeconds(program, seconds),
        bands: gapMarks,
        cmSections: null,
        jikkyo: null,
        autoCaptions: [],
        // タイムシフトの再生は、ほかの端末から操作させない。
        remote: false,
        // データ放送は、HD の映像なら、映像と同じ位置から読んで出す (#2270)。
        dataBroadcast: true,
        topBitrate: null,
        // タイムシフトの再生は、プレイヤーが先まで読むので測れない。平均も分からないので出さない。
        streamInfo: { average: null },
        recordingId: null,
        // スリープタイマーと「再生をやめる」のキーは、プレイヤーを閉じて、上の「ライブ視聴に戻る」と同じ画面へ移る (#1426)。
        stop: () => void navigate({ to: '/play/live/$serviceId', params: { serviceId } }),
      }}
    />
  );
}
