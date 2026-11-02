// VideoStats は、右の列の「情報」(#828) に出す、映像の再生の様子 (#499)。
export type VideoStats = {
  width: number;
  height: number;
  // dropped は捨てたフレームの数、total は受け取ったフレームの数 (捨てたものを含む)。数えられないブラウザでは null。
  dropped: number | null;
  total: number | null;
  // buffered は、今の位置から先に読み込んである秒数。
  buffered: number;
  rate: number;
};

// readVideoStats は、video から、再生の様子を読む。
export function readVideoStats(
  video: Pick<
    HTMLVideoElement,
    'videoWidth' | 'videoHeight' | 'currentTime' | 'buffered' | 'playbackRate'
  > &
    Partial<Pick<HTMLVideoElement, 'getVideoPlaybackQuality'>>,
): VideoStats {
  let buffered = 0;
  for (let i = 0; i < video.buffered.length; i++) {
    if (
      video.buffered.start(i) <= video.currentTime &&
      video.currentTime <= video.buffered.end(i)
    ) {
      buffered = video.buffered.end(i) - video.currentTime;
    }
  }
  const quality = video.getVideoPlaybackQuality?.();

  return {
    width: video.videoWidth,
    height: video.videoHeight,
    dropped: quality?.droppedVideoFrames ?? null,
    total: quality?.totalVideoFrames ?? null,
    buffered,
    rate: video.playbackRate,
  };
}
