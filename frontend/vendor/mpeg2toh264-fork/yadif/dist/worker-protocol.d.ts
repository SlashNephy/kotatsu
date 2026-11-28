import type { DecodedPicture } from "./decoded-video.js";
import type { DeinterlaceStats, Scan, VideoState } from "./deinterlace.js";
/** What the page observed alongside one decoded frame. */
interface WorkerFrameObservation {
    mediaTime: number;
    presentedFrames: number;
    /**
     * The display time reported by requestVideoFrameCallback(), measured on
     * the acquisition clock. The Worker translates it using `timeOrigin`
     * before anchoring its presentation schedule to it.
     */
    expectedDisplayTime: number;
    /**
     * The performance.timeOrigin against which `expectedDisplayTime` was measured.
     *
     * Sent with every frame: document picture-in-picture can move the video
     * node to another window between frames, so an origin sent only at
     * initialization would go stale. A frame waiting for an acknowledgement
     * keeps the origin recorded when it was acquired.
     */
    timeOrigin: number;
    width: number;
    height: number;
    /**
     * Timing from Firefox's frame counters. Include it in the wire type so
     * an explicit message literal carries it just as spread and structured
     * clone do.
     */
    mozTiming?: {
        periodMs: number;
        discontinuity: boolean;
    };
}
/** Deinterlacer settings accepted by the rendering engine inside the Worker. */
export interface WorkerRenderingOptions {
    doubleRate: boolean;
    spatialCheck: boolean;
    film: boolean;
    debug: boolean;
}
/** Media element state copied to the Worker, where the DOM is unavailable. */
export interface WorkerVideoState {
    currentTime: number;
    playbackRate: number;
    seeking: boolean;
    paused: boolean;
    ended: boolean;
    readyState: number;
    videoWidth: number;
    videoHeight: number;
    buffered: Array<{
        start: number;
        end: number;
    }>;
}
export type WorkerCommand = {
    type: "initialize";
    canvas: OffscreenCanvas;
    options: WorkerRenderingOptions;
    scan: Scan | null;
    videoTimeline: readonly VideoState[];
    enabled: boolean;
    video: WorkerVideoState;
} | {
    type: "frame";
    id: number;
    frame: VideoFrame;
    metadata: WorkerFrameObservation;
    video: WorkerVideoState;
} | {
    type: "settings";
    options: WorkerRenderingOptions;
    retryFilm?: "film";
} | {
    type: "scan";
    scan: Scan | null;
} | {
    type: "timeline";
    videoTimeline: readonly VideoState[];
} | {
    type: "enabled";
    enabled: boolean;
}
/** Start or stop showing decoded pictures, which `enabled` does not. */
 | {
    type: "running";
    running: boolean;
}
/** A decoded picture, its buffer transferred. `id` comes back with the buffer. */
 | {
    type: "picture";
    id: number;
    picture: DecodedPicture;
} | {
    type: "pictures";
    action: "reset" | "finish";
}
/** `DecodedVideoInput.startsAt`, `at` taken on the page's clock. */
 | {
    type: "start";
    media: number;
    at: number;
    timeOrigin: number;
    video: WorkerVideoState;
}
/** A reading of the element's clock, taken at `at` on the page's clock. */
 | {
    type: "clock";
    video: WorkerVideoState;
    at: number;
    timeOrigin: number;
}
/** Whether the page showing the canvas is hidden, which a Worker cannot see. */
/** Whether the page showing the canvas is hidden, which a Worker cannot see, with the element's state then. */
 | {
    type: "pageVisibility";
    hidden: boolean;
    video: WorkerVideoState;
} | {
    type: "event";
    name: "emptied" | "pause" | "ended" | "seeking" | "seeked" | "ratechange" | "waiting";
    video: WorkerVideoState;
} | {
    type: "capture";
    id: number;
    width: number;
    height: number;
} | {
    type: "destroy";
};
export type WorkerNotification = 
/** The renderer is set up, and the GPU has run that setup. */
{
    type: "ready";
} | {
    type: "failed";
    message: string;
} | {
    type: "consumed";
    id: number;
} | {
    type: "visibility";
    visible: boolean;
} | {
    type: "stats";
    stats: Omit<DeinterlaceStats, "dropped">;
    /** Failed film attempts, including identical reasons between reports. */
    filmFailure: number;
} | {
    type: "capture";
    id: number;
    image: ImageBitmap | null;
}
/** A decoded picture's buffer, transferred back. */
 | {
    type: "release";
    id: number;
    data: ArrayBuffer;
};
export {};
