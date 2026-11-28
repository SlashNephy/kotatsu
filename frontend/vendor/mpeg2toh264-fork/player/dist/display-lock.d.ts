/**
 * Decode mode's hold of the sound to the pictures on the screen.
 *
 * The deinterlacer shows decoded pictures against a count of refreshes, as
 * mpv's display sync does: where the screen runs close to a whole number of
 * picture steps a refresh (59.94 fields a second on a 59.95 Hz screen, or
 * twice that at double speed) or of refreshes a step (half speed), every
 * refresh moves the pictures by exactly whole steps, so none is shown twice
 * or passed over; elsewhere it moves them by the refresh. Either way the
 * pictures keep the screen's time, not the sound's, and this class makes the
 * sound follow: it plays the sound a fraction of a percent faster or slower
 * (`adjust`, which the AudioWorklet applies by resampling).
 *
 * The speed has three parts. The rate match is what the pictures' count
 * demands: whole steps per measured refresh, or the speed asked for. The
 * correction takes back the deinterlacer's `avDifference` (the sound's lead
 * over the picture shown, averaged over a second) over `AV_RECOVERY_SECONDS`:
 * rounding, a start that did not fall on a refresh. The drift is what the
 * lead keeps asking for after that -- the output device's clock running
 * faster or slower than `performance.now()`, a few hundredths of a percent --
 * learnt from the lead as it stays, so that none is left standing. With the
 * correction it takes back a change of the drift critically damped.
 *
 * The device's rate is learnt from the lead rather than taken from the sound
 * clock's fit of the context's time: in a context's first second or two its
 * time can run ahead (Edge: by up to 0.7 %), which a fit takes for a faster
 * device for tens of seconds, and a start falls in that time. The lead is
 * what is heard against what is shown.
 *
 * Only a difference past a refresh and 20 ms -- a stall -- is taken back on
 * the pictures' side, by the deinterlacer moving its count. The pitch moves
 * by as little as the speed, which no one hears.
 */
export interface DisplayLockInput {
    /** The time of this animation frame's refresh, on `performance.now()`'s clock. */
    now: number;
    /** The step the pictures change on in seconds, or null while there is none. */
    step: number | null;
    /** The playback speed asked for. */
    rate: number;
    /** Whether the sound plays now. */
    active: boolean;
    /** The deinterlacer's `avDifference` in seconds, or NaN while not known. */
    avDifference: number;
}
export declare class DisplayLock {
    #private;
    /** `drift` is what an earlier lock on the same output device learnt. */
    constructor(send: (adjust: number) => void, drift?: number);
    /** How much faster than asked the output device plays, as learnt so far. */
    get drift(): number;
    /** The speed factor last sent. */
    get adjust(): number;
    /** The refresh period, in ms, or NaN until enough refreshes were seen. */
    get period(): number;
    /**
     * The first refresh at or after `wall` on the grid of those seen, as an
     * animation frame's time; NaN while the period is not known.
     */
    refreshAfter(wall: number): number;
    /** Take one animation frame. */
    frame(input: DisplayLockInput): void;
}
