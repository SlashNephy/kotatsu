// 画面で日時を表すタイムゾーン (設計書 8.1、#504)。サーバーの設定 (time_zone) に合わせる。
// 起動するときに読み込み (loadTimeZone)、それまでと、読めなかったときは、既定の Asia/Tokyo を使う。

import i18next from 'i18next';
import { useTranslation } from 'react-i18next';

import { dateLocale } from '../i18n/language';

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
// locale を渡さなければ、画面の言語の書式で書く (#831)。
export function zonedFormat(
  options: Omit<Intl.DateTimeFormatOptions, 'timeZone'>,
  locale?: string,
): ZonedFormat {
  let cached: { zone: string; locale: string; format: Intl.DateTimeFormat } | null = null;
  const current = () => {
    const resolved = locale ?? dateLocale(i18next.language);
    if (cached?.zone !== zone || cached.locale !== resolved) {
      cached = {
        zone,
        locale: resolved,
        format: new Intl.DateTimeFormat(resolved, { ...options, timeZone: zone }),
      };
    }

    return cached.format;
  };

  return { format: (date) => current().format(date) };
}

// partsFormat は、時差を求めるのに使う、今のタイムゾーンの日付と時刻の書式。タイムゾーンが変わったら作り直す。
let partsFormat: { zone: string; format: Intl.DateTimeFormat } | null = null;

// wallClock は、時刻 time の、今のタイムゾーンでの日付と時刻を、UTC の時刻として表した値を返す。
// 例えば日本時間の 2026-10-16 21:00 は、Date.UTC(2026, 9, 16, 21, 0) を返す。
function wallClock(time: number): number {
  if (partsFormat?.zone !== zone) {
    partsFormat = {
      zone,
      format: new Intl.DateTimeFormat('en-US', {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: 'numeric',
        second: 'numeric',
        hourCycle: 'h23',
        timeZone: zone,
      }),
    };
  }
  // 書いた文字列を区切って読むと、ブラウザが足す見えない文字で崩れるので、部分ごとに受け取る。
  const parts: Partial<Record<Intl.DateTimeFormatPartTypes, number>> = {};
  for (const part of partsFormat.format.formatToParts(time)) {
    parts[part.type] = Number(part.value);
  }
  const utc = new Date(0);
  utc.setUTCFullYear(parts.year ?? 1970, (parts.month ?? 1) - 1, parts.day ?? 1);
  utc.setUTCHours(
    parts.hour ?? 0,
    parts.minute ?? 0,
    parts.second ?? 0,
    ((time % 1000) + 1000) % 1000,
  );

  return utc.getTime();
}

// zoneOffset は、時刻 time での、今のタイムゾーンの UTC からの差 (ミリ秒) を返す。夏時間のある地域では、時刻で変わる。
export function zoneOffset(time: number): number {
  return wallClock(time) - time;
}

const day = 24 * 3_600_000;

// fromWallClock は、今のタイムゾーンでの日付と時刻 (UTC の時刻として表した値) から、その時刻を返す。
// 夏時間の切り替えで、2 回ある時刻は早いほうにし、ない時刻 (時計を進めて飛ばした時刻) は、切り替える前の差で読む
// (2:30 が飛ばされたら 3:30 になる)。時差が 1 日のうちに 2 回変わることはないとみなす。
export function fromWallClock(local: number): number {
  const before = local - zoneOffset(local - day);
  const after = local - zoneOffset(local + day);
  const candidates = [before, after].filter((time) => wallClock(time) === local);
  if (candidates.length > 0) {
    return Math.min(...candidates);
  }

  return before;
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
