/**
 * A steady estimate of the media element's playback position.
 *
 * `currentTime` is the position when the element last updated it, and some
 * browsers update it in steps of tens of milliseconds, so while the element
 * plays a reading is at or a little behind the true position and never ahead
 * of it (as it starts it can be, see below). The clock runs on
 * the caller's wall clock at the playback rate and is pulled, slowly, towards
 * the highest reading of the last second relative to itself: the reading
 * taken nearest a step is the closest to the truth.
 *
 * After a hold the clock stays on the readings until they show the element
 * playing, and a start is shown only by the change after it. An element that
 * has been told to play may move its time a little and then stand still
 * before the sound starts: Chromium, for one, extrapolates the position from
 * wall time from its first audio callback, before the device has played any
 * of it, and then holds the position until the sound catches up. A clock
 * started on that move runs ahead of the sound.
 *
 * Where the reading changes at every reading, the next reading settles the
 * question, so the clock waits for it: it runs once two changes in a row
 * have come at playing pace. Where the element updates its reading in steps,
 * waiting costs a whole step: the readings between two updates stand still
 * whether the element plays or not, so they cannot tell, and the pictures
 * due in that step pass meanwhile. So a clock that has seen the reading stand
 * between updates in all of its last second of running runs from the first
 * change after a hold, and stops again if the next change is overdue or out
 * of pace. A start taken back that way is not tried again until a start on
 * two changes has confirmed that the element plays.
 *
 * A running clock stops when the reading stands longer than it has been
 * seen to stand while the element played, and one reading more: the element
 * has stopped, though it has not said so, and pictures shown on would run
 * ahead of the sound.
 *
 * While it waits, the position is the reading, except where the reading has
 * been seen to run ahead of the sound when the element starts: at each start
 * after the element was stopped, the clock runs from a reading on the sound's
 * line, and so learns how far ahead of the sound the reading's first change
 * was. In Chromium that is the device's output delay, about 50 ms at each
 * start; in Firefox the reading leads by less than a reading, if at all.
 * Where it has led by more than a reading at the last starts, the sound
 * while the clock waits is where the reading's first change was, less that
 * lead, and playing on from there.
 */
export declare class MediaClock {
    #private;
    /** The position at `wall`, in media seconds, or NaN while not known. */
    at(wall: number): number;
    /** The rate the clock advances at: the playback rate, or 0 while held. */
    get rate(): number;
    /**
     * Where the clock was last put by a reading instead of running there: at a
     * hold, or on a reading too far from it. Playback has been continuous from
     * here, so nothing that ended at or before it has been due since. NaN
     * before the first reading.
     */
    get origin(): number;
    /**
     * Stop at the next reading and wait for the readings to show playing: a
     * seek, a pause or a stall the element has reported. Readings taken across
     * the event say nothing about the position after it.
     */
    hold(): void;
    /**
     * Take a reading of `currentTime` made at `wall`. `playing` is whether the
     * element is playing at all: not paused, seeking or ended. Returns whether
     * the clock started running again from this reading.
     */
    observe(wall: number, currentTime: number, playbackRate: number, playing: boolean): boolean;
}
