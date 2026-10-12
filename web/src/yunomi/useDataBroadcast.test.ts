import { describe, expect, test } from 'vitest';

import { deinterlacerCanvas } from './deinterlacer';
import { moveMedia } from './useDataBroadcast';
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
});
