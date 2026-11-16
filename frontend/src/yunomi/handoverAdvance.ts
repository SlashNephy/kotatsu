import { isBuffered } from './position';

// HandoverMedia は、裏で読み込んだ映像のうち、切り替える前に進めるのに使うところ。
export type HandoverMedia = {
  currentTime: number;
  readonly buffered: TimeRanges;
  readonly seeking: boolean;
};

// HandoverClock は、待ち方。テストで時計を差し替える。
export type HandoverClock = {
  now: () => number;
  sleep: (ms: number) => Promise<void>;
};

const realClock: HandoverClock = {
  now: () => Date.now(),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
};

// handoverAdvanceMs は、裏で読み込んだ映像を進める先が、読み込んだ範囲に入るのを待つ時間の上限。
// 過ぎたら進めずに切り替え、その分だけ巻き戻る。待つ間も今の映像は流れているので、長く待っても見え方は変わらない。
const handoverAdvanceMs = 5_000;

// handoverPollMs は、進める先が読み込んだ範囲に入ったかを見直す間隔。
const handoverPollMs = 200;

// handoverSeekMs は、進めた後、映像がその位置のコマを出すのを待つ時間の上限。
const handoverSeekMs = 1_000;

// advanceBeforeHandover は、裏で読み込んだ映像を、切り替える前に、今の映像が進んだ分だけ進める (#1728)。
// target は、進める先の video の時刻を返す。null なら進めない (位置が分からないとき)。
// 今の映像が流れ続けるので、進める先は見直すたびに求め直す。cancelled が true になったら、すぐ終える。
// 戻り値は、進めたかどうか。
export async function advanceBeforeHandover(
  video: HandoverMedia,
  target: () => number | null,
  cancelled: () => boolean,
  clock: HandoverClock = realClock,
): Promise<boolean> {
  const deadline = clock.now() + handoverAdvanceMs;
  for (;;) {
    if (cancelled()) {
      return false;
    }
    const to = target();
    if (to === null || to <= video.currentTime) {
      return false;
    }
    if (isBuffered(video.buffered, to)) {
      video.currentTime = to;
      const seekDeadline = clock.now() + handoverSeekMs;
      while (video.seeking && clock.now() < seekDeadline && !cancelled()) {
        await clock.sleep(handoverPollMs / 4);
      }

      return true;
    }
    if (clock.now() >= deadline) {
      return false;
    }
    await clock.sleep(handoverPollMs);
  }
}
