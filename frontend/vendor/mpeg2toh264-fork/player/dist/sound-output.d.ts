/**
 * Decode mode's way to the speaker for one load, on the page: the
 * AudioWorklet that plays the decode worker's PCM, in a context and against
 * a clock of what is being heard that the player keeps from load to load --
 * a context made anew for each load would start each one with the clock's
 * mapping to wall time not yet settled, when the first pictures need it.
 *
 * The worklet's module loads asynchronously, while the worker may start
 * sending PCM at once: the port the worker is given buffers what it is sent
 * until the processor takes it, and the commands for the processor wait here
 * until it exists.
 */
import { SoundClock, type SoundControl, type SoundPosition } from "./sound/index.js";
export interface SoundOutputHandlers {
    /** Every report of the processor, after the clock has taken it. */
    report: (position: SoundPosition) => void;
    /** The processor could not be loaded or failed. */
    error: (error: Error) => void;
    /** Each animation frame, with its refresh time, after the clock has been read. */
    frame: (now: number) => void;
}
export declare class SoundOutput {
    #private;
    readonly context: AudioContext;
    readonly clock: SoundClock;
    /** The end of the PCM channel that belongs to the decode worker. */
    readonly workerPort: MessagePort;
    constructor(context: AudioContext, clock: SoundClock, handlers: SoundOutputHandlers);
    /** Give the processor a command, now or as soon as it exists. */
    control(message: SoundControl): void;
    /**
     * Let the context run. A browser that wants a gesture before sound plays
     * keeps it suspended outside one; the answer is whether it runs now.
     *
     * Such a browser does not refuse the resume either: Chrome leaves its
     * promise pending until a gesture comes, which may be never. So the answer
     * is not waited for past a short while, longer inside a gesture, where a
     * resume is let through; the context still starts if one comes later.
     */
    resume(): Promise<boolean>;
    close(): void;
}
