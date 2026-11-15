const R = `#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;
function b(s, e, t) {
  const i = s.createProgram(), A = G(s, s.VERTEX_SHADER, t), r = G(s, s.FRAGMENT_SHADER, e);
  if (s.attachShader(i, A), s.attachShader(i, r), s.linkProgram(i), s.deleteShader(A), s.deleteShader(r), !s.getProgramParameter(i, s.LINK_STATUS)) {
    const o = s.getProgramInfoLog(i);
    throw s.deleteProgram(i), new Error(
      `the deinterlacer failed to link: ${o ?? "no reason given"}`
    );
  }
  return i;
}
function G(s, e, t) {
  const i = s.createShader(e);
  if (!i) throw new Error("the deinterlacer could not create a shader");
  if (s.shaderSource(i, t), s.compileShader(i), !s.getShaderParameter(i, s.COMPILE_STATUS)) {
    const A = s.getShaderInfoLog(i);
    throw s.deleteShader(i), new Error(
      `the deinterlacer failed to compile: ${A ?? "no reason given"}`
    );
  }
  return i;
}
const K = `#version 300 es
precision highp float;
uniform sampler2D uTexture;
in vec2 vTextureCoord;
uniform vec3 uTextColor;
uniform vec3 uBackColor;
out vec4 fragColor;

void main() {
  float a = texture(uTexture, vTextureCoord)[3];
  fragColor = vec4(uTextColor * a + uBackColor * (1.0 - a), 1.0);
}
`, J = `#version 300 es
precision highp float;
in vec4 aVertexPosition;
in vec2 aTextureCoord;
uniform mat4 uMatrix;
uniform mat3 uUvMatrix;
out vec2 vTextureCoord;
out vec3 vTextColor;

void main() {
  gl_Position = uMatrix * aVertexPosition;
  vTextureCoord = vec2(uUvMatrix * vec3(aTextureCoord, 1.0));
}
`;
function q(s, e) {
  const t = b(s, K, J), i = s.getAttribLocation(t, "aVertexPosition"), A = s.getAttribLocation(t, "aTextureCoord"), r = s.getUniformLocation(t, "uTexture"), o = s.getUniformLocation(t, "uMatrix"), a = s.getUniformLocation(t, "uUvMatrix"), h = s.getUniformLocation(t, "uTextColor"), c = s.getUniformLocation(t, "uBackColor");
  if (r == null || o == null || a == null || h == null || c == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const n = s.createBuffer(), u = s.createBuffer();
  return {
    gl: s,
    ...ee(s, e),
    program: t,
    programUniforms: {
      vertex: i,
      textureCoord: A,
      texture: r,
      matrix: o,
      uvMatrix: a,
      textColor: h,
      backColor: c
    },
    positionBuffer: n,
    textureBuffer: u
  };
}
function ee(s, e) {
  const t = new OffscreenCanvas(0, 0), i = t.getContext("2d"), A = /* @__PURE__ */ new Map();
  let r = 0;
  const o = 0;
  let a = 1;
  i.font = e, i.fillStyle = "white";
  for (let c = 32; c < 128; c++) {
    const n = String.fromCharCode(c), u = i.measureText(n), d = Math.ceil(
      u.actualBoundingBoxDescent + u.actualBoundingBoxAscent + 1
    ), l = Math.ceil(
      u.actualBoundingBoxLeft + u.actualBoundingBoxRight + 1
    );
    A.set(n, {
      x: r,
      y: o,
      width: l,
      height: d,
      metrics: u
    }), a = Math.max(a, d), r += l;
  }
  t.width = r, t.height = a, i.font = e, i.fillStyle = "white";
  for (const [c, n] of A)
    i.fillText(
      c,
      Math.floor(n.x + n.metrics.actualBoundingBoxLeft + 1),
      Math.floor(n.metrics.actualBoundingBoxAscent + 1)
    );
  const h = s.createTexture();
  return s.bindTexture(s.TEXTURE_2D, h), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MIN_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MAG_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_S, s.CLAMP_TO_EDGE), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_T, s.CLAMP_TO_EDGE), s.texImage2D(s.TEXTURE_2D, 0, s.RGBA, s.RGBA, s.UNSIGNED_BYTE, t), { fontTexture: h, chars: A, textureSize: { width: r, height: a } };
}
function te(s) {
  const e = s.gl;
  e.deleteBuffer(s.positionBuffer), e.deleteBuffer(s.textureBuffer), e.deleteTexture(s.fontTexture), e.deleteProgram(s.program);
}
function ie(s, e, t, i, A, r, o) {
  const a = [], h = [], c = t;
  for (const d of e) {
    if (d === `
`) {
      t = c, i += o;
      continue;
    }
    const l = s.chars.get(d);
    if (l == null)
      continue;
    if (l.width === 1) {
      t += l.metrics.width;
      continue;
    }
    const E = Math.floor(t - l.metrics.actualBoundingBoxLeft), v = Math.floor(i - l.metrics.actualBoundingBoxAscent), T = E + l.width, x = v + l.height;
    a.push(E, v), h.push(l.x, l.y), a.push(E, x), h.push(l.x, l.y + l.height), a.push(E + l.width, x), h.push(l.x + l.width, l.y + l.height), a.push(T, x), h.push(l.x + l.width, l.y + l.height), a.push(E, v), h.push(l.x, l.y), a.push(T, v), h.push(l.x + l.width, l.y), t += l.metrics.width;
  }
  const n = s.gl;
  n.useProgram(s.program), n.bindBuffer(n.ARRAY_BUFFER, s.positionBuffer), n.bufferData(n.ARRAY_BUFFER, new Float32Array(a), n.STATIC_DRAW), n.vertexAttribPointer(
    s.programUniforms.vertex,
    2,
    n.FLOAT,
    !1,
    0,
    0
  ), n.enableVertexAttribArray(s.programUniforms.vertex), n.bindBuffer(n.ARRAY_BUFFER, s.textureBuffer), n.bufferData(
    n.ARRAY_BUFFER,
    new Float32Array(h),
    n.STATIC_DRAW
  ), n.vertexAttribPointer(
    s.programUniforms.textureCoord,
    2,
    n.FLOAT,
    !1,
    0,
    0
  ), n.enableVertexAttribArray(s.programUniforms.textureCoord), n.activeTexture(n.TEXTURE0), n.bindTexture(n.TEXTURE_2D, s.fontTexture), n.uniform1i(s.programUniforms.texture, 0), n.uniform3fv(s.programUniforms.textColor, [1, 1, 1]), n.uniform3fv(s.programUniforms.backColor, [0, 0, 0]);
  function u(d, l, E) {
    const v = [];
    for (let T = 0; T < l; T++)
      for (let x = 0; x < d; x++)
        v.push(E[x * d + T]);
    return v;
  }
  n.uniformMatrix4fv(s.programUniforms.matrix, !1, u(4, 4, [
    1 / (A / 2),
    0,
    0,
    -1,
    0,
    -2 / r,
    0,
    1,
    0,
    0,
    1,
    0,
    0,
    0,
    0,
    1
  ])), n.uniformMatrix3fv(s.programUniforms.uvMatrix, !1, u(3, 3, [
    1 / s.textureSize.width,
    0,
    0,
    0,
    1 / s.textureSize.height,
    0,
    0,
    0,
    1
  ])), n.viewport(0, 0, A, r), n.enable(n.BLEND), n.blendFunc(n.SRC_ALPHA, n.ONE_MINUS_SRC_ALPHA), n.drawArrays(n.TRIANGLES, 0, a.length / 2), n.disable(n.BLEND);
}
const f = {
  /** This frame's first field is the previous frame's first field. */
  firstRepeatsPrevious: 0,
  /** This frame's second field is the next frame's second field. */
  secondRepeatsNext: 1,
  /** This frame's second field is the previous frame's second field. */
  secondRepeatsPrevious: 2,
  /** The previous frame's second field was the one before's. */
  previousSecondRepeated: 3,
  /** This frame's first field is the next frame's first field. */
  firstRepeatsNext: 4,
  /** The previous frame's first field was the one before's. */
  previousFirstRepeated: 5,
  phase: 6
}, p = 7, $ = 2, Ae = 16, se = 4e-3, k = 1, re = 2, S = 5, ne = {
  a: "uA",
  b: "uB",
  fieldMetrics: "uFieldMetrics",
  first: "uFirst",
  size: "uSize"
}, Q = 16, V = 8, oe = `#version 300 es

precision highp float;

uniform sampler2D uA;
uniform sampler2D uB;
uniform sampler2D uFieldMetrics;

/** The parity of the field captured first. */
uniform int uFirst;
uniform ivec2 uSize;

layout(location = 0) out vec4 outSecond;
layout(location = 1) out vec4 outFirst;

float luma(vec3 c)
{
  return dot(c, vec3(0.2126, 0.7152, 0.0722));
}

vec4 measure(float diff, float signedDiff, float total, float threshold)
{
  diff /= total;
  return vec4(signedDiff / total, diff > threshold ? 1.0 : 0.0, diff, 1.0);
}

