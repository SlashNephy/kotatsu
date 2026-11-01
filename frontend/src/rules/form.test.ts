import { describe, expect, test } from 'vitest';

import { emptyRuleForm, fromRule, toInput, type RuleData } from './form';

// imported は、EPGStation から取り込んだような、画面で編集しない項目まで埋まったルール。
const imported: RuleData = {
  name: '名探偵',
  memo: 'メモ',
  enabled: true,
  conditions: {
    keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
    keywordMatch: 'ANY',
    excludeKeywords: [
      { text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
    ],
    channelTypes: ['GR'],
    services: [{ id: 'U2VydmljZTox' }],
    excludeServices: [{ id: 'U2VydmljZToy' }],
    genres: [{ lv1: 7, lv2: 0 }],
    excludeGenres: [{ lv1: 0, lv2: null }],
    timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
    minDurationSeconds: 1200,
    maxDurationSeconds: 7200,
    freeOnly: true,
    marks: ['新'],
    excludeMarks: ['再'],
  },
  settings: {
    poolId: 'main',
    fileNameTemplate: '{{.Name}}',
    startMarginSeconds: 5,
    endMarginSeconds: 10,
    priority: 3,
    joinSegments: false,
    detectCm: true,
  },
  validFrom: '2026-10-01T00:00:00Z',
  validUntil: null,
  avoidDuplicates: true,
  duplicateWindowDays: 30,
  followEventRelay: false,
};

describe('fromRule と toInput', () => {
  test('画面で編集しない項目も、保存し直したときに失わない', () => {
    expect(toInput(fromRule(imported))).toEqual({
      name: '名探偵',
      memo: 'メモ',
      enabled: true,
      conditions: {
        keywords: [{ text: '^名探偵', regexp: true, caseSensitive: true, fields: ['NAME'] }],
        keywordMatch: 'ANY',
        excludeKeywords: [
          { text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
        ],
        channelTypes: ['GR'],
        services: ['U2VydmljZTox'],
        excludeServices: ['U2VydmljZToy'],
        genres: [{ lv1: 7, lv2: 0 }],
        excludeGenres: [{ lv1: 0, lv2: null }],
        timeRanges: [{ weekdays: [6], startMinute: 18 * 60, durationMinutes: 90 }],
        minDurationSeconds: 1200,
        maxDurationSeconds: 7200,
        freeOnly: true,
        marks: ['新'],
        excludeMarks: ['再'],
      },
      settings: {
        poolId: 'main',
        fileNameTemplate: '{{.Name}}',
        startMarginSeconds: 5,
        endMarginSeconds: 10,
        priority: 3,
        joinSegments: false,
        detectCm: true,
      },
      validFrom: '2026-10-01T00:00:00Z',
      validUntil: null,
      avoidDuplicates: true,
      duplicateWindowDays: 30,
      followEventRelay: false,
    });
  });
});

describe('toInput', () => {
  test('重複を避けないときは、期間を送らない', () => {
    const form = { ...emptyRuleForm(), avoidDuplicates: false, duplicateWindowDays: 30 };
    expect(toInput(form).duplicateWindowDays).toBeNull();
  });
});
