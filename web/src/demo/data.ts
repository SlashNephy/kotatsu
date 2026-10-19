import type { EventHub } from './events';

// demoUser は、デモでログインしているものとして扱うユーザー。
const demoUser = {
  __typename: 'User',
  id: 'User:demo',
  username: 'demo',
  displayName: 'デモ',
  role: 'ADMIN',
  disabled: false,
  createdAt: '2026-10-01T00:00:00Z',
  avatarUrl: null,
};

const emptyPageInfo = {
  hasNextPage: false,
  hasPreviousPage: false,
  startCursor: null,
  endCursor: null,
};

// createRootValue は、デモの GraphQL の rootValue を作る。Query、Mutation、Subscription のフィールドを 1 つにまとめて持つ。
// 番組や録画のデータは、対象の画面ごとに足していく (#857)。
export function createRootValue(events: EventHub): object {
  return {
    // Query
    viewer: () => demoUser,
    version: () => ({ version: 'demo', revision: null, modified: false, latestRelease: null }),
    timeZone: () => 'Asia/Tokyo',
    tunerServerHealth: () => ({
      degraded: false,
      degradedSince: null,
      clockSkewed: false,
      clockSkewMs: null,
    }),
    channelTypes: () => [
      { type: 'GR', displayName: '地上波', sortOrder: 0 },
      { type: 'BS', displayName: 'BS', sortOrder: 1 },
    ],
    guide: () => [],
    continueWatching: () => [],
    recordings: () => ({ edges: [], pageInfo: emptyPageInfo }),
    reservations: () => ({ edges: [], pageInfo: emptyPageInfo }),

    // Subscription。graphql-js は、流れた値を rootValue にして選択集合を解くので、フィールド名で包む。
    viewerEvents: () => events.listen((event) => ({ viewerEvents: event })),
  };
}
