import { fromInputValue, toInputValue } from '../reservations/timeReservation';

// ルールの有効期間は、日付だけで入れる (#464)。開始日の 0 時から、終了日の翌日の 0 時までとし、終了日を含める。
// 日付は表示のタイムゾーンで読む。

const dayMs = 24 * 60 * 60 * 1000;

// validFromDate は、有効期間の開始の時刻を、入力の日付 (YYYY-MM-DD) にする。区切らなければ空。
export function validFromDate(validFrom: string | null): string {
  return validFrom === null ? '' : toInputValue(Date.parse(validFrom)).slice(0, 10);
}

// validUntilDate は、有効期間の終了の時刻 (この時刻を含まない) を、入力の日付 (この日を含む) にする。区切らなければ空。
export function validUntilDate(validUntil: string | null): string {
  return validUntil === null ? '' : toInputValue(Date.parse(validUntil) - 1).slice(0, 10);
}

// fromValidFromDate は、入力の日付を、その日の 0 時の時刻にする。空か、読めなければ、期間を区切らない。
export function fromValidFromDate(date: string): string | null {
  return startOfDay(date);
}

// fromValidUntilDate は、入力の日付を、その翌日の 0 時の時刻にする。終了日の番組も予約するためである。
// 空か、読めなければ、期間を区切らない。
export function fromValidUntilDate(date: string): string | null {
  const day = Date.parse(`${date}T00:00:00Z`);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(day)) {
    return null;
  }

  return startOfDay(new Date(day + dayMs).toISOString().slice(0, 10));
}

// startOfDay は、表示のタイムゾーンでの、日付 date の 0 時の時刻を返す。読めなければ null。
function startOfDay(date: string): string | null {
  const ms = /^\d{4}-\d{2}-\d{2}$/.test(date) ? fromInputValue(`${date}T00:00`) : null;

  return ms === null ? null : new Date(ms).toISOString();
}

// outOfValidity は、now がルールの有効期間の外かどうか。開始の前か、終了の後なら true。
export function outOfValidity(
  validFrom: string | null | undefined,
  validUntil: string | null | undefined,
  now: number,
): boolean {
  return (
    (validFrom != null && now < Date.parse(validFrom)) ||
    (validUntil != null && now >= Date.parse(validUntil))
  );
}
