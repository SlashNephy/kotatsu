import { Alert, Button, Group, Text } from '@mantine/core';
import { Captions, MessageSquareText, Share2 } from 'lucide-react';
import { type ComponentProps, type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CommentOffsetSetting } from '../jikkyo/CommentOffsetSetting';
import { formatCommentOffset, useCommentOffsetCommands } from '../jikkyo/commentOffset';
import {
  jikkyoListedCount,
  jikkyoPartBands,
  RecordedJikkyoList,
  recordedJikkyoLabel,
} from '../jikkyo/RecordedJikkyoList';
import { type RecordedJikkyoLive, useRecordedJikkyo } from '../jikkyo/useRecordedJikkyo';
import type { ProgramAudio } from './audio';
import {
  CaptionTrackSelect,
  PositionedAutoCaptionOverlay,
  type AutoCaption,
  type CaptionTrack,
} from './AutoCaptionOverlay';
import type { CmSection } from './cmSkip';
import type { PlayerCommands } from './commands';
import { useFastForward } from './fastForward';
import { HlsPlayer } from './HlsPlayer';
import {
  CommandButton,
  PlayButton,
  PlayerControls,
  PositionStatus,
  SeekStepButtons,
  SettingsButton,
  VolumeControl,
} from './PlayerControls';
import {
  PlayerBox,
  PlayerErrorOverlay,
  PlayerFeedback,
  PlayerFrame,
  PlayerToolButtons,
  usePlayerInputs,
  usePlayerPresence,
  usePlayerShell,
} from './playerShell';
import type { PlayerMedia } from './playerMedia';
import type { PlayerSnapshot } from './playerState';
import { useCapture } from './PlayerTools';
import { formatClock, formatDuration } from './position';
import {
  AudioDelaySetting,
  CropSelect,
  NormalizeVolumeToggle,
  QualitySelect,
  RateSelect,
  SleepTimerSelect,
  VirtualSurroundToggle,
} from './QualitySelect';
import { hlsURL, type Transcoding, type VideoFormat } from './route';
import { SeekBar } from './SeekBar';
import type { SeekBand } from './seekHover';
import { StreamInfo, type StreamAudio, useStreamMeter } from './StreamInfo';
import { TsPlayer } from './TsPlayer';
import { useAdaptiveQuality } from './useAdaptiveQuality';
import { type CmOpening, useCmSkip } from './useCmSkip';
import { usePlaybackRoute } from './usePlaybackRoute';
import { usePlayerAudio } from './usePlayerAudio';
import { usePlayerSettings } from './usePlayerSettings';
import { useRecordedCaptions } from './useRecordedCaptions';
import { isSurroundAudio, useVirtualSurround } from './useVirtualSurround';
import { useVodTimeline } from './useVodTimeline';
import { WatchDescription, WatchHeading, WatchLayout, watchCommentsHeight } from './WatchLayout';
import { WatchPanel } from './WatchPanel';

// 録画、タイムシフトのバッファ、ライブラリのファイルのように、位置を指定して読み直せる再生元を再生するプレイヤー (#2244)。
// 画面は、再生元の情報と、使える機能を VodSource にまとめて渡すだけにする。使えない機能は、null を渡して入れない。

