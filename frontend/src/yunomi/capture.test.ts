import { describe, expect, test } from 'vitest';

import { captureFileName, captureTexts, fitContain, overlayCanvases } from './capture';

describe('fitContain', () => {
  test.each([
    {
      name: '同じ比率なら、枠いっぱい',
      size: [1280, 720],
      want: { x: 0, y: 0, width: 1920, height: 1080 },
    },
    {
      name: '4:3 は、左右に黒い帯を残す',
      size: [640, 480],
      want: { x: 240, y: 0, width: 1440, height: 1080 },
    },
    {
      name: '大きさが分からなければ、描かない',
      size: [0, 0],
      want: { x: 0, y: 0, width: 0, height: 0 },
    },
  ])('$name', ({ size, want }) => {
    expect(fitContain(size[0] ?? 0, size[1] ?? 0, 1920, 1080)).toEqual(want);
  });
});

describe('overlayCanvases', () => {
  test('印の付いた、出している canvas だけを返し、yadif の canvas は除く', () => {
    const frame = document.createElement('div');
    const media = document.createElement('div');
    const video = document.createElement('video');
    const yadif = document.createElement('canvas');
    const captions = document.createElement('canvas');
    captions.dataset.overlay = 'captions';
    const hidden = document.createElement('canvas');
    hidden.dataset.overlay = 'captions';
    hidden.style.visibility = 'hidden';
    const comments = document.createElement('canvas');
    comments.dataset.overlay = 'comments';
    // kyusu の経路では、字幕の canvas が video と同じ親に入る。
    media.append(video, yadif, captions, hidden);
    frame.append(media, comments);

    expect(overlayCanvases(frame)).toEqual([captions, comments]);
  });
});

describe('captureFileName', () => {
  // 端末のタイムゾーンによらないよう、ローカル時刻で作る。
  const time = new Date(2026, 9, 16, 21, 5, 9).getTime();

  test.each([
    {
      name: 'テンプレートの書き方を、日時、サービス名、囲み文字を外した番組名にする',
      template: '%title%_%year%%month%%day%-%hour%%minute%%second% (%service%)',
      want: 'ニュース７_20261016-210509 (NHK総合1・東京).png',
    },
    {
      name: '日付と時刻をまとめた書き方',
      template: '%date% %time%',
      want: '2026-10-16 21.05.09.png',
    },
    {
      name: 'ファイル名に使えない文字は _ にする',
      template: '%service%/%title%',
      want: 'NHK総合1・東京_ニュース７.png',
    },
    {
      name: 'TVTest と同じ名前 (%event-name% と %service-name%) も使える (#814)',
      template: '%service-name% %event-name%',
      want: 'NHK総合1・東京 ニュース７.png',
    },
    {
      name: '知らない書き方は、そのまま残す',
      template: '%unknown%-%title%',
      want: '%unknown%-ニュース７.png',
    },
    {
      name: '空なら、既定の形',
      template: '  ',
      want: '2026-10-16 21.05.09 - NHK総合1・東京 - ニュース７.png',
    },
  ])('$name', ({ template, want }) => {
    expect(captureFileName(time, 'NHK総合1・東京', 'ニュース７🈑', template)).toBe(want);
  });

  test.each([
    {
      name: 'ローカル時刻、サービス名、番組名を並べる',
      service: 'NHK総合1・東京',
      program: '大河ドラマ「ひかる」',
      want: '2026-10-16 21.05.09 - NHK総合1・東京 - 大河ドラマ「ひかる」.png',
    },
    {
      name: '番組名の囲み文字を外す',
      service: 'NHK総合1・東京',
      program: '🈑ニュース7🈞',
      want: '2026-10-16 21.05.09 - NHK総合1・東京 - ニュース7.png',
    },
    {
      name: '番組がなければ、サービス名だけ',
      service: 'BS11',
      program: null,
      want: '2026-10-16 21.05.09 - BS11.png',
    },
    {
      name: '囲み文字だけの番組名は、ないものとして扱う',
      service: 'BS11',
      program: '🈑🈞',
      want: '2026-10-16 21.05.09 - BS11.png',
    },
    {
      name: 'サービス名の使えない文字も置き換える',
      service: 'A/B',
      program: null,
      want: '2026-10-16 21.05.09 - A_B.png',
    },
    {
      name: 'ファイル名に使えない文字を置き換える',
      service: 'TOKYO MX',
      program: 'A/B: 前編? <1>|"2"*\\3',
      want: '2026-10-16 21.05.09 - TOKYO MX - A_B_ 前編_ _1___2___3.png',
    },
  ])('$name', ({ service, program, want }) => {
    expect(captureFileName(time, service, program)).toBe(want);
  });
});

describe('captureTexts', () => {
  // 2026-10-16 21:00:05 (日本時間)。
  const time = Date.UTC(2026, 9, 16, 12, 0, 5);

  test.each([
    {
      name: '番組名は囲み文字を外し、放送時刻は RFC 1123 で書く',
      programName: 'ニュース７🈑🈞',
      want: [
        { keyword: 'Title', text: 'ニュース７' },
        { keyword: 'Source', text: 'NHK総合1・東京' },
        { keyword: 'Creation Time', text: 'Fri, 16 Oct 2026 12:00:05 GMT' },
      ],
    },
    {
      name: '番組がなければ、番組名は空にする',
      programName: null,
      want: [
        { keyword: 'Title', text: '' },
        { keyword: 'Source', text: 'NHK総合1・東京' },
        { keyword: 'Creation Time', text: 'Fri, 16 Oct 2026 12:00:05 GMT' },
      ],
    },
  ])('$name', ({ programName, want }) => {
    expect(captureTexts(time, 'NHK総合1・東京', programName)).toEqual(want);
  });

  test.each([
    {
      name: '字幕が出ていれば、Description に書く',
      caption: 'こんにちは\n元気？',
      want: 'こんにちは\n元気？',
    },
    { name: '字幕が出ていなければ、Description は書かない', caption: null, want: undefined },
    { name: '空の字幕も書かない', caption: '', want: undefined },
  ])('$name', ({ caption, want }) => {
    const texts = captureTexts(time, 'NHK総合1・東京', 'ニュース７', caption);

    expect(texts.find((text) => text.keyword === 'Description')?.text).toBe(want);
  });
});
