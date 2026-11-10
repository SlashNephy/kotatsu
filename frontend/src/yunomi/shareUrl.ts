import { broadcastTime, recordingPosition, type Gap } from './position';

// 再生位置を含めた URL の共有 (設計書 7.5)。録画とタイムシフトの再生の画面は、URL の t で再生を始める位置を受け取る。

// parseStartSeconds は、URL の t を読む。秒として読めなければ undefined を返す。
// 空の t (`?t=`) は、Number では 0 になり、続きからの再生を消してしまうので、指定がないものとして扱う。
export function parseStartSeconds(value: unknown): number | undefined {
  const seconds =
    typeof value === 'number'
      ? value
      : typeof value === 'string' && value.trim() !== ''
        ? Number(value)
        : NaN;

  return Number.isFinite(seconds) && seconds >= 0 ? seconds : undefined;
}

// positionUrl は、href に、再生を始める位置 t (秒) を付けた URL を返す。今の URL に付いている t は置き換え、ほかのクエリは残す。
export function positionUrl(href: string, seconds: number): string {
  const url = new URL(href);
  url.searchParams.set('t', String(Math.max(Math.floor(seconds), 0)));
  url.hash = '';

  return url.toString();
}

// TimeshiftRange は、タイムシフトの番組の始まりと、再生できる範囲。
// 再生位置の起点 (playableFrom) は、番組の頭のチャンクを捨てると後ろへ動くので、URL の t は番組の始まりからの放送の秒で表す。
type TimeshiftRange = { startAt: string; playableFrom: string; gaps: readonly Gap[] };

// timeshiftShareSeconds は、タイムシフトの再生位置 position (秒) を、URL に載せる、番組の始まりからの秒に直す。
export function timeshiftShareSeconds(range: TimeshiftRange, position: number): number {
  return (
    (broadcastTime(range.playableFrom, range.gaps, position) - Date.parse(range.startAt)) / 1000
  );
}

// timeshiftStartPosition は、URL の t (番組の始まりからの秒) を、今の再生位置に直す。もう捨てた所を指していたら、再生できる範囲の始まりにする。
export function timeshiftStartPosition(range: TimeshiftRange, seconds: number): number {
  return recordingPosition(
    range.playableFrom,
    range.gaps,
    Date.parse(range.startAt) + seconds * 1000,
  );
}

// copyPositionUrl は、今の画面の URL に位置を付けて、クリップボードに入れる。element は、プレイヤーの中の要素。
// プレイヤーを別のウィンドウに出している間は、フォーカスがそのウィンドウにあるので、そのウィンドウのクリップボードを使う。
// クリップボードを使えない (HTTP で開いているなど) か、ブラウザに断られたら、reject する。
export async function copyPositionUrl(seconds: number, element: Element | null): Promise<void> {
  const target = element?.ownerDocument.defaultView ?? window;
  await target.navigator.clipboard.writeText(positionUrl(window.location.href, seconds));
}
