/**
 * The page's view of the sound output: which media time is being heard at a
 * given `performance.now()`, from the processor's `position` reports and
 * readings of the context clock.
 *
 * Context frame → wall time. `AudioContext.currentTime` counts what has been
 * rendered, and rendering runs in bursts of several quanta per device
 * callback: plotted against `performance.now()` it is a staircase above the
 * line the speaker follows. The readings taken just before a burst are the
 * least ahead of the speaker, so the mapping follows the lower envelope of
 * the readings. Each `BLOCK` of readings contributes its lowest one, and a
 * Kalman filter tracks the line through these block minima: its state is
 * the context time on the line now and the line's slope, which is the
 * device clock's rate against `performance.now()` (they differ by a few
 * hundredths of a percent). The slope's uncertainty starts at `DRIFT_PRIOR`
 * and shrinks as minima come in, so the start settles in about a second
 * while the slope is in the end averaged over tens of seconds; the small
 * process noise (`OFFSET_WANDER`, `DRIFT_WANDER`) lets both follow slow
 * changes. A block minimum far from the prediction is left out (the
 * innovation gate).
 *
 * A minimum per block rather than the lower convex hull of all readings: the
 * hull rests on single extreme readings, so one reading slightly under the
 * line (the start of a device stand-still, a late timestamp) holds a whole
 * window down, and which readings are extreme changes in steps. Filtering
 * many block minima averages that out at the cost of sitting a fraction of
 * a callback higher, which is part of the constant below.
 *
 * `outputLatency` is added on top. How far the envelope itself is ahead of
 * the speaker is not observable from the page: it is somewhere between
 * nothing and one callback (about 10 ms), and constant while the device runs
 * the same way. That constant is the remaining ambiguity of the absolute
 * offset; what this class keeps small is the variation, which is what a
 * picture schedule notices.
 *
 * Disturbances. A main-thread stall only leaves a gap in the readings; the
 * line carries on across it. When `currentTime` has not moved for `STAND`,
 * longer than any gap between callbacks, the device is standing still and
 * the readings are left out until it moves again: they say nothing about
 * the line, and when it catches up it is back on it. (Leaving out readings
 * merely under the line instead would drop exactly the lowest readings
 * whenever the slope is still off, and the fit would never correct it.) A
 * reading more than `BELOW` under or `ABOVE` over the line is left out too,
 * and if the readings stay that far off for `RELOCK`, the device's timeline
 * really moved, and the fit starts again from the new readings.
 */
import type { SoundPosition } from "./messages.js";
export declare class SoundClock {
    #private;
    constructor(context: AudioContext);
    /**
     * Feed every `position` report.
     *
     * Reports are made when the processor renders, which is ahead of the
     * speaker by the output latency and the device buffer, so a report stays
     * needed until the frame being heard has passed its successor's frame. That
     * is pruned by frame, not by count: at a speed change a report can come every
     * quantum, and with an output latency of a Bluetooth device (a few hundred
     * ms) a fixed count would drop the report for the frame still being heard.
     * While the mapping is not known, only the last `REPORTS` are kept.
     */
    report(position: SoundPosition): void;
    /**
     * Feed a reading of the context clock. `now` is the `performance.now()` at
     * which it is taken, which is what it defaults to; pass it only if it was
     * read immediately before.
     */
    sample(now?: number): void;
    /** Media time being heard at wall time `wall`, or NaN when not playing or not yet known. */
    at(wall: number): number;
    /** The processor's state as last reported; "idle" before the first report. */
    get state(): SoundPosition["state"] | "idle";
    /** Wall time at which the device plays context frame `frame` (outputLatency included); NaN until known. */
    wallOfFrame(frame: number): number;
    /** The context frame being heard at wall time `wall`; NaN until known. */
    frameAt(wall: number): number;
}