// VodSource は、録画型の再生元。
export type VodSource = {
  // kind と target は、OS のメディア操作とほかの端末に通知するプレイヤーの種類と対象 (#811)。
  kind: 'RECORDING' | 'TIMESHIFT';
  target: string;
  // program は、番組の ID と名前。名前は、タブの題名とキャプチャの名前に使う。
  program: { id: string; name: string };
  // title は、見出しに出す番組名。
  title: ReactNode;
  serviceName: string;
  serviceLogoUrl: string | null;
  // time は、見出しに出す放送の時間。分からなければ null。
  time: ComponentProps<typeof WatchHeading>['time'];
  // description は、見出しの下に出す番組の説明。なければ null。
  description: {
    text: string;
    extended: ComponentProps<typeof WatchDescription>['extended'];
  } | null;
  artwork: string | null;
  // previewUrl は、シークバー用のプレビュー画像。なければ null。
  previewUrl: string | null;
  // transcoding と videoFormat は、経路の選び方 (設計書 7.1)。inheritCap は、ほかの画面で下げた段階の上限を引き継ぐか (#1513)。
  transcoding: Transcoding;
  videoFormat: VideoFormat;
  inheritCap: boolean;
  // streamUrl は、変換の経路で流す配信。hlsUrl は、kyusu の仮想 HLS。まだ読めなければ null。
  streamUrl: string;
  hlsUrl: string | null;
  // audios は、番組の音声の構成。分からなければ null で、プレイヤーに任せる。
  audios: readonly (ProgramAudio & StreamAudio)[] | null;
  // duration は、シークバーの長さ (秒)。seekLimit は、シークできる所。インデックスを作っている間は、長さより手前になる。
  duration: number;
  seekLimit: number;
  // opening は、開いた位置。initialSeek は、URL で位置を渡されて開いたときの、読み始めた位置。
  opening: CmOpening;
  initialSeek: number | null;
  // initial は、最初に読み始める位置。clampSeek、restartFrom、endKnown は、useVodTimeline に渡す。
  initial: number;
  clampSeek?: (seconds: number) => number;
  restartFrom?: (seconds: number) => number;
  endKnown?: boolean;
  // growing は、再生元の終わりが伸びているか (録画中、放送中)。早回しを端の手前で止め、デコードが追いつかなければ段階を下げる。
  growing: boolean;
  // pauseAtEnd は、終わりまで再生したら、次へ進まずに止めるか。タイムシフトのバッファは、番組の後も続いているので止める。
  pauseAtEnd?: boolean;
  // clockAt は、再生の位置に当たる放送時刻。分からなければ null。
  clockAt: (seconds: number) => number | null;
  // shareSeconds は、共有する URL に入れる位置。既定は、再生の位置。
  shareSeconds?: (seconds: number) => number;
  // bands は、シークバーに重ねる、再生元の帯とマーク (途切れた区間、インデックスを作っている所など)。
  bands: readonly SeekBand[];
  // cmSections は、CM とみなした区間。CM を扱わない再生元は null。
  cmSections: readonly CmSection[] | null;
  // jikkyo は、実況の過去ログ (設計書 7.4)。実況のチャンネルがなければ null。
  jikkyo: {
    // url は、過去ログ。まだ読めなければ null。
    url: string | null;
    // live は、録画中の追っかけ再生で流すリアルタイムのコメント (#667)。
    live: RecordedJikkyoLive | null;
    // others は、同じ話の別の放送の過去ログ (#1361)。
    others: string | null;
    // nicolog は、上映会のコメント (#1363)。設定でオプトインしていれば読む。
    nicolog: { url: string; cmSections: readonly CmSection[] } | null;
    // growing は、過去ログの時間帯がまだ伸びるか (#1910)。
    growing: boolean;
    // recordedFrom と gaps は、一覧の時刻を放送時刻で出すための録り始めと途切れた区間。録り始めが分からなければ null。
    recordedFrom: string | null;
    gaps: ComponentProps<typeof RecordedJikkyoList>['gaps'];
  } | null;
  // autoCaptions は、自動生成した字幕 (#1682)。再生の位置で渡す。
  autoCaptions: readonly AutoCaption[];
  // remote は、ほかの端末から操作させるか (#459)。
  remote: boolean;
  // network は、画質を自動で上げるときに比較するオリジナルのビットレートと、再生元に終わりがあるか。
  topBitrate: number | null;
  // streamInfo は、右の欄の「情報」に添える、再生元ごとの値 (#828)。
  streamInfo: Pick<ComponentProps<typeof StreamInfo>, 'average' | 'recordedKinds' | 'transport'>;
  // recordingId は、キャプチャに添える録画 (#457)。録画でなければ null。
  recordingId: string | null;
  // stop は、スリープタイマーと「再生をやめる」のキーで、プレイヤーを閉じて移る先へ移る (#1426)。
  stop: () => void;
  // info と side は、見出しの下と、右の欄の下に足す、再生元ごとの部品。
  info?: ReactNode;
  side?: ReactNode;
};

