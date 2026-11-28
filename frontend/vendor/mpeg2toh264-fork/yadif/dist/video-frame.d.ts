export interface FrameMetadata {
    width: number;
    height: number;
    mediaTime: number;
    presentedFrames: number;
    expectedDisplayTime: number;
    /**
     * `performance.timeOrigin` of the realm `now` and `expectedDisplayTime` were
     * measured against.
     *
     * There is more than one candidate. requestVideoFrameCallback() reports on
     * the clock of the window that owns the video node, which a document
     * picture-in-picture move replaces; the counter path below times its own
     * polling, on this module's realm. A consumer on a third clock -- the
     * rendering Worker -- cannot tell them apart, so each acquisition says which
     * it used and the timestamps stay convertible.
     */
    timeOrigin: number;
    /** The frame for this notification, valid only until the callback returns. */
    frame?: VideoFrame;
    /** Firefox counters supply identity and wall-clock cadence, not a frame PTS. */
    mozTiming?: {
        periodMs: number;
        discontinuity: boolean;
    };
    /**
     * The take that holds this frame's picture. Set when the frame was found by
     * comparing the element's pictures. It can be adopted only until the
     * callback returns.
     */
    picture?: number;
}
/**
 * Takes of the element's current picture, one per refresh. Each take has a
 * result saying whether it differs from the previous take. See PictureWatch.
 */
export interface Pictures {
    /** Take the current picture. Null when no texture is available. */
    take(): number | null;
    /** Report the oldest unreported take, once its result is known. */
    settle(): {
        id: number;
        changed: boolean;
    } | null;
    /** Release a reported take. Has no effect if it was already adopted. */
    release(id: number): void;
    /** Discard every take. */
    reset(): void;
}
type FrameCallback = (now: number, metadata: FrameMetadata) => void;
export declare function supportsVideoFrames(): boolean;
/**
 * One outstanding, cancellable frame request. Prefer Firefox's counters even
 * when native rVFC exists: its notifications/mediaTime can advance in 40 ms
 * steps. Parsed/decoded frames include read-ahead, and presented frames are
 * accounted by the media sink separately from painting. Only the painted
 * count is followed, because OR-ing the counters would ingest one picture
 * repeatedly.
 *
 * The painted count shows that there are new pictures and how often, but not
 * which picture the element holds when it is read. The two can be out of step
 * by up to one refresh. When a PictureWatch is available (`pictures`), it
 * decides which frames are new and the painted count only measures the
 * cadence. Without one, the painted count alone decides.
 *
 * https://searchfox.org/firefox-main/source/dom/media/mediaelement/HTMLVideoElement.cpp
 */
export declare class VideoFrames {
    #private;
    constructor(video: HTMLVideoElement, 
    /**
     * Returns the PictureWatch used to find Firefox's new pictures, or null
     * when there is none. Called on every poll because the renderer that owns
     * the watch can change.
     */
    pictures?: () => Pictures | null);
    /** Whether acquisition uses the captured pictures' own timestamps. */
    get captureDriven(): boolean;
    request(callback: FrameCallback): void;
    cancel(): void;
    destroy(): void;
    /** Deliver pending input on the rendering window's refresh, before drawing. */
    flush(now: number): void;
}
export {};
