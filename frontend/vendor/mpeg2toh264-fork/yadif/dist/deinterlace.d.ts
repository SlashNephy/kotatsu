import { type DecodedVideoInput } from "./decoded-video.js";
/** How often pictures come out: one per frame, one per field, or film at 24p. */
export type Cadence = "frame" | "field" | "film";
/**
 * Something that makes the pictures between two filtered pictures, such as
 * motion-compensated frame interpolation. It is handed this filter's own
 * context and textures, so nothing leaves the GPU: it draws onto the
 * framebuffer it is given (null for the canvas) itself, and puts back any
 * state it changes.
 *
 * Making a picture between two needs the second one already filtered, so the
 * pictures of a cadence it wants are put up one picture late. Only decoded
 * pictures on the main-thread renderer are made between.
 */
export interface PictureInterpolator {
    /** Whether to make pictures between pictures of this cadence. */
    wants(cadence: Cadence): boolean;
    /**
     * The two pictures the next draws fall between: called once for each pair,
     * before the first draw between them. Textures are held the way a
     * framebuffer is, rows from the bottom, `width` by `height`.
     */
    pair(gl: WebGL2RenderingContext, previous: WebGLTexture, next: WebGLTexture, width: number, height: number): void;
    /**
     * Draw the picture `t` (0 to 1) of the way from the first to the second
     * onto `target`, `width` by `height`, and say whether it did: one that keeps
     * to a rate of its own can leave the picture on the canvas as it is for a
     * refresh. The target is a texture of the picture's size rather than the
     * canvas when a scaler puts the pictures up.
     */
    draw(gl: WebGL2RenderingContext, t: number, width: number, height: number, target: WebGLFramebuffer | null): boolean;
}
/**
 * Something that puts a filtered picture onto the canvas at a size of its
 * own, such as an upscaler drawing at the size the canvas is shown at rather
 * than the size the picture is coded at. Like a PictureInterpolator, it is
 * handed this filter's own context and textures and puts back any state it
 * changes. Only the main-thread renderer uses one.
 */
export interface PictureScaler {
    /**
     * The size to give the canvas's buffer for a picture `width` by `height`
     * shown across `boxWidth` by `boxHeight` device pixels, or null to keep the
     * picture's own size and copy it texel for texel. The context is there to
     * read its limits from.
     */
    size(gl: WebGL2RenderingContext, width: number, height: number, boxWidth: number, boxHeight: number): {
        width: number;
        height: number;
    } | null;
    /**
     * Draw `texture`, `width` by `height`, across the whole of the canvas,
     * `outputWidth` by `outputHeight`, and say whether it did. The texture
     * holds rows from the bottom the way a framebuffer does, or from the top
     * when `flipped`. One that did not is let go of, and the canvas goes back to
     * the picture's own size.
     */
    draw(gl: WebGL2RenderingContext, texture: WebGLTexture, flipped: boolean, width: number, height: number, outputWidth: number, outputHeight: number): boolean;
}
/**
 * How the filter is getting on, and where it is being let down.
 *
 * A deinterlacer is only as good as the frames it is given. yadif reads the
 * frame either side of the one it is filtering, and every count here that is
 * not `filtered` is a way of not having them: a frame the callback never saw
 * leaves the two neighbours two frames apart, and one filtered against a copy
 * of itself has no motion to measure at all. Both show up as combing that
 * comes and goes, which is worth being able to point at rather than guess at.
 */
