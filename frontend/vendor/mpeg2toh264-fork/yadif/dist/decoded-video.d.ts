/** One decoded picture. */
export interface DecodedPicture {
    /** Presentation time of the picture's first field, in seconds on the media element's timeline. */
    timestamp: number;
    /**
     * How long the picture is presented, in seconds: up to the timestamp of
     * the picture after it, where one follows on. Normally two field periods.
     * Its fields are spread evenly over it.
     */
    duration: number;
    /** Visible luma size in samples. */
    width: number;
    height: number;
    /** Size the picture is displayed at after its sample aspect ratio, e.g. 1920x1080 for 1440x1080. */
    displayWidth: number;
    displayHeight: number;
    /** Planar 4:2:0, 8-bit, limited range: Y, then Cb, then Cr. Ownership passes to the renderer. */
    data: ArrayBuffer;
    /** Where each plane starts in `data` and its row stride in bytes. The chroma planes hold ceil(width/2) x ceil(height/2) visible samples. */
    planes: readonly [
        {
            offset: number;
            stride: number;
        },
        {
            offset: number;
            stride: number;
        },
        {
            offset: number;
            stride: number;
        }
    ];
    /** False when both fields of the frame are one instant (MPEG-2 progressive_frame). */
    interlaced: boolean;
    topFieldFirst: boolean;
    repeatFirstField: boolean;
    /** YCbCr matrix of the samples. */
    matrix: "bt709" | "bt601";
}
/** Where decoded pictures enter the deinterlacer. */
export interface DecodedVideoInput {
    /**
     * Hand over a picture. Pictures come in presentation order, a decoder's
     * display order, each starting after the one before it since `reset()`,
     * and a little ahead of their time. One out of order throws, and its
     * buffer is released.
     */
    push(picture: DecodedPicture): void;
    /** Forget everything queued or held: a seek, or a new stream. */
    reset(): void;
    /** No more pictures will come; present what is queued. */
    finish(): void;
    /**
     * Set by the producer. Called with a picture's `data` once the renderer no
     * longer needs it, so the buffer can be reused. Every pushed buffer comes
     * back exactly once, including on reset(), finish(), stop and destroy.
     */
    onrelease: ((data: ArrayBuffer) => void) | null;
    /**
     * The media time being heard less that of the picture on the screen, in
     * seconds, averaged over about a second; NaN while pictures are not being
     * shown. Pictures are shown for counted refreshes, in whole picture steps
     * where the screen allows, so a player that plays the sound itself keeps
     * the two together by moving the sound's speed by this.
     */
    avDifference(): number;
    /**
     * Playback is about to start: media time `media` will be heard at `wall`,
     * a `performance.now()` time of this realm, and from then on at the
     * element's playback rate. Pictures then go up from the first refresh at or
     * after `wall`, with the sound, rather than once the element's clock has
     * been seen to move, which takes a reading or two and would pass over the
     * pictures in between. Forgotten at a pause or a `reset()`.
     *
     * Also for a change of the playback rate while playing: `media` and `wall`
     * are then where the sound takes up the new rate. From the change until
     * `wall` the pictures go on at the rate they had.
     */
    startsAt(media: number, wall: number): void;
    /**
     * Where the renderer stands. A start waits for `prepared`; pictures can be
     * pushed before. Decoded pictures have no picture of the element's to fall
     * back on, so at `failed` the producer has nothing to show them with.
     */
    rendererStatus(): RendererStatus;
    /** Set by the producer. Called when `rendererStatus()` may have changed. */
    onrendererstatus: (() => void) | null;
}
/** Where the renderer of decoded pictures stands; see `DecodedVideoInput.rendererStatus`. */
export type RendererStatus = 
/**
 * The GPU has not yet run the renderer's setup, the first draw of every
 * program it draws with included (see `drawOnce`): a picture could miss
 * the refresh it is due in.
 */
{
    state: "preparing";
}
/** Pictures go up in the refresh they are due in. */
 | {
    state: "prepared";
}
/**
 * It has stopped for good: a lost WebGL context, a Worker that died again
 * after its restart, Worker rendering asked for where there is none, or a
 * picture it could not take in. Nothing pushed is shown any more.
 */
 | {
    state: "failed";
    reason: string;
};
/** Throw if the picture's fields or its planes do not describe a picture inside `data`. */
export declare function checkPicture(picture: DecodedPicture): void;
/**
 * Coefficients of limited-range YCbCr to RGB, from each matrix's Kr and Kb:
 * Cr in R, Cb and Cr in G, and Cb in B, as `uMatrix` takes them.
 */
