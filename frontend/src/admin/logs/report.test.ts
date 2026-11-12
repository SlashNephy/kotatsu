import { describe, expect, test } from 'vitest';

import {
  buildURL,
  encodedLength,
  environmentLimit,
  fitURL,
  formatLine,
  fullLogs,
  logsBudget,
  reportFileName,
  selectLogs,
  titleLimit,
  toWellFormed,
  truncate,
  truncateEncoded,
  urlLimit,
  type IssueFields,
  type ReportLine,
  type ReportSource,
} from './report';

const base = new Date(2026, 10, 12, 21, 0, 0).getTime();

const line = (
  minute: number,
  level: ReportLine['level'],
  text: string,
  inferred = false,
): ReportLine => ({
  time: base + minute * 60_000,
  level,
  inferred,
  text,
});

const fields: Omit<IssueFields, 'logs'> = {
  title: '不具合: 録画が止まる',
  version: 'v2026.11.0',
  environment: '- OS: linux',
};

// logsOf は、URL の logs の欄を返す。
const logsOf = (url: string) => new URL(url).searchParams.get('logs') ?? '';

describe('formatLine', () => {
  test.each([
    {
      name: 'info には印を付けない',
      line: line(0, 'INFO', 'started'),
      want: '2026-11-12 21:00:00 started',
    },
    {
      name: '本物の warn には印を付ける',
      line: line(1, 'WARN', 'slow'),
      want: '2026-11-12 21:01:00 [warn] slow',
    },
    {
      name: '本物の error には印を付ける',
      line: line(2, 'ERROR', 'boom'),
      want: '2026-11-12 21:02:00 [error] boom',
    },
    {
      name: '推し量った重さには印を付けない',
      line: line(3, 'ERROR', 'connection failed', true),
      want: '2026-11-12 21:03:00 connection failed',
    },
    { name: '改行は空白にする', line: line(4, 'INFO', 'a\nb'), want: '2026-11-12 21:04:00 a b' },
  ])('$name', ({ line, want }) => {
    expect(formatLine(line)).toBe(want);
  });

  test('報告の 1 行の上限で切る', () => {
    expect(formatLine(line(0, 'INFO', 'x'.repeat(1000)), 10)).toBe(
      `2026-11-12 21:00:00 ${'x'.repeat(9)}…`,
    );
  });
});

describe('selectLogs', () => {
  test('warn と error を先に入れ、入れた行は出どころごとに古い順に並べる', () => {
    const source: ReportSource = {
      name: 'Kotatsu',
      total: 4,
      failure: null,
      lines: [
        line(0, 'ERROR', 'old error'),
        line(1, 'INFO', 'old info'),
        line(2, 'WARN', 'warn'),
        line(3, 'INFO', 'new info'),
      ],
    };
    const header = '== Kotatsu (4 / 4) ==';
    // 見出しと、省いた行数を知らせる行と、warn と error の 2 行だけが入る長さ。
    const budget =
      encodedLength(
        '(URL の上限に入らない 9 行を省きました。全部入りの報告は「コピー」か「ダウンロード」で取れます)\n',
      ) +
      encodedLength(`${header}\n`) +
      encodedLength(`${formatLine(source.lines[0]!)}\n`) +
      encodedLength(`${formatLine(source.lines[2]!)}\n`);
    expect(selectLogs([source], budget).split('\n')).toEqual([
      '(URL の上限に入らない 2 行を省きました。全部入りの報告は「コピー」か「ダウンロード」で取れます)',
      '== Kotatsu (2 / 4) ==',
      '2026-11-12 21:00:00 [error] old error',
      '2026-11-12 21:02:00 [warn] warn',
    ]);
  });

  test('入らない行は飛ばして、短い行を試す (途中で打ち切らない)', () => {
    const source: ReportSource = {
      name: 'Kotatsu',
      total: 3,
      failure: null,
      lines: [line(0, 'INFO', 'short'), line(1, 'INFO', 'x'.repeat(300)), line(2, 'INFO', 'tiny')],
    };
    const budget = 600;
    const selected = selectLogs([source], budget);
    expect(selected).toContain('short');
    expect(selected).toContain('tiny');
    expect(selected).not.toContain('xxxx');
    expect(encodedLength(selected)).toBeLessThanOrEqual(budget);
  });

  test('出どころごとの見出しと、取れなかった理由を入れる', () => {
    const sources: ReportSource[] = [
      { name: 'Kotatsu', total: 1, failure: null, lines: [line(0, 'INFO', 'a')] },
      { name: 'チューナーサーバー', total: 0, failure: 'ログの API がありません', lines: [] },
    ];
    expect(selectLogs(sources, 10_000).split('\n')).toEqual([
      '== Kotatsu (1 / 1) ==',
      '2026-11-12 21:00:00 a',
      '== チューナーサーバー (0 / 0) ==',
      '(ログの API がありません)',
    ]);
  });
});

