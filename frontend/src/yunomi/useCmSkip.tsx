import { type ReactNode, type RefObject, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useCmAutoSkip } from './cmAutoSkip';
import { CmAutoSkipper } from './cmAutoSkipper';
import {
  cmButtonSectionAt,
  cmMergeGap,
  cmSectionAt,
  mergeCmSections,
  type CmSection,
} from './cmSkip';
import { formatDuration } from './position';
import type { PlayerCover } from './playerCover';
import type { PlayerMedia } from './playerMedia';
import { SkipButton } from './PlayerControls';
import { useVideoFrames } from './useVideoFrames';

// 録画とライブラリのファイルの再生で、CM とみなした区間 (#479、設計書 5.8) を扱う機能の部品 (#2244)。シークバーの帯、
// CM の中で出す飛ばすボタン (#1300)、自動で飛ばす設定 (#1282、#1512) をまとめる。

// CmOpening は、開いた位置。skipped は、開いた位置が CM の中で、最初から CM の終わりを求めたかどうか。resumed は、前回の続きから開いたかどうか。
export type CmOpening = { at: number; skipped: boolean; resumed: boolean };

// CmSkipOptions は、再生元から受け取る、位置とシークの扱い。
export type CmSkipOptions = {
  // sections は、CM とみなした区間。再生の位置 (秒) で渡す。
  sections: readonly CmSection[];
  media: PlayerMedia | null;
  mediaRef: RefObject<PlayerMedia | null>;
  // framePosition は、映ったコマの時刻に当たる再生位置。読み直していて、まだ位置を信じられないときは null。
  framePosition: (t: number) => number | null;
  // jumpTo は、指定の位置に移り、実際に移った先を返す。自動で飛ぶときにも使うので、利用者のシークとしては数えない。
  jumpTo: (target: number) => number;
  currentPosition: () => number;
  // scrubbing は、シークバーのつまみや映像を動かしている最中かどうか。動かしている間は、自動では飛ばない。
  scrubbing: boolean;
  showFlash: (text: string) => void;
  opening: CmOpening;
  // initialSeek は、URL で位置を渡されて開いたときの、読み始めた位置。渡されたら、その場面を見たいものとして扱う。
  initialSeek: number | null;
};

// CmSkip は、useCmSkip が返す、CM の扱いの口。
export type CmSkip = {
  // coverRef は、映像の上に重ねる蓋 (#1512)。プレイヤーに渡すと、プレイヤーが出し入れの口を入れる。
  coverRef: RefObject<PlayerCover | null>;
  // seek は、利用者の操作で、指定の位置に移る。移った先が CM の中なら、その CM は自動で飛ばさない。
  seek: (target: number) => void;
  // skip は、今の位置から seconds 秒だけ進むか戻る。CM を飛んだら true を返す。
  skip: (seconds: number) => boolean;
  // bands は、シークバーに出す CM の帯。
  bands: { from: number; to: number; label: string; kind: 'cm' }[];
  // button は、いま再生している CM の中で出す、区間の終わりへ飛ばすボタン。CM の外では null。
  button: (position: number, raised: boolean) => ReactNode;
};

