import { describe, expect, test } from 'vitest';

import { captionStartSeconds } from './captionMatches';

describe('captionStartSeconds', () => {
  const recordedFrom = '2026-11-09T14:00:00Z';

  test.each([
    { name: '途切れのない録画', gaps: [], from: '2026-11-09T14:01:05.700Z', want: 65 },
    {
      name: '前の途切れた分を詰める',
      gaps: [{ from: '2026-11-09T14:00:30Z', to: '2026-11-09T14:00:50Z' }],
      from: '2026-11-09T14:01:05Z',
      want: 45,
    },
    {
      name: '後ろの途切れは詰めない',
      gaps: [{ from: '2026-11-09T14:02:00Z', to: '2026-11-09T14:03:00Z' }],
      from: '2026-11-09T14:01:05Z',
      want: 65,
    },
    {
      name: '途切れの中に始まる字幕は、途切れの始まりから',
      gaps: [{ from: '2026-11-09T14:00:30Z', to: '2026-11-09T14:00:50Z' }],
      from: '2026-11-09T14:00:40Z',
      want: 30,
    },
    {
      name: '録り始めより前の字幕は、録画の始まりから',
      gaps: [],
      from: '2026-11-09T13:59:58Z',
      want: 0,
    },
  ])('$name', ({ gaps, from, want }) => {
    expect(captionStartSeconds(recordedFrom, gaps, from)).toBe(want);
  });
});
