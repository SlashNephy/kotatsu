import { ptsTicksPerSecond } from './ptsTimeline';

// ブラウザ内でデコードする経路 (#1516) の、音を Web Audio の時間の上に並べる部分。
// 届いた順に、隙間なく予約する。鳴らし始めは「今 + 鳴らし始めの余裕」にする (今すぐにすると先頭が欠けるため)。
// 予約した区間から、Web Audio の時刻と PTS を対応させ、いま鳴っている PTS を出す (これが再生の時計になる)。
// 前より古い時刻の音は捨てる。番を過ぎた音 (途切れた後の音) は、その音から今の後ろに鳴らし直す (遅れたまま並べると、そこから先が全部ずれるため)。

export type AudioTimelineParams = {
  // lead は、鳴らし始めを「今」から遅らせる量 (秒)。予約から実際に鳴るまでの処理の時間より長くする。
  lead: number;
};

export const defaultAudioTimelineParams: AudioTimelineParams = {
  lead: 0.1,
};

// Scheduled は、予約した音。when は Web Audio の時刻 (秒)、offset は、音の頭から切り落とす秒数。
export type Scheduled = { when: number; offset: number };

// AudioTimeline は、PTS と Web Audio の時刻の対応を持つ。
export class AudioTimeline {
  // base は、対応の起点 (PTS と Web Audio の時刻)。鳴らし始めと、途切れて鳴らし直したときに決め直す。
  private base: { pts: number; time: number } | null = null;
  // end は、予約した音の終わりの PTS。
  private end: number | null = null;
  // restarts は、途切れて鳴らし直した回数。
  restarts = 0;

  constructor(private readonly params: AudioTimelineParams = defaultAudioTimelineParams) {}

  // schedule は、pts から duration (PTS の単位) の音を、Web Audio の時刻 now に予約する時刻を返す。
  // 捨てる音なら null を返す。
  schedule(pts: number, duration: number, now: number): Scheduled | null {
    if (this.end !== null && pts + duration <= this.end) {
      // 前より古い音。
      return null;
    }
    // 隙間があっても、時刻の対応は保ち、隙間の分だけ空けて鳴らす (音と映像の時刻をずらさない)。
    if (this.base === null) {
      this.start(pts, now + this.params.lead);
    }
    let when = this.timeOf(pts);
    let offset = 0;
    if (this.end !== null && pts < this.end) {
      // 前の音と重なる分は、頭を切り落とす。
      offset = (this.end - pts) / ptsTicksPerSecond;
      when = this.timeOf(this.end);
    }
    if (when < now) {
      // 番を過ぎた (途切れた)。今から鳴らし直す。
      this.start(pts + offset * ptsTicksPerSecond, now + this.params.lead);
      this.restarts += 1;
      when = this.timeOf(pts + offset * ptsTicksPerSecond);
    }
    this.end = pts + duration;

    return { when, offset };
  }

  // ptsAt は、Web Audio の時刻 time に鳴っている PTS を返す。まだ鳴らし始めていなければ null。
  ptsAt(time: number): number | null {
    if (this.base === null) {
      return null;
    }

    return this.base.pts + (time - this.base.time) * ptsTicksPerSecond;
  }

  // bufferedUntil は、予約した音の終わりの PTS。
  get bufferedUntil(): number | null {
    return this.end;
  }

  // reset は、対応を忘れる。止めたときと、選局したときに呼ぶ。
  reset(): void {
    this.base = null;
    this.end = null;
  }

  private start(pts: number, time: number): void {
    this.base = { pts, time };
  }

  private timeOf(pts: number): number {
    return this.base!.time + (pts - this.base!.pts) / ptsTicksPerSecond;
  }
}
