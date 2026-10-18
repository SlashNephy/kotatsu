import { describe, expect, test } from 'vitest';

import {
  appendLiveCaption,
  insertRecordedCaption,
  maxCaptions,
  normalizeCaption,
  type CaptionEntry,
} from './captionHistory';

describe('normalizeCaption', () => {
  test('行の前後の空白と、空の行を除く', () => {
    expect(normalizeCaption('  こんにちは　\n\n　元気？ ')).toBe('こんにちは\n元気？');
  });
});

describe('appendLiveCaption', () => {
  test('出た順に足し、空の字幕と、直前と同じ文は足さない', () => {
    let list: readonly CaptionEntry[] = [];
    list = appendLiveCaption(list, 1000, 'こんにちは');
    list = appendLiveCaption(list, 2000, ' こんにちは ');
    list = appendLiveCaption(list, 3000, '　');
    list = appendLiveCaption(list, 4000, 'さようなら');
    list = appendLiveCaption(list, 5000, 'こんにちは');

    expect(list).toEqual([
      { key: 0, at: 1000, text: 'こんにちは' },
      { key: 1, at: 4000, text: 'さようなら' },
      { key: 2, at: 5000, text: 'こんにちは' },
    ]);
  });

  test('上限を超えたら、古いものから捨てる', () => {
    let list: readonly CaptionEntry[] = [];
    for (let i = 0; i < maxCaptions + 5; i++) {
      list = appendLiveCaption(list, i, `字幕${i}`);
    }

    expect(list).toHaveLength(maxCaptions);
    expect(list[0]?.text).toBe('字幕5');
  });
});

describe('insertRecordedCaption', () => {
  test('位置の順に差し込み、近い位置で出し直した同じ字幕は足さない', () => {
    let list: readonly CaptionEntry[] = [];
    list = insertRecordedCaption(list, 60, '後の字幕');
    list = insertRecordedCaption(list, 10, '前の字幕');
    // シークで戻り、同じ字幕がもう一度出た。
    list = insertRecordedCaption(list, 10.5, '前の字幕');
    // 離れた位置で、同じ文の字幕が出た。
    list = insertRecordedCaption(list, 120, '前の字幕');

    expect(list.map(({ at, text }) => ({ at, text }))).toEqual([
      { at: 10, text: '前の字幕' },
      { at: 60, text: '後の字幕' },
      { at: 120, text: '前の字幕' },
    ]);
  });

  test('上限を超えたら、差し込んだ位置から最も遠いものを捨てる', () => {
    let list: readonly CaptionEntry[] = [];
    for (let i = 0; i < maxCaptions; i++) {
      list = insertRecordedCaption(list, i * 10, `字幕${i}`);
    }
    // 先頭の近くに差し込んだので、最も遠い末尾を捨てる。
    list = insertRecordedCaption(list, 5, '新しい字幕');

    expect(list).toHaveLength(maxCaptions);
    expect(list[1]?.text).toBe('新しい字幕');
    expect(list.at(-1)?.text).toBe(`字幕${maxCaptions - 2}`);
  });
});
