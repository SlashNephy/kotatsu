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
const J = `#version 300 es
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
`, q = `#version 300 es
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
function ee(s, e) {
  const t = b(s, J, q), i = s.getAttribLocation(t, "aVertexPosition"), A = s.getAttribLocation(t, "aTextureCoord"), r = s.getUniformLocation(t, "uTexture"), o = s.getUniformLocation(t, "uMatrix"), h = s.getUniformLocation(t, "uUvMatrix"), a = s.getUniformLocation(t, "uTextColor"), c = s.getUniformLocation(t, "uBackColor");
  if (r == null || o == null || h == null || a == null || c == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const n = s.createBuffer(), u = s.createBuffer();
  return {
    gl: s,
    ...te(s, e),
    program: t,
    programUniforms: {
      vertex: i,
      textureCoord: A,
      texture: r,
      matrix: o,
      uvMatrix: h,
      textColor: a,
      backColor: c
    },
    positionBuffer: n,
    textureBuffer: u
  };
}
function te(s, e) {
  const t = new OffscreenCanvas(0, 0), i = t.getContext("2d"), A = /* @__PURE__ */ new Map();
  let r = 0;
  const o = 0;
  let h = 1;
  i.font = e, i.fillStyle = "white";
  for (let c = 32; c < 128; c++) {
    const n = String.fromCharCode(c), u = i.measureText(n), f = Math.ceil(
      u.actualBoundingBoxDescent + u.actualBoundingBoxAscent + 1
    ), l = Math.ceil(
      u.actualBoundingBoxLeft + u.actualBoundingBoxRight + 1
    );
    A.set(n, {
      x: r,
      y: o,
      width: l,
      height: f,
      metrics: u
    }), h = Math.max(h, f), r += l;
  }
  t.width = r, t.height = h, i.font = e, i.fillStyle = "white";
  for (const [c, n] of A)
    i.fillText(
      c,
      Math.floor(n.x + n.metrics.actualBoundingBoxLeft + 1),
      Math.floor(n.metrics.actualBoundingBoxAscent + 1)
    );
  const a = s.createTexture();
  return s.bindTexture(s.TEXTURE_2D, a), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MIN_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MAG_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_S, s.CLAMP_TO_EDGE), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_T, s.CLAMP_TO_EDGE), s.texImage2D(s.TEXTURE_2D, 0, s.RGBA, s.RGBA, s.UNSIGNED_BYTE, t), { fontTexture: a, chars: A, textureSize: { width: r, height: h } };
}
function ie(s) {
  const e = s.gl;
  e.deleteBuffer(s.positionBuffer), e.deleteBuffer(s.textureBuffer), e.deleteTexture(s.fontTexture), e.deleteProgram(s.program);
}
function Ae(s, e, t, i, A, r, o) {
  const h = [], a = [], c = t;
  for (const f of e) {
    if (f === `
`) {
      t = c, i += o;
      continue;
    }
    const l = s.chars.get(f);
    if (l == null)
      continue;
    if (l.width === 1) {
      t += l.metrics.width;
      continue;
    }
    const p = Math.floor(t - l.metrics.actualBoundingBoxLeft), v = Math.floor(i - l.metrics.actualBoundingBoxAscent), T = p + l.width, x = v + l.height;
    h.push(p, v), a.push(l.x, l.y), h.push(p, x), a.push(l.x, l.y + l.height), h.push(p + l.width, x), a.push(l.x + l.width, l.y + l.height), h.push(T, x), a.push(l.x + l.width, l.y + l.height), h.push(p, v), a.push(l.x, l.y), h.push(T, v), a.push(l.x + l.width, l.y), t += l.metrics.width;
  }
  const n = s.gl;
  n.useProgram(s.program), n.bindBuffer(n.ARRAY_BUFFER, s.positionBuffer), n.bufferData(n.ARRAY_BUFFER, new Float32Array(h), n.STATIC_DRAW), n.vertexAttribPointer(
    s.programUniforms.vertex,
    2,
    n.FLOAT,
    !1,
    0,
    0
  ), n.enableVertexAttribArray(s.programUniforms.vertex), n.bindBuffer(n.ARRAY_BUFFER, s.textureBuffer), n.bufferData(
    n.ARRAY_BUFFER,
    new Float32Array(a),
    n.STATIC_DRAW
  ), n.vertexAttribPointer(
    s.programUniforms.textureCoord,
    2,
    n.FLOAT,
    !1,
    0,
    0
  ), n.enableVertexAttribArray(s.programUniforms.textureCoord), n.activeTexture(n.TEXTURE0), n.bindTexture(n.TEXTURE_2D, s.fontTexture), n.uniform1i(s.programUniforms.texture, 0), n.uniform3fv(s.programUniforms.textColor, [1, 1, 1]), n.uniform3fv(s.programUniforms.backColor, [0, 0, 0]);
  function u(f, l, p) {
    const v = [];
    for (let T = 0; T < l; T++)
      for (let x = 0; x < f; x++)
        v.push(p[x * f + T]);
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
  ])), n.viewport(0, 0, A, r), n.enable(n.BLEND), n.blendFunc(n.SRC_ALPHA, n.ONE_MINUS_SRC_ALPHA), n.drawArrays(n.TRIANGLES, 0, h.length / 2), n.disable(n.BLEND);
}
const d = {
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
}, E = 7, Q = 2, se = 16, re = 4e-3, k = 1, ne = 2, S = 5, oe = {
  a: "uA",
  b: "uB",
  fieldMetrics: "uFieldMetrics",
  first: "uFirst",
  size: "uSize"
}, V = 16, Y = 8, he = `#version 300 es

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
  const int BLOCK_W = ${V};
  const int BLOCK_H = ${Y};

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

  float run = texelFetch(uFieldMetrics, ivec2(${d.phase}, 0), 0)[1];
  float threshold = run == 0.0 ? 0.025 : (run <= 10.0 ? 0.11 : 0.15);

  vec4 even = measure(diffEven, signedEven, float(totalEven), threshold);
  vec4 odd = measure(diffOdd, signedOdd, float(totalOdd), threshold);
  outFirst = uFirst == 0 ? even : odd;
  outSecond = uFirst == 0 ? odd : even;
}
`, ae = {
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, g = 8, ce = `#version 300 es
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
`, le = {
  previous: "uPrevious",
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, ue = `#version 300 es
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
  float differing = abs(signedMean) > ${re} ? sum[3] : sum[1];
  return vec4(signedMean, differing, sum[2] / blocks, sum[3]);
}

