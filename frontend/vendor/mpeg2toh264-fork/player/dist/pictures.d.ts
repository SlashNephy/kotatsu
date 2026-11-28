/**
 * Decode mode's bookkeeping in the worker: which units to decode for the
 * pictures the page asks for, and the buffers the pictures are copied into.
 *
 * Nothing here touches the DOM or the WebAssembly module, so it runs as it
 * is in Node for the tests.
 */
/** What the schedule needs of a unit. A `video` fragment has it all. */
export interface ScheduledUnit {
    readonly start: number;
    readonly restart: boolean;
    readonly intra: boolean;
    /** In decode order. */
    readonly pictures: readonly {
        readonly time: number;
        readonly duration: number;
    }[];
}
/** One unit to decode now. */
export interface DecodeStep<U> {
    readonly unit: U;
    /** Forget the references first: decoding begins again at this unit. */
    readonly reset: boolean;
    /** What `shows` is asked with for each of the unit's pictures. */
    readonly from: number;
    readonly epoch: number;
}
/**
 * Whether a picture decoded for a request starting at `from` is handed out:
 * all but those ending more than one picture before it, which are decoded
 * only as references. The one before `from` is the renderer's neighbour.
 */
export declare function shows(picture: {
    readonly time: number;
    readonly duration: number;
}, from: number): boolean;
/**
 * A request that starts this far before the last one is a jump back, not
 * the clock wavering.
 */
export declare const BACKWARD_TOLERANCE_SECONDS = 0.05;
/**
 * The units of a load in time order, and where decoding stands in them.
 *
 * A request (`need`) names an epoch and a span of media time. Decoding goes
 * on from where it stopped while requests follow on from each other; a new
 * epoch, a jump back, or a jump forward past the unit after the next one
 * starts again at the latest unit a decoder can begin at, at or before the
 * request. Until the store holds one, nothing is decoded. No unit is begun
 * whose pictures all start after the request, unless the decoder is holding
 * back a picture the request reaches: it lets an anchor out only once it
 * decodes the next one.
 *
 * Decoding can begin only at some units, a group of pictures apart, so the
 * units are kept and dropped a group at a time: the earliest one held is
 * always one decoding can begin at, and every picture held can be decoded.
 * `reaches` says whether that goes for a time, by the rule `next` begins
 * decoding by, so that a seek the store is said to reach is one it serves.
 */
export declare class PictureSchedule<U extends ScheduledUnit> {
    #private;
    constructor(keepBehindSeconds: number);
    /** How many units are held. */
    get size(): number;
    /**
     * Whether the last `next` came back empty with nothing left to come for the
     * request: every unit it reaches is decoded, and any other starts after it
     * or will never arrive. Only the playhead moving on asks for more.
     */
    get exhausted(): boolean;
    /** Take a unit the session made. A unit without pictures is not one. */
    add(unit: U): void;
    /**
     * Forget every unit and every request, as for a seek that reopens the
     * input. Requests of epochs before `from` are refused from here on: by
     * default every epoch seen so far, and for a seek the page already started
     * an epoch for -- its request can come before the seek -- that epoch's.
     * The units to come are read for that epoch.
     */
    clear(from?: number): void;
    /** The units to come are read from the input's start. */
    readFromStart(): void;
    /**
     * Whether the pictures at `time` can be decoded from the units held, now
     * or once the units after them arrive: a unit decoding can begin at lies
     * at or before `time`, or the input begins after it. Where neither is so,
     * only reading the input again gives them.
     */
    reaches(time: number): boolean;
    /** The input has ended: no unit is coming after the last one held. */
    end(): void;
    /**
     * Drop the units ending more than the keep-behind span before the
     * playhead, as far as a unit decoding can begin at: those left behind one
     * that was dropped could not be decoded. Never the last such unit before
     * the playhead, nor any after it.
     */
    playhead(time: number): void;
    /**
     * Take a request for the pictures from `from` to `until`. Returns false
     * for one of an epoch a `clear` left behind.
     */
    need(epoch: number, from: number, until: number): boolean;
    /**
     * The unit to decode next for the last request, or null for none now.
     * `heldBack` is the time of the picture the decoder is holding back, or
     * null when it holds none.
     */
    next(heldBack: number | null): DecodeStep<U> | null;
    /** The unit last returned by `next` failed to decode. */
    failed(): void;
    /**
     * The epoch whose pictures have all been handed out, once per epoch: the
     * input has ended and every unit after the start of decoding is done, or
     * none reaches the request at all.
     */
    takeEnd(): number | null;
}
/**
 * Picture buffers of one size, used again once the page hands them back.
 *
 * A buffer that comes back detached, or of another size than the pictures
 * now being made, is left for the collector.
 */
export declare class PictureBuffers {
    #private;
    /** How many buffers have been made, for whoever is measuring. */
    get allocated(): number;
    take(byteLength: number): ArrayBuffer;
    /** Returns whether the buffer will be used again. */
    recycle(data: unknown): boolean;
}