export interface DeinterlaceStats {
    /** Frames filtered with a real frame on either side: the good case. */
    filtered: number;
    /**
     * Frames the element presented that the filter never saw, counted from the
     * gaps in `presentedFrames`. The neighbours of the frames either side of a
     * gap are further apart in time than the filter believes, so its idea of
     * what moved is wrong. A page that sees this climbing is asking the filter
     * to keep up with more than it can.
     *
     * Firefox does not report this count. With main-thread rendering the gaps
     * are taken from the time between new pictures. A frame whose pixels equal
     * the previous frame's cannot be told apart from a missed one, so a still
     * scene raises this count although nothing is wrong. With Worker rendering
     * the gaps are those in `mozPaintedFrames`.
     *
     * With decoded pictures, the pictures dropped because their time had
     * passed before they could be taken in, none of them shown. Pictures that
     * ended before the place playback last started from are not counted: they
     * were never due.
     */
    missed: number;
    /**
     * Frames the browser decoded and threw away without presenting, as the
     * element itself counts them. This is the machine being behind rather than
     * this filter: it happens with the deinterlacer off as well.
     */
    dropped: number;
    /**
     * Frames filtered with a copy of themselves standing in for a neighbour,
     * which is the start of a stream, the frame a seek lands on, and the last
     * frame before playback stops. Expected in ones and twos; a stream of them
     * means the held frames keep being thrown away.
     */
    degraded: number;
    /** Times the held frames were dropped as stale: seeks, and stream changes. */
    discontinuities: number;
    /**
     * Times the schedule was restarted from the clock instead of following on
     * from the last picture: a seek, a stall, a cadence change, or a missed
     * frame. Climbing during steady playback means the cadence is not holding.
     * (otya) With decoded pictures, times the media clock started again from
     * `currentTime`: after a seek, a pause, a rate change or a stall, or on a
     * reading too far from it. A start on the first change after a hold that
     * the readings then take back counts, and so does the start after it.
     */
    resynced: number;
    /**
     * Fields skipped because their display time passed or disagreed with the
     * presentation clock. With decoded pictures, every field or frame filtered
     * or due and never shown, including those a rate above the refresh skips,
     * but not those from before the place playback last started from.
     */
    late: number;
    /**
     * With decoded pictures: the media time being heard less that of the
     * output being shown, in seconds, averaged over about the last second -- a
     * player that plays the sound itself moves the sound's speed by it to keep
     * the two together. NaN before pictures are shown by counting refreshes.
     */
    avDifference: number;
    /**
     * Times the presentation queue was reset because its scheduled lead exceeded
     * the useful display window. With decoded pictures, pictures dropped
     * because more were pushed than are held.
     */
    queueResetted: number;
    /**
     * Frames presented per second over the last report. With decoded pictures,
     * pictures taken in per second.
     */
    fps: number;
    /**
     * Average rendering-thread time per input frame spent uploading, filtering
     * and presenting during the last report. Excludes GPU execution time and,
     * when rendering in a Worker, time spent on the main thread. On Firefox
     * with main-thread rendering it also excludes the upload, which happens
     * once per refresh while polling for new pictures, not once per frame.
     */
    frameMs: number;
    /**
     * The largest number of pictures queued during the last reporting interval,
     * across both the field-rate and film scheduling paths. With decoded
     * pictures, the largest number pushed and not yet taken in.
     */
    maxQueuedFields: number;
    /**
     * Pictures actually copied to the canvas per second: with decoded pictures,
     * fields, film frames or frames, as the mode shows them.
     */
    outputFps: number;
    /**
     * GPU processing time, supported only in Chrome. On ANGLE's Metal backend
     * it spans command buffers rather than the work in them, so it overstates
     * anything split into many small passes; a change is best judged by its
     * direction here and by a synchronous readback in isolation.
     */
    gpuMs: number | undefined;
    /** Whether 2:3 pulldown has been detected and the frames are shown at 24p. */
    film: boolean;
    /**
     * Whether the two fields of each frame were recently found to be one
     * instant, as in 30p material or film carried by repeated whole frames.
     * This is the detection's last verdict read back from the GPU. While it
     * holds, both fields of a frame are drawn as the same whole picture instead
     * of two interpolated ones, except while a pulldown cadence is held and on
     * the frame after a gap. Only evaluated while `film` is on.
     */
    oneInstant: boolean;
    /**
     * Frames filtered with no cadence held, in which a field could be woven
     * with a matching field of a neighbouring frame. These are the frames
     * shortly after a cadence is lost. Weaving because of the `oneInstant`
     * verdict is not counted. Blocks without a matching partner are still
     * interpolated, so on video inside this window the count includes frames
     * in which nothing was woven.
     */
    matched: number;
    /**
     * Why requested film reconstruction is currently unavailable, or null
     * while healthy. The requesting option (`film`) stays as the
     * caller set it -- intent is preserved -- while pictures continue through
     * plain YADIF. Cleared on the next retry (start, scan change, resize, or
     * re-setting the option); a repeated failure notifies again as a new
     * episode. Always null when film reconstruction is not requested.
     */
    filmError?: string | null;
}
export interface DeinterlacerOptions {
    /** Rendering target, defaulting to `main`; `auto` prefers the bundled Worker and falls back to the main thread if initialization fails. */
    rendering?: "auto" | "worker" | "main";
    /** The module Worker's URL, defaulting to the file bundled with the package. */
    workerUrl?: string | URL;
    /**
     * Whether to show a picture for every field rather than for every frame.
     *
     * Interlaced video carries two moments in each frame, and one output frame
     * per input frame throws the second one away: motion that was captured
     * fifty or sixty times a second is shown twenty-five or thirty. With this
     * on, each frame is filtered twice -- once keeping the field that came
     * first and once keeping the other -- and the second picture is put up half
     * a frame after the first, which is the moment it was taken at. Motion is
     * as smooth as the broadcast was, at twice the filtering.
     *
     * It needs a display that can show them: at 59.94 fields a second there is
     * one refresh of a 60 Hz screen for each, and nothing to spare.
     *
     * With `spatialCheck`, this is which of yadif's four modes runs: off/on is
     * `send_frame`, on/on is `send_field`, and the two with `spatialCheck` off
     * are the `nospatial` pair.
     */
    doubleRate?: boolean;
    /**
     * Whether to let the local vertical range widen what the temporal check
     * allows. This is yadif's default and its `nospatial` mode turns it off.
     */
    spatialCheck?: boolean;
    /**
     * Called about once a second while frames are arriving, and not at all while
     * nothing is playing -- there is nothing to say about a filter that is not
     * being asked for anything.
     */
    onStats?(stats: DeinterlaceStats): void;
    /**
     * Whether to detect 2:3 pulldown and show film at 24 frames a second.
     * Frames that are not film are filtered as usual. Needs
     * `EXT_color_buffer_float`.
     */
    film?: boolean;
    /**
     * Called when a rendering resource fails after construction: a GPU film detector that throws mid-stream,
     * which leaves plain YADIF running, or the renderer itself -- a Worker that died again after its restart,
     * Worker rendering asked for where there is none, a lost WebGL context, a decoded picture it could not
     * take in -- which stops it for good (see `DecodedVideoInput.rendererStatus`).
     * The same message is dispatched as a `failure` event.
     * This option lets callers install the handler alongside `onStats`.
     */
    onFailure?(message: string): void;
    /**
     * Whether to draw the pulldown detection over the picture while `film` is enabled.
     */
    debug?: boolean;
    /** What to make the pictures between pictures with. See PictureInterpolator. */
    interpolator?: PictureInterpolator | null;
    /** What to put the pictures up with at a size of its own. See PictureScaler. */
    scaler?: PictureScaler | null;
}
/** Field information supplied by a player or another video source. */
export interface Scan {
    interlaced: boolean;
    topFieldFirst: boolean;
}
export interface VideoState {
    start: number;
    codedSize?: {
        width: number;
        height: number;
    };
    scan?: Scan;
}
export interface DeinterlacerEventMap {
    stats: CustomEvent<DeinterlaceStats>;
    /** A rendering resource failed after construction; detail is the reason. */
    failure: CustomEvent<string>;
}
/** Whether this browser has the two things the deinterlacer is built on. */
export declare function supportsDeinterlace(): boolean;
/**
 * Puts a deinterlaced copy of a `<video>` over the top of it.
 *
 * ```ts
 * const deinterlacer = new Deinterlacer(video);
 * deinterlacer.start();
 * ```
 *
 * `start()` wraps the element in a positioned `<div>` and adds a canvas over
 * it. The canvas takes no pointer events, so anything the page put on the
 * element still works, but it does cover the element's own controls; a page
 * that wants controls with this on has to draw them itself. `stop()` hides the
 * canvas again, which is all it takes to compare the two.
 *
 * While frames are arriving, the current counters are also dispatched as a
 * `stats` event about once a second. The optional `onStats` callback receives
 * the same snapshot for callers that prefer a constructor option.
 */
