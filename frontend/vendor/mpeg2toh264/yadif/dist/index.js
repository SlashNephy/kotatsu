const b = `#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;
function R(s, e, t) {
  const A = s.createProgram(), i = G(s, s.VERTEX_SHADER, t), r = G(s, s.FRAGMENT_SHADER, e);
  if (s.attachShader(A, i), s.attachShader(A, r), s.linkProgram(A), s.deleteShader(i), s.deleteShader(r), !s.getProgramParameter(A, s.LINK_STATUS)) {
    const n = s.getProgramInfoLog(A);
    throw s.deleteProgram(A), new Error(
      `the deinterlacer failed to link: ${n ?? "no reason given"}`
    );
  }
  return A;
}
function G(s, e, t) {
  const A = s.createShader(e);
  if (!A) throw new Error("the deinterlacer could not create a shader");
  if (s.shaderSource(A, t), s.compileShader(A), !s.getShaderParameter(A, s.COMPILE_STATUS)) {
    const i = s.getShaderInfoLog(A);
    throw s.deleteShader(A), new Error(
      `the deinterlacer failed to compile: ${i ?? "no reason given"}`
    );
  }
  return A;
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
  const t = R(s, K, J), A = s.getAttribLocation(t, "aVertexPosition"), i = s.getAttribLocation(t, "aTextureCoord"), r = s.getUniformLocation(t, "uTexture"), n = s.getUniformLocation(t, "uMatrix"), a = s.getUniformLocation(t, "uUvMatrix"), h = s.getUniformLocation(t, "uTextColor"), c = s.getUniformLocation(t, "uBackColor");
  if (r == null || n == null || a == null || h == null || c == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const o = s.createBuffer(), l = s.createBuffer();
  return {
    gl: s,
    ...ee(s, e),
    program: t,
    programUniforms: {
      vertex: A,
      textureCoord: i,
      texture: r,
      matrix: n,
      uvMatrix: a,
      textColor: h,
      backColor: c
    },
    positionBuffer: o,
    textureBuffer: l
  };
}
function ee(s, e) {
  const t = new OffscreenCanvas(0, 0), A = t.getContext("2d"), i = /* @__PURE__ */ new Map();
  let r = 0;
  const n = 0;
  let a = 1;
  A.font = e, A.fillStyle = "white";
  for (let c = 32; c < 128; c++) {
    const o = String.fromCharCode(c), l = A.measureText(o), d = Math.ceil(
      l.actualBoundingBoxDescent + l.actualBoundingBoxAscent + 1
    ), u = Math.ceil(
      l.actualBoundingBoxLeft + l.actualBoundingBoxRight + 1
    );
    i.set(o, {
      x: r,
      y: n,
      width: u,
      height: d,
      metrics: l
    }), a = Math.max(a, d), r += u;
  }
  t.width = r, t.height = a, A.font = e, A.fillStyle = "white";
  for (const [c, o] of i)
    A.fillText(
      c,
      Math.floor(o.x + o.metrics.actualBoundingBoxLeft + 1),
      Math.floor(o.metrics.actualBoundingBoxAscent + 1)
    );
  const h = s.createTexture();
  return s.bindTexture(s.TEXTURE_2D, h), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MIN_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_MAG_FILTER, s.LINEAR), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_S, s.CLAMP_TO_EDGE), s.texParameteri(s.TEXTURE_2D, s.TEXTURE_WRAP_T, s.CLAMP_TO_EDGE), s.texImage2D(s.TEXTURE_2D, 0, s.RGBA, s.RGBA, s.UNSIGNED_BYTE, t), { fontTexture: h, chars: i, textureSize: { width: r, height: a } };
}
function te(s) {
  const e = s.gl;
  e.deleteBuffer(s.positionBuffer), e.deleteBuffer(s.textureBuffer), e.deleteTexture(s.fontTexture), e.deleteProgram(s.program);
}
function Ae(s, e, t, A, i, r, n) {
  const a = [], h = [], c = t;
  for (const d of e) {
    if (d === `
`) {
      t = c, A += n;
      continue;
    }
    const u = s.chars.get(d);
    if (u == null)
      continue;
    if (u.width === 1) {
      t += u.metrics.width;
      continue;
    }
    const E = Math.floor(t - u.metrics.actualBoundingBoxLeft), v = Math.floor(A - u.metrics.actualBoundingBoxAscent), T = E + u.width, x = v + u.height;
    a.push(E, v), h.push(u.x, u.y), a.push(E, x), h.push(u.x, u.y + u.height), a.push(E + u.width, x), h.push(u.x + u.width, u.y + u.height), a.push(T, x), h.push(u.x + u.width, u.y + u.height), a.push(E, v), h.push(u.x, u.y), a.push(T, v), h.push(u.x + u.width, u.y), t += u.metrics.width;
  }
  const o = s.gl;
  o.useProgram(s.program), o.bindBuffer(o.ARRAY_BUFFER, s.positionBuffer), o.bufferData(o.ARRAY_BUFFER, new Float32Array(a), o.STATIC_DRAW), o.vertexAttribPointer(
    s.programUniforms.vertex,
    2,
    o.FLOAT,
    !1,
    0,
    0
  ), o.enableVertexAttribArray(s.programUniforms.vertex), o.bindBuffer(o.ARRAY_BUFFER, s.textureBuffer), o.bufferData(
    o.ARRAY_BUFFER,
    new Float32Array(h),
    o.STATIC_DRAW
  ), o.vertexAttribPointer(
    s.programUniforms.textureCoord,
    2,
    o.FLOAT,
    !1,
    0,
    0
  ), o.enableVertexAttribArray(s.programUniforms.textureCoord), o.activeTexture(o.TEXTURE0), o.bindTexture(o.TEXTURE_2D, s.fontTexture), o.uniform1i(s.programUniforms.texture, 0), o.uniform3fv(s.programUniforms.textColor, [1, 1, 1]), o.uniform3fv(s.programUniforms.backColor, [0, 0, 0]);
  function l(d, u, E) {
    const v = [];
    for (let T = 0; T < u; T++)
      for (let x = 0; x < d; x++)
        v.push(E[x * d + T]);
    return v;
  }
  o.uniformMatrix4fv(s.programUniforms.matrix, !1, l(4, 4, [
    1 / (i / 2),
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
  ])), o.uniformMatrix3fv(s.programUniforms.uvMatrix, !1, l(3, 3, [
    1 / s.textureSize.width,
    0,
    0,
    0,
    1 / s.textureSize.height,
    0,
    0,
    0,
    1
  ])), o.viewport(0, 0, i, r), o.enable(o.BLEND), o.blendFunc(o.SRC_ALPHA, o.ONE_MINUS_SRC_ALPHA), o.drawArrays(o.TRIANGLES, 0, a.length / 2), o.disable(o.BLEND);
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
}, p = 7, Q = 2, ie = 16, I = 1, se = 2, M = 5, re = {
  a: "uA",
  b: "uB",
  fieldMetrics: "uFieldMetrics",
  first: "uFirst",
  size: "uSize"
}, $ = 16, V = 8, oe = `#version 300 es

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

