import type { ConcreteRequest } from 'relay-runtime';
import { describe, expect, it } from 'vitest';

import RecordingIdDeleteMutation from '../routes/recordings/__generated__/RecordingIdDeleteMutation.graphql';
import RecordingIdPlayQuery from '../routes/recordings/__generated__/RecordingIdPlayQuery.graphql';
import RecordingIdProtectMutation from '../routes/recordings/__generated__/RecordingIdProtectMutation.graphql';
import RecordingIdQuery from '../routes/recordings/__generated__/RecordingIdQuery.graphql';
import RecordingIdReanalyzeMutation from '../routes/recordings/__generated__/RecordingIdReanalyzeMutation.graphql';
import RecordingIdRegenerateMutation from '../routes/recordings/__generated__/RecordingIdRegenerateMutation.graphql';
import RecordingIdTagsMutation from '../routes/recordings/__generated__/RecordingIdTagsMutation.graphql';
import recordingsDeleteMutation from '../routes/recordings/__generated__/recordingsDeleteMutation.graphql';
import recordingsListQuery from '../routes/recordings/__generated__/recordingsListQuery.graphql';
import recordingsTagsQuery from '../routes/recordings/__generated__/recordingsTagsQuery.graphql';
import ServiceIdLiveQuery from '../routes/live/__generated__/ServiceIdLiveQuery.graphql';
import ServiceIdRecordingsQuery from '../routes/live/__generated__/ServiceIdRecordingsQuery.graphql';
import ExternalStreamURLMutation from '../streams/__generated__/ExternalStreamURLMutation.graphql';
import usePlayerSettingsMutation from '../yunomi/__generated__/usePlayerSettingsMutation.graphql';
import usePlayerSettingsQuery from '../yunomi/__generated__/usePlayerSettingsQuery.graphql';
import useRemotePlayerRegisterMutation from '../yunomi/__generated__/useRemotePlayerRegisterMutation.graphql';
import useRemotePlayerUnregisterMutation from '../yunomi/__generated__/useRemotePlayerUnregisterMutation.graphql';
import useRemotePlayerUpdateMutation from '../yunomi/__generated__/useRemotePlayerUpdateMutation.graphql';
import useWatchPositionSaverMutation from '../yunomi/__generated__/useWatchPositionSaverMutation.graphql';
import { createRootValue } from './data';
import { createEventHub } from './events';
import { createExecutor, type Variables } from './executor';

const now = Date.parse('2026-10-19T12:00:00Z');
const at = (offset: number) => new Date(now + offset).toISOString();

function newExecutor() {
  return createExecutor(createRootValue(createEventHub(), now));
}

const text = (request: ConcreteRequest) => request.params.text ?? '';

type RecordingNode = { id: string; name: string; state: string; protected: boolean };

async function listRecordings(
  executor: ReturnType<typeof newExecutor>,
  filter: object | null = null,
) {
  const result = await executor.execute(
    `query ($filter: RecordingFilter) {
      recordings(first: 100, filter: $filter) { edges { node { id name state protected } } }
    }`,
    { filter },
  );
  expect(result.errors).toBeUndefined();

  return (result.data as { recordings: { edges: { node: RecordingNode }[] } }).recordings.edges.map(
    (edge) => edge.node,
  );
}

const playerState = {
  kind: 'RECORDING',
  target: '',
  paused: false,
  volume: 1,
  muted: false,
  audios: [{ id: '0', label: '日本語' }],
  dataBroadcast: false,
  keyGroups: [],
};

describe('demo executor answers the queries of the recording and player screens', () => {
  const cases: {
    name: string;
    request: ConcreteRequest;
    variables: (ids: { recording: string; removable: string }) => Variables;
  }[] = [
    {
      name: 'recordings',
      request: recordingsListQuery,
      variables: () => ({ first: 50, after: null, filter: null, order: 'NEWEST_FIRST' }),
    },
    {
      name: 'tags',
      request: recordingsTagsQuery,
      variables: () => ({ ruleId: 'Rule:1', withRule: true }),
    },
    {
      name: 'recording',
      request: RecordingIdQuery,
      variables: ({ recording }) => ({ id: recording }),
    },
    {
      name: 'play',
      request: RecordingIdPlayQuery,
      variables: ({ recording }) => ({ id: recording }),
    },
    {
      name: 'protect',
      request: RecordingIdProtectMutation,
      variables: ({ recording }) => ({ id: recording, protected: true }),
    },
    {
      name: 'set tags',
      request: RecordingIdTagsMutation,
      variables: ({ recording }) => ({ id: recording, tags: ['お気に入り'] }),
    },
    {
      name: 'reanalyze',
      request: RecordingIdReanalyzeMutation,
      variables: ({ recording }) => ({ id: recording }),
    },
    {
      name: 'regenerate',
      request: RecordingIdRegenerateMutation,
      variables: ({ recording }) => ({ id: recording }),
    },
    {
      name: 'delete',
      request: RecordingIdDeleteMutation,
      variables: ({ removable }) => ({ id: removable }),
    },
    {
      name: 'delete many',
      request: recordingsDeleteMutation,
      variables: ({ removable }) => ({ ids: [removable] }),
    },
    {
      name: 'save position',
      request: useWatchPositionSaverMutation,
      variables: ({ recording }) => ({ id: recording, positionMs: 60_000 }),
    },
    {
      name: 'live',
      request: ServiceIdLiveQuery,
      variables: () => ({ id: 'Service:1024', from: at(0), to: at(60 * 60 * 1000) }),
    },
    {
      name: 'recordings while the tuner is taken',
      request: ServiceIdRecordingsQuery,
      variables: () => ({}),
    },
    {
      name: 'stream URL',
      request: ExternalStreamURLMutation,
      variables: ({ recording }) => ({ target: recording }),
    },
    { name: 'player settings', request: usePlayerSettingsQuery, variables: () => ({}) },
    {
      name: 'update player settings',
      request: usePlayerSettingsMutation,
      variables: () => ({
        input: {
          jikkyoEnabled: true,
          comments: {
            opacity: 0.5,
            speed: 1,
            scale: 1,
            showAll: false,
            mutedKeywords: [],
            mutedUsers: [],
          },
        },
      }),
    },
    {
      name: 'register player',
      request: useRemotePlayerRegisterMutation,
      variables: ({ recording }) => ({ state: { ...playerState, target: recording } }),
    },
    {
      name: 'update player',
      request: useRemotePlayerUpdateMutation,
      variables: ({ recording }) => ({
        id: 'Player:1',
        state: { ...playerState, target: recording },
        announce: false,
      }),
    },
    {
      name: 'unregister player',
      request: useRemotePlayerUnregisterMutation,
      variables: () => ({ id: 'Player:1' }),
    },
  ];

  it.each(cases)('$name', async ({ request, variables }) => {
    const executor = newExecutor();
    const list = await listRecordings(executor);
    const ids = {
      recording: list[0]!.id,
      removable: list.find((item) => !item.protected && item.state !== 'RECORDING')!.id,
    };

    const result = await executor.execute(text(request), variables(ids));

    expect(result.errors).toBeUndefined();
    expect(result.data).not.toBeNull();
  });
});