bool same(float differing) {
  return differing <= ${Q}.0;
}

bool differs(float differing) {
  return differing >= ${se}.0;
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
  if (metric == ${d.firstRepeatsPrevious}) {
    outValue = previous(${d.firstRepeatsNext});
  } else if (metric == ${d.secondRepeatsNext}) {
    outValue = fold(uSecond);
  } else if (metric == ${d.secondRepeatsPrevious}) {
    outValue = previous(${d.secondRepeatsNext});
  } else if (metric == ${d.previousSecondRepeated}) {
    outValue = previous(${d.secondRepeatsPrevious});
  } else if (metric == ${d.firstRepeatsNext}) {
    outValue = fold(uFirst);
  } else if (metric == ${d.previousFirstRepeated}) {
    outValue = previous(${d.firstRepeatsPrevious});
  } else {
    outValue = decide(
      previous(${d.phase}),
      previous(${d.firstRepeatsNext})[1],
      fold(uSecond)[1],
      previous(${d.secondRepeatsNext})[1],
      previous(${d.secondRepeatsPrevious})[1],
      fold(uFirst)[1],
      previous(${d.firstRepeatsPrevious})[1]
    );
  }
}
`, fe = {
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
}, de = `#version 300 es
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
  return texelFetch(uFieldMetrics, ivec2(metric, 0), 0)[1] <= ${Q}.0;
}

/** The pulldown phase the detection gave this frame, or 0. See film-shader.ts. */
int detectedPhase() {
  return int(texelFetch(uFieldMetrics, ivec2(${d.phase}, 0), 0)[0]);
}

