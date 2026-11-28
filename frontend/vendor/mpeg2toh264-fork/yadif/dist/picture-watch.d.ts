import type { Pictures } from "./video-frame.js";
/**
 * Detects new pictures of a video element by comparing them pixel by pixel.
 *
 * Firefox has no reliable notification of a new picture. The media sink
 * replaces the element's current picture on the audio clock, independently of
 * the display refresh. `mozPaintedFrames` counts frames the compositor has
 * painted and lags that replacement by up to one refresh. The picture read
 * when the count increases may already be the next one, so the next increase
 * reads it again, and the picture in between is never read.
 * requestVideoFrameCallback() identifies frames exactly but is not called for
 * every frame. The picture itself is reliable: texImage2D() reads whichever
 * picture is current. Taking it on every refresh and comparing it with the
 * previous take finds each picture exactly once, as long as each picture lasts
 * at least one refresh.
 *
 * The comparison runs on the GPU and the result is read back through a pixel
 * buffer behind a fence. Reading it back immediately would block the calling
 * thread on every refresh until the GPU process catches up. In Firefox that
 * costs about 3 ms, against 0.3 ms with the fence. A result is therefore known
 * one or more refreshes after its take, and the take keeps its texture until
 * then. A new picture hands that texture to the filter, because reading the
 * element again by then could return a later picture.
 *
 * Two pictures with identical pixels cannot be told apart. The second is
 * reported as unchanged.
 */
export declare class PictureWatch implements Pictures {
    #private;
    constructor(gl: WebGL2RenderingContext, video: HTMLVideoElement);
    take(): number | null;
    settle(): {
        id: number;
        changed: boolean;
    } | null;
    /**
     * Give the texture of a reported take to the caller and accept a texture in
     * exchange. The replacement becomes available for later takes.
     */
    adopt(id: number, replacement: WebGLTexture): WebGLTexture;
    release(id: number): void;
    reset(): void;
    destroy(): void;
}