export declare class Deinterlacer extends EventTarget {
    #private;
    /**
     * Receive decoded pictures with their presentation times. From the first
     * push on they are the only pictures shown, and the element is only the
     * clock they are shown by.
     */
    readonly decodedVideo: DecodedVideoInput;
    constructor(video: HTMLVideoElement, options?: DeinterlacerOptions);
    /**
     * Whether the canvas, rather than the element, is what shows the picture.
     * A page uses this to decide which of the two to capture or transform.
     * Filtering an element's own picture that is the same as filtering it at
     * all, since a progressive source is left to the element. Decoded pictures
     * are on the canvas filtered or not, because the element has none.
     */
    get running(): boolean;
    /** The HTML canvas currently placed over the media element. */
    get canvas(): HTMLCanvasElement;
    /** Whether the caller wants filtering, independently of the current source. */
    get enabled(): boolean;
    set enabled(enabled: boolean);
    /** Update whether the source needs filtering and which field comes first. */
    set scan(scan: Scan | null);
    get scan(): Scan | null;
    set videoTimeline(timeline: readonly VideoState[]);
    get videoTimeline(): readonly VideoState[];
    /**
     * What to put on the screen for fullscreen: the `<div>` holding both the
     * element and the canvas once there is one, and the element itself before
     * that. Fullscreening the element alone would leave the canvas behind in
     * the page, and with it the only deinterlaced picture there is.
     */
    get container(): HTMLElement;
    /** Whether a picture goes up for every field rather than every frame. */
    get doubleRate(): boolean;
    set doubleRate(doubleRate: boolean);
    get spatialCheck(): boolean;
    set spatialCheck(spatialCheck: boolean);
    get film(): boolean;
    set film(film: boolean);
    get debug(): boolean;
    set debug(debug: boolean);
    get interpolator(): PictureInterpolator | null;
    set interpolator(interpolator: PictureInterpolator | null);
    get scaler(): PictureScaler | null;
    /**
     * Setting the same scaler again asks it for the size afresh, for a page
     * that changed something the size depends on that this cannot see.
     */
    set scaler(scaler: PictureScaler | null);
    start(): void;
    /**
     * Take the deinterlaced picture away, leaving the element's own showing.
     * Decoded pictures held are released, and those pushed until the next
     * start() are released at once.
     */
    stop(): void;
    destroy(): void;
    /**
     * Copy the picture currently represented by the deinterlacer.
     *
     * The WebGL drawing buffer is deliberately not preserved between browser
     * composites. Repeating the exact draw path of the presented picture before
     * `createImageBitmap` makes a snapshot reliable without imposing the
     * permanent cost of `preserveDrawingBuffer` on ordinary playback.
     */
    capture(): Promise<ImageBitmap>;
    addEventListener<K extends keyof DeinterlacerEventMap>(type: K, listener: (event: DeinterlacerEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
    addEventListener(type: string, listener: EventListenerOrEventListenerObject | null, options?: boolean | AddEventListenerOptions): void;
    removeEventListener<K extends keyof DeinterlacerEventMap>(type: K, listener: (event: DeinterlacerEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
    removeEventListener(type: string, listener: EventListenerOrEventListenerObject | null, options?: boolean | EventListenerOptions): void;
}
