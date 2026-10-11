// 再生中の場面を画像にする (設計書 7.5)。

// captureSize は、画像の大きさ。放送の映像の表示の比率 (16:9) にそろえる。
const captureSize = { width: 1920, height: 1080 };

// fitContain は、width x height のものを、枠 (frameWidth x frameHeight) に収めたときの位置と大きさを返す。
export function fitContain(
  width: number,
  height: number,
  frameWidth: number,
  frameHeight: number,
): { x: number; y: number; width: number; height: number } {
  if (width <= 0 || height <= 0) {
    return { x: 0, y: 0, width: 0, height: 0 };
  }
  const scale = Math.min(frameWidth / width, frameHeight / height);

  return {
    x: (frameWidth - width * scale) / 2,
    y: (frameHeight - height * scale) / 2,
    width: width * scale,
    height: height * scale,
  };
}

// captureFrame は、video の今の場面を PNG にする。overlays を渡せば、字幕とコメントの canvas を、枠いっぱいに重ねる。
// 字幕とコメントの canvas は、どちらも映像の枠 (16:9) に合わせて描いている。
export async function captureFrame(
  video: HTMLVideoElement,
  overlays: HTMLCanvasElement[],
): Promise<Blob> {
  const canvas = document.createElement('canvas');
  canvas.width = captureSize.width;
  canvas.height = captureSize.height;
  const context = canvas.getContext('2d');
  if (context === null) {
    throw new Error('2D canvas is not available');
  }
  context.fillStyle = 'black';
  context.fillRect(0, 0, canvas.width, canvas.height);
  // 画面と同じく、枠に収めて描く。videoWidth と videoHeight は、画素の比率 (1440x1080 の放送の 4:3 の画素など) を当てた後の大きさ。
  const shown = fitContain(video.videoWidth, video.videoHeight, canvas.width, canvas.height);
  context.drawImage(video, shown.x, shown.y, shown.width, shown.height);
  for (const overlay of overlays) {
    context.drawImage(overlay, 0, 0, canvas.width, canvas.height);
  }

  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob === null ? reject(new Error('failed to encode the capture')) : resolve(blob)),
      'image/png',
    );
  });
}

// overlayCanvases は、映像の枠の中にある、字幕とコメントの canvas のうち、出しているものを返す。
// どちらも data-overlay の印を付けている。yadif が映像の上に重ねる canvas は、映像そのものなので、印がない。
export function overlayCanvases(frame: HTMLElement): HTMLCanvasElement[] {
  return [...frame.querySelectorAll<HTMLCanvasElement>('canvas[data-overlay]')].filter(
    (canvas) => canvas.style.visibility !== 'hidden',
  );
}

// saveBlob は、画像をファイルとして保存させる。
export function saveBlob(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  // 文書に置いてから押さないと、保存しないブラウザがある。
  document.body.append(link);
  link.click();
  link.remove();
  // 保存を始めた後で、URL を手放す。
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

// copyBlob は、画像をクリップボードに入れる。
export async function copyBlob(blob: Blob): Promise<void> {
  await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
}
