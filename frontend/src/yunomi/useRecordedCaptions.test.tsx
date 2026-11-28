import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';

import { useRecordedCaptions } from './useRecordedCaptions';

describe('useRecordedCaptions', () => {
  test.each([
    {
      name: '字幕の文が出たら、キャプチャに書き込む文にする',
      text: 'こんばんは',
      want: 'こんばんは',
    },
    { name: '字幕を消したら (空の文)、キャプチャに書き込む文をなくす', text: '', want: null },
  ])('$name', ({ text, want }) => {
    const { result } = renderHook(() => useRecordedCaptions(() => 12));

    act(() => result.current.onCaptionText('前の字幕'));
    act(() => result.current.onCaptionText(text));

    expect(result.current.currentCaption()).toBe(want);
  });
});
