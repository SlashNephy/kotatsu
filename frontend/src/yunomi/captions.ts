import type { CanvasMainThreadRenderer, Controller, MPEGTSFeeder, TextRenderer } from 'aribb24.js';

import {
  broadcastExtendedFontUrl,
  broadcastExtendedUnicodeRange,
  broadcastFontUrl,
  usesExtendedFont,
} from './broadcastFont';
import { defaultCaptionStyle, rendererColor, type CaptionStyle } from './captionStyle';
import {
  drcsGlyphs,
  drcsRenderReplacements,
  drcsReplacements,
  prepareDRCS,
  recordDRCS,
} from './drcs';

export { rendererColor, type CaptionStyle } from './captionStyle';

// Captions は、字幕と文字スーパーを、映像の上に描く (設計書 7.4)。
// ブラウザの中で変換する経路では、プレイヤーが渡す private PES を、kyusu の経路では、hls.js が取り出す ID3 を渡す。
// どちらも、時刻は video の時刻 (秒) にそろえて渡す。
export type Captions = {
  feedB24: (data: ArrayBuffer, pts: number) => void;
  feedID3: (data: Uint8Array, pts: number) => void;
  setVisible: (visible: boolean) => void;
  // setSuperimposeVisible は、文字スーパーを出すかどうかを、字幕とは別に切り替える (#491)。setVisible で隠していれば、どちらも出さない。
  setSuperimposeVisible: (visible: boolean) => void;
  // setStyle は、字幕と文字スーパーの見た目を変える (#491)。次に描くときから効く。
  setStyle: (style: CaptionStyle) => void;
  // reset は、受け取った字幕を捨てる。読み直すと video の時刻の基準が変わるので、前の読み込みの字幕を出さないようにする。
  reset: () => void;
  destroy: () => void;
};

// RendererWithOption は、aribb24.js の描画器のうち、描くたびに読み直す設定。型の上では隠れているので、ここで名前を付ける。
type RendererWithOption = {
  option: { color: { stroke: string | null; background: string | null } };
};

type Layer = {
  // type は、字幕 (Caption) か文字スーパー (Superimpose) か。
  type: 'Caption' | 'Superimpose';
  controller: Controller;
  feeder: MPEGTSFeeder;
  renderer: CanvasMainThreadRenderer;
  // text は、字幕の文を取り出す描画器。字幕の層にだけ付ける (#450)。
  text: TextRenderer | null;
};

