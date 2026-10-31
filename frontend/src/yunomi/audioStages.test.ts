import { describe, expect, test } from 'vitest';

import { audioStageChain, type AudioStageName } from './audioStages';

describe('audioStageChain', () => {
  test.each([
    {
      name: '入れた順によらず、決まった順に並べる',
      enabled: ['normalize', 'surround', 'delay'],
      want: ['delay', 'surround', 'normalize'],
    },
    { name: '入れていない段は飛ばす', enabled: ['normalize'], want: ['normalize'] },
    { name: '何も入れていなければ空', enabled: [], want: [] },
  ] satisfies { name: string; enabled: AudioStageName[]; want: AudioStageName[] }[])(
    '$name',
    ({ enabled, want }) => {
      expect(audioStageChain(new Set(enabled))).toEqual(want);
    },
  );
});
