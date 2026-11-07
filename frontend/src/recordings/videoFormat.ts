// SegmentVideo は、セグメントの映像から読んだ映像の形式 (#1310)。
export type SegmentVideo = {
  readonly codec: string;
  readonly width: number;
  readonly height: number;
  readonly interlaced: boolean;
  readonly frameRate?: number | null;
};

// codecNames は、映像の符号化方式の表示名。
const codecNames: Record<string, string> = { mpeg2: 'MPEG-2', h264: 'H.264', hevc: 'HEVC' };

// videoFormatLabel は、映像の形式を「MPEG-2 1440×1080i 29.97fps」のように書く。フレームレートが分からなければ省く。
export function videoFormatLabel(video: SegmentVideo): string {
  const scan = video.interlaced ? 'i' : 'p';
  const parts = [codecNames[video.codec] ?? video.codec, `${video.width}×${video.height}${scan}`];
  if (video.frameRate != null && video.frameRate > 0) {
    parts.push(`${video.frameRate.toFixed(2)}fps`);
  }

  return parts.join(' ');
}

// distinctVideoLabels は、セグメントの映像の形式を、重ねずに、始まりの順に並べる。読めなかったセグメントは飛ばす。
export function distinctVideoLabels(
  videos: readonly (SegmentVideo | null | undefined)[],
): string[] {
  const labels: string[] = [];
  for (const video of videos) {
    if (video == null) {
      continue;
    }
    const label = videoFormatLabel(video);
    if (!labels.includes(label)) {
      labels.push(label);
    }
  }

  return labels;
}
