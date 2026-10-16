// 画面で日時を表すタイムゾーン (設計書 8.1、#504)。サーバーの設定 (time_zone) に合わせる。
// 起動するときに読み込み (loadTimeZone)、それまでと、読めなかったときは、既定の Asia/Tokyo を使う。

import { useTranslation } from 'react-i18next';

export const defaultTimeZone = 'Asia/Tokyo';

let zone = defaultTimeZone;

// timeZone は、今のタイムゾーンを返す。
export function timeZone(): string {
  return zone;
}

// setTimeZone は、タイムゾーンを変える。ブラウザが知らない名前なら、変えずに false を返す。
export function setTimeZone(name: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: name });
  } catch {
    return false;
  }
  zone = name;

  return true;
}

// ZonedFormat は、今のタイムゾーンで日時を書く書式。
export type ZonedFormat = { format: (date?: Date | number) => string };

// zonedFormat は、options の書式で、今のタイムゾーンの日時を書く ZonedFormat を返す。
// 書式は、モジュールを読み込んだとき (タイムゾーンを読み込む前) に作られるので、書くときにタイムゾーンを選ぶ。
export function zonedFormat(
  options: Omit<Intl.DateTimeFormatOptions, 'timeZone'>,
  locale = 'ja-JP',
): ZonedFormat {
  let cached: { zone: string; format: Intl.DateTimeFormat } | null = null;
  const current = () => {
    if (cached?.zone !== zone) {
      cached = { zone, format: new Intl.DateTimeFormat(locale, { ...options, timeZone: zone }) };
    }

    return cached.format;
  };

  return { format: (date) => current().format(date) };
}

const offsetParts = zonedFormat(
  {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
    hourCycle: 'h23',
  },
  'en-US',
);

// wallClock は、時刻 time の、今のタイムゾーンでの日付と時刻を、UTC の時刻として表した値を返す。
// 例えば日本時間の 2026-10-16 21:00 は、Date.UTC(2026, 9, 16, 21, 0) を返す。
function wallClock(time: number): number {
  // en-US の h23 では「10/16/2026, 21:00:00」の形になる。
  const text = offsetParts.format(time);
  const [month, day, year, hour, minute, second] = text.split(/\D+/).map(Number);

  return (
    Date.UTC(year!, month! - 1, day!, hour!, minute!, second!) + (((time % 1000) + 1000) % 1000)
  );
}

// zoneOffset は、時刻 time での、今のタイムゾーンの UTC からの差 (ミリ秒) を返す。夏時間のある地域では、時刻で変わる。
export function zoneOffset(time: number): number {
  return wallClock(time) - time;
}

// fromWallClock は、今のタイムゾーンでの日付と時刻 (UTC の時刻として表した値) から、その時刻を返す。
// 夏時間の切り替えで、ない時刻や 2 回ある時刻は、切り替える前の差で決める。
export function fromWallClock(local: number): number {
  const guess = local - zoneOffset(local);

  return local - zoneOffset(guess);
}

// useTimeZoneLabel は、入力の説明に添える、今のタイムゾーンの呼び方 (日本時間、Europe/London の時刻など) を返す。
export function useTimeZoneLabel(): string {
  const { t } = useTranslation();

  return zone === defaultTimeZone ? t('timeZone.tokyo') : t('timeZone.other', { zone });
}

// loadTimeZoneTimeoutMs は、タイムゾーンを読み込むのを待つ上限。待たせすぎずに、既定のタイムゾーンで画面を出す。
const loadTimeZoneTimeoutMs = 3000;

// loadTimeZone は、サーバーの表示のタイムゾーンを読み込む。ログインしていないときや、読めなかったときは、既定のまま。
// ログインすると画面を読み直すので、ログインした後の画面は、サーバーのタイムゾーンで出る。
export async function loadTimeZone(endpoint = '/graphql'): Promise<void> {
  try {
    const response = await fetch(endpoint, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ query: '{ timeZone }' }),
      signal: AbortSignal.timeout(loadTimeZoneTimeoutMs),
    });
    if (!response.ok) {
      return;
    }
    const body: unknown = await response.json();
    const name =
      typeof body === 'object' && body !== null && 'data' in body
        ? (body.data as { timeZone?: unknown } | null)?.timeZone
        : undefined;
    if (typeof name === 'string') {
      setTimeZone(name);
    }
  } catch {
    // 読めなくても、既定のタイムゾーンで画面を出す。
  }
}