vec4 measure(float diff, float total, float threshold)
{
  diff /= total;
  return vec4(diff, diff > threshold ? 1.0 : 0.0, diff, 1.0);
}

void main()
{
  const int BLOCK_W = ${$};
  const int BLOCK_H = ${V};

  ivec2 block = ivec2(gl_FragCoord.xy);
  ivec2 base = ivec2(block.x * BLOCK_W, block.y * BLOCK_H * 2);

  // Even and odd frame lines, summed apart.
  float diffEven = 0.0;
  float diffOdd = 0.0;
  int totalEven = 0;
  int totalOdd = 0;

  for (int y = 0; y < BLOCK_H; ++y) {
    for (int x = 0; x < BLOCK_W; ++x) {
      ivec2 p = base + ivec2(x, y * 2);

      if (p.x < uSize.x && p.y < uSize.y) {
        float a = luma(texelFetch(uA, p, 0).rgb);
        float b = luma(texelFetch(uB, p, 0).rgb);

        diffEven += abs(a - b);
        totalEven += 1;
      }
      p.y += 1;
      if (p.x < uSize.x && p.y < uSize.y) {
        float a = luma(texelFetch(uA, p, 0).rgb);
        float b = luma(texelFetch(uB, p, 0).rgb);

        diffOdd += abs(a - b);
        totalOdd += 1;
      }
    }
  }

  float run = texelFetch(uFieldMetrics, ivec2(${f.phase}, 0), 0)[1];
  float threshold = run == 0.0 ? 0.025 : (run <= 10.0 ? 0.11 : 0.15);

  vec4 even = measure(diffEven, float(totalEven), threshold);
  vec4 odd = measure(diffOdd, float(totalOdd), threshold);
  outFirst = uFirst == 0 ? even : odd;
  outSecond = uFirst == 0 ? odd : even;
}
`, ne = {
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, g = 8, he = `#version 300 es
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
        vec4 value = texelFetch(uSecond, p, 0);
        second[0] = max(second[0], value[0]);
        second.yzw += value.yzw;
        value = texelFetch(uFirst, p, 0);
        first[0] = max(first[0], value[0]);
        first.yzw += value.yzw;
      }
    }
  }

  outSecond = second;
  outFirst = first;
}
`, ae = {
  previous: "uPrevious",
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, ce = `#version 300 es
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

/** Fold what REDUCTION left into one texel. */
vec4 fold(sampler2D reduced) {
  vec4 sum = vec4(0.0);
  for (int y = 0; y < uSize.y; ++y) {
    for (int x = 0; x < uSize.x; ++x) {
      vec4 value = texelFetch(reduced, ivec2(x, y), 0);
      sum[0] = max(sum[0], value[0]);
      sum.yzw += value.yzw;
    }
  }
  return vec4(sum[0], sum[1], sum[2] / max(sum[3], 1.0), sum[3]);
}

bool same(float differing) {
  return differing <= ${Q}.0;
}

