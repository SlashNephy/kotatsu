// 再生中の場面を画像にする (設計書 7.5)。

import { toHalfWidth } from '../futon/halfWidth';
import { splitProgramMarks } from '../futon/programMarks';
import { cropSource, isCropped, noCrop, type Crop } from './crop';
import { withPNGText, type PNGText } from './pngText';

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
// crop で切り抜いているときは、画面と同じく、映像と字幕だけを切り抜いた範囲で写し、コメントは切り抜かずに重ねる (#496)。
export async function captureFrame(
  video: HTMLVideoElement,
  overlays: HTMLCanvasElement[],
  crop: Crop = noCrop,
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
  const captions = overlays.filter((overlay) => overlay.dataset['overlay'] === 'captions');
  const others = overlays.filter((overlay) => overlay.dataset['overlay'] !== 'captions');
  if (!isCropped(crop)) {
    context.drawImage(video, shown.x, shown.y, shown.width, shown.height);
    for (const overlay of captions) {
      context.drawImage(overlay, 0, 0, canvas.width, canvas.height);
    }
  } else {
    // 映像と字幕を枠いっぱいに描いてから、切り抜いた範囲を拡大して写す。
    const full = document.createElement('canvas');
    full.width = canvas.width;
    full.height = canvas.height;
    const fullContext = full.getContext('2d');
    if (fullContext === null) {
      throw new Error('2D canvas is not available');
    }
    fullContext.drawImage(video, shown.x, shown.y, shown.width, shown.height);
    for (const overlay of captions) {
      fullContext.drawImage(overlay, 0, 0, full.width, full.height);
    }
    const source = cropSource(crop, full.width, full.height);
    context.drawImage(
      full,
      source.x,
      source.y,
      source.width,
      source.height,
      0,
      0,
      canvas.width,
      canvas.height,
    );
  }
  for (const overlay of others) {
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

// unsafeFileNameChars は、Windows や macOS でファイル名に使えない文字と、制御文字。
const unsafeFileNameChars = /[\\/:*?"<>|\p{Cc}]/gu;

const pad2 = (value: number) => String(value).padStart(2, '0');

// captureFileNameItems は、キャプチャのファイル名のテンプレートの書き方 (#500、#1707)。
// 録画のファイル名のテンプレート (設計書 4.7) と同じ、Go のテンプレートの書き方のうち、キャプチャで分かる項目だけを受け付ける。
// .StartAt は撮った場面の時刻で、端末のローカル時刻で書く。番組名は、囲み文字を外す。
export const captureFileNameItems = [
  '{{.Name}}',
  '{{half .Name}}',
  '{{.ServiceName}}',
  '{{half .ServiceName}}',
  '{{.StartAt.Format "2006"}}',
  '{{.StartAt.Format "01"}}',
  '{{.StartAt.Format "02"}}',
  '{{weekdayJa .StartAt}}',
  '{{.StartAt.Format "15"}}',
  '{{.StartAt.Format "04"}}',
  '{{.StartAt.Format "05"}}',
] as const;

// captureFileNameMacros は、前の版の書き方 (#500)。TVTest と同じ書き方 (#814) なので、
// TVTest から移ってくる人のために、Go のテンプレートの書き方と並べて受け付け続ける。
export const captureFileNameMacros = [
  '%date%',
  '%time%',
  '%year%',
  '%month%',
  '%day%',
  '%hour%',
  '%minute%',
  '%second%',
  '%service%',
  '%title%',
  '%service-name%',
  '%event-name%',
] as const;

const weekdaysJa = ['日', '月', '火', '水', '木', '金', '土'];
const monthNames = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];
const weekdayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

// goLayoutTokens は、Go の time.Format の書き方のうち、受け付けるもの。長いものから順に当てる。
// タイムゾーンと秒の端数は、キャプチャのファイル名には要らないので、受け付けずにそのまま残す。
const goLayoutTokens = /January|Monday|2006|Jan|Mon|_2|01|02|03|04|05|06|15|PM|pm|1|2|3|4|5/g;

// formatGoLayout は、date を、Go の time.Format の layout で書く。
export function formatGoLayout(date: Date, layout: string): string {
  const hour12 = date.getHours() % 12 === 0 ? 12 : date.getHours() % 12;
  const values: Record<string, string> = {
    January: monthNames[date.getMonth()] ?? '',
    Monday: weekdayNames[date.getDay()] ?? '',
    '2006': String(date.getFullYear()),
    Jan: (monthNames[date.getMonth()] ?? '').slice(0, 3),
    Mon: (weekdayNames[date.getDay()] ?? '').slice(0, 3),
    _2: String(date.getDate()).padStart(2, ' '),
    '01': pad2(date.getMonth() + 1),
    '02': pad2(date.getDate()),
    '03': pad2(hour12),
    '04': pad2(date.getMinutes()),
    '05': pad2(date.getSeconds()),
    '06': pad2(date.getFullYear() % 100),
    '15': pad2(date.getHours()),
    PM: date.getHours() < 12 ? 'AM' : 'PM',
    pm: date.getHours() < 12 ? 'am' : 'pm',
    '1': String(date.getMonth() + 1),
    '2': String(date.getDate()),
    '3': String(hour12),
    '4': String(date.getMinutes()),
    '5': String(date.getSeconds()),
  };

  return layout.replace(goLayoutTokens, (token) => values[token] ?? token);
}

// goAction は、{{ と }} の中身のうち、受け付ける形。
// .Name、.ServiceName、half .Name、half .ServiceName、weekdayJa .StartAt、.StartAt.Format "layout" (` で囲んでもよい)。
const goAction =
  /^(?:(?<field>\.Name|\.ServiceName)|half\s+(?<halfField>\.Name|\.ServiceName)|weekdayJa\s+\.StartAt|\.StartAt\.Format\s+(?:"(?<layout>[^"\\]*)"|`(?<rawLayout>[^`]*)`))$/;

// expandGoAction は、{{ と }} の中身 body を当てはめる。受け付けない形なら null を返す。
function expandGoAction(body: string, date: Date, serviceName: string, program: string) {
  const match = goAction.exec(body);
  if (match === null) {
    return null;
  }
  const { field, halfField, layout, rawLayout } = match.groups ?? {};
  const fields: Record<string, string> = { '.Name': program, '.ServiceName': serviceName };
  if (field !== undefined) {
    return fields[field] ?? null;
  }
  if (halfField !== undefined) {
    return toHalfWidth(fields[halfField] ?? '');
  }
  if (layout !== undefined || rawLayout !== undefined) {
    return formatGoLayout(date, layout ?? rawLayout ?? '');
  }

  return weekdaysJa[date.getDay()] ?? '';
}

// captureFileName は、キャプチャのファイル名を返す (設計書 7.5)。
// template を渡せば、その書き方 (captureFileNameItems と captureFileNameMacros) で作る (#500、#1707)。空か null なら、
// time (Unix ミリ秒) を端末のローカル時刻で書き、サービス名と、囲み文字を外した番組名を続ける。
export function captureFileName(
  time: number,
  serviceName: string,
  programName: string | null,
  template: string | null = null,
): string {
  const date = new Date(time);
  const program = programName === null ? '' : splitProgramMarks(programName).name;
  if (template !== null && template.trim() !== '') {
    const values: Record<(typeof captureFileNameMacros)[number], string> = {
      '%date%': `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`,
      '%time%': `${pad2(date.getHours())}.${pad2(date.getMinutes())}.${pad2(date.getSeconds())}`,
      '%year%': String(date.getFullYear()),
      '%month%': pad2(date.getMonth() + 1),
      '%day%': pad2(date.getDate()),
      '%hour%': pad2(date.getHours()),
      '%minute%': pad2(date.getMinutes()),
      '%second%': pad2(date.getSeconds()),
      '%service%': serviceName,
      '%title%': program,
      '%service-name%': serviceName,
      '%event-name%': program,
    };
    // 両方の書き方を 1 回で当てはめ、当てはめた番組名の中の書き方を、もう一度は当てはめない。
    // 知らない書き方はそのまま残す。前後の空白を詰める {{- と -}} も、受け付けずに残す。
    const name = template
      .trim()
      .replace(/%[a-z-]+%|\{\{\s*(.*?)\s*\}\}/g, (written, body: string | undefined) => {
        if (body === undefined) {
          return written in values ? values[written as keyof typeof values] : written;
        }

        return expandGoAction(body, date, serviceName, program) ?? written;
      })
      .replace(unsafeFileNameChars, '_')
      .trim();
    // 書き方の結果が空なら、既定の形にする。
    if (name !== '') {
      return `${name}.png`;
    }
  }
  const stamp =
    `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ` +
    `${pad2(date.getHours())}.${pad2(date.getMinutes())}.${pad2(date.getSeconds())}`;
  const parts = [stamp, serviceName];
  if (program !== '') {
    parts.push(program);
  }

  return `${parts.join(' - ').replace(unsafeFileNameChars, '_')}.png`;
}

// captureTexts は、キャプチャの PNG に書き込む文字の情報を返す (#458)。
// 番組名は、ファイル名と同じく囲み文字を外す。放送時刻は、PNG の仕様に従い RFC 1123 の形で書く。
// caption は、撮ったときに出ていた字幕の文。出ていなければ null で、Description は書かない。
export function captureTexts(
  time: number,
  serviceName: string,
  programName: string | null,
  caption: string | null = null,
): PNGText[] {
  const texts: PNGText[] = [
    { keyword: 'Title', text: programName === null ? '' : splitProgramMarks(programName).name },
    { keyword: 'Source', text: serviceName },
    { keyword: 'Creation Time', text: new Date(time).toUTCString() },
  ];
  if (caption !== null && caption !== '') {
    texts.push({ keyword: 'Description', text: caption });
  }

  return texts;
}

// withCaptureTexts は、PNG の画像に、captureTexts の文字の情報を書き込む。
export async function withCaptureTexts(blob: Blob, texts: readonly PNGText[]): Promise<Blob> {
  const png = new Uint8Array(await blob.arrayBuffer());

  return new Blob([withPNGText(png, texts)], { type: blob.type });
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
