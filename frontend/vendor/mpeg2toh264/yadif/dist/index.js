const R = `#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;
function b(s, e, t) {
  const A = s.createProgram(), i = k(s, s.VERTEX_SHADER, t), r = k(s, s.FRAGMENT_SHADER, e);
  if (s.attachShader(A, i), s.attachShader(A, r), s.linkProgram(A), s.deleteShader(i), s.deleteShader(r), !s.getProgramParameter(A, s.LINK_STATUS)) {
    const n = s.getProgramInfoLog(A);
    throw s.deleteProgram(A), new Error(
      `the deinterlacer failed to link: ${n ?? "no reason given"}`
    );
  }
  return A;
}
function k(s, e, t) {
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
const j = `#version 300 es
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
function K(s, e) {
  const t = b(s, j, J), A = s.getAttribLocation(t, "aVertexPosition"), i = s.getAttribLocation(t, "aTextureCoord"), r = s.getUniformLocation(t, "uTexture"), n = s.getUniformLocation(t, "uMatrix"), a = s.getUniformLocation(t, "uUvMatrix"), h = s.getUniformLocation(t, "uTextColor"), c = s.getUniformLocation(t, "uBackColor");
  if (r == null || n == null || a == null || h == null || c == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const o = s.createBuffer(), l = s.createBuffer();
  return {
    gl: s,
    ...q(s, e),
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
function q(s, e) {
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
function ee(s) {
  const e = s.gl;
  e.deleteBuffer(s.positionBuffer), e.deleteBuffer(s.textureBuffer), e.deleteTexture(s.fontTexture), e.deleteProgram(s.program);
}
function te(s, e, t, A, i, r, n) {
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
    const m = Math.floor(t - u.metrics.actualBoundingBoxLeft), E = Math.floor(A - u.metrics.actualBoundingBoxAscent), T = m + u.width, v = E + u.height;
    a.push(m, E), h.push(u.x, u.y), a.push(m, v), h.push(u.x, u.y + u.height), a.push(m + u.width, v), h.push(u.x + u.width, u.y + u.height), a.push(T, v), h.push(u.x + u.width, u.y + u.height), a.push(m, E), h.push(u.x, u.y), a.push(T, E), h.push(u.x + u.width, u.y), t += u.metrics.width;
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
  function l(d, u, m) {
    const E = [];
    for (let T = 0; T < u; T++)
      for (let v = 0; v < d; v++)
        E.push(m[v * d + T]);
    return E;
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
}, x = 7, W = 2, Ae = 16, I = 1, ie = 2, Z = 5, se = {
  a: "uA",
  b: "uB",
  fieldMetrics: "uFieldMetrics",
  first: "uFirst",
  size: "uSize"
}, Q = 16, $ = 8, re = `#version 300 es

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
  const int BLOCK_W = ${Q};
  const int BLOCK_H = ${$};

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
`, oe = {
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, g = 8, ne = `#version 300 es
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
`, he = {
  previous: "uPrevious",
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, ae = `#version 300 es
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
  return differing <= ${W}.0;
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
  bool believed = run >= ${Z}.0;
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
`, ce = {
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
}, ue = `#version 300 es
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
  return texelFetch(uFieldMetrics, ivec2(metric, 0), 0)[1] <= ${W}.0;
}

/** The pulldown phase the detection gave this frame, or 0. See film-shader.ts. */
int detectedPhase() {
  return int(texelFetch(uFieldMetrics, ivec2(${f.phase}, 0), 0)[0]);
}

bool isMixedPhase(int phase) {
  return phase == ${I} || phase == ${ie};
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
function S(s, e, t) {
  return Object.fromEntries(
    Object.entries(t).map(([A, i]) => [
      A,
      s.getUniformLocation(e, i)
    ])
  );
}
class le {
  #t;
  #c;
  #e;
  #o;
  #m;
  #s;
  #a;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #l = null;
  #f = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #i = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #h = null;
  #A = null;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(x * 4);
  #n = 0;
  #E = 0;
  constructor(e) {
    this.#t = e, this.#c = b(
      e,
      re,
      R
    ), this.#e = S(
      e,
      this.#c,
      se
    ), this.#o = b(
      e,
      ne,
      R
    ), this.#m = S(
      e,
      this.#o,
      oe
    ), this.#s = b(
      e,
      ae,
      R
    ), this.#a = S(e, this.#s, he);
  }
  /** The newest measurements, or null before any frame has been measured. */
  get texture() {
    return this.#i?.[this.#r]?.textures[0] ?? null;
  }
  /** The size of the frames to be measured, which sizes the block grid. */
  resize(e, t) {
    e === this.#n && t === this.#E || (this.#n = e, this.#E = t, this.#v());
  }
  /** Forget every measurement: the next frame starts a cycle from nothing. */
  reset() {
    const e = this.#t;
    e.deleteSync(this.#A), this.#A = null;
    const t = this.#i?.[this.#r];
    if (!t) return;
    const A = new Float32Array(x * 4);
    for (let i = 0; i < f.phase; i++)
      A[i * 4 + 1] = 1;
    e.bindTexture(e.TEXTURE_2D, t.textures[0] ?? null), e.texSubImage2D(
      e.TEXTURE_2D,
      0,
      0,
      0,
      x,
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
    const i = this.#t;
    if (this.#n === 0 || this.#E === 0) return;
    this.#y();
    const r = this.#l, n = this.#f, a = this.#i;
    if (r === null || n === null || a === null) return;
    const h = a[this.#r], c = a[1 - this.#r];
    i.bindFramebuffer(i.FRAMEBUFFER, r.framebuffer), i.useProgram(this.#c), this.#T(0, e, this.#e.a), this.#T(1, t, this.#e.b), this.#T(2, h.textures[0], this.#e.fieldMetrics), i.uniform1i(this.#e.first, A), i.uniform2i(this.#e.size, this.#n, this.#E), i.viewport(0, 0, r.width, r.height), i.drawArrays(i.TRIANGLES, 0, 3), i.bindFramebuffer(i.FRAMEBUFFER, n.framebuffer), i.useProgram(this.#o), this.#T(0, r.textures[0], this.#m.second), this.#T(1, r.textures[1], this.#m.first), i.uniform2i(this.#m.size, r.width, r.height), i.viewport(0, 0, n.width, n.height), i.drawArrays(i.TRIANGLES, 0, 3), i.bindFramebuffer(i.FRAMEBUFFER, c.framebuffer), i.useProgram(this.#s), this.#T(0, h.textures[0], this.#a.previous), this.#T(1, n.textures[0], this.#a.second), this.#T(2, n.textures[1], this.#a.first), i.uniform2i(this.#a.size, n.width, n.height), i.viewport(0, 0, x, 1), i.drawArrays(i.TRIANGLES, 0, 3), this.#r = 1 - this.#r, i.deleteSync(this.#A), i.bindBuffer(i.PIXEL_PACK_BUFFER, this.#h), i.readPixels(0, 0, x, 1, i.RGBA, i.FLOAT, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), this.#A = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), i.flush();
  }
  /**
   * The phase of the last frame measured, once the GPU has handed it back,
   * and null while it is still on its way. It is handed back once.
   */
  poll() {
    const e = this.#t, t = this.#A;
    if (t === null || this.#h === null) return null;
    switch (e.clientWaitSync(t, 0, 0)) {
      case e.ALREADY_SIGNALED:
      case e.CONDITION_SATISFIED:
        return e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#h), e.getBufferSubData(e.PIXEL_PACK_BUFFER, 0, this.metrics), e.bindBuffer(e.PIXEL_PACK_BUFFER, null), e.deleteSync(t), this.#A = null, {
          phase: this.metrics[f.phase * 4] ?? 0,
          run: this.metrics[f.phase * 4 + 1] ?? 0
        };
      default:
        return null;
    }
  }
  destroy() {
    const e = this.#t;
    if (this.#v(), this.#i !== null) {
      for (const t of this.#i) y(e, t);
      this.#i = null;
    }
    e.deleteSync(this.#A), this.#A = null, e.deleteBuffer(this.#h), this.#h = null, e.deleteProgram(this.#c), e.deleteProgram(this.#o), e.deleteProgram(this.#s);
  }
  #T(e, t, A) {
    const i = this.#t;
    i.activeTexture(i.TEXTURE0 + e), i.bindTexture(i.TEXTURE_2D, t ?? null), i.uniform1i(A, e);
  }
  #v() {
    const e = this.#t;
    this.#l !== null && y(e, this.#l), this.#f !== null && y(e, this.#f), this.#l = null, this.#f = null;
  }
  /** Everything detect needs that is not there yet. */
  #y() {
    const e = this.#t;
    if (this.#l === null || this.#f === null) {
      this.#v();
      const t = Math.ceil(this.#n / Q), A = Math.ceil(this.#E / ($ * 2));
      this.#l = _(e, t, A, 2), this.#f = _(
        e,
        Math.ceil(t / g),
        Math.ceil(A / g),
        2
      );
    }
    this.#i === null && (this.#i = [
      _(e, x, 1, 1),
      _(e, x, 1, 1)
    ], this.#r = 0, this.reset()), this.#h === null && (this.#h = e.createBuffer(), e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#h), e.bufferData(
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
    throw y(s, a), new Error("failed to allocate framebuffer");
  return a;
}
function y(s, { framebuffer: e, textures: t }) {
  s.deleteFramebuffer(e);
  for (const A of t) s.deleteTexture(A);
}
const V = [
  "mozParsedFrames",
  "mozDecodedFrames",
  "mozPresentedFrames",
  "mozPaintedFrames"
];
function Y(s) {
  return V.every((e) => e in s);
}
function fe() {
  return typeof HTMLVideoElement < "u" && (Y(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const de = 250, pe = 500;
class me {
  #t;
  #c;
  #e = null;
  #o = null;
  #m = null;
  #s = null;
  #a = !0;
  #l = null;
  #f = null;
  #i = 0;
  constructor(e) {
    if (this.#t = e, this.#c = Y(e) ? e : null, this.#c) {
      for (const t of ["emptied", "seeking", "seeked"])
        e.addEventListener(t, this.#h);
      for (const t of ["pause", "playing", "waiting", "ratechange"])
        e.addEventListener(t, this.#r);
    }
  }
  request(e) {
    this.#e === null && (this.#o = e, this.#e = this.#c ? requestAnimationFrame(this.#n) : this.#t.requestVideoFrameCallback(this.#A));
  }
  cancel() {
    this.#e !== null && (this.#c ? cancelAnimationFrame(this.#e) : this.#t.cancelVideoFrameCallback(this.#e)), this.#e = null, this.#o = null, this.#h();
  }
  destroy() {
    this.cancel();
    for (const e of ["emptied", "seeking", "seeked"])
      this.#t.removeEventListener(e, this.#h);
    for (const e of ["pause", "playing", "waiting", "ratechange"])
      this.#t.removeEventListener(e, this.#r);
  }
  #r = () => {
    this.#l = null, this.#f = null, this.#i = 0;
  };
  #h = () => {
    this.#m = null, this.#s = null, this.#a = !0, this.#r();
  };
  #A = (e, t) => {
    const A = this.#o;
    this.#e = null, this.#o = null, A?.(e, t);
  };
  #n = (e) => {
    const t = this.#c, A = V.map((a) => t[a]);
    this.#m?.some((a, h) => A[h] < a) && this.#h(), this.#m = A;
    const i = t.mozPaintedFrames, r = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, n = this.#s === null && (i > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (r && (n || this.#s !== null && i !== this.#s)) {
      if (this.#l !== null && e - this.#l > pe && (this.#r(), this.#a = !0), !t.paused && !t.ended) {
        const h = this.#f;
        if (h && e - h.at >= de) {
          const c = i - h.frames, o = (e - h.at) / c;
          c > 0 && o >= 4 && o <= 200 && (this.#i = this.#i ? this.#i + (o - this.#i) * 0.25 : o), this.#f = null;
        }
        this.#f ??= { at: e, frames: i };
      }
      this.#l = e, this.#s = i;
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
      this.#e = requestAnimationFrame(this.#n);
  };
}
const Ee = 0.5, p = 4, w = 5, G = 1e3, ve = 4, P = 200, xe = 0.25, we = 0.75, Te = 0.1, N = 1, De = 1e3 / 60, Fe = 0.02, be = 0.1, X = 1, C = 5, Re = 4, ge = {
  2: 0,
  3: 0.25,
  4: 0.5,
  5: 0.75
}, _e = `#version 300 es
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
function Ue() {
  return fe() && typeof WebGL2RenderingContext < "u";
}
class ke {
  canvas;
  #t;
  #c;
  #e;
  /** The pulldown detection, which keeps its measurements on the GPU. */
  #o;
  #m;
  #s;
  /** The program that copies a filtered picture onto the canvas. */
  #a;
  #l;
  #f;
  #i = [];
  /** Somewhere to filter a field into, and to read it back out of. */
  #r = [];
  /** Which output slot was written last; the next one follows round the ring. */
  #h = w - 1;
  /** Filtered fields waiting for their moment, oldest first. */
  #A = [];
  /** The last picture scheduled, shown or not; the next follows on from it. */
  #n = null;
  /** The rAF loop that puts them up, which is all that draws on the canvas. */
  #E = null;
  /** When the loop last ran, and the refresh grid fitted to it. See #measureRefresh. */
  #T = 0;
  #v = De;
  #y = 0;
  /** How far ahead of its moment the last picture was shown. See #present. */
  #ve = 0;
  /** Debug only: the rAF the last picture was drawn in, and repeats dropped since. */
  #oe = 0;
  #ne = 0;
  /** The `<div>` this put around the element, so it can be taken away again. */
  #P = null;
  #he;
  #C;
  #xe;
  #M;
  #R;
  /** How long a frame lasts in wall time, from what the frames themselves say. */
  #u = 0;
  /** The size of a frame as it is coded, which is what a texture holds. */
  #F = 0;
  #g = 0;
  /** Where the newest frame is. The two before it follow round the ring. */
  #x = p - 1;
  /** How many of the held frames are consecutive, up to HISTORY. */
  #d = 0;
  #K = 0;
  #b = !1;
  #ae = !1;
  #p = null;
  #Z = [];
  #L = !1;
  #ce;
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
  #q = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #ue = 0;
  #ee = 0;
  #B = 0;
  #Q = 0;
  #S = 0;
  #$ = 0;
  #I = 0;
  #U;
  #te = [];
  #k = [];
  #V = 0;
  #G = 0;
  #Y = 0;
  #N = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #j = D;
  #X = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #w = D;
  #z = !1;
  #O = null;
  #we = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #Ae = [0, 0, 0, 0, 0, 0];
  #le = 0;
  constructor(e, t = {}) {
    this.#t = e, this.#C = t.doubleRate ?? !1, this.#xe = t.spatialCheck ?? !0, this.#M = t.debug ?? !1, this.#R = t.film ?? !1, this.#ce = t.onStats, this.canvas = document.createElement("canvas"), this.canvas.style.cssText = "position:absolute;pointer-events:none;visibility:hidden";
    const A = this.canvas.getContext("webgl2", {
      alpha: !1,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!A) throw new Error("this browser has no WebGL2");
    this.#e = A, this.#o = new le(A), this.#m = b(A, ue, R);
    const i = this.#m;
    this.#s = Object.fromEntries(
      Object.entries(ce).map(([r, n]) => [
        r,
        A.getUniformLocation(i, n)
      ])
    ), this.#a = b(A, _e, R), this.#l = A.getUniformLocation(this.#a, "uField"), this.#f = A.getUniformLocation(this.#a, "uFlip"), this.#R && this.#De(), this.#U = A.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.canvas.addEventListener("webglcontextlost", this.#Ie), this.#he = new ResizeObserver(() => this.#re()), this.#c = new me(e), e.addEventListener("emptied", this.#Ce), e.addEventListener("resize", this.#Pe), e.addEventListener("pause", this.#_), e.addEventListener("ended", this.#_), e.addEventListener("seeked", this.#_), e.addEventListener("ratechange", this.#_);
  }
  get running() {
    return this.#b && (this.#p?.interlaced ?? !0);
  }
  /** Whether the caller wants filtering, independently of the current source. */
  get enabled() {
    return this.#ae;
  }
  set enabled(e) {
    this.#ae = e, this.#fe();
  }
  /** Update whether the source needs filtering and which field comes first. */
  set scan(e) {
    this.#p = e, this.#fe();
  }
  get scan() {
    return this.#p;
  }
  set videoTimeline(e) {
    this.#Z = e, e.length === 0 && (this.#p = null), this.#fe();
  }
  get videoTimeline() {
    return this.#Z;
  }
  /**
   * What to put on the screen for fullscreen: the `<div>` holding both the
   * element and the canvas once there is one, and the element itself before
   * that. Fullscreening the element alone would leave the canvas behind in
   * the page, and with it the only deinterlaced picture there is.
   */
  get container() {
    return this.#P ?? this.#t;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#C;
  }
  set doubleRate(e) {
    e !== this.#C && (this.#C = e, this.#Te());
  }
  get film() {
    return this.#R;
  }
  set film(e) {
    e !== this.#R && (e && this.#De(), this.#R = e, e || (this.#z = !1, this.#w = D, this.#W()), this.#Te());
  }
  get debug() {
    return this.#M;
  }
  set debug(e) {
    this.#M = e;
  }
  /** Whether pictures are queued and put up by the loop rather than drawn on arrival. */
  get #H() {
    return this.#C || this.#R;
  }
  #Te() {
    this.#H ? (this.#F > 0 && this.#Se(), this.#pe()) : (this.#me(), this.#J());
  }
  #De() {
    if (this.#e.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  #fe() {
    this.#ae && (this.#Z.length > 0 || (this.#p?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  start() {
    this.#b || this.#L || (this.#b = !0, this.#Le(), this.#je(), this.#ie(), (this.#p?.interlaced ?? !0) && this.#pe());
  }
  /** Take the deinterlaced picture away, leaving the element's own showing. */
  stop() {
    this.#b && (this.#b = !1, this.#c.cancel(), this.#me(), this.#d = 0, this.canvas.style.visibility = "hidden");
  }
  destroy() {
    this.stop(), this.#c.destroy(), this.canvas.removeEventListener("webglcontextlost", this.#Ie), this.#t.removeEventListener("emptied", this.#Ce), this.#t.removeEventListener("resize", this.#Pe), this.#t.removeEventListener("pause", this.#_), this.#t.removeEventListener("ended", this.#_), this.#t.removeEventListener("seeked", this.#_), this.#t.removeEventListener("ratechange", this.#_), this.#Je();
    for (const e of this.#i) this.#e.deleteTexture(e);
    this.#i = [], this.#J();
    for (const e of [
      ...this.#te,
      ...this.#k.map(({ q: t }) => t)
    ])
      this.#e.deleteQuery(e);
    this.#te.length = 0, this.#k.length = 0, this.#o.destroy(), this.#O !== null && (ee(this.#O), this.#O = null), this.#e.deleteProgram(this.#m), this.#e.deleteProgram(this.#a), this.#e.getExtension("WEBGL_lose_context")?.loseContext();
  }
  #ie() {
    this.#b && this.#c.request(this.#Xe);
  }
  #Ue(e, t, A) {
    this.#O == null && (this.#O = K(this.#e, "20px monospace")), te(this.#O, e, t, A, this.#F, this.#g, 20);
  }
  #Fe(e) {
    if (this.#U == null || this.#k.length > 30)
      return;
    const t = this.#te.pop() ?? this.#e.createQuery();
    return this.#e.beginQuery(this.#U.TIME_ELAPSED_EXT, t), this.#k.push({ q: t, isField: e }), t;
  }
  #be(e) {
    this.#U != null && (e != null && this.#e.endQuery(this.#U.TIME_ELAPSED_EXT), this.#k = this.#k.filter(
      ({ q: t, isField: A }) => {
        if (this.#e.getQueryParameter(t, this.#e.QUERY_RESULT_AVAILABLE)) {
          const i = this.#e.getQueryParameter(t, this.#e.QUERY_RESULT);
          return A ? (this.#Y += i, this.#N++) : (this.#V += i, this.#G++), this.#te.push(t), !1;
        }
        return !0;
      }
    ));
  }
  #W() {
    this.#w = D, this.#j = D, this.#X = 0, this.#z = !1, this.#o.reset();
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #ke() {
    const { cur: e, next: t } = this.#Me(!1), A = this.#i[e], i = this.#i[t];
    if (!A || !i) return;
    const r = this.#p?.topFieldFirst !== !1 ? 0 : 1;
    this.#o.detect(A, i, r);
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #Ge() {
    const e = this.#o.poll();
    e !== null && (this.#j = e, this.#X = 0), this.#X++;
    const { phase: t, run: A } = this.#j;
    t === 0 || this.#X > C ? this.#w = D : this.#w = {
      phase: (t - 1 + this.#X) % C + 1,
      run: A
    };
  }
  #Ne(e) {
    const t = [], A = this.#o.metrics;
    for (let r = 0; r < f.phase; r++) {
      const n = A[r * 4] ?? 0, a = A[r * 4 + 1] ?? 0, h = A[r * 4 + 2] ?? 0;
      t.push(
        `${n.toFixed(3)},${a.toString().padStart(4)},${h.toFixed(3)}`
      );
    }
    const i = this.#Ae.map((r, n) => `${n === 0 ? "-" : n}:${r}`).join(" ");
    return `frame=${e} phase=${this.#w.phase} run=${this.#w.run} known=${this.#j.phase}/${this.#j.run} age=${this.#X} ${this.#z ? "film" : "video"} period=${this.#u.toFixed(3)}
${t.join(" ")}
${i} dropped:${this.#le}`;
  }
  #Xe = (e, t) => {
    if (!(!this.#b || this.#L)) {
      if (this.#ze(t.mediaTime), t.width > 0 && t.height > 0) {
        if ((this.#F === 0 || this.#g === 0) && this.#Be(t.width, t.height), this.#p && !this.#p.interlaced) {
          this.#$e(), this.#ie();
          return;
        }
        const A = t.mediaTime - this.#K, i = t.mozTiming ? t.mozTiming.discontinuity : A < 0 || A > Ee;
        i && (this.#d = 0, this.#D.discontinuities++, this.#A.length = 0, this.#n = null, this.#W());
        const r = this.#Ve(t.presentedFrames, i);
        if (!t.mozTiming && this.#d > 0 && t.mediaTime === this.#K) {
          this.#ie();
          return;
        }
        t.mozTiming ? this.#u = t.mozTiming.periodMs : !i && A > 0 && this.#Oe(A), this.#K = t.mediaTime;
        const n = performance.now();
        n - this.#ue > G && (this.#ee = n, this.#B = 0, this.#Q = 0, this.#S = 0, this.#$ = 0, this.#I = 0, this.#V = 0, this.#G = 0, this.#Y = 0, this.#N = 0), this.#ue = n;
        const a = performance.now(), h = this.#Fe(!1);
        if (this.#ye(), this.#I = Math.max(
          this.#I,
          this.#A.length
        ), this.#R && (this.#d === p && r === 0 ? (this.#Ge(), this.#ke()) : this.#W(), this.#Ae[this.#w.phase] = (this.#Ae[this.#w.phase] ?? 0) + 1, this.#z = this.#w.phase !== 0 && this.#w.run >= Z, this.#M && (this.#we = this.#Ne(t.presentedFrames))), this.#Re() && !this.#t.paused && !this.#t.ended) {
          this.#A.length >= w && (this.#A.length = 0, this.#n = null, this.#D.queueResetted++);
          const c = (t.expectedDisplayTime || e) + this.#v;
          if (this.#z) {
            const o = this.#w.phase;
            if (o === I)
              this.#le++, this.#ne++;
            else {
              const l = this.#u * C / Re, d = ge[o] ?? 0, u = this.#de(
                "film",
                c + d * this.#u,
                l
              );
              this.#se("film", o, !1, u, l);
            }
          } else if (this.#C) {
            const o = this.#u / 2, l = this.#de("field", c, o);
            this.#se("field", 1, !1, l, o), this.#se("field", 2, !0, l + o, o);
          } else {
            const o = this.#u, l = this.#de("frame", c, o);
            this.#se("frame", 0, !1, l, o);
          }
        } else
          this.#Ee(this.#t.paused || this.#t.ended, !1, null);
        this.#be(h), this.#Q += performance.now() - a, this.#B++, this.#Ye(n);
      }
      this.#ie();
    }
  };
  #ze(e) {
    let t;
    for (let i = this.#Z.length - 1; i >= 0; i--) {
      const r = this.#Z[i];
      if (r.start <= e + 1e-6) {
        t = r;
        break;
      }
    }
    t?.codedSize && (t.codedSize.width !== this.#F || t.codedSize.height !== this.#g) && this.#Be(t.codedSize.width, t.codedSize.height);
    const A = t?.scan;
    !A || this.#p?.interlaced === A.interlaced && this.#p.topFieldFirst === A.topFieldFirst || (this.#p = A, this.#d = 0, this.#A.length = 0, this.#n = null, this.#W(), A.interlaced ? this.#H && this.#pe() : this.#me());
  }
  /**
   * Whether pictures are being filtered ahead of time and queued, rather than
   * drawn as their frame arrives.
   *
   * A picture for every frame has nothing to schedule -- there is one of them
   * and it goes up now -- and neither has a filter that has yet to see two
   * frames go by, since until then there is no idea how long a frame lasts.
   */
  #Re() {
    return this.#H && this.#u > 0 && this.#r.length === w;
  }
  /**
   * When a picture goes up: one duration after the last one of its cadence,
   * nudged towards `ideal` by a fraction of the gap so that the schedule
   * follows the clock without a picture ever moving across a refresh. It
   * restarts from `ideal` when the gap has grown to a whole picture or the
   * cadence has changed.
   */
  #de(e, t, A) {
    const i = this.#n;
    if (i !== null && i.cadence === e) {
      const r = i.at + i.duration, n = t - r;
      if (Math.abs(n) < A) {
        const a = Math.max(
          -N,
          Math.min(N, n * Te)
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
  #Oe(e) {
    const t = e * 1e3 / (this.#t.playbackRate || 1), A = this.#u > 0 ? Math.max(1, Math.round(t / this.#u)) : 1, i = t / A;
    i < ve || i > P || (this.#u = this.#u > 0 && i > this.#u * we ? this.#u + (i - this.#u) * xe : i);
  }
  /**
   * Filter one field into an output texture and put it in the queue.
   *
   * The three frames the filter reads are only the right three between one
   * frame arriving and the next, so both fields of a frame are built here and
   * held as pictures. What is queued after that is a copy waiting for a
   * moment, which no later frame can take away.
   */
  #se(e, t, A, i, r) {
    const n = (this.#h + 1) % w, a = this.#r[n];
    if (!a) return;
    for (this.#h = n; this.#A.length > 0 && this.#A[0]?.slot === n; )
      this.#A.shift(), this.#D.late++;
    this.#Ee(!1, A, a.framebuffer);
    const h = {
      slot: n,
      at: i,
      duration: r,
      cadence: e,
      phase: t,
      droppedBefore: this.#ne
    };
    this.#ne = 0, this.#A.push(h), this.#n = h;
  }
  /** The loop that puts filtered fields up, and the only thing that draws. */
  #pe() {
    this.#E === null && (!this.#b || this.#L || !this.#H || (this.#E = requestAnimationFrame(this.#ge)));
  }
  #me() {
    this.#E !== null && cancelAnimationFrame(this.#E), this.#E = null, this.#A.length = 0, this.#n = null;
  }
  #ge = (e) => {
    this.#E = null, !(!this.#b || this.#L || !this.#H) && (this.#He(e), this.#We(this.#y, e), this.#E = requestAnimationFrame(this.#ge));
  };
  /**
   * Fit a grid of refreshes (period and phase) to the animation frames. rAF
   * timestamps wander by a millisecond or so, which is more than the
   * nearest-refresh decision in #present can take; the grid is what it
   * compares against. A frame far off the grid restarts it.
   */
  #He(e) {
    const t = e - this.#T;
    this.#T = e;
    const A = Math.max(1, Math.round(t / this.#v)), i = this.#y + A * this.#v, r = e - i;
    if (this.#y === 0 || t <= 0 || t > P || Math.abs(r) > this.#v / 4) {
      t > 0 && t <= P && (this.#v = t), this.#y = e;
      return;
    }
    this.#v += r / A * Fe, this.#y = i + r * be;
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
  #We(e, t) {
    const A = this.#v / 2, i = (h) => {
      const c = h.at - e;
      return c <= A - X ? !0 : c > A + X ? !1 : this.#ve > 0;
    };
    for (; this.#A[1] && i(this.#A[1]); )
      this.#D.late++, this.#A.shift();
    const r = this.#A[0];
    if (!r || !i(r)) return;
    this.#A.shift(), this.#ve = r.at - e;
    const n = performance.now(), a = this.#Fe(!0);
    this.#Qe(r.slot), this.#be(a), this.#$ += performance.now() - n, this.#S++, this.#M && this.#Ze(r, t), this.#oe = t;
  }
  #Ze(e, t) {
    const A = this.#oe === 0 ? 0 : t - this.#oe, i = A / this.#v, r = e.cadence === "film" ? `phase ${e.phase}` : e.cadence === "field" ? `field ${e.phase}` : "frame", n = e.droppedBefore > 0 ? `, phase ${I} dropped before it` + (e.droppedBefore > 1 ? ` (${e.droppedBefore})` : "") : "";
    console.log(
      `yadif: +${A.toFixed(2)} ms (${i.toFixed(2)} refreshes) ${e.cadence} ${r}, due ${(e.at - t).toFixed(2)} ms${n}`
    );
  }
  /** Copy one of the filtered pictures onto the canvas. */
  #Qe(e) {
    const t = this.#r[e];
    t && this.#_e(t.texture);
  }
  /** Put a progressive frame through unchanged, keeping one display surface. */
  #$e() {
    this.#ye();
    const e = this.#i[this.#x];
    e && this.#_e(e, !0), this.#d = 0;
  }
  #_e(e, t = !1) {
    const A = this.#e;
    A.bindFramebuffer(A.FRAMEBUFFER, null), A.useProgram(this.#a), A.activeTexture(A.TEXTURE0), A.bindTexture(A.TEXTURE_2D, e), A.uniform1i(this.#l, 0), A.uniform1i(this.#f, t ? 1 : 0), A.viewport(0, 0, this.#F, this.#g), A.drawArrays(A.TRIANGLES, 0, 3), this.canvas.style.visibility = "visible";
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
  #Ve(e, t) {
    let A = 0;
    return this.#q !== 0 && !t && (A = Math.max(0, e - this.#q - 1), this.#D.missed += A), this.#q = e, A;
  }
  #Ye(e) {
    if (!this.#ce) return;
    const t = e - this.#ee;
    if (t < G) return;
    const A = this.#Re() ? this.#S : this.#B;
    let i = 0;
    this.#B !== 0 && (i += this.#Q / this.#B), this.#S !== 0 && (i += this.#$ / this.#S / 2);
    let r;
    this.#U != null && (r = 0, this.#G !== 0 && (r += this.#V / 1e6 / this.#G), this.#N !== 0 && (r += this.#Y / 1e6 / this.#N / 2)), this.#ce({
      ...this.#D,
      // The element's own count of what its decoder could not keep up with,
      // which is the machine being behind rather than this filter.
      dropped: this.#t.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
      fps: A * 1e3 / t,
      frameMs: i,
      maxQueuedFields: this.#I,
      gpuMs: r,
      film: this.#z
    }), this.#ee = e, this.#B = 0, this.#Q = 0, this.#S = 0, this.#$ = 0, this.#I = 0, this.#V = 0, this.#G = 0, this.#Y = 0, this.#N = 0;
  }
  /** Take the newest frame into the ring. */
  #ye() {
    const e = this.#e;
    this.#x = (this.#x + 1) % p, e.bindTexture(e.TEXTURE_2D, this.#i[this.#x] ?? null), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      e.RGBA,
      e.UNSIGNED_BYTE,
      this.#t
    ), this.#d = Math.min(this.#d + 1, p);
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
  #Ee(e, t, A) {
    if (this.#d === 0 || this.#L) return;
    this.#d === p && !e ? this.#D.filtered++ : this.#D.degraded++;
    const i = this.#e, { prev: r, cur: n, next: a } = this.#Me(e);
    i.bindFramebuffer(i.FRAMEBUFFER, A), i.useProgram(this.#m);
    for (const [l, d] of [r, n, a].entries())
      i.activeTexture(i.TEXTURE0 + l), i.bindTexture(i.TEXTURE_2D, this.#i[d] ?? null);
    i.uniform1i(this.#s.prev, 0), i.uniform1i(this.#s.cur, 1), i.uniform1i(this.#s.next, 2);
    const h = this.#R ? this.#o.texture : null, c = h !== null;
    h !== null && (i.activeTexture(i.TEXTURE0 + 3), i.bindTexture(i.TEXTURE_2D, h), i.uniform1i(this.#s.fieldMetrics, 3)), i.uniform2i(this.#s.size, this.#F, this.#g);
    const o = this.#p?.topFieldFirst !== !1 ? 0 : 1;
    i.uniform1i(this.#s.parity, t ? 1 - o : o), i.uniform1i(this.#s.second, t ? 1 : 0), i.uniform1i(
      this.#s.tff,
      this.#p?.topFieldFirst !== !1 ? 1 : 0
    ), i.uniform1i(this.#s.spatialCheck, this.#xe ? 1 : 0), i.uniform1i(this.#s.debug, this.#M ? 1 : 0), i.uniform1i(this.#s.film, c ? 1 : 0), i.uniform1i(this.#s.phase, this.#w.phase), i.viewport(0, 0, this.#F, this.#g), i.drawArrays(i.TRIANGLES, 0, 3), this.#M && c && this.#Ue(this.#we, 0, 90), A === null && (this.canvas.style.visibility = "visible");
  }
  #Me(e) {
    const t = (A) => (this.#x + p - A) % p;
    return this.#d === 1 ? { prev: this.#x, cur: this.#x, next: this.#x } : e ? { prev: t(1), cur: this.#x, next: this.#x } : this.#d === 2 ? { prev: t(1), cur: t(1), next: this.#x } : { prev: t(2), cur: t(1), next: this.#x };
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
  #re() {
    if (!this.#P) return;
    const e = this.#t, t = e.videoWidth, A = e.videoHeight;
    if (t === 0 || A === 0) return;
    const i = Math.min(
      e.offsetWidth / t,
      e.offsetHeight / A
    ), r = t * i, n = A * i;
    this.canvas.style.left = `${e.offsetLeft + (e.offsetWidth - r) / 2}px`, this.canvas.style.top = `${e.offsetTop + (e.offsetHeight - n) / 2}px`, this.canvas.style.width = `${r}px`, this.canvas.style.height = `${n}px`;
  }
  #Be(e, t) {
    const A = this.#e;
    this.canvas.width = e, this.canvas.height = t, this.#F = e, this.#g = t, this.#d = 0, this.#re();
    for (const i of this.#i) A.deleteTexture(i);
    this.#i = [];
    for (let i = 0; i < p; i++) {
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
    this.#J(), this.#o.resize(e, t), this.#H && this.#Se();
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
  #Se() {
    const e = this.#e;
    if (!(this.#r.length === w || this.#F === 0)) {
      this.#J();
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
          e.deleteFramebuffer(i), e.deleteTexture(A), this.#J();
          return;
        }
        this.#r.push({ texture: A, framebuffer: i });
      }
      this.#h = w - 1;
    }
  }
  #J() {
    const e = this.#e;
    for (const { texture: t, framebuffer: A } of this.#r)
      e.deleteFramebuffer(A), e.deleteTexture(t);
    this.#r = [], this.#A.length = 0, this.#n = null;
  }
  /**
   * Wrap the element in a `<div>` of this one's own and put the canvas over
   * it. The wrapper is what the canvas is positioned against; moving the
   * element out of the tree and back within the one task leaves playback
   * alone, which is what makes turning this on mid-stream free.
   */
  #je() {
    if (this.#P) return;
    const e = this.#t.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#t), t.appendChild(this.#t), t.appendChild(this.canvas), this.#P = t, this.#he.observe(this.#t), this.#re();
  }
  #Je() {
    const e = this.#P;
    this.#P = null, this.#he.disconnect(), this.canvas.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#t, e), e.remove());
  }
  #Pe = () => this.#re();
  #Ce = () => {
    this.#d = 0, this.#K = 0, this.#A.length = 0, this.#n = null, this.#W(), this.#u = 0, this.#Le(), this.canvas.style.visibility = "hidden";
  };
  #Le() {
    this.#D = {
      filtered: 0,
      missed: 0,
      degraded: 0,
      discontinuities: 0,
      late: 0,
      resynced: 0,
      queueResetted: 0
    }, this.#Ae.fill(0), this.#le = 0, this.#q = 0, this.#ee = 0, this.#ue = 0, this.#B = 0, this.#Q = 0, this.#S = 0, this.#$ = 0, this.#I = 0, this.#V = 0, this.#G = 0, this.#Y = 0, this.#N = 0;
  }
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #_ = () => {
    this.#A.length = 0, this.#n = null, this.#W(), this.#b && this.#Ee(!0, !1, null);
  };
  /**
   * A lost context takes the textures and the program with it. Rebuilding
   * them is possible, but a page that has lost its context has bigger
   * problems; getting out of the way leaves the element's own picture showing.
   */
  #Ie = (e) => {
    e.preventDefault(), this.#L = !0, this.stop();
  };
}
const z = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", ye = 0.5, Me = 3e3, O = 0.1, F = 16, H = 'video/mp4; codecs="avc1.640029"';
let U = null;
function Be(s = {}) {
  return U ??= Se(s), U;
}
async function Ge(s = {}) {
  return (await Be(s)).deinterlaces;
}
function Ne() {
  U = null;
}
async function Se(s) {
  const e = s.tolerance ?? ye, t = s.timeoutMs ?? Me, A = performance.now(), i = (a) => ({
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
    n = Ce(r, t);
    const a = B(M(r, "loadeddata"), t), h = r.play().then(
      () => !0,
      () => !1
    );
    if (await n.ready, await a, await Le(r, t, await h), r.videoWidth === 0 || r.videoHeight === 0)
      return i(new Error("the probe clip decoded to nothing"));
    const c = Ie(r);
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
const L = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, Pe = typeof MediaSource > "u";
function Ce(s, e) {
  if (!L || !L.isTypeSupported(H))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = z.indexOf(","), A = atob(z.slice(t + 1)), i = new Uint8Array(A.length);
  for (let h = 0; h < A.length; h++) i[h] = A.charCodeAt(h);
  const r = new L(), n = URL.createObjectURL(r);
  if (Pe) {
    s.disableRemotePlayback = !0;
    const h = document.createElement("source");
    h.type = "video/mp4", h.src = n, s.append(h), s.load();
  } else
    s.src = n;
  const a = (async () => {
    await B(M(r, "sourceopen"), e);
    const h = r.addSourceBuffer(H), c = B(M(h, "updateend"), e);
    h.appendBuffer(i), await c, r.endOfStream();
  })();
  return { url: n, ready: a };
}
async function Le(s, e, t) {
  if (t) {
    const A = performance.now();
    for (; s.currentTime < O && performance.now() - A < e; )
      await new Promise((i) => requestAnimationFrame(i));
    s.pause();
  } else
    s.currentTime = O, await B(M(s, "seeked"), e);
}
function Ie(s) {
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
function M(s, e) {
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
function B(s, e) {
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
  ke as Deinterlacer,
  ue as YADIF_FRAGMENT_SHADER,
  ce as YADIF_UNIFORMS,
  Ge as decoderDeinterlaces,
  Ne as forgetDecoderProbe,
  Be as probeDecoder,
  Ue as supportsDeinterlace
};
