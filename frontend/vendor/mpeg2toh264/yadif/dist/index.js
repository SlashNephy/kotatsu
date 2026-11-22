const b = `#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;
function R(r, e, t) {
  const i = r.createProgram(), s = k(r, r.VERTEX_SHADER, t), A = k(r, r.FRAGMENT_SHADER, e);
  if (r.attachShader(i, s), r.attachShader(i, A), r.linkProgram(i), r.deleteShader(s), r.deleteShader(A), !r.getProgramParameter(i, r.LINK_STATUS)) {
    const n = r.getProgramInfoLog(i);
    throw r.deleteProgram(i), new Error(
      `the deinterlacer failed to link: ${n ?? "no reason given"}`
    );
  }
  return i;
}
function k(r, e, t) {
  const i = r.createShader(e);
  if (!i) throw new Error("the deinterlacer could not create a shader");
  if (r.shaderSource(i, t), r.compileShader(i), !r.getShaderParameter(i, r.COMPILE_STATUS)) {
    const s = r.getShaderInfoLog(i);
    throw r.deleteShader(i), new Error(
      `the deinterlacer failed to compile: ${s ?? "no reason given"}`
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
function ee(r, e) {
  const t = R(r, J, q), i = r.getAttribLocation(t, "aVertexPosition"), s = r.getAttribLocation(t, "aTextureCoord"), A = r.getUniformLocation(t, "uTexture"), n = r.getUniformLocation(t, "uMatrix"), o = r.getUniformLocation(t, "uUvMatrix"), a = r.getUniformLocation(t, "uTextColor"), c = r.getUniformLocation(t, "uBackColor");
  if (A == null || n == null || o == null || a == null || c == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const h = r.createBuffer(), u = r.createBuffer();
  return {
    gl: r,
    ...te(r, e),
    program: t,
    programUniforms: {
      vertex: i,
      textureCoord: s,
      texture: A,
      matrix: n,
      uvMatrix: o,
      textColor: a,
      backColor: c
    },
    positionBuffer: h,
    textureBuffer: u
  };
}
function te(r, e) {
  const t = new OffscreenCanvas(0, 0), i = t.getContext("2d"), s = /* @__PURE__ */ new Map();
  let A = 0;
  const n = 0;
  let o = 1;
  i.font = e, i.fillStyle = "white";
  for (let c = 32; c < 128; c++) {
    const h = String.fromCharCode(c), u = i.measureText(h), f = Math.ceil(
      u.actualBoundingBoxDescent + u.actualBoundingBoxAscent + 1
    ), l = Math.ceil(
      u.actualBoundingBoxLeft + u.actualBoundingBoxRight + 1
    );
    s.set(h, {
      x: A,
      y: n,
      width: l,
      height: f,
      metrics: u
    }), o = Math.max(o, f), A += l;
  }
  t.width = A, t.height = o, i.font = e, i.fillStyle = "white";
  for (const [c, h] of s)
    i.fillText(
      c,
      Math.floor(h.x + h.metrics.actualBoundingBoxLeft + 1),
      Math.floor(h.metrics.actualBoundingBoxAscent + 1)
    );
  const a = r.createTexture();
  return r.bindTexture(r.TEXTURE_2D, a), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_MIN_FILTER, r.LINEAR), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_MAG_FILTER, r.LINEAR), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_WRAP_S, r.CLAMP_TO_EDGE), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_WRAP_T, r.CLAMP_TO_EDGE), r.texImage2D(r.TEXTURE_2D, 0, r.RGBA, r.RGBA, r.UNSIGNED_BYTE, t), { fontTexture: a, chars: s, textureSize: { width: A, height: o } };
}
function ie(r) {
  const e = r.gl;
  e.deleteBuffer(r.positionBuffer), e.deleteBuffer(r.textureBuffer), e.deleteTexture(r.fontTexture), e.deleteProgram(r.program);
}
function se(r, e, t, i, s, A, n) {
  const o = [], a = [], c = t;
  for (const f of e) {
    if (f === `