describe('demo recordings', () => {
  it('starts with finished recordings, one recording on the air, and one partly watched', async () => {
    const executor = newExecutor();

    const list = await listRecordings(executor);
    const watching = await executor.execute('{ continueWatching(first: 12) { id } }', {});

    expect(list.length).toBeGreaterThan(5);
    expect(list.filter((item) => item.state === 'RECORDING')).toHaveLength(1);
    expect((watching.data as { continueWatching: unknown[] }).continueWatching).toHaveLength(1);
  });

  it('refuses to delete a protected recording', async () => {
    const executor = newExecutor();
    const target = (await listRecordings(executor)).find((item) => item.protected)!;

    const result = await executor.execute('mutation ($id: ID!) { deleteRecording(id: $id) }', {
      id: target.id,
    });

    expect(result.errors?.[0]?.message).toBe('the recording is protected or still recording');
    expect((await listRecordings(executor)).some((item) => item.id === target.id)).toBe(true);
  });

  it('creates tags on demand, and narrows the list by a tag', async () => {
    const executor = newExecutor();
    const target = (await listRecordings(executor))[0]!;

    await executor.execute(
      'mutation ($id: ID!, $tags: [String!]!) { setRecordingTags(id: $id, tags: $tags) { id } }',
      {
        id: target.id,
        tags: ['お気に入り', ' お気に入り '],
      },
    );
    const tags = await executor.execute('{ tags { id name recordings } }', {});
    const favorite = (
      tags.data as { tags: { id: string; name: string; recordings: number }[] }
    ).tags.find((tag) => tag.name === 'お気に入り')!;

    expect(favorite.recordings).toBe(1);
    expect(
      (await listRecordings(executor, { tagIds: [favorite.id] })).map((item) => item.id),
    ).toEqual([target.id]);
  });

  it('lists a recording in continue watching after its position is saved', async () => {
    const executor = newExecutor();
    const target = (await listRecordings(executor)).find((item) => item.state === 'COMPLETED')!;

    await executor.execute(text(useWatchPositionSaverMutation), {
      id: target.id,
      positionMs: 120_000,
    });
    const watching = await executor.execute('{ continueWatching(first: 12) { id } }', {});

    expect((watching.data as { continueWatching: { id: string }[] }).continueWatching[0]?.id).toBe(
      target.id,
    );
  });

  it('lists recordings from the oldest when asked', async () => {
    const executor = newExecutor();

    const result = await executor.execute(
      'query { recordings(first: 100, order: OLDEST_FIRST) { edges { node { startAt } } } }',
      {},
    );

    const starts = (
      result.data as { recordings: { edges: { node: { startAt: string } }[] } }
    ).recordings.edges.map((edge) => Date.parse(edge.node.startAt));
    expect(starts).toEqual(starts.toSorted((a, b) => a - b));
  });

  it('skips protected, recording, and unknown recordings when deleting many', async () => {
    const executor = newExecutor();
    const list = await listRecordings(executor);
    const removable = list.find((item) => !item.protected && item.state !== 'RECORDING')!;
    const kept = [
      list.find((item) => item.protected)!.id,
      list.find((item) => item.state === 'RECORDING')!.id,
      'Recording:unknown',
    ];

    const result = await executor.execute(
      'mutation ($ids: [ID!]!) { deleteRecordings(ids: $ids) { processed skipped } }',
      { ids: [removable.id, ...kept] },
    );

    expect(result.data).toEqual({ deleteRecordings: { processed: 1, skipped: 3 } });
    const after = (await listRecordings(executor)).map((item) => item.id);
    expect(after).not.toContain(removable.id);
    expect(after).toEqual(expect.arrayContaining(kept.slice(0, 2)));
  });
});
