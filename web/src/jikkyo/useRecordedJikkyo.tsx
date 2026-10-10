import { useEffect, useRef, useState, type ReactNode } from 'react';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import type { JikkyoComment } from './comments';

type RecordedComments = {
  channel: { id: string } | null;
  comments: JikkyoComment[];
};

export type RecordedJikkyoState =
  | { kind: 'loading' }
  | { kind: 'loaded'; count: number }
  | { kind: 'error' };

// useRecordedJikkyo は、録画の再生で、録画の時間帯の実況の過去ログを流す (設計書 7.4)。
// 過去ログは、tenban が過去ログ API から取って中継する。url が null なら、何もしない。
// clock は、録画の始まりからの再生位置 (秒) を返す。過去ログの時刻も、録画の始まりからの秒。
export function useRecordedJikkyo(
  url: string | null,
  clock: () => number,
  drawing: boolean,
): { overlay: ReactNode; state: RecordedJikkyoState | null } {
  const canvas = useRef<CommentCanvasHandle | null>(null);
  const [state, setState] = useState<RecordedJikkyoState | null>(null);

  useEffect(() => {
    if (url === null) {
      setState(null);

      return;
    }
    const controller = new AbortController();
    setState({ kind: 'loading' });
    void fetch(url, { signal: controller.signal, credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`unexpected status ${response.status}`);
        }

        return (await response.json()) as RecordedComments;
      })
      .then((body) => {
        canvas.current?.add(body.comments);
        setState({ kind: 'loaded', count: body.comments.length });
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setState({ kind: 'error' });
        }
      });

    return () => controller.abort();
  }, [url]);

  return {
    overlay:
      url === null ? undefined : (
        <CommentCanvas key={url} ref={canvas} clock={clock} drawing={drawing} />
      ),
    state,
  };
}
