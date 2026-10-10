import type NiconiComments from '@xpadev-net/niconicomments';
import { useEffect, useImperativeHandle, useRef, type Ref } from 'react';

import classes from './CommentCanvas.module.css';
import { shouldDraw, toFormatted, type JikkyoComment } from './comments';

export type CommentCanvasHandle = {
  // add は、コメントを足す。コメントの time は、clock と同じ時計の秒。
  add: (comments: JikkyoComment[]) => void;
};

type Props = {
  // clock は、今の時刻 (秒) を返す。ライブ視聴では経過時間、録画の再生では録画の始まりからの再生位置。
  clock: () => number;
  // drawing は、コメントを描くかどうか。隠れている間や、実況を切っている間は描かない。
  drawing: boolean;
  ref?: Ref<CommentCanvasHandle>;
};

// CommentCanvas は、実況のコメントを niconicomments で映像の上に流す (設計書 7.4)。
export function CommentCanvas({ clock, drawing, ref }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const rendererRef = useRef<NiconiComments | null>(null);
  // pending は、niconicomments を読み終える前に届いたコメント。
  const pending = useRef<JikkyoComment[]>([]);
  const nextID = useRef(0);
  const clockRef = useRef(clock);
  clockRef.current = clock;

  const add = (comments: JikkyoComment[]) => {
    const renderer = rendererRef.current;
    if (renderer === null) {
      pending.current.push(...comments);

      return;
    }
    const formatted = comments
      .filter(shouldDraw)
      .map((comment) => toFormatted(comment, nextID.current++));
    if (formatted.length > 0) {
      renderer.addComments(...formatted);
    }
  };
  useImperativeHandle(ref, () => ({ add }));

  // niconicomments は大きいので、実況を出すときに読む。
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) {
      return;
    }
    let destroyed = false;
    void import('@xpadev-net/niconicomments').then(({ default: Renderer }) => {
      if (destroyed) {
        return;
      }
      rendererRef.current = new Renderer(canvas, [], {
        format: 'formatted',
        mode: 'default',
        keepCA: true,
      });
      const queued = pending.current;
      pending.current = [];
      add(queued);
    });

    return () => {
      destroyed = true;
      rendererRef.current?.destroy();
      rendererRef.current = null;
    };
    // add は描画の準備を終えたときに一度だけ使うので、依存に入れない。
  }, []);

  // 画面の更新に合わせて描く。video の timeupdate は間隔が粗く、コメントがかくつく。
  useEffect(() => {
    if (!drawing) {
      rendererRef.current?.clear();

      return;
    }
    let frame = 0;
    const draw = () => {
      rendererRef.current?.drawCanvas(Math.round(clockRef.current() * 100));
      frame = requestAnimationFrame(draw);
    };
    frame = requestAnimationFrame(draw);

    return () => cancelAnimationFrame(frame);
  }, [drawing]);

  return <canvas ref={canvasRef} className={classes.canvas} width={1920} height={1080} />;
}