export declare const MATRIX_COEFFICIENTS: Readonly<Record<DecodedPicture["matrix"], Float32Array>>;
/**
 * Conversion of one picture to RGB, one output texel per luma sample, rows
 * from the top as the history textures hold them.
 *
 * Chroma is interpolated linearly from the four nearest chroma samples of the
 * right rows. Horizontally a chroma sample sits on an even luma column
 * (H.262 6.1.1.8). Vertically, in a progressive picture chroma row c sits
 * between luma rows 2c and 2c + 1. In an interlaced picture the chroma rows
 * alternate between the fields as the luma rows do, and a luma row takes its
 * chroma from its own field only: within a field, chroma row k sits at field
 * row 2k + 1/4 in the top field and 2k + 3/4 in the bottom field (H.262
 * Figures 6-2 and 6-3), which puts it where it would be in a progressive
 * frame. Taking rows of the other field would mix two instants wherever
 * anything moves.
 *
 * The functions before `main` use only scalar arithmetic, shifts and the
 * helpers they call, so that a test can evaluate them outside a GPU.
 */
export declare const PICTURE_FRAGMENT_SHADER = "#version 300 es\nprecision highp float;\nprecision highp int;\n\nuniform highp sampler2D uLuma;\nuniform highp sampler2D uCb;\nuniform highp sampler2D uCr;\n/** Whether chroma rows belong alternately to the two fields. */\nuniform bool uInterlaced;\nuniform vec4 uMatrix;\n\nout vec4 fragColor;\n\n/** Position of luma column x in chroma columns. */\nfloat chromaColumn(int x) {\n  return float(x) * 0.5;\n}\n\n/** Position of luma row y in chroma rows, progressive. */\nfloat chromaRow(int y) {\n  return float(y) * 0.5 - 0.25;\n}\n\n/** Position of luma row y among the chroma rows of its own field. */\nfloat chromaFieldRow(int y) {\n  return (float(y >> 1) - 0.25 - 0.5 * float(y & 1)) * 0.5;\n}\n\nvec2 tap(int x, int y) {\n  return vec2(texelFetch(uCb, ivec2(x, y), 0).r, texelFetch(uCr, ivec2(x, y), 0).r);\n}\n\nvec2 chroma(int x, int y, int width, int height) {\n  float u = clamp(chromaColumn(x), 0.0, float(width - 1));\n  int x0 = int(u);\n  int x1 = min(x0 + 1, width - 1);\n  float wx = u - float(x0);\n  int y0;\n  int y1;\n  float wy;\n  if (uInterlaced) {\n    int parity = y & 1;\n    int rows = (height + 1 - parity) >> 1;\n    float v = clamp(chromaFieldRow(y), 0.0, float(rows - 1));\n    int row = int(v);\n    y0 = 2 * row + parity;\n    y1 = 2 * min(row + 1, rows - 1) + parity;\n    wy = v - float(row);\n  } else {\n    float v = clamp(chromaRow(y), 0.0, float(height - 1));\n    y0 = int(v);\n    y1 = min(y0 + 1, height - 1);\n    wy = v - float(y0);\n  }\n  return mix(mix(tap(x0, y0), tap(x1, y0), wx), mix(tap(x0, y1), tap(x1, y1), wx), wy);\n}\n\n/** Limited-range YCbCr, as texture values, to full-range RGB. */\nvec3 rgb(float luma, vec2 c) {\n  float l = (luma * 255.0 - 16.0) / 219.0;\n  float b = (c.x * 255.0 - 128.0) / 224.0;\n  float r = (c.y * 255.0 - 128.0) / 224.0;\n  return clamp(vec3(l + uMatrix.x * r, l - uMatrix.y * b - uMatrix.z * r, l + uMatrix.w * b), 0.0, 1.0);\n}\n\nvoid main() {\n  ivec2 at = ivec2(gl_FragCoord.xy);\n  ivec2 size = textureSize(uCb, 0);\n  fragColor = vec4(rgb(texelFetch(uLuma, at, 0).r, chroma(at.x, at.y, size.x, size.y)), 1.0);\n}\n";
/** Uploads decoded pictures and converts them into RGBA textures of the same size. */
export declare class PictureConverter {
    #private;
    constructor(gl: WebGL2RenderingContext);
    /** Allocate the plane textures for pictures of this luma size. */
    resize(width: number, height: number): void;
    /**
     * Upload the picture's planes and draw its RGB into `target`, a texture of
     * the size last given to resize(). Throws if `target` cannot be drawn into.
     */
    convert(picture: DecodedPicture, target: WebGLTexture): void;
    destroy(): void;
}