// useCmSkip は、CM の帯と、飛ばすボタンと、自動で飛ばす判定を用意する。
export function useCmSkip({
  sections,
  media,
  mediaRef,
  framePosition,
  jumpTo,
  currentPosition,
  scrubbing,
  showFlash,
  opening,
  initialSeek,
}: CmSkipOptions): CmSkip {
  const { t } = useTranslation();
  // spans は、自動で飛ばすときに使う、続いた区間をまとめた CM の区間 (#1512)。
  const spans = mergeCmSections(sections, cmMergeGap);
  // lastSeek は、利用者が最後にシークした位置 (#1282)。そこが CM の区間の中なら、自動では飛ばさない。
  // URL で位置を渡されて開いたときも、その場面を見たいものとして扱う。自動で飛んだ位置は入れない (#1512)。
  const lastSeek = useRef<number | null>(initialSeek);
  const [cmAutoSkip] = useCmAutoSkip();
  const coverRef = useRef<PlayerCover | null>(null);
  // live は、自動で飛ばす判定が読む、描画ごとに変わる値。判定は描画の外 (コマごと) で行う。
  const live = useRef({ cmAutoSkip, spans, scrubbing, jumpTo, showFlash, t });
  live.current = { cmAutoSkip, spans, scrubbing, jumpTo, showFlash, t };
  // skipper は、CM を自動で飛ばすときに、CM のコマを 1 コマも画面に出さないための判定と蓋 (#1512)。
  const [skipper] = useState(
    () =>
      new CmAutoSkipper({
        enabled: () => live.current.cmAutoSkip,
        scrubbing: () => live.current.scrubbing,
        paused: () => mediaRef.current?.paused ?? true,
        lastSeek: () => lastSeek.current,
        sections: () => live.current.spans,
        cover: (kind) => coverRef.current?.show(kind),
        uncover: () => coverRef.current?.hide(),
        jump: (target) => live.current.jumpTo(target),
        notify: () => live.current.showFlash(live.current.t('player.flash.cmSkipped')),
        setTimer: (callback, ms) => {
          const timer = setTimeout(callback, ms);

          return () => clearTimeout(timer);
        },
      }),
  );
  useEffect(() => () => skipper.dispose(), [skipper]);
  // 判定は、映ったコマごとに、そのコマの時刻で行う。timeupdate は間隔が粗く、currentTime は代入した時点で飛び先を返すため。
  // 読み直していて、求めた位置以上のコマがまだ映っていない間は、位置を渡さない。
  useVideoFrames(media, ({ mediaTime, presented }) => {
    if (media === null) {
      return;
    }
    skipper.frame({
      position: framePosition(mediaTime ?? media.currentTime),
      seeking: media.seeking,
      paused: media.paused,
      presented,
    });
  });
  // 開いた位置が CM の中で、最初から CM の終わりを求めたときは、黒い蓋で覆い、CM の終わり以上のコマが映るまで待つ。
  // 配信は手前の I フレームから流れるので、CM の末尾が映りうる。続きから開いたときは、続きから始めたことだけを伝え、
  // CM を飛ばした通知は出さない。先頭から開いたときは、ほかの自動のスキップと同じく通知する。
  const openingHeld = useRef(false);
  useEffect(() => {
    if (!opening.skipped || openingHeld.current || media === null) {
      return;
    }
    openingHeld.current = true;
    skipper.holdUntil(opening.at);
    if (!opening.resumed) {
      showFlash(t('player.flash.cmSkipped'));
    }
  }, [media]);
  // 自動で飛ばす設定を入れた瞬間に CM の中にいれば、その場で飛ぶ。「CM が始まったので入れた」が多いので、
  // 自分でシークして CM の区間の中にいたときも、設定を入れたことを、飛ばしたい意思として扱う (#1512)。
  const previousCmAutoSkip = useRef(cmAutoSkip);
  useEffect(() => {
    if (cmAutoSkip && !previousCmAutoSkip.current) {
      lastSeek.current = null;
    }
    previousCmAutoSkip.current = cmAutoSkip;
  }, [cmAutoSkip]);

  const seek = (target: number) => {
    lastSeek.current = jumpTo(target);
  };
  // 進む向きの相対の移動 (キー、ダブルタップ、15 秒のボタン。#1572) は、行き先を選んだシークではないので、CM を見たい意思と
  // みなさない (#1512)。CM を自動で飛ばす設定で、行き先が CM の中なら、CM の終わりへ飛ぶ。自分でシークして見ていた CM の中で
  // 動いたときは、そのまま移る。戻る向きは、CM の終わりへ押し戻すと CM より前へ戻れなくなるので、これまでどおりシークとして扱う。
  const skip = (seconds: number): boolean => {
    const target = currentPosition() + seconds;
    const watching = lastSeek.current;
    const span = cmSectionAt(spans, target);
    const insideWatched =
      span !== null && watching !== null && span.from <= watching && watching < span.to;
    if (seconds > 0 && !insideWatched && skipper.skipFrom(target)) {
      return true;
    }
    seek(target);

    return false;
  };
  const bands = sections.map(({ from, to }) => ({
    from,
    to,
    label: t('player.cmSection', { duration: formatDuration(Math.max(to - from, 0)) }),
    kind: 'cm' as const,
  }));
  const button = (position: number, raised: boolean) => {
    const current = cmButtonSectionAt(bands, position);
    if (current === null) {
      return null;
    }

    return (
      <SkipButton
        raised={raised}
        // CM から出るための操作なので、CM を見たい意思 (lastSeek) を消す (#1512)。自動で飛ばす設定なら、続いた CM の終わりまで、
        // 蓋を出して 1 回で飛ぶ。区間の終わりへ移るだけだと、残りの CM の先頭が映ってから飛ぶ。
        onClick={() => {
          lastSeek.current = null;
          if (!skipper.skipFrom(currentPosition())) {
            jumpTo(current.to);
          }
        }}
      >
        {t('player.skipCm')}
      </SkipButton>
    );
  };

  return { coverRef, seek, skip, bands, button };
}