bool differs(float differing) {
  return differing >= ${ie}.0;
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
  bool believed = run >= ${M}.0;
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
}, le = `#version 300 es
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
  return int(texelFetch(uFieldMetrics, ivec2(${f.phase}, 0), 0)[0]);
}

bool isMixedPhase(int phase) {
  return phase == ${I} || phase == ${se};
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
function C(s, e, t) {
  return Object.fromEntries(
    Object.entries(t).map(([A, i]) => [
      A,
      s.getUniformLocation(e, i)
    ])
  );
}
class fe {
  #e;
  #c;
  #t;
  #o;
  #E;
  #s;
  #a;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #f = null;
  #d = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #i = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #n = null;
  #A = null;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(p * 4);
  #h = 0;
  #v = 0;
  constructor(e) {
    this.#e = e, this.#c = R(
      e,
      oe,
      b
    ), this.#t = C(
      e,
      this.#c,
      re
    ), this.#o = R(
      e,
      he,
      b
    ), this.#E = C(
      e,
      this.#o,
      ne
    ), this.#s = R(
      e,
      ce,
      b
    ), this.#a = C(e, this.#s, ae);
  }
  /** The newest measurements, or null before any frame has been measured. */
  get texture() {
    return this.#i?.[this.#r]?.textures[0] ?? null;
  }
  /** The size of the frames to be measured, which sizes the block grid. */
  resize(e, t) {
    e === this.#h && t === this.#v || (this.#h = e, this.#v = t, this.#x());
  }
  /** Forget every measurement: the next frame starts a cycle from nothing. */
  reset() {
    const e = this.#e;
    e.deleteSync(this.#A), this.#A = null;
    const t = this.#i?.[this.#r];
    if (!t) return;
    const A = new Float32Array(p * 4);
    for (let i = 0; i < f.phase; i++)
      A[i * 4 + 1] = 1;
    e.bindTexture(e.TEXTURE_2D, t.textures[0] ?? null), e.texSubImage2D(
      e.TEXTURE_2D,
      0,
      0,
      0,
      p,
      1,
      e.RGBA,
      e.FLOAT,
      A
    );
  }
  /**
   * Detect the pulldown phase of `cur`, the frame being filtered, against
   * `next`, and start reading it back. Only the two comparisons against the
   * next frame are measured; the rest are earlier ones moved along a frame.
   * `first` is the parity of the field that was captured first.
   */
  detect(e, t, A) {
    const i = this.#e;
    if (this.#h === 0 || this.#v === 0) return;
    this.#M();
    const r = this.#f, n = this.#d, a = this.#i;
    if (r === null || n === null || a === null) return;
    const h = a[this.#r], c = a[1 - this.#r];
    i.bindFramebuffer(i.FRAMEBUFFER, r.framebuffer), i.useProgram(this.#c), this.#T(0, e, this.#t.a), this.#T(1, t, this.#t.b), this.#T(2, h.textures[0], this.#t.fieldMetrics), i.uniform1i(this.#t.first, A), i.uniform2i(this.#t.size, this.#h, this.#v), i.viewport(0, 0, r.width, r.height), i.drawArrays(i.TRIANGLES, 0, 3), i.bindFramebuffer(i.FRAMEBUFFER, n.framebuffer), i.useProgram(this.#o), this.#T(0, r.textures[0], this.#E.second), this.#T(1, r.textures[1], this.#E.first), i.uniform2i(this.#E.size, r.width, r.height), i.viewport(0, 0, n.width, n.height), i.drawArrays(i.TRIANGLES, 0, 3), i.bindFramebuffer(i.FRAMEBUFFER, c.framebuffer), i.useProgram(this.#s), this.#T(0, h.textures[0], this.#a.previous), this.#T(1, n.textures[0], this.#a.second), this.#T(2, n.textures[1], this.#a.first), i.uniform2i(this.#a.size, n.width, n.height), i.viewport(0, 0, p, 1), i.drawArrays(i.TRIANGLES, 0, 3), this.#r = 1 - this.#r, i.deleteSync(this.#A), i.bindBuffer(i.PIXEL_PACK_BUFFER, this.#n), i.readPixels(0, 0, p, 1, i.RGBA, i.FLOAT, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), this.#A = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), i.flush();
  }
  /**
   * Overwrite the phase of the newest frame, for one whose comparisons cannot
   * be trusted but whose place in the cycle is known some other way. The
   * readback is taken again so that it says the same.
   */
  seed(e, t) {
    const A = this.#e, i = this.#i?.[this.#r];
    i && (A.bindTexture(A.TEXTURE_2D, i.textures[0] ?? null), A.texSubImage2D(
      A.TEXTURE_2D,
      0,
      f.phase,
      0,
      1,
      1,
      A.RGBA,
      A.FLOAT,
      new Float32Array([e, t, 0, 0])
    ), A.deleteSync(this.#A), A.bindFramebuffer(A.FRAMEBUFFER, i.framebuffer), A.bindBuffer(A.PIXEL_PACK_BUFFER, this.#n), A.readPixels(0, 0, p, 1, A.RGBA, A.FLOAT, 0), A.bindBuffer(A.PIXEL_PACK_BUFFER, null), A.bindFramebuffer(A.FRAMEBUFFER, null), this.#A = A.fenceSync(A.SYNC_GPU_COMMANDS_COMPLETE, 0), A.flush());
  }
  /**
   * The phase of the last frame measured, once the GPU has handed it back,
   * and null while it is still on its way. It is handed back once.
   */
  poll() {
    const e = this.#e, t = this.#A;
    if (t === null || this.#n === null) return null;
    switch (e.clientWaitSync(t, 0, 0)) {
      case e.ALREADY_SIGNALED:
      case e.CONDITION_SATISFIED:
        return e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#n), e.getBufferSubData(e.PIXEL_PACK_BUFFER, 0, this.metrics), e.bindBuffer(e.PIXEL_PACK_BUFFER, null), e.deleteSync(t), this.#A = null, {
          phase: this.metrics[f.phase * 4] ?? 0,
          run: this.metrics[f.phase * 4 + 1] ?? 0
        };
      default:
        return null;
    }
  }
  destroy() {
    const e = this.#e;
    if (this.#x(), this.#i !== null) {
      for (const t of this.#i) S(e, t);
      this.#i = null;
    }
    e.deleteSync(this.#A), this.#A = null, e.deleteBuffer(this.#n), this.#n = null, e.deleteProgram(this.#c), e.deleteProgram(this.#o), e.deleteProgram(this.#s);
  }
  #T(e, t, A) {
    const i = this.#e;
    i.activeTexture(i.TEXTURE0 + e), i.bindTexture(i.TEXTURE_2D, t ?? null), i.uniform1i(A, e);
  }
  #x() {
    const e = this.#e;
    this.#f !== null && S(e, this.#f), this.#d !== null && S(e, this.#d), this.#f = null, this.#d = null;
  }
  /** Everything detect needs that is not there yet. */
  #M() {
    const e = this.#e;
    if (this.#f === null || this.#d === null) {
      this.#x();
      const t = Math.ceil(this.#h / $), A = Math.ceil(this.#v / (V * 2));
      this.#f = _(e, t, A, 2), this.#d = _(
        e,
        Math.ceil(t / g),
        Math.ceil(A / g),
        2
      );
    }
    this.#i === null && (this.#i = [
      _(e, p, 1, 1),
      _(e, p, 1, 1)
    ], this.#r = 0, this.reset()), this.#n === null && (this.#n = e.createBuffer(), e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#n), e.bufferData(
      e.PIXEL_PACK_BUFFER,
      this.metrics.byteLength,
      e.STREAM_READ
    ), e.bindBuffer(e.PIXEL_PACK_BUFFER, null));
  }
}
function _(s, e, t, A) {
  const i = s.createFramebuffer();
  s.bindFramebuffer(s.FRAMEBUFFER, i);
  const r = [];
  for (let h = 0; h < A; h++) {
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
  const n = s.checkFramebufferStatus(s.FRAMEBUFFER) === s.FRAMEBUFFER_COMPLETE;
  s.bindFramebuffer(s.FRAMEBUFFER, null);
  const a = { framebuffer: i, textures: r, width: e, height: t };
  if (!n)
    throw S(s, a), new Error("failed to allocate framebuffer");
  return a;
}
function S(s, { framebuffer: e, textures: t }) {
  s.deleteFramebuffer(e);
  for (const A of t) s.deleteTexture(A);
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
function de() {
  return typeof HTMLVideoElement < "u" && (j(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const pe = 250, me = 500;
class Ee {
  #e;
  #c;
  #t = null;
  #o = null;
  #E = null;
  #s = null;
  #a = !0;
  #f = null;
  #d = null;
  #i = 0;
  constructor(e) {
    if (this.#e = e, this.#c = j(e) ? e : null, this.#c) {
      for (const t of ["emptied", "seeking", "seeked"])
        e.addEventListener(t, this.#n);
      for (const t of ["pause", "playing", "waiting", "ratechange"])
        e.addEventListener(t, this.#r);
    }
  }
  request(e) {
    this.#t === null && (this.#o = e, this.#t = this.#c ? requestAnimationFrame(this.#h) : this.#e.requestVideoFrameCallback(this.#A));
  }
  cancel() {
    this.#t !== null && (this.#c ? cancelAnimationFrame(this.#t) : this.#e.cancelVideoFrameCallback(this.#t)), this.#t = null, this.#o = null, this.#n();
  }
  destroy() {
    this.cancel();
    for (const e of ["emptied", "seeking", "seeked"])
      this.#e.removeEventListener(e, this.#n);
    for (const e of ["pause", "playing", "waiting", "ratechange"])
      this.#e.removeEventListener(e, this.#r);
  }
  #r = () => {
    this.#f = null, this.#d = null, this.#i = 0;
  };
  #n = () => {
    this.#E = null, this.#s = null, this.#a = !0, this.#r();
  };
  #A = (e, t) => {
    const A = this.#o;
    this.#t = null, this.#o = null, A?.(e, t);
  };
  #h = (e) => {
    const t = this.#c, A = Y.map((a) => t[a]);
    this.#E?.some((a, h) => A[h] < a) && this.#n(), this.#E = A;
    const i = t.mozPaintedFrames, r = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, n = this.#s === null && (i > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (r && (n || this.#s !== null && i !== this.#s)) {
      if (this.#f !== null && e - this.#f > me && (this.#r(), this.#a = !0), !t.paused && !t.ended) {
        const h = this.#d;
        if (h && e - h.at >= pe) {
          const c = i - h.frames, o = (e - h.at) / c;
          c > 0 && o >= 4 && o <= 200 && (this.#i = this.#i ? this.#i + (o - this.#i) * 0.25 : o), this.#d = null;
        }
        this.#d ??= { at: e, frames: i };
      }
      this.#f = e, this.#s = i;
      const a = this.#a;
      this.#a = !1, this.#A(e, {
        width: t.videoWidth,
        height: t.videoHeight,
        // Used only to select source scan/size metadata on the media timeline.
        // It is deliberately NOT used as the frame identity or field clock.
        mediaTime: t.currentTime,
        presentedFrames: i,
        expectedDisplayTime: e,
        mozTiming: { periodMs: this.#i, discontinuity: a }
      });
    } else
      this.#t = requestAnimationFrame(this.#h);
  };
}
const ve = 0.5, m = 4, w = 5, N = 1e3, xe = 4, L = 200, we = 0.25, Te = 0.75, De = 0.1, X = 1, Fe = 1e3 / 60, Re = 0.02, be = 0.1, O = 1, y = 5, ge = 4, _e = 1001 / 3e4, z = 3, ye = 60, Me = 3, Se = {
  2: 0,
  3: 0.25,
  4: 0.5,
  5: 0.75
}, Be = `#version 300 es
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
function Xe() {
  return de() && typeof WebGL2RenderingContext < "u";
}
class Oe {
  canvas;
  #e;
  #c;
  #t;
  /** The pulldown detection, which keeps its measurements on the GPU. */
  #o;
  #E;
  #s;
  /** The program that copies a filtered picture onto the canvas. */
  #a;
  #f;
  #d;
  #i = [];
  /** Somewhere to filter a field into, and to read it back out of. */
  #r = [];
  /** Which output slot was written last; the next one follows round the ring. */
  #n = w - 1;
  /** Filtered fields waiting for their moment, oldest first. */
  #A = [];
  /** The last picture scheduled, shown or not; the next follows on from it. */
  #h = null;
  /** The rAF loop that puts them up, which is all that draws on the canvas. */
  #v = null;
  /** When the loop last ran, and the refresh grid fitted to it. See #measureRefresh. */
  #T = 0;
  #x = Fe;
  #M = 0;
  /** How far ahead of its moment the last picture was shown. See #present. */
  #Te = 0;
  /** Debug only: the rAF the last picture was drawn in, and repeats dropped since. */
  #ae = 0;
  #ce = 0;
  /** The `<div>` this put around the element, so it can be taken away again. */
  #U = null;
  #ue;
  #I;
  #De;
  #S;
  #b;
  /** How long a frame lasts in wall time, from what the frames themselves say. */
  #u = 0;
  /** The size of a frame as it is coded, which is what a texture holds. */
  #F = 0;
  #g = 0;
  /** Where the newest frame is. The two before it follow round the ring. */
  #w = m - 1;
  /** How many of the held frames are consecutive, up to HISTORY. */
  #p = 0;
  #k = 0;
  #R = !1;
  #le = !1;
  #l = null;
  #j = [];
  #G = !1;
  #fe;
  /** Everything the next report is counted from. See DeinterlaceStats. */
  #D = {
    filtered: 0,
    missed: 0,
    degraded: 0,
    discontinuities: 0,
    late: 0,
    resynced: 0,
    queueResetted: 0
  };
  /** `presentedFrames` of the last frame the callback saw; 0 before any. */
  #Ae = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #de = 0;
  #ie = 0;
  #B = 0;
  #K = 0;
  #P = 0;
  #J = 0;
  #N = 0;
  #X;
  #se = [];
  #O = [];
  #q = 0;
  #z = 0;
  #ee = 0;
  #H = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #W = D;
  #C = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #m = D;
  #L = !1;
  /**
   * Where the cycle was when a frame was last seen locked to it. A broadcast
   * can cut between soft pulldown, which is progressive pictures with repeat
   * flags, and the same film laid down as interlaced frames, every few seconds
   * and without losing its place in the cycle, so this outlives those cuts.
   */
  #_ = null;
  /** How many of the coming frames take their phase from `#cadence`. */
  #Z = 0;
  #Q = null;
  #Fe = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #re = [0, 0, 0, 0, 0, 0];
  #pe = 0;
  constructor(e, t = {}) {
    this.#e = e, this.#I = t.doubleRate ?? !1, this.#De = t.spatialCheck ?? !0, this.#S = t.debug ?? !1, this.#b = t.film ?? !1, this.#fe = t.onStats, this.canvas = document.createElement("canvas"), this.canvas.style.cssText = "position:absolute;pointer-events:none;visibility:hidden";
    const A = this.canvas.getContext("webgl2", {
      alpha: !1,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!A) throw new Error("this browser has no WebGL2");
    this.#t = A, this.#o = new fe(A), this.#E = R(A, le, b);
    const i = this.#E;
    this.#s = Object.fromEntries(
      Object.entries(ue).map(([r, n]) => [
        r,
        A.getUniformLocation(i, n)
      ])
    ), this.#a = R(A, Be, b), this.#f = A.getUniformLocation(this.#a, "uField"), this.#d = A.getUniformLocation(this.#a, "uFlip"), this.#b && this.#be(), this.#X = A.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.canvas.addEventListener("webglcontextlost", this.#Xe), this.#ue = new ResizeObserver(() => this.#he()), this.#c = new Ee(e), e.addEventListener("emptied", this.#Ge), e.addEventListener("resize", this.#ke), e.addEventListener("pause", this.#y), e.addEventListener("ended", this.#y), e.addEventListener("seeked", this.#y), e.addEventListener("ratechange", this.#y);
  }
  get running() {
    return this.#R && (this.#l?.interlaced ?? !0);
  }
  /** Whether the caller wants filtering, independently of the current source. */
  get enabled() {
    return this.#le;
  }
  set enabled(e) {
    this.#le = e, this.#me();
  }
  /** Update whether the source needs filtering and which field comes first. */
  set scan(e) {
    this.#l = e, this.#me();
  }
  get scan() {
    return this.#l;
  }
  set videoTimeline(e) {
    this.#j = e, e.length === 0 && (this.#l = null), this.#me();
  }
  get videoTimeline() {
    return this.#j;
  }
  /**
   * What to put on the screen for fullscreen: the `<div>` holding both the
   * element and the canvas once there is one, and the element itself before
   * that. Fullscreening the element alone would leave the canvas behind in
   * the page, and with it the only deinterlaced picture there is.
   */
  get container() {
    return this.#U ?? this.#e;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#I;
  }
  set doubleRate(e) {
    e !== this.#I && (this.#I = e, this.#Re());
  }
  get film() {
    return this.#b;
  }
  set film(e) {
    e !== this.#b && (e && this.#be(), this.#b = e, e || (this.#L = !1, this.#m = D, this.#V(), this.#Y()), this.#Re());
  }
  get debug() {
    return this.#S;
  }
  set debug(e) {
    this.#S = e;
  }
  /** Whether pictures are queued and put up by the loop rather than drawn on arrival. */
  get #$() {
    return this.#I || this.#b;
  }
  #Re() {
    this.#$ ? (this.#F > 0 && this.#Ie(), this.#ve()) : (this.#xe(), this.#te());
  }
  #be() {
    if (this.#t.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  #me() {
    this.#le && (this.#j.length > 0 || (this.#l?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  start() {
    this.#R || this.#G || (this.#R = !0, this.#Ne(), this.#tt(), this.#oe(), (this.#l?.interlaced ?? !0) && this.#ve());
  }
  /** Take the deinterlaced picture away, leaving the element's own showing. */
  stop() {
    this.#R && (this.#R = !1, this.#c.cancel(), this.#xe(), this.#p = 0, this.canvas.style.visibility = "hidden");
  }
  destroy() {
    this.stop(), this.#c.destroy(), this.canvas.removeEventListener("webglcontextlost", this.#Xe), this.#e.removeEventListener("emptied", this.#Ge), this.#e.removeEventListener("resize", this.#ke), this.#e.removeEventListener("pause", this.#y), this.#e.removeEventListener("ended", this.#y), this.#e.removeEventListener("seeked", this.#y), this.#e.removeEventListener("ratechange", this.#y), this.#At();
    for (const e of this.#i) this.#t.deleteTexture(e);
    this.#i = [], this.#te();
    for (const e of [
      ...this.#se,
      ...this.#O.map(({ q: t }) => t)
    ])
      this.#t.deleteQuery(e);
    this.#se.length = 0, this.#O.length = 0, this.#o.destroy(), this.#Q !== null && (te(this.#Q), this.#Q = null), this.#t.deleteProgram(this.#E), this.#t.deleteProgram(this.#a), this.#t.getExtension("WEBGL_lose_context")?.loseContext();
  }
  #oe() {
    this.#R && this.#c.request(this.#Qe);
  }
  #Oe(e, t, A) {
    this.#Q == null && (this.#Q = q(this.#t, "20px monospace")), Ae(this.#Q, e, t, A, this.#F, this.#g, 20);
  }
  #ge(e) {
    if (this.#X == null || this.#O.length > 30)
      return;
    const t = this.#se.pop() ?? this.#t.createQuery();
    return this.#t.beginQuery(this.#X.TIME_ELAPSED_EXT, t), this.#O.push({ q: t, isField: e }), t;
  }
  #_e(e) {
    this.#X != null && (e != null && this.#t.endQuery(this.#X.TIME_ELAPSED_EXT), this.#O = this.#O.filter(
      ({ q: t, isField: A }) => {
        if (this.#t.getQueryParameter(t, this.#t.QUERY_RESULT_AVAILABLE)) {
          const i = this.#t.getQueryParameter(t, this.#t.QUERY_RESULT);
          return A ? (this.#ee += i, this.#H++) : (this.#q += i, this.#z++), this.#se.push(t), !1;
        }
        return !0;
      }
    ));
  }
  #V() {
    this.#_ = null, this.#Z = 0;
  }
  /**
   * Give the frame being filtered its phase from the cycle seen before the
   * progressive pictures, if it is one of those that needs it.
   */
  #ze(e) {
    if (this.#Z === 0 || this.#_ === null) return;
    const t = e - this.#_.mediaTime;
    if (t < 0 || t > ye) {
      this.#V();
      return;
    }
    const A = this.#Z === z ? Me : (this.#_.phase - 1 + Math.round(t / _e)) % y + 1;
    this.#W = this.#m = { phase: A, run: M }, this.#C = 0, this.#o.seed(A, M), this.#Z--;
  }
  #Y() {
    this.#m = D, this.#W = D, this.#C = 0, this.#L = !1, this.#o.reset();
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #He() {
    const { cur: e, next: t } = this.#Le(!1), A = this.#i[e], i = this.#i[t];
    if (!A || !i) return;
    const r = this.#l?.topFieldFirst !== !1 ? 0 : 1;
    this.#o.detect(A, i, r);
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #We() {
    const e = this.#o.poll();
    e !== null && (this.#W = e, this.#C = 0), this.#C++;
    const { phase: t, run: A } = this.#W;
    t === 0 || this.#C > y ? this.#m = D : this.#m = {
      phase: (t - 1 + this.#C) % y + 1,
      run: A
    };
  }
  #Ze(e) {
    const t = [], A = this.#o.metrics;
    for (let r = 0; r < f.phase; r++) {
      const n = A[r * 4] ?? 0, a = A[r * 4 + 1] ?? 0, h = A[r * 4 + 2] ?? 0;
      t.push(
        `${n.toFixed(3)},${a.toString().padStart(4)},${h.toFixed(3)}`
      );
    }
    const i = this.#re.map((r, n) => `${n === 0 ? "-" : n}:${r}`).join(" ");
    return `frame=${e} phase=${this.#m.phase} run=${this.#m.run} known=${this.#W.phase}/${this.#W.run} age=${this.#C} ${this.#L ? "film" : "video"} period=${this.#u.toFixed(3)}
${t.join(" ")}
${i} dropped:${this.#pe}`;
  }
  /** Whether this frame is not the one that follows the last one seen. */
  #ye(e) {
    if (e.mozTiming) return e.mozTiming.discontinuity;
    const t = e.mediaTime - this.#k;
    return t < 0 || t > ve;
  }
  #Qe = (e, t) => {
    if (!(!this.#R || this.#G)) {
      if (this.#$e(t.mediaTime), t.width > 0 && t.height > 0) {
        if ((this.#F === 0 || this.#g === 0) && this.#Ue(t.width, t.height), this.#l && !this.#l.interlaced) {
          const c = this.#ye(t);
          c && this.#V(), this.#Pe(t.presentedFrames, c), this.#k = t.mediaTime, this.#qe(), this.#oe();
          return;
        }
        const A = t.mediaTime - this.#k, i = this.#ye(t);
        i && (this.#V(), this.#p = 0, this.#D.discontinuities++, this.#A.length = 0, this.#h = null, this.#Y());
        const r = this.#Pe(t.presentedFrames, i);
        if (!t.mozTiming && this.#p > 0 && t.mediaTime === this.#k) {
          this.#oe();
          return;
        }
        t.mozTiming ? this.#u = t.mozTiming.periodMs : !i && A > 0 && this.#Ve(A), this.#k = t.mediaTime;
        const n = performance.now();
        n - this.#de > N && (this.#ie = n, this.#B = 0, this.#K = 0, this.#P = 0, this.#J = 0, this.#N = 0, this.#q = 0, this.#z = 0, this.#ee = 0, this.#H = 0), this.#de = n;
        const a = performance.now(), h = this.#ge(!1);
        if (this.#Ce(), this.#N = Math.max(
          this.#N,
          this.#A.length
        ), this.#b && (this.#p === m && r === 0 ? (this.#We(), this.#He(), this.#ze(t.mediaTime)) : this.#Y(), this.#re[this.#m.phase] = (this.#re[this.#m.phase] ?? 0) + 1, this.#L = this.#m.phase !== 0 && this.#m.run >= M, this.#L && this.#Z === 0 && (this.#_ = {
          mediaTime: t.mediaTime,
          phase: this.#m.phase
        }), this.#S && (this.#Fe = this.#Ze(t.presentedFrames))), this.#Me() && !this.#e.paused && !this.#e.ended) {
          this.#A.length >= w && (this.#A.length = 0, this.#h = null, this.#D.queueResetted++);
          const c = (t.expectedDisplayTime || e) + this.#x;
          if (this.#L) {
            const o = this.#m.phase;
            if (o === I)
              this.#pe++, this.#ce++;
            else {
              const l = this.#u * y / ge, d = Se[o] ?? 0, u = this.#Ee(
                "film",
                c + d * this.#u,
                l
              );
              this.#ne("film", o, !1, u, l);
            }
          } else if (this.#I) {
            const o = this.#u / 2, l = this.#Ee("field", c, o);
            this.#ne("field", 1, !1, l, o), this.#ne("field", 2, !0, l + o, o);
          } else {
            const o = this.#u, l = this.#Ee("frame", c, o);
            this.#ne("frame", 0, !1, l, o);
          }
        } else
          this.#we(this.#e.paused || this.#e.ended, !1, null);
        this.#_e(h), this.#K += performance.now() - a, this.#B++, this.#et(n);
      }
      this.#oe();
    }
  };
  #$e(e) {
    let t;
    for (let r = this.#j.length - 1; r >= 0; r--) {
      const n = this.#j[r];
      if (n.start <= e + 1e-6) {
        t = n;
        break;
      }
    }
    t?.codedSize && (t.codedSize.width !== this.#F || t.codedSize.height !== this.#g) && this.#Ue(t.codedSize.width, t.codedSize.height);
    const A = t?.scan;
    if (!A || this.#l?.interlaced === A.interlaced && this.#l.topFieldFirst === A.topFieldFirst)
      return;
    const i = this.#l?.interlaced === !1;
    this.#l = A, this.#_ === null && (this.#p = 0), this.#A.length = 0, this.#h = null, this.#Y(), A.interlaced && i && this.#_ !== null && (this.#Z = z), A.interlaced ? this.#$ && this.#ve() : this.#xe();
  }
  /**
   * Whether pictures are being filtered ahead of time and queued, rather than
   * drawn as their frame arrives.
   *
   * A picture for every frame has nothing to schedule -- there is one of them
   * and it goes up now -- and neither has a filter that has yet to see two
   * frames go by, since until then there is no idea how long a frame lasts.
   */
  #Me() {
    return this.#$ && this.#u > 0 && this.#r.length === w;
  }
  /**
   * When a picture goes up: one duration after the last one of its cadence,
   * nudged towards `ideal` by a fraction of the gap so that the schedule
   * follows the clock without a picture ever moving across a refresh. It
   * restarts from `ideal` when the gap has grown to a whole picture or the
   * cadence has changed.
   */
  #Ee(e, t, A) {
    const i = this.#h;
    if (i !== null && i.cadence === e) {
      const r = i.at + i.duration, n = t - r;
      if (Math.abs(n) < A) {
        const a = Math.max(
          -X,
          Math.min(X, n * De)
        );
        return r + a;
      }
    }
    i !== null && this.#D.resynced++;
    for (let r = this.#A.at(-1); r && r.at >= t; )
      this.#A.pop(), this.#D.late++, r = this.#A.at(-1);
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
  #Ve(e) {
    const t = e * 1e3 / (this.#e.playbackRate || 1), A = this.#u > 0 ? Math.max(1, Math.round(t / this.#u)) : 1, i = t / A;
    i < xe || i > L || (this.#u = this.#u > 0 && i > this.#u * Te ? this.#u + (i - this.#u) * we : i);
  }
  /**
   * Filter one field into an output texture and put it in the queue.
   *
   * The three frames the filter reads are only the right three between one
   * frame arriving and the next, so both fields of a frame are built here and
   * held as pictures. What is queued after that is a copy waiting for a
   * moment, which no later frame can take away.
   */
  #ne(e, t, A, i, r) {
    const n = (this.#n + 1) % w, a = this.#r[n];
    if (!a) return;
    for (this.#n = n; this.#A.length > 0 && this.#A[0]?.slot === n; )
      this.#A.shift(), this.#D.late++;
    this.#we(!1, A, a.framebuffer);
    const h = {
      slot: n,
      at: i,
      duration: r,
      cadence: e,
      phase: t,
      droppedBefore: this.#ce
    };
    this.#ce = 0, this.#A.push(h), this.#h = h;
  }
  /** The loop that puts filtered fields up, and the only thing that draws. */
  #ve() {
    this.#v === null && (!this.#R || this.#G || !this.#$ || (this.#v = requestAnimationFrame(this.#Se)));
  }
  #xe() {
    this.#v !== null && cancelAnimationFrame(this.#v), this.#v = null, this.#A.length = 0, this.#h = null;
  }
  #Se = (e) => {
    this.#v = null, !(!this.#R || this.#G || !this.#$) && (this.#Ye(e), this.#je(this.#M, e), this.#v = requestAnimationFrame(this.#Se));
  };
  /**
   * Fit a grid of refreshes (period and phase) to the animation frames. rAF
   * timestamps wander by a millisecond or so, which is more than the
   * nearest-refresh decision in #present can take; the grid is what it
   * compares against. A frame far off the grid restarts it.
   */
  #Ye(e) {
    const t = e - this.#T;
    this.#T = e;
    const A = Math.max(1, Math.round(t / this.#x)), i = this.#M + A * this.#x, r = e - i;
    if (this.#M === 0 || t <= 0 || t > L || Math.abs(r) > this.#x / 4) {
      t > 0 && t <= L && (this.#x = t), this.#M = e;
      return;
    }
    this.#x += r / A * Re, this.#M = i + r * be;
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
  #je(e, t) {
    const A = this.#x / 2, i = (h) => {
      const c = h.at - e;
      return c <= A - O ? !0 : c > A + O ? !1 : this.#Te > 0;
    };
    for (; this.#A[1] && i(this.#A[1]); )
      this.#D.late++, this.#A.shift();
    const r = this.#A[0];
    if (!r || !i(r)) return;
    this.#A.shift(), this.#Te = r.at - e;
    const n = performance.now(), a = this.#ge(!0);
    this.#Je(r.slot), this.#_e(a), this.#J += performance.now() - n, this.#P++, this.#S && this.#Ke(r, t), this.#ae = t;
  }
  #Ke(e, t) {
    const A = this.#ae === 0 ? 0 : t - this.#ae, i = A / this.#x, r = e.cadence === "film" ? `phase ${e.phase}` : e.cadence === "field" ? `field ${e.phase}` : "frame", n = e.droppedBefore > 0 ? `, phase ${I} dropped before it` + (e.droppedBefore > 1 ? ` (${e.droppedBefore})` : "") : "";
    console.log(
      `yadif: +${A.toFixed(2)} ms (${i.toFixed(2)} refreshes) ${e.cadence} ${r}, due ${(e.at - t).toFixed(2)} ms${n}`
    );
  }
  /** Copy one of the filtered pictures onto the canvas. */
  #Je(e) {
    const t = this.#r[e];
    t && this.#Be(t.texture);
  }
  /** Put a progressive frame through unchanged, keeping one display surface. */
  #qe() {
    this.#Ce();
    const e = this.#i[this.#w];
    e && this.#Be(e, !0), this.#_ === null && (this.#p = 0);
  }
  #Be(e, t = !1) {
    const A = this.#t;
    A.bindFramebuffer(A.FRAMEBUFFER, null), A.useProgram(this.#a), A.activeTexture(A.TEXTURE0), A.bindTexture(A.TEXTURE_2D, e), A.uniform1i(this.#f, 0), A.uniform1i(this.#d, t ? 1 : 0), A.viewport(0, 0, this.#F, this.#g), A.drawArrays(A.TRIANGLES, 0, 3), this.canvas.style.visibility = "visible";
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
  #Pe(e, t) {
    let A = 0;
    return this.#Ae !== 0 && !t && (A = Math.max(0, e - this.#Ae - 1), this.#D.missed += A), this.#Ae = e, A;
  }
  #et(e) {
    if (!this.#fe) return;
    const t = e - this.#ie;
    if (t < N) return;
    const A = this.#Me() ? this.#P : this.#B;
    let i = 0;
    this.#B !== 0 && (i += this.#K / this.#B), this.#P !== 0 && (i += this.#J / this.#P / 2);
    let r;
    this.#X != null && (r = 0, this.#z !== 0 && (r += this.#q / 1e6 / this.#z), this.#H !== 0 && (r += this.#ee / 1e6 / this.#H / 2)), this.#fe({
      ...this.#D,
      // The element's own count of what its decoder could not keep up with,
      // which is the machine being behind rather than this filter.
      dropped: this.#e.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
      fps: A * 1e3 / t,
      frameMs: i,
      maxQueuedFields: this.#N,
      gpuMs: r,
      film: this.#L
    }), this.#ie = e, this.#B = 0, this.#K = 0, this.#P = 0, this.#J = 0, this.#N = 0, this.#q = 0, this.#z = 0, this.#ee = 0, this.#H = 0;
  }
  /** Take the newest frame into the ring. */
  #Ce() {
    const e = this.#t;
    this.#w = (this.#w + 1) % m, e.bindTexture(e.TEXTURE_2D, this.#i[this.#w] ?? null), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      e.RGBA,
      e.UNSIGNED_BYTE,
      this.#e
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
  #we(e, t, A) {
    if (this.#p === 0 || this.#G) return;
    this.#p === m && !e ? this.#D.filtered++ : this.#D.degraded++;
    const i = this.#t, { prev: r, cur: n, next: a } = this.#Le(e);
    i.bindFramebuffer(i.FRAMEBUFFER, A), i.useProgram(this.#E);
    for (const [l, d] of [r, n, a].entries())
      i.activeTexture(i.TEXTURE0 + l), i.bindTexture(i.TEXTURE_2D, this.#i[d] ?? null);
    i.uniform1i(this.#s.prev, 0), i.uniform1i(this.#s.cur, 1), i.uniform1i(this.#s.next, 2);
    const h = this.#b ? this.#o.texture : null, c = h !== null;
    h !== null && (i.activeTexture(i.TEXTURE0 + 3), i.bindTexture(i.TEXTURE_2D, h), i.uniform1i(this.#s.fieldMetrics, 3)), i.uniform2i(this.#s.size, this.#F, this.#g);
    const o = this.#l?.topFieldFirst !== !1 ? 0 : 1;
    i.uniform1i(this.#s.parity, t ? 1 - o : o), i.uniform1i(this.#s.second, t ? 1 : 0), i.uniform1i(
      this.#s.tff,
      this.#l?.topFieldFirst !== !1 ? 1 : 0
    ), i.uniform1i(this.#s.spatialCheck, this.#De ? 1 : 0), i.uniform1i(this.#s.debug, this.#S ? 1 : 0), i.uniform1i(this.#s.film, c ? 1 : 0), i.uniform1i(this.#s.phase, this.#m.phase), i.viewport(0, 0, this.#F, this.#g), i.drawArrays(i.TRIANGLES, 0, 3), this.#S && c && this.#Oe(this.#Fe, 0, 90), A === null && (this.canvas.style.visibility = "visible");
  }
  #Le(e) {
    const t = (A) => (this.#w + m - A) % m;
    return this.#p === 1 ? { prev: this.#w, cur: this.#w, next: this.#w } : e ? { prev: t(1), cur: this.#w, next: this.#w } : this.#p === 2 ? { prev: t(1), cur: t(1), next: this.#w } : { prev: t(2), cur: t(1), next: this.#w };
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
  #he() {
    if (!this.#U) return;
    const e = this.#e, t = e.videoWidth, A = e.videoHeight;
    if (t === 0 || A === 0) return;
    const i = Math.min(
      e.offsetWidth / t,
      e.offsetHeight / A
    ), r = t * i, n = A * i;
    this.canvas.style.left = `${e.offsetLeft + (e.offsetWidth - r) / 2}px`, this.canvas.style.top = `${e.offsetTop + (e.offsetHeight - n) / 2}px`, this.canvas.style.width = `${r}px`, this.canvas.style.height = `${n}px`;
  }
  #Ue(e, t) {
    const A = this.#t;
    this.canvas.width = e, this.canvas.height = t, this.#F = e, this.#g = t, this.#p = 0, this.#he();
    for (const i of this.#i) A.deleteTexture(i);
    this.#i = [];
    for (let i = 0; i < m; i++) {
      const r = A.createTexture();
      A.bindTexture(A.TEXTURE_2D, r), A.texParameteri(A.TEXTURE_2D, A.TEXTURE_MIN_FILTER, A.NEAREST), A.texParameteri(A.TEXTURE_2D, A.TEXTURE_MAG_FILTER, A.NEAREST), A.texParameteri(A.TEXTURE_2D, A.TEXTURE_WRAP_S, A.CLAMP_TO_EDGE), A.texParameteri(A.TEXTURE_2D, A.TEXTURE_WRAP_T, A.CLAMP_TO_EDGE), A.texImage2D(
        A.TEXTURE_2D,
        0,
        A.RGBA,
        e,
        t,
        0,
        A.RGBA,
        A.UNSIGNED_BYTE,
        null
      ), this.#i.push(r);
    }
    this.#te(), this.#o.resize(e, t), this.#$ && this.#Ie();
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
  #Ie() {
    const e = this.#t;
    if (!(this.#r.length === w || this.#F === 0)) {
      this.#te();
      for (let t = 0; t < w; t++) {
        const A = e.createTexture();
        e.bindTexture(e.TEXTURE_2D, A), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
          e.TEXTURE_2D,
          0,
          e.RGBA,
          this.#F,
          this.#g,
          0,
          e.RGBA,
          e.UNSIGNED_BYTE,
          null
        );
        const i = e.createFramebuffer();
        e.bindFramebuffer(e.FRAMEBUFFER, i), e.framebufferTexture2D(
          e.FRAMEBUFFER,
          e.COLOR_ATTACHMENT0,
          e.TEXTURE_2D,
          A,
          0
        );
        const r = e.checkFramebufferStatus(e.FRAMEBUFFER) === e.FRAMEBUFFER_COMPLETE;
        if (e.bindFramebuffer(e.FRAMEBUFFER, null), !r) {
          e.deleteFramebuffer(i), e.deleteTexture(A), this.#te();
          return;
        }
        this.#r.push({ texture: A, framebuffer: i });
      }
      this.#n = w - 1;
    }
  }
  #te() {
    const e = this.#t;
    for (const { texture: t, framebuffer: A } of this.#r)
      e.deleteFramebuffer(A), e.deleteTexture(t);
    this.#r = [], this.#A.length = 0, this.#h = null;
  }
  /**
   * Wrap the element in a `<div>` of this one's own and put the canvas over
   * it. The wrapper is what the canvas is positioned against; moving the
   * element out of the tree and back within the one task leaves playback
   * alone, which is what makes turning this on mid-stream free.
   */
  #tt() {
    if (this.#U) return;
    const e = this.#e.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#e), t.appendChild(this.#e), t.appendChild(this.canvas), this.#U = t, this.#ue.observe(this.#e), this.#he();
  }
  #At() {
    const e = this.#U;
    this.#U = null, this.#ue.disconnect(), this.canvas.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#e, e), e.remove());
  }
  #ke = () => this.#he();
  #Ge = () => {
    this.#p = 0, this.#k = 0, this.#A.length = 0, this.#h = null, this.#V(), this.#Y(), this.#u = 0, this.#Ne(), this.canvas.style.visibility = "hidden";
  };
  #Ne() {
    this.#D = {
      filtered: 0,
      missed: 0,
      degraded: 0,
      discontinuities: 0,
      late: 0,
      resynced: 0,
      queueResetted: 0
    }, this.#re.fill(0), this.#pe = 0, this.#Ae = 0, this.#ie = 0, this.#de = 0, this.#B = 0, this.#K = 0, this.#P = 0, this.#J = 0, this.#N = 0, this.#q = 0, this.#z = 0, this.#ee = 0, this.#H = 0;
  }
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #y = () => {
    this.#V(), this.#A.length = 0, this.#h = null, this.#Y(), this.#R && this.#we(!0, !1, null);
  };
  /**
   * A lost context takes the textures and the program with it. Rebuilding
   * them is possible, but a page that has lost its context has bigger
   * problems; getting out of the way leaves the element's own picture showing.
   */
  #Xe = (e) => {
    e.preventDefault(), this.#G = !0, this.stop();
  };
}
const H = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", Pe = 0.5, Ce = 3e3, W = 0.1, F = 16, Z = 'video/mp4; codecs="avc1.640029"';
let k = null;
function Le(s = {}) {
  return k ??= Ue(s), k;
}
async function ze(s = {}) {
  return (await Le(s)).deinterlaces;
}
function He() {
  k = null;
}
async function Ue(s) {
  const e = s.tolerance ?? Pe, t = s.timeoutMs ?? Ce, A = performance.now(), i = (a) => ({
    deinterlaces: !1,
    survives: null,
    tookMs: performance.now() - A,
    error: a instanceof Error ? a.message : String(a)
  });
  if (typeof document > "u")
    return i(new Error("there is no document to decode in"));
  const r = document.createElement("video");
  r.muted = !0, r.defaultMuted = !0, r.playsInline = !0, r.preload = "auto";
  let n = null;
  try {
    n = ke(r, t);
    const a = P(B(r, "loadeddata"), t), h = r.play().then(
      () => !0,
      () => !1
    );
    if (await n.ready, await a, await Ge(r, t, await h), r.videoWidth === 0 || r.videoHeight === 0)
      return i(new Error("the probe clip decoded to nothing"));
    const c = Ne(r);
    return {
      deinterlaces: c < 1 - e,
      survives: c,
      tookMs: performance.now() - A
    };
  } catch (a) {
    return i(a);
  } finally {
    r.pause(), r.removeAttribute("src"), r.replaceChildren(), r.load(), n && URL.revokeObjectURL(n.url);
  }
}
const U = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, Ie = typeof MediaSource > "u";
function ke(s, e) {
  if (!U || !U.isTypeSupported(Z))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = H.indexOf(","), A = atob(H.slice(t + 1)), i = new Uint8Array(A.length);
  for (let h = 0; h < A.length; h++) i[h] = A.charCodeAt(h);
  const r = new U(), n = URL.createObjectURL(r);
  if (Ie) {
    s.disableRemotePlayback = !0;
    const h = document.createElement("source");
    h.type = "video/mp4", h.src = n, s.append(h), s.load();
  } else
    s.src = n;
  const a = (async () => {
    await P(B(r, "sourceopen"), e);
    const h = r.addSourceBuffer(Z), c = P(B(h, "updateend"), e);
    h.appendBuffer(i), await c, r.endOfStream();
  })();
  return { url: n, ready: a };
}
async function Ge(s, e, t) {
  if (t) {
    const A = performance.now();
    for (; s.currentTime < W && performance.now() - A < e; )
      await new Promise((i) => requestAnimationFrame(i));
    s.pause();
  } else
    s.currentTime = W, await P(B(s, "seeked"), e);
}
function Ne(s) {
  const e = s.videoHeight, t = document.createElement("canvas");
  t.width = F, t.height = e;
  const A = t.getContext("2d", { willReadFrequently: !0 });
  if (!A) throw new Error("there is no 2d context to read the clip with");
  A.imageSmoothingEnabled = !1, A.drawImage(s, 0, 0, F, e);
  const i = A.getImageData(0, 0, F, e).data, r = (o) => {
    let l = 0;
    for (let d = 0; d < F; d++)
      l += i[(o * F + d) * 4 + 1] ?? 0;
    return l / F;
  };
  let n = 0;
  const a = 2, h = e - 3;
  let c = r(a);
  for (let o = a + 1; o <= h; o++) {
    const l = r(o);
    n += Math.abs(l - c), c = l;
  }
  return n / (h - a) / 255;
}
function B(s, e) {
  return new Promise((t, A) => {
    s.addEventListener(e, () => t(), { once: !0 }), s.addEventListener(
      "error",
      () => {
        const i = s instanceof HTMLMediaElement ? s.error : null, r = i ? ` (MediaError ${i.code}${i.message ? `: ${i.message}` : ""})` : "";
        A(new Error(`the probe clip ${e} failed${r}`));
      },
      { once: !0 }
    );
  });
}
function P(s, e) {
  return Promise.race([
    s,
    new Promise(
      (t, A) => setTimeout(
        () => A(new Error("the probe clip took too long")),
        e
      )
    )
  ]);
}
export {
  Oe as Deinterlacer,
  le as YADIF_FRAGMENT_SHADER,
  ue as YADIF_UNIFORMS,
  ze as decoderDeinterlaces,
  He as forgetDecoderProbe,
  Le as probeDecoder,
  Xe as supportsDeinterlace
};
