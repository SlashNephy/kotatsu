import type NiconiComments from '@xpadev-net/niconicomments';
import { useEffect, useImperativeHandle, useMemo, useRef, type Ref } from 'react';

import classes from './CommentCanvas.module.css';
import {
  compileMute,
  defaultCommentSettings,
  limitedComments,
  shouldDraw,
  toFormatted,
  type CommentSettings,
  type JikkyoComment,
} from './comments';

// baseCommentScale は、niconicomments の既定の commentScale (1920 / 683)。文字の大きさの倍率は、これに掛ける。
const baseCommentScale = 1920 / 683;

export type CommentCanvasHandle = {
  // add は、コメントを足す。コメントの time は、clock と同じ時計の秒。
  add: (comments: JikkyoComment[]) => void;
};

type Props = {
  // clock は、今の時刻 (秒) を返す。ライブ視聴では経過時間、録画の再生では録画の始まりからの再生位置。
  clock: () => number;
  // drawing は、コメントを描くかどうか。隠れている間や、実況を切っている間は描かない。
  drawing: boolean;
  // settings は、コメントの表示とミュートの設定 (#488)。省けば既定値。
  settings?: CommentSettings;
  // retainSeconds を渡すと、今より retainSeconds 秒より前のコメントは、設定を変えて描き直すときに足し直さない。
  // ライブ視聴では、流れ終わったコメントを、見ている間ずっと持ち続けないようにする。録画では、戻って見るので省く。
  retainSeconds?: number;
  ref?: Ref<CommentCanvasHandle>;
};

// CommentCanvas は、実況のコメントを niconicomments で映像の上に流す (設計書 7.4)。
export function CommentCanvas({
  clock,
  drawing,
  settings = defaultCommentSettings,
  retainSeconds,
  ref,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const rendererRef = useRef<NiconiComments | null>(null);
  // received は、受け取ったコメントと、niconicomments に渡す ID。設定を変えたときに、作り直した描画に足し直す。
  const received = useRef<{ comment: JikkyoComment; id: number }[]>([]);
  const nextID = useRef(0);
  const retainRef = useRef(retainSeconds);
  retainRef.current = retainSeconds;
  const clockRef = useRef(clock);
  clockRef.current = clock;
  // 描画を作り直すのは、描き方とミュートが変わったときだけにする。不透明度は canvas の style で変える。
  // ミュートの一覧は、ほかの設定を変えただけでも新しい配列で届くので、中身で比べる。
  const { speed, scale, showAll, mutedKeywords, mutedUsers } = settings;
  const muteKey = JSON.stringify([mutedKeywords, mutedUsers]);
  const view = useMemo(() => {
    const [keywords, users] = JSON.parse(muteKey) as [CommentSettings['mutedKeywords'], string[]];

    return {
      speed,
      scale,
      showAll,
      muted: compileMute({ ...defaultCommentSettings, mutedKeywords: keywords, mutedUsers: users }),
    };
  }, [speed, scale, showAll, muteKey]);
  const viewRef = useRef(view);
  viewRef.current = view;

  const draw = (
    renderer: NiconiComments,
    entries: readonly { comment: JikkyoComment; id: number }[],
  ) => {
    const { speed: currentSpeed, muted } = viewRef.current;
    const formatted = entries.flatMap(({ comment, id }) =>
      shouldDraw(comment) && !muted(comment) ? [toFormatted(comment, id, currentSpeed)] : [],
    );
    if (formatted.length > 0) {
      renderer.addComments(...formatted);
    }
  };
  const add = (comments: JikkyoComment[]) => {
    const entries = comments.map((comment) => ({ comment, id: nextID.current++ }));
    const retain = retainRef.current;
    if (retain !== undefined) {
      const since = clockRef.current() - retain;
      received.current = received.current.filter(({ comment }) => comment.time >= since);
    }
    received.current.push(...entries);
    const renderer = rendererRef.current;
    if (renderer !== null) {
      draw(renderer, entries);
    }
  };
  // ハンドルは作り直さない。作り直すと、受け取った側がコメントを足し直し、同じコメントが重なって流れる。
  // add は ref だけを使うので、最初のハンドルのままでよい。
  useImperativeHandle(ref, () => ({ add }), []);

  // niconicomments は大きいので、実況を出すときに読む。設定を変えたら作り直し、受け取ったコメントを足し直す。
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
        config: {
          commentScale: {
            html5: baseCommentScale * view.scale,
            flash: baseCommentScale * view.scale,
          },
          // すべては出さないなら、同時に出すコメントを減らし、古いものから隠す。
          ...(view.showAll ? {} : { commentLimit: limitedComments }),
        },
      });
      draw(rendererRef.current, received.current);
    });

    return () => {
      destroyed = true;
      rendererRef.current?.destroy();
      rendererRef.current = null;
    };
    // draw は viewRef を読むので、依存に入れない。
  }, [view]);

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

  // data-overlay は、キャプチャで写す canvas の印。
  return (
    <canvas
      ref={canvasRef}
      className={classes.canvas}
      style={{ opacity: settings.opacity }}
      width={1920}
      height={1080}
      data-overlay="comments"
    />
  );
}
