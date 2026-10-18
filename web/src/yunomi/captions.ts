import type { CanvasMainThreadRenderer, Controller, MPEGTSFeeder, TextRenderer } from 'aribb24.js';

import { broadcastFontUrl } from './broadcastFont';

// Captions は、字幕と文字スーパーを、映像の上に描く (設計書 7.4)。
// ブラウザの中で変換する経路では、プレイヤーが渡す private PES を、kyusu の経路では、hls.js が取り出す ID3 を渡す。
// どちらも、時刻は video の時刻 (秒) にそろえて渡す。
export type Captions = {
  feedB24: (data: ArrayBuffer, pts: number) => void;
  feedID3: (data: Uint8Array, pts: number) => void;
  setVisible: (visible: boolean) => void;
  // reset は、受け取った字幕を捨てる。読み直すと video の時刻の基準が変わるので、前の読み込みの字幕を出さないようにする。
  reset: () => void;
  destroy: () => void;
};

type Layer = {
  controller: Controller;
  feeder: MPEGTSFeeder;
  renderer: CanvasMainThreadRenderer;
  // text は、字幕の文を取り出す描画器。字幕の層にだけ付ける (#450)。
  text: TextRenderer | null;
};

// textRecorder は、aribb24.js の TextRenderer を広げ、字幕を描くたびに、その文を onText に渡す描画器を作る。
// aribb24.js は、字幕の時刻 (PTS) に合わせて、付けた描画器すべての render を呼ぶので、時刻を自前で合わせなくてよい。
// 消すとき (clear) は、空の文を渡す。今出ている字幕をキャプチャに書き込むときに、消えたことを知るためである (#458)。
function textRecorder(Base: typeof TextRenderer, onText: (text: string) => void): TextRenderer {
  class Recorder extends Base {
    override render(...args: Parameters<TextRenderer['render']>): void {
      super.render(...args);
      onText(this.getText());
    }

    override clear(): void {
      super.clear();
      onText('');
    }
  }

  return new Recorder();
}

type Feed =
  | { kind: 'b24'; data: ArrayBuffer; pts: number }
  | { kind: 'id3'; data: Uint8Array; pts: number };

// maxQueued は、aribb24.js を読み終えるまでに溜めておく字幕の数の上限。1 秒に数個しか届かないので、読み終えるまでには足りる。
const maxQueued = 256;

// captionFontFamily は、字幕を描くフォント (Rounded M+ 1m WadaLab mix) の名前。
const captionFontFamily = 'Rounded M+ 1m WadaLab mix';
// captionFont は、字幕のフォントの並び。読み込めなかったときは、aribb24.js の既定の並びで描く。
const captionFont = `'${captionFontFamily}', 'Hiragino Maru Gothic Pro', 'BIZ UDGothic', 'Yu Gothic Medium', sans-serif`;

let captionFontLoading: Promise<void> | null = null;

// loadCaptionFont は、字幕のフォントを読み込む。canvas に描くので、描く前に読み終えておく。
// 7 MB ほどあるので、字幕を描くときに初めて読み、ページの中では 1 回だけ読む。読めなくても失敗にはしない。
function loadCaptionFont(): Promise<void> {
  if (captionFontLoading === null) {
    captionFontLoading =
      typeof FontFace === 'undefined'
        ? Promise.resolve()
        : new FontFace(captionFontFamily, `url(${broadcastFontUrl})`).load().then(
            (face) => {
              document.fonts.add(face);
            },
            () => {},
          );
  }

  return captionFontLoading;
}

// createCaptions は、video の上に字幕を描く。描画用の canvas は container に置くので、container は position: relative にする。
// aribb24.js と字幕のフォントは大きいので、ここから読み始める。読み終えるまでに届いた字幕は溜めておき、読み終えたら描く。
// 読めなかったときは、字幕なしで再生を続ける。
// onText は、字幕 (文字スーパーは除く) が出るたびに、その文を受け取る (#450)。字幕が消えたら、空の文を受け取る。
export function createCaptions(
  video: HTMLVideoElement,
  container: HTMLElement,
  onText?: (text: string) => void,
): Captions {
  let layers: Layer[] | null = null;
  let queue: Feed[] = [];
  let visible = true;
  let destroyed = false;

  const feed = (layer: Layer, item: Feed) => {
    if (item.kind === 'b24') {
      layer.feeder.feedB24(item.data, item.pts);
    } else {
      layer.feeder.feedID3(item.data, item.pts);
    }
  };
  const push = (item: Feed) => {
    if (layers === null) {
      if (queue.length < maxQueued) {
        queue.push(item);
      }

      return;
    }
    for (const layer of layers) {
      feed(layer, item);
    }
  };
  const show = () => {
    for (const { controller } of layers ?? []) {
      if (visible) {
        controller.show();
      } else {
        controller.hide();
      }
    }
  };

  void Promise.all([import('aribb24.js'), loadCaptionFont()]).then(
    ([
      {
        Controller: ControllerClass,
        MPEGTSFeeder: FeederClass,
        CanvasMainThreadRenderer: RendererClass,
        TextRenderer: TextRendererClass,
      },
    ]) => {
      if (destroyed) {
        return;
      }
      // 字幕と文字スーパーは、別のデータグループで届き、同時に出ることもあるので、別々に描く。
      layers = (['Caption', 'Superimpose'] as const).map((type) => {
        const controller = new ControllerClass();
        const feeder = new FeederClass({ receive: { type } });
        // 描画の領域を、映像の枠に合わせる。HTML や SVG で描く方法は、CSP で禁じたインラインの style を使うので選ばない。
        const renderer = new RendererClass({
          font: { normal: captionFont },
          resize: { target: 'container', objectFit: 'contain' },
        });
        // キャプチャで写す canvas だと分かるよう、印を付ける。
        renderer.getPresentationCanvas().dataset.overlay = 'captions';
        controller.attachFeeder(feeder);
        controller.attachRenderer(renderer);
        // 字幕の文は、字幕の層からだけ取り出す。文字スーパーは、速報などで字幕の流れとは別のものなので、履歴に入れない。
        const text =
          type === 'Caption' && onText !== undefined
            ? textRecorder(TextRendererClass, onText)
            : null;
        if (text !== null) {
          controller.attachRenderer(text);
        }
        controller.attachMedia(video, container);

        return { controller, feeder, renderer, text };
      });
      show();
      const queued = queue;
      queue = [];
      for (const item of queued) {
        push(item);
      }
    },
    // 字幕は再生に欠かせないものではないので、読めなくても映像は流し続ける。
    () => {
      queue = [];
    },
  );

  return {
    feedB24: (data, pts) => push({ kind: 'b24', data, pts }),
    feedID3: (data, pts) => push({ kind: 'id3', data, pts }),
    setVisible: (next) => {
      visible = next;
      show();
    },
    reset: () => {
      queue = [];
      for (const { feeder, renderer } of layers ?? []) {
        feeder.clear();
        renderer.clear();
      }
    },
    destroy: () => {
      destroyed = true;
      queue = [];
      for (const { controller, renderer, text } of layers ?? []) {
        controller.detachMedia();
        controller.detachFeeder();
        controller.detachRenderer(renderer);
        renderer.destroy();
        if (text !== null) {
          controller.detachRenderer(text);
          text.destroy();
        }
      }
      layers = null;
    },
  };
}
