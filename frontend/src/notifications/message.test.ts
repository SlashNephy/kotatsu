import { describe, expect, test } from 'vitest';

import { splitMessage } from './message';

describe('splitMessage', () => {
  test.each([
    {
      name: '文の中の番組名で分ける',
      message: '「相棒14 #4🈖🈑」の録画が終わりました',
      payload: '{"name":"相棒14 #4🈖🈑"}',
      want: { before: '「', name: '相棒14 #4🈖🈑', after: '」の録画が終わりました' },
    },
    {
      name: '全角の番組名は、サーバーが文に書くのと同じく半角にして探す',
      message: '21:00 から TOKYO MX1で「相棒14 #4🈑」が始まります',
      payload: '{"name":"相棒１４　＃４🈑"}',
      want: { before: '21:00 から TOKYO MX1で「', name: '相棒14 #4🈑', after: '」が始まります' },
    },
    {
      name: '番組名のない通知は、文をそのまま返す',
      message: '保存先の空き容量が足りません',
      payload: '{}',
      want: { before: '保存先の空き容量が足りません', name: undefined, after: '' },
    },
    {
      name: '番組名が空なら、文をそのまま返す',
      message: '録画を始めました',
      payload: '{"name":""}',
      want: { before: '録画を始めました', name: undefined, after: '' },
    },
    {
      name: '番組名が文になければ、文をそのまま返す',
      message: '録画を始めました',
      payload: '{"name":"ニュース"}',
      want: { before: '録画を始めました', name: undefined, after: '' },
    },
    {
      name: 'payload が JSON でなければ、文をそのまま返す',
      message: '録画を始めました',
      payload: 'not json',
      want: { before: '録画を始めました', name: undefined, after: '' },
    },
    {
      name: 'payload が null でも、文をそのまま返す',
      message: '録画を始めました',
      payload: 'null',
      want: { before: '録画を始めました', name: undefined, after: '' },
    },
  ])('$name', ({ message, payload, want }) => {
    expect(splitMessage(message, payload)).toEqual(want);
  });
});
