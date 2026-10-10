// 録画の再生位置の計算。シークは、新しい開始位置で読み直すので、video の時刻だけでは録画の中の位置が分からない。

// playbackPosition は、録画の始まりからの再生位置 (秒) を返す。
// offset は今の読み込みを始めた位置、base は読み込み直後の video の時刻、current は今の video の時刻。
export function playbackPosition(offset: number, base: number, current: number): number {
  return offset + Math.max(current - base, 0);
}

// formatDuration は、秒を H:MM:SS か M:SS で表す。
export function formatDuration(seconds: number): string {
  const total = Math.max(Math.floor(seconds), 0);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, '0');

  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}
