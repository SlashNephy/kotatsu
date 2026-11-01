import { useEffect, useRef } from 'react';
import { graphql, useMutation } from 'react-relay';

import type { usePlaybackReporterMutation } from './__generated__/usePlaybackReporterMutation.graphql';

// sampleInterval は、再生した区間を確かめる間隔 (ミリ秒)。
const sampleInterval = 1_000;
// reportInterval は、再生した区間をサーバーに送る間隔 (ミリ秒、付録 A)。
export const reportInterval = 30_000;
// slackMs は、1 回の確かめで、放送の時刻が壁の時計より進んでもよい余裕 (ミリ秒)。タイマーの遅れを見込む。
// これより大きく進んだら、シークしたものとして、区間を切る。
const slackMs = 1_500;
// minIntervalMs は、送る区間の長さの下限 (ミリ秒)。
const minIntervalMs = 1_000;
// maxPending は、送れずに溜めておく区間の数の上限。サーバーが受け付ける数 (100) より少なくする。
const maxPending = 50;

export type PlaybackTarget = { serviceId: string } | { recordingId: string };

export type PlayedInterval = { from: number; to: number };

// PlaybackTracker は、プレイヤーが実際に再生した、放送の時刻の区間を集める (#927)。
// 再生位置ではなく、確かめるたびに、映像の時刻が進み、放送の時刻が壁の時計と同じくらい進んだ分だけを、再生したものとして足す。
// 一時停止している間は進まず、シークで飛んだ分は区間を切るので、最後までシークしただけでは数えない。
export class PlaybackTracker {
  private closed: PlayedInterval[] = [];
  private current: PlayedInterval | null = null;
  private last: { wall: number; at: number; media: number } | null = null;

  // sample は、壁の時計 wall の時点で、放送の時刻 at の場面を出していて、映像の時刻が media (秒) であることを記録する。
  // playing は、再生中かどうか。rate は、再生の速さ。
  sample(wall: number, at: number | null, media: number, playing: boolean, rate: number): void {
    const last = this.last;
    this.last = at === null ? null : { wall, at, media };
    if (at === null || last === null) {
      this.close();

      return;
    }
    const advanced = at - last.at;
    const allowed = (wall - last.wall) * Math.max(rate, 1) + slackMs;
    // 映像が進んでいない (止まっている、読み込みを待っている) か、放送の時刻が戻ったか飛んだら、区間を切る。
    if (!playing || media <= last.media || advanced <= 0 || advanced > allowed) {
      this.close();

      return;
    }
    if (this.current === null) {
      this.current = { from: last.at, to: at };
    } else {
      this.current.to = at;
    }
  }

  // pending は、まだ送り届けていない区間と、続いている区間を返す。サーバーは区間を重ねずにまとめるので、送り直してもよい。
  // 1 秒に満たない区間は送らない。
  pending(): { intervals: PlayedInterval[]; closed: number } {
    const intervals = [...this.closed];
    if (this.current !== null && this.current.to - this.current.from >= minIntervalMs) {
      intervals.push({ ...this.current });
    }

    return { intervals, closed: this.closed.length };
  }

  // acknowledge は、送り届けた区間のうち、閉じた先頭の closed 個を捨てる。続いている区間は、伸び続けるので残す。
  acknowledge(closed: number): void {
    this.closed = this.closed.slice(Math.min(closed, this.closed.length));
  }

  private close(): void {
    if (this.current !== null && this.current.to - this.current.from >= minIntervalMs) {
      this.closed.push(this.current);
      // 送れない間に溜まりすぎたら、古いものから捨てる。
      if (this.closed.length > maxPending) {
        this.closed = this.closed.slice(-maxPending);
      }
    }
    this.current = null;
  }
}

const mutation = graphql`
  mutation usePlaybackReporterMutation(
    $serviceId: ID
    $recordingId: ID
    $intervals: [PlaybackIntervalInput!]!
  ) {
    reportAnimePlayback(serviceId: $serviceId, recordingId: $recordingId, intervals: $intervals)
  }
`;

function variablesOf(target: PlaybackTarget, intervals: PlayedInterval[]) {
  return {
    serviceId: 'serviceId' in target ? target.serviceId : null,
    recordingId: 'recordingId' in target ? target.recordingId : null,
    intervals: intervals.map((interval) => ({
      from: new Date(interval.from).toISOString(),
      to: new Date(interval.to).toISOString(),
    })),
  };
}

// usePlaybackReporter は、ライブ視聴と録画の再生で、実際に再生した区間を、しばらくごとと、画面を閉じるときにサーバーへ送る (#927)。
// サーバーは、番組の長さの一定の割合を見たら、Annict と AniList に記録する。broadcastAt は、いま出している場面の放送の時刻
// (Unix ミリ秒) を返す。分からなければ null を返す。記録の対象でないとサーバーが答えたら、この画面では送るのをやめる。
export function usePlaybackReporter(
  target: PlaybackTarget | null,
  video: HTMLVideoElement | null,
  broadcastAt: () => number | null,
): void {
  const [commit] = useMutation<usePlaybackReporterMutation>(mutation);
  const broadcastAtRef = useRef(broadcastAt);
  broadcastAtRef.current = broadcastAt;
  const targetKey = target === null ? null : JSON.stringify(target);

  useEffect(() => {
    if (targetKey === null || video === null) {
      return;
    }
    const reportTarget = JSON.parse(targetKey) as PlaybackTarget;
    const tracker = new PlaybackTracker();
    // stopped は、記録の対象でないとサーバーが答えたかどうか。
    let stopped = false;
    let sending = false;
    const sample = () =>
      tracker.sample(
        Date.now(),
        broadcastAtRef.current(),
        video.currentTime,
        !video.paused && !video.seeking,
        video.playbackRate,
      );
    const report = () => {
      const { intervals, closed } = tracker.pending();
      if (stopped || sending || intervals.length === 0) {
        return;
      }
      sending = true;
      commit({
        variables: variablesOf(reportTarget, intervals),
        onCompleted: (response) => {
          sending = false;
          tracker.acknowledge(closed);
          stopped = !response.reportAnimePlayback;
        },
        // 送れなくても再生は続ける。区間は残しておき、次に送る。
        onError: () => {
          sending = false;
        },
      });
    };
    const sampler = setInterval(sample, sampleInterval);
    const reporter = setInterval(report, reportInterval);
    // タブを閉じるときは、ページが消えても届くよう、keepalive の要求で送る。
    const onPageHide = () => {
      sample();
      const { intervals } = tracker.pending();
      if (stopped || intervals.length === 0) {
        return;
      }
      void fetch('/graphql', {
        method: 'POST',
        keepalive: true,
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          query:
            'mutation ($serviceId: ID, $recordingId: ID, $intervals: [PlaybackIntervalInput!]!) { reportAnimePlayback(serviceId: $serviceId, recordingId: $recordingId, intervals: $intervals) }',
          variables: variablesOf(reportTarget, intervals),
        }),
      }).catch(() => {});
    };
    window.addEventListener('pagehide', onPageHide);

    return () => {
      window.removeEventListener('pagehide', onPageHide);
      clearInterval(sampler);
      clearInterval(reporter);
      sample();
      report();
    };
  }, [targetKey, video, commit]);
}
