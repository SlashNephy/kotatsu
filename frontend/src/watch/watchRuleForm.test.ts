import { describe, expect, test } from 'vitest';

import {
  emptyWatchRuleForm,
  fromWatchRule,
  toPreviewInput,
  toWatchRuleInput,
} from './watchRuleForm';

describe('watch rule form', () => {
  test('a saved watch rule round-trips through the form', () => {
    const conditions = {
      keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
      keywordMatch: 'ANY',
      excludeKeywords: [],
      channelTypes: ['GR'],
      services: [{ id: 'U2VydmljZTox' }],
      excludeServices: [],
      genres: [{ lv1: 7, lv2: 0 }],
      excludeGenres: [],
      timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
      minDurationSeconds: 1200,
      maxDurationSeconds: null,
      freeOnly: true,
      marks: ['新'],
      excludeMarks: [],
    } as const;

    const input = toWatchRuleInput(fromWatchRule({ name: '名探偵', enabled: false, conditions }));

    expect(input).toEqual({
      name: '名探偵',
      enabled: false,
      conditions: {
        keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
        keywordMatch: 'ANY',
        excludeKeywords: [],
        channelTypes: ['GR'],
        services: ['U2VydmljZTox'],
        excludeServices: [],
        genres: [{ lv1: 7, lv2: 0 }],
        excludeGenres: [],
        timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
        minDurationSeconds: 1200,
        maxDurationSeconds: null,
        freeOnly: true,
        marks: ['新'],
        excludeMarks: [],
      },
    });
  });

  test('the preview matches like a recording rule without a validity period or duplicate avoidance', () => {
    const form = { ...emptyWatchRuleForm(), name: '  ニュース  ' };

    expect(toPreviewInput(form)).toMatchObject({
      name: 'ニュース',
      validFrom: null,
      validUntil: null,
      avoidDuplicates: false,
    });
  });
});