// textRecorder は、aribb24.js の TextRenderer を広げ、字幕を描くたびに、その文を onText に渡す描画器を作る。
// aribb24.js は、字幕の時刻 (PTS) に合わせて、付けた描画器すべての render を呼ぶので、時刻を自前で合わせなくてよい。
// 消すとき (clear) は、空の文を渡す。今出ている字幕をキャプチャに書き込むときに、消えたことを知るためである (#458)。
// 字幕に出た外字も覚え、表に無いものを管理者が登録できるようにする (#825)。
function textRecorder(Base: typeof TextRenderer, onText: (text: string) => void): TextRenderer {
  class Recorder extends Base {
    override render(...args: Parameters<TextRenderer['render']>): void {
      recordDRCS(args[1] ?? []);
      super.render(...args);
      onText(this.getText());
    }

    override clear(): void {
      super.clear();
      onText('');
    }
  }

  return new Recorder({ replace: { half: true, drcs: drcsReplacements } });
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
// フォントは、基本の字と、JIS X 0213 で足された漢字の 2 つの面に分けてある (#1520)。どちらも同じファミリ名で宣言し、
// 基本の面は、字幕を描くときに初めて読み、ページの中では 1 回だけ読む。後の面は unicodeRange を付けて宣言だけしておき、
// その字が出たときに読む (loadExtendedCaptionFont)。読めなくても失敗にはしない。
function loadCaptionFont(): Promise<void> {
  if (captionFontLoading === null) {
    if (typeof FontFace === 'undefined') {
      captionFontLoading = Promise.resolve();
    } else {
      if (broadcastExtendedUnicodeRange !== null) {
        document.fonts.add(
          new FontFace(captionFontFamily, `url(${broadcastExtendedFontUrl})`, {
            unicodeRange: broadcastExtendedUnicodeRange,
            display: 'swap',
          }),
        );
      }
      captionFontLoading = new FontFace(captionFontFamily, `url(${broadcastFontUrl})`, {
        display: 'swap',
      })
        .load()
        .then(
          (face) => {
            document.fonts.add(face);
          },
          () => {},
        );
    }
  }

  return captionFontLoading;
}

// loadExtendedCaptionFont は、text を描くのに要る、後の面を読み終えるのを待つ。canvas は、描く前に読み込みを待たないので、
// 待たずに描くと、その字をほかのフォントで描いてしまう。読めなくても失敗にはせず、ほかのフォントで描く。
function loadExtendedCaptionFont(text: string): Promise<void> {
  if (typeof document.fonts === 'undefined') {
    return Promise.resolve();
  }

  return document.fonts.load(`1em '${captionFontFamily}'`, text).then(
    () => {},
    () => {},
  );
}

// fontAwareRenderer は、aribb24.js の CanvasMainThreadRenderer を広げ、JIS X 0213 で足された漢字のある字幕を、
// 後の面を読み終えてから描く描画器を作る (#1520)。待つ間に次の字幕が来るか、消えるか、描画器を捨てたら、古い字幕は描かない。
// 外字 (DRCS) を JIS X 0213 の漢字に置き換えたものは、ここでは見分けない。後の面を読むまでは、ほかのフォントで描く。
// 描く直前には、置き換えられない外字の、なめらかにした path を登録する (#1518)。待つ間に別の字幕を描くと、登録が変わりうるので、
// 待った後にも登録し直す。
export function fontAwareRenderer(
  Base: typeof CanvasMainThreadRenderer,
  option: ConstructorParameters<typeof CanvasMainThreadRenderer>[0],
): CanvasMainThreadRenderer {
  class FontAware extends Base {
    // generation は、render と clear のたびに進める。待った後に、まだ最新の字幕かを確かめるのに使う。
    generation = 0;

    override render(...args: Parameters<CanvasMainThreadRenderer['render']>): void {
      const generation = ++this.generation;
      const text = (args[1] ?? [])
        .map((token) => (token.tag === 'Character' ? token.character : ''))
        .join('');
      const draw = () => {
        prepareDRCS(args[0].fontsize, args[1] ?? []);
        super.render(...args);
      };
      if (!usesExtendedFont(text)) {
        draw();

        return;
      }
      void loadExtendedCaptionFont(text).then(() => {
        if (generation === this.generation) {
          draw();
        }
      });
    }

    override clear(): void {
      this.generation++;
      super.clear();
    }

    override destroy(): void {
      this.generation++;
      super.destroy();
    }
  }

  return new FontAware(option);
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
  let superimposeVisible = true;
  let style = defaultCaptionStyle;
  // applyStyle は、描画器の色の設定を変える。aribb24.js は、描くたびに設定を読むので、作り直さなくてよい。
  const applyStyle = () => {
    for (const { renderer } of layers ?? []) {
      Object.assign((renderer as unknown as RendererWithOption).option.color, rendererColor(style));
    }
  };
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
    for (const { type, controller } of layers ?? []) {
      const layerVisible = visible && (type === 'Caption' || superimposeVisible);
      if (layerVisible) {
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
        // 外字は、共有の表で文字に置き換える (#825)。表は描くたびに読まれるので、入れ替えれば次の字幕から効く。
        // 表に無い外字は、私用領域の文字に置き換え、なめらかにした path で描く (#1518)。
        const renderer = fontAwareRenderer(RendererClass, {
          font: { normal: captionFont },
          replace: { drcs: drcsRenderReplacements, glyph: drcsGlyphs },
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

        return { type, controller, feeder, renderer, text };
      });
      show();
      applyStyle();
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
    setSuperimposeVisible: (next) => {
      superimposeVisible = next;
      show();
    },
    setStyle: (next) => {
      style = next;
      applyStyle();
    },
    reset: () => {
      queue = [];
      for (const { feeder, renderer, text } of layers ?? []) {
        feeder.clear();
        renderer.clear();
        // 読み直す前の字幕を、今出ている字幕として残さない (#458)。
        text?.clear();
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
