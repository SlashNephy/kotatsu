import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { NXJikkyoSession, type LiveComment, type SessionStatus } from '../jikkyo/session';
import { demoRecordedComments, FakeJikkyoSocket } from './jikkyo';

describe('FakeJikkyoSocket', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  // 偽物の WebSocket が、画面の実況のセッションが求める手順に沿い、架空のコメントを流せるかを確かめる。
  test('drives NXJikkyoSession to connected and streams comments', async () => {
    const sockets: FakeJikkyoSocket[] = [];
    const statuses: SessionStatus[] = [];
    const comments: LiveComment[] = [];
    const session = new NXJikkyoSession(
      'demo1024',
      {
        onComment: (comment) => comments.push(comment),
        onStatus: (status) => statuses.push(status),
      },
      {
        createSocket: (url) => {
          const socket = new FakeJikkyoSocket(url);
          sockets.push(socket);

          return socket;
        },
      },
    );

    await vi.advanceTimersByTimeAsync(5000);

    expect(sockets.map((socket) => socket.url)).toEqual([
      'wss://nx-jikkyo.tsukumijima.net/api/v1/channels/demo1024/ws/watch',
      'wss://nx-jikkyo.tsukumijima.net/api/v1/channels/demo1024/ws/comment',
    ]);
    expect(statuses).toEqual([{ kind: 'connecting' }, { kind: 'connected' }]);
    expect(comments.length).toBeGreaterThanOrEqual(3);
    expect(comments[0]).toMatchObject({ content: 'おはよう', userId: 'demo-user-0' });

    const posted = session.post('こんにちは');
    await vi.advanceTimersByTimeAsync(0);
    await expect(posted).resolves.toBeUndefined();

    // 閉じたら、コメントは届かなくなる。
    session.close();
    const count = comments.length;
    await vi.advanceTimersByTimeAsync(5000);
    expect(comments).toHaveLength(count);
  });
});

describe('demoRecordedComments', () => {
  test('spreads comments over the demo video in time order', () => {
    const comments = demoRecordedComments(20);

    expect(comments.length).toBeGreaterThan(10);
    expect(comments.every((comment) => comment.time >= 0 && comment.time < 20)).toBe(true);
    expect(comments.map((comment) => comment.time)).toEqual(
      [...comments].map((comment) => comment.time).sort((a, b) => a - b),
    );
  });
});