bool isMixedPhase(int phase) {
  return phase == ${k} || phase == ${ne};
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
class pe {
  #t;
  #c;
  #e;
  #n;
  #E;
  #s;
  #a;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #d = null;
  #p = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #A = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #o = null;
  #i = null;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(E * 4);
  #h = 0;
  #v = 0;
  constructor(e) {
    this.#t = e, this.#c = b(
      e,
      he,
      R
    ), this.#e = L(
      e,
      this.#c,
      oe
    ), this.#n = b(
      e,
      ce,
      R
    ), this.#E = L(
      e,
      this.#n,
      ae
    ), this.#s = b(
      e,
      ue,
      R
    ), this.#a = L(e, this.#s, le);
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
    const i = new Float32Array(E * 4);
    for (let A = 0; A < d.phase; A++)
      i[A * 4 + 1] = 1;
    e.bindTexture(e.TEXTURE_2D, t.textures[0] ?? null), e.texSubImage2D(
      e.TEXTURE_2D,
      0,
      0,
      0,
      E,
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
    const r = this.#d, o = this.#p, h = this.#A;
    if (r === null || o === null || h === null) return;
    const a = h[this.#r], c = h[1 - this.#r];
    A.bindFramebuffer(A.FRAMEBUFFER, r.framebuffer), A.useProgram(this.#c), this.#D(0, e, this.#e.a), this.#D(1, t, this.#e.b), this.#D(2, a.textures[0], this.#e.fieldMetrics), A.uniform1i(this.#e.first, i), A.uniform2i(this.#e.size, this.#h, this.#v), A.viewport(0, 0, r.width, r.height), A.drawArrays(A.TRIANGLES, 0, 3), A.bindFramebuffer(A.FRAMEBUFFER, o.framebuffer), A.useProgram(this.#n), this.#D(0, r.textures[0], this.#E.second), this.#D(1, r.textures[1], this.#E.first), A.uniform2i(this.#E.size, r.width, r.height), A.viewport(0, 0, o.width, o.height), A.drawArrays(A.TRIANGLES, 0, 3), A.bindFramebuffer(A.FRAMEBUFFER, c.framebuffer), A.useProgram(this.#s), this.#D(0, a.textures[0], this.#a.previous), this.#D(1, o.textures[0], this.#a.second), this.#D(2, o.textures[1], this.#a.first), A.uniform2i(this.#a.size, o.width, o.height), A.viewport(0, 0, E, 1), A.drawArrays(A.TRIANGLES, 0, 3), this.#r = 1 - this.#r, A.deleteSync(this.#i), A.bindBuffer(A.PIXEL_PACK_BUFFER, this.#o), A.readPixels(0, 0, E, 1, A.RGBA, A.FLOAT, 0), A.bindBuffer(A.PIXEL_PACK_BUFFER, null), A.bindFramebuffer(A.FRAMEBUFFER, null), this.#i = A.fenceSync(A.SYNC_GPU_COMMANDS_COMPLETE, 0), A.flush();
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
      d.phase,
      0,
      1,
      1,
      i.RGBA,
      i.FLOAT,
      new Float32Array([e, t, 0, 0])
    ), i.deleteSync(this.#i), i.bindFramebuffer(i.FRAMEBUFFER, A.framebuffer), i.bindBuffer(i.PIXEL_PACK_BUFFER, this.#o), i.readPixels(0, 0, E, 1, i.RGBA, i.FLOAT, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), this.#i = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), i.flush());
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
          phase: this.metrics[d.phase * 4] ?? 0,
          run: this.metrics[d.phase * 4 + 1] ?? 0
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
    this.#d !== null && B(e, this.#d), this.#p !== null && B(e, this.#p), this.#d = null, this.#p = null;
  }
  /** Everything detect needs that is not there yet. */
  #P() {
    const e = this.#t;
    if (this.#d === null || this.#p === null) {
      this.#x();
      const t = Math.ceil(this.#h / V), i = Math.ceil(this.#v / (Y * 2));
      this.#d = _(e, t, i, 2), this.#p = _(
        e,
        Math.ceil(t / g),
        Math.ceil(i / g),
        2
      );
    }
    this.#A === null && (this.#A = [
      _(e, E, 1, 1),
      _(e, E, 1, 1)
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
  for (let a = 0; a < i; a++) {
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
      s.COLOR_ATTACHMENT0 + a,
      s.TEXTURE_2D,
      c,
      0
    ), r.push(c);
  }
  s.drawBuffers(r.map((a, c) => s.COLOR_ATTACHMENT0 + c));
  const o = s.checkFramebufferStatus(s.FRAMEBUFFER) === s.FRAMEBUFFER_COMPLETE;
  s.bindFramebuffer(s.FRAMEBUFFER, null);
  const h = { framebuffer: A, textures: r, width: e, height: t };
  if (!o)
    throw B(s, h), new Error("failed to allocate framebuffer");
  return h;
}
function B(s, { framebuffer: e, textures: t }) {
  s.deleteFramebuffer(e);
  for (const i of t) s.deleteTexture(i);
}
const j = [
  "mozParsedFrames",
  "mozDecodedFrames",
  "mozPresentedFrames",
  "mozPaintedFrames"
];
function K(s) {
  return j.every((e) => e in s);
}
function me() {
  return typeof HTMLVideoElement < "u" && (K(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const Ee = 250, ve = 500;
class xe {
  #t;
  #c;
  #e = null;
  #n = null;
  #E = null;
  #s = null;
  #a = !0;
  #d = null;
  #p = null;
  #A = 0;
  constructor(e) {
    if (this.#t = e, this.#c = K(e) ? e : null, this.#c) {
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
    this.#d = null, this.#p = null, this.#A = 0;
  };
  #o = () => {
    this.#E = null, this.#s = null, this.#a = !0, this.#r();
  };
  #i = (e, t) => {
    const i = this.#n;
    this.#e = null, this.#n = null, i?.(e, t);
  };
  #h = (e) => {
    const t = this.#c, i = j.map((h) => t[h]);
    this.#E?.some((h, a) => i[a] < h) && this.#o(), this.#E = i;
    const A = t.mozPaintedFrames, r = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, o = this.#s === null && (A > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (r && (o || this.#s !== null && A !== this.#s)) {
      if (this.#d !== null && e - this.#d > ve && (this.#r(), this.#a = !0), !t.paused && !t.ended) {
        const a = this.#p;
        if (a && e - a.at >= Ee) {
          const c = A - a.frames, n = (e - a.at) / c;
          c > 0 && n >= 4 && n <= 200 && (this.#A = this.#A ? this.#A + (n - this.#A) * 0.25 : n), this.#p = null;
        }
        this.#p ??= { at: e, frames: A };
      }
      this.#d = e, this.#s = A;
      const h = this.#a;
      this.#a = !1, this.#i(e, {
        width: t.videoWidth,
        height: t.videoHeight,
        // Used only to select source scan/size metadata on the media timeline.
        // It is deliberately NOT used as the frame identity or field clock.
        mediaTime: t.currentTime,
        presentedFrames: A,
        expectedDisplayTime: e,
        mozTiming: { periodMs: this.#A, discontinuity: h }
      });
    } else
      this.#e = requestAnimationFrame(this.#h);
  };
}
const we = 0.5, m = 4, w = 5, X = 1e3, Te = 4, I = 200, De = 0.25, Fe = 0.75, be = 0.1, O = 1, Re = 1e3 / 60, ge = 0.02, _e = 0.1, H = 1, M = 5, Me = 4, z = 1001 / 3e4, y = 3, ye = 60, Se = 3, Be = {
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
  return me() && typeof WebGL2RenderingContext < "u";
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
  #d;
  #p;
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
  #x = Re;
  #P = 0;
  /** How far ahead of its moment the last picture was shown. See #present. */
  #_e = 0;
  /** What makes the pictures between pictures, and the last picture put up. */
  #C = null;
  #F = null;
  /** The pair the interpolator was last handed. */
  #re = null;
  /** Debug only: the rAF the last picture was drawn in, and repeats dropped since. */
  #fe = 0;
  #de = 0;
  /** The `<div>` this put around the element, so it can be taken away again. */
  #X = null;
  #pe;
  #O;
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
  #u = 0;
  #I = 0;
  #g = !1;
  #me = !1;
  #f = null;
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
  #ne = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #ve = 0;
  #oe = 0;
  #U = 0;
  #ee = 0;
  #S = 0;
  #z = 0;
  #W = 0;
  #Z;
  #he = [];
  #$ = [];
  #te = 0;
  #Q = 0;
  #ie = 0;
  #V = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #Y = D;
  #k = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #m = D;
  #N = !1;
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
  #Ae = !1;
  #j = null;
  #ye = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #ae = [0, 0, 0, 0, 0, 0];
  #xe = 0;
  constructor(e, t = {}) {
    this.#t = e, this.#O = t.doubleRate ?? !1, this.#Me = t.spatialCheck ?? !0, this.#L = t.debug ?? !1, this.#C = t.interpolator ?? null, this.#y = t.film ?? !1, this.#Ee = t.onStats, this.canvas = document.createElement("canvas"), this.canvas.style.cssText = "position:absolute;pointer-events:none;visibility:hidden";
    const i = this.canvas.getContext("webgl2", {
      alpha: !1,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!i) throw new Error("this browser has no WebGL2");
    this.#e = i, this.#n = new pe(i), this.#E = b(i, de, R);
    const A = this.#E;
    this.#s = Object.fromEntries(
      Object.entries(fe).map(([r, o]) => [
        r,
        i.getUniformLocation(A, o)
      ])
    ), this.#a = b(i, Pe, R), this.#d = i.getUniformLocation(this.#a, "uField"), this.#p = i.getUniformLocation(this.#a, "uFlip"), this.#y && this.#Se(), this.#Z = i.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.canvas.addEventListener("webglcontextlost", this.#$e), this.#pe = new ResizeObserver(() => this.#ue()), this.#c = new xe(e), e.addEventListener("emptied", this.#We), e.addEventListener("resize", this.#ze), e.addEventListener("pause", this.#B), e.addEventListener("ended", this.#B), e.addEventListener("seeked", this.#B), e.addEventListener("ratechange", this.#B);
  }
  get running() {
    return this.#g && (this.#f?.interlaced ?? !0);
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
    this.#f = e, this.#Te();
  }
  get scan() {
    return this.#f;
  }
  set videoTimeline(e) {
    this.#q = e, e.length === 0 && (this.#f = null), this.#Te();
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
    return this.#X ?? this.#t;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#O;
  }
  set doubleRate(e) {
    e !== this.#O && (this.#O = e, this.#we());
  }
  get film() {
    return this.#y;
  }
  set film(e) {
    e !== this.#y && (e && this.#Se(), this.#y = e, e || (this.#N = !1, this.#m = D, this.#J(), this.#G()), this.#we());
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
    e !== this.#C && (this.#C = e, this.#re = null, this.#we());
  }
  /** Whether pictures are queued and put up by the loop rather than drawn on arrival. */
  get #K() {
    return this.#O || this.#y || (this.#C?.wants("frame") ?? !1);
  }
  #we() {
    this.#K ? (this.#w > 0 && this.#He(), this.#Fe()) : (this.#be(), this.#se());
  }
  #Se() {
    if (this.#e.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  #Te() {
    this.#me && (this.#q.length > 0 || (this.#f?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  start() {
    this.#g || this.#H || (this.#g = !0, this.#Ze(), this.#ot(), this.#ce(), (this.#f?.interlaced ?? !0) && this.#Fe());
  }
  /** Take the deinterlaced picture away, leaving the element's own showing. */
  stop() {
    this.#g && (this.#g = !1, this.#c.cancel(), this.#be(), this.#u = 0, this.canvas.style.visibility = "hidden");
  }
  destroy() {
    this.stop(), this.#c.destroy(), this.canvas.removeEventListener("webglcontextlost", this.#$e), this.#t.removeEventListener("emptied", this.#We), this.#t.removeEventListener("resize", this.#ze), this.#t.removeEventListener("pause", this.#B), this.#t.removeEventListener("ended", this.#B), this.#t.removeEventListener("seeked", this.#B), this.#t.removeEventListener("ratechange", this.#B), this.#ht();
    for (const e of this.#A) this.#e.deleteTexture(e);
    this.#A = [], this.#se();
    for (const e of [
      ...this.#he,
      ...this.#$.map(({ q: t }) => t)
    ])
      this.#e.deleteQuery(e);
    this.#he.length = 0, this.#$.length = 0, this.#n.destroy(), this.#j !== null && (ie(this.#j), this.#j = null), this.#e.deleteProgram(this.#E), this.#e.deleteProgram(this.#a), this.#e.getExtension("WEBGL_lose_context")?.loseContext();
  }
  #ce() {
    this.#g && this.#c.request(this.#Ke);
  }
  #Qe(e, t, i) {
    this.#j == null && (this.#j = ee(this.#e, "20px monospace")), Ae(this.#j, e, t, i, this.#w, this.#R, 20);
  }
  #Be(e) {
    if (this.#Z == null || this.#$.length > 30)
      return;
    const t = this.#he.pop() ?? this.#e.createQuery();
    return this.#e.beginQuery(this.#Z.TIME_ELAPSED_EXT, t), this.#$.push({ q: t, isField: e }), t;
  }
  #Pe(e) {
    this.#Z != null && (e != null && this.#e.endQuery(this.#Z.TIME_ELAPSED_EXT), this.#$ = this.#$.filter(
      ({ q: t, isField: i }) => {
        if (this.#e.getQueryParameter(t, this.#e.QUERY_RESULT_AVAILABLE)) {
          const A = this.#e.getQueryParameter(t, this.#e.QUERY_RESULT);
          return i ? (this.#ie += A, this.#V++) : (this.#te += A, this.#Q++), this.#he.push(t), !1;
        }
        return !0;
      }
    ));
  }
  #J() {
    this.#_ = null, this.#M = 0, this.#Ae = !1;
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
    const i = this.#M === y && !this.#Ae ? Se : (this.#_.phase - 1 + Math.round(t / z)) % M + 1;
    this.#Y = this.#m = { phase: i, run: S }, this.#k = 0, this.#n.seed(i, S), this.#M--, this.#M === 0 && (this.#Ae = !1);
  }
  #G() {
    this.#m = D, this.#Y = D, this.#k = 0, this.#N = !1, this.#n.reset();
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #Ve() {
    const { cur: e, next: t } = this.#Xe(!1), i = this.#A[e], A = this.#A[t];
    if (!i || !A) return;
    const r = this.#f?.topFieldFirst !== !1 ? 0 : 1;
    this.#n.detect(i, A, r);
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #Ye() {
    const e = this.#n.poll();
    e !== null && (this.#Y = e, this.#k = 0), this.#k++;
    const { phase: t, run: i } = this.#Y;
    t === 0 || this.#k > M ? this.#m = D : this.#m = {
      phase: (t - 1 + this.#k) % M + 1,
      run: i
    };
  }
  #je(e) {
    const t = [], i = this.#n.metrics;
    for (let r = 0; r < d.phase; r++) {
      const o = i[r * 4] ?? 0, h = i[r * 4 + 1] ?? 0, a = i[r * 4 + 2] ?? 0;
      t.push(
        `${o.toFixed(4)},${h.toString().padStart(4)},${a.toFixed(3)}`
      );
    }
    const A = this.#ae.map((r, o) => `${o === 0 ? "-" : o}:${r}`).join(" ");
    return `frame=${e} phase=${this.#m.phase} run=${this.#m.run} known=${this.#Y.phase}/${this.#Y.run} age=${this.#k} ${this.#N ? "film" : "video"} period=${this.#l.toFixed(3)}
${t.join(" ")}
${A} dropped:${this.#xe}`;
  }
  /** Whether this frame is not the one that follows the last one seen. */
  #Le(e) {
    if (e.mozTiming) return e.mozTiming.discontinuity;
    const t = e.mediaTime - this.#I;
    return t < 0 || t > we;
  }
  #Ke = (e, t) => {
    if (!(!this.#g || this.#H)) {
      if (this.#Je(t.mediaTime), t.width > 0 && t.height > 0) {
        if ((this.#w === 0 || this.#R === 0) && this.#Oe(t.width, t.height), this.#f && !this.#f.interlaced) {
          const n = this.#Le(t);
          n && this.#J(), this.#Ne(t.presentedFrames, n), this.#I = t.mediaTime, this.#rt(), this.#ce();
          return;
        }
        const i = t.mediaTime - this.#I, A = this.#Le(t);
        A && (this.#J(), this.#u = 0, this.#b.discontinuities++, this.#i.length = 0, this.#h = null, this.#F = null, this.#G());
        const r = this.#Ne(t.presentedFrames, A);
        if (!t.mozTiming && this.#u > 0 && t.mediaTime === this.#I) {
          this.#ce();
          return;
        }
        t.mozTiming ? this.#l = t.mozTiming.periodMs : !A && i > 0 && this.#qe(i);
        const o = this.#I;
        this.#I = t.mediaTime;
        const h = performance.now();
        h - this.#ve > X && (this.#oe = h, this.#U = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0), this.#ve = h;
        const a = performance.now(), c = this.#Be(!1);
        if (this.#Ge(), this.#W = Math.max(
          this.#W,
          this.#i.length
        ), this.#y && (this.#u === m && r === 0 ? (this.#Ye(), this.#Ve(), this.#Ce(t.mediaTime)) : r > 0 && this.#u === m && this.#_ !== null ? (this.#G(), this.#M !== y && (this.#M = y, this.#Ae = !0), this.#Ce(o + z)) : this.#G(), this.#ae[this.#m.phase] = (this.#ae[this.#m.phase] ?? 0) + 1, this.#N = this.#m.phase !== 0 && this.#m.run >= S, this.#N && this.#M === 0 && (this.#_ = {
          mediaTime: t.mediaTime,
          phase: this.#m.phase
        }), this.#L && (this.#ye = this.#je(t.presentedFrames))), this.#Ie() && !this.#t.paused && !this.#t.ended) {
          this.#i.length >= w && (this.#i.length = 0, this.#h = null, this.#F = null, this.#b.queueResetted++);
          const n = (t.expectedDisplayTime || e) + this.#x;
          if (this.#N) {
            const u = this.#m.phase;
            if (u === k)
              this.#xe++, this.#de++;
            else {
              const f = this.#l * M / Me, l = Be[u] ?? 0, p = this.#De(
                "film",
                n + l * this.#l,
                f
              );
              this.#le("film", u, !1, p, f);
            }
          } else if (this.#O) {
            const u = this.#l / 2, f = this.#De("field", n, u);
            this.#le("field", 1, !1, f, u), this.#le("field", 2, !0, f + u, u);
          } else {
            const u = this.#l, f = this.#De("frame", n, u);
            this.#le("frame", 0, !1, f, u);
          }
        } else
          this.#ge(this.#t.paused || this.#t.ended, !1, null);
        this.#Pe(c), this.#ee += performance.now() - a, this.#U++, this.#nt(h);
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
    if (!i || this.#f?.interlaced === i.interlaced && this.#f.topFieldFirst === i.topFieldFirst)
      return;
    const A = this.#f?.interlaced === !1;
    this.#f = i, this.#_ === null && (this.#u = 0), this.#i.length = 0, this.#h = null, this.#F = null, this.#G(), i.interlaced && A && this.#_ !== null && (this.#M = y, this.#Ae = !1), i.interlaced ? this.#K && this.#Fe() : this.#be();
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
        const h = Math.max(
          -O,
          Math.min(O, o * be)
        );
        return r + h;
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
    A < Te || A > I || (this.#l = this.#l > 0 && A > this.#l * Fe ? this.#l + (A - this.#l) * De : A);
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
    const o = (this.#o + 1) % w, h = this.#r[o];
    if (!h) return;
    for (this.#o = o, this.#F?.slot === o && (this.#F = null); this.#i.length > 0 && this.#i[0]?.slot === o; )
      this.#i.shift(), this.#b.late++;
    this.#ge(!1, i, h.framebuffer);
    const a = {
      slot: o,
      at: A,
      duration: r,
      cadence: e,
      phase: t,
      droppedBefore: this.#de
    };
    this.#de = 0, this.#i.push(a), this.#h = a;
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
    this.#x += r / i * ge, this.#P = A + r * _e;
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
    const i = this.#x / 2, A = (a) => {
      const c = a.at + this.#Re(a) - e;
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
    const o = performance.now(), h = this.#Be(!0);
    this.#st(r.slot), this.#Pe(h), this.#z += performance.now() - o, this.#S++, this.#L && this.#At(r, t), this.#fe = t;
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
    const h = this.#r[A.slot], a = this.#r[e.slot];
    if (!h || !a) return;
    const c = this.#e;
    (this.#re?.previous !== A || this.#re.next !== e) && (i.pair(c, h.texture, a.texture, this.#w, this.#R), this.#re = { previous: A, next: e });
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
    e && this.#ke(e, !0), this.#_ === null && (this.#u = 0);
  }
  #ke(e, t = !1) {
    const i = this.#e;
    i.bindFramebuffer(i.FRAMEBUFFER, null), i.useProgram(this.#a), i.activeTexture(i.TEXTURE0), i.bindTexture(i.TEXTURE_2D, e), i.uniform1i(this.#d, 0), i.uniform1i(this.#p, t ? 1 : 0), i.viewport(0, 0, this.#w, this.#R), i.drawArrays(i.TRIANGLES, 0, 3), this.canvas.style.visibility = "visible";
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
    return this.#ne !== 0 && !t && (i = Math.max(0, e - this.#ne - 1), this.#b.missed += i), this.#ne = e, i;
  }
  #nt(e) {
    if (!this.#Ee) return;
    const t = e - this.#oe;
    if (t < X) return;
    const i = this.#Ie() ? this.#S : this.#U;
    let A = 0;
    this.#U !== 0 && (A += this.#ee / this.#U), this.#S !== 0 && (A += this.#z / this.#S / 2);
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
      film: this.#N
    }), this.#oe = e, this.#U = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0;
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
    ), this.#u = Math.min(this.#u + 1, m);
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
    if (this.#u === 0 || this.#H) return;
    this.#u === m && !e ? this.#b.filtered++ : this.#b.degraded++;
    const A = this.#e, { prev: r, cur: o, next: h } = this.#Xe(e);
    A.bindFramebuffer(A.FRAMEBUFFER, i), A.useProgram(this.#E);
    for (const [u, f] of [r, o, h].entries())
      A.activeTexture(A.TEXTURE0 + u), A.bindTexture(A.TEXTURE_2D, this.#A[f] ?? null);
    A.uniform1i(this.#s.prev, 0), A.uniform1i(this.#s.cur, 1), A.uniform1i(this.#s.next, 2);
    const a = this.#y ? this.#n.texture : null, c = a !== null;
    a !== null && (A.activeTexture(A.TEXTURE0 + 3), A.bindTexture(A.TEXTURE_2D, a), A.uniform1i(this.#s.fieldMetrics, 3)), A.uniform2i(this.#s.size, this.#w, this.#R);
    const n = this.#f?.topFieldFirst !== !1 ? 0 : 1;
    A.uniform1i(this.#s.parity, t ? 1 - n : n), A.uniform1i(this.#s.second, t ? 1 : 0), A.uniform1i(
      this.#s.tff,
      this.#f?.topFieldFirst !== !1 ? 1 : 0
    ), A.uniform1i(this.#s.spatialCheck, this.#Me ? 1 : 0), A.uniform1i(this.#s.debug, this.#L ? 1 : 0), A.uniform1i(this.#s.film, c ? 1 : 0), A.uniform1i(this.#s.phase, this.#m.phase), A.viewport(0, 0, this.#w, this.#R), A.drawArrays(A.TRIANGLES, 0, 3), this.#L && c && this.#Qe(this.#ye, 0, 90), i === null && (this.canvas.style.visibility = "visible");
  }
  #Xe(e) {
    const t = (i) => (this.#T + m - i) % m;
    return this.#u === 1 ? { prev: this.#T, cur: this.#T, next: this.#T } : e ? { prev: t(1), cur: this.#T, next: this.#T } : this.#u === 2 ? { prev: t(1), cur: t(1), next: this.#T } : { prev: t(2), cur: t(1), next: this.#T };
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
    if (!this.#X) return;
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
    this.canvas.width = e, this.canvas.height = t, this.#w = e, this.#R = t, this.#u = 0, this.#ue();
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
    this.#se(), this.#n.resize(e, t), this.#K && this.#He();
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
      this.#se();
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
          e.deleteFramebuffer(A), e.deleteTexture(i), this.#se();
          return;
        }
        this.#r.push({ texture: i, framebuffer: A });
      }
      this.#o = w - 1;
    }
  }
  #se() {
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
    if (this.#X) return;
    const e = this.#t.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#t), t.appendChild(this.#t), t.appendChild(this.canvas), this.#X = t, this.#pe.observe(this.#t), this.#ue();
  }
  #ht() {
    const e = this.#X;
    this.#X = null, this.#pe.disconnect(), this.canvas.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#t, e), e.remove());
  }
  #ze = () => this.#ue();
  #We = () => {
    this.#u = 0, this.#I = 0, this.#i.length = 0, this.#h = null, this.#F = null, this.#J(), this.#G(), this.#l = 0, this.#Ze(), this.canvas.style.visibility = "hidden";
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
    }, this.#ae.fill(0), this.#xe = 0, this.#ne = 0, this.#oe = 0, this.#ve = 0, this.#U = 0, this.#ee = 0, this.#S = 0, this.#z = 0, this.#W = 0, this.#te = 0, this.#Q = 0, this.#ie = 0, this.#V = 0;
  }
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #B = () => {
    this.#J(), this.#i.length = 0, this.#h = null, this.#F = null, this.#G(), this.#g && this.#ge(!0, !1, null);
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
const W = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", Ce = 0.5, Le = 3e3, Z = 0.1, F = 16, $ = 'video/mp4; codecs="avc1.640029"';
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
  const e = s.tolerance ?? Ce, t = s.timeoutMs ?? Le, i = performance.now(), A = (h) => ({
    deinterlaces: !1,
    survives: null,
    tookMs: performance.now() - i,
    error: h instanceof Error ? h.message : String(h)
  });
  if (typeof document > "u")
    return A(new Error("there is no document to decode in"));
  const r = document.createElement("video");
  r.muted = !0, r.defaultMuted = !0, r.playsInline = !0, r.preload = "auto";
  let o = null;
  try {
    o = Ne(r, t);
    const h = C(P(r, "loadeddata"), t), a = r.play().then(
      () => !0,
      () => !1
    );
    if (await o.ready, await h, await Ge(r, t, await a), r.videoWidth === 0 || r.videoHeight === 0)
      return A(new Error("the probe clip decoded to nothing"));
    const c = Xe(r);
    return {
      deinterlaces: c < 1 - e,
      survives: c,
      tookMs: performance.now() - i
    };
  } catch (h) {
    return A(h);
  } finally {
    r.pause(), r.removeAttribute("src"), r.replaceChildren(), r.load(), o && URL.revokeObjectURL(o.url);
  }
}
const U = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, ke = typeof MediaSource > "u";
function Ne(s, e) {
  if (!U || !U.isTypeSupported($))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = W.indexOf(","), i = atob(W.slice(t + 1)), A = new Uint8Array(i.length);
  for (let a = 0; a < i.length; a++) A[a] = i.charCodeAt(a);
  const r = new U(), o = URL.createObjectURL(r);
  if (ke) {
    s.disableRemotePlayback = !0;
    const a = document.createElement("source");
    a.type = "video/mp4", a.src = o, s.append(a), s.load();
  } else
    s.src = o;
  const h = (async () => {
    await C(P(r, "sourceopen"), e);
    const a = r.addSourceBuffer($), c = C(P(a, "updateend"), e);
    a.appendBuffer(A), await c, r.endOfStream();
  })();
  return { url: o, ready: h };
}
async function Ge(s, e, t) {
  if (t) {
    const i = performance.now();
    for (; s.currentTime < Z && performance.now() - i < e; )
      await new Promise((A) => requestAnimationFrame(A));
    s.pause();
  } else
    s.currentTime = Z, await C(P(s, "seeked"), e);
}
function Xe(s) {
  const e = s.videoHeight, t = document.createElement("canvas");
  t.width = F, t.height = e;
  const i = t.getContext("2d", { willReadFrequently: !0 });
  if (!i) throw new Error("there is no 2d context to read the clip with");
  i.imageSmoothingEnabled = !1, i.drawImage(s, 0, 0, F, e);
  const A = i.getImageData(0, 0, F, e).data, r = (n) => {
    let u = 0;
    for (let f = 0; f < F; f++)
      u += A[(n * F + f) * 4 + 1] ?? 0;
    return u / F;
  };
  let o = 0;
  const h = 2, a = e - 3;
  let c = r(h);
  for (let n = h + 1; n <= a; n++) {
    const u = r(n);
    o += Math.abs(u - c), c = u;
  }
  return o / (a - h) / 255;
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
  de as YADIF_FRAGMENT_SHADER,
  fe as YADIF_UNIFORMS,
  ze as decoderDeinterlaces,
  We as forgetDecoderProbe,
  Ie as probeDecoder,
  Oe as supportsDeinterlace
};
