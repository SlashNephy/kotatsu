import { describe, expect, test } from 'vitest';

import { toHalfWidth } from './halfWidth';
import { splitLinks, type TextSegment } from './linkify';

const text = (value: string): TextSegment => ({ kind: 'text', text: value });
const link = (value: string, href = value): TextSegment => ({ kind: 'link', text: value, href });

describe('splitLinks', () => {
  test.each([
    {
      name: 'URL がなければ、文字だけにする',
      input: 'ニュースと天気',
      want: [text('ニュースと天気')],
    },
    { name: '空の文字は、何も返さない', input: '', want: [] },
    {
      name: '改行で区切られた URL をリンクにする',
      input: '【番組HP】\nhttps://www.example.jp/news/\n【X】',
      want: [text('【番組HP】\n'), link('https://www.example.jp/news/'), text('\n【X】')],
    },
    {
      name: '和文が続いても、URL に使えない文字で切る',
      input: 'こちら→http://example.jp/a　をご覧ください',
      want: [text('こちら→'), link('http://example.jp/a'), text('　をご覧ください')],
    },
    {
      name: '末尾の句読点は、URL から外す',
      input: '詳しくは https://example.jp/a. または https://example.jp/b!',
      want: [
        text('詳しくは '),
        link('https://example.jp/a'),
        text('. または '),
        link('https://example.jp/b'),
        text('!'),
      ],
    },
    {
      name: '対になる開き括弧のない閉じ括弧は、URL から外す',
      input: '(https://example.jp/a) と https://example.jp/wiki/A_(B)',
      want: [
        text('('),
        link('https://example.jp/a'),
        text(') と '),
        link('https://example.jp/wiki/A_(B)'),
      ],
    },
    {
      name: 'スキームのない www. は、https:// を補う',
      input: 'www.example.jp/ten',
      want: [link('www.example.jp/ten', 'https://www.example.jp/ten')],
    },
    {
      name: '英数字の途中から始まる www. はリンクにしない',
      input: 'abcwww.example.jp',
      want: [text('abcwww.example.jp')],
    },
    {
      name: 'スキームだけで、ホストがなければリンクにしない',
      input: 'http:// のあと',
      want: [text('http:// のあと')],
    },
    {
      name: 'メールアドレスはリンクにしない',
      input: '情報提供は news@example.jp',
      want: [text('情報提供は news@example.jp')],
    },
    {
      name: 'javascript: のような、http(s) でないスキームはリンクにしない',
      input: 'javascript:alert(1)',
      want: [text('javascript:alert(1)')],
    },
  ])('$name', ({ input, want }) => {
    expect(splitLinks(input)).toEqual(want);
  });

  test('全角の URL は、半角に直してから渡せばリンクになり、全角の「？」は ? にする', () => {
    const input = toHalfWidth(
      '配信\nｈｔｔｐｓ：／／ｙｏｕｔｕｂｅ．ｃｏｍ／ｐｌａｙｌｉｓｔ？ｌｉｓｔ＝ＰＬ１',
    );

    expect(splitLinks(input)).toEqual([
      text('配信\n'),
      link('https://youtube.com/playlist？list=PL1', 'https://youtube.com/playlist?list=PL1'),
    ]);
  });
});