describe('URL', () => {
  // sources は、URL の上限を大きく超える量のログ。日本語と絵文字を混ぜる。
  const sources: ReportSource[] = [
    {
      name: 'Kotatsu',
      total: 600,
      failure: null,
      lines: Array.from({ length: 600 }, (_, i) => {
        const level = i % 50 === 0 ? 'ERROR' : i % 20 === 0 ? 'WARN' : 'INFO';

        return line(i, level, `録画のログ ${i} 🎬 ${level === 'INFO' ? 'あ'.repeat(i % 40) : ''}`);
      }),
    },
  ];

  test('URL の上限に収まり、warn と error が先に入る', () => {
    const logs = selectLogs(sources, logsBudget(fields));
    const url = buildURL({ ...fields, logs });
    expect(url.length).toBeLessThanOrEqual(urlLimit);
    expect(url.length).toBeGreaterThan(urlLimit - 400);
    // error は 12 行、warn は 24 行。info より先に、どちらも全部入る。
    expect(logsOf(url).match(/\[error\]/g)).toHaveLength(12);
    expect(logsOf(url).match(/\[warn\]/g)).toHaveLength(24);
    // info は、新しい方から入る。最も新しい行は入り、最も古い行は入らない。
    expect(logsOf(url)).toContain('録画のログ 599 ');
    expect(logsOf(url)).not.toContain('録画のログ 1 ');
    expect(logsOf(url)).toContain('(URL の上限に入らない');
  });

  test('フォームの欄の id を問い合わせのキーにする', () => {
    const url = new URL(buildURL({ ...fields, logs: 'x' }));
    expect(url.origin + url.pathname).toBe('https://github.com/SlashNephy/kotatsu/issues/new');
    expect(Object.fromEntries(url.searchParams)).toEqual({
      template: 'bug_report.yml',
      title: fields.title,
      version: fields.version,
      environment: fields.environment,
      logs: 'x',
    });
  });

  test('書き足して上限を超えたら、ログの先頭から削り、何行省いたかを書く', () => {
    const logs = selectLogs(sources, logsBudget(fields));
    const original = logs.split('\n');
    const { url, trimmed } = fitURL({
      ...fields,
      environment: `${fields.environment}\n${'追記'.repeat(100)}`,
      logs,
    });
    expect(url.length).toBeLessThanOrEqual(urlLimit);
    expect(trimmed).toBeGreaterThan(0);
    const got = logsOf(url).split('\n');
    expect(got[0]).toBe(`(古い方から ${trimmed} 行を省きました)`);
    // 残るのは、元の後ろ (新しい方) の行。
    expect(got.slice(1)).toEqual(original.slice(trimmed));
  });

  test('上限に収まれば削らない', () => {
    expect(fitURL({ ...fields, logs: 'a\nb' })).toEqual({
      url: buildURL({ ...fields, logs: 'a\nb' }),
      trimmed: 0,
    });
  });

  test('先頭の 1 行も入らないときは、ログを空にする', () => {
    const { url } = fitURL({ ...fields, logs: 'x'.repeat(urlLimit) });
    expect(logsOf(url)).toBe('');
    expect(url.length).toBeLessThanOrEqual(urlLimit);
  });

  test('絵文字の途中で切っても、URL を作れる', () => {
    const broken = '🎬'.repeat(3).slice(0, 5);
    expect(() => buildURL({ ...fields, logs: broken })).not.toThrow();
    expect(logsOf(buildURL({ ...fields, logs: broken }))).toBe('🎬🎬�');
    expect(() => encodeURIComponent(toWellFormed('\uDC00a\uD800'))).not.toThrow();
    expect(toWellFormed('\uDC00a\uD800')).toBe('�a�');
  });

  test('タイトルと環境は、字数とエンコード後の長さの上限で切る', () => {
    const { url } = fitURL({
      title: 'あ'.repeat(500),
      version: 'v1',
      environment: 'e'.repeat(5000),
      logs: '',
    });
    const params = new URL(url).searchParams;
    const title = params.get('title') ?? '';
    const environment = params.get('environment') ?? '';
    // 日本語のタイトルは、字数より先にエンコード後の長さの上限に当たる。
    expect(encodedLength(title)).toBeLessThanOrEqual(titleLimit.encoded);
    expect(title.length).toBeLessThan(titleLimit.chars);
    expect(title.endsWith('…')).toBe(true);
    expect(environment.length).toBeLessThanOrEqual(environmentLimit.chars);
    expect(encodedLength(environment)).toBeLessThanOrEqual(environmentLimit.encoded);
  });
});

describe('truncate', () => {
  test.each([
    { text: 'abcdef', max: 4, want: 'abc…' },
    { text: 'abc', max: 4, want: 'abc' },
    // サロゲートペアの間では切らない。
    { text: 'ab🎬cd', max: 4, want: 'ab…' },
  ])('$text を $max 字に', ({ text, max, want }) => {
    expect(truncate(text, max)).toBe(want);
  });

  test('エンコード後の長さで切るときも、サロゲートペアの間では切らない', () => {
    const cut = truncateEncoded('🎬'.repeat(100), { chars: 1000, encoded: 50 });
    expect(encodedLength(cut)).toBeLessThanOrEqual(50);
    expect(toWellFormed(cut)).toBe(cut);
  });
});

describe('fullLogs', () => {
  test('1 行も削らず、行も切らない', () => {
    const source: ReportSource = {
      name: 'Kotatsu',
      total: 2,
      failure: null,
      lines: [line(0, 'INFO', 'x'.repeat(5000)), line(1, 'WARN', 'y')],
    };
    const lines = fullLogs([source]).split('\n');
    expect(lines).toHaveLength(3);
    expect(lines[1]).toHaveLength('2026-11-12 21:00:00 '.length + 5000);
  });
});

test('reportFileName は、作った時刻を分まで入れる', () => {
  expect(reportFileName(new Date(2026, 10, 12, 9, 5, 59))).toBe('kotatsu-report-20261112-0905.md');
});
