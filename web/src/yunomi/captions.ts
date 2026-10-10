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

// createCaptions は、video の上に字幕を描く。描画用の canvas は container に置くので、container は position: relative にする。
// aribb24.js は大きいので、再生を始めるときに読む。
export async function createCaptions(
  video: HTMLVideoElement,
  container: HTMLElement,
): Promise<Captions> {
  const { Controller, MPEGTSFeeder, CanvasMainThreadRenderer } = await import('aribb24.js');
  // 字幕と文字スーパーは、別のデータグループで届き、同時に出ることもあるので、別々に描く。
  const layers = (['Caption', 'Superimpose'] as const).map((type) => {
    const controller = new Controller();
    const feeder = new MPEGTSFeeder({ receive: { type } });
    // 描画の領域を、映像の枠に合わせる。HTML や SVG で描く方法は、CSP で禁じたインラインの style を使うので選ばない。
    const renderer = new CanvasMainThreadRenderer({
      resize: { target: 'container', objectFit: 'contain' },
    });
    controller.attachFeeder(feeder);
    controller.attachRenderer(renderer);
    controller.attachMedia(video, container);

    return { controller, feeder, renderer };
  });

  return {
    feedB24: (data, pts) => {
      for (const { feeder } of layers) {
        feeder.feedB24(data, pts);
      }
    },
    feedID3: (data, pts) => {
      for (const { feeder } of layers) {
        feeder.feedID3(data, pts);
      }
    },
    setVisible: (visible) => {
      for (const { controller } of layers) {
        if (visible) {
          controller.show();
        } else {
          controller.hide();
        }
      }
    },
    reset: () => {
      for (const { feeder, renderer } of layers) {
        feeder.clear();
        renderer.clear();
      }
    },
    destroy: () => {
      for (const { controller, renderer } of layers) {
        controller.detachMedia();
        controller.detachFeeder();
        controller.detachRenderer(renderer);
        renderer.destroy();
      }
    },
  };
}
