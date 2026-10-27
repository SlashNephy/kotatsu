import { describe, expect, test } from 'vitest';

import { deinterlacerCanvas } from './deinterlacer';
import { fitDocument, keyGroup, moveMedia, RemoteKey, usedKeyGroups } from './useDataBroadcast';
import classes from './TsPlayer.module.css';

// player は、TsPlayer と同じく、映像の要素 (.media) の中に、yadif の包みと video と字幕の canvas を置いたものを作る。
function player() {
  const media = document.createElement('div');
  media.className = classes.media ?? '';
  const wrapper = document.createElement('div');
  const video = document.createElement('video');
  video.style.filter = 'blur(1px)';
  const canvas = document.createElement('canvas');
  wrapper.append(video);
  media.append(wrapper, canvas);
  document.body.append(media);

  return { media, wrapper, video, canvas };
}

describe('moveMedia', () => {
  test('映像の要素をすべて移し、閉じた shadow root の中でも枠いっぱいに広げる', () => {
    const { media, wrapper, video, canvas } = player();
    const target = document.createElement('div');

    moveMedia(video, target);

    expect(Array.from(target.children)).toEqual([wrapper, canvas]);
    expect(media.childNodes).toHaveLength(0);
    for (const element of [wrapper, video, canvas]) {
      expect(element.style.width).toBe('100%');
      expect(element.style.height).toBe('100%');
    }
    expect(video.style.objectFit).toBe('contain');
  });

  test('yadif の canvas は、yadif が決めた位置と大きさのまま移す', () => {
    const { wrapper, video } = player();
    // yadif は、video と同じ包みの中に、video に合わせて置いた canvas を足す。
    const deinterlaced = document.createElement('canvas');
    deinterlaced.dataset[deinterlacerCanvas] = '';
    deinterlaced.style.cssText = 'position:absolute;left:12px;top:0px;width:320px;height:180px';
    wrapper.append(deinterlaced);
    const target = document.createElement('div');

    const restore = moveMedia(video, target);

    expect(wrapper.parentElement).toBe(target);
    expect(deinterlaced.parentElement).toBe(wrapper);
    expect(deinterlaced.style.left).toBe('12px');
    expect(deinterlaced.style.width).toBe('320px');
    expect(deinterlaced.style.height).toBe('180px');

    // 戻しても、yadif の canvas は包みの中に残り、位置と大きさも変わらない。
    restore();

    expect(deinterlaced.parentElement).toBe(wrapper);
    expect(deinterlaced.style.left).toBe('12px');
    expect(deinterlaced.style.width).toBe('320px');
  });

  test('戻すと、元の並びと、移す前のスタイルに戻す', () => {
    const { media, wrapper, video, canvas } = player();
    const target = document.createElement('div');

    const restore = moveMedia(video, target);
    restore();

    expect(Array.from(media.children)).toEqual([wrapper, canvas]);
    expect(target.childNodes).toHaveLength(0);
    expect(video.style.cssText).toBe('filter: blur(1px);');
    expect(wrapper.style.cssText).toBe('');
  });

  test('切り替えのときに重ねた組 (.slot) があれば、映っている組だけを移す', () => {
    const media = document.createElement('div');
    media.className = classes.media ?? '';
    const [active, pending] = [document.createElement('div'), document.createElement('div')];
    for (const slot of [active, pending]) {
      slot.className = classes.slot ?? '';
      slot.append(document.createElement('video'));
    }
    pending.style.visibility = 'hidden';
    media.append(active, pending);
    document.body.append(media);
    const target = document.createElement('div');

    moveMedia(active.querySelector('video')!, target);

    expect(Array.from(target.children)).toEqual([active]);
    expect(Array.from(media.children)).toEqual([pending]);
    expect(pending.style.visibility).toBe('hidden');
  });

  test('移している間に組を切り替えたら、戻すときは、そのとき移した先にある組を戻す', () => {
    const media = document.createElement('div');
    media.className = classes.media ?? '';
    const [previous, next] = [document.createElement('div'), document.createElement('div')];
    for (const slot of [previous, next]) {
      slot.className = classes.slot ?? '';
      slot.append(document.createElement('video'));
    }
    media.append(previous);
    document.body.append(media);
    const target = document.createElement('div');

    const restore = moveMedia(previous.querySelector('video')!, target);
    // TsPlayer は、切り替えるとき、移した先の前の組を、新しい組と入れ替える。
    previous.replaceWith(next);
    restore();

    expect(Array.from(media.children)).toEqual([next]);
    expect(target.childNodes).toHaveLength(0);
  });
});

describe('fitDocument', () => {
  test.each([
    {
      name: '16:9 の層に 960x540 の文書は、ちょうど収まる',
      layer: { width: 1920, height: 1080 },
      resolution: { width: 960, height: 540 },
      want: { scale: 2, x: 0, y: 0 },
    },
    {
      name: '16:9 の層に 720x480 の文書は、高さに合わせ、左右を空ける',
      layer: { width: 1920, height: 1080 },
      resolution: { width: 720, height: 480 },
      want: { scale: 2.25, x: 150, y: 0 },
    },
    {
      name: '縦に長い全画面では、幅に合わせ、上下を空ける',
      layer: { width: 960, height: 720 },
      resolution: { width: 960, height: 540 },
      want: { scale: 1, x: 0, y: 90 },
    },
    {
      name: '大きさがまだ決まっていない層では、拡大しない',
      layer: { width: 0, height: 0 },
      resolution: { width: 960, height: 540 },
      want: { scale: 1, x: 0, y: 0 },
    },
  ])('$name', ({ layer, resolution, want }) => {
    expect(fitDocument(layer, resolution)).toEqual(want);
  });
});

describe('usedKeyGroups', () => {
  test.each([
    { name: '宣言がなければ、basic と data-button', list: [], want: ['basic', 'data-button'] },
    { name: '宣言したグループだけ', list: ['numeric-tuning'], want: ['numeric-tuning'] },
    {
      name: 'リモコンにないグループは除く',
      list: ['basic', 'special-1'],
      want: ['basic'],
    },
  ])('$name', ({ list, want }) => {
    expect([...usedKeyGroups(new Set(list))].sort()).toEqual([...want].sort());
  });
});

describe('keyGroup', () => {
  test.each([
    { key: RemoteKey.up, want: 'basic' },
    { key: RemoteKey.enter, want: 'basic' },
    { key: RemoteKey.back, want: 'basic' },
    { key: RemoteKey.blue, want: 'data-button' },
    { key: RemoteKey.yellow, want: 'data-button' },
    { key: RemoteKey.digit0, want: 'numeric-tuning' },
    { key: RemoteKey.digit12, want: 'numeric-tuning' },
  ])('キー $key は $want', ({ key, want }) => {
    expect(keyGroup(key)).toBe(want);
  });
});
