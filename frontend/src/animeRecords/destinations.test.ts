import { describe, expect, it } from 'vitest';

import { animeRecordCallbackError } from './destinations';

describe('animeRecordCallbackError', () => {
  it.each([
    ['anime_record_expired', 'expired'],
    ['anime_record_mismatch', 'mismatch'],
    ['anime_record_denied', 'denied'],
    ['anime_record_unavailable', 'unavailable'],
    ['anime_record_failed', 'failed'],
    // Simkl から戻ってきたときの理由も、同じ文で出す (#1649)。
    ['drama_record_expired', 'expired'],
    ['drama_record_mismatch', 'mismatch'],
    ['drama_record_denied', 'denied'],
    ['drama_record_unavailable', 'unavailable'],
    ['drama_record_failed', 'failed'],
    ['something_else', 'failed'],
    ['<script>', 'failed'],
  ])('maps %s to %s', (reason, want) => {
    expect(animeRecordCallbackError(reason)).toBe(want);
  });
});
