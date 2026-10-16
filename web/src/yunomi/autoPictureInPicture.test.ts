import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  autoPictureInPictureSupported,
  loadAutoPictureInPicture,
  saveAutoPictureInPicture,
} from './autoPictureInPicture';

function memoryStorage() {
  const items = new Map<string, string>();

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
  delete (window as { documentPictureInPicture?: unknown }).documentPictureInPicture;
});

describe('autoPictureInPicture', () => {
  test('覚えた設定がなければ、自動で出さない', () => {
    expect(loadAutoPictureInPicture(memoryStorage())).toBe(false);
    expect(loadAutoPictureInPicture(null)).toBe(false);
  });

  test.each([true, false])('%s を覚えたら、次に読める', (enabled) => {
    const storage = memoryStorage();
    saveAutoPictureInPicture(storage, enabled);

    expect(loadAutoPictureInPicture(storage)).toBe(enabled);
  });
});

describe('autoPictureInPictureSupported', () => {
  test.each([
    { name: 'Document PiP がなければ、使えない', documentPiP: false, accepts: true, want: false },
    {
      name: 'enterpictureinpicture を受け付けなければ、使えない',
      documentPiP: true,
      accepts: false,
      want: false,
    },
    { name: 'どちらもあれば、使える', documentPiP: true, accepts: true, want: true },
  ])('$name', ({ documentPiP, accepts, want }) => {
    if (documentPiP) {
      (window as { documentPictureInPicture?: unknown }).documentPictureInPicture = {};
    }
    vi.stubGlobal('navigator', {
      mediaSession: {
        setActionHandler: (action: string) => {
          if (!accepts && action === 'enterpictureinpicture') {
            throw new TypeError(`unknown action ${action}`);
          }
        },
      },
    });

    expect(autoPictureInPictureSupported()).toBe(want);
  });
});
