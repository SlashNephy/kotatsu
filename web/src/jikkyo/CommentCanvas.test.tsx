import { render } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';

vi.mock('@xpadev-net/niconicomments', () => ({
  default: class {
    addComments() {}
    drawCanvas() {}
    clear() {}
    destroy() {}
  },
}));

describe('CommentCanvas', () => {
  // ref のハンドルが描画のたびに変わると、ハンドルを受け取った側がコメントを足し直し、同じコメントが重なって流れる。
  test('描画し直しても、ref に渡すハンドルを作り直さない', () => {
    const handles: (CommentCanvasHandle | null)[] = [];
    const ref = (handle: CommentCanvasHandle | null) => {
      handles.push(handle);
    };
    const { rerender } = render(<CommentCanvas ref={ref} clock={() => 0} drawing />);
    // 再生位置を返す関数は、親が描画し直すたびに新しくなる。
    rerender(<CommentCanvas ref={ref} clock={() => 1} drawing />);
    rerender(<CommentCanvas ref={ref} clock={() => 2} drawing />);

    expect(handles.filter((handle) => handle !== null)).toHaveLength(1);
  });
});