void main()
{
  const int BLOCK_W = ${Q};
  const int BLOCK_H = ${V};

  ivec2 block = ivec2(gl_FragCoord.xy);
  ivec2 base = ivec2(block.x * BLOCK_W, block.y * BLOCK_H * 2);

  // Even and odd frame lines, summed apart.
  float diffEven = 0.0;
  float diffOdd = 0.0;
  float signedEven = 0.0;
  float signedOdd = 0.0;
  int totalEven = 0;
  int totalOdd = 0;

  for (int y = 0; y < BLOCK_H; ++y) {
    for (int x = 0; x < BLOCK_W; ++x) {
      ivec2 p = base + ivec2(x, y * 2);

      if (p.x < uSize.x && p.y < uSize.y) {
        float a = luma(texelFetch(uA, p, 0).rgb);
        float b = luma(texelFetch(uB, p, 0).rgb);

        diffEven += abs(a - b);
        signedEven += a - b;
        totalEven += 1;
      }
      p.y += 1;
      if (p.x < uSize.x && p.y < uSize.y) {
        float a = luma(texelFetch(uA, p, 0).rgb);
        float b = luma(texelFetch(uB, p, 0).rgb);

        diffOdd += abs(a - b);
        signedOdd += a - b;
        totalOdd += 1;
      }
    }
  }

  float run = texelFetch(uFieldMetrics, ivec2(${f.phase}, 0), 0)[1];
  float threshold = run == 0.0 ? 0.025 : (run <= 10.0 ? 0.11 : 0.15);

  vec4 even = measure(diffEven, signedEven, float(totalEven), threshold);
  vec4 odd = measure(diffOdd, signedOdd, float(totalOdd), threshold);
  outFirst = uFirst == 0 ? even : odd;
  outSecond = uFirst == 0 ? odd : even;
}
`, he = {
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, g = 8, ae = `#version 300 es
precision highp float;

uniform sampler2D uSecond;
uniform sampler2D uFirst;

uniform ivec2 uSize;

layout(location = 0) out vec4 outSecond;
layout(location = 1) out vec4 outFirst;

void main()
{
  ivec2 dst = ivec2(gl_FragCoord.xy);
  ivec2 base = dst * ${g};

  vec4 second = vec4(0.0);
  vec4 first = vec4(0.0);

  for (int y = 0; y < ${g}; ++y) {
    for (int x = 0; x < ${g}; ++x) {
      ivec2 p = base + ivec2(x, y);

      if (p.x < uSize.x && p.y < uSize.y) {
        second += texelFetch(uSecond, p, 0);
        first += texelFetch(uFirst, p, 0);
      }
    }
  }

  outSecond = second;
  outFirst = first;
}
`, ce = {
  previous: "uPrevious",
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, le = `#version 300 es
precision highp float;

uniform sampler2D uPrevious;
uniform sampler2D uSecond;
uniform sampler2D uFirst;

/** The size of the reduced comparisons. */
uniform ivec2 uSize;

out vec4 outValue;

vec4 previous(int metric) {
  return texelFetch(uPrevious, ivec2(metric, 0), 0);
}

/**
 * Fold what REDUCTION left into one texel. Two fields apart by more than
 * FIELD_SIGNED_MEAN_MAX on average count every block as differing.
 */
vec4 fold(sampler2D reduced) {
  vec4 sum = vec4(0.0);
  for (int y = 0; y < uSize.y; ++y) {
    for (int x = 0; x < uSize.x; ++x) {
      sum += texelFetch(reduced, ivec2(x, y), 0);
    }
  }
  float blocks = max(sum[3], 1.0);
  float signedMean = sum[0] / blocks;
  float differing = abs(signedMean) > ${se} ? sum[3] : sum[1];
  return vec4(signedMean, differing, sum[2] / blocks, sum[3]);
}

bool same(float differing) {
  return differing <= ${$}.0;
}

bool differs(float differing) {
  return differing >= ${Ae}.0;
}

/** The phase texel, (phase, run), from the six comparisons' differing counts. */
vec4 decide(vec4 last, float dFirstRepeatsPrevious, float dSecondRepeatsNext,
            float dSecondRepeatsPrevious, float dPreviousSecondRepeated,
            float dFirstRepeatsNext, float dPreviousFirstRepeated)
{
  bool firstRepeatsPrevious = same(dFirstRepeatsPrevious);
  bool secondRepeatsNext = same(dSecondRepeatsNext);
  bool secondRepeatsPrevious = same(dSecondRepeatsPrevious);
  bool previousSecondRepeated = same(dPreviousSecondRepeated);
  bool firstRepeatsNext = same(dFirstRepeatsNext);
  bool previousFirstRepeated = same(dPreviousFirstRepeated);
  bool still = (firstRepeatsPrevious && secondRepeatsPrevious) || (secondRepeatsNext && firstRepeatsNext);

  int previous = int(last[0]);
  float run = last[1];
  // What each phase looks like: one field repeats, the other plainly does not.
  bool looks1 = firstRepeatsPrevious && differs(dSecondRepeatsPrevious);
  bool looks2 = secondRepeatsNext && differs(dFirstRepeatsNext);
  bool looks3 = secondRepeatsPrevious && differs(dFirstRepeatsPrevious);
  bool looks4 = previousSecondRepeated && differs(dPreviousFirstRepeated);
  bool looks5 = firstRepeatsNext && differs(dSecondRepeatsNext);

  int expected = previous == 0 ? 0 : (previous == 5 ? 1 : previous + 1);
  bool expectedRepeat =
    expected == 1 ? firstRepeatsPrevious :
    expected == 2 ? secondRepeatsNext :
    expected == 3 ? secondRepeatsPrevious :
    expected == 4 ? previousSecondRepeated :
    expected == 5 ? firstRepeatsNext : false;
  bool expectedLooks =
    expected == 1 ? looks1 :
    expected == 2 ? looks2 :
    expected == 3 ? looks3 :
    expected == 4 ? looks4 :
    expected == 5 ? looks5 : false;
  bool believed = run >= ${S}.0;
  bool carried = believed ? expectedRepeat : expectedLooks;

  int phase = 0;
  if (expected != 0 && carried) {
    phase = expected;
    run += 1.0;
  } else if (expected != 0 && (still || expectedRepeat)) {
    // Uninformative: keeps the cycle's place, counts only once believed.
    phase = expected;
    if (believed) run += 1.0;
  } else if (looks1) {
    phase = 1;
  } else if (looks2) {
    phase = 2;
  } else if (looks3) {
    phase = 3;
  } else if (looks4) {
    phase = 4;
  } else if (looks5) {
    phase = 5;
  }
  if (phase != expected) run = phase != 0 ? 1.0 : 0.0;
  return vec4(float(phase), run, 0.0, 0.0);
}

void main()
{
  int metric = int(gl_FragCoord.x);
  // The comparisons against the next frame become, a frame later, the ones
  // against the previous frame, and those the ones before.
  if (metric == ${f.firstRepeatsPrevious}) {
    outValue = previous(${f.firstRepeatsNext});
  } else if (metric == ${f.secondRepeatsNext}) {
    outValue = fold(uSecond);
  } else if (metric == ${f.secondRepeatsPrevious}) {
    outValue = previous(${f.secondRepeatsNext});
  } else if (metric == ${f.previousSecondRepeated}) {
    outValue = previous(${f.secondRepeatsPrevious});
  } else if (metric == ${f.firstRepeatsNext}) {
    outValue = fold(uFirst);
  } else if (metric == ${f.previousFirstRepeated}) {
    outValue = previous(${f.firstRepeatsPrevious});
  } else {
    outValue = decide(
      previous(${f.phase}),
      previous(${f.firstRepeatsNext})[1],
      fold(uSecond)[1],
      previous(${f.secondRepeatsNext})[1],
      previous(${f.secondRepeatsPrevious})[1],
      fold(uFirst)[1],
      previous(${f.firstRepeatsPrevious})[1]
    );
  }
}
`, ue = {
  prev: "uPrev",
  cur: "uCur",
  next: "uNext",
  size: "uSize",
  parity: "uParity",
  tff: "uTff",
  spatialCheck: "uSpatialCheck",
  debug: "uDebug",
  film: "uFilm",
  second: "uSecond",
  phase: "uPhase",
  fieldMetrics: "uFieldMetrics"
}, fe = `#version 300 es
precision highp float;
precision highp int;

uniform sampler2D uPrev;
uniform sampler2D uCur;
uniform sampler2D uNext;
uniform sampler2D uFieldMetrics;
/** The size of a frame in texels. */
uniform ivec2 uSize;
/** The parity of the lines that are kept; the others are interpolated. */
uniform int uParity;
/** Whether the first field of a frame is its top field. */
uniform int uTff;
/** Whether the temporal bound is widened by the local vertical range. */
uniform bool uSpatialCheck;
uniform bool uDebug;
uniform bool uFilm;
uniform bool uSecond;
uniform int uPhase;

out vec4 fragColor;

/**
 * A texel, with the edges of the frame mirrored.
 *
 * The reference reflects its line offsets on the first and last line rather
 * than reading outside the frame, and this is the same thing said once.
 */
vec3 fetch(sampler2D image, int x, int y) {
  int line = y < 0 ? -y : (y >= uSize.y ? 2 * (uSize.y - 1) - y : y);
  return texelFetch(image, ivec2(clamp(x, 0, uSize.x - 1), clamp(line, 0, uSize.y - 1)), 0).rgb;
}

/**
 * Interpolate the missing line along whichever direction the picture runs in.
 *
 * a..g are the seven texels of the line above and h..n those of the line
 * below, both centred on the pixel being built. The straight vertical average
 * is the starting point, and each candidate direction is taken only if the
 * three differences across it are smaller than the best so far; the steeper
 * pair of directions is only considered when the shallower one was an
 * improvement, which is what keeps a busy picture from finding an edge that is
 * not there.
 */
vec3 spatialPredictor(vec3 a, vec3 b, vec3 c, vec3 d, vec3 e, vec3 f, vec3 g,
                      vec3 h, vec3 i, vec3 j, vec3 k, vec3 l, vec3 m, vec3 n) {
  vec3 pred = (d + k) * 0.5;
  vec3 best = abs(c - j) + abs(d - k) + abs(e - l);

  vec3 score = abs(b - k) + abs(c - l) + abs(d - m);
  vec3 taken = vec3(lessThan(score, best));
  pred = mix(pred, (c + l) * 0.5, taken);
  best = mix(best, score, taken);

  score = abs(a - l) + abs(b - m) + abs(c - n);
  taken *= vec3(lessThan(score, best));
  pred = mix(pred, (b + m) * 0.5, taken);
  best = mix(best, score, taken);

  score = abs(d - i) + abs(e - j) + abs(f - k);
  taken = vec3(lessThan(score, best));
  pred = mix(pred, (e + j) * 0.5, taken);
  best = mix(best, score, taken);

  score = abs(e - h) + abs(f - i) + abs(g - j);
  taken *= vec3(lessThan(score, best));
  pred = mix(pred, (f + i) * 0.5, taken);

  return pred;
}

