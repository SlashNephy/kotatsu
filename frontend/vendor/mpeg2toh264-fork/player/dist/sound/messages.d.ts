/**
 * The messages of the sound output stage.
 *
 * Three parties talk: the page (owns the `AudioContext` and the node), the
 * decode Worker (produces PCM), and the AudioWorklet processor (plays it). The
 * page hands the processor one end of a `MessageChannel` and the Worker the
 * other, so PCM never passes through the main thread.
 */
/** The name the processor registers under, for `new AudioWorkletNode`. */
export declare const SOUND_PROCESSOR = "mpeg2toh264-sound";
/** The processor's playback state; see processor.ts for the transitions. */
export type SoundState = "buffering" | "ready" | "playing" | "starved" | "paused" | "ended";
/**
 * Decoded PCM for the processor, Worker → processor on the PCM port. Transfer
 * both buffers: the processor plays from these arrays in place.
 */
export interface SoundPcm {
    type: "pcm";
    epoch: number;
    /** Media seconds of the first frame. */
    start: number;
    /** Frames per second of `channels`. */
    sampleRate: number;
    /** Left and right, the same length. */
    channels: [Float32Array, Float32Array];
    /**
     * Whether these are the two services of a dual-mono stream instead -- the
     * main one first, the second next -- of which the one chosen with
     * `dual-mono` is played on both sides.
     */
    dualMono: boolean;
}
/** No more PCM in this epoch, Worker → processor on the PCM port. */
export interface SoundEnd {
    type: "end";
    epoch: number;
}
/** What the Worker sends on the PCM port. */
export type SoundPcmMessage = SoundPcm | SoundEnd;
/**
 * Processor → Worker on the PCM port: the media time up to which PCM has
 * been taken out of the queue. Sent at each epoch start and about every
 * 100 ms of output, so the Worker can stay a few seconds ahead.
 */
export interface SoundConsumed {
    type: "consumed";
    epoch: number;
    until: number;
}
/** Page → processor on `AudioWorkletNode.port`. */
export type SoundControl = {
    type: "pcm-port";
    port: MessagePort;
}
/**
 * Drop everything queued, accept chunks of `epoch` from now on, discard
 * samples before media time `at`, and buffer again. Epochs must increase:
 * PCM of a later epoch that arrives before this message is held for it.
 */
 | {
    type: "epoch";
    epoch: number;
    at: number;
}
/** Play from the first quantum that starts at or after context frame `at`. */
 | {
    type: "play";
    at: number;
} | {
    type: "pause";
}
/** Playback speed, 0.25 to 4, pitch preserved. Exactly 1 bypasses the time stretch. */
 | {
    type: "rate";
    rate: number;
}
/** Fine speed factor in [0.99, 1.01], applied by resampling (pitch follows). */
 | {
    type: "adjust";
    ratio: number;
}
/** Linear gain, ramped. */
 | {
    type: "volume";
    gain: number;
}
/**
 * Which service of dual-mono sound to play: the second if `sub`. It takes
 * effect at the PCM being taken from the queue now, so it is heard after the
 * stages behind the queue and the output latency, a few tens of ms.
 */
 | {
    type: "dual-mono";
    sub: boolean;
};
/**
 * Processor → page on `AudioWorkletNode.port`: where the output is in media
 * time.
 *
 * The context frame `frame + n` carries the sound of media time
 * `media + n * speed / sampleRate` until the next report, for the frames this
 * report covers (see `frame`). Reports come about every 50 ms of context time,
 * at every state change, on the first quantum after `play` and `epoch`, and
 * wherever that linear mapping stops holding: a speed change (`rate`,
 * `adjust`) or a jump in the media timeline.
 */
export interface SoundPosition {
    type: "position";
    epoch: number;
    /**
     * The context frame this report starts at: the `currentFrame` of the quantum
     * normally, or the frame inside a quantum at which the media timeline jumps
     * or the speed changes.
     */
    frame: number;
    /** Media time of the sound output at `frame`; while not playing, where it stands. */
    media: number;
    /** Media seconds per context second from `frame` on; 0 when not playing. */
    speed: number;
    state: SoundState;
    /** Media time at which the queued PCM ends. */
    queuedUntil: number;
}
