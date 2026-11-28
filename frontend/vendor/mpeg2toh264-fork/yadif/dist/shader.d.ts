/**
 * Everything this shader wants to be told, besides the three frames.
 *
 * `parity` is which lines survive: a line whose parity is this one is copied
 * from the current frame, and every other line is what the filter builds. For
 * one output frame per input frame that is `tff ? 0 : 1`, which keeps the field
 * that came first and rebuilds the moment it was captured at.
 *
 * With `film`, a frame whose pulldown phase `fieldMetrics` (see
 * film-shader.ts) gives is put back together into its film frame instead of
 * filtered. This is not done for the `second` field of a frame. `phase` is the
 * phase the page expects, for the debug overlay only. `comb` is the detector's
 * count of the combing each way of weaving would leave, block by block (see
 * COMB_FLAGS_FRAGMENT_SHADER), and a block past COMB_BLOCK_PIXELS is filtered
 * instead of woven.
 *
 * With `match`, a line that would be filtered is first taken from the field
 * it may have been captured with: the frame's own other field, or the
 * adjoining field of the frame before (for the first field) or after (for the
 * second). `combAhead` is `comb` for the frame after. With `instant`, the
 * line is also taken from the frame's own other field, where the detection
 * has found the two fields of each frame to be one instant (see INSTANT_ON in
 * film-shader.ts).
 */
export declare const YADIF_UNIFORMS: {
    readonly prev: "uPrev";
    readonly cur: "uCur";
    readonly next: "uNext";
    readonly size: "uSize";
    readonly parity: "uParity";
    readonly tff: "uTff";
    readonly spatialCheck: "uSpatialCheck";
    readonly debug: "uDebug";
    readonly film: "uFilm";
    readonly second: "uSecond";
    readonly phase: "uPhase";
    readonly fieldMetrics: "uFieldMetrics";
    readonly comb: "uComb";
    readonly combAhead: "uCombAhead";
    readonly match: "uMatch";
    readonly instant: "uInstant";
};
/**
 * The filter itself.
 *
 * It runs on RGB rather than on planes of YCbCr, which is what the browser
 * hands over when a frame is uploaded as a texture. The reference filters each
 * plane on its own and this filters each channel on its own, so the arithmetic
 * is the same one three times over; every comparison below is per channel,
 * which is what the `mix` by a `lessThan` mask is doing.
 *
 * The reference is never without a frame either side of the one it is
 * filtering: it holds frames back until it has them, and where its input ends
 * it duplicates rather than doing without. A caller here is expected to do the
 * same. A frame standing in as its own neighbour leaves the temporal check
 * nothing to measure, and what comes back is then the picture as it was --
 * except where it alternates strongly from line to line, which is what combing
 * is, and which is where the spatial check lets the interpolation through.
 */
export declare const YADIF_FRAGMENT_SHADER: string;