/**
 * Hold the spatial guess to what the moving picture allows.
 *
 * p2 is where the line would be if nothing moved -- the average of the same
 * line in the two frames that bracket this moment -- and the three temporal
 * differences say how much did move. The spatial guess is then clamped to that
 * distance from p2: still picture, and the answer is the line that is really
 * there; motion, and the interpolation is free to take over.
 */
vec3 temporalPredictor(vec3 A, vec3 B, vec3 C, vec3 D, vec3 E, vec3 F,
                       vec3 G, vec3 H, vec3 I, vec3 J, vec3 K, vec3 L,
                       vec3 spatialPred, bool skipCheck) {
  vec3 p0 = (C + H) * 0.5;
  vec3 p1 = F;
  vec3 p2 = (D + I) * 0.5;
  vec3 p3 = G;
  vec3 p4 = (E + J) * 0.5;

  vec3 tdiff0 = abs(D - I) * 0.5;
  vec3 tdiff1 = (abs(A - F) + abs(B - G)) * 0.5;
  vec3 tdiff2 = (abs(K - F) + abs(G - L)) * 0.5;

  vec3 diff = max(tdiff0, max(tdiff1, tdiff2));

  if (!skipCheck) {
    vec3 hi = max(p2 - p3, max(p2 - p1, min(p0 - p1, p4 - p3)));
    vec3 lo = min(p2 - p3, min(p2 - p1, max(p0 - p1, p4 - p3)));
    diff = max(diff, max(lo, -hi));
  }

  return clamp(spatialPred, p2 - diff, p2 + diff);
}

/**
 * Build one interpolated pixel.
 *
 * prev2 and next2 are the frames the missing line is bracketed by, which is
 * not the same pair as prev and next: the field being rebuilt is half a frame
 * from one of its neighbours and one and a half from the other, and it is the
 * near pair that says what the picture looked like around this moment. prev
 * and next themselves are still read, for the two motion measurements.
 */
vec3 filterPixel(sampler2D prev2, sampler2D next2, int x, int y) {
  vec3 a = fetch(uCur, x - 3, y - 1);
  vec3 b = fetch(uCur, x - 2, y - 1);
  vec3 c = fetch(uCur, x - 1, y - 1);
  vec3 d = fetch(uCur, x, y - 1);
  vec3 e = fetch(uCur, x + 1, y - 1);
  vec3 f = fetch(uCur, x + 2, y - 1);
  vec3 g = fetch(uCur, x + 3, y - 1);

  vec3 h = fetch(uCur, x - 3, y + 1);
  vec3 i = fetch(uCur, x - 2, y + 1);
  vec3 j = fetch(uCur, x - 1, y + 1);
  vec3 k = fetch(uCur, x, y + 1);
  vec3 l = fetch(uCur, x + 1, y + 1);
  vec3 m = fetch(uCur, x + 2, y + 1);
  vec3 n = fetch(uCur, x + 3, y + 1);

  // Within three texels of either side there is no room to look along an edge,
  // so the reference takes the vertical average there and so does this.
  bool interior = x >= 3 && x + 3 < uSize.x;
  vec3 spatialPred = interior ? spatialPredictor(a, b, c, d, e, f, g, h, i, j, k, l, m, n)
                              : (d + k) * 0.5;

  vec3 A = fetch(uPrev, x, y - 1);
  vec3 B = fetch(uPrev, x, y + 1);
  vec3 C = fetch(prev2, x, y - 2);
  vec3 D = fetch(prev2, x, y);
  vec3 E = fetch(prev2, x, y + 2);
  vec3 F = d;
  vec3 G = k;
  vec3 H = fetch(next2, x, y - 2);
  vec3 I = fetch(next2, x, y);
  vec3 J = fetch(next2, x, y + 2);
  vec3 K = fetch(uNext, x, y - 1);
  vec3 L = fetch(uNext, x, y + 1);

  // The first and last line the filter builds have only one line of picture
  // outside them, so the range the spatial check would be measured over is not
  // there. The reference drops the check on those two lines.
  bool skipCheck = !uSpatialCheck || y < 2 || y + 2 >= uSize.y;
  return temporalPredictor(A, B, C, D, E, F, G, H, I, J, K, L, spatialPred, skipCheck);
}

int firstParity() {
  return uTff != 0 ? 0 : 1;
}

bool same(int metric) {
  return texelFetch(uFieldMetrics, ivec2(metric, 0), 0)[1] <= ${$}.0;
}

/** The pulldown phase the detection gave this frame, or 0. See film-shader.ts. */
int detectedPhase() {
  return int(texelFetch(uFieldMetrics, ivec2(${f.phase}, 0), 0)[0]);
}

bool isMixedPhase(int phase) {
  return phase == ${k} || phase == ${re};
}

const int DEBUG_BAR_CELL = 32;
const int DEBUG_BAR_WIDTH = DEBUG_BAR_CELL * 7;
const int DEBUG_BAR_HEIGHT = 16;

vec3 debugBar(int x) {
  int cell = x / DEBUG_BAR_CELL;
  bool lit = x % DEBUG_BAR_CELL >= DEBUG_BAR_CELL - 4;
  if (cell == 6) return lit || uSecond ? vec3(1.0, 0.0, 0.0) : vec3(0.0);
  return lit || same(cell) ? vec3(1.0) : vec3(0.0);
}

const int DIGIT_WIDTH = 24;
const int DIGIT_HEIGHT = 60;

bool digitLit(int digit, int x, int y) {
  bool a = y < 20;
  bool b = x >= 20 && y < 40;
  bool c = x >= 20 && y >= 40;
  bool d = y >= 56;
  bool e = x < 4 && y >= 40;
  bool f = x < 4 && y < 40;
  bool g = y >= 36 && y < 40;
  switch (digit) {
    case 1: return b || c;
    case 2: return a || b || d || e || g;
    case 3: return a || b || c || d || g;
    case 4: return b || c || f || g;
    case 5: return a || c || d || f || g;
    default: return false;
  }
}