// VodPlayerApi は、再生元ごとの部品 (位置の保存など) に渡す、プレイヤーの今の様子。
export type VodPlayerApi = {
  media: PlayerMedia | null;
  currentPosition: () => number;
  paused: boolean;
};

// VodPlayer は、録画型の再生元を再生する。children は、再生元ごとの部品 (位置の保存など) を、プレイヤーの様子から描く。
export function VodPlayer({
  source,
  children,
}: {
  source: VodSource;
  children?: (api: VodPlayerApi) => ReactNode;
}) {
  const { t } = useTranslation();
  // shell は、再生元によらないプレイヤーの外側。別のウィンドウに出している間は、このページが隠れても、実況のコメントなどを描き続ける (設計書 7.5)。
  const shell = usePlayerShell();
  const { mediaRef, media, setMedia, visible, fullscreen, controls, videoState, showFlash } = shell;
  const playback = usePlaybackRoute(source.transcoding, source.videoFormat, {
    inheritCap: source.inheritCap,
  });
  const { route, choices, quality, setQuality } = playback;
  const hls = route.kind === 'kyusu';
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    commentSources,
    setJikkyoEnabled,
    comments: commentSettings,
    muteUser,
    captions: captionSettings,
  } = usePlayerSettings();
  const { duration } = source;
  const timeline = useVodTimeline({
    streamUrl: source.streamUrl,
    media,
    mediaRef,
    convert: route.kind === 'convert',
    hls,
    initial: source.initial,
    duration,
    ...(source.clampSeek === undefined ? {} : { clampSeek: source.clampSeek }),
    ...(source.restartFrom === undefined ? {} : { restartFrom: source.restartFrom }),
    ...(source.endKnown === undefined ? {} : { endKnown: source.endKnown }),
  });
  const {
    stream,
    hlsStream,
    position,
    positionRef,
    currentPosition,
    restartAt,
    jumpTo,
    retry,
    error,
  } = timeline;
  const [seeking, setSeeking] = useState<number | null>(null);
  // cm は、CM とみなした区間 (#479) の帯と、飛ばすボタンと、自動で飛ばす判定 (#1512)。CM を扱わない再生元では、区間がない。
  const cm = useCmSkip({
    sections: source.cmSections ?? [],
    media,
    mediaRef,
    framePosition: (time) => (hls ? hlsStream.framePosition(time) : stream.framePosition(time)),
    jumpTo,
    currentPosition,
    scrubbing: seeking !== null,
    showFlash,
    opening: source.opening,
    initialSeek: source.initialSeek,
  });
  // seek は、利用者の操作で、指定の位置に移る。skip は、今の位置から相対に移り、CM を飛んだら true を返す。
  const { seek, skip } = cm;
  // captionTrack は、出す字幕の種類 (#1682)。自動生成した字幕があるときだけ選べ、既定は放送の字幕にする。
  const [captionTrack, setCaptionTrack] = useState<CaptionTrack>('broadcast');
  const hasAutoCaptions = source.autoCaptions.length > 0;
  const autoCaptionsShown = captions && hasAutoCaptions && captionTrack === 'auto';
  // 実況は、チャンネルがあるときだけボタンを出す。実況を切っている間は、過去ログを読まない。
  const jikkyoSource = source.jikkyo;
  const jikkyoOn = jikkyoSource !== null && jikkyoEnabled;
  const jikkyo = useRecordedJikkyo(
    jikkyoOn ? jikkyoSource.url : null,
    currentPosition,
    visible,
    jikkyoOn ? jikkyoSource.live : null,
    commentSettings,
    jikkyoOn ? jikkyoSource.others : null,
    // 上映会のコメントは、設定でオプトインしているときだけ読む。
    jikkyoOn && commentSources.includes('NICOLOG') ? jikkyoSource.nicolog : null,
    jikkyoSource?.growing ?? false,
  );
  // listedCount は、再生位置までに流れた過去ログの数 (#489)。一覧は、この数が変わったときだけ作り直す。
  const listedCount = jikkyoListedCount(jikkyo.comments, position, jikkyo.offset);
  // partBands は、実況のコメントから見つけた番組の区切りのマーク (#1358)。
  const partBands = jikkyoPartBands(t, jikkyo.ownComments, jikkyo.offset);
  // 実況の過去ログの読み込み具合は、実況のボタンのツールチップで示す。
  const jikkyoLabel = recordedJikkyoLabel(t, jikkyo.state);
  // streamMeter は、右のカラムの「情報」に出す、届いた映像のビットレートと放送の形式をバッファに保持する (#828)。
  const streamMeter = useStreamMeter(playback.current);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(source.audios);
  useVirtualSurround(media, surroundAvailable);
  // captionHistory は、字幕の履歴 (#450) と、キャプチャに書き込む今の字幕の文 (#458)。
  const captionHistory = useRecordedCaptions(currentPosition);
  // changeQuality は、画質を変え、それまでの位置から続ける。
  const changeQuality = (next: string) => {
    restartAt(positionRef.current);
    setQuality(next);
  };
  // 映像だけの遅れを飛び直しで修正する (#1513)。デコードが追いつかなければ、画質が「自動」の間は段階を下げ、それまでの位置から続ける。
  // 段階を下げるのは、終わりが伸びている再生元 (録画中の追っかけ再生と、放送中の番組) でだけ。
  const adaptive = useAdaptiveQuality(
    media,
    {
      automatic: playback.automatic,
      lowerAvailable: playback.lowerAvailable,
      stepDownForDecode: () => {
        const next = playback.stepDownForDecode();
        if (next !== null) {
          restartAt(positionRef.current);
        }

        return next;
      },
      current: playback.current,
      networkLadder: playback.networkLadder,
      setNetworkStep: (name) => {
        restartAt(positionRef.current);
        playback.setNetworkStep(name);
      },
    },
    {
      stepDown: source.growing,
      pictureInPicture: shell.pip.container !== null,
      // 先に読むので、受け取りの速さで余裕を確認して上げる。終わりが伸びている間は、端に追いつくと伸びるのを待つので、終わりがない。
      network: {
        mode: 'rate',
        convert: route.kind === 'convert',
        topBitrate: source.topBitrate,
        ends: !source.growing,
      },
    },
  );
  // 音声は、経路ごとのメニューで選び、画質を変えても同じ音声を保つ。kyusu の音声を変えるときも、それまでの位置から続ける。
  const audio = usePlayerAudio(route, source.audios, () => restartAt(positionRef.current));
  // 再生速度は、video の要素を作り直したときにも当て直す。早回し (#1572) の間は、選んだ速さより速く再生する。
  const [rate, setRate] = useState(1);
  const fastForward = useFastForward(rate, { growing: source.growing, position, duration });
  useEffect(() => {
    if (media !== null) {
      media.defaultPlaybackRate = rate;
      media.playbackRate = fastForward.playbackRate;
    }
  }, [rate, fastForward.playbackRate, media, stream.url, hlsStream.loads]);
  // 早回しの間は、押している間ずっと、速さを映像の上に出しておく。
  const shownFlash = fastForward.active
    ? t('player.flash.fastForward', { rate: fastForward.playbackRate })
    : shell.flash;
  const capture = useCapture({
    crop: shell.crop,
    frameRef: shell.playerBoxRef,
    media: () => mediaRef.current,
    serviceName: source.serviceName,
    programName: source.program.name,
    captureTime: () => source.clockAt(currentPosition()) ?? Date.now(),
    recordingId: source.recordingId,
    ...(source.recordingId === null ? {} : { position: currentPosition }),
    caption: captionHistory.currentCaption,
  });
  const saveCapture = async () => {
    if ((await capture.capture('save')) === 'saved') {
      showFlash(t('player.flash.captured'));
    }
  };
  // commentOffsetCommands は、実況のコメントの位置 (#1357) の操作。変えたら、ずらした量を映像の上に短く出す。
  const commentOffsetCommands = useCommentOffsetCommands((ms) =>
    showFlash(t('player.flash.commentOffset', { seconds: formatCommentOffset(ms) })),
  );
  const shareSeconds = source.shareSeconds ?? ((seconds: number) => seconds);
  // commands は、この画面で今使える操作 (#809)。キー、OS のメディア操作、ほかの端末からの操作は、どれもここから呼ぶ。
  const commands: PlayerCommands = {
    ...shell.commands,
    ...shell.seekCommands({
      skip,
      seekTo: seek,
      currentPosition: () => shareSeconds(currentPosition()),
      retry,
    }),
    ...(jikkyoOn ? commentOffsetCommands : {}),
    toggleCaptions: () => setCaptions(!captions),
    ...(jikkyoSource === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    capture: () => void saveCapture(),
    selectAudio: audio.remote.select,
    stop: source.stop,
  };
  // seekLabel は、シークバーと滑らせている間の吹き出しに出す、経過時間と放送時刻。
  const seekLabel = (value: number) => {
    const at = source.clockAt(value);

    return at === null ? formatDuration(value) : `${formatDuration(value)} (${formatClock(at)})`;
  };
  const inputs = usePlayerInputs(shell, commands, {
    // タッチで映像の左右の 3 分の 1 をダブルタップすると、10 秒戻るか進む (#452)。CM を飛んだら、シークの結果は出さない。
    onDoubleTap: (side) => {
      if (!skip(side === 'left' ? -10 : 10)) {
        showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
      }
    },
    // 映像を長押ししたまま横に滑らせて、再生位置を動かす (#1601)。離したら、自分でシークしたものとして扱う。
    slideSeek: {
      enabled: source.seekLimit > 0,
      range: () => ({ position: currentPosition(), duration }),
      onScrub: (value) => setSeeking(value === null ? null : Math.min(value, source.seekLimit)),
      onCommit: (target) => {
        setSeeking(null);
        seek(target);
      },
      onCancel: () => setSeeking(null),
    },
  });
  const { sleep, scrub } = inputs;
  // player は、プレイヤーの今の状態 (#811)。タブの題名、OS のメディア操作、ほかの端末は、どれもこれを読む。
  const player = {
    kind: source.kind,
    target: source.target,
    serviceName: source.serviceName,
    program: source.program,
    artwork: source.artwork,
    paused: videoState.paused,
    position,
    duration,
    playbackRate: fastForward.playbackRate,
    volume: videoState.volume,
    muted: videoState.muted,
    audios: audio.remote.choices,
    audio: audio.remote.selected,
    dataBroadcast: false,
    keyGroups: [],
  } satisfies PlayerSnapshot;
  usePlayerPresence(
    player,
    commands,
    source.remote && source.kind === 'RECORDING' && media !== null
      ? { ...player, kind: 'RECORDING' }
      : null,
  );

  // 終わりまで再生したら、次へ進まずに止める。終わりが伸びている間は止めない。
  const pauseAtEnd = source.pauseAtEnd === true && !source.growing;
  useEffect(() => {
    if (pauseAtEnd && position >= duration && media !== null && !media.paused) {
      media.pause();
    }
  }, [pauseAtEnd, position, duration, media]);

  const shown = seeking ?? position;
  const shownClock = source.clockAt(shown);
  // skipButton は、いま再生している CM の中で出す、区間の終わりへ飛ばすボタン (#1300)。
  const skipButton = seeking === null && cm.button(position, controls.visible);
  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          // CM の帯を先に並べ、途切れた位置のマークを上に重ねる。
          bands={[...cm.bands, ...source.bands, ...partBands]}
          duration={duration}
          value={shown}
          previewUrl={source.previewUrl}
          label={seekLabel}
          onChange={(value) => setSeeking(Math.min(value, source.seekLimit))}
          onChangeEnd={(value) => {
            setSeeking(null);
            seek(value);
          }}
        />
      }
      start={
        <>
          <PlayButton video={videoState} commands={commands} />
          <SeekStepButtons commands={commands} onFastForward={fastForward.setHeld} />
          {/* 狭い画面 (スマートフォン) では、再生位置を出す幅を空けるため、ミュートのボタンを外す。本体の音量キーで操作できる。 */}
          <VolumeControl video={videoState} commands={commands} optional />
        </>
      }
      status={
        <PositionStatus
          position={formatDuration(shown)}
          rest={`${duration > 0 ? ` / ${formatDuration(duration)}` : ''}${
            shownClock === null ? '' : ` · ${formatClock(shownClock)}`
          }`}
        />
      }
      end={
        <>
          <CommandButton
            commands={commands}
            command="toggleCaptions"
            label={t('player.captions')}
            icon={Captions}
            active={captions}
          />
          <CommandButton
            commands={commands}
            command="toggleJikkyo"
            label={jikkyoLabel}
            icon={MessageSquareText}
            active={jikkyoEnabled}
          />
          <SettingsButton>
            <QualitySelect
              choices={choices}
              quality={quality}
              current={playback.current}
              currentCodec={playback.currentCodec}
              onChange={changeQuality}
            />
            {audio.menu}
            <CaptionTrackSelect
              available={hasAutoCaptions}
              value={captionTrack}
              onChange={setCaptionTrack}
            />
            <RateSelect value={rate} onChange={setRate} />
            <CropSelect value={shell.crop} onChange={shell.setCrop} />
            <NormalizeVolumeToggle />
            <AudioDelaySetting />
            {jikkyoOn && <CommentOffsetSetting />}
            <SleepTimerSelect timer={sleep} />
            {surroundAvailable && <VirtualSurroundToggle />}
          </SettingsButton>
          <PlayerToolButtons shell={shell} commands={commands} capture={capture} />
        </>
      }
    />
  );
  const overlay = (
    <>
      {autoCaptionsShown && (
        <PositionedAutoCaptionOverlay captions={source.autoCaptions} clock={currentPosition} />
      )}
      {jikkyo.overlay}
      <PlayerFeedback
        shell={shell}
        flash={shownFlash}
        notice={adaptive.notice}
        scrub={
          scrub === null
            ? null
            : { seconds: scrub, label: seekLabel(scrub), previewUrl: source.previewUrl }
        }
        loadingHidden={error !== null}
      />
      {skipButton}
      <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
    </>
  );
  const playerBox = (
    <PlayerBox shell={shell} inputs={inputs}>
      {route.kind === 'convert' && (
        <TsPlayer
          url={stream.url}
          handover={timeline.handover}
          mediaRef={mediaRef}
          onMedia={setMedia}
          controls={controlBar}
          // 自動生成した字幕を出している間は、放送の字幕を重ねない。
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={captionHistory.onCaptionText}
          onLoaded={timeline.onConvertLoaded}
          handoverTarget={stream.handoverTarget}
          onError={timeline.onError}
          onState={timeline.onConvertState}
          onAudio={audio.onAudio}
          onReading={(reading) => {
            streamMeter.onReading(reading);
            adaptive.network.onReading(reading);
          }}
          audio={audio.playerAudio}
          overlay={overlay}
          coverRef={cm.coverRef}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          // 読み直すたびに URL を変え、hls.js を作り直す。fragment は、サーバーへは送られない。
          url={
            source.hlsUrl === null
              ? null
              : `${hlsURL(source.hlsUrl, route.preset, audio.kyusuAudio)}#${hlsStream.loads}`
          }
          mediaRef={mediaRef}
          onMedia={setMedia}
          onSegment={(segment) => {
            streamMeter.onSegment(segment);
            adaptive.network.onSegment(segment);
          }}
          controls={controlBar}
          // 自動生成した字幕を出している間は、放送の字幕を重ねない。
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={captionHistory.onCaptionText}
          live={false}
          startPosition={hlsStream.start}
          overlay={overlay}
          onLoaded={timeline.onHlsLoaded}
          onError={timeline.onError}
          coverRef={cm.coverRef}
        />
      )}
    </PlayerBox>
  );
  // 実況の過去ログがあれば、右のカラムで字幕の履歴と切り替えて出す。
  const commentsShown = jikkyo.comments !== null && jikkyo.comments.length > 0;

  return (
    <>
      {children?.({ media, currentPosition, paused: videoState.paused })}
      <WatchLayout
        player={
          <PlayerFrame shell={shell} route={route}>
            {playerBox}
          </PlayerFrame>
        }
        info={
          <>
            <WatchHeading
              title={source.title}
              serviceName={source.serviceName}
              serviceLogoUrl={source.serviceLogoUrl}
              time={source.time}
              actions={
                <Button
                  variant="default"
                  size="compact-sm"
                  leftSection={<Share2 size={14} aria-hidden />}
                  onClick={commands.copyPositionUrl}
                >
                  {t('player.share')}
                </Button>
              }
            />
            {source.description !== null && (
              <WatchDescription
                text={source.description.text}
                extended={source.description.extended}
              />
            )}
            {source.info}
            {error !== null && (
              <Alert color="red" role="alert">
                <Group justify="space-between" gap="xs">
                  <Text size="sm">{error}</Text>
                  <Button size="compact-sm" variant="light" color="red" onClick={retry}>
                    {t('player.retry')}
                  </Button>
                </Group>
              </Alert>
            )}
          </>
        }
        side={
          <>
            {/* 実況の過去ログのうち再生位置までに流れたもの (#489) と、字幕の履歴と再生の情報 (#828) を、見出しで切り替える。過去ログがなければ、コメントの一覧を出さない。 */}
            <WatchPanel
              label={t('player.sidePanel')}
              tabs={[
                ...(commentsShown && jikkyo.comments !== null
                  ? [
                      {
                        value: 'jikkyo',
                        label: t('jikkyo.toggle'),
                        content: (
                          <RecordedJikkyoList
                            comments={jikkyo.comments}
                            count={listedCount}
                            offset={jikkyo.offset}
                            recordedFrom={jikkyoSource?.recordedFrom ?? null}
                            gaps={jikkyoSource?.gaps ?? []}
                            // 全画面の間は一覧が見えないので、コメントが届いても描き直さない。
                            frozen={fullscreen.active}
                            onMuteUser={muteUser}
                          />
                        ),
                      },
                    ]
                  : []),
                captionHistory.tab(
                  (entry) => {
                    const at = source.clockAt(entry.at);

                    return at === null ? formatDuration(entry.at) : formatClock(at);
                  },
                  (entry) => seek(entry.at),
                ),
                {
                  value: 'info',
                  label: t('player.streamInfo.tab'),
                  content: (
                    <StreamInfo
                      videoLag={adaptive.videoLag}
                      network={adaptive.network}
                      meter={streamMeter.meter}
                      media={media}
                      quality={playback.current}
                      kind={route.kind === 'convert' || route.kind === 'kyusu' ? route.kind : null}
                      audios={source.audios}
                      {...source.streamInfo}
                      height={watchCommentsHeight}
                    />
                  ),
                },
              ]}
            />
            {source.side}
          </>
        }
      />
    </>
  );
}
