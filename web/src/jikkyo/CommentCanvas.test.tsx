import { render, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { describe, expect, test, vi } from 'vitest';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import { defaultCommentSettings } from './comments';

// renderers は、作った描画と、渡した設定とコメント。
const renderers = vi.hoisted(
  () =>
    [] as {
      options: { config?: Record<string, unknown> };
      contents: string[];
      destroyed: boolean;
    }[],
);

vi.mock('@xpadev-net/niconicomments', () => ({
  default: class {
    state: (typeof renderers)[number];
    constructor(
      _canvas: unknown,
      _comments: unknown,
      options: (typeof renderers)[number]['options'],
    ) {
      this.state = { options, contents: [], destroyed: false };
      renderers.push(this.state);
    }
    addComments(...comments: { content: string }[]) {
      this.state.contents.push(...comments.map((comment) => comment.content));
    }
    drawCanvas() {}
    clear() {}
    destroy() {
      this.state.destroyed = true;
    }
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

  // #488
  test('設定を変えたら描画を作り直し、受け取ったコメントを、ミュートしたものを除いて足し直す', async () => {
    renderers.length = 0;
    const ref = createRef<CommentCanvasHandle>();
    const { rerender } = render(<CommentCanvas ref={ref} clock={() => 0} drawing />);
    await waitFor(() => expect(renderers).toHaveLength(1));
    ref.current?.add([
      { time: 0, content: 'わこつ', mail: '' },
      { time: 1, content: 'ネタバレ注意', mail: '', userId: 'a:x' },
    ]);
    expect(renderers[0]?.contents).toEqual(['わこつ', 'ネタバレ注意']);

    const settings = {
      ...defaultCommentSettings,
      scale: 1.5,
      showAll: false,
      mutedKeywords: [{ text: 'ネタバレ', match: 'PARTIAL' }],
    };
    rerender(<CommentCanvas ref={ref} clock={() => 0} drawing settings={settings} />);
    await waitFor(() => expect(renderers).toHaveLength(2));
    expect(renderers[0]?.destroyed).toBe(true);
    expect(renderers[1]?.contents).toEqual(['わこつ']);
    expect(renderers[1]?.options.config).toEqual({
      commentScale: { html5: (1920 / 683) * 1.5, flash: (1920 / 683) * 1.5 },
      commentLimit: 40,
    });
  });
});