void main() {
  ivec2 at = ivec2(gl_FragCoord.xy);
  int x = at.x;
  // The framebuffer counts its rows from the bottom and a frame from the top.
  int y = uSize.y - 1 - at.y;

  vec3 rgb;
  if (uFilm && uDebug && y < DEBUG_BAR_HEIGHT && x < DEBUG_BAR_WIDTH) {
    rgb = debugBar(x);
  } else if (uFilm && uDebug && x < DIGIT_WIDTH && y < DIGIT_HEIGHT && uPhase > 0) {
    rgb = digitLit(uPhase, x, y) ? vec3(1.0, 0.0, 0.0) : vec3(0.0);
  } else if (uFilm && !uSecond && isMixedPhase(detectedPhase())) {
    rgb = (y & 1) == firstParity() ? texelFetch(uCur, ivec2(x, y), 0).rgb
                                   : texelFetch(uPrev, ivec2(x, y), 0).rgb;
  } else if (uFilm && !uSecond && detectedPhase() != 0) {
    // One film frame, whole.
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
  } else if ((y & 1) == uParity) {
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
  } else if ((uParity ^ uTff) != 0) {
    // The first field of the frame: the moment it holds sits between the
    // previous frame and the second field of this one.
    rgb = filterPixel(uPrev, uCur, x, y);
  } else {
    rgb = filterPixel(uCur, uNext, x, y);
  }
  fragColor = vec4(rgb, 1.0);
}
`, D = { phase: 0, run: 0 };
function L(s, e, t) {
  return Object.fromEntries(
    Object.entries(t).map(([i, A]) => [
      i,
      s.getUniformLocation(e, A)
    ])
  );
}
class de {
  #t;
  #c;
  #e;
  #n;
  #E;
  #s;
  #a;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #f = null;
  #d = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #A = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #o = null;
  #i = null;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(p * 4);
  #h = 0;
  #v = 0;
  constructor(e) {
    this.#t = e, this.#c = b(
      e,
      oe,
      R
    ), this.#e = L(
      e,
      this.#c,
      ne
    ), this.#n = b(
      e,
      ae,
      R
    ), this.#E = L(
      e,
      this.#n,
      he
    ), this.#s = b(
      e,
      le,
      R
    ), this.#a = L(e, this.#s, ce);
  }
  /** The newest measurements, or null before any frame has been measured. */
  get texture() {
    return this.#A?.[this.#r]?.textures[0] ?? null;
  }
  /** The size of the frames to be measured, which sizes the block grid. */
  resize(e, t) {
    e === this.#h && t === this.#v || (this.#h = e, this.#v = t, this.#x());
  }
  /** Forget every measurement: the next frame starts a cycle from nothing. */
  reset() {
    const e = this.#t;
    e.deleteSync(this.#i), this.#i = null;
    const t = this.#A?.[this.#r];
    if (!t) return;
    const i = new Float32Array(p * 4);
    for (let A = 0; A < f.phase; A++)
      i[A * 4 + 1] = 1;
    e.bindTexture(e.TEXTURE_2D, t.textures[0] ?? null), e.texSubImage2D(
      e.TEXTURE_2D,
      0,
      0,
      0,
      p,
      1,
      e.RGBA,
      e.FLOAT,
      i
    );
  }
  /**
   * Detect the pulldown phase of `cur`, the frame being filtered, against
   * `next`, and start reading it back. Only the two comparisons against the
   * next frame are measured; the rest are earlier ones moved along a frame.
   * `first` is the parity of the field that was captured first.
   */
  detect(e, t, i) {
    const A = this.#t;
    if (this.#h === 0 || this.#v === 0) return;
    this.#P();
    const r = this.#f, o = this.#d, a = this.#A;
    if (r === null || o === null || a === null) return;
    const h = a[this.#r], c = a[1 - this.#r];
    A.bindFramebuffer(A.FRAMEBUFFER, r.framebuffer), A.useProgram(this.#c), this.#D(0, e, this.#e.a), this.#D(1, t, this.#e.b), this.#D(2, h.textures[0], this.#e.fieldMetrics), A.uniform1i(this.#e.first, i), A.uniform2i(this.#e.size, this.#h, this.#v), A.viewport(0, 0, r.width, r.height), A.drawArrays(A.TRIANGLES, 0, 3), A.bindFramebuffer(A.FRAMEBUFFER, o.framebuffer), A.useProgram(this.#n), this.#D(0, r.textures[0], this.#E.second), this.#D(1, r.textures[1], this.#E.first), A.uniform2i(this.#E.size, r.width, r.height), A.viewport(0, 0, o.width, o.height), A.drawArrays(A.TRIANGLES, 0, 3), A.bindFramebuffer(A.FRAMEBUFFER, c.framebuffer), A.useProgram(this.#s), this.#D(0, h.textures[0], this.#a.previous), this.#D(1, o.textures[0], this.#a.second), this.#D(2, o.textures[1], this.#a.first), A.uniform2i(this.#a.size, o.width, o.height), A.viewport(0, 0, p, 1), A.drawArrays(A.TRIANGLES, 0, 3), this.#r = 1 - this.#r, A.deleteSync(this.#i), A.bindBuffer(A.PIXEL_PACK_BUFFER, this.#o), A.readPixels(0, 0, p, 1, A.RGBA, A.FLOAT, 0), A.bindBuffer(A.PIXEL_PACK_BUFFER, null), A.bindFramebuffer(A.FRAMEBUFFER, null), this.#i = A.fenceSync(A.SYNC_GPU_COMMANDS_COMPLETE, 0), A.flush();
  }
  /**
   * Overwrite the phase of the newest frame, for one whose comparisons cannot
   * be trusted but whose place in the cycle is known some other way. The
   * readback is taken again so that it says the same.
   */
  seed(e, t) {
    const i = this.#t, A = this.#A?.[this.#r];
    A && (i.bindTexture(i.TEXTURE_2D, A.textures[0] ?? null), i.texSubImage2D(
      i.TEXTURE_2D,
      0,
      f.phase,
      0,
      1,
      1,
      i.RGBA,
      i.FLOAT,
      new Float32Array([e, t, 0, 0])
    ), i.deleteSync(this.#i), i.bindFramebuffer(i.FRAMEBUFFER, A.framebuffer), i.bindBuffer(i.PIXEL_PACK_BUFFER, this.#o), i.readPixels(0, 0, p, 1, i.RGBA, i.FLOAT, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), this.#i = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), i.flush());
  }
  /**
   * The phase of the last frame measured, once the GPU has handed it back,
   * and null while it is still on its way. It is handed back once.
   */
  poll() {
    const e = this.#t, t = this.#i;
    if (t === null || this.#o === null) return null;
    switch (e.clientWaitSync(t, 0, 0)) {
      case e.ALREADY_SIGNALED:
      case e.CONDITION_SATISFIED:
        return e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#o), e.getBufferSubData(e.PIXEL_PACK_BUFFER, 0, this.metrics), e.bindBuffer(e.PIXEL_PACK_BUFFER, null), e.deleteSync(t), this.#i = null, {
          phase: this.metrics[f.phase * 4] ?? 0,
          run: this.metrics[f.phase * 4 + 1] ?? 0
        };
      default:
        return null;
    }
  }
  destroy() {
    const e = this.#t;
    if (this.#x(), this.#A !== null) {
      for (const t of this.#A) B(e, t);
      this.#A = null;
    }
    e.deleteSync(this.#i), this.#i = null, e.deleteBuffer(this.#o), this.#o = null, e.deleteProgram(this.#c), e.deleteProgram(this.#n), e.deleteProgram(this.#s);
  }
  #D(e, t, i) {
    const A = this.#t;
    A.activeTexture(A.TEXTURE0 + e), A.bindTexture(A.TEXTURE_2D, t ?? null), A.uniform1i(i, e);
  }
  #x() {
    const e = this.#t;
    this.#f !== null && B(e, this.#f), this.#d !== null && B(e, this.#d), this.#f = null, this.#d = null;
  }
  /** Everything detect needs that is not there yet. */
  #P() {
    const e = this.#t;
    if (this.#f === null || this.#d === null) {
      this.#x();
      const t = Math.ceil(this.#h / Q), i = Math.ceil(this.#v / (V * 2));
      this.#f = _(e, t, i, 2), this.#d = _(
        e,
        Math.ceil(t / g),
        Math.ceil(i / g),
        2
      );
    }
    this.#A === null && (this.#A = [
      _(e, p, 1, 1),
      _(e, p, 1, 1)
    ], this.#r = 0, this.reset()), this.#o === null && (this.#o = e.createBuffer(), e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#o), e.bufferData(
      e.PIXEL_PACK_BUFFER,
      this.metrics.byteLength,
      e.STREAM_READ
    ), e.bindBuffer(e.PIXEL_PACK_BUFFER, null));
  }
}
function _(s, e, t, i) {
  const A = s.createFramebuffer();
  s.bindFramebuffer(s.FRAMEBUFFER, A);
  const r = [];
  for (let h = 0; h < i; h++) {
    const c = s.createTexture();
    s.bindTexture(s.TEXTURE_2D, c), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MIN_FILTER, s.NEAREST), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MAG_FILTER, s.NEAREST), s.texImage2D(
      s.TEXTURE_2D,
      0,
      s.RGBA32F,
      e,
      t,
      0,
      s.RGBA,
      s.FLOAT,
      null
    ), s.framebufferTexture2D(
      s.FRAMEBUFFER,
      s.COLOR_ATTACHMENT0 + h,
      s.TEXTURE_2D,
      c,
      0
    ), r.push(c);
  }
  s.drawBuffers(r.map((h, c) => s.COLOR_ATTACHMENT0 + c));
  const o = s.checkFramebufferStatus(s.FRAMEBUFFER) === s.FRAMEBUFFER_COMPLETE;
  s.bindFramebuffer(s.FRAMEBUFFER, null);
  const a = { framebuffer: A, textures: r, width: e, height: t };
  if (!o)
    throw B(s, a), new Error("failed to allocate framebuffer");
  return a;
}
function B(s, { framebuffer: e, textures: t }) {
  s.deleteFramebuffer(e);
  for (const i of t) s.deleteTexture(i);
}
const Y = [
  "mozParsedFrames",
  "mozDecodedFrames",
  "mozPresentedFrames",
  "mozPaintedFrames"
];
function j(s) {
  return Y.every((e) => e in s);
}
function pe() {
  return typeof HTMLVideoElement < "u" && (j(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const me = 250, Ee = 500;
class ve {
  #t;
  #c;
  #e = null;
  #n = null;
  #E = null;
  #s = null;
  #a = !0;
  #f = null;
  #d = null;
  #A = 0;
  constructor(e) {
    if (this.#t = e, this.#c = j(e) ? e : null, this.#c) {
      for (const t of ["emptied", "seeking", "seeked"])
        e.addEventListener(t, this.#o);
      for (const t of ["pause", "playing", "waiting", "ratechange"])
        e.addEventListener(t, this.#r);
    }
  }
  request(e) {
    this.#e === null && (this.#n = e, this.#e = this.#c ? requestAnimationFrame(this.#h) : this.#t.requestVideoFrameCallback(this.#i));
  }
  cancel() {
    this.#e !== null && (this.#c ? cancelAnimationFrame(this.#e) : this.#t.cancelVideoFrameCallback(this.#e)), this.#e = null, this.#n = null, this.#o();
  }
  destroy() {
    this.cancel();
    for (const e of ["emptied", "seeking", "seeked"])
      this.#t.removeEventListener(e, this.#o);
    for (const e of ["pause", "playing", "waiting", "ratechange"])
      this.#t.removeEventListener(e, this.#r);
  }
  #r = () => {
    this.#f = null, this.#d = null, this.#A = 0;
  };
  #o = () => {
    this.#E = null, this.#s = null, this.#a = !0, this.#r();
  };
  #i = (e, t) => {
    const i = this.#n;
    this.#e = null, this.#n = null, i?.(e, t);
  };
  #h = (e) => {
    const t = this.#c, i = Y.map((a) => t[a]);
    this.#E?.some((a, h) => i[h] < a) && this.#o(), this.#E = i;
    const A = t.mozPaintedFrames, r = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, o = this.#s === null && (A > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (r && (o || this.#s !== null && A !== this.#s)) {
      if (this.#f !== null && e - this.#f > Ee && (this.#r(), this.#a = !0), !t.paused && !t.ended) {
        const h = this.#d;
        if (h && e - h.at >= me) {
          const c = A - h.frames, n = (e - h.at) / c;
          c > 0 && n >= 4 && n <= 200 && (this.#A = this.#A ? this.#A + (n - this.#A) * 0.25 : n), this.#d = null;
        }
        this.#d ??= { at: e, frames: A };
      }
      this.#f = e, this.#s = A;
      const a = this.#a;
      this.#a = !1, this.#i(e, {
        width: t.videoWidth,
        height: t.videoHeight,
        // Used only to select source scan/size metadata on the media timeline.
        // It is deliberately NOT used as the frame identity or field clock.
        mediaTime: t.currentTime,
        presentedFrames: A,
        expectedDisplayTime: e,
        mozTiming: { periodMs: this.#A, discontinuity: a }
      });
    } else
      this.#e = requestAnimationFrame(this.#h);
  };
}
const xe = 0.5, m = 4, w = 5, X = 1e3, we = 4, I = 200, Te = 0.25, De = 0.75, Fe = 0.1, O = 1, be = 1e3 / 60, Re = 0.02, ge = 0.1, H = 1, M = 5, _e = 4, Me = 1001 / 3e4, y = 3, ye = 60, Se = 3, Be = {
  2: 0,
  3: 0.25,
  4: 0.5,
  5: 0.75
}, Pe = `#version 300 es
precision highp float;
uniform sampler2D uField;
uniform bool uFlip;
out vec4 fragColor;
void main() {
  ivec2 position = ivec2(gl_FragCoord.xy);
  if (uFlip) position.y = textureSize(uField, 0).y - 1 - position.y;
  fragColor = texelFetch(uField, position, 0);
}
`;
function Oe() {
  return pe() && typeof WebGL2RenderingContext < "u";
}
class He {
  canvas;
  #t;
  #c;
  #e;
  /** The pulldown detection, which keeps its measurements on the GPU. */
  #n;
  #E;
  #s;
  /** The program that copies a filtered picture onto the canvas. */
  #a;
  #f;
  #d;
  #A = [];
  /** Somewhere to filter a field into, and to read it back out of. */
  #r = [];
  /** Which output slot was written last; the next one follows round the ring. */
  #o = w - 1;
  /** Filtered fields waiting for their moment, oldest first. */
  #i = [];
  /** The last picture scheduled, shown or not; the next follows on from it. */
  #h = null;
  /** The rAF loop that puts them up, which is all that draws on the canvas. */
  #v = null;
  /** When the loop last ran, and the refresh grid fitted to it. See #measureRefresh. */
  #D = 0;
  #x = be;
  #P = 0;
  /** How far ahead of its moment the last picture was shown. See #present. */
  #_e = 0;
  /** What makes the pictures between pictures, and the last picture put up. */
  #C = null;
  #F = null;
  /** The pair the interpolator was last handed. */
  #se = null;
  /** Debug only: the rAF the last picture was drawn in, and repeats dropped since. */
  #fe = 0;
  #de = 0;
  /** The `<div>` this put around the element, so it can be taken away again. */
  #G = null;
  #pe;
  #X;
  #Me;
  #L;
  #y;
  /** How long a frame lasts in wall time, from what the frames themselves say. */
  #l = 0;
  /** The size of a frame as it is coded, which is what a texture holds. */
  #w = 0;
  #R = 0;
  /** Where the newest frame is. The two before it follow round the ring. */
  #T = m - 1;
  /** How many of the held frames are consecutive, up to HISTORY. */
  #p = 0;
  #O = 0;
  #g = !1;
  #me = !1;
  #u = null;
  #q = [];
  #H = !1;
  #Ee;
  /** Everything the next report is counted from. See DeinterlaceStats. */
  #b = {
    filtered: 0,
    missed: 0,
    degraded: 0,
    discontinuities: 0,
    late: 0,
    resynced: 0,
    queueResetted: 0
  };
  /** `presentedFrames` of the last frame the callback saw; 0 before any. */
  #re = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #ve = 0;
  #ne = 0;
  #I = 0;
  #ee = 0;
  #S = 0;
  #z = 0;
  #W = 0;
  #Z;
  #oe = [];
  #$ = [];
  #te = 0;
  #Q = 0;
  #ie = 0;
  #V = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #Y = D;
  #U = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #m = D;
  #k = !1;
  /**
   * Where the cycle was when a frame was last seen locked to it. A broadcast
   * can cut between soft pulldown, which is progressive pictures with repeat
   * flags, and the same film laid down as interlaced frames, every few seconds
   * and without losing its place in the cycle, so this outlives those cuts.
   */
  #_ = null;
  /** How many of the coming frames take their phase from `#cadence`. */
  #M = 0;
  /**
   * Whether the frames being seeded follow a missed frame rather than
   * progressive pictures, so that the first of them is not one of those.
   */
  #he = !1;
  #j = null;
  #ye = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #ae = [0, 0, 0, 0, 0, 0];
  #xe = 0;
  constructor(e, t = {}) {
    this.#t = e, this.#X = t.doubleRate ?? !1, this.#Me = t.spatialCheck ?? !0, this.#L = t.debug ?? !1, this.#C = t.interpolator ?? null, this.#y = t.film ?? !1, this.#Ee = t.onStats, this.canvas = document.createElement("canvas"), this.canvas.style.cssText = "position:absolute;pointer-events:none;visibility:hidden";
    const i = this.canvas.getContext("webgl2", {
      alpha: !1,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!i) throw new Error("this browser has no WebGL2");
    this.#e = i, this.#n = new de(i), this.#E = b(i, fe, R);
    const A = this.#E;
    this.#s = Object.fromEntries(
      Object.entries(ue).map(([r, o]) => [
        r,
        i.getUniformLocation(A, o)
      ])
    ), this.#a = b(i, Pe, R), this.#f = i.getUniformLocation(this.#a, "uField"), this.#d = i.getUniformLocation(this.#a, "uFlip"), this.#y && this.#Se(), this.#Z = i.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.canvas.addEventListener("webglcontextlost", this.#$e), this.#pe = new ResizeObserver(() => this.#ue()), this.#c = new ve(e), e.addEventListener("emptied", this.#We), e.addEventListener("resize", this.#ze), e.addEventListener("pause", this.#B), e.addEventListener("ended", this.#B), e.addEventListener("seeked", this.#B), e.addEventListener("ratechange", this.#B);
  }
  get running() {
    return this.#g && (this.#u?.interlaced ?? !0);
  }
  /** Whether the caller wants filtering, independently of the current source. */
  get enabled() {
    return this.#me;
  }
  set enabled(e) {
    this.#me = e, this.#Te();
  }
  /** Update whether the source needs filtering and which field comes first. */
  set scan(e) {
    this.#u = e, this.#Te();
  }
  get scan() {
    return this.#u;
  }
  set videoTimeline(e) {
    this.#q = e, e.length === 0 && (this.#u = null), this.#Te();
  }
  get videoTimeline() {
    return this.#q;
  }
  /**
   * What to put on the screen for fullscreen: the `<div>` holding both the
   * element and the canvas once there is one, and the element itself before
   * that. Fullscreening the element alone would leave the canvas behind in
   * the page, and with it the only deinterlaced picture there is.
   */
  get container() {
    return this.#G ?? this.#t;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#X;
  }
  set doubleRate(e) {
    e !== this.#X && (this.#X = e, this.#we());
  }
  get film() {
    return this.#y;
  }
  set film(e) {
    e !== this.#y && (e && this.#Se(), this.#y = e, e || (this.#k = !1, this.#m = D, this.#J(), this.#N()), this.#we());
  }
  get debug() {
    return this.#L;
  }
  set debug(e) {
    this.#L = e;
  }
  get interpolator() {
    return this.#C;
  }
  set interpolator(e) {
    e !== this.#C && (this.#C = e, this.#se = null, this.#we());
  }
  /** Whether pictures are queued and put up by the loop rather than drawn on arrival. */
  get #K() {
    return this.#X || this.#y || (this.#C?.wants("frame") ?? !1);
  }
  #we() {
    this.#K ? (this.#w > 0 && this.#He(), this.#Fe()) : (this.#be(), this.#Ae());
  }
  #Se() {
    if (this.#e.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  #Te() {
    this.#me && (this.#q.length > 0 || (this.#u?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  start() {
    this.#g || this.#H || (this.#g = !0, this.#Ze(), this.#ot(), this.#ce(), (this.#u?.interlaced ?? !0) && this.#Fe());
  }
  /** Take the deinterlaced picture away, leaving the element's own showing. */
  stop() {
    this.#g && (this.#g = !1, this.#c.cancel(), this.#be(), this.#p = 0, this.canvas.style.visibility = "hidden");
  }
  destroy() {
    this.stop(), this.#c.destroy(), this.canvas.removeEventListener("webglcontextlost", this.#$e), this.#t.removeEventListener("emptied", this.#We), this.#t.removeEventListener("resize", this.#ze), this.#t.removeEventListener("pause", this.#B), this.#t.removeEventListener("ended", this.#B), this.#t.removeEventListener("seeked", this.#B), this.#t.removeEventListener("ratechange", this.#B), this.#ht();
    for (const e of this.#A) this.#e.deleteTexture(e);
    this.#A = [], this.#Ae();
    for (const e of [
      ...this.#oe,
      ...this.#$.map(({ q: t }) => t)
    ])
      this.#e.deleteQuery(e);
    this.#oe.length = 0, this.#$.length = 0, this.#n.destroy(), this.#j !== null && (te(this.#j), this.#j = null), this.#e.deleteProgram(this.#E), this.#e.deleteProgram(this.#a), this.#e.getExtension("WEBGL_lose_context")?.loseContext();
  }
  #ce() {
    this.#g && this.#c.request(this.#Ke);
  }
  #Qe(e, t, i) {
    this.#j == null && (this.#j = q(this.#e, "20px monospace")), ie(this.#j, e, t, i, this.#w, this.#R, 20);
  }
  #Be(e) {
    if (this.#Z == null || this.#$.length > 30)
      return;
    const t = this.#oe.pop() ?? this.#e.createQuery();
    return this.#e.beginQuery(this.#Z.TIME_ELAPSED_EXT, t), this.#$.push({ q: t, isField: e }), t;
  }
  #Pe(e) {
    this.#Z != null && (e != null && this.#e.endQuery(this.#Z.TIME_ELAPSED_EXT), this.#$ = this.#$.filter(
      ({ q: t, isField: i }) => {
        if (this.#e.getQueryParameter(t, this.#e.QUERY_RESULT_AVAILABLE)) {
          const A = this.#e.getQueryParameter(t, this.#e.QUERY_RESULT);
          return i ? (this.#ie += A, this.#V++) : (this.#te += A, this.#Q++), this.#oe.push(t), !1;
        }
        return !0;
      }
    ));
  }
  #J() {
    this.#_ = null, this.#M = 0, this.#he = !1;
  }
  /**
   * Give the frame being filtered its phase from the cycle seen before the
   * progressive pictures, if it is one of those that needs it.
   */
  #Ce(e) {
    if (this.#M === 0 || this.#_ === null) return;
    const t = e - this.#_.mediaTime;
    if (t < 0 || t > ye) {
      this.#J();
      return;
    }
    const i = this.#M === y && !this.#he ? Se : (this.#_.phase - 1 + Math.round(t / Me)) % M + 1;
    this.#Y = this.#m = { phase: i, run: S }, this.#U = 0, this.#n.seed(i, S), this.#M--, this.#M === 0 && (this.#he = !1);
  }
  #N() {
    this.#m = D, this.#Y = D, this.#U = 0, this.#k = !1, this.#n.reset();
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #Ve() {
    const { cur: e, next: t } = this.#Xe(!1), i = this.#A[e], A = this.#A[t];
    if (!i || !A) return;
    const r = this.#u?.topFieldFirst !== !1 ? 0 : 1;
    this.#n.detect(i, A, r);
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #Ye() {
    const e = this.#n.poll();
    e !== null && (this.#Y = e, this.#U = 0), this.#U++;
    const { phase: t, run: i } = this.#Y;
    t === 0 || this.#U > M ? this.#m = D : this.#m = {
      phase: (t - 1 + this.#U) % M + 1,
      run: i
    };
  }
  #je(e) {
    const t = [], i = this.#n.metrics;
    for (let r = 0; r < f.phase; r++) {
      const o = i[r * 4] ?? 0, a = i[r * 4 + 1] ?? 0, h = i[r * 4 + 2] ?? 0;
      t.push(
        `${o.toFixed(4)},${a.toString().padStart(4)},${h.toFixed(3)}`
      );
    }
    const A = this.#ae.map((r, o) => `${o === 0 ? "-" : o}:${r}`).join(" ");
    return `frame=${e} phase=${this.#m.phase} run=${this.#m.run} known=${this.#Y.phase}/${this.#Y.run} age=${this.#U} ${this.#k ? "film" : "video"} period=${this.#l.toFixed(3)}
${t.join(" ")}
${A} dropped:${this.#xe}`;
  }
  /** Whether this frame is not the one that follows the last one seen. */
  #Le(e) {
    if (e.mozTiming) return e.mozTiming.discontinuity;
    const t = e.mediaTime - this.#O;
    return t < 0 || t > xe;
  }
  #Ke = (e, t) => {
    if (!(!this.#g || this.#H)) {
      if (this.#Je(t.mediaTime), t.width > 0 && t.height > 0) {
        if ((this.#w === 0 || this.#R === 0) && this.#Oe(t.width, t.height), this.#u && !this.#u.interlaced) {
          const c = this.#Le(t);
          c && this.#J(), this.#Ne(t.presentedFrames, c), this.#O = t.mediaTime, this.#rt(), this.#ce();
          return;
        }
        const i = t.mediaTime - this.#O, A = this.#Le(t);
        A && (this.#J(), this.#p = 0, this.#b.discontinuities++, this.#i.length = 0, this.#h = null, this.#F = null, this.#N());
        const r = this.#Ne(t.presentedFrames, A);
        if (!t.mozTiming && this.#p > 0 && t.mediaTime === this.#O) {
          this.#ce();
          return;
        }
        t.mozTiming ? this.#l = t.mozTiming.periodMs : !A && i > 0 && this.#qe(i), this.#O = t.mediaTime;
        const o = performance.now();
        o - this.#ve > X && (this.#ne = o, this.#I = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0), this.#ve = o;
        const a = performance.now(), h = this.#Be(!1);
        if (this.#Ge(), this.#W = Math.max(
          this.#W,
          this.#i.length
        ), this.#y && (this.#p === m && r === 0 ? (this.#Ye(), this.#Ve(), this.#Ce(t.mediaTime)) : r > 0 && this.#_ !== null ? (this.#N(), this.#M !== y && (this.#M = y, this.#he = !0), this.#Ce(t.mediaTime)) : this.#N(), this.#ae[this.#m.phase] = (this.#ae[this.#m.phase] ?? 0) + 1, this.#k = this.#m.phase !== 0 && this.#m.run >= S, this.#k && this.#M === 0 && (this.#_ = {
          mediaTime: t.mediaTime,
          phase: this.#m.phase
        }), this.#L && (this.#ye = this.#je(t.presentedFrames))), this.#Ie() && !this.#t.paused && !this.#t.ended) {
          this.#i.length >= w && (this.#i.length = 0, this.#h = null, this.#F = null, this.#b.queueResetted++);
          const c = (t.expectedDisplayTime || e) + this.#x;
          if (this.#k) {
            const n = this.#m.phase;
            if (n === k)
              this.#xe++, this.#de++;
            else {
              const u = this.#l * M / _e, d = Be[n] ?? 0, l = this.#De(
                "film",
                c + d * this.#l,
                u
              );
              this.#le("film", n, !1, l, u);
            }
          } else if (this.#X) {
            const n = this.#l / 2, u = this.#De("field", c, n);
            this.#le("field", 1, !1, u, n), this.#le("field", 2, !0, u + n, n);
          } else {
            const n = this.#l, u = this.#De("frame", c, n);
            this.#le("frame", 0, !1, u, n);
          }
        } else
          this.#ge(this.#t.paused || this.#t.ended, !1, null);
        this.#Pe(h), this.#ee += performance.now() - a, this.#I++, this.#nt(o);
      }
      this.#ce();
    }
  };
  #Je(e) {
    let t;
    for (let r = this.#q.length - 1; r >= 0; r--) {
      const o = this.#q[r];
      if (o.start <= e + 1e-6) {
        t = o;
        break;
      }
    }
    t?.codedSize && (t.codedSize.width !== this.#w || t.codedSize.height !== this.#R) && this.#Oe(t.codedSize.width, t.codedSize.height);
    const i = t?.scan;
    if (!i || this.#u?.interlaced === i.interlaced && this.#u.topFieldFirst === i.topFieldFirst)
      return;
    const A = this.#u?.interlaced === !1;
    this.#u = i, this.#_ === null && (this.#p = 0), this.#i.length = 0, this.#h = null, this.#F = null, this.#N(), i.interlaced && A && this.#_ !== null && (this.#M = y), i.interlaced ? this.#K && this.#Fe() : this.#be();
  }
  /**
   * Whether pictures are being filtered ahead of time and queued, rather than
   * drawn as their frame arrives.
   *
   * A picture for every frame has nothing to schedule -- there is one of them
   * and it goes up now -- and neither has a filter that has yet to see two
   * frames go by, since until then there is no idea how long a frame lasts.
   */
  #Ie() {
    return this.#K && this.#l > 0 && this.#r.length === w;
  }
  /**
   * When a picture goes up: one duration after the last one of its cadence,
   * nudged towards `ideal` by a fraction of the gap so that the schedule
   * follows the clock without a picture ever moving across a refresh. It
   * restarts from `ideal` when the gap has grown to a whole picture or the
   * cadence has changed.
   */
  #De(e, t, i) {
    const A = this.#h;
    if (A !== null && A.cadence === e) {
      const r = A.at + A.duration, o = t - r;
      if (Math.abs(o) < i) {
        const a = Math.max(
          -O,
          Math.min(O, o * Fe)
        );
        return r + a;
      }
    }
    A !== null && this.#b.resynced++;
    for (let r = this.#i.at(-1); r && r.at >= t; )
      this.#i.pop(), this.#b.late++, r = this.#i.at(-1);
    return t;
  }
  /**
   * How long a frame lasts in wall time, kept as a smoothed estimate.
   *
   * Taken from the frames themselves rather than from a frame rate nobody
   * reports, and in wall time, so a rate other than 1 moves the fields with
   * it. A frame the callback never saw makes the step between two of them a
   * whole multiple of the period, and dividing that back out matters: taken
   * at face value, one missed frame would put every field of the next one
   * half a frame late and hold the picture through a refresh it should have
   * moved in.
   */
  #qe(e) {
    const t = e * 1e3 / (this.#t.playbackRate || 1), i = this.#l > 0 ? Math.max(1, Math.round(t / this.#l)) : 1, A = t / i;
    A < we || A > I || (this.#l = this.#l > 0 && A > this.#l * De ? this.#l + (A - this.#l) * Te : A);
  }
  /**
   * Filter one field into an output texture and put it in the queue.
   *
   * The three frames the filter reads are only the right three between one
   * frame arriving and the next, so both fields of a frame are built here and
   * held as pictures. What is queued after that is a copy waiting for a
   * moment, which no later frame can take away.
   */
  #le(e, t, i, A, r) {
    const o = (this.#o + 1) % w, a = this.#r[o];
    if (!a) return;
    for (this.#o = o, this.#F?.slot === o && (this.#F = null); this.#i.length > 0 && this.#i[0]?.slot === o; )
      this.#i.shift(), this.#b.late++;
    this.#ge(!1, i, a.framebuffer);
    const h = {
      slot: o,
      at: A,
      duration: r,
      cadence: e,
      phase: t,
      droppedBefore: this.#de
    };
    this.#de = 0, this.#i.push(h), this.#h = h;
  }
  /** The loop that puts filtered fields up, and the only thing that draws. */
  #Fe() {
    this.#v === null && (!this.#g || this.#H || !this.#K || (this.#v = requestAnimationFrame(this.#Ue)));
  }
  #be() {
    this.#v !== null && cancelAnimationFrame(this.#v), this.#v = null, this.#i.length = 0, this.#h = null, this.#F = null;
  }
  #Ue = (e) => {
    this.#v = null, !(!this.#g || this.#H || !this.#K) && (this.#et(e), this.#tt(this.#P, e), this.#v = requestAnimationFrame(this.#Ue));
  };
  /**
   * Fit a grid of refreshes (period and phase) to the animation frames. rAF
   * timestamps wander by a millisecond or so, which is more than the
   * nearest-refresh decision in #present can take; the grid is what it
   * compares against. A frame far off the grid restarts it.
   */
  #et(e) {
    const t = e - this.#D;
    this.#D = e;
    const i = Math.max(1, Math.round(t / this.#x)), A = this.#P + i * this.#x, r = e - A;
    if (this.#P === 0 || t <= 0 || t > I || Math.abs(r) > this.#x / 4) {
      t > 0 && t <= I && (this.#x = t), this.#P = e;
      return;
    }
    this.#x += r / i * Re, this.#P = A + r * ge;
  }
  /**
   * Put up whichever filtered field belongs on the screen next.
   *
   * What is drawn during an animation frame reaches the screen at the composite
   * after it, so that is the moment being filled, and a field goes up at
   * whichever composite falls nearest the moment it stands for -- half a
   * refresh either side of it. Where two of them have come due since the last
   * one, only the newer is shown: a screen has one picture per refresh, and
   * the older of the two is a moment the viewer should already be past.
   *
   * Near the half-refresh boundary a picture goes the way the last one went,
   * so that the slip a cadence the refresh does not divide into must make
   * every so often happens once rather than flapping.
   */
  #tt(e, t) {
    const i = this.#x / 2, A = (h) => {
      const c = h.at + this.#Re(h) - e;
      return c <= i - H ? !0 : c > i + H ? !1 : this.#_e > 0;
    };
    for (; this.#i[1] && A(this.#i[1]); )
      this.#b.late++, this.#i.shift();
    const r = this.#i[0];
    if (!r || !A(r)) {
      r && this.#it(r, e);
      return;
    }
    this.#i.shift(), this.#_e = r.at + this.#Re(r) - e, this.#F = r;
    const o = performance.now(), a = this.#Be(!0);
    this.#st(r.slot), this.#Pe(a), this.#z += performance.now() - o, this.#S++, this.#L && this.#At(r, t), this.#fe = t;
  }
  /** How late a picture goes up: one picture, when the pictures after it are made. */
  #Re(e) {
    return this.#C?.wants(e.cadence) ? e.duration : 0;
  }
  /**
   * Put up a picture between the last one and the next, at the moment this
   * refresh shows. Nothing is drawn where the two are not neighbours: a new
   * cadence, a gap, or a moment outside them.
   */
  #it(e, t) {
    const i = this.#C, A = this.#F;
    if (!i || !A || A.cadence !== e.cadence || !i.wants(e.cadence))
      return;
    const r = e.at - A.at;
    if (r <= 0 || r > A.duration * 1.5) return;
    const o = (t - this.#Re(A) - A.at) / r;
    if (o <= 0 || o >= 1) return;
    const a = this.#r[A.slot], h = this.#r[e.slot];
    if (!a || !h) return;
    const c = this.#e;
    (this.#se?.previous !== A || this.#se.next !== e) && (i.pair(c, a.texture, h.texture, this.#w, this.#R), this.#se = { previous: A, next: e });
    const n = performance.now();
    c.bindFramebuffer(c.FRAMEBUFFER, null), i.draw(c, o, this.#w, this.#R) && (this.#z += performance.now() - n, this.#S++, this.canvas.style.visibility = "visible");
  }
  #At(e, t) {
    const i = this.#fe === 0 ? 0 : t - this.#fe, A = i / this.#x, r = e.cadence === "film" ? `phase ${e.phase}` : e.cadence === "field" ? `field ${e.phase}` : "frame", o = e.droppedBefore > 0 ? `, phase ${k} dropped before it` + (e.droppedBefore > 1 ? ` (${e.droppedBefore})` : "") : "";
    console.log(
      `yadif: +${i.toFixed(2)} ms (${A.toFixed(2)} refreshes) ${e.cadence} ${r}, due ${(e.at - t).toFixed(2)} ms${o}`
    );
  }
  /** Copy one of the filtered pictures onto the canvas. */
  #st(e) {
    const t = this.#r[e];
    t && this.#ke(t.texture);
  }
  /** Put a progressive frame through unchanged, keeping one display surface. */
  #rt() {
    this.#Ge();
    const e = this.#A[this.#T];
    e && this.#ke(e, !0), this.#_ === null && (this.#p = 0);
  }
  #ke(e, t = !1) {
    const i = this.#e;
    i.bindFramebuffer(i.FRAMEBUFFER, null), i.useProgram(this.#a), i.activeTexture(i.TEXTURE0), i.bindTexture(i.TEXTURE_2D, e), i.uniform1i(this.#f, 0), i.uniform1i(this.#d, t ? 1 : 0), i.viewport(0, 0, this.#w, this.#R), i.drawArrays(i.TRIANGLES, 0, 3), this.canvas.style.visibility = "visible";
  }
  /**
   * Account for the frames between this one and the last one seen.
   *
   * There is no event for a frame the callback was not run for; the only sign
   * of one is that the count of frames the compositor has taken went up by
   * more than one. Frames thrown away either side of a discontinuity are not
   * counted: the held frames were being dropped anyway, and a seek presents
   * what it passes over.
   *
   * Returns how many frames were missed between the last one and this one.
   */
  #Ne(e, t) {
    let i = 0;
    return this.#re !== 0 && !t && (i = Math.max(0, e - this.#re - 1), this.#b.missed += i), this.#re = e, i;
  }
  #nt(e) {
    if (!this.#Ee) return;
    const t = e - this.#ne;
    if (t < X) return;
    const i = this.#Ie() ? this.#S : this.#I;
    let A = 0;
    this.#I !== 0 && (A += this.#ee / this.#I), this.#S !== 0 && (A += this.#z / this.#S / 2);
    let r;
    this.#Z != null && (r = 0, this.#Q !== 0 && (r += this.#te / 1e6 / this.#Q), this.#V !== 0 && (r += this.#ie / 1e6 / this.#V / 2)), this.#Ee({
      ...this.#b,
      // The element's own count of what its decoder could not keep up with,
      // which is the machine being behind rather than this filter.
      dropped: this.#t.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
      fps: i * 1e3 / t,
      frameMs: A,
      maxQueuedFields: this.#W,
      gpuMs: r,
      film: this.#k
    }), this.#ne = e, this.#I = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0;
  }
  /** Take the newest frame into the ring. */
  #Ge() {
    const e = this.#e;
    this.#T = (this.#T + 1) % m, e.bindTexture(e.TEXTURE_2D, this.#A[this.#T] ?? null), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      e.RGBA,
      e.UNSIGNED_BYTE,
      this.#t
    ), this.#p = Math.min(this.#p + 1, m);
  }
  /**
   * Filter one frame, onto the canvas or into an output texture.
   *
   * A null `target` is the canvas itself, which is where the picture goes when
   * there is one per frame and nothing to schedule. An output framebuffer is a
   * field being kept for its moment.
   *
   * `second` asks for the frame's other field: the same three frames filtered
   * the other way round, keeping the field that came second and rebuilding
   * the first. The shader takes the pair of frames the missing line sits
   * between from the parity, so this is the whole of it.
   *
   * With `film` on, the shader is also given the field metrics, and puts a
   * film frame back together rather than filtering it.
   */
  #ge(e, t, i) {
    if (this.#p === 0 || this.#H) return;
    this.#p === m && !e ? this.#b.filtered++ : this.#b.degraded++;
    const A = this.#e, { prev: r, cur: o, next: a } = this.#Xe(e);
    A.bindFramebuffer(A.FRAMEBUFFER, i), A.useProgram(this.#E);
    for (const [u, d] of [r, o, a].entries())
      A.activeTexture(A.TEXTURE0 + u), A.bindTexture(A.TEXTURE_2D, this.#A[d] ?? null);
    A.uniform1i(this.#s.prev, 0), A.uniform1i(this.#s.cur, 1), A.uniform1i(this.#s.next, 2);
    const h = this.#y ? this.#n.texture : null, c = h !== null;
    h !== null && (A.activeTexture(A.TEXTURE0 + 3), A.bindTexture(A.TEXTURE_2D, h), A.uniform1i(this.#s.fieldMetrics, 3)), A.uniform2i(this.#s.size, this.#w, this.#R);
    const n = this.#u?.topFieldFirst !== !1 ? 0 : 1;
    A.uniform1i(this.#s.parity, t ? 1 - n : n), A.uniform1i(this.#s.second, t ? 1 : 0), A.uniform1i(
      this.#s.tff,
      this.#u?.topFieldFirst !== !1 ? 1 : 0
    ), A.uniform1i(this.#s.spatialCheck, this.#Me ? 1 : 0), A.uniform1i(this.#s.debug, this.#L ? 1 : 0), A.uniform1i(this.#s.film, c ? 1 : 0), A.uniform1i(this.#s.phase, this.#m.phase), A.viewport(0, 0, this.#w, this.#R), A.drawArrays(A.TRIANGLES, 0, 3), this.#L && c && this.#Qe(this.#ye, 0, 90), i === null && (this.canvas.style.visibility = "visible");
  }
  #Xe(e) {
    const t = (i) => (this.#T + m - i) % m;
    return this.#p === 1 ? { prev: this.#T, cur: this.#T, next: this.#T } : e ? { prev: t(1), cur: this.#T, next: this.#T } : this.#p === 2 ? { prev: t(1), cur: t(1), next: this.#T } : { prev: t(2), cur: t(1), next: this.#T };
  }
  /**
   * Put the canvas exactly where the element's picture is.
   *
   * The buffer holds coded pixels and is stretched across a box of the shape
   * the picture is meant to be seen in, which is what applies the sample
   * aspect ratio -- the same stretch the element does with its own picture.
   * The box itself is the picture's, not the element's: a media element fits
   * its picture inside its box and this has to land on top of that, so the fit
   * is worked out again here. It assumes the element's `object-fit` is the
   * `contain` it is by default.
   */
  #ue() {
    if (!this.#G) return;
    const e = this.#t, t = e.videoWidth, i = e.videoHeight;
    if (t === 0 || i === 0) return;
    const A = Math.min(
      e.offsetWidth / t,
      e.offsetHeight / i
    ), r = t * A, o = i * A;
    this.canvas.style.left = `${e.offsetLeft + (e.offsetWidth - r) / 2}px`, this.canvas.style.top = `${e.offsetTop + (e.offsetHeight - o) / 2}px`, this.canvas.style.width = `${r}px`, this.canvas.style.height = `${o}px`;
  }
  #Oe(e, t) {
    const i = this.#e;
    this.canvas.width = e, this.canvas.height = t, this.#w = e, this.#R = t, this.#p = 0, this.#ue();
    for (const A of this.#A) i.deleteTexture(A);
    this.#A = [];
    for (let A = 0; A < m; A++) {
      const r = i.createTexture();
      i.bindTexture(i.TEXTURE_2D, r), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MIN_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MAG_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_S, i.CLAMP_TO_EDGE), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_T, i.CLAMP_TO_EDGE), i.texImage2D(
        i.TEXTURE_2D,
        0,
        i.RGBA,
        e,
        t,
        0,
        i.RGBA,
        i.UNSIGNED_BYTE,
        null
      ), this.#A.push(r);
    }
    this.#Ae(), this.#n.resize(e, t), this.#K && this.#He();
  }
  /**
   * Somewhere to keep a filtered field until its moment comes.
   *
   * A frame's worth of texture each, so they exist only while a picture is
   * being shown for every field. Where a framebuffer will not take one -- an
   * implementation that will not render to RGBA8, or memory it will not find
   * -- the whole lot goes and the fields are drawn as their frames arrive,
   * which is the timing this replaces but is still a picture.
   */
  #He() {
    const e = this.#e;
    if (!(this.#r.length === w || this.#w === 0)) {
      this.#Ae();
      for (let t = 0; t < w; t++) {
        const i = e.createTexture();
        e.bindTexture(e.TEXTURE_2D, i), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
          e.TEXTURE_2D,
          0,
          e.RGBA,
          this.#w,
          this.#R,
          0,
          e.RGBA,
          e.UNSIGNED_BYTE,
          null
        );
        const A = e.createFramebuffer();
        e.bindFramebuffer(e.FRAMEBUFFER, A), e.framebufferTexture2D(
          e.FRAMEBUFFER,
          e.COLOR_ATTACHMENT0,
          e.TEXTURE_2D,
          i,
          0
        );
        const r = e.checkFramebufferStatus(e.FRAMEBUFFER) === e.FRAMEBUFFER_COMPLETE;
        if (e.bindFramebuffer(e.FRAMEBUFFER, null), !r) {
          e.deleteFramebuffer(A), e.deleteTexture(i), this.#Ae();
          return;
        }
        this.#r.push({ texture: i, framebuffer: A });
      }
      this.#o = w - 1;
    }
  }
  #Ae() {
    const e = this.#e;
    for (const { texture: t, framebuffer: i } of this.#r)
      e.deleteFramebuffer(i), e.deleteTexture(t);
    this.#r = [], this.#i.length = 0, this.#h = null, this.#F = null;
  }
  /**
   * Wrap the element in a `<div>` of this one's own and put the canvas over
   * it. The wrapper is what the canvas is positioned against; moving the
   * element out of the tree and back within the one task leaves playback
   * alone, which is what makes turning this on mid-stream free.
   */
  #ot() {
    if (this.#G) return;
    const e = this.#t.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#t), t.appendChild(this.#t), t.appendChild(this.canvas), this.#G = t, this.#pe.observe(this.#t), this.#ue();
  }
  #ht() {
    const e = this.#G;
    this.#G = null, this.#pe.disconnect(), this.canvas.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#t, e), e.remove());
  }
  #ze = () => this.#ue();
  #We = () => {
    this.#p = 0, this.#O = 0, this.#i.length = 0, this.#h = null, this.#F = null, this.#J(), this.#N(), this.#l = 0, this.#Ze(), this.canvas.style.visibility = "hidden";
  };
  #Ze() {
    this.#b = {
      filtered: 0,
      missed: 0,
      degraded: 0,
      discontinuities: 0,
      late: 0,
      resynced: 0,
      queueResetted: 0
    }, this.#ae.fill(0), this.#xe = 0, this.#re = 0, this.#ne = 0, this.#ve = 0, this.#I = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0;
  }
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #B = () => {
    this.#J(), this.#i.length = 0, this.#h = null, this.#F = null, this.#N(), this.#g && this.#ge(!0, !1, null);
  };
  /**
   * A lost context takes the textures and the program with it. Rebuilding
   * them is possible, but a page that has lost its context has bigger
   * problems; getting out of the way leaves the element's own picture showing.
   */
  #$e = (e) => {
    e.preventDefault(), this.#H = !0, this.stop();
  };
}
const z = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", Ce = 0.5, Le = 3e3, W = 0.1, F = 16, Z = 'video/mp4; codecs="avc1.640029"';
let N = null;
function Ie(s = {}) {
  return N ??= Ue(s), N;
}
async function ze(s = {}) {
  return (await Ie(s)).deinterlaces;
}
function We() {
  N = null;
}
async function Ue(s) {
  const e = s.tolerance ?? Ce, t = s.timeoutMs ?? Le, i = performance.now(), A = (a) => ({
    deinterlaces: !1,
    survives: null,
    tookMs: performance.now() - i,
    error: a instanceof Error ? a.message : String(a)
  });
  if (typeof document > "u")
    return A(new Error("there is no document to decode in"));
  const r = document.createElement("video");
  r.muted = !0, r.defaultMuted = !0, r.playsInline = !0, r.preload = "auto";
  let o = null;
  try {
    o = Ne(r, t);
    const a = C(P(r, "loadeddata"), t), h = r.play().then(
      () => !0,
      () => !1
    );
    if (await o.ready, await a, await Ge(r, t, await h), r.videoWidth === 0 || r.videoHeight === 0)
      return A(new Error("the probe clip decoded to nothing"));
    const c = Xe(r);
    return {
      deinterlaces: c < 1 - e,
      survives: c,
      tookMs: performance.now() - i
    };
  } catch (a) {
    return A(a);
  } finally {
    r.pause(), r.removeAttribute("src"), r.replaceChildren(), r.load(), o && URL.revokeObjectURL(o.url);
  }
}
const U = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, ke = typeof MediaSource > "u";
function Ne(s, e) {
  if (!U || !U.isTypeSupported(Z))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = z.indexOf(","), i = atob(z.slice(t + 1)), A = new Uint8Array(i.length);
  for (let h = 0; h < i.length; h++) A[h] = i.charCodeAt(h);
  const r = new U(), o = URL.createObjectURL(r);
  if (ke) {
    s.disableRemotePlayback = !0;
    const h = document.createElement("source");
    h.type = "video/mp4", h.src = o, s.append(h), s.load();
  } else
    s.src = o;
  const a = (async () => {
    await C(P(r, "sourceopen"), e);
    const h = r.addSourceBuffer(Z), c = C(P(h, "updateend"), e);
    h.appendBuffer(A), await c, r.endOfStream();
  })();
  return { url: o, ready: a };
}
async function Ge(s, e, t) {
  if (t) {
    const i = performance.now();
    for (; s.currentTime < W && performance.now() - i < e; )
      await new Promise((A) => requestAnimationFrame(A));
    s.pause();
  } else
    s.currentTime = W, await C(P(s, "seeked"), e);
}
function Xe(s) {
  const e = s.videoHeight, t = document.createElement("canvas");
  t.width = F, t.height = e;
  const i = t.getContext("2d", { willReadFrequently: !0 });
  if (!i) throw new Error("there is no 2d context to read the clip with");
  i.imageSmoothingEnabled = !1, i.drawImage(s, 0, 0, F, e);
  const A = i.getImageData(0, 0, F, e).data, r = (n) => {
    let u = 0;
    for (let d = 0; d < F; d++)
      u += A[(n * F + d) * 4 + 1] ?? 0;
    return u / F;
  };
  let o = 0;
  const a = 2, h = e - 3;
  let c = r(a);
  for (let n = a + 1; n <= h; n++) {
    const u = r(n);
    o += Math.abs(u - c), c = u;
  }
  return o / (h - a) / 255;
}
function P(s, e) {
  return new Promise((t, i) => {
    s.addEventListener(e, () => t(), { once: !0 }), s.addEventListener(
      "error",
      () => {
        const A = s instanceof HTMLMediaElement ? s.error : null, r = A ? ` (MediaError ${A.code}${A.message ? `: ${A.message}` : ""})` : "";
        i(new Error(`the probe clip ${e} failed${r}`));
      },
      { once: !0 }
    );
  });
}
function C(s, e) {
  return Promise.race([
    s,
    new Promise(
      (t, i) => setTimeout(
        () => i(new Error("the probe clip took too long")),
        e
      )
    )
  ]);
}
export {
  He as Deinterlacer,
  fe as YADIF_FRAGMENT_SHADER,
  ue as YADIF_UNIFORMS,
  ze as decoderDeinterlaces,
  We as forgetDecoderProbe,
  Ie as probeDecoder,
  Oe as supportsDeinterlace
};
