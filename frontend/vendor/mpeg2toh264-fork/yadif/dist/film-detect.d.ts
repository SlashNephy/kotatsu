/** What the pulldown detection said about a frame. */
export interface Phase {
    /** Which frame of the cycle it is, 1 to 5, or 0 for none. */
    phase: number;
    /** How many frames in a row have had a phase. See film-shader.ts. */
    run: number;
}
export declare const NO_PHASE: Phase;
/**
 * Detects the pulldown phase of frames on the GPU.
 *
 * `detect` measures the frame being filtered against the one after it, moves
 * the earlier measurements along, decides the phase, and starts reading the
 * result back; `poll` collects it once it has arrived, a frame or so later.
 * `texture` is the newest measurements, for the filter to read the phase
 * from without waiting for the page.
 * `measureComb` counts the combing weaving the frame would leave, block by
 * block, into `combTexture`, for the filter to interpolate those blocks. With
 * `ahead` it counts the frame after it into `combAheadTexture`. The second
 * count there is the combing left by weaving that frame's first field between
 * the second-field lines of this one.
 *
 * The measurements of one frame are written from those of the frame before
 * in a single pass, so they live in two textures taken in turns; nothing is
 * copied between passes, and the frame costs three draws and a readback,
 * and one more draw for each `measureComb` call.
 */
export declare class FilmDetector {
    #private;
    /** The last metrics read back, laid out as FIELD_METRICS says. */
    readonly metrics: Float32Array<ArrayBuffer>;
    constructor(gl: WebGL2RenderingContext);
    /** The newest measurements, or null before any frame has been measured. */
    get texture(): WebGLTexture | null;
    /**
     * Combed pixels per block of the frame last measured by `measureComb`, or
     * null before one has been. See COMB_FLAGS_FRAGMENT_SHADER.
     */
    get combTexture(): WebGLTexture | null;
    /**
     * Combed pixels per block of the frame last measured with `ahead`, or null
     * before one has been.
     */
    get combAheadTexture(): WebGLTexture | null;
    /** The size of the frames to be measured, which sizes the block grid. */
    resize(width: number, height: number): void;
    /**
     * Forget the measurements: the next frame starts a cycle from nothing.
     * `keepInstant` keeps the one-instant score, for a gap in the frames when
     * the content has not changed.
     */
    reset(keepInstant?: boolean): void;
    /**
     * Detect the pulldown phase of `cur`, the frame being filtered, against
     * `next`, and start reading it back. Only the two comparisons against the
     * next frame are measured; the rest are earlier ones moved along a frame.
     * `first` is the parity of the field that was captured first.
     *
     * The same pass scores whether the two fields of a frame are one instant,
     * from the comb counts of the frame as it stands. `measureComb` for this
     * frame must have run before this.
     */
    detect(cur: WebGLTexture, next: WebGLTexture, first: number): void;
    /**
     * Count the combing each way of weaving `cur`, the frame being filtered,
     * would leave: as it stands, and with `prev`'s second field. `first` is the
     * parity of the field that was captured first. With `ahead`, `prev` is the
     * frame being filtered and `cur` is the one after it, and the counts are
     * those of `cur`.
     */
    measureComb(prev: WebGLTexture, cur: WebGLTexture, first: number, ahead?: boolean): void;
    /**
     * The phase of the last frame measured, once the GPU has handed it back,
     * and null while it is still on its way. It is handed back once.
     * `instant` is the one-instant verdict as of that frame. The filter reads
     * the same verdict from `texture` without waiting for the readback.
     */
    poll(): (Phase & {
        age: number;
        instant: boolean;
    }) | null;
    destroy(): void;
}