`) {
      t = c, i += n;
      continue;
    }
    const l = r.chars.get(f);
    if (l == null)
      continue;
    if (l.width === 1) {
      t += l.metrics.width;
      continue;
    }
    const p = Math.floor(t - l.metrics.actualBoundingBoxLeft), v = Math.floor(i - l.metrics.actualBoundingBoxAscent), w = p + l.width, T = v + l.height;
    o.push(p, v), a.push(l.x, l.y), o.push(p, T), a.push(l.x, l.y + l.height), o.push(p + l.width, T), a.push(l.x + l.width, l.y + l.height), o.push(w, T), a.push(l.x + l.width, l.y + l.height), o.push(p, v), a.push(l.x, l.y), o.push(w, v), a.push(l.x + l.width, l.y), t += l.metrics.width;
  }
  const h = r.gl;
  h.useProgram(r.program), h.bindBuffer(h.ARRAY_BUFFER, r.positionBuffer), h.bufferData(h.ARRAY_BUFFER, new Float32Array(o), h.STATIC_DRAW), h.vertexAttribPointer(
    r.programUniforms.vertex,
    2,
    h.FLOAT,
    !1,
    0,
    0
  ), h.enableVertexAttribArray(r.programUniforms.vertex), h.bindBuffer(h.ARRAY_BUFFER, r.textureBuffer), h.bufferData(
    h.ARRAY_BUFFER,
    new Float32Array(a),
    h.STATIC_DRAW
  ), h.vertexAttribPointer(
    r.programUniforms.textureCoord,
    2,
    h.FLOAT,
    !1,
    0,
    0
  ), h.enableVertexAttribArray(r.programUniforms.textureCoord), h.activeTexture(h.TEXTURE0), h.bindTexture(h.TEXTURE_2D, r.fontTexture), h.uniform1i(r.programUniforms.texture, 0), h.uniform3fv(r.programUniforms.textColor, [1, 1, 1]), h.uniform3fv(r.programUniforms.backColor, [0, 0, 0]);
  function u(f, l, p) {
    const v = [];
    for (let w = 0; w < l; w++)
      for (let T = 0; T < f; T++)
        v.push(p[T * f + w]);
    return v;
  }
  h.uniformMatrix4fv(r.programUniforms.matrix, !1, u(4, 4, [
    1 / (s / 2),
    0,
    0,
    -1,
    0,
    -2 / A,
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
  ])), h.uniformMatrix3fv(r.programUniforms.uvMatrix, !1, u(3, 3, [
    1 / r.textureSize.width,
    0,
    0,
    0,
    1 / r.textureSize.height,
    0,
    0,
    0,
    1
  ])), h.viewport(0, 0, s, A), h.enable(h.BLEND), h.blendFunc(h.SRC_ALPHA, h.ONE_MINUS_SRC_ALPHA), h.drawArrays(h.TRIANGLES, 0, o.length / 2), h.disable(h.BLEND);
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
}, E = 7, Q = 2, Ae = 16, re = 4e-3, N = 1, ne = 2, S = 5, oe = {
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
  return phase == ${N} || phase == ${ne};
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
function L(r, e, t) {
  return Object.fromEntries(
    Object.entries(t).map(([i, s]) => [
      i,
      r.getUniformLocation(e, s)
    ])
  );
}
class pe {
  #t;
  #l;
  #e;
  #n;
  #T;
  #A;
  #a;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #p = null;
  #m = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #s = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #o = null;
  #i = null;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(E * 4);
  #h = 0;
  #x = 0;
  constructor(e) {
    this.#t = e, this.#l = R(
      e,
      he,
      b
    ), this.#e = L(
      e,
      this.#l,
      oe
    ), this.#n = R(
      e,
      ce,
      b
    ), this.#T = L(
      e,
      this.#n,
      ae
    ), this.#A = R(
      e,
      ue,
      b
    ), this.#a = L(e, this.#A, le);
  }
  /** The newest measurements, or null before any frame has been measured. */
  get texture() {
    return this.#s?.[this.#r]?.textures[0] ?? null;
  }
  /** The size of the frames to be measured, which sizes the block grid. */
  resize(e, t) {
    e === this.#h && t === this.#x || (this.#h = e, this.#x = t, this.#w());
  }
  /** Forget every measurement: the next frame starts a cycle from nothing. */
  reset() {
    const e = this.#t;
    e.deleteSync(this.#i), this.#i = null;
    const t = this.#s?.[this.#r];
    if (!t) return;
    const i = new Float32Array(E * 4);
    for (let s = 0; s < d.phase; s++)
      i[s * 4 + 1] = 1;
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
    const s = this.#t;
    if (this.#h === 0 || this.#x === 0) return;
    this.#I();
    const A = this.#p, n = this.#m, o = this.#s;
    if (A === null || n === null || o === null) return;
    const a = o[this.#r], c = o[1 - this.#r];
    s.bindFramebuffer(s.FRAMEBUFFER, A.framebuffer), s.useProgram(this.#l), this.#F(0, e, this.#e.a), this.#F(1, t, this.#e.b), this.#F(2, a.textures[0], this.#e.fieldMetrics), s.uniform1i(this.#e.first, i), s.uniform2i(this.#e.size, this.#h, this.#x), s.viewport(0, 0, A.width, A.height), s.drawArrays(s.TRIANGLES, 0, 3), s.bindFramebuffer(s.FRAMEBUFFER, n.framebuffer), s.useProgram(this.#n), this.#F(0, A.textures[0], this.#T.second), this.#F(1, A.textures[1], this.#T.first), s.uniform2i(this.#T.size, A.width, A.height), s.viewport(0, 0, n.width, n.height), s.drawArrays(s.TRIANGLES, 0, 3), s.bindFramebuffer(s.FRAMEBUFFER, c.framebuffer), s.useProgram(this.#A), this.#F(0, a.textures[0], this.#a.previous), this.#F(1, n.textures[0], this.#a.second), this.#F(2, n.textures[1], this.#a.first), s.uniform2i(this.#a.size, n.width, n.height), s.viewport(0, 0, E, 1), s.drawArrays(s.TRIANGLES, 0, 3), this.#r = 1 - this.#r, s.deleteSync(this.#i), s.bindBuffer(s.PIXEL_PACK_BUFFER, this.#o), s.readPixels(0, 0, E, 1, s.RGBA, s.FLOAT, 0), s.bindBuffer(s.PIXEL_PACK_BUFFER, null), s.bindFramebuffer(s.FRAMEBUFFER, null), this.#i = s.fenceSync(s.SYNC_GPU_COMMANDS_COMPLETE, 0), s.flush();
  }
  /**
   * Overwrite the phase of the newest frame, for one whose comparisons cannot
   * be trusted but whose place in the cycle is known some other way. The
   * readback is taken again so that it says the same.
   */
  seed(e, t) {
    const i = this.#t, s = this.#s?.[this.#r];
    s && (i.bindTexture(i.TEXTURE_2D, s.textures[0] ?? null), i.texSubImage2D(
      i.TEXTURE_2D,
      0,
      d.phase,
      0,
      1,
      1,
      i.RGBA,
      i.FLOAT,
      new Float32Array([e, t, 0, 0])
    ), i.deleteSync(this.#i), i.bindFramebuffer(i.FRAMEBUFFER, s.framebuffer), i.bindBuffer(i.PIXEL_PACK_BUFFER, this.#o), i.readPixels(0, 0, E, 1, i.RGBA, i.FLOAT, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), this.#i = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), i.flush());
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
    if (this.#w(), this.#s !== null) {
      for (const t of this.#s) B(e, t);
      this.#s = null;
    }
    e.deleteSync(this.#i), this.#i = null, e.deleteBuffer(this.#o), this.#o = null, e.deleteProgram(this.#l), e.deleteProgram(this.#n), e.deleteProgram(this.#A);
  }
  #F(e, t, i) {
    const s = this.#t;
    s.activeTexture(s.TEXTURE0 + e), s.bindTexture(s.TEXTURE_2D, t ?? null), s.uniform1i(i, e);
  }
  #w() {
    const e = this.#t;
    this.#p !== null && B(e, this.#p), this.#m !== null && B(e, this.#m), this.#p = null, this.#m = null;
  }
  /** Everything detect needs that is not there yet. */
  #I() {
    const e = this.#t;
    if (this.#p === null || this.#m === null) {
      this.#w();
      const t = Math.ceil(this.#h / V), i = Math.ceil(this.#x / (Y * 2));
      this.#p = _(e, t, i, 2), this.#m = _(
        e,
        Math.ceil(t / g),
        Math.ceil(i / g),
        2
      );
    }
    this.#s === null && (this.#s = [
      _(e, E, 1, 1),
      _(e, E, 1, 1)
    ], this.#r = 0, this.reset()), this.#o === null && (this.#o = e.createBuffer(), e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#o), e.bufferData(
      e.PIXEL_PACK_BUFFER,
      this.metrics.byteLength,
      e.STREAM_READ
    ), e.bindBuffer(e.PIXEL_PACK_BUFFER, null));
  }
}
function _(r, e, t, i) {
  const s = r.createFramebuffer();
  r.bindFramebuffer(r.FRAMEBUFFER, s);
  const A = [];
  for (let a = 0; a < i; a++) {
    const c = r.createTexture();
    r.bindTexture(r.TEXTURE_2D, c), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_MIN_FILTER, r.NEAREST), r.texParameteri(r.TEXTURE_2D, r.TEXTURE_MAG_FILTER, r.NEAREST), r.texImage2D(
      r.TEXTURE_2D,
      0,
      r.RGBA32F,
      e,
      t,
      0,
      r.RGBA,
      r.FLOAT,
      null
    ), r.framebufferTexture2D(
      r.FRAMEBUFFER,
      r.COLOR_ATTACHMENT0 + a,
      r.TEXTURE_2D,
      c,
      0
    ), A.push(c);
  }
  r.drawBuffers(A.map((a, c) => r.COLOR_ATTACHMENT0 + c));
  const n = r.checkFramebufferStatus(r.FRAMEBUFFER) === r.FRAMEBUFFER_COMPLETE;
  r.bindFramebuffer(r.FRAMEBUFFER, null);
  const o = { framebuffer: s, textures: A, width: e, height: t };
  if (!n)
    throw B(r, o), new Error("failed to allocate framebuffer");
  return o;
}
function B(r, { framebuffer: e, textures: t }) {
  r.deleteFramebuffer(e);
  for (const i of t) r.deleteTexture(i);
}
const j = [
  "mozParsedFrames",
  "mozDecodedFrames",
  "mozPresentedFrames",
  "mozPaintedFrames"
];
function K(r) {
  return j.every((e) => e in r);
}
function me() {
  return typeof HTMLVideoElement < "u" && (K(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const Ee = 250, ve = 500;
class Te {
  #t;
  #l;
  #e = null;
  #n = null;
  #T = null;
  #A = null;
  #a = !0;
  #p = null;
  #m = null;
  #s = 0;
  constructor(e) {
    if (this.#t = e, this.#l = K(e) ? e : null, this.#l) {
      for (const t of ["emptied", "seeking", "seeked"])
        e.addEventListener(t, this.#o);
      for (const t of ["pause", "playing", "waiting", "ratechange"])
        e.addEventListener(t, this.#r);
    }
  }
  request(e) {
    this.#e === null && (this.#n = e, this.#e = this.#l ? requestAnimationFrame(this.#h) : this.#t.requestVideoFrameCallback(this.#i));
  }
  cancel() {
    this.#e !== null && (this.#l ? cancelAnimationFrame(this.#e) : this.#t.cancelVideoFrameCallback(this.#e)), this.#e = null, this.#n = null, this.#o();
  }
  destroy() {
    this.cancel();
    for (const e of ["emptied", "seeking", "seeked"])
      this.#t.removeEventListener(e, this.#o);
    for (const e of ["pause", "playing", "waiting", "ratechange"])
      this.#t.removeEventListener(e, this.#r);
  }
  #r = () => {
    this.#p = null, this.#m = null, this.#s = 0;
  };
  #o = () => {
    this.#T = null, this.#A = null, this.#a = !0, this.#r();
  };
  #i = (e, t) => {
    const i = this.#n;
    this.#e = null, this.#n = null, i?.(e, t);
  };
  #h = (e) => {
    const t = this.#l, i = j.map((o) => t[o]);
    this.#T?.some((o, a) => i[a] < o) && this.#o(), this.#T = i;
    const s = t.mozPaintedFrames, A = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, n = this.#A === null && (s > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (A && (n || this.#A !== null && s !== this.#A)) {
      if (this.#p !== null && e - this.#p > ve && (this.#r(), this.#a = !0), !t.paused && !t.ended) {
        const a = this.#m;
        if (a && e - a.at >= Ee) {
          const c = s - a.frames, h = (e - a.at) / c;
          c > 0 && h >= 4 && h <= 200 && (this.#s = this.#s ? this.#s + (h - this.#s) * 0.25 : h), this.#m = null;
        }
        this.#m ??= { at: e, frames: s };
      }
      this.#p = e, this.#A = s;
      const o = this.#a;
      this.#a = !1, this.#i(e, {
        width: t.videoWidth,
        height: t.videoHeight,
        // Used only to select source scan/size metadata on the media timeline.
        // It is deliberately NOT used as the frame identity or field clock.
        mediaTime: t.currentTime,
        presentedFrames: s,
        expectedDisplayTime: e,
        mozTiming: { periodMs: this.#s, discontinuity: o }
      });
    } else
      this.#e = requestAnimationFrame(this.#h);
  };
}
const xe = 0.5, m = 4, x = 5, X = 1e3, we = 4, U = 200, De = 0.25, Fe = 0.75, Re = 0.1, O = 1, be = 1e3 / 60, ge = 0.02, _e = 0.1, z = 1, M = 5, Me = 4, H = 1001 / 3e4, y = 3, ye = 60, Se = 3, Be = {
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
class ze {
  canvas;
  #t;
  #l;
  #e;
  /** The pulldown detection, which keeps its measurements on the GPU. */
  #n;
  #T;
  #A;
  /** The program that copies a filtered picture onto the canvas. */
  #a;
  #p;
  #m;
  #s = [];
  /** Somewhere to filter a field into, and to read it back out of. */
  #r = [];
  /** Which output slot was written last; the next one follows round the ring. */
  #o = x - 1;
  /** Filtered fields waiting for their moment, oldest first. */
  #i = [];
  /** The last picture scheduled, shown or not; the next follows on from it. */
  #h = null;
  /** The rAF loop that puts them up, which is all that draws on the canvas. */
  #x = null;
  /** When the loop last ran, and the refresh grid fitted to it. See #measureRefresh. */
  #F = 0;
  #w = be;
  #I = 0;
  /** How far ahead of its moment the last picture was shown. See #present. */
  #Ie = 0;
  /** What makes the pictures between pictures, and the last picture put up. */
  #N = null;
  #R = null;
  /** The pair the interpolator was last handed. */
  #le = null;
  /** What puts the pictures up at a size of its own, and the size it chose. */
  #B = null;
  #se = null;
  /** The size the canvas is shown at, in device pixels. */
  #W = null;
  /** A picture-sized texture to filter or interpolate into for the scaler. */
  #M = null;
  /** The texture last put up, to put up again when the canvas is resized. */
  #_ = null;
  #ue;
  /** Debug only: the rAF the last picture was drawn in, and repeats dropped since. */
  #we = 0;
  #De = 0;
  /** The `<div>` this put around the element, so it can be taken away again. */
  #Z = null;
  #Fe;
  #$;
  #Ne;
  #G;
  #P;
  /** How long a frame lasts in wall time, from what the frames themselves say. */
  #u = 0;
  /** The size of a frame as it is coded, which is what a texture holds. */
  #c = 0;
  #E = 0;
  /** Where the newest frame is. The two before it follow round the ring. */
  #D = m - 1;
  /** How many of the held frames are consecutive, up to HISTORY. */
  #f = 0;
  #k = 0;
  #b = !1;
  #Re = !1;
  #d = null;
  #Ae = [];
  #C = !1;
  #be;
  /** Everything the next report is counted from. See DeinterlaceStats. */
  #g = {
    filtered: 0,
    missed: 0,
    degraded: 0,
    discontinuities: 0,
    late: 0,
    resynced: 0,
    queueResetted: 0
  };
  /** `presentedFrames` of the last frame the callback saw; 0 before any. */
  #fe = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #ge = 0;
  #de = 0;
  #X = 0;
  #re = 0;
  #L = 0;
  #Q = 0;
  #V = 0;
  #Y;
  #pe = [];
  #j = [];
  #ne = 0;
  #K = 0;
  #oe = 0;
  #J = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #q = D;
  #O = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #v = D;
  #z = !1;
  /**
   * Where the cycle was when a frame was last seen locked to it. A broadcast
   * can cut between soft pulldown, which is progressive pictures with repeat
   * flags, and the same film laid down as interlaced frames, every few seconds
   * and without losing its place in the cycle, so this outlives those cuts.
   */
  #y = null;
  /** How many of the coming frames take their phase from `#cadence`. */
  #S = 0;
  /**
   * Whether the frames being seeded follow a missed frame rather than
   * progressive pictures, so that the first of them is not one of those.
   */
  #he = !1;
  #ee = null;
  #Ge = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #me = [0, 0, 0, 0, 0, 0];
  #_e = 0;
  constructor(e, t = {}) {
    this.#t = e, this.#$ = t.doubleRate ?? !1, this.#Ne = t.spatialCheck ?? !0, this.#G = t.debug ?? !1, this.#N = t.interpolator ?? null, this.#B = t.scaler ?? null, this.#P = t.film ?? !1, this.#be = t.onStats, this.canvas = document.createElement("canvas"), this.canvas.style.cssText = "position:absolute;pointer-events:none;visibility:hidden";
    const i = this.canvas.getContext("webgl2", {
      alpha: t.alpha ?? !1,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!i) throw new Error("this browser has no WebGL2");
    this.#e = i, this.#n = new pe(i), this.#T = R(i, de, b);
    const s = this.#T;
    this.#A = Object.fromEntries(
      Object.entries(fe).map(([A, n]) => [
        A,
        i.getUniformLocation(s, n)
      ])
    ), this.#a = R(i, Pe, b), this.#p = i.getUniformLocation(this.#a, "uField"), this.#m = i.getUniformLocation(this.#a, "uFlip"), this.#P && this.#ke(), this.#Y = i.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.canvas.addEventListener("webglcontextlost", this.#it), this.#Fe = new ResizeObserver(() => this.#xe()), this.#ue = new ResizeObserver(([A]) => {
      if (!A) return;
      const n = window.devicePixelRatio || 1, o = {
        width: Math.round(A.contentRect.width * n),
        height: Math.round(A.contentRect.height * n)
      }, a = A.devicePixelContentBoxSize?.[0];
      this.#W = a && Math.abs(a.inlineSize - o.width) <= 1 && Math.abs(a.blockSize - o.height) <= 1 ? { width: a.inlineSize, height: a.blockSize } : o, this.#Le();
    }), this.#l = new Te(e), e.addEventListener("emptied", this.#et), e.addEventListener("resize", this.#Ke), e.addEventListener("pause", this.#U), e.addEventListener("ended", this.#U), e.addEventListener("seeked", this.#U), e.addEventListener("ratechange", this.#U);
  }
  get running() {
    return this.#b && (this.#d?.interlaced ?? !0);
  }
  /** Whether the caller wants filtering, independently of the current source. */
  get enabled() {
    return this.#Re;
  }
  set enabled(e) {
    this.#Re = e, this.#ye();
  }
  /** Update whether the source needs filtering and which field comes first. */
  set scan(e) {
    this.#d = e, this.#ye();
  }
  get scan() {
    return this.#d;
  }
  set videoTimeline(e) {
    this.#Ae = e, e.length === 0 && (this.#d = null), this.#ye();
  }
  get videoTimeline() {
    return this.#Ae;
  }
  /**
   * What to put on the screen for fullscreen: the `<div>` holding both the
   * element and the canvas once there is one, and the element itself before
   * that. Fullscreening the element alone would leave the canvas behind in
   * the page, and with it the only deinterlaced picture there is.
   */
  get container() {
    return this.#Z ?? this.#t;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#$;
  }
  set doubleRate(e) {
    e !== this.#$ && (this.#$ = e, this.#Me());
  }
  get film() {
    return this.#P;
  }
  set film(e) {
    e !== this.#P && (e && this.#ke(), this.#P = e, e || (this.#z = !1, this.#v = D, this.#ie(), this.#H()), this.#Me());
  }
  get debug() {
    return this.#G;
  }
  set debug(e) {
    this.#G = e;
  }
  get interpolator() {
    return this.#N;
  }
  set interpolator(e) {
    e !== this.#N && (this.#N = e, this.#le = null, this.#Me());
  }
  get scaler() {
    return this.#B;
  }
  /**
   * Setting the same scaler again asks it for the size afresh, for a page
   * that changed something the size depends on that this cannot see.
   */
  set scaler(e) {
    this.#B = e, this.#Le();
  }
  /** Whether pictures are queued and put up by the loop rather than drawn on arrival. */
  get #te() {
    return this.#$ || this.#P || (this.#N?.wants("frame") ?? !1);
  }
  #Me() {
    this.#te ? (this.#c > 0 && this.#je(), this.#Be()) : (this.#Pe(), this.#ce());
  }
  #ke() {
    if (this.#e.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  #ye() {
    this.#Re && (this.#Ae.length > 0 || (this.#d?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  start() {
    this.#b || this.#C || (this.#b = !0, this.#tt(), this.#Et(), this.#Ee(), (this.#d?.interlaced ?? !0) && this.#Be());
  }
  /** Take the deinterlaced picture away, leaving the element's own showing. */
  stop() {
    this.#b && (this.#b = !1, this.#l.cancel(), this.#Pe(), this.#f = 0, this.canvas.style.visibility = "hidden");
  }
  destroy() {
    this.stop(), this.#l.destroy(), this.canvas.removeEventListener("webglcontextlost", this.#it), this.#t.removeEventListener("emptied", this.#et), this.#t.removeEventListener("resize", this.#Ke), this.#t.removeEventListener("pause", this.#U), this.#t.removeEventListener("ended", this.#U), this.#t.removeEventListener("seeked", this.#U), this.#t.removeEventListener("ratechange", this.#U), this.#vt();
    for (const e of this.#s) this.#e.deleteTexture(e);
    this.#s = [], this.#ce(), this.#Ue();
    for (const e of [
      ...this.#pe,
      ...this.#j.map(({ q: t }) => t)
    ])
      this.#e.deleteQuery(e);
    this.#pe.length = 0, this.#j.length = 0, this.#n.destroy(), this.#ee !== null && (ie(this.#ee), this.#ee = null), this.#e.deleteProgram(this.#T), this.#e.deleteProgram(this.#a), this.#e.getExtension("WEBGL_lose_context")?.loseContext();
  }
  #Ee() {
    this.#b && this.#l.request(this.#ot);
  }
  #st(e, t, i) {
    this.#ee == null && (this.#ee = ee(this.#e, "20px monospace")), se(this.#ee, e, t, i, this.#c, this.#E, 20);
  }
  #Xe(e) {
    if (this.#Y == null || this.#j.length > 30)
      return;
    const t = this.#pe.pop() ?? this.#e.createQuery();
    return this.#e.beginQuery(this.#Y.TIME_ELAPSED_EXT, t), this.#j.push({ q: t, isField: e }), t;
  }
  #Oe(e) {
    this.#Y != null && (e != null && this.#e.endQuery(this.#Y.TIME_ELAPSED_EXT), this.#j = this.#j.filter(
      ({ q: t, isField: i }) => {
        if (this.#e.getQueryParameter(t, this.#e.QUERY_RESULT_AVAILABLE)) {
          const s = this.#e.getQueryParameter(t, this.#e.QUERY_RESULT);
          return i ? (this.#oe += s, this.#J++) : (this.#ne += s, this.#K++), this.#pe.push(t), !1;
        }
        return !0;
      }
    ));
  }
  #ie() {
    this.#y = null, this.#S = 0, this.#he = !1;
  }
  /**
   * Give the frame being filtered its phase from the cycle seen before the
   * progressive pictures, if it is one of those that needs it.
   */
  #ze(e) {
    if (this.#S === 0 || this.#y === null) return;
    const t = e - this.#y.mediaTime;
    if (t < 0 || t > ye) {
      this.#ie();
      return;
    }
    const i = this.#S === y && !this.#he ? Se : (this.#y.phase - 1 + Math.round(t / H)) % M + 1;
    this.#q = this.#v = { phase: i, run: S }, this.#O = 0, this.#n.seed(i, S), this.#S--, this.#S === 0 && (this.#he = !1);
  }
  #H() {
    this.#v = D, this.#q = D, this.#O = 0, this.#z = !1, this.#n.reset();
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #At() {
    const { cur: e, next: t } = this.#Ve(!1), i = this.#s[e], s = this.#s[t];
    if (!i || !s) return;
    const A = this.#d?.topFieldFirst !== !1 ? 0 : 1;
    this.#n.detect(i, s, A);
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #rt() {
    const e = this.#n.poll();
    e !== null && (this.#q = e, this.#O = 0), this.#O++;
    const { phase: t, run: i } = this.#q;
    t === 0 || this.#O > M ? this.#v = D : this.#v = {
      phase: (t - 1 + this.#O) % M + 1,
      run: i
    };
  }
  #nt(e) {
    const t = [], i = this.#n.metrics;
    for (let A = 0; A < d.phase; A++) {
      const n = i[A * 4] ?? 0, o = i[A * 4 + 1] ?? 0, a = i[A * 4 + 2] ?? 0;
      t.push(
        `${n.toFixed(4)},${o.toString().padStart(4)},${a.toFixed(3)}`
      );
    }
    const s = this.#me.map((A, n) => `${n === 0 ? "-" : n}:${A}`).join(" ");
    return `frame=${e} phase=${this.#v.phase} run=${this.#v.run} known=${this.#q.phase}/${this.#q.run} age=${this.#O} ${this.#z ? "film" : "video"} period=${this.#u.toFixed(3)}
${t.join(" ")}
${s} dropped:${this.#_e}`;
  }
  /** Whether this frame is not the one that follows the last one seen. */
  #He(e) {
    if (e.mozTiming) return e.mozTiming.discontinuity;
    const t = e.mediaTime - this.#k;
    return t < 0 || t > xe;
  }
  #ot = (e, t) => {
    if (!(!this.#b || this.#C)) {
      if (this.#ht(t.mediaTime), t.width > 0 && t.height > 0) {
        if ((this.#c === 0 || this.#E === 0) && this.#Ye(t.width, t.height), this.#d && !this.#d.interlaced) {
          const h = this.#He(t);
          h && this.#ie(), this.#$e(t.presentedFrames, h), this.#k = t.mediaTime, this.#pt(), this.#Ee();
          return;
        }
        const i = t.mediaTime - this.#k, s = this.#He(t);
        s && (this.#ie(), this.#f = 0, this.#g.discontinuities++, this.#i.length = 0, this.#h = null, this.#R = null, this.#H());
        const A = this.#$e(t.presentedFrames, s);
        if (!t.mozTiming && this.#f > 0 && t.mediaTime === this.#k) {
          this.#Ee();
          return;
        }
        t.mozTiming ? this.#u = t.mozTiming.periodMs : !s && i > 0 && this.#at(i);
        const n = this.#k;
        this.#k = t.mediaTime;
        const o = performance.now();
        o - this.#ge > X && (this.#de = o, this.#X = 0, this.#re = 0, this.#L = 0, this.#Q = 0, this.#V = 0, this.#ne = 0, this.#K = 0, this.#oe = 0, this.#J = 0), this.#ge = o;
        const a = performance.now(), c = this.#Xe(!1);
        if (this.#Qe(), this.#V = Math.max(
          this.#V,
          this.#i.length
        ), this.#P && (this.#f === m && A === 0 ? (this.#rt(), this.#At(), this.#ze(t.mediaTime)) : A > 0 && this.#f === m && this.#y !== null ? (this.#H(), this.#S !== y && (this.#S = y, this.#he = !0), this.#ze(n + H)) : this.#H(), this.#me[this.#v.phase] = (this.#me[this.#v.phase] ?? 0) + 1, this.#z = this.#v.phase !== 0 && this.#v.run >= S, this.#z && this.#S === 0 && (this.#y = {
          mediaTime: t.mediaTime,
          phase: this.#v.phase
        }), this.#G && (this.#Ge = this.#nt(t.presentedFrames))), this.#We() && !this.#t.paused && !this.#t.ended) {
          this.#i.length >= x && (this.#i.length = 0, this.#h = null, this.#R = null, this.#g.queueResetted++);
          const h = (t.expectedDisplayTime || e) + this.#w;
          if (this.#z) {
            const u = this.#v.phase;
            if (u === N)
              this.#_e++, this.#De++;
            else {
              const f = this.#u * M / Me, l = Be[u] ?? 0, p = this.#Se(
                "film",
                h + l * this.#u,
                f
              );
              this.#ve("film", u, !1, p, f);
            }
          } else if (this.#$) {
            const u = this.#u / 2, f = this.#Se("field", h, u);
            this.#ve("field", 1, !1, f, u), this.#ve("field", 2, !0, f + u, u);
          } else {
            const u = this.#u, f = this.#Se("frame", h, u);
            this.#ve("frame", 0, !1, f, u);
          }
        } else
          this.#Te(this.#t.paused || this.#t.ended, !1, null);
        this.#Oe(c), this.#re += performance.now() - a, this.#X++, this.#mt(o);
      }
      this.#Ee();
    }
  };
  #ht(e) {
    let t;
    for (let A = this.#Ae.length - 1; A >= 0; A--) {
      const n = this.#Ae[A];
      if (n.start <= e + 1e-6) {
        t = n;
        break;
      }
    }
    t?.codedSize && (t.codedSize.width !== this.#c || t.codedSize.height !== this.#E) && this.#Ye(t.codedSize.width, t.codedSize.height);
    const i = t?.scan;
    if (!i || this.#d?.interlaced === i.interlaced && this.#d.topFieldFirst === i.topFieldFirst)
      return;
    const s = this.#d?.interlaced === !1;
    this.#d = i, this.#y === null && (this.#f = 0), this.#i.length = 0, this.#h = null, this.#R = null, this.#H(), i.interlaced && s && this.#y !== null && (this.#S = y, this.#he = !1), i.interlaced ? this.#te && this.#Be() : this.#Pe();
  }
  /**
   * Whether pictures are being filtered ahead of time and queued, rather than
   * drawn as their frame arrives.
   *
   * A picture for every frame has nothing to schedule -- there is one of them
   * and it goes up now -- and neither has a filter that has yet to see two
   * frames go by, since until then there is no idea how long a frame lasts.
   */
  #We() {
    return this.#te && this.#u > 0 && this.#r.length === x;
  }
  /**
   * When a picture goes up: one duration after the last one of its cadence,
   * nudged towards `ideal` by a fraction of the gap so that the schedule
   * follows the clock without a picture ever moving across a refresh. It
   * restarts from `ideal` when the gap has grown to a whole picture or the
   * cadence has changed.
   */
  #Se(e, t, i) {
    const s = this.#h;
    if (s !== null && s.cadence === e) {
      const A = s.at + s.duration, n = t - A;
      if (Math.abs(n) < i) {
        const o = Math.max(
          -O,
          Math.min(O, n * Re)
        );
        return A + o;
      }
    }
    s !== null && this.#g.resynced++;
    for (let A = this.#i.at(-1); A && A.at >= t; )
      this.#i.pop(), this.#g.late++, A = this.#i.at(-1);
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
  #at(e) {
    const t = e * 1e3 / (this.#t.playbackRate || 1), i = this.#u > 0 ? Math.max(1, Math.round(t / this.#u)) : 1, s = t / i;
    s < we || s > U || (this.#u = this.#u > 0 && s > this.#u * Fe ? this.#u + (s - this.#u) * De : s);
  }
  /**
   * Filter one field into an output texture and put it in the queue.
   *
   * The three frames the filter reads are only the right three between one
   * frame arriving and the next, so both fields of a frame are built here and
   * held as pictures. What is queued after that is a copy waiting for a
   * moment, which no later frame can take away.
   */
  #ve(e, t, i, s, A) {
    const n = (this.#o + 1) % x, o = this.#r[n];
    if (!o) return;
    for (this.#o = n, this.#R?.slot === n && (this.#R = null); this.#i.length > 0 && this.#i[0]?.slot === n; )
      this.#i.shift(), this.#g.late++;
    this.#Te(!1, i, o.framebuffer);
    const a = {
      slot: n,
      at: s,
      duration: A,
      cadence: e,
      phase: t,
      droppedBefore: this.#De
    };
    this.#De = 0, this.#i.push(a), this.#h = a;
  }
  /** The loop that puts filtered fields up, and the only thing that draws. */
  #Be() {
    this.#x === null && (!this.#b || this.#C || !this.#te || (this.#x = requestAnimationFrame(this.#Ze)));
  }
  #Pe() {
    this.#x !== null && cancelAnimationFrame(this.#x), this.#x = null, this.#i.length = 0, this.#h = null, this.#R = null;
  }
  #Ze = (e) => {
    this.#x = null, !(!this.#b || this.#C || !this.#te) && (this.#ct(e), this.#lt(this.#I, e), this.#x = requestAnimationFrame(this.#Ze));
  };
  /**
   * Fit a grid of refreshes (period and phase) to the animation frames. rAF
   * timestamps wander by a millisecond or so, which is more than the
   * nearest-refresh decision in #present can take; the grid is what it
   * compares against. A frame far off the grid restarts it.
   */
  #ct(e) {
    const t = e - this.#F;
    this.#F = e;
    const i = Math.max(1, Math.round(t / this.#w)), s = this.#I + i * this.#w, A = e - s;
    if (this.#I === 0 || t <= 0 || t > U || Math.abs(A) > this.#w / 4) {
      t > 0 && t <= U && (this.#w = t), this.#I = e;
      return;
    }
    this.#w += A / i * ge, this.#I = s + A * _e;
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
  #lt(e, t) {
    const i = this.#w / 2, s = (a) => {
      const c = a.at + this.#Ce(a) - e;
      return c <= i - z ? !0 : c > i + z ? !1 : this.#Ie > 0;
    };
    for (; this.#i[1] && s(this.#i[1]); )
      this.#g.late++, this.#i.shift();
    const A = this.#i[0];
    if (!A || !s(A)) {
      A && this.#ut(A, e);
      return;
    }
    this.#i.shift(), this.#Ie = A.at + this.#Ce(A) - e, this.#R = A;
    const n = performance.now(), o = this.#Xe(!0);
    this.#dt(A.slot), this.#Oe(o), this.#Q += performance.now() - n, this.#L++, this.#G && this.#ft(A, t), this.#we = t;
  }
  /** How late a picture goes up: one picture, when the pictures after it are made. */
  #Ce(e) {
    return this.#N?.wants(e.cadence) ? e.duration : 0;
  }
  /**
   * Put up a picture between the last one and the next, at the moment this
   * refresh shows. Nothing is drawn where the two are not neighbours: a new
   * cadence, a gap, or a moment outside them.
   */
  #ut(e, t) {
    const i = this.#N, s = this.#R;
    if (!i || !s || s.cadence !== e.cadence || !i.wants(e.cadence))
      return;
    const A = e.at - s.at;
    if (A <= 0 || A > s.duration * 1.5) return;
    const n = (t - this.#Ce(s) - s.at) / A;
    if (n <= 0 || n >= 1) return;
    const o = this.#r[s.slot], a = this.#r[e.slot];
    if (!o || !a) return;
    const c = this.#e;
    (this.#le?.previous !== s || this.#le.next !== e) && (i.pair(c, o.texture, a.texture, this.#c, this.#E), this.#le = { previous: s, next: e });
    const h = performance.now(), u = this.#se ? this.#qe() : null, f = u?.framebuffer ?? null;
    c.bindFramebuffer(c.FRAMEBUFFER, f), i.draw(c, n, this.#c, this.#E, f) && (u ? this.#ae(u.texture) : this.#_ = null, this.#Q += performance.now() - h, this.#L++, this.canvas.style.visibility = "visible");
  }
  #ft(e, t) {
    const i = this.#we === 0 ? 0 : t - this.#we, s = i / this.#w, A = e.cadence === "film" ? `phase ${e.phase}` : e.cadence === "field" ? `field ${e.phase}` : "frame", n = e.droppedBefore > 0 ? `, phase ${N} dropped before it` + (e.droppedBefore > 1 ? ` (${e.droppedBefore})` : "") : "";
    console.log(
      `yadif: +${i.toFixed(2)} ms (${s.toFixed(2)} refreshes) ${e.cadence} ${A}, due ${(e.at - t).toFixed(2)} ms${n}`
    );
  }
  /** Copy one of the filtered pictures onto the canvas. */
  #dt(e) {
    const t = this.#r[e];
    t && this.#ae(t.texture);
  }
  /** Put a progressive frame through unchanged, keeping one display surface. */
  #pt() {
    this.#Qe();
    const e = this.#s[this.#D];
    e && this.#ae(e, !0), this.#y === null && (this.#f = 0);
  }
  #ae(e, t = !1) {
    const i = this.#e;
    this.#_ = { texture: e, flipped: t }, i.bindFramebuffer(i.FRAMEBUFFER, null);
    const s = this.#B, A = this.#se;
    if (s && A) {
      let n = !1;
      try {
        n = s.draw(
          i,
          e,
          t,
          this.#c,
          this.#E,
          A.width,
          A.height
        );
      } catch (o) {
        console.warn("yadif: the scaler failed; showing pictures as they are", o);
      }
      if (n) {
        this.canvas.style.visibility = "visible";
        return;
      }
      this.#Je(), i.bindFramebuffer(i.FRAMEBUFFER, null);
    }
    i.useProgram(this.#a), i.activeTexture(i.TEXTURE0), i.bindTexture(i.TEXTURE_2D, e), i.uniform1i(this.#p, 0), i.uniform1i(this.#m, t ? 1 : 0), i.viewport(0, 0, this.#c, this.#E), i.drawArrays(i.TRIANGLES, 0, 3), this.canvas.style.visibility = "visible";
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
  #$e(e, t) {
    let i = 0;
    return this.#fe !== 0 && !t && (i = Math.max(0, e - this.#fe - 1), this.#g.missed += i), this.#fe = e, i;
  }
  #mt(e) {
    if (!this.#be) return;
    const t = e - this.#de;
    if (t < X) return;
    const i = this.#We() ? this.#L : this.#X;
    let s = 0;
    this.#X !== 0 && (s += this.#re / this.#X), this.#L !== 0 && (s += this.#Q / this.#L / 2);
    let A;
    this.#Y != null && (A = 0, this.#K !== 0 && (A += this.#ne / 1e6 / this.#K), this.#J !== 0 && (A += this.#oe / 1e6 / this.#J / 2)), this.#be({
      ...this.#g,
      // The element's own count of what its decoder could not keep up with,
      // which is the machine being behind rather than this filter.
      dropped: this.#t.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
      fps: i * 1e3 / t,
      frameMs: s,
      maxQueuedFields: this.#V,
      gpuMs: A,
      film: this.#z
    }), this.#de = e, this.#X = 0, this.#re = 0, this.#L = 0, this.#Q = 0, this.#V = 0, this.#ne = 0, this.#K = 0, this.#oe = 0, this.#J = 0;
  }
  /** Take the newest frame into the ring. */
  #Qe() {
    const e = this.#e;
    this.#D = (this.#D + 1) % m, e.bindTexture(e.TEXTURE_2D, this.#s[this.#D] ?? null), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      e.RGBA,
      e.UNSIGNED_BYTE,
      this.#t
    ), this.#f = Math.min(this.#f + 1, m);
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
  #Te(e, t, i) {
    if (this.#f === 0 || this.#C) return;
    const s = i === null && this.#se ? this.#qe() : null;
    s && (i = s.framebuffer), this.#f === m && !e ? this.#g.filtered++ : this.#g.degraded++;
    const A = this.#e, { prev: n, cur: o, next: a } = this.#Ve(e);
    A.bindFramebuffer(A.FRAMEBUFFER, i), A.useProgram(this.#T);
    for (const [f, l] of [n, o, a].entries())
      A.activeTexture(A.TEXTURE0 + f), A.bindTexture(A.TEXTURE_2D, this.#s[l] ?? null);
    A.uniform1i(this.#A.prev, 0), A.uniform1i(this.#A.cur, 1), A.uniform1i(this.#A.next, 2);
    const c = this.#P ? this.#n.texture : null, h = c !== null;
    c !== null && (A.activeTexture(A.TEXTURE0 + 3), A.bindTexture(A.TEXTURE_2D, c), A.uniform1i(this.#A.fieldMetrics, 3)), A.uniform2i(this.#A.size, this.#c, this.#E);
    const u = this.#d?.topFieldFirst !== !1 ? 0 : 1;
    A.uniform1i(this.#A.parity, t ? 1 - u : u), A.uniform1i(this.#A.second, t ? 1 : 0), A.uniform1i(
      this.#A.tff,
      this.#d?.topFieldFirst !== !1 ? 1 : 0
    ), A.uniform1i(this.#A.spatialCheck, this.#Ne ? 1 : 0), A.uniform1i(this.#A.debug, this.#G ? 1 : 0), A.uniform1i(this.#A.film, h ? 1 : 0), A.uniform1i(this.#A.phase, this.#v.phase), A.viewport(0, 0, this.#c, this.#E), A.drawArrays(A.TRIANGLES, 0, 3), this.#G && h && this.#st(this.#Ge, 0, 90), s ? this.#ae(s.texture) : i === null && (this.#_ = null, this.canvas.style.visibility = "visible");
  }
  #Ve(e) {
    const t = (i) => (this.#D + m - i) % m;
    return this.#f === 1 ? { prev: this.#D, cur: this.#D, next: this.#D } : e ? { prev: t(1), cur: this.#D, next: this.#D } : this.#f === 2 ? { prev: t(1), cur: t(1), next: this.#D } : { prev: t(2), cur: t(1), next: this.#D };
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
  #xe() {
    if (!this.#Z) return;
    const e = this.#t, t = e.videoWidth, i = e.videoHeight;
    if (t === 0 || i === 0) return;
    const s = Math.min(
      e.offsetWidth / t,
      e.offsetHeight / i
    ), A = t * s, n = i * s;
    this.canvas.style.left = `${e.offsetLeft + (e.offsetWidth - A) / 2}px`, this.canvas.style.top = `${e.offsetTop + (e.offsetHeight - n) / 2}px`, this.canvas.style.width = `${A}px`, this.canvas.style.height = `${n}px`;
  }
  #Ye(e, t) {
    const i = this.#e;
    this.#c = e, this.#E = t, this.#f = 0, this.#_ = null, this.#Ue(), this.#Le(), this.#xe();
    for (const s of this.#s) i.deleteTexture(s);
    this.#s = [];
    for (let s = 0; s < m; s++) {
      const A = i.createTexture();
      i.bindTexture(i.TEXTURE_2D, A), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MIN_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MAG_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_S, i.CLAMP_TO_EDGE), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_T, i.CLAMP_TO_EDGE), i.texImage2D(
        i.TEXTURE_2D,
        0,
        i.RGBA,
        e,
        t,
        0,
        i.RGBA,
        i.UNSIGNED_BYTE,
        null
      ), this.#s.push(A);
    }
    this.#ce(), this.#n.resize(e, t), this.#te && this.#je();
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
  #je() {
    const e = this.#e;
    if (!(this.#r.length === x || this.#c === 0)) {
      this.#ce();
      for (let t = 0; t < x; t++) {
        const i = e.createTexture();
        e.bindTexture(e.TEXTURE_2D, i), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
          e.TEXTURE_2D,
          0,
          e.RGBA,
          this.#c,
          this.#E,
          0,
          e.RGBA,
          e.UNSIGNED_BYTE,
          null
        );
        const s = e.createFramebuffer();
        e.bindFramebuffer(e.FRAMEBUFFER, s), e.framebufferTexture2D(
          e.FRAMEBUFFER,
          e.COLOR_ATTACHMENT0,
          e.TEXTURE_2D,
          i,
          0
        );
        const A = e.checkFramebufferStatus(e.FRAMEBUFFER) === e.FRAMEBUFFER_COMPLETE;
        if (e.bindFramebuffer(e.FRAMEBUFFER, null), !A) {
          e.deleteFramebuffer(s), e.deleteTexture(i), this.#ce();
          return;
        }
        this.#r.push({ texture: i, framebuffer: s });
      }
      this.#o = x - 1;
    }
  }
  #ce() {
    const e = this.#e;
    for (const { texture: t, framebuffer: i } of this.#r)
      e.deleteFramebuffer(i), e.deleteTexture(t);
    this.#r = [], this.#i.length = 0, this.#h = null, this.#R = null;
  }
  /**
   * Wrap the element in a `<div>` of this one's own and put the canvas over
   * it. The wrapper is what the canvas is positioned against; moving the
   * element out of the tree and back within the one task leaves playback
   * alone, which is what makes turning this on mid-stream free.
   */
  #Et() {
    if (this.#Z) return;
    const e = this.#t.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#t), t.appendChild(this.#t), t.appendChild(this.canvas), this.#Z = t, this.#Fe.observe(this.#t);
    try {
      this.#ue.observe(this.canvas, { box: "device-pixel-content-box" });
    } catch {
      this.#ue.observe(this.canvas);
    }
    this.#xe();
  }
  #vt() {
    const e = this.#Z;
    this.#Z = null, this.#Fe.disconnect(), this.#ue.disconnect(), this.canvas.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#t, e), e.remove());
  }
  #Ke = () => this.#xe();
  /**
   * Size the canvas's buffer for what puts the pictures up: the picture's own
   * size for the copy, or whatever the scaler asks for. Resizing the buffer
   * clears it, so the last picture goes up again.
   */
  #Le() {
    const e = this.#c, t = this.#E;
    if (e === 0 || t === 0 || this.#C) return;
    let i = null;
    if (this.#B && this.#W && this.#W.width > 0 && this.#W.height > 0)
      try {
        i = this.#B.size(
          this.#e,
          e,
          t,
          this.#W.width,
          this.#W.height
        );
      } catch (n) {
        console.warn("yadif: the scaler failed; showing pictures as they are", n), this.#B = null;
      }
    this.#se = i;
    const s = i ?? { width: e, height: t }, A = this.canvas.width !== s.width || this.canvas.height !== s.height;
    A && (this.canvas.width = s.width, this.canvas.height = s.height), (A || i) && this.#Tt(), i || (this.#_ && this.#_.texture === this.#M?.texture && (this.#_ = null), this.#Ue());
  }
  /** Put the last picture up again, on a canvas that has just been cleared. */
  #Tt() {
    !this.#b || this.#C || (this.#_ ? this.#ae(this.#_.texture, this.#_.flipped) : (this.#t.paused || this.#t.ended) && this.#Te(!0, !1, null));
  }
  /** Let go of the scaler that failed, and copy pictures as they are again. */
  #Je() {
    this.#B = null, this.#se = null, this.canvas.width = this.#c, this.canvas.height = this.#E;
  }
  /** The picture-sized texture the scaler reads, made when it is first wanted. */
  #qe() {
    if (this.#M) return this.#M;
    const e = this.#e, t = e.createTexture();
    e.bindTexture(e.TEXTURE_2D, t), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      this.#c,
      this.#E,
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
      t,
      0
    );
    const s = e.checkFramebufferStatus(e.FRAMEBUFFER) === e.FRAMEBUFFER_COMPLETE;
    return e.bindFramebuffer(e.FRAMEBUFFER, null), s ? (this.#M = { texture: t, framebuffer: i }, this.#M) : (e.deleteFramebuffer(i), e.deleteTexture(t), this.#Je(), null);
  }
  #Ue() {
    this.#M && (this.#e.deleteFramebuffer(this.#M.framebuffer), this.#e.deleteTexture(this.#M.texture), this.#M = null);
  }
  #et = () => {
    this.#f = 0, this.#k = 0, this.#i.length = 0, this.#h = null, this.#R = null, this.#ie(), this.#H(), this.#u = 0, this.#tt(), this.canvas.style.visibility = "hidden";
  };
  #tt() {
    this.#g = {
      filtered: 0,
      missed: 0,
      degraded: 0,
      discontinuities: 0,
      late: 0,
      resynced: 0,
      queueResetted: 0
    }, this.#me.fill(0), this.#_e = 0, this.#fe = 0, this.#de = 0, this.#ge = 0, this.#X = 0, this.#re = 0, this.#L = 0, this.#Q = 0, this.#V = 0, this.#ne = 0, this.#K = 0, this.#oe = 0, this.#J = 0;
  }
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #U = () => {
    this.#ie(), this.#i.length = 0, this.#h = null, this.#R = null, this.#H(), this.#b && this.#Te(!0, !1, null);
  };
  /**
   * A lost context takes the textures and the program with it. Rebuilding
   * them is possible, but a page that has lost its context has bigger
   * problems; getting out of the way leaves the element's own picture showing.
   */
  #it = (e) => {
    e.preventDefault(), this.#C = !0, this.stop();
  };
}
const W = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", Ce = 0.5, Le = 3e3, Z = 0.1, F = 16, $ = 'video/mp4; codecs="avc1.640029"';
let G = null;
function Ue(r = {}) {
  return G ??= Ie(r), G;
}
async function He(r = {}) {
  return (await Ue(r)).deinterlaces;
}
function We() {
  G = null;
}
async function Ie(r) {
  const e = r.tolerance ?? Ce, t = r.timeoutMs ?? Le, i = performance.now(), s = (o) => ({
    deinterlaces: !1,
    survives: null,
    tookMs: performance.now() - i,
    error: o instanceof Error ? o.message : String(o)
  });
  if (typeof document > "u")
    return s(new Error("there is no document to decode in"));
  const A = document.createElement("video");
  A.muted = !0, A.defaultMuted = !0, A.playsInline = !0, A.preload = "auto";
  let n = null;
  try {
    n = Ge(A, t);
    const o = C(P(A, "loadeddata"), t), a = A.play().then(
      () => !0,
      () => !1
    );
    if (await n.ready, await o, await ke(A, t, await a), A.videoWidth === 0 || A.videoHeight === 0)
      return s(new Error("the probe clip decoded to nothing"));
    const c = Xe(A);
    return {
      deinterlaces: c < 1 - e,
      survives: c,
      tookMs: performance.now() - i
    };
  } catch (o) {
    return s(o);
  } finally {
    A.pause(), A.removeAttribute("src"), A.replaceChildren(), A.load(), n && URL.revokeObjectURL(n.url);
  }
}
const I = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, Ne = typeof MediaSource > "u";
function Ge(r, e) {
  if (!I || !I.isTypeSupported($))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = W.indexOf(","), i = atob(W.slice(t + 1)), s = new Uint8Array(i.length);
  for (let a = 0; a < i.length; a++) s[a] = i.charCodeAt(a);
  const A = new I(), n = URL.createObjectURL(A);
  if (Ne) {
    r.disableRemotePlayback = !0;
    const a = document.createElement("source");
    a.type = "video/mp4", a.src = n, r.append(a), r.load();
  } else
    r.src = n;
  const o = (async () => {
    await C(P(A, "sourceopen"), e);
    const a = A.addSourceBuffer($), c = C(P(a, "updateend"), e);
    a.appendBuffer(s), await c, A.endOfStream();
  })();
  return { url: n, ready: o };
}
async function ke(r, e, t) {
  if (t) {
    const i = performance.now();
    for (; r.currentTime < Z && performance.now() - i < e; )
      await new Promise((s) => requestAnimationFrame(s));
    r.pause();
  } else
    r.currentTime = Z, await C(P(r, "seeked"), e);
}
function Xe(r) {
  const e = r.videoHeight, t = document.createElement("canvas");
  t.width = F, t.height = e;
  const i = t.getContext("2d", { willReadFrequently: !0 });
  if (!i) throw new Error("there is no 2d context to read the clip with");
  i.imageSmoothingEnabled = !1, i.drawImage(r, 0, 0, F, e);
  const s = i.getImageData(0, 0, F, e).data, A = (h) => {
    let u = 0;
    for (let f = 0; f < F; f++)
      u += s[(h * F + f) * 4 + 1] ?? 0;
    return u / F;
  };
  let n = 0;
  const o = 2, a = e - 3;
  let c = A(o);
  for (let h = o + 1; h <= a; h++) {
    const u = A(h);
    n += Math.abs(u - c), c = u;
  }
  return n / (a - o) / 255;
}
function P(r, e) {
  return new Promise((t, i) => {
    r.addEventListener(e, () => t(), { once: !0 }), r.addEventListener(
      "error",
      () => {
        const s = r instanceof HTMLMediaElement ? r.error : null, A = s ? ` (MediaError ${s.code}${s.message ? `: ${s.message}` : ""})` : "";
        i(new Error(`the probe clip ${e} failed${A}`));
      },
      { once: !0 }
    );
  });
}
function C(r, e) {
  return Promise.race([
    r,
    new Promise(
      (t, i) => setTimeout(
        () => i(new Error("the probe clip took too long")),
        e
      )
    )
  ]);
}
export {
  ze as Deinterlacer,
  de as YADIF_FRAGMENT_SHADER,
  fe as YADIF_UNIFORMS,
  He as decoderDeinterlaces,
  We as forgetDecoderProbe,
  Ue as probeDecoder,
  Oe as supportsDeinterlace
};
