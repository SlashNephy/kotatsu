import { describe, expect, test } from 'vitest';

import { JikkyoSession, jikkyoSocketURL, type LiveComment, type SessionStatus } from './session';

// FakeSocket は、送ったものを覚え、テストからイベントを起こせる WebSocket の代わり。
class FakeSocket {
  readonly url: string;
  readonly sent: unknown[] = [];
  closed = false;
  private readonly listeners = new Map<string, ((event: { data?: string }) => void)[]>();

  constructor(url: string) {
    this.url = url;
  }

  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }

  close(): void {
    this.closed = true;
  }

  addEventListener(type: string, listener: (event: { data?: string }) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  emit(type: string, data?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(data === undefined ? {} : { data: JSON.stringify(data) });
    }
  }
}

// setup は、偽の WebSocket とタイマーでセッションを作る。
function setup() {
  const sockets: FakeSocket[] = [];
  const timers: { ms: number; callback: () => void; cancelled: boolean }[] = [];
  const comments: LiveComment[] = [];
  const statuses: SessionStatus[] = [];
  const session = new JikkyoSession(
    'jk1',
    { onComment: (comment) => comments.push(comment), onStatus: (status) => statuses.push(status) },
    {
      url: 'ws://kotatsu.test/media/jikkyo/jk1/ws',
      createSocket: (url) => {
        const socket = new FakeSocket(url);
        sockets.push(socket);

        return socket as unknown as WebSocket;
      },
      now: () => 1234,
      setTimer: (callback, ms) => {
        const timer = { ms, callback, cancelled: false };
        timers.push(timer);

        return () => {
          timer.cancelled = true;
        };
      },
    },
  );

  return { session, sockets, timers, comments, statuses };
}

describe('jikkyoSocketURL', () => {
  test.each([
    ['https:', 'wss://tv.example/internal/media/jikkyo/jk1/ws'],
    ['http:', 'ws://tv.example/internal/media/jikkyo/jk1/ws'],
  ])('%s のページからは、同じオリジンの WebSocket につなぐ', (protocol, want) => {
    expect(jikkyoSocketURL('jk1', { protocol, host: 'tv.example' } as Location)).toBe(want);
  });
});

describe('JikkyoSession', () => {
  test('tenban が知らせた状態とコメントを、画面に渡す', () => {
    const { sockets, comments, statuses } = setup();
    expect(sockets).toHaveLength(1);
    expect(sockets[0]!.url).toBe('ws://kotatsu.test/media/jikkyo/jk1/ws');
    expect(statuses).toEqual([{ kind: 'connecting' }]);

    sockets[0]!.emit('open');
    sockets[0]!.emit('message', { type: 'status', kind: 'connected' });
    sockets[0]!.emit('message', {
      type: 'comment',
      content: 'こんにちは',
      mail: '184 shita',
      userId: 'a:b',
    });
    // 書き込まれた時刻があれば、添える (#1515)。
    sockets[0]!.emit('message', {
      type: 'comment',
      content: '時刻つき',
      mail: '184',
      postedAt: 1794647289933,
    });
    sockets[0]!.emit('message', {
      type: 'status',
      kind: 'disconnected',
      reason: 'UPSTREAM',
      retryIn: 5,
    });

    expect(comments).toEqual([
      { content: 'こんにちは', mail: '184 shita', at: 1234, userId: 'a:b' },
      { content: '時刻つき', mail: '184', at: 1234, postedAt: 1794647289933 },
    ]);
    expect(statuses.slice(1)).toEqual([
      { kind: 'connected' },
      { kind: 'disconnected', reason: 'UPSTREAM', retryIn: 5 },
    ]);
    // tenban と NX-Jikkyo の間が切れただけなら、ブラウザはつなぎ直さない。
    expect(sockets).toHaveLength(1);
  });

  test('tenban との接続が切れたら、間隔を延ばしながらつなぎ直し、つながったら戻す', () => {
    const { sockets, timers, statuses } = setup();

    sockets[0]!.emit('close');
    expect(statuses.at(-1)).toEqual({
      kind: 'disconnected',
      reason: 'CONNECTION_CLOSED',
      retryIn: 3,
    });
    expect(timers.at(-1)!.ms).toBe(3000);
    timers.at(-1)!.callback();
    expect(sockets).toHaveLength(2);

    sockets[1]!.emit('error');
    expect(timers.at(-1)!.ms).toBe(5000);
    timers.at(-1)!.callback();
    sockets[2]!.emit('open');
    sockets[2]!.emit('message', { type: 'status', kind: 'connected' });

    sockets[2]!.emit('close');
    expect(timers.at(-1)!.ms).toBe(3000);
  });

  test('投稿は、tenban の結果で成功か失敗かが決まる', async () => {
    const { session, sockets } = setup();
    sockets[0]!.emit('open');

    const posted = session.post('こんにちは');
    expect(sockets[0]!.sent).toEqual([{ type: 'post', id: 1, text: 'こんにちは' }]);
    sockets[0]!.emit('message', { type: 'postResult', id: 1 });
    await expect(posted).resolves.toBeUndefined();

    const rejected = session.post('連投');
    sockets[0]!.emit('message', { type: 'postResult', id: 2, error: 'COMMENT_TOO_FAST' });
    await expect(rejected).rejects.toThrow('COMMENT_TOO_FAST');
  });

  test('つながっていなければ、投稿は失敗する。切れたら、待っている投稿も失敗する', async () => {
    const { session, sockets } = setup();
    await expect(session.post('早すぎ')).rejects.toThrow('not connected');

    sockets[0]!.emit('open');
    const posted = session.post('途中で切れる');
    sockets[0]!.emit('close');
    await expect(posted).rejects.toThrow('disconnected');
  });

  test('閉じた後は、遅れて届いたイベントでつなぎ直さない', () => {
    const { session, sockets, timers, statuses } = setup();
    session.close();
    expect(sockets[0]!.closed).toBe(true);

    sockets[0]!.emit('close');
    expect(timers).toHaveLength(0);
    expect(statuses).toEqual([{ kind: 'connecting' }]);
  });
});
