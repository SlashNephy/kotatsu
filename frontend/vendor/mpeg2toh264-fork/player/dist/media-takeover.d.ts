/**
 * Decode mode's media element.
 *
 * In decode mode the `<video>` the player was given plays nothing itself: the
 * pictures are drawn by the deinterlacer and the sound comes out of an
 * AudioWorklet. Everything around it -- the player UI, the captions, the app
 * -- still talks to the element, so its media interface is answered from here
 * for the length of the load: own properties on the element instance shadow
 * the prototype's, as the deinterlacer already does for `videoWidth` and
 * `getVideoPlaybackQuality`, and the media events are dispatched on the
 * element by this class. The element stays the same DOM node, so its place in
 * the document, its style and its parent are untouched. Removing the own
 * properties gives the element back as it was, for a load that passes the
 * video through.
 *
 * Only what the player UI and the app use is here (the list was taken from
 * DPlayer, KonomiTV and aribb24.js); the rest of the element is its own. The
 * behaviour follows the HTML media element where it is the same thing:
 * events after the calling code has run, `play()` refusing with
 * `NotAllowedError` where sound may not start yet, `paused` set and `pause`
 * fired before `ended`, and a `play()` at the end starting over.
 */
/** HTMLMediaElement's readyState values. */
export declare const enum Ready {
    Nothing = 0,
    Metadata = 1,
    CurrentData = 2,
    FutureData = 3,
    EnoughData = 4
}
export interface TakeoverHandlers {
    /** The viewer wants playback to run; the sound may start once it can. */
    play: () => void;
    pause: () => void;
    /** The viewer moved the playhead to `time`. */
    seek: (time: number) => void;
    rate: (rate: number) => void;
    /** The linear gain the sound should have: zero while muted. */
    volume: (gain: number) => void;
    /** The media time being heard now, or NaN while that is not known. */
    heard: () => number;
    /** Let sound play; resolves to whether the browser allows it now. */
    resume: () => Promise<boolean>;
}
export declare class MediaTakeover {
    #private;
    constructor(video: HTMLVideoElement, handlers: TakeoverHandlers);
    /** The gain the sound should have now. */
    get gain(): number;
    get rate(): number;
    get paused(): boolean;
    /** Whether the sound is playing and playback is wanted. */
    get playing(): boolean;
    get readyState(): Ready;
    /** The position the element reports. */
    get currentTime(): number;
    install(): void;
    /** Give the element its own media interface back, with the volume and speed it has now. */
    uninstall(): void;
    /** How long the media is; Infinity for a live input, NaN before it is known. */
    setDuration(duration: number): void;
    setBuffered(range: {
        start: number;
        end: number;
    } | null): void;
    /**
     * Move readyState, with the events a media element fires on the way up,
     * and `waiting` on the way down while playback is wanted. `loadedmetadata`
     * and `loadeddata` come once a load, as the element's do; the others again
     * after every fall, as after a seek.
     */
    setReadyState(state: Ready): void;
    /** A seek's media is in place: `seeked`. */
    seeked(): void;
    /** The sound started playing at media time `media`. */
    soundStarted(media: number): void;
    /**
     * The sound stopped at media time `media`: run out (`waiting` follows from
     * the readyState the caller sets), paused, or at the end of the media.
     */
    soundStopped(media: number, ended: boolean): void;
    /** Where the media begins, for a load whose first media is later than zero. */
    place(time: number): void;
}
