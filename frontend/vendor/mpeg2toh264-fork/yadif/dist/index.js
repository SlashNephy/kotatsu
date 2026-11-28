const Fe = "" + new URL("assets/worker-BgcJM-Cv.js", import.meta.url).href, S = `#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;
function R(n, e, t) {
  const i = n.createProgram(), s = K(n, n.VERTEX_SHADER, t), r = K(n, n.FRAGMENT_SHADER, e);
  if (n.attachShader(i, s), n.attachShader(i, r), n.linkProgram(i), n.deleteShader(s), n.deleteShader(r), !n.getProgramParameter(i, n.LINK_STATUS)) {
    const o = n.getProgramInfoLog(i);
    throw n.deleteProgram(i), new Error(
      `the deinterlacer failed to link: ${o ?? "no reason given"}`
    );
  }
  return i;
}
function y(n, e, t, i = 1) {
  const s = n.createTexture();
  n.activeTexture(n.TEXTURE0), n.bindTexture(n.TEXTURE_2D, s), n.texStorage2D(n.TEXTURE_2D, 1, n.RGBA8, 1, 1), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MIN_FILTER, n.NEAREST), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MAG_FILTER, n.NEAREST);
  const r = n.createFramebuffer();
  n.bindFramebuffer(n.FRAMEBUFFER, r);
  const o = [];
  for (let h = 0; h < i; h++) {
    const a = n.createTexture();
    n.bindTexture(n.TEXTURE_2D, a), n.texStorage2D(n.TEXTURE_2D, 1, t, 1, 1), n.framebufferTexture2D(
      n.FRAMEBUFFER,
      n.COLOR_ATTACHMENT0 + h,
      n.TEXTURE_2D,
      a,
      0
    ), o.push(a);
  }
  n.drawBuffers(o.map((h, a) => n.COLOR_ATTACHMENT0 + a)), n.bindTexture(n.TEXTURE_2D, s), n.useProgram(e), n.viewport(0, 0, 1, 1), n.drawArrays(n.TRIANGLES, 0, 3), n.bindFramebuffer(n.FRAMEBUFFER, null), n.deleteFramebuffer(r);
  for (const h of o) n.deleteTexture(h);
  n.deleteTexture(s);
}
function K(n, e, t) {
  const i = n.createShader(e);
  if (!i) throw new Error("the deinterlacer could not create a shader");
  if (n.shaderSource(i, t), n.compileShader(i), !n.getShaderParameter(i, n.COMPILE_STATUS)) {
    const s = n.getShaderInfoLog(i);
    throw n.deleteShader(i), new Error(
      `the deinterlacer failed to compile: ${s ?? "no reason given"}`
    );
  }
  return i;
}
const Re = `#version 300 es
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
`, ye = `#version 300 es
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
function De(n, e) {
  const t = R(n, Re, ye), i = n.getAttribLocation(t, "aVertexPosition"), s = n.getAttribLocation(t, "aTextureCoord"), r = n.getUniformLocation(t, "uTexture"), o = n.getUniformLocation(t, "uMatrix"), h = n.getUniformLocation(t, "uUvMatrix"), a = n.getUniformLocation(t, "uTextColor"), u = n.getUniformLocation(t, "uBackColor");
  if (r == null || o == null || h == null || a == null || u == null)
    throw new Error(
      "failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER"
    );
  const A = n.createBuffer(), f = n.createBuffer();
  return {
    gl: n,
    ..._e(n, e),
    program: t,
    programUniforms: {
      vertex: i,
      textureCoord: s,
      texture: r,
      matrix: o,
      uvMatrix: h,
      textColor: a,
      backColor: u
    },
    positionBuffer: A,
    textureBuffer: f
  };
}
function _e(n, e) {
  const t = new OffscreenCanvas(0, 0), i = t.getContext("2d"), s = /* @__PURE__ */ new Map();
  let r = 0;
  const o = 0;
  let h = 1;
  i.font = e, i.fillStyle = "white";
  for (let u = 32; u < 128; u++) {
    const A = String.fromCharCode(u), f = i.measureText(A), d = Math.ceil(
      f.actualBoundingBoxDescent + f.actualBoundingBoxAscent + 1
    ), l = Math.ceil(
      f.actualBoundingBoxLeft + f.actualBoundingBoxRight + 1
    );
    s.set(A, {
      x: r,
      y: o,
      width: l,
      height: d,
      metrics: f
    }), h = Math.max(h, d), r += l;
  }
  t.width = r, t.height = h, i.font = e, i.fillStyle = "white";
  for (const [u, A] of s)
    i.fillText(
      u,
      Math.floor(A.x + A.metrics.actualBoundingBoxLeft + 1),
      Math.floor(A.metrics.actualBoundingBoxAscent + 1)
    );
  const a = n.createTexture();
  return n.bindTexture(n.TEXTURE_2D, a), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MIN_FILTER, n.LINEAR), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MAG_FILTER, n.LINEAR), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_WRAP_S, n.CLAMP_TO_EDGE), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_WRAP_T, n.CLAMP_TO_EDGE), n.texImage2D(n.TEXTURE_2D, 0, n.RGBA, n.RGBA, n.UNSIGNED_BYTE, t), { fontTexture: a, chars: s, textureSize: { width: r, height: h } };
}
function Me(n) {
  const e = n.gl;
  e.deleteBuffer(n.positionBuffer), e.deleteBuffer(n.textureBuffer), e.deleteTexture(n.fontTexture), e.deleteProgram(n.program);
}
function Ne(n, e, t, i, s, r, o) {
  const h = [], a = [], u = t;
  for (const d of e) {
    if (d === `
`) {
      t = u, i += o;
      continue;
    }
    const l = n.chars.get(d);
    if (l == null)
      continue;
    if (l.width === 1) {
      t += l.metrics.width;
      continue;
    }
    const c = Math.floor(t - l.metrics.actualBoundingBoxLeft), m = Math.floor(i - l.metrics.actualBoundingBoxAscent), E = c + l.width, v = m + l.height;
    h.push(c, m), a.push(l.x, l.y), h.push(c, v), a.push(l.x, l.y + l.height), h.push(c + l.width, v), a.push(l.x + l.width, l.y + l.height), h.push(E, v), a.push(l.x + l.width, l.y + l.height), h.push(c, m), a.push(l.x, l.y), h.push(E, m), a.push(l.x + l.width, l.y), t += l.metrics.width;
  }
  const A = n.gl;
  A.useProgram(n.program), A.bindBuffer(A.ARRAY_BUFFER, n.positionBuffer), A.bufferData(A.ARRAY_BUFFER, new Float32Array(h), A.STATIC_DRAW), A.vertexAttribPointer(
    n.programUniforms.vertex,
    2,
    A.FLOAT,
    !1,
    0,
    0
  ), A.enableVertexAttribArray(n.programUniforms.vertex), A.bindBuffer(A.ARRAY_BUFFER, n.textureBuffer), A.bufferData(
    A.ARRAY_BUFFER,
    new Float32Array(a),
    A.STATIC_DRAW
  ), A.vertexAttribPointer(
    n.programUniforms.textureCoord,
    2,
    A.FLOAT,
    !1,
    0,
    0
  ), A.enableVertexAttribArray(n.programUniforms.textureCoord), A.activeTexture(A.TEXTURE0), A.bindTexture(A.TEXTURE_2D, n.fontTexture), A.uniform1i(n.programUniforms.texture, 0), A.uniform3fv(n.programUniforms.textColor, [1, 1, 1]), A.uniform3fv(n.programUniforms.backColor, [0, 0, 0]);
  function f(d, l, c) {
    const m = [];
    for (let E = 0; E < l; E++)
      for (let v = 0; v < d; v++)
        m.push(c[v * d + E]);
    return m;
  }
  A.uniformMatrix4fv(n.programUniforms.matrix, !1, f(4, 4, [
    1 / (s / 2),
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
  ])), A.uniformMatrix3fv(n.programUniforms.uvMatrix, !1, f(3, 3, [
    1 / n.textureSize.width,
    0,
    0,
    0,
    1 / n.textureSize.height,
    0,
    0,
    0,
    1
  ])), A.viewport(0, 0, s, r), A.enable(A.BLEND), A.blendFunc(A.SRC_ALPHA, A.ONE_MINUS_SRC_ALPHA), A.drawArrays(A.TRIANGLES, 0, h.length / 2), A.disable(A.BLEND);
}
const p = {
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
  phase: 6,
  /**
   * `(score, clean, combed blocks, on)`: the one-instant score; whether this
   * frame was clean and how many blocks were combed (both 0 for a still
   * frame); and whether the verdict is on. See INSTANT_ON and the `instant`
   * function of METRICS.
   */
  instant: 7
}, P = 8, g = 16, X = 8, Se = 15, Pe = 6, Ce = 8, Ie = 30, Be = 0.01, Ue = 2e-3, be = 2, z = 1, Le = 2, j = 10, ke = {
  a: "uA",
  b: "uB",
  fieldMetrics: "uFieldMetrics",
  first: "uFirst",
  size: "uSize"
}, ve = 16, Te = 8, Oe = `#version 300 es

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
  const int BLOCK_W = ${ve};
  const int BLOCK_H = ${Te};

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

        // Ignore small compression artifacts when accumulating differences at moving edges.
        diffEven += max(abs(a - b) - 8.0 / 255.0, 0.0);
        totalEven += 1;
      }
      p.y += 1;
      if (p.x < uSize.x && p.y < uSize.y) {
        float a = luma(texelFetch(uA, p, 0).rgb);
        float b = luma(texelFetch(uB, p, 0).rgb);

        diffOdd += max(abs(a - b) - 8.0 / 255.0, 0.0);
        totalOdd += 1;
      }
    }
  }

  float run = texelFetch(uFieldMetrics, ivec2(${p.phase}, 0), 0)[1];
  float threshold = run == 0.0 ? 0.025 : (run <= 10.0 ? 0.11 : 0.15);

  vec4 even = measure(diffEven, float(totalEven), threshold);
  vec4 odd = measure(diffOdd, float(totalOdd), threshold);
  outFirst = uFirst == 0 ? even : odd;
  outSecond = uFirst == 0 ? odd : even;
}
`, Ge = {
  second: "uSecond",
  first: "uFirst",
  size: "uSize"
}, U = 8, Xe = `#version 300 es
precision highp float;

uniform sampler2D uSecond;
uniform sampler2D uFirst;

uniform ivec2 uSize;

layout(location = 0) out vec4 outSecond;
layout(location = 1) out vec4 outFirst;

void main()
{
  ivec2 dst = ivec2(gl_FragCoord.xy);
  ivec2 base = dst * ${U};

  vec4 second = vec4(0.0);
  vec4 first = vec4(0.0);

  for (int y = 0; y < ${U}; ++y) {
    for (int x = 0; x < ${U}; ++x) {
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
`, ze = {
  previous: "uPrevious",
  second: "uSecond",
  first: "uFirst",
  comb: "uComb",
  size: "uSize"
}, We = `#version 300 es
precision highp float;

uniform sampler2D uPrevious;
uniform sampler2D uSecond;
uniform sampler2D uFirst;
/** The comb counts of the frame being decided, as it stands. */
uniform sampler2D uComb;

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
  return differing <= ${be}.0;
}

// A repeated field changes less than the opposite field.
// Distinguish motion shared by both fields from a still image with small absolute differences.
bool repeats(vec4 field, vec4 other) {
  return same(field[1]) &&
    (max(field[0], other[0]) < 0.025 || field[2] < other[2] * 0.75);
}

/** The phase texel, (phase, run), from the six comparisons' differing counts. */
vec4 decide(vec4 last, vec4 dFirstRepeatsPrevious, vec4 dSecondRepeatsNext,
            vec4 dSecondRepeatsPrevious, vec4 dPreviousSecondRepeated,
            vec4 dFirstRepeatsNext, vec4 dPreviousFirstRepeated)
{
  bool firstRepeatsPrevious = repeats(dFirstRepeatsPrevious, dSecondRepeatsPrevious);
  bool secondRepeatsNext = repeats(dSecondRepeatsNext, dFirstRepeatsNext);
  bool secondRepeatsPrevious = repeats(dSecondRepeatsPrevious, dFirstRepeatsPrevious);
  bool previousSecondRepeated = repeats(dPreviousSecondRepeated, dPreviousFirstRepeated);
  bool firstRepeatsNext = repeats(dFirstRepeatsNext, dSecondRepeatsNext);
  bool previousFirstRepeated = repeats(dPreviousFirstRepeated, dPreviousSecondRepeated);
  bool still = (firstRepeatsPrevious && secondRepeatsPrevious) || (secondRepeatsNext && firstRepeatsNext);

  int previous = int(last[0]);
  float run = last[1];
  // What each phase looks like: one field repeats, the other plainly does not.
  bool looks1 = firstRepeatsPrevious && !secondRepeatsPrevious;
  bool looks2 = secondRepeatsNext && !firstRepeatsNext;
  bool looks3 = secondRepeatsPrevious && !firstRepeatsPrevious;
  bool looks4 = previousSecondRepeated && !previousFirstRepeated;
  bool looks5 = firstRepeatsNext && !secondRepeatsNext;

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
  bool believed = run >= ${j}.0;
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

/**
 * The instant texel; see INSTANT_ON. A frame is moving if it differs from
 * the frame before it. This is the pair the comb count uses, and the count
 * only sees what changed since that frame. A frame equal to the one before
 * it shows nothing, however much the next frame differs.
 */
vec4 instant(vec4 last, vec4 dSecondPrevious, vec4 dFirstPrevious)
{
  float score = last[0];
  if (max(dSecondPrevious[2], dFirstPrevious[2]) <= ${Ue}) return vec4(score, 0.0, 0.0, last[3]);
  ivec2 blocks = textureSize(uComb, 0);
  float combed = 0.0;
  for (int y = 0; y < blocks.y; ++y) {
    for (int x = 0; x < blocks.x; ++x) {
      if (texelFetch(uComb, ivec2(x, y), 0)[0] > ${X}.0) combed += 1.0;
    }
  }
  bool clean = combed <= ${Be} * float(blocks.x * blocks.y);
  score = clean ? min(score + 1.0, ${Ie}.0) : max(score - ${Ce}.0, 0.0);
  bool on = last[3] > 0.5 ? score >= ${Pe}.0 : score >= ${Se}.0;
  return vec4(score, clean ? 1.0 : 0.0, combed, on ? 1.0 : 0.0);
}

void main()
{
  int metric = int(gl_FragCoord.x);
  // The comparisons against the next frame become, a frame later, the ones
  // against the previous frame, and those the ones before.
  if (metric == ${p.firstRepeatsPrevious}) {
    outValue = previous(${p.firstRepeatsNext});
  } else if (metric == ${p.secondRepeatsNext}) {
    outValue = fold(uSecond);
  } else if (metric == ${p.secondRepeatsPrevious}) {
    outValue = previous(${p.secondRepeatsNext});
  } else if (metric == ${p.previousSecondRepeated}) {
    outValue = previous(${p.secondRepeatsPrevious});
  } else if (metric == ${p.firstRepeatsNext}) {
    outValue = fold(uFirst);
  } else if (metric == ${p.previousFirstRepeated}) {
    outValue = previous(${p.firstRepeatsPrevious});
  } else if (metric == ${p.instant}) {
    outValue = instant(
      previous(${p.instant}),
      previous(${p.secondRepeatsNext}),
      previous(${p.firstRepeatsNext})
    );
  } else {
    outValue = decide(
      previous(${p.phase}),
      previous(${p.firstRepeatsNext}),
      fold(uSecond),
      previous(${p.secondRepeatsNext}),
      previous(${p.secondRepeatsPrevious}),
      fold(uFirst),
      previous(${p.firstRepeatsPrevious})
    );
  }
}
`, He = {
  prev: "uPrev",
  cur: "uCur",
  first: "uFirst",
  size: "uSize"
}, $e = `#version 300 es
precision highp float;
precision highp int;

uniform sampler2D uPrev;
uniform sampler2D uCur;
/** The parity of the lines of the field captured first. */
uniform int uFirst;
uniform ivec2 uSize;

out vec4 outValue;

const float COMB = 8.0 / 255.0;

float luma(sampler2D image, int x, int y) {
  int line = y < 0 ? -y : (y >= uSize.y ? 2 * (uSize.y - 1) - y : y);
  vec3 c = texelFetch(image, ivec2(x, clamp(line, 0, uSize.y - 1)), 0).rgb;
  return dot(c, vec3(0.2126, 0.7152, 0.0722));
}

bool combed(float above2, float above, float pixel, float below, float below2) {
  float d1 = pixel - above;
  float d2 = pixel - below;
  return ((d1 > COMB && d2 > COMB) || (d1 < -COMB && d2 < -COMB)) &&
    abs(above2 + 4.0 * pixel + below2 - 3.0 * (above + below)) > 6.0 * COMB;
}

void main() {
  ivec2 at = ivec2(gl_FragCoord.xy);
  int x = at.x;
  int y = 2 * at.y + 1 - uFirst;
  outValue = vec4(0.0);
  if (y >= uSize.y) return;
  float current = luma(uCur, x, y);
  float previous = luma(uPrev, x, y);
  if (abs(current - previous) <= COMB) return;
  // The first field's lines either side are the same whichever second
  // field is woven between them.
  float above = luma(uCur, x, y - 1);
  float below = luma(uCur, x, y + 1);
  outValue.r = float(combed(luma(uCur, x, y - 2), above, current, below, luma(uCur, x, y + 2)));
  outValue.g = float(combed(luma(uPrev, x, y - 2), above, previous, below, luma(uPrev, x, y + 2)));
}
`, Ve = {
  flags: "uFlags",
  size: "uSize"
}, Ye = `#version 300 es
precision highp float;
precision highp int;

uniform sampler2D uFlags;
/** The size of the marks. */
uniform ivec2 uSize;

out vec4 outValue;

void main() {
  ivec2 block = ivec2(gl_FragCoord.xy);
  ivec2 base = block * ivec2(${g}, ${g / 2});
  ivec2 end = min(base + ivec2(${g}, ${g / 2}), uSize);
  vec2 count = vec2(0.0);
  for (int y = base.y; y < end.y; ++y)
    for (int x = base.x; x < end.x; ++x)
      count += texelFetch(uFlags, ivec2(x, y), 0).rg;
  outValue = vec4(count, 0.0, 0.0);
}
`, Qe = {
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
  fieldMetrics: "uFieldMetrics",
  comb: "uComb",
  combAhead: "uCombAhead",
  match: "uMatch",
  instant: "uInstant"
}, Ze = `#version 300 es
precision highp float;
precision highp int;

uniform sampler2D uPrev;
uniform sampler2D uCur;
uniform sampler2D uNext;
uniform sampler2D uFieldMetrics;
uniform sampler2D uComb;
uniform sampler2D uCombAhead;
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
/** Whether a missing line may be taken from a neighbouring field that fits. */
uniform bool uMatch;
/** Whether it may come from the frame's own other field where frames are one instant. */
uniform bool uInstant;
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
  return texelFetch(uFieldMetrics, ivec2(metric, 0), 0)[1] <= ${be}.0;
}

/** The pulldown phase the detection gave this frame, or 0. See film-shader.ts. */
int detectedPhase() {
  vec4 phase = texelFetch(uFieldMetrics, ivec2(${p.phase}, 0), 0);
  // Deinterlace each field normally until the cadence is confirmed.
  return phase[1] >= ${j}.0 ? int(phase[0]) : 0;
}

bool isMixedPhase(int phase) {
  return phase == ${z} || phase == ${Le};
}

/** Detect new combing in phase 4, whose cadence is inferred from previous repeats. */
bool movingComb(vec3 pixel, int x, int y) {
  vec3 above = fetch(uCur, x, y - 1);
  vec3 below = fetch(uCur, x, y + 1);
  vec3 comb = max(min(above, below) - pixel, pixel - max(above, below));
  // Phase 3 is a complete film frame, so use its vertical detail as the reference.
  // Preserve existing horizontal lines and interpolate only pixels with new combing.
  vec3 previous = fetch(uPrev, x, y);
  vec3 previousAbove = fetch(uPrev, x, y - 1);
  vec3 previousBelow = fetch(uPrev, x, y + 1);
  vec3 previousComb = max(min(previousAbove, previousBelow) - previous,
                          previous - max(previousAbove, previousBelow));
  return any(greaterThan(comb, max(previousComb, vec3(0.0)) + vec3(8.0 / 255.0)));
}

/**
 * Take a missing line from the field it was captured with, if there is one.
 *
 * A cut made after pulldown restarts the film cycle, and the phase is unknown
 * until two cycles have passed. Each field still has a partner from the same
 * film frame, and the partner can be found without knowing the phase. It is
 * the other field of this frame, or the adjoining field of a neighbouring
 * frame: the second field of the previous frame for a first field, the first
 * field of the next frame for a second field. The block is woven from
 * whichever candidate leaves it less combed. If neither leaves it clean, the
 * block is left to the filter, as in the film branch.
 *
 * Video has no partner, but a block that barely moves can look as if it had
 * one. So the neighbouring frames are used only shortly after film
 * (neighbours). Where the frames are found to hold one instant each, the
 * partner is always the frame's own other field. Both fields of a frame then
 * come out as the same picture, except in blocks that are combed as the frame
 * stands, which are filtered.
 */
bool matched(int x, int y, bool neighbours, out vec3 rgb) {
  ivec2 block = ivec2(x / ${g}, y / ${g});
  float standing = texelFetch(uComb, block, 0)[0];
  if (!neighbours) {
    if (standing > ${X}.0) return false;
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
    return true;
  }
  float adjoining = uSecond ? texelFetch(uCombAhead, block, 0)[1]
                            : texelFetch(uComb, block, 0)[1];
  if (min(standing, adjoining) > ${X}.0) return false;
  if (standing <= adjoining) {
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
  } else if (uSecond) {
    rgb = texelFetch(uNext, ivec2(x, y), 0).rgb;
  } else {
    rgb = texelFetch(uPrev, ivec2(x, y), 0).rgb;
  }
  return true;
}

/** Whether the detection found the two fields of each frame to be one instant. */
bool oneInstant() {
  return texelFetch(uFieldMetrics, ivec2(${p.instant}, 0), 0)[3] > 0.5;
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
  } else if (uFilm && !uSecond && detectedPhase() != 0) {
    bool mixed = isMixedPhase(detectedPhase());
    rgb = mixed && (y & 1) != firstParity()
      ? texelFetch(uPrev, ivec2(x, y), 0).rgb
      : texelFetch(uCur, ivec2(x, y), 0).rgb;
    // A block the weave would leave combed -- a cut, a fade or a telop that
    // does not follow the cadence -- is filtered like video instead.
    vec4 comb = texelFetch(uComb, ivec2(x / ${g}, y / ${g}), 0);
    bool combedBlock = (mixed ? comb[1] : comb[0]) > ${X}.0;
    if ((y & 1) != uParity &&
        (combedBlock || (detectedPhase() == 4 && movingComb(rgb, x, y))))
      rgb = filterPixel(uPrev, uCur, x, y);
  } else if ((y & 1) == uParity) {
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
  } else if ((uMatch || (uInstant && oneInstant())) && matched(x, y, uMatch, rgb)) {
    // Woven from the field that fits.
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
function B(n, e, t) {
  return Object.fromEntries(
    Object.entries(t).map(([i, s]) => [
      i,
      n.getUniformLocation(e, s)
    ])
  );
}
class J {
  #i;
  #t;
  #e;
  #u;
  #E;
  #R;
  #s;
  #n;
  #A;
  #o;
  #v;
  /** The block comparisons of both fields, and the same folded most of the way. */
  #T = null;
  #B = null;
  /** The combed pixels of the frame measured last, one field's lines; see COMB_FLAGS_FRAGMENT_SHADER. */
  #a = null;
  /** Combed pixels per block of the frame being filtered, both ways of weaving it. */
  #h = null;
  /** Combed pixels per block of the frame after the one being filtered. */
  #f = null;
  /** The field metrics (see FIELD_METRICS) of this frame and the one before. */
  #l = null;
  /** Which of the two holds the newest metrics. */
  #r = 0;
  /** The metrics being read back asynchronously. */
  #d = null;
  #g = null;
  #_ = 0;
  #H = 0;
  /** The last metrics read back, laid out as FIELD_METRICS says. */
  metrics = new Float32Array(P * 4);
  #x = 0;
  #p = 0;
  constructor(e) {
    this.#i = e, this.#t = R(
      e,
      Oe,
      S
    ), this.#e = B(
      e,
      this.#t,
      ke
    ), this.#u = R(
      e,
      Xe,
      S
    ), this.#E = B(
      e,
      this.#u,
      Ge
    ), this.#R = R(
      e,
      We,
      S
    ), this.#s = B(e, this.#R, ze), this.#n = R(
      e,
      $e,
      S
    ), this.#A = B(
      e,
      this.#n,
      He
    ), this.#o = R(
      e,
      Ye,
      S
    ), this.#v = B(e, this.#o, Ve), y(e, this.#t, e.RGBA32F, 2), y(e, this.#u, e.RGBA32F, 2), y(e, this.#R, e.RGBA32F), y(e, this.#n, e.RG8), y(e, this.#o, e.RGBA32F);
  }
  /** The newest measurements, or null before any frame has been measured. */
  get texture() {
    return this.#l?.[this.#r]?.textures[0] ?? null;
  }
  /**
   * Combed pixels per block of the frame last measured by `measureComb`, or
   * null before one has been. See COMB_FLAGS_FRAGMENT_SHADER.
   */
  get combTexture() {
    return this.#h?.textures[0] ?? null;
  }
  /**
   * Combed pixels per block of the frame last measured with `ahead`, or null
   * before one has been.
   */
  get combAheadTexture() {
    return this.#f?.textures[0] ?? null;
  }
  /** The size of the frames to be measured, which sizes the block grid. */
  resize(e, t) {
    e === this.#x && t === this.#p || (this.#x = e, this.#p = t, this.#c());
  }
  /**
   * Forget the measurements: the next frame starts a cycle from nothing.
   * `keepInstant` keeps the one-instant score, for a gap in the frames when
   * the content has not changed.
   */
  reset(e = !1) {
    const t = this.#i;
    t.deleteSync(this.#g), this.#g = null, this.#_ = 0;
    const i = this.#l?.[this.#r];
    if (!i) return;
    const s = e ? p.instant : P, r = new Float32Array(s * 4);
    for (let o = 0; o < p.phase; o++)
      r[o * 4 + 1] = 1;
    t.bindTexture(t.TEXTURE_2D, i.textures[0] ?? null), t.texSubImage2D(
      t.TEXTURE_2D,
      0,
      0,
      0,
      s,
      1,
      t.RGBA,
      t.FLOAT,
      r
    );
  }
  /**
   * Detect the pulldown phase of `cur`, the frame being filtered, against
   * `next`, and start reading it back. Only the two comparisons against the
   * next frame are measured; the rest are earlier ones moved along a frame.
   * `first` is the parity of the field that was captured first.
   *
   * The same pass scores whether the two fields of a frame are one instant,
   * from the comb counts of the frame as it stands. `measureComb` for this
   * frame must have run before this.
   */
  detect(e, t, i) {
    const s = this.#i;
    if (this.#x === 0 || this.#p === 0) return;
    this.#y();
    const r = this.#T, o = this.#B, h = this.#l;
    if (r === null || o === null || h === null) return;
    const a = h[this.#r], u = h[1 - this.#r];
    if (s.bindFramebuffer(s.FRAMEBUFFER, r.framebuffer), s.useProgram(this.#t), this.#M(0, e, this.#e.a), this.#M(1, t, this.#e.b), this.#M(2, a.textures[0], this.#e.fieldMetrics), s.uniform1i(this.#e.first, i), s.uniform2i(this.#e.size, this.#x, this.#p), s.viewport(0, 0, r.width, r.height), s.drawArrays(s.TRIANGLES, 0, 3), s.bindFramebuffer(s.FRAMEBUFFER, o.framebuffer), s.useProgram(this.#u), this.#M(0, r.textures[0], this.#E.second), this.#M(1, r.textures[1], this.#E.first), s.uniform2i(this.#E.size, r.width, r.height), s.viewport(0, 0, o.width, o.height), s.drawArrays(s.TRIANGLES, 0, 3), s.bindFramebuffer(s.FRAMEBUFFER, u.framebuffer), s.useProgram(this.#R), this.#M(0, a.textures[0], this.#s.previous), this.#M(1, o.textures[0], this.#s.second), this.#M(2, o.textures[1], this.#s.first), this.#M(3, this.#h?.textures[0], this.#s.comb), s.uniform2i(this.#s.size, o.width, o.height), s.viewport(0, 0, P, 1), s.drawArrays(s.TRIANGLES, 0, 3), this.#r = 1 - this.#r, this.#_++, this.#g !== null) {
      s.bindFramebuffer(s.FRAMEBUFFER, null);
      return;
    }
    this.#H = this.#_, s.bindBuffer(s.PIXEL_PACK_BUFFER, this.#d), s.readPixels(0, 0, P, 1, s.RGBA, s.FLOAT, 0), s.bindBuffer(s.PIXEL_PACK_BUFFER, null), s.bindFramebuffer(s.FRAMEBUFFER, null), this.#g = s.fenceSync(s.SYNC_GPU_COMMANDS_COMPLETE, 0), s.flush();
  }
  /**
   * Count the combing each way of weaving `cur`, the frame being filtered,
   * would leave: as it stands, and with `prev`'s second field. `first` is the
   * parity of the field that was captured first. With `ahead`, `prev` is the
   * frame being filtered and `cur` is the one after it, and the counts are
   * those of `cur`.
   */
  measureComb(e, t, i, s = !1) {
    const r = this.#i;
    if (this.#x === 0 || this.#p === 0) return;
    this.#y();
    const o = s ? this.#f : this.#h, h = this.#a;
    o === null || h === null || (r.bindFramebuffer(r.FRAMEBUFFER, h.framebuffer), r.useProgram(this.#n), this.#M(0, e, this.#A.prev), this.#M(1, t, this.#A.cur), r.uniform1i(this.#A.first, i), r.uniform2i(this.#A.size, this.#x, this.#p), r.viewport(0, 0, h.width, h.height), r.drawArrays(r.TRIANGLES, 0, 3), r.bindFramebuffer(r.FRAMEBUFFER, o.framebuffer), r.useProgram(this.#o), this.#M(0, h.textures[0], this.#v.flags), r.uniform2i(this.#v.size, h.width, h.height), r.viewport(0, 0, o.width, o.height), r.drawArrays(r.TRIANGLES, 0, 3), r.bindFramebuffer(r.FRAMEBUFFER, null));
  }
  /**
   * The phase of the last frame measured, once the GPU has handed it back,
   * and null while it is still on its way. It is handed back once.
   * `instant` is the one-instant verdict as of that frame. The filter reads
   * the same verdict from `texture` without waiting for the readback.
   */
  poll() {
    const e = this.#i, t = this.#g;
    if (t === null || this.#d === null) return null;
    switch (e.clientWaitSync(t, 0, 0)) {
      case e.ALREADY_SIGNALED:
      case e.CONDITION_SATISFIED:
        return e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#d), e.getBufferSubData(e.PIXEL_PACK_BUFFER, 0, this.metrics), e.bindBuffer(e.PIXEL_PACK_BUFFER, null), e.deleteSync(t), this.#g = null, {
          phase: this.metrics[p.phase * 4] ?? 0,
          run: this.metrics[p.phase * 4 + 1] ?? 0,
          instant: (this.metrics[p.instant * 4 + 3] ?? 0) > 0.5,
          age: this.#_ - this.#H
        };
      default:
        return null;
    }
  }
  destroy() {
    const e = this.#i;
    if (this.#c(), this.#l !== null) {
      for (const t of this.#l) N(e, t);
      this.#l = null;
    }
    e.deleteSync(this.#g), this.#g = null, e.deleteBuffer(this.#d), this.#d = null, e.deleteProgram(this.#t), e.deleteProgram(this.#u), e.deleteProgram(this.#R), e.deleteProgram(this.#n), e.deleteProgram(this.#o);
  }
  #M(e, t, i) {
    const s = this.#i;
    s.activeTexture(s.TEXTURE0 + e), s.bindTexture(s.TEXTURE_2D, t ?? null), s.uniform1i(i, e);
  }
  #c() {
    const e = this.#i;
    this.#T !== null && N(e, this.#T), this.#B !== null && N(e, this.#B), this.#a !== null && N(e, this.#a), this.#h !== null && N(e, this.#h), this.#f !== null && N(e, this.#f), this.#T = null, this.#B = null, this.#a = null, this.#h = null, this.#f = null;
  }
  /** Everything detect needs that is not there yet. */
  #y() {
    const e = this.#i;
    if (this.#T === null || this.#B === null || this.#a === null || this.#h === null || this.#f === null) {
      this.#c();
      const t = Math.ceil(this.#x / ve), i = Math.ceil(this.#p / (Te * 2));
      this.#T = _(e, t, i, 2), this.#B = _(
        e,
        Math.ceil(t / U),
        Math.ceil(i / U),
        2
      ), this.#a = _(
        e,
        this.#x,
        Math.ceil(this.#p / 2),
        1,
        "marks"
      ), this.#h = _(
        e,
        Math.ceil(this.#x / g),
        Math.ceil(this.#p / g),
        1
      ), this.#f = _(
        e,
        Math.ceil(this.#x / g),
        Math.ceil(this.#p / g),
        1
      );
    }
    this.#l === null && (this.#l = [
      _(e, P, 1, 1),
      _(e, P, 1, 1)
    ], this.#r = 0, this.reset()), this.#d === null && (this.#d = e.createBuffer(), e.bindBuffer(e.PIXEL_PACK_BUFFER, this.#d), e.bufferData(
      e.PIXEL_PACK_BUFFER,
      this.metrics.byteLength,
      e.STREAM_COPY
    ), e.bindBuffer(e.PIXEL_PACK_BUFFER, null));
  }
}
function _(n, e, t, i, s = "floats") {
  const r = s === "marks", o = n.createFramebuffer();
  n.bindFramebuffer(n.FRAMEBUFFER, o);
  const h = [];
  for (let A = 0; A < i; A++) {
    const f = n.createTexture();
    n.bindTexture(n.TEXTURE_2D, f), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MIN_FILTER, n.NEAREST), n.texParameteri(n.TEXTURE_2D, n.TEXTURE_MAG_FILTER, n.NEAREST), n.texImage2D(
      n.TEXTURE_2D,
      0,
      r ? n.RG8 : n.RGBA32F,
      e,
      t,
      0,
      r ? n.RG : n.RGBA,
      r ? n.UNSIGNED_BYTE : n.FLOAT,
      null
    ), n.framebufferTexture2D(
      n.FRAMEBUFFER,
      n.COLOR_ATTACHMENT0 + A,
      n.TEXTURE_2D,
      f,
      0
    ), h.push(f);
  }
  n.drawBuffers(h.map((A, f) => n.COLOR_ATTACHMENT0 + f));
  const a = n.checkFramebufferStatus(n.FRAMEBUFFER) === n.FRAMEBUFFER_COMPLETE;
  n.bindFramebuffer(n.FRAMEBUFFER, null);
  const u = { framebuffer: o, textures: h, width: e, height: t };
  if (!a)
    throw N(n, u), new Error("failed to allocate framebuffer");
  return u;
}
function N(n, { framebuffer: e, textures: t }) {
  n.deleteFramebuffer(e);
  for (const i of t) n.deleteTexture(i);
}
function ee(n) {
  return Number.isInteger(n) && n > 0;
}
function je(n) {
  const {
    timestamp: e,
    duration: t,
    width: i,
    height: s,
    displayWidth: r,
    displayHeight: o,
    data: h,
    planes: a,
    matrix: u
  } = n;
  if (!Number.isFinite(e) || !(t > 0) || !Number.isFinite(t))
    throw new TypeError(`invalid picture timing ${e}+${t}`);
  if (!ee(i) || !ee(s))
    throw new TypeError(`invalid picture size ${i}x${s}`);
  if (!(r > 0) || !(o > 0))
    throw new TypeError(
      `invalid display size ${r}x${o}`
    );
  if (u !== "bt709" && u !== "bt601")
    throw new TypeError(`unknown matrix ${String(u)}`);
  if (a.length !== 3) throw new TypeError("a picture has three planes");
  for (let A = 0; A < 3; A++) {
    const f = A === 0 ? i : Math.ceil(i / 2), d = A === 0 ? s : Math.ceil(s / 2), { offset: l, stride: c } = a[A];
    if (!Number.isInteger(l) || l < 0 || !Number.isInteger(c) || c < f || l + c * (d - 1) + f > h.byteLength)
      throw new TypeError(
        `plane ${A} at ${l} with stride ${c} does not fit ${f}x${d} samples in ${h.byteLength} bytes`
      );
  }
}
function te(n, e) {
  const t = 1 - n - e;
  return Float32Array.of(
    2 * (1 - n),
    2 * (1 - e) * e / t,
    2 * (1 - n) * n / t,
    2 * (1 - e)
  );
}
const qe = {
  bt709: te(0.2126, 0.0722),
  bt601: te(0.299, 0.114)
}, Ke = `#version 300 es
precision highp float;
precision highp int;

uniform highp sampler2D uLuma;
uniform highp sampler2D uCb;
uniform highp sampler2D uCr;
/** Whether chroma rows belong alternately to the two fields. */
uniform bool uInterlaced;
uniform vec4 uMatrix;

out vec4 fragColor;

/** Position of luma column x in chroma columns. */
float chromaColumn(int x) {
  return float(x) * 0.5;
}

/** Position of luma row y in chroma rows, progressive. */
float chromaRow(int y) {
  return float(y) * 0.5 - 0.25;
}

/** Position of luma row y among the chroma rows of its own field. */
float chromaFieldRow(int y) {
  return (float(y >> 1) - 0.25 - 0.5 * float(y & 1)) * 0.5;
}

vec2 tap(int x, int y) {
  return vec2(texelFetch(uCb, ivec2(x, y), 0).r, texelFetch(uCr, ivec2(x, y), 0).r);
}

vec2 chroma(int x, int y, int width, int height) {
  float u = clamp(chromaColumn(x), 0.0, float(width - 1));
  int x0 = int(u);
  int x1 = min(x0 + 1, width - 1);
  float wx = u - float(x0);
  int y0;
  int y1;
  float wy;
  if (uInterlaced) {
    int parity = y & 1;
    int rows = (height + 1 - parity) >> 1;
    float v = clamp(chromaFieldRow(y), 0.0, float(rows - 1));
    int row = int(v);
    y0 = 2 * row + parity;
    y1 = 2 * min(row + 1, rows - 1) + parity;
    wy = v - float(row);
  } else {
    float v = clamp(chromaRow(y), 0.0, float(height - 1));
    y0 = int(v);
    y1 = min(y0 + 1, height - 1);
    wy = v - float(y0);
  }
  return mix(mix(tap(x0, y0), tap(x1, y0), wx), mix(tap(x0, y1), tap(x1, y1), wx), wy);
}

/** Limited-range YCbCr, as texture values, to full-range RGB. */
vec3 rgb(float luma, vec2 c) {
  float l = (luma * 255.0 - 16.0) / 219.0;
  float b = (c.x * 255.0 - 128.0) / 224.0;
  float r = (c.y * 255.0 - 128.0) / 224.0;
  return clamp(vec3(l + uMatrix.x * r, l - uMatrix.y * b - uMatrix.z * r, l + uMatrix.w * b), 0.0, 1.0);
}

void main() {
  ivec2 at = ivec2(gl_FragCoord.xy);
  ivec2 size = textureSize(uCb, 0);
  fragColor = vec4(rgb(texelFetch(uLuma, at, 0).r, chroma(at.x, at.y, size.x, size.y)), 1.0);
}
`;
class Je {
  #i;
  #t;
  #e;
  #u;
  #E;
  #R;
  #s;
  #n;
  #A = [];
  #o = 0;
  #v = 0;
  /** Whether the target format was found renderable since the last resize. */
  #T = !1;
  constructor(e) {
    this.#i = e, this.#t = R(e, Ke, S), this.#e = e.getUniformLocation(this.#t, "uLuma"), this.#u = e.getUniformLocation(this.#t, "uCb"), this.#E = e.getUniformLocation(this.#t, "uCr"), this.#R = e.getUniformLocation(this.#t, "uInterlaced"), this.#s = e.getUniformLocation(this.#t, "uMatrix"), this.#n = e.createFramebuffer(), y(e, this.#t, e.RGBA8);
  }
  /** Allocate the plane textures for pictures of this luma size. */
  resize(e, t) {
    const i = this.#i;
    for (const s of this.#A) i.deleteTexture(s);
    this.#A = [];
    for (let s = 0; s < 3; s++) {
      const r = i.createTexture();
      i.bindTexture(i.TEXTURE_2D, r), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MIN_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MAG_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_S, i.CLAMP_TO_EDGE), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_WRAP_T, i.CLAMP_TO_EDGE), i.texStorage2D(
        i.TEXTURE_2D,
        1,
        i.R8,
        s === 0 ? e : Math.ceil(e / 2),
        s === 0 ? t : Math.ceil(t / 2)
      ), this.#A.push(r);
    }
    this.#o = e, this.#v = t, this.#T = !1;
  }
  /**
   * Upload the picture's planes and draw its RGB into `target`, a texture of
   * the size last given to resize(). Throws if `target` cannot be drawn into.
   */
  convert(e, t) {
    const i = this.#i, s = new Uint8Array(e.data);
    i.pixelStorei(i.UNPACK_ALIGNMENT, 1);
    for (let r = 0; r < 3; r++) {
      const { offset: o, stride: h } = e.planes[r];
      i.bindTexture(i.TEXTURE_2D, this.#A[r]), i.pixelStorei(i.UNPACK_ROW_LENGTH, h), i.texSubImage2D(
        i.TEXTURE_2D,
        0,
        0,
        0,
        r === 0 ? this.#o : Math.ceil(this.#o / 2),
        r === 0 ? this.#v : Math.ceil(this.#v / 2),
        i.RED,
        i.UNSIGNED_BYTE,
        s,
        o
      );
    }
    if (i.pixelStorei(i.UNPACK_ROW_LENGTH, 0), i.pixelStorei(i.UNPACK_ALIGNMENT, 4), i.bindFramebuffer(i.FRAMEBUFFER, this.#n), i.framebufferTexture2D(
      i.FRAMEBUFFER,
      i.COLOR_ATTACHMENT0,
      i.TEXTURE_2D,
      t,
      0
    ), !this.#T) {
      if (i.checkFramebufferStatus(i.FRAMEBUFFER) !== i.FRAMEBUFFER_COMPLETE)
        throw i.bindFramebuffer(i.FRAMEBUFFER, null), new Error("decoded pictures cannot be drawn into RGBA8");
      this.#T = !0;
    }
    i.useProgram(this.#t);
    for (let r = 0; r < 3; r++)
      i.activeTexture(i.TEXTURE0 + r), i.bindTexture(i.TEXTURE_2D, this.#A[r]);
    i.uniform1i(this.#e, 0), i.uniform1i(this.#u, 1), i.uniform1i(this.#E, 2), i.uniform1i(this.#R, e.interlaced ? 1 : 0), i.uniform4fv(this.#s, qe[e.matrix]), i.viewport(0, 0, this.#o, this.#v), i.drawArrays(i.TRIANGLES, 0, 3), i.activeTexture(i.TEXTURE0), i.bindFramebuffer(i.FRAMEBUFFER, null);
  }
  destroy() {
    const e = this.#i;
    for (const t of this.#A) e.deleteTexture(t);
    this.#A = [], e.deleteFramebuffer(this.#n), e.deleteProgram(this.#t);
  }
}
const ie = 250, x = 4, et = 0.01, tt = 100, it = 250, se = 0.5, F = 2;
class st {
  /**
   * The position is #media + #rate * (wall - #wall) / 1000 seconds. It is
   * NaN between a hold and the reading after it, when it is not known.
   */
  #i = 0;
  #t = Number.NaN;
  /** The playback rate while running, 0 while held. */
  #e = 0;
  #u = 1;
  /** Whether the clock waits for the readings to show playing before it runs. */
  #E = !0;
  /** The last reading, and the wall time at which it was first seen. */
  #R = Number.NaN;
  #s = 0;
  /** While waiting: whether the last reading was a change, not where the clock was held. */
  #n = !1;
  /** Wall time between the last reading that changed and the reading before it. */
  #A = 0;
  /** Wall time of the last reading taken, changed or not. */
  #o = 0;
  #v = Number.NaN;
  /** Wall time of the last correction. */
  #T = 0;
  /** The highest error of a reading, in media seconds, per bucket; the last is the newest. */
  #B = new Float64Array(x);
  #a = 0;
  /** Whether the next reading holds the clock, as hold() asks. */
  #h = !0;
  /**
   * The longest the reading was seen unchanged before a change that moved it
   * as playing does, in media seconds, per BUCKET_MS of running; the last is
   * the newest. NaN for a bucket the clock has not run through.
   */
  #f = new Float64Array(x).fill(Number.NaN);
  /** Running time, in wall milliseconds, counted into the newest bucket of #stands. */
  #l = 0;
  /**
   * While running on a start that is still to be confirmed, which is any
   * start where the reading steps: the wall time by which the reading has to
   * change again for the start to hold. 0 otherwise.
   */
  #r = 0;
  /**
   * Whether a start still to be confirmed was taken back, so that the clock
   * does not run on a first change again before a start has held.
   */
  #d = !1;
  /** While waiting: whether the last change came at playing pace. */
  #g = !1;
  /** While waiting after a stop on a standstill, until the reading changes. */
  #_ = !1;
  /**
   * How far ahead of the sound the reading's first change was, in wall
   * milliseconds, at the last starts whose reading moved at the first
   * reading after the element was stopped, oldest overwritten first. NaN
   * until there have been that many.
   */
  #H = new Float64Array(3).fill(Number.NaN);
  #x = 0;
  /**
   * While waiting after the element was stopped: where it was held. NaN
   * otherwise, and after a change between two rates it plays at.
   */
  #p = Number.NaN;
  /** The reading's first change after that hold, and when it was seen. */
  #M = Number.NaN;
  #c = Number.NaN;
  /**
   * While waiting: the lead the sound is worked out with, or NaN where the
   * position is the reading.
   */
  #y = Number.NaN;
  /** Whether the reading has stood since its first change, while the lead is in use. */
  #oe = !1;
  /** A reading that stood and then moved on at playing pace, while the lead is in use. */
  #ie = Number.NEGATIVE_INFINITY;
  /** The position at `wall`, in media seconds, or NaN while not known. */
  at(e) {
    return this.#E && !Number.isNaN(this.#y) && !Number.isNaN(this.#t) ? this.#Y(e) : this.#t + this.#e * (e - this.#i) / 1e3;
  }
  /**
   * Where the sound is at `wall` while the clock waits on a reading known to
   * lead it: where the reading's first change was, less the lead, playing on
   * from there. Not before where the element was held, from where the sound
   * starts. Not past the reading and the time since it was read, which is the
   * furthest the element can be. Not behind a reading that stood and moved
   * on at playing pace, which is where the sound was when it moved.
   */
  #Y(e) {
    const t = this.#u, i = this.#M + t * (e - this.#c - this.#y) / 1e3, s = this.#R + t * (e - this.#o) / 1e3;
    return Math.max(
      this.#ie,
      Math.min(s, Math.max(this.#p, i))
    );
  }
  /** The rate the clock advances at: the playback rate, or 0 while held. */
  get rate() {
    return this.#e;
  }
  /**
   * Where the clock was last put by a reading instead of running there: at a
   * hold, or on a reading too far from it. Playback has been continuous from
   * here, so nothing that ended at or before it has been due since. NaN
   * before the first reading.
   */
  get origin() {
    return this.#v;
  }
  /**
   * Stop at the next reading and wait for the readings to show playing: a
   * seek, a pause or a stall the element has reported. Readings taken across
   * the event say nothing about the position after it.
   */
  hold() {
    this.#h = !0, this.#t = Number.NaN, this.#e = 0;
  }
  /**
   * Take a reading of `currentTime` made at `wall`. `playing` is whether the
   * element is playing at all: not paused, seeking or ended. Returns whether
   * the clock started running again from this reading.
   */
  observe(e, t, i, s) {
    const r = e - this.#o;
    if (this.#o = e, !s || i === 0 || this.#h || i !== this.#u) {
      const b = i !== 0 && (!s || this.#u === 0 || i === this.#u);
      return this.#h = !1, this.#u = i, this.#P(e, t), b && (this.#p = t), !1;
    }
    const o = this.#r !== 0;
    if (!this.#E && !o && this.#se(r), t === this.#R) {
      if (this.#E)
        return Number.isNaN(this.#y) || (this.#oe = !0), !1;
      if (o)
        return e > this.#r && (this.#d = !0, this.#O(e, t, !0)), !1;
      const b = this.#ee(i), I = Number.isNaN(b) ? it : b + r + F;
      return e - this.#s > I && this.#O(e, t, !Number.isNaN(b)), !1;
    }
    const h = (t - this.#R) * 1e3 / i, a = e - this.#s, u = a - r, A = this.#A;
    this.#R = t, this.#s = e, this.#A = r;
    const f = h <= a + A + F, d = h >= a * se && f;
    if (this.#E) {
      const b = !this.#n;
      if (this.#n = !0, b && !Number.isNaN(this.#p) && u <= F) {
        this.#M = t, this.#c = e;
        const q = this.#re();
        this.#y = q > r + F ? q : Number.NaN;
      } else this.#oe && h >= r * se && h <= r + A + F && (this.#ie = t);
      this.#_ && !b && d && this.#$(u, i), this.#_ = !1;
      const I = this.#U(i, r);
      if (!b && d) {
        if (I || this.#g)
          return this.#d = !1, this.#Q(
            e,
            t,
            i,
            r,
            I
          ), !0;
        this.#g = !0;
      } else if (this.#g = !1, b && !this.#d && I && h > 0 && f)
        return this.#Q(
          e,
          t,
          i,
          r,
          !0
        ), !0;
      return this.#i = e, this.#t = t, !1;
    }
    if (o) {
      if (!d)
        return this.#d = !0, this.#P(e, t), !1;
      this.#r = 0, this.#d = !1;
    } else d && this.#$(u, i);
    const l = t - this.at(e);
    if (Math.abs(l) > tt / 1e3 * i)
      return this.#v = t, this.#K(e, t), !0;
    const c = this.#B;
    e - this.#a >= ie ? (c.copyWithin(0, 1), c[x - 1] = l, this.#a = e) : c[x - 1] = Math.max(c[x - 1], l);
    let m = c[0];
    for (let b = 1; b < x; b++)
      m = Math.max(m, c[b]);
    const E = et * i * (e - this.#T) / 1e3, v = Math.max(-E, Math.min(E, m));
    this.#T = e, this.#t += v;
    for (let b = 0; b < x; b++)
      c[b] = c[b] - v;
    return !1;
  }
  /**
   * Count running time towards the window of #stands. The window advances
   * only while the clock runs, so holds and seeks in quick succession keep
   * what the playing before them showed.
   */
  #se(e) {
    this.#l += e, !(this.#l < ie) && (this.#f.copyWithin(0, 1), this.#f[x - 1] = 0, this.#l = 0);
  }
  /**
   * Run from a reading that has just changed. Where the reading steps, the
   * start holds only if the next change comes within the longest stand seen
   * and one reading more.
   */
  #Q(e, t, i, s, r) {
    Number.isNaN(this.#c) || (this.#H[this.#x++ % this.#H.length] = (this.#M - t) * 1e3 / i + (e - this.#c)), this.#K(e, t), r && (this.#r = e + this.#ee(i) + s + F);
  }
  /**
   * Stop a running clock on a reading that has stood too long, keeping when
   * the reading was first seen where its updates are known: the change that
   * ends the standstill is then judged over the whole time the reading
   * stood. An update that was merely late moves as far as that time; where
   * the reading steps it starts the clock again at once, and where it follows
   * the element the next change at playing pace does. A standstill of the
   * element moves less, and the clock waits.
   */
  #O(e, t, i) {
    const s = this.#s;
    this.#P(e, t), i && (this.#s = s, this.#n = !0, this.#_ = !0);
  }
  /** Note that the reading stood `stood` wall milliseconds between two updates. */
  #$(e, t) {
    const i = this.#f[x - 1], s = e * t / 1e3;
    this.#f[x - 1] = Number.isNaN(i) ? s : Math.max(i, s);
  }
  /**
   * Whether the element updates its reading in steps: in each whole bucket
   * of the last second of running the reading stood, between two updates,
   * for longer than readings taken close together and at least as long as
   * the time between readings. One reading that happened to repeat does not
   * make it so.
   */
  #U(e, t) {
    let i = Number.POSITIVE_INFINITY;
    for (let r = 0; r < x - 1; r++) {
      const o = this.#f[r];
      if (Number.isNaN(o)) return !1;
      i = Math.min(i, o);
    }
    const s = i * 1e3 / e;
    return s > F && s + F >= t;
  }
  /**
   * The longest the reading was seen to stand between updates over the last
   * second of running, in wall milliseconds at `playbackRate`. NaN until the
   * clock has run through a whole bucket: the newest one is still filling.
   */
  #ee(e) {
    let t = Number.NaN;
    for (let s = 0; s < x - 1; s++) {
      const r = this.#f[s];
      Number.isNaN(r) || (t = Number.isNaN(t) ? r : Math.max(t, r));
    }
    if (Number.isNaN(t)) return t;
    const i = this.#f[x - 1];
    return Number.isNaN(i) || (t = Math.max(t, i)), t * 1e3 / e;
  }
  /**
   * The lead of the last starts, the middle of them, so that one start the
   * element made differently does not move it. NaN until there have been
   * enough.
   */
  #re() {
    const [e, t, i] = this.#H;
    return Number.isNaN(e) || Number.isNaN(t) || Number.isNaN(i) ? Number.NaN : Math.max(Math.min(e, t), Math.min(Math.max(e, t), i));
  }
  #P(e, t) {
    this.#p = Number.NaN, this.#M = Number.NaN, this.#c = Number.NaN, this.#y = Number.NaN, this.#oe = !1, this.#ie = Number.NEGATIVE_INFINITY, this.#_ = !1, this.#g = !1, this.#r = 0, this.#i = e, this.#t = t, this.#e = 0, this.#E = !0, this.#n = !1, this.#R = t, this.#s = e, this.#v = t;
  }
  #K(e, t) {
    this.#c = Number.NaN, this.#y = Number.NaN, this.#r = 0, this.#i = e, this.#t = t, this.#e = this.#u, this.#E = !1, this.#T = e, this.#a = e, this.#B.fill(Number.NEGATIVE_INFINITY), this.#B[x - 1] = 0;
  }
}
const L = 16, rt = 6, nt = `#version 300 es
precision highp float;
uniform sampler2D uTake;
uniform sampler2D uReference;
out vec4 fragColor;

void main() {
  // The caller compares sizes taken from the element. Those can briefly differ
  // from the uploaded picture when the stream changes resolution, so compare
  // the texture sizes here too. Different sizes count as changed.
  if (textureSize(uTake, 0) != textureSize(uReference, 0)) {
    fragColor = vec4(1.0);
    return;
  }
  // Each cell samples every texel it covers, so a change anywhere in the picture is detected.
  // Edge cells clamp and re-read their last row or column.
  ivec2 last = textureSize(uTake, 0) - 1;
  ivec2 origin = ivec2(gl_FragCoord.xy) * ${L};
  vec3 difference = vec3(0.0);
  for (int y = 0; y < ${L}; y++) {
    for (int x = 0; x < ${L}; x++) {
      ivec2 at = min(origin + ivec2(x, y), last);
      difference = max(
        difference,
        abs(texelFetch(uTake, at, 0).rgb - texelFetch(uReference, at, 0).rgb)
      );
    }
  }
  fragColor = vec4(difference, 1.0);
}
`;
class ht {
  #i;
  #t;
  #e = null;
  #u = null;
  /** Textures available for the next take. */
  #E = [];
  #R = 0;
  /** Readback buffers available for the next comparison. */
  #s = [];
  #n = new Uint8Array(0);
  /** Takes still held, oldest first. */
  #A = [];
  /** The newest take. The next take is compared with it. */
  #o = null;
  #v = 0;
  constructor(e, t) {
    this.#i = e, this.#t = t;
  }
  take() {
    const e = this.#i, t = this.#E.pop() ?? this.#h();
    if (t === null) return null;
    const i = this.#t.videoWidth, s = this.#t.videoHeight;
    e.bindTexture(e.TEXTURE_2D, t);
    try {
      e.texImage2D(
        e.TEXTURE_2D,
        0,
        e.RGBA,
        e.RGBA,
        e.UNSIGNED_BYTE,
        this.#t
      );
    } catch (h) {
      throw this.#E.push(t), h;
    }
    const r = {
      id: ++this.#v,
      texture: t,
      width: i,
      height: s,
      changed: null,
      fence: null,
      buffer: null,
      reported: !1
    }, o = this.#o;
    return o === null || o.width !== i || o.height !== s ? r.changed = !0 : this.#T(r, o), this.#o = r, this.#A.push(r), r.id;
  }
  settle() {
    const e = this.#i, t = this.#A.find((i) => !i.reported);
    if (t === void 0 || t === this.#o) return null;
    if (t.changed === null) {
      const { fence: i, buffer: s } = t;
      if (i === null || s === null) return null;
      const r = e.clientWaitSync(i, 0, 0);
      if (r !== e.ALREADY_SIGNALED && r !== e.CONDITION_SATISFIED)
        return null;
      this.#n.byteLength < s.bytes && (this.#n = new Uint8Array(s.bytes));
      const o = this.#n.subarray(0, s.bytes);
      e.bindBuffer(e.PIXEL_PACK_BUFFER, s.buffer), e.getBufferSubData(e.PIXEL_PACK_BUFFER, 0, o), e.bindBuffer(e.PIXEL_PACK_BUFFER, null), t.changed = ot(o), this.#B(t);
    }
    return t.reported = !0, { id: t.id, changed: t.changed };
  }
  /**
   * Give the texture of a reported take to the caller and accept a texture in
   * exchange. The replacement becomes available for later takes.
   */
  adopt(e, t) {
    const i = this.#A.findIndex((r) => r.id === e), s = this.#A[i];
    if (s === void 0 || !s.reported)
      throw new Error(`picture ${e} is not available to adopt`);
    return this.#A.splice(i, 1), this.#E.push(t), s.texture;
  }
  release(e) {
    const t = this.#A.findIndex((s) => s.id === e), i = this.#A[t];
    i !== void 0 && (this.#A.splice(t, 1), this.#a(i));
  }
  reset() {
    const e = this.#A;
    this.#A = [], this.#o = null;
    for (const t of e)
      this.#B(t), this.#a(t);
  }
  destroy() {
    const e = this.#i;
    this.reset();
    for (const t of this.#E) e.deleteTexture(t);
    this.#E = [], this.#R = 0;
    for (const { buffer: t } of this.#s) e.deleteBuffer(t);
    this.#s = [], this.#r(), this.#e !== null && e.deleteProgram(this.#e), this.#e = null;
  }
  /** Compare a take with the reference on the GPU and start reading back the result. */
  #T(e, t) {
    const i = this.#i, s = Math.ceil(e.width / L), r = Math.ceil(e.height / L), o = this.#l(s, r), h = this.#d(s * r * 4);
    this.#e ??= this.#f(), i.bindFramebuffer(i.FRAMEBUFFER, o.framebuffer), i.useProgram(this.#e), i.activeTexture(i.TEXTURE1), i.bindTexture(i.TEXTURE_2D, t.texture), i.activeTexture(i.TEXTURE0), i.bindTexture(i.TEXTURE_2D, e.texture), i.viewport(0, 0, s, r), i.drawArrays(i.TRIANGLES, 0, 3), i.bindBuffer(i.PIXEL_PACK_BUFFER, h.buffer), i.readPixels(0, 0, s, r, i.RGBA, i.UNSIGNED_BYTE, 0), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), i.bindFramebuffer(i.FRAMEBUFFER, null), e.fence = i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE, 0), e.buffer = h, i.flush();
  }
  /** Release the fence and readback buffer of a comparison, whether or not it was read. */
  #B(e) {
    const t = this.#i;
    e.fence !== null && t.deleteSync(e.fence), e.fence = null;
    const i = e.buffer;
    if (e.buffer = null, i === null) return;
    const s = this.#u;
    s !== null && i.bytes === s.width * s.height * 4 ? this.#s.push(i) : t.deleteBuffer(i.buffer);
  }
  /**
   * Return the texture of a removed take to the pool. This is safe because a
   * later take has replaced it as the reference and the comparison that read
   * it has been issued.
   */
  #a(e) {
    this.#E.push(e.texture);
  }
  #h() {
    if (this.#R === rt) return null;
    const e = this.#i, t = e.createTexture();
    return e.bindTexture(e.TEXTURE_2D, t), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), this.#R++, t;
  }
  #f() {
    const e = this.#i, t = R(e, nt, S);
    return e.useProgram(t), e.uniform1i(e.getUniformLocation(t, "uTake"), 0), e.uniform1i(e.getUniformLocation(t, "uReference"), 1), t;
  }
  #l(e, t) {
    const i = this.#i, s = this.#u;
    if (s?.width === e && s.height === t) return s;
    this.#r();
    for (const { buffer: a } of this.#s) i.deleteBuffer(a);
    this.#s = [];
    const r = i.createTexture();
    i.bindTexture(i.TEXTURE_2D, r), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MIN_FILTER, i.NEAREST), i.texParameteri(i.TEXTURE_2D, i.TEXTURE_MAG_FILTER, i.NEAREST), i.texImage2D(
      i.TEXTURE_2D,
      0,
      i.RGBA8,
      e,
      t,
      0,
      i.RGBA,
      i.UNSIGNED_BYTE,
      null
    );
    const o = i.createFramebuffer();
    i.bindFramebuffer(i.FRAMEBUFFER, o), i.framebufferTexture2D(
      i.FRAMEBUFFER,
      i.COLOR_ATTACHMENT0,
      i.TEXTURE_2D,
      r,
      0
    ), i.bindFramebuffer(i.FRAMEBUFFER, null);
    const h = { framebuffer: o, texture: r, width: e, height: t };
    return this.#u = h, h;
  }
  #r() {
    const e = this.#i;
    this.#u !== null && (e.deleteFramebuffer(this.#u.framebuffer), e.deleteTexture(this.#u.texture), this.#u = null);
  }
  #d(e) {
    const t = this.#s.pop();
    if (t !== void 0) return t;
    const i = this.#i, s = i.createBuffer();
    return i.bindBuffer(i.PIXEL_PACK_BUFFER, s), i.bufferData(i.PIXEL_PACK_BUFFER, e, i.STREAM_COPY), i.bindBuffer(i.PIXEL_PACK_BUFFER, null), { buffer: s, bytes: e };
  }
}
function ot(n) {
  for (let e = 0; e < n.length; e += 4)
    if (n[e] !== 0 || n[e + 1] !== 0 || n[e + 2] !== 0)
      return !0;
  return !1;
}
const xe = [
  "mozParsedFrames",
  "mozDecodedFrames",
  "mozPresentedFrames",
  "mozPaintedFrames"
];
function ge(n) {
  return xe.every((e) => e in n);
}
function at(n) {
  return n.ownerDocument?.defaultView?.performance.timeOrigin ?? performance.timeOrigin;
}
function At() {
  return typeof HTMLVideoElement < "u" && (ge(HTMLVideoElement.prototype) || typeof HTMLVideoElement.prototype.requestVideoFrameCallback == "function");
}
const ut = 250, $ = 500, lt = 1.75, ct = 3, ft = 200;
class dt {
  #i;
  #t;
  #e;
  /** The watch that owns the takes in #taken. */
  #u = null;
  /** Takes whose results are pending, oldest first. */
  #E = [];
  /** Frame count on the picture-watching path, reported as `presentedFrames`. */
  #R = 0;
  #s = null;
  /** Consecutive unchanged results for takes made at rest. */
  #n = 0;
  /** When the last delivered take was made, and the painted count when it was delivered. */
  #A = null;
  #o = null;
  #v = null;
  #T = null;
  #B = null;
  #a = null;
  #h = !0;
  #f = null;
  #l = null;
  #r = 0;
  #d;
  #g;
  #_ = null;
  #H = null;
  #x = null;
  #p = null;
  #M = 0;
  #c = [];
  #y = [];
  constructor(e, t) {
    if (this.#i = e, this.#e = t, this.#t = ge(e) ? e : null, this.#d = this.#t === null && typeof VideoFrame < "u", this.#g = this.#d && e.playbackRate > 1, this.#t) {
      for (const i of ["emptied", "seeking", "seeked"])
        e.addEventListener(i, this.#$);
      for (const i of ["pause", "playing", "waiting", "ratechange"])
        e.addEventListener(i, this.#O);
    }
    if (this.#d)
      for (const i of [
        "loadeddata",
        "playing",
        "pause",
        "ended",
        "seeking",
        "seeked",
        "emptied",
        "ratechange"
      ])
        e.addEventListener(i, this.#ie);
  }
  /** Whether acquisition uses the captured pictures' own timestamps. */
  get captureDriven() {
    return this.#g;
  }
  request(e) {
    if (this.#T === null) {
      if (this.#T = e, this.#g) {
        this.#Y();
        return;
      }
      this.#v = this.#t ? requestAnimationFrame(this.#P) : this.#i.requestVideoFrameCallback(this.#ee);
    }
  }
  cancel() {
    this.#v !== null && (this.#t ? cancelAnimationFrame(this.#v) : this.#i.cancelVideoFrameCallback(this.#v)), this.#v = null, this.#T = null, this.#_?.(), this.#_ = null, this.#x !== null && this.#i.cancelVideoFrameCallback(this.#x), this.#x = null, this.#oe(), this.#p = null, this.#c = [], this.#g = this.#d && this.#i.playbackRate > 1, this.#$();
  }
  destroy() {
    this.cancel();
    for (const e of ["emptied", "seeking", "seeked"])
      this.#i.removeEventListener(e, this.#$);
    for (const e of ["pause", "playing", "waiting", "ratechange"])
      this.#i.removeEventListener(e, this.#O);
    for (const e of [
      "loadeddata",
      "playing",
      "pause",
      "ended",
      "seeking",
      "seeked",
      "emptied",
      "ratechange"
    ])
      this.#i.removeEventListener(e, this.#ie);
  }
  /** Deliver pending input on the rendering window's refresh, before drawing. */
  flush(e) {
    if (this.#g)
      for (this.#i.ownerDocument !== this.#H && (this.#_?.(), this.#_ = null, this.#Y()); this.#y.length > 0 && this.#T !== null; ) {
        const t = this.#y.shift(), i = t.frame;
        try {
          this.#re(e, {
            width: i.visibleRect?.width ?? i.codedWidth,
            height: i.visibleRect?.height ?? i.codedHeight,
            mediaTime: i.timestamp / 1e6,
            presentedFrames: t.count,
            // A picture held until this refresh is scheduled no earlier than now.
            expectedDisplayTime: Math.max(t.at, e),
            timeOrigin: performance.timeOrigin,
            frame: i
          });
        } finally {
          i.close();
        }
      }
  }
  #oe() {
    for (const e of this.#y) e.frame.close();
    this.#y = [];
  }
  #ie = (e) => {
    const t = this.#d && this.#i.playbackRate > 1;
    if (t !== this.#g) {
      const i = this.#T;
      this.cancel(), this.#g = t, i !== null && this.request(i);
      return;
    }
    ["seeking", "seeked", "emptied", "ratechange"].includes(e.type) && (this.#p = null, this.#c = []), this.#g && ((e.type === "pause" || e.type === "ended") && this.flush(performance.now()), this.#oe(), this.#_?.(), this.#_ = null, this.#T !== null && this.#Y());
  };
  #Y() {
    if (!this.#g || this.#_ !== null || this.#T === null || (this.#i.paused || this.#i.ended) && this.#p !== null)
      return;
    this.#x === null && typeof this.#i.requestVideoFrameCallback == "function" && (this.#x = this.#i.requestVideoFrameCallback(
      this.#se
    ));
    const e = Math.max(4, 8 / Math.max(1, this.#i.playbackRate)), t = this.#i.ownerDocument?.defaultView;
    if (this.#H = this.#i.ownerDocument, t) {
      const i = t.setTimeout(this.#Q, e);
      this.#_ = () => t.clearTimeout(i);
    } else {
      const i = setTimeout(this.#Q, e);
      this.#_ = () => clearTimeout(i);
    }
  }
  #se = () => {
    this.#x = null, this.#_?.(), this.#_ = null, this.#Q();
  };
  #Q = () => {
    this.#_ = null;
    const e = this.#i;
    if (this.#T === null) return;
    if (e.readyState < 2 || e.seeking) {
      this.#Y();
      return;
    }
    let t;
    try {
      t = new VideoFrame(e);
    } catch (i) {
      if (!(i instanceof DOMException) || i.name !== "InvalidStateError")
        throw i;
      this.#Y();
      return;
    }
    if (t.timestamp === this.#p)
      t.close();
    else {
      let i = 1;
      if (this.#p !== null) {
        const s = t.timestamp - this.#p;
        if (s > 1e3 && s < 25e4 && (this.#c.push(s), this.#c.length > 7 && this.#c.shift()), s > 0 && this.#c.length > 0) {
          const r = [...this.#c].sort((h, a) => h - a), o = r[Math.floor(r.length / 2)];
          i = Math.max(1, Math.round(s / o));
        }
      }
      for (this.#p = t.timestamp, this.#M += i, this.#y.push({
        frame: t,
        at: performance.now(),
        count: this.#M
      }); this.#y.length > 4; ) this.#y.shift().frame.close();
    }
    this.#Y();
  };
  #O = () => {
    this.#f = null, this.#l = null, this.#r = 0;
  };
  #$ = () => {
    this.#B = null, this.#a = null, this.#h = !0, this.#U(), this.#O();
  };
  /** Discard all takes. The next take has no reference to compare with. */
  #U() {
    this.#u?.reset(), this.#E = [], this.#n = 0;
  }
  /** Native acquisition: pass the report on, with the clock it was made on. */
  #ee = (e, t) => {
    this.#re(e, {
      width: t.width,
      height: t.height,
      mediaTime: t.mediaTime,
      presentedFrames: t.presentedFrames,
      expectedDisplayTime: t.expectedDisplayTime,
      timeOrigin: at(this.#i)
    });
  };
  #re = (e, t) => {
    const i = this.#T;
    this.#v = null, this.#T = null, i?.(e, t);
  };
  #P = (e) => {
    const t = this.#t, i = xe.map((a) => t[a]);
    this.#B?.some((a, u) => i[u] < a) && this.#$(), this.#B = i;
    const s = this.#e?.() ?? null;
    s !== this.#u && (this.#$(), this.#u = s);
    const r = t.mozPaintedFrames, o = !t.seeking && t.readyState >= 2 && t.videoWidth > 0 && t.videoHeight > 0, h = this.#a === null && (r > 0 || t.paused && (t.mozPresentedFrames > 0 || t.mozDecodedFrames > 0));
    if (s !== null) {
      this.#pe(e, t, s, o, h);
      return;
    }
    if (o && (h || this.#a !== null && r !== this.#a)) {
      this.#f !== null && e - this.#f > $ && (this.#O(), this.#h = !0), this.#K(e, t), this.#f = e, this.#a = r;
      const a = this.#h;
      this.#h = !1, this.#re(e, {
        width: t.videoWidth,
        height: t.videoHeight,
        // Used only to select source scan/size metadata on the media timeline.
        // It is deliberately NOT used as the frame identity or field clock.
        mediaTime: t.currentTime,
        presentedFrames: r,
        // The counters report no display time. The poll that found the frame
        // is the best stand-in, and it was taken on this module's clock --
        // not the video node's window, which may be a different one.
        expectedDisplayTime: e,
        timeOrigin: performance.timeOrigin,
        mozTiming: { periodMs: this.#r, discontinuity: a }
      });
    } else
      this.#v = requestAnimationFrame(this.#P);
  };
  /** Update the cadence estimate from the painted count. Called on a poll where the count advanced. */
  #K(e, t) {
    if (t.paused || t.ended) return;
    const i = t.mozPaintedFrames, s = this.#l;
    if (s && e - s.at >= ut) {
      const r = i - s.frames;
      if (r > 0) {
        const o = (e - s.at) / r;
        o >= 4 && o <= 200 && (this.#r = this.#r ? this.#r + (o - this.#r) * 0.25 : o);
      }
      this.#l = null;
    }
    this.#l ??= { at: e, frames: i };
  }
  /**
   * Poll that finds new frames by comparing pictures. Every refresh takes the
   * element's picture. A take is delivered once its result is known, one or
   * more refreshes after it was made.
   */
  #pe(e, t, i, s, r) {
    const o = t.mozPaintedFrames, h = this.#a !== null && o !== this.#a;
    this.#s !== null && e - this.#s > $ && (this.#U(), this.#O(), this.#h = !0), this.#s = e, h && this.#K(e, t);
    const a = t.paused || t.ended;
    (!a || h) && (this.#n = 0), a && this.#A !== null && (this.#A = e);
    const u = a && this.#n >= ct;
    if (s && (r || this.#a !== null)) {
      this.#a = o;
      const l = u ? null : i.take();
      l !== null && this.#E.push({
        id: l,
        at: e,
        mediaTime: t.currentTime,
        width: t.videoWidth,
        height: t.videoHeight,
        resting: a
      });
    }
    let A = null;
    for (let l = i.settle(); l !== null; l = i.settle()) {
      const c = this.#E.shift();
      if (c?.id !== l.id)
        throw new Error("picture verdicts arrived out of order");
      let m = l.changed;
      if (m ? this.#n = 0 : (this.#n = c.resting ? this.#n + 1 : 0, m = A === null && !c.resting && this.#A !== null && (c.at - this.#A) * (t.playbackRate || 1) >= ft && o !== this.#o), !m) {
        i.release(c.id);
        continue;
      }
      A !== null && i.release(A.id), A = c;
    }
    if (A === null) {
      this.#v = requestAnimationFrame(this.#P);
      return;
    }
    let f = 1;
    if (this.#f !== null) {
      const l = A.at - this.#f;
      l > $ ? (this.#O(), this.#h = !0) : this.#r > 0 && l > this.#r * lt && (f = Math.round(l / this.#r));
    }
    this.#R += f, this.#f = A.at, this.#A = A.at, this.#o = o;
    const d = this.#h;
    this.#h = !1;
    try {
      this.#re(e, {
        width: A.width,
        height: A.height,
        // As on the counter path, this only places the frame on the media
        // timeline. It is the clock value when the picture was taken.
        mediaTime: A.mediaTime,
        presentedFrames: this.#R,
        // The picture is delivered now, not when it was taken, and the
        // schedule is anchored to the delivery time.
        expectedDisplayTime: e,
        timeOrigin: performance.timeOrigin,
        mozTiming: { periodMs: this.#r, discontinuity: d },
        picture: A.id
      });
    } finally {
      i.release(A.id);
    }
  }
}
let we = null;
function mt(n) {
  we = n;
}
const V = 0.5, T = 4, Q = 5, w = Q + 1, re = 1e3, ne = 4, k = 200, pt = 0.25, Et = 1e3 / 60, bt = `#version 300 es
void main() {
  // One triangle over the whole viewport, from the vertex index alone. There
  // is no geometry here worth a buffer: every pixel is the fragment shader's.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`, vt = 0.75, Tt = 0.1, he = 1, xt = 60, gt = 0.02, wt = 0.1, O = 1, Ft = 0.02, Rt = 5e-3, yt = 250, oe = 1e3, ae = 16, Dt = 4, _t = 90, Mt = 1.5, Ae = 4, Nt = 24, M = 1e-3, G = 5, ue = 4, le = {
  2: 0,
  3: 0.25,
  4: 0.5,
  5: 0.75
};
function ce(n) {
  return Pt[(n.interlaced ? 2 : 0) + (n.topFieldFirst ? 1 : 0)];
}
const St = `#version 300 es
precision highp float;
uniform sampler2D uField;
uniform bool uFlip;
out vec4 fragColor;
void main() {
  ivec2 position = ivec2(gl_FragCoord.xy);
  if (uFlip) position.y = textureSize(uField, 0).y - 1 - position.y;
  fragColor = texelFetch(uField, position, 0);
}
`, Pt = [
  { interlaced: !1, topFieldFirst: !1 },
  { interlaced: !1, topFieldFirst: !0 },
  { interlaced: !0, topFieldFirst: !1 },
  { interlaced: !0, topFieldFirst: !0 }
];
function Wt() {
  return At() && typeof WebGL2RenderingContext < "u";
}
const Ct = {
  requestAnimationFrame: (n) => requestAnimationFrame(n),
  cancelAnimationFrame: (n) => cancelAnimationFrame(n)
}, It = {
  setTimeout: (n, e) => setTimeout(n, e),
  clearTimeout: (n) => clearTimeout(n)
};
class Ht extends EventTarget {
  /**
   * Receive decoded pictures with their presentation times. From the first
   * push on they are the only pictures shown, and the element is only the
   * clock they are shown by.
   */
  decodedVideo;
  #i;
  #t;
  #e;
  #u;
  /** Finds Firefox's new pictures for the main-thread renderer. */
  #E;
  /** The take holding the picture of the frame being ingested, if any. */
  #R = null;
  #s;
  /** The pulldown detection, built only while the `film` option needs it. */
  #n = null;
  #A;
  #o;
  /** The program that copies a filtered picture onto the canvas. */
  #v;
  #T;
  #B;
  #a = [];
  /** Somewhere to filter a field into, and to read it back out of. */
  #h = [];
  /** Which output slot was written last; the next one follows round the ring. */
  #f = w - 1;
  /** The draw path currently shown on the canvas, retained for snapshots. */
  #l = null;
  /** Filtered fields waiting for their moment, oldest first. */
  #r = [];
  /** The requestAnimationFrame() loop that puts them up, which is all that draws on the canvas. */
  #d = null;
  /** The reading of the element's clock a Worker renderer is to be sent before its next frame. */
  #g = null;
  /** When the last reading went to the Worker renderer. */
  #_ = Number.NEGATIVE_INFINITY;
  #H = 0;
  /** Deliver captured pictures on the page's refresh during faster playback. */
  #x = null;
  /** The document the owner-change listener is on, if any. */
  #p = null;
  #M;
  /** The gap between animation frames: as near as the page gets to the screen. */
  #c = Et;
  /** Where the refresh grid fitted to the animation frames stands. (otya) */
  #y = 0;
  /** How far ahead of its animation frame the last picture shown stood. (otya) */
  #oe = 0;
  #ie = 0;
  #Y = 0;
  /** The last picture chained onto the schedule; a break restarts it. (otya) */
  #se = null;
  /** The lateness the schedule holds on purpose, in refreshes. See #schedule. */
  #Q = 0;
  /** Pictures in a row that arrived off the phase the lag stands for, all on one side of it. */
  #O = 0;
  /** Of those, the phase closest to the lag's phase, in refreshes. */
  #$ = 0;
  /**
   * Drop the presentation queue and restart the schedule from the clock.
   * Every schedule restart goes through here (otya nulls #lastScheduled at
   * each queue clear): the old chain's clock no longer applies, so keeping
   * it would either count a phantom resync or pop freshly queued pictures
   * as late. Restart accounting for cadence changes lives in #schedule.
   */
  #U() {
    this.#r.length = 0, this.#Ye.length = 0, this.#se = null, this.#Q = 0, this.#O = 0, this.#Y = 0;
  }
  /** The `<div>` this put around the element, so it can be taken away again. */
  #ee = null;
  #re;
  #P;
  #K;
  #pe = 0;
  /** otya GPU pulldown path: enabled by the `film` option (see below). */
  #ae;
  #X;
  /** How long a frame lasts in wall time, from what the frames themselves say. */
  #L = 0;
  /** The size of a frame as it is coded, which is what a texture holds. */
  #C = 0;
  #G = 0;
  /** Where the newest frame is. The two before it follow round the ring. */
  #D = T - 1;
  /** How many of the held frames are consecutive, up to HISTORY. */
  #N = 0;
  #et = 0;
  /** presentedFrames at the last ingested frame; pairs with #lastMediaTime. */
  #Jt = 0;
  /** A destination frame that arrived before the browser finished seeking. */
  #tt = !1;
  #I = !1;
  /** Cancels work suspended inside a synchronous owner callback. */
  #ne = 0;
  #Le = !1;
  #J = !1;
  #k = null;
  #ke = [];
  #te = !1;
  /** Why the renderer stopped for good, or null; see `#rendererFailed`. */
  #Ft = null;
  #ei;
  #ti;
  #S;
  #Oe;
  #Ee;
  #ii;
  #m = null;
  #w;
  #it = !1;
  #si = 0;
  #ri = !1;
  #_s = 0;
  #st = !1;
  #Rt = !1;
  #Ge = null;
  #Ms = 0;
  #rt = /* @__PURE__ */ new Map();
  /** Everything the next report is counted from. See DeinterlaceStats. */
  #F = {
    filtered: 0,
    missed: 0,
    degraded: 0,
    discontinuities: 0,
    resynced: 0,
    late: 0,
    queueResetted: 0,
    matched: 0
  };
  /** `presentedFrames` of the last frame the callback saw; 0 before any. */
  #nt = 0;
  /** When the last frame the filter took arrived, to see the gaps between. */
  #ht = 0;
  #yt = 0;
  #Ae = 0;
  #ye = 0;
  #be = 0;
  #De = 0;
  #ve = 0;
  #Xe;
  #Dt = [];
  #ze = [];
  #ot = 0;
  #We = 0;
  #at = 0;
  #He = 0;
  /** The last phase read back from the GPU, and how many frames ago it was for. */
  #At = D;
  #$e = 0;
  /** The phase of the frame being filtered: #known advanced by #knownAge. */
  #z = D;
  #Z = !1;
  /** Frames left of the FILM_HOLD_FRAMES window that starts when the cadence is held. */
  #W = 0;
  /** Whether the comb counts were made for the frame being filtered and its neighbours. */
  #_t = !1;
  /** Whether the frame being filtered may have its fields matched. See #render. */
  #_e = !1;
  /** Whether the last detection read back found each frame to be one instant. */
  #ni = !1;
  /** Whether comb counts were also made for the frame after it. */
  #hi = !1;
  /** Whether the frame behind the newest (the one being filtered) came after a gap in the frames seen. */
  #oi = !1;
  /** Whether the previous frame was progressive. */
  #ut = !1;
  #Ve = null;
  #Li = "";
  /** Debug only: frames given each phase (0 for none), and repeats dropped. */
  #Mt = [0, 0, 0, 0, 0, 0];
  #Nt = 0;
  /**
   * Why requested film reconstruction is currently degraded, or null while
   * healthy. The `film` option stays as the caller set it;
   * only the engine stands down, so this is never a silent option change.
   */
  #Me = null;
  /** Last worker-reported filmError, to derive the page-side failure event. */
  #ai = null;
  #Ai = 0;
  /** Whether pictures come from `decodedVideo`. Set by the first push and kept. */
  #b = !1;
  /** Pushed pictures not yet taken in, in timestamp order. */
  #Ne = [];
  /** Whether finish() was called since the last reset. */
  #ui = !1;
  #j = new st();
  /**
   * Made with the rest of the GPU setup, so that the first decoded picture
   * does not draw with its program first; null once destroyed.
   */
  #lt;
  /** Whether the GPU has run this renderer's setup; see `#watchPrepared`. */
  #ki = !1;
  /** The decoded picture in each history slot; the newest is at #head. */
  #Te = Array.from({ length: T }, () => ({
    timestamp: 0,
    duration: 0,
    interlaced: !0,
    topFieldFirst: !0,
    repeatFirstField: !1
  }));
  /** Whether a decoded picture has been taken in since the last reset. */
  #li = !1;
  /** When the last picture pushed since the last reset starts. */
  #St = Number.NEGATIVE_INFINITY;
  /** Whether the newest picture is still to be filtered against its successor. */
  #Se = !1;
  /** Whether it has been filtered against itself meanwhile, its successor being late. */
  #ct = !1;
  /** Filtered decoded outputs waiting for their media time, in time order. */
  #Ye = [];
  /** The decoded output on the canvas. */
  #Pt = null;
  /** What makes the pictures between pictures, and the pair it was last handed. */
  #Pe = null;
  #ft = null;
  /** What puts the pictures up at a size of its own, and the size it chose. */
  #xe = null;
  #dt = null;
  /** The size the canvas is shown at, in device pixels. */
  #Qe = null;
  /** A picture-sized texture to filter or interpolate into for the scaler. */
  #ge = null;
  #Ct;
  /** The media time of the output on the canvas; nothing older goes up after it. */
  #mt = Number.NEGATIVE_INFINITY;
  /**
   * Decoded pictures are picked against a count of refreshes rather than the
   * clock read at each one; see `#countRefresh`. Whether the count runs, the
   * media time it has reached, and the refresh (wall ms) that time is for.
   */
  #he = !1;
  #ue = 0;
  #Ze = 0;
  /** The playback speed the count last moved at. */
  #pt = 1;
  /** A start announced through `startsAt` and not yet reached, on this realm's clock. */
  #Ce = null;
  /**
   * The refresh (wall ms) the count began at from an announced start or
   * change of speed, while
   * no reading of the clock has confirmed it yet; NaN otherwise.
   */
  #le = Number.NaN;
  /**
   * The step decoded pictures change on -- a field, or a frame of
   * progressive video -- in seconds, from the last picture taken; 0 before.
   */
  #Oi = 0;
  /** The sound less the shown output, averaged, and when it was last taken; see `avDifference`. */
  #ce = Number.NaN;
  #Gi = 0;
  /** `avDifference` as the Worker renderer last reported it. */
  #ci = Number.NaN;
  /** Field order of the decoded picture being filtered. */
  #fi = !0;
  /** The size the decoded pictures are shown at. */
  #Et = 0;
  #bt = 0;
  /** The element's own inline aspect ratio, from before decoded pictures gave it theirs. */
  #di = null;
  /** Whether the element answers for the decoded pictures; see `#standInForElement`. */
  #It = !1;
  /** Decoded pictures pushed since the counts were last reset. */
  #mi = 0;
  /** Pictures a Worker renderer dropped unshown, as of its last report. */
  #pi = 0;
  /** Whether the page showing the canvas is hidden; it is then given no refreshes. */
  #Bt = !1;
  /**
   * Whether no refresh has been filled since the page was hidden: while it
   * is hidden, and after it shows again until the first refresh it is given,
   * which comes some time after the page says it is visible. Nothing that
   * went by meanwhile could be seen.
   */
  #Ut = !1;
  /**
   * The position of the first refresh filled after the page was last hidden,
   * and the clock's origin then. The position bounds what passed unseen only
   * for as long as playback has been continuous since, which the origin
   * tells. It is the element's own position where the clock has not caught
   * up with it: a hidden page sends no readings, so playback resumed while
   * it was hidden finds the clock still where the element was paused.
   */
  #Xi = Number.NaN;
  #zi = Number.NaN;
  /**
   * Buffers transferred to the Worker and not yet back, by the id they went
   * with. A Worker that goes away takes them with it, and what is handed
   * back for them then is the detached original.
   */
  #Lt = /* @__PURE__ */ new Map();
  #Ns = 0;
  constructor(e, t = {}, i = null) {
    super(), this.#e = e, this.#P = t.doubleRate ?? !1, this.#K = t.spatialCheck ?? !0, this.#ae = t.debug ?? !1, this.#X = t.film ?? !1, this.#Pe = t.interpolator ?? null, this.#xe = t.scaler ?? null, this.#ei = t.onStats, this.#ti = t.onFailure, this.#S = i, this.#Ee = i ? "main" : t.rendering ?? "main", this.#ii = t.workerUrl ?? we, this.#w = this.#Ee === "main" ? "main" : "idle", this.#t = i ? i.canvas : document.createElement("canvas"), this.#i = i?.canvas ?? (this.#Ee === "main" ? this.#t : document.createElement("canvas")), this.#Oe = e, i || (this.#t.style.cssText = "position:absolute;pointer-events:none;visibility:hidden");
    const s = this.#i.getContext("webgl2", {
      // Keep the underlying video in Chromium's compositor to receive frame notifications at the source rate.
      alpha: !0,
      antialias: !1,
      depth: !1,
      stencil: !1,
      preserveDrawingBuffer: !1,
      powerPreference: "high-performance"
    });
    if (!(s instanceof WebGL2RenderingContext))
      throw new Error("this browser has no WebGL2");
    this.#s = s, this.#X && (this.#$i(), this.#n = new J(s)), this.#A = fe(s, Ze);
    const r = this.#A;
    this.#o = Object.fromEntries(
      Object.entries(Qe).map(([o, h]) => [
        o,
        s.getUniformLocation(r, h)
      ])
    ), this.#v = fe(s, St), this.#T = s.getUniformLocation(this.#v, "uField"), this.#B = s.getUniformLocation(this.#v, "uFlip"), y(s, this.#A, s.RGBA8), y(s, this.#v, s.RGBA8), this.#lt = new Je(s), this.#Ps(), this.#Xe = s.getExtension(
      "EXT_disjoint_timer_query_webgl2"
    ), this.#i.addEventListener(
      "webglcontextlost",
      this.#Ds
    ), this.#re = i ? null : new ResizeObserver(() => this.#Je()), this.#Ct = i ? null : new ResizeObserver(([o]) => {
      if (!o) return;
      const h = window.devicePixelRatio || 1, a = {
        width: Math.round(o.contentRect.width * h),
        height: Math.round(o.contentRect.height * h)
      }, u = o.devicePixelContentBoxSize?.[0];
      this.#Qe = u && Math.abs(u.inlineSize - a.width) <= 1 && Math.abs(u.blockSize - a.height) <= 1 ? { width: u.inlineSize, height: u.blockSize } : a, this.#Ui();
    }), this.#M = i ? null : new MutationObserver(() => {
      this.#t.ownerDocument !== this.#p && this.#Je();
    }), this.decodedVideo = {
      push: (o) => this.#Hs(o),
      reset: () => this.#qi(),
      finish: () => {
        this.#ui = !0, this.#m?.postMessage({
          type: "pictures",
          action: "finish"
        });
      },
      onrelease: null,
      avDifference: () => this.#m ? this.#ci : this.#ce,
      startsAt: (o, h) => {
        if (!Number.isFinite(o) || !Number.isFinite(h))
          throw new RangeError(`start at ${o} s, ${h} ms`);
        if (this.#m) {
          this.#m.postMessage({
            type: "start",
            media: o,
            at: h,
            timeOrigin: performance.timeOrigin,
            video: this.#Ie()
          });
          return;
        }
        this.#Ce = { media: o, wall: h };
      },
      rendererStatus: () => this.#Ft !== null ? { state: "failed", reason: this.#Ft } : { state: (this.#w === "main" ? this.#ki : this.#w === "active") ? "prepared" : "preparing" },
      onrendererstatus: null
    }, this.#E = i ? null : new ht(s, e), this.#u = new dt(
      e,
      () => this.#w === "main" ? this.#E : null
    ), e.addEventListener("emptied", this.#ws), e.addEventListener("resize", this.#Ts), e.addEventListener("pause", this.#Re), e.addEventListener("ended", this.#Re), e.addEventListener("seeking", this.#Rs), e.addEventListener("seeked", this.#Re), e.addEventListener("ratechange", this.#Re), e.addEventListener("waiting", this.#ys);
  }
  /**
   * Whether the canvas, rather than the element, is what shows the picture.
   * A page uses this to decide which of the two to capture or transform.
   * Filtering an element's own picture that is the same as filtering it at
   * all, since a progressive source is left to the element. Decoded pictures
   * are on the canvas filtered or not, because the element has none.
   */
  get running() {
    return this.#b ? this.#I : this.#I && (this.#k?.interlaced ?? !0);
  }
  /** The HTML canvas currently placed over the media element. */
  get canvas() {
    return this.#t;
  }
  /** Field order for the current scan state, defaulting to top-field-first. */
  get #kt() {
    return this.#b ? this.#fi : this.#k?.topFieldFirst !== !1;
  }
  /** The same public filter options for either rendering target. */
  #Wi() {
    return {
      doubleRate: this.#P,
      spatialCheck: this.#K,
      film: this.#X,
      debug: this.#ae
    };
  }
  /** Whether the caller wants filtering, independently of the current source. */
  get enabled() {
    return this.#Le;
  }
  set enabled(e) {
    this.#Le = e, this.#Gt(), this.#m?.postMessage({
      type: "enabled",
      enabled: e
    });
  }
  /** Update whether the source needs filtering and which field comes first. */
  set scan(e) {
    if (this.#b) return;
    const t = this.#k?.interlaced !== e?.interlaced, i = t || this.#k?.topFieldFirst !== e?.topFieldFirst;
    this.#k = e, !(i && (!this.#qe() || this.#k !== e)) && (this.#m?.postMessage({ type: "scan", scan: e }), i && (this.#N = 0, this.#U(), this.#q(), t && (this.#L = 0), this.#l = null, this.#Fe(!1)), this.#Gt(), i && ((e?.interlaced ?? !0) && (this.#S || this.#w === "main") ? this.#Be() : this.#Fi()));
  }
  get scan() {
    return this.#b ? this.#Pt?.scan ?? null : this.#k;
  }
  set videoTimeline(e) {
    this.#ke = e, this.#m?.postMessage({
      type: "timeline",
      videoTimeline: e
    }), e.length === 0 && (this.#k = null), this.#Gt();
  }
  get videoTimeline() {
    return this.#ke;
  }
  /**
   * What to put on the screen for fullscreen: the `<div>` holding both the
   * element and the canvas once there is one, and the element itself before
   * that. Fullscreening the element alone would leave the canvas behind in
   * the page, and with it the only deinterlaced picture there is.
   */
  get container() {
    return this.#ee ?? this.#e;
  }
  /** Whether a picture goes up for every field rather than every frame. */
  get doubleRate() {
    return this.#P;
  }
  set doubleRate(e) {
    e !== this.#P && (this.#P = e, this.#je(), !this.#b && (this.#U(), this.#Hi()));
  }
  get spatialCheck() {
    return this.#K;
  }
  set spatialCheck(e) {
    e !== this.#K && (this.#K = e, this.#je());
  }
  get film() {
    return this.#X;
  }
  set film(e) {
    const t = this.#m ? this.#ai : this.#Me;
    if (!(e === this.#X && (e === !1 || t === null))) {
      if (this.#m) {
        this.#X = e, this.#je(e ? "film" : void 0);
        return;
      }
      if (e) {
        if (!this.#qe()) return;
        try {
          this.#Vi();
        } catch (i) {
          this.#X = !0, this.#je(), this.#Xt(
            `film detector unavailable: ${i instanceof Error ? i.message : String(i)}`
          );
          return;
        }
      }
      if (this.#X = e, this.#je(), !e) {
        if (!this.#qe()) return;
        this.#Z = !1, this.#W = 0, this.#_e = !1, this.#z = D, this.#q(), this.#n?.destroy(), this.#n = null;
      }
      this.#Hi();
    }
  }
  get debug() {
    return this.#ae;
  }
  set debug(e) {
    e !== this.#ae && (this.#ae = e, this.#je());
  }
  get interpolator() {
    return this.#Pe;
  }
  set interpolator(e) {
    e !== this.#Pe && (this.#Pe = e, this.#ft = null);
  }
  get scaler() {
    return this.#xe;
  }
  /**
   * Setting the same scaler again asks it for the size afresh, for a page
   * that changed something the size depends on that this cannot see.
   */
  set scaler(e) {
    this.#xe = e, this.#Ui();
  }
  /** Whether pictures are queued for presentation by the frame loop. */
  get #Ss() {
    return this.#P || this.#X;
  }
  #Hi() {
    this.#J || this.#b || (this.#Ss ? (this.#C > 0 && this.#vs(), (this.#k?.interlaced ?? !0) && (this.#S || this.#w === "main") && this.#Be()) : this.#X || (this.#l = null, this.#Fe(!1), this.#wt()));
  }
  /** Apply mutable filter settings to the existing Worker and canvas. */
  #je(e) {
    this.#m?.postMessage({
      type: "settings",
      options: this.#Wi(),
      retryFilm: e
    });
  }
  #$i() {
    if (this.#s.getExtension("EXT_color_buffer_float") === null)
      throw new Error("film needs EXT_color_buffer_float");
  }
  /**
   * Note when the GPU has run the setup issued so far, every program's first
   * draw included (see `drawOnce`). The draws only queue the work, and how
   * long it takes shows nowhere but here; until then a picture can miss the
   * refresh it is due in, which is why a start waits for `rendererStatus()`
   * to say so.
   */
  #Ps() {
    const e = this.#s, t = e.fenceSync(e.SYNC_GPU_COMMANDS_COMPLETE, 0);
    e.flush();
    const i = this.#ms(), s = () => {
      if (e.clientWaitSync(t, 0, 0) === e.TIMEOUT_EXPIRED) {
        i.setTimeout(s, ae);
        return;
      }
      e.deleteSync(t), this.#ki = !0, this.#w === "main" && this.decodedVideo.onrendererstatus?.();
    };
    i.setTimeout(s, ae);
  }
  /**
   * Stop for good, for `reason`: the renderer cannot go on. The element's
   * own picture shows again, and decoded pictures, which have none to fall
   * back on, are left to their producer through `rendererStatus()`; nothing
   * starts this renderer again (see `start()`), and its owner makes a new
   * one if it wants another. A renderer in a Worker reports to the page
   * instead, which tries one new Worker before it gives up.
   */
  #Ot(e) {
    if (this.#Ft = e, this.#S) {
      this.#S.onFailure(e), this.stop();
      return;
    }
    this.#Qi(e), this.stop(), this.decodedVideo.onrendererstatus?.();
  }
  /** Build the GPU pulldown detector the `film` option needs, or throw. */
  #Vi() {
    this.#$i(), this.#n ??= new J(this.#s), this.#C > 0 && this.#n.resize(this.#C, this.#G);
  }
  #Gt() {
    this.#b || this.#Le && (this.#ke.length > 0 || (this.#k?.interlaced ?? !0)) ? this.start() : this.stop();
  }
  /** Start the bundled Worker when the required transfer APIs are available. */
  #Cs() {
    return this.#S || this.#Ee === "main" ? !1 : this.#w === "starting" || this.#w === "active" ? !0 : typeof Worker < "u" && typeof VideoFrame < "u" && typeof OffscreenCanvas < "u" && this.#ii !== null && "transferControlToOffscreen" in HTMLCanvasElement.prototype ? (this.#Yi(), !0) : this.#Ee === "auto" ? (this.#zt(), !1) : (this.#w = "failed", this.#Ot(
      "rendering in a Worker needs Worker, VideoFrame, OffscreenCanvas transfer and a worker URL, and this page lacks one"
    ), !0);
  }
  /** Transfer the display canvas to a new Worker, replacing it if already transferred. */
  #Yi() {
    this.#fe(), this.#m?.terminate(), this.#m = null, this.#Ht(), this.#st = !1, this.#Rt = !1, this.#ai = null, this.#Ai = 0;
    let e = this.#t;
    if (this.#ri) {
      e = document.createElement("canvas"), e.className = this.#t.className;
      const r = this.#t.getAttribute("style");
      r === null ? e.removeAttribute("style") : e.setAttribute("style", r), e.style.visibility = "hidden", this.#t.parentElement && this.#t.replaceWith(e), this.#t = e;
    }
    const t = ++this.#si;
    this.#w = "starting", this.decodedVideo.onrendererstatus?.();
    let i, s;
    try {
      s = e.transferControlToOffscreen(), this.#ri = !0, i = new Worker(this.#ii, { type: "module" });
    } catch (r) {
      this.#vt(
        r instanceof Error ? r.message : String(r)
      );
      return;
    }
    this.#m = i, i.onmessage = (r) => {
      t === this.#si && this.#Is(r.data);
    }, i.onerror = (r) => {
      t === this.#si && (r.preventDefault(), this.#vt(r.message || "the deinterlacer worker failed"));
    }, i.postMessage(
      {
        type: "initialize",
        canvas: s,
        options: this.#Wi(),
        scan: this.#k,
        videoTimeline: this.#ke,
        enabled: this.#I,
        video: this.#Ie()
      },
      [s]
    ), this.#Bt && i.postMessage({
      type: "pageVisibility",
      hidden: !0,
      video: this.#Ie()
    });
  }
  /** Apply Worker notifications and release the wait for the next input frame. */
  #Is(e) {
    switch (e.type) {
      case "ready":
        this.#w = "active", this.decodedVideo.onrendererstatus?.(), this.#I && (this.#Tt(), this.#jt());
        break;
      case "failed":
        this.#vt(e.message);
        break;
      case "consumed": {
        this.#st = !1, this.#Rt = !0;
        const t = this.#Ge;
        this.#Ge = null, t && this.#ji(t);
        break;
      }
      case "visibility":
        this.#t.style.visibility = e.visible ? "visible" : "hidden";
        break;
      case "stats": {
        this.#b && (this.#pi = e.stats.missed + e.stats.queueResetted), this.#ci = e.stats.avDifference;
        const t = {
          ...e.stats,
          dropped: this.#e.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0
        }, i = t.filmError ?? null, s = this.#Ai;
        if (this.#ai = i, this.#Ai = e.filmFailure, i !== null && e.filmFailure !== s) {
          this.dispatchEvent(
            new CustomEvent("failure", { detail: i })
          );
          try {
            this.#ti?.(i);
          } catch {
          }
        }
        this.dispatchEvent(new CustomEvent("stats", { detail: t })), this.#ei?.(t);
        break;
      }
      case "capture": {
        const t = this.#rt.get(e.id);
        if (this.#rt.delete(e.id), !t) {
          e.image?.close();
          break;
        }
        e.image ? t.resolve(e.image) : this.#b ? t.reject(new Error("no rendered picture is available")) : createImageBitmap(this.#e).then(
          t.resolve,
          t.reject
        );
        break;
      }
      case "release":
        this.#Lt.delete(e.id) && this.#de(e.data);
        break;
    }
  }
  /**
   * Tell the owner that a rendering resource failed after construction.
   * In the worker domain the boundary counts episodes: the reason travels in
   * `stats.filmError` and the page derives the event from the episode count,
   * so a film degradation is never mistaken for worker death.
   */
  #Qi(e) {
    if (this.dispatchEvent(new CustomEvent("failure", { detail: e })), !this.#S)
      try {
        this.#ti?.(e);
      } catch {
      }
  }
  /**
   * Stand down film reconstruction without touching the caller's options.
   * Pictures keep flowing through plain YADIF; the reason is reported once
   * per episode through the failure event/option and `stats.filmError`.
   * Silently turning the `film` option off is not a fallback -- it would
   * rewrite the caller's 24fps intent -- so the option stays on and only
   * the engine stands down.
   */
  #Xt(e) {
    this.#Me !== e && (this.#Me = e, this.#Z = !1, this.#W = 0, this.#_e = !1, this.#z = D, this.#q(), this.#n?.destroy(), this.#n = null, this.#Qi(e));
  }
  /**
   * Re-arm film reconstruction at a resource-reallocation point (start, scan
   * change, resize, or re-setting the option). The next frame retries; a
   * repeated failure degrades again and notifies as a new episode.
   */
  #qe() {
    return this.#J ? !1 : (this.#m || (this.#Me = null), !0);
  }
  /** Retry a transient Worker failure once, then show the media element if it fails again. */
  #vt(e) {
    if (this.#w === "starting" && this.#Ee === "auto" && !this.#it) {
      this.#zt();
      return;
    }
    if (this.#Zi(e), !this.#it) {
      this.#it = !0, this.#Yi();
      return;
    }
    console.error(`Deinterlacer Worker stopped: ${e}`), this.#w = "failed", this.#m?.terminate(), this.#m = null, this.#fe(), this.#Ht(), this.#Ot(`deinterlacer worker stopped: ${e}`);
  }
  /** Restore the main-thread canvas when automatic Worker selection fails. */
  #zt() {
    const e = this.#i;
    e.className = this.#t.className;
    const t = this.#t.getAttribute("style");
    t === null ? e.removeAttribute("style") : e.setAttribute("style", t), e.style.visibility = "hidden", this.#t.parentElement && this.#t.replaceWith(e), this.#t = e, this.#ri = !1, this.#m?.terminate(), this.#m = null, this.#w = "main", this.#fe(), this.#Ht(), this.decodedVideo.onrendererstatus?.(), this.#I && (this.#Tt(), this.#jt(), (this.#k?.interlaced ?? !0) && this.#Be());
  }
  /** Close the queued frame still owned by the page when switching renderers. */
  #fe() {
    this.#Ge?.frame.close(), this.#Ge = null;
  }
  /** Reject captures whose replies would be lost when the Worker is rebuilt. */
  #Zi(e) {
    for (const t of this.#rt.values())
      t.reject(new Error(e));
    this.#rt.clear();
  }
  start() {
    if (!(this.#I || this.#J || this.#Ft !== null) && (this.#ne++, this.#I = !0, this.#Fs(), !!this.#qe())) {
      if (this.#Er(), this.#jt(), this.#Cs()) {
        if (this.#b) {
          this.#m?.postMessage({
            type: "running",
            running: !0
          }), this.#Be();
          return;
        }
        this.#m?.postMessage({
          type: "enabled",
          enabled: !0
        }), this.#w === "active" && this.#Tt();
        return;
      }
      this.#Tt(), (this.#b || (this.#k?.interlaced ?? !0)) && this.#Be();
    }
  }
  /**
   * Take the deinterlaced picture away, leaving the element's own showing.
   * Decoded pictures held are released, and those pushed until the next
   * start() are released at once.
   */
  stop() {
    this.#ne++, this.#I && (this.#I = !1, this.#b && this.#qi(), this.#u.cancel(), this.#_i(), this.#Fi(), this.#N = 0, this.#l = null, this.#Fe(!1), this.#fe(), this.#m?.postMessage(
      this.#b ? { type: "running", running: !1 } : { type: "enabled", enabled: !1 }
    ));
  }
  destroy() {
    if (!this.#J) {
      this.#J = !0, this.#Le = !1, this.stop(), this.#m?.postMessage({ type: "destroy" }), this.#m?.terminate(), this.#m = null, this.#fe(), this.#Ht(), this.#Zi("the deinterlacer was destroyed"), this.#p?.removeEventListener(
        "visibilitychange",
        this.#Di
      ), this.#p = null, this.#M?.disconnect(), this.#i.removeEventListener(
        "webglcontextlost",
        this.#Ds
      ), this.#u.destroy(), this.#E?.destroy(), this.#e.removeEventListener("emptied", this.#ws), this.#e.removeEventListener("resize", this.#Ts), this.#e.removeEventListener("pause", this.#Re), this.#e.removeEventListener("ended", this.#Re), this.#e.removeEventListener("seeking", this.#Rs), this.#e.removeEventListener("seeked", this.#Re), this.#e.removeEventListener("ratechange", this.#Re), this.#e.removeEventListener("waiting", this.#ys), this.#di !== null && (this.#e.style.aspectRatio = this.#di), this.#Ys(), this.#br();
      for (const e of this.#a) this.#s.deleteTexture(e);
      this.#a = [], this.#wt(), this.#Kt();
      for (const e of [
        ...this.#Dt,
        ...this.#ze.map(({ q: t }) => t)
      ])
        this.#s.deleteQuery(e);
      this.#Dt.length = 0, this.#ze.length = 0, this.#n?.destroy(), this.#n = null, this.#lt?.destroy(), this.#lt = null, this.#Ve !== null && (Me(this.#Ve), this.#Ve = null), this.#s.deleteProgram(this.#A), this.#s.deleteProgram(this.#v), this.#s.getExtension("WEBGL_lose_context")?.loseContext();
    }
  }
  /**
   * Copy the picture currently represented by the deinterlacer.
   *
   * The WebGL drawing buffer is deliberately not preserved between browser
   * composites. Repeating the exact draw path of the presented picture before
   * `createImageBitmap` makes a snapshot reliable without imposing the
   * permanent cost of `preserveDrawingBuffer` on ordinary playback.
   */
  capture() {
    if (this.#w === "active" && this.#t.style.visibility === "visible" && this.#m) {
      const s = ++this.#Ms, r = new Promise((o, h) => {
        this.#rt.set(s, { resolve: o, reject: h });
      });
      return this.#m.postMessage({
        type: "capture",
        id: s,
        width: this.#e.videoWidth,
        height: this.#e.videoHeight
      }), r;
    }
    if (!this.#b && (this.#w === "starting" || this.#w === "failed"))
      return createImageBitmap(this.#e);
    const e = this.#l;
    if ((this.#S || this.#b) && (!this.#I || this.#te || !e))
      return Promise.reject(new Error("no rendered picture is available"));
    if (!this.#I || this.#te || !e)
      return createImageBitmap(this.#e);
    e.kind === "texture" ? this.#qt(e.texture, e.flip, !1) : this.#me(e.flush, e.second, null, !1);
    const t = this.#b ? this.#Et : this.#e.videoWidth, i = this.#b ? this.#bt : this.#e.videoHeight;
    return t > 0 && i > 0 && (t !== this.#i.width || i !== this.#i.height) ? createImageBitmap(this.#i, {
      resizeWidth: t,
      resizeHeight: i,
      resizeQuality: "high"
    }) : createImageBitmap(this.#i);
  }
  addEventListener(e, t, i) {
    super.addEventListener(e, t, i);
  }
  removeEventListener(e, t, i) {
    super.removeEventListener(e, t, i);
  }
  #Tt() {
    this.#S || !this.#I || this.#b || this.#u.request(this.#zs);
  }
  /** Copy the element's playback state for seek and presentation timing decisions. */
  #Ie() {
    const e = [];
    for (let t = 0; t < this.#e.buffered.length; t++)
      e.push({
        start: this.#e.buffered.start(t),
        end: this.#e.buffered.end(t)
      });
    return {
      currentTime: this.#e.currentTime,
      playbackRate: this.#e.playbackRate,
      seeking: this.#e.seeking,
      paused: this.#e.paused,
      ended: this.#e.ended,
      readyState: this.#e.readyState,
      videoWidth: this.#e.videoWidth,
      videoHeight: this.#e.videoHeight,
      buffered: e
    };
  }
  /** Keep one frame in flight and the newest one waiting, staying close to the audio clock. */
  #Bs(e, t, i) {
    let s;
    try {
      s = i?.clone() ?? new VideoFrame(this.#e, {
        timestamp: Math.max(0, Math.round(t.mediaTime * 1e6))
      });
    } catch (o) {
      const h = o instanceof Error ? o.message : String(o);
      this.#Ee === "auto" && !this.#Rt && !this.#it ? (this.#zt(), this.#Yt(e, t)) : this.#vt(h);
      return;
    }
    const r = {
      id: ++this.#_s,
      frame: s,
      now: e,
      metadata: t,
      video: this.#Ie()
    };
    if (this.#st) {
      this.#Ge?.frame.close(), this.#Ge = r;
      return;
    }
    this.#ji(r);
  }
  /** Transfer the selected frame once the Worker has released the previous input. */
  #ji(e) {
    const t = this.#m;
    if (!t || this.#w !== "active") {
      e.frame.close();
      return;
    }
    this.#st = !0;
    const i = {
      type: "frame",
      id: e.id,
      frame: e.frame,
      metadata: e.metadata,
      video: e.video
    };
    try {
      t.postMessage(i, [e.frame]);
    } catch (s) {
      this.#st = !1, e.frame.close();
      const r = s instanceof Error ? s.message : String(s);
      this.#Ee === "auto" && !this.#Rt && !this.#it ? (this.#zt(), this.#Yt(e.now, e.metadata)) : this.#vt(r);
    }
  }
  #Us(e, t, i) {
    this.#Ve == null && (this.#Ve = De(this.#s, "20px monospace")), Ne(this.#Ve, e, t, i, this.#C, this.#G, 20);
  }
  #Wt(e) {
    if (this.#Xe == null || this.#ze.length > 30)
      return;
    const t = this.#Dt.pop() ?? this.#s.createQuery();
    return this.#s.beginQuery(this.#Xe.TIME_ELAPSED_EXT, t), this.#ze.push({ q: t, isField: e }), t;
  }
  #we(e) {
    this.#Xe != null && (e != null && this.#s.endQuery(this.#Xe.TIME_ELAPSED_EXT), this.#ze = this.#ze.filter(
      ({ q: t, isField: i }) => {
        if (this.#s.getQueryParameter(t, this.#s.QUERY_RESULT_AVAILABLE)) {
          const s = this.#s.getQueryParameter(t, this.#s.QUERY_RESULT);
          return i ? (this.#at += s, this.#He++) : (this.#ot += s, this.#We++), this.#Dt.push(t), !1;
        }
        return !0;
      }
    ));
  }
  /**
   * Restart the detection. `gap` is for frames that went unseen in a stream
   * that carries on: the cadence must be found again, but the one-instant
   * verdict is kept because it has not changed.
   */
  #q(e = !1) {
    this.#z = D, this.#At = D, this.#$e = 0, e || (this.#ni = !1), this.#Z = !1, this.#n?.reset(e);
  }
  /** Detect the pulldown phase of the frame being filtered on the GPU. */
  #Ls() {
    const { prev: e, cur: t, next: i } = this.#Ii(!1), s = this.#a[e], r = this.#a[t], o = this.#a[i];
    if (!s || !r || !o) return;
    const h = this.#kt ? 0 : 1;
    this.#n?.measureComb(s, r, h), this.#n?.detect(r, o, h);
  }
  /**
   * Count the combing for the frame after the one being filtered. The count
   * says whether that frame's first field fits between this frame's
   * second-field lines. Returns whether the count was made.
   */
  #ks() {
    const { cur: e, next: t } = this.#Ii(!1), i = this.#a[e], s = this.#a[t], r = this.#n;
    if (!i || !s || !r) return !1;
    const o = this.#kt ? 0 : 1;
    try {
      r.measureComb(i, s, o, !0);
    } catch (h) {
      return this.#Xt(
        `film detection failed: ${h instanceof Error ? h.message : String(h)}`
      ), !1;
    }
    return !0;
  }
  /**
   * Read back the previous frame's phase if it has arrived, and advance it
   * to the frame being filtered. The run is not advanced: only the GPU
   * counts observed frames.
   */
  #Os() {
    const e = this.#n?.poll() ?? null;
    e !== null && (this.#At = e, this.#$e = e.age, this.#ni = e.instant), this.#$e++;
    const { phase: t, run: i } = this.#At;
    t === 0 || this.#$e > Math.ceil(G * Math.max(1, this.#e.playbackRate)) ? this.#z = D : this.#z = {
      phase: (t - 1 + this.#$e) % G + 1,
      run: i
    };
  }
  #Gs(e) {
    const t = [], i = this.#n?.metrics ?? new Float32Array(0);
    for (let r = 0; r < p.phase; r++) {
      const o = i[r * 4] ?? 0, h = i[r * 4 + 1] ?? 0, a = i[r * 4 + 2] ?? 0;
      t.push(
        `${o.toFixed(3)},${h.toString().padStart(4)},${a.toFixed(3)}`
      );
    }
    const s = this.#Mt.map((r, o) => `${o === 0 ? "-" : o}:${r}`).join(" ");
    return `frame=${e} phase=${this.#z.phase} run=${this.#z.run} known=${this.#At.phase}/${this.#At.run} age=${this.#$e} ${this.#Z ? "film" : "video"} period=${this.#L.toFixed(3)}
${t.join(" ")}
${s} dropped:${this.#Nt}`;
  }
  /**
   * A timestamp taken on `origin`'s clock, expressed on this realm's.
   *
   * `performance.timeOrigin` is defined against the same moment in every
   * realm, so the difference between two origins is the whole of the
   * conversion between their clocks (W3C HR-Time 1.2). The origins are
   * subtracted from one another before the timestamp is added: they are the
   * large numbers, their difference is not, and doing it the other way round
   * would spend the precision the schedule is measured in.
   */
  #xt(e, t) {
    const i = t - performance.timeOrigin;
    return Number.isFinite(i) && i !== 0 ? e + i : e;
  }
  /**
   * When this frame reaches the screen, on the clock this renderer draws by.
   *
   * requestVideoFrameCallback()'s display time is what holds the schedule on
   * the refresh grid, so it is preferred wherever it is usable -- including
   * across realms, since #localTime converts it exactly. It is not always
   * usable: 0 and NaN have both been reported, an origin may be missing, and
   * a time that cannot belong to this frame is worse than no time at all.
   * Those fall back to the moment the frame was taken in hand, which is this
   * renderer's own clock and always valid.
   */
  #Xs(e, t) {
    const i = e.expectedDisplayTime;
    if (!Number.isFinite(i) || i <= 0 || !Number.isFinite(e.timeOrigin)) return t;
    const s = this.#xt(i, e.timeOrigin), r = Dt * Math.max(this.#c, this.#L);
    return s < t - r || s > t + r ? t : s;
  }
  #zs = (e, t) => {
    if (!this.#I || this.#te) return;
    this.#yi();
    const i = this.#xt(e, t.timeOrigin), { frame: s, picture: r, ...o } = t;
    this.#Oe = s ?? this.#e, this.#R = r ?? null;
    try {
      this.#Ws(i, o, s);
    } finally {
      this.#Oe = this.#e, this.#R = null;
    }
    this.#Tt();
  };
  /**
   * Take a frame from either acquisition path into the selected renderer.
   * `now` is the ingestion time on this realm's clock.
   */
  #Ws(e, t, i) {
    if (this.#w === "active") {
      this.#Bs(e, t, i);
      return;
    }
    this.#w !== "starting" && this.#Yt(e, t);
  }
  /**
   * @internal Feed Worker input through the same history and rendering decisions
   * as main-thread input. `now` is the ingestion time on the Worker's clock.
   * The metadata's display time stays on the page's clock and is converted
   * using the accompanying timeOrigin.
   */
  ingestExternalFrame(e, t, i) {
    this.#Oe = i;
    try {
      this.#Yt(e, t);
    } finally {
      this.#Oe = this.#e;
    }
  }
  /** Queue a pushed picture behind the ones before it. */
  #Hs(e) {
    try {
      if (je(e), e.timestamp < this.#St + M)
        throw new RangeError(
          `a picture at ${e.timestamp} s came after one at ${this.#St} s; pictures come in presentation order`
        );
      this.#b || this.#Zs();
    } catch (s) {
      throw this.#de(e.data), s;
    }
    if (this.#St = e.timestamp, this.#mi++, this.#$s(e.displayWidth, e.displayHeight), !this.#I) {
      this.#de(e.data);
      return;
    }
    if (this.#w === "starting" || this.#w === "active") {
      this.#Qs(e);
      return;
    }
    const t = this.#Ne;
    t.push(e), this.#Ei(this.#j.at(performance.now()));
    const i = Math.ceil(
      Nt * Math.max(1, this.#e.playbackRate)
    );
    for (; t.length > i; ) {
      const s = t.pop();
      this.#$t(s.timestamp + s.duration) && this.#F.queueResetted++, this.#de(s.data);
    }
    this.#ve = Math.max(
      this.#ve,
      t.length
    );
  }
  /**
   * Note the size decoded pictures are shown at. An element playing only
   * sound has no proportions of its own, and a page that sizes its player
   * from the element's would lay it out as 2:1, so the element is given the
   * picture's.
   */
  #$s(e, t) {
    if (!(e === this.#Et && t === this.#bt)) {
      if (this.#Et = e, this.#bt = t, this.#S || this.#J) {
        this.#Je();
        return;
      }
      this.#di ??= this.#e.style.aspectRatio, this.#e.style.aspectRatio = `${e} / ${t}`, this.#Vs(), this.#e.dispatchEvent(new Event("resize"));
    }
  }
  /**
   * Let the element answer for the pictures it does not carry. A page reads
   * a video's size and frame counts from the element -- to refuse a capture
   * of nothing, to shape a picture-in-picture placeholder, to show them --
   * and an element playing only the sound says 0 by 0 and no frames. So it
   * is given the decoded pictures' answers as properties of its own, which
   * hide the built-in ones until `destroy` takes them away again.
   */
  #Vs() {
    if (this.#It) return;
    this.#It = !0;
    const e = this.#e;
    Object.defineProperty(e, "videoWidth", {
      configurable: !0,
      get: () => this.#Et
    }), Object.defineProperty(e, "videoHeight", {
      configurable: !0,
      get: () => this.#bt
    }), Object.defineProperty(e, "getVideoPlaybackQuality", {
      configurable: !0,
      value: () => ({
        creationTime: performance.now(),
        totalVideoFrames: this.#mi,
        // Whole pictures that were never shown: too late to be taken in, or
        // more than are held. A Worker renderer keeps its own count.
        droppedVideoFrames: this.#F.missed + this.#F.queueResetted + this.#pi,
        corruptedVideoFrames: 0
      })
    });
  }
  /** Give the element's own size and frame counts back. */
  #Ys() {
    if (!this.#It) return;
    this.#It = !1;
    const e = this.#e;
    delete e.videoWidth, delete e.videoHeight, delete e.getVideoPlaybackQuality;
  }
  /** Transfer a picture to the Worker, which sends its buffer back when done. */
  #Qs(e) {
    const t = ++this.#Ns;
    try {
      this.#m.postMessage(
        { type: "picture", id: t, picture: e },
        [e.data]
      );
    } catch (i) {
      throw this.#de(e.data), i;
    }
    this.#Lt.set(t, e.data);
  }
  /** Hand back the buffers a Worker that is going away still holds. */
  #Ht() {
    const e = [...this.#Lt.values()];
    this.#Lt.clear();
    for (const t of e) this.#de(t);
  }
  /** Make pushed pictures the only pictures shown, from now on. */
  #Zs() {
    this.#b = !0, this.#fe(), this.#u.cancel(), this.#_i(), this.#N = 0, this.#U(), this.#q(), this.#l = null, this.#C = 0, this.#G = 0, this.#j.hold(), this.#Gt(), this.#Be();
  }
  /** Hand a picture's buffer back to the producer. */
  #de(e) {
    const t = this.decodedVideo.onrelease;
    if (t !== null)
      try {
        t(e);
      } catch (i) {
        queueMicrotask(() => {
          throw i;
        });
      }
  }
  /** Release everything pushed and forget the history, keeping the canvas as it is. */
  #qi() {
    if (!this.#b) return;
    this.#m?.postMessage({
      type: "pictures",
      action: "reset"
    });
    const e = this.#Ne;
    this.#Ne = [], this.#ui = !1, this.#li = !1, this.#St = Number.NEGATIVE_INFINITY, this.#ci = Number.NaN, this.#Se = !1, this.#ct = !1, this.#N = 0, this.#U(), this.#mt = Number.NEGATIVE_INFINITY, this.#he = !1, this.#Ce = null, this.#le = Number.NaN, this.#ce = Number.NaN, this.#q(), this.#W = 0, this.#j.hold();
    for (const t of e) this.#de(t.data);
  }
  /** Release pictures that will neither be shown nor be the neighbour of one that is. */
  #Ei(e) {
    const t = this.#Ne;
    for (; t.length > 2 && t[2].timestamp <= e; ) {
      const i = t.shift();
      this.#es(i.timestamp + i.duration), this.#de(i.data);
    }
  }
  /**
   * Whether something that ended at `end` was there to be shown. What ended
   * at or before the place playback last started from was not: the pictures
   * a seek's target is given as the filter's neighbours, and whatever was
   * still queued when the position jumped past it. Before the first reading
   * of the clock nothing is known to have been skipped.
   */
  #Ki(e) {
    return !(e <= this.#j.origin + M);
  }
  /**
   * Whether something ending at `end` that goes unshown counts against the
   * renderer. A hidden page is given no refreshes, so nothing goes up from
   * when it is hidden until the first refresh after it shows again, and what
   * ended by that refresh passed unseen. Neither was dropped for being late,
   * and counting them would report a background tab as thousands of dropped
   * frames, and every return to it as one more.
   */
  #$t(e) {
    return this.#Ut || !this.#Ki(e) ? !1 : !(this.#j.origin === this.#zi && e <= this.#Xi + M);
  }
  /** Note whether the page showing the canvas is hidden. */
  #Ji(e) {
    e !== this.#Bt && (this.#Bt = e, this.#m?.postMessage({
      type: "pageVisibility",
      hidden: e,
      video: this.#Ie()
    }), e && (this.#Ut = !0));
  }
  /**
   * @internal Tell a Worker renderer whether the page showing its canvas is
   * hidden. It has no document of its own to ask.
   */
  observePageVisibility(e) {
    this.#Ji(e);
  }
  /** Count a decoded picture ending at `end` that was dropped unshown. */
  #es(e) {
    this.#$t(e) && this.#F.missed++;
  }
  /** Read the element's clock, as it was at `wall` on this realm's clock. */
  #ts(e) {
    const t = this.#e;
    this.#j.observe(
      e,
      t.currentTime,
      t.playbackRate,
      !t.paused && !t.seeking && !t.ended
    ) && (this.#F.resynced++, Number.isNaN(this.#le) && (this.#he = !1));
  }
  /**
   * @internal Give a Worker renderer a reading of the element's clock, taken
   * at `at` on the clock whose origin is `timeOrigin`. The element state it
   * belongs to has been applied to the Worker's video.
   */
  observeMediaClock(e, t) {
    this.#ts(this.#xt(e, t));
  }
  /**
   * @internal Give a Worker renderer `DecodedVideoInput.startsAt`, `at`
   * taken on the clock whose origin is `timeOrigin`.
   */
  observeStart(e, t, i) {
    this.#Ce = { media: e, wall: this.#xt(t, i) };
  }
  /**
   * Send the Worker renderer a reading of the element's clock just before its
   * next animation frame.
   *
   * The Worker fills a refresh in its own frame, which comes at the same
   * refresh as the page's and runs within a millisecond or so of it, before a
   * message posted from the page's frame can arrive. A reading taken there is
   * a refresh old by the time it is used; when it is the one that starts the
   * clock, the pictures due in that refresh are not shown. Taken late in the
   * refresh instead, the reading is used in the frame it was taken for.
   *
   * One reading per refresh, as before, and never two closer together than
   * a refresh less the lead: the clock tells an element that plays from one
   * that merely moves by the pace of readings taken about a refresh apart,
   * and two taken together look like a jump. A reading still waiting when
   * this frame comes (the page was busy past its time) goes now, unless a
   * late timer has only just sent one; either way the next is set for the
   * coming refresh.
   */
  #js() {
    this.#g !== null && (this.#ss(), this.#is());
    const e = this.#ms(), t = this.#y + this.#c - Ae, i = e.setTimeout(
      () => {
        this.#g = null, this.#is();
      },
      Math.max(0, t - performance.now())
    );
    this.#g = { timers: e, handle: i };
  }
  /** Send a reading unless the last one went less than a refresh less the lead ago. */
  #is() {
    performance.now() - this.#_ < this.#c - Ae || this.#qs();
  }
  #ss() {
    const e = this.#g;
    e !== null && (this.#g = null, e.timers.clearTimeout(e.handle));
  }
  /** Send the page's reading of the element's clock to the Worker renderer. */
  #qs() {
    this.#_ = performance.now(), this.#m?.postMessage({
      type: "clock",
      video: this.#Ie(),
      at: performance.now(),
      timeOrigin: performance.timeOrigin
    });
  }
  /** Take in what the coming refresh needs and put up what belongs on it. */
  #Ks() {
    const e = this.#e;
    this.#S || this.#ts(performance.now());
    const t = this.#y + this.#c;
    if (e.paused)
      this.#Ce = null, this.#le = Number.NaN, this.#he = !1;
    else if (this.#Ce !== null) {
      const o = this.#Ce;
      if (t < o.wall) {
        if (!this.#he) return;
        const h = this.#pt;
        this.#rs(this.#Ti(t, h), h, !1);
        return;
      }
      this.#Ce = null, this.#he = !0, this.#ue = o.media + (t - o.wall) * e.playbackRate / 1e3, this.#Ze = t, this.#pt = e.playbackRate, this.#le = t, this.#ce = Number.NaN;
    }
    let i = this.#j.rate;
    const s = this.#j.at(t);
    let r;
    if (!Number.isNaN(this.#le) && (Number.isNaN(s) || !(i > 0))) {
      if (t - this.#le > yt) {
        this.#le = Number.NaN, this.#he = !1;
        return;
      }
      i = this.#pt, r = this.#Ze === t ? this.#ue : this.#Ti(t, i);
    } else {
      if (this.#le = Number.NaN, Number.isNaN(s)) return;
      r = e.paused ? s : this.#hr(s, i);
    }
    this.#rs(r, i, e.paused);
  }
  /**
   * Fill the coming refresh, whose media time is `time`: take in what it
   * needs and put up what belongs on it.
   */
  #rs(e, t, i) {
    this.#Ut && !this.#Bt && (this.#Ut = !1, this.#Xi = Math.max(e, this.#e.currentTime), this.#zi = this.#j.origin);
    const s = this.#ne;
    this.#Js(e, t), this.#V(s) && this.#nr(e, t, i);
  }
  /**
   * Take in pictures until the history holds the successor of every picture
   * whose outputs are due within the lookahead of `time`, the media time of
   * the refresh being filled.
   */
  #Js(e, t) {
    const i = this.#ne, s = Mt * this.#c * Math.max(1, t) / 1e3;
    for (this.#Ei(e); ; ) {
      const a = this.#Ne[0];
      if (a === void 0 || (this.#Se ? this.#Te[this.#D].timestamp : a.timestamp - a.duration) > e + s || (a.width === this.#C && a.height === this.#G ? this.#os() : w) < this.#er(a))
        break;
      if (this.#Ne.shift(), this.#tr(a, e, t), !this.#V(i)) return;
      this.#Ei(e);
    }
    if (!this.#Se || this.#ct || this.#os() < this.#ns())
      return;
    const r = this.#Te[this.#D], o = this.#Ne[0];
    if (o !== void 0 && o.timestamp - (r.timestamp + r.duration) <= M)
      return;
    const h = O * t / 1e3;
    (this.#ui || r.timestamp <= e + h) && (this.#bi(!0, e, t), this.#ct = !0);
  }
  /** Output slots a filtered picture takes. A repeated first field shares the first one's. */
  #ns() {
    return this.#P && !this.#Z ? 2 : 1;
  }
  /** Output slots taking `picture` in can fill. */
  #er(e) {
    const t = !e.interlaced || !this.#Le;
    return (this.#Se ? this.#ns() : 0) + (t ? 1 : 0);
  }
  /** Whether an output slot holds nothing still wanted. */
  #hs(e) {
    const t = this.#h[e];
    if (t === void 0) return !1;
    const i = this.#l;
    if (i?.kind === "texture" && i.texture === t.texture)
      return !1;
    for (const s of this.#Ye)
      if (s.slot === e) return !1;
    return !0;
  }
  #os() {
    let e = 0;
    for (let t = 0; t < this.#h.length; t++)
      this.#hs(t) && e++;
    return e;
  }
  /** The next free output slot round the ring, or null. */
  #as() {
    for (let e = 1; e <= w; e++) {
      const t = (this.#f + e) % w;
      if (this.#hs(t)) return t;
    }
    return null;
  }
  /**
   * Take a decoded picture into the history and filter the picture before it,
   * which has its successor now.
   */
  #tr(e, t, i) {
    this.#Oi = e.duration / (e.interlaced ? e.repeatFirstField ? 3 : 2 : 1);
    const s = this.#li ? this.#Te[this.#D] : null, r = s === null ? 0 : s.timestamp + s.duration, o = e.width !== this.#C || e.height !== this.#G, h = s === null || o || e.timestamp < r - M || e.timestamp - r > V, a = !h && e.timestamp - r > M, u = !e.interlaced || !this.#Le, A = this.#Se, f = this.#ct;
    this.#Se = !1, this.#ct = !1, A && !f && (h || u) && !o && this.#bi(!0, t, i), h ? (s !== null && this.#F.discontinuities++, this.#N = 0, this.#q(), this.#W = 0) : s !== null && !u && s.interlaced && s.topFieldFirst !== e.topFieldFirst && this.#q(!0);
    const d = performance.now();
    this.#Pi(d), this.#ht = d;
    const l = this.#ne, c = this.#Wt(!1);
    try {
      if (o && (this.#Bi(e.width, e.height), this.#h.length !== w))
        throw new Error("decoded pictures have no output textures");
      this.#ir(e);
    } catch (m) {
      this.#we(c), this.#Ot(
        m instanceof Error ? m.message : String(m)
      );
      return;
    } finally {
      this.#de(e.data);
    }
    if (u)
      e.interlaced || (this.#W > 0 && this.#W--, this.#ut = !0), this.#sr(), this.#N = 0;
    else {
      if (A && !h && !(f && a)) {
        const m = this.#Te[(this.#D + T - 1) % T];
        this.#fi = m.topFieldFirst, this.#us(), this.#V(l) && this.#ls(
          a ? 1 : 0,
          Math.round(m.timestamp * 1e3),
          this.#P
        ), this.#V(l) && this.#bi(!1, t, i);
      } else
        this.#oi = a;
      this.#ut = !1, this.#Se = !0;
    }
    if (!this.#V(l)) {
      this.#J || this.#we(c);
      return;
    }
    this.#we(c), this.#ye += performance.now() - d, this.#Ae++, this.#Ci(d);
  }
  /** Convert a decoded picture into the next history slot. */
  #ir(e) {
    this.#D = (this.#D + 1) % T, this.#lt.convert(e, this.#a[this.#D]);
    const t = this.#Te[this.#D];
    t.timestamp = e.timestamp, t.duration = e.duration, t.interlaced = e.interlaced, t.topFieldFirst = e.topFieldFirst, t.repeatFirstField = e.repeatFirstField, this.#N = Math.min(this.#N + 1, T), this.#li = !0;
  }
  /** Queue the newest picture as it is, both fields woven. */
  #sr() {
    const e = this.#Te[this.#D], t = this.#as();
    if (t === null) {
      this.#F.late++;
      return;
    }
    this.#f = t, this.#Es(
      this.#a[this.#D],
      !0,
      this.#h[t].framebuffer
    ), this.#Ye.push({
      slot: t,
      time: e.timestamp,
      picture: e.timestamp,
      end: e.timestamp + e.duration,
      scan: ce(e),
      cadence: "frame",
      duration: e.duration
    });
  }
  /**
   * Filter the waiting picture into its outputs: against its successor, or
   * against itself with `flush`. Each output is queued at its own media time:
   * a field at its field's time, a film frame at its place in the film
   * cadence, one picture a frame at the first field's time.
   */
  #bi(e, t, i) {
    const s = this.#Te[e ? this.#D : (this.#D + T - 1) % T], r = this.#Ye;
    for (; r.length > 0 && r.at(-1).picture === s.timestamp; )
      r.pop();
    this.#fi = s.topFieldFirst;
    const o = e ? Number.POSITIVE_INFINITY : this.#Te[this.#D].timestamp, h = ce(s), a = s.timestamp + s.duration;
    let u = !1;
    if (this.#Z && !e) {
      const A = this.#z.phase;
      if (A === z) {
        this.#Nt++;
        return;
      }
      const f = s.timestamp + (le[A] ?? 0) * s.duration;
      u = this.#vi(
        f,
        f + s.duration * G / ue,
        o,
        s.timestamp,
        a,
        !1,
        null,
        e,
        h,
        t,
        i,
        "film"
      ) !== null;
    } else if (!this.#P)
      u = this.#vi(
        s.timestamp,
        o,
        o,
        s.timestamp,
        a,
        !1,
        null,
        e,
        h,
        t,
        i,
        "frame"
      ) !== null;
    else {
      const A = s.repeatFirstField ? 3 : 2, f = s.duration / A;
      let d = null;
      for (let l = 0; l < A; l++) {
        const c = s.timestamp + l * f, m = this.#vi(
          c,
          l + 1 < A ? c + f : o,
          o,
          s.timestamp,
          a,
          l === 1,
          l === 2 ? d : null,
          e,
          h,
          t,
          i,
          "field"
        );
        l === 0 && (d = m), m !== null && (u = !0);
      }
    }
    !u && o <= t && s.timestamp > this.#mt && this.#es(a);
  }
  /**
   * Filter one output into a slot and queue it for `at`, unless it can never
   * be shown. `until` is when the output after it is due and `pictureUntil`
   * when the next picture is; `picture` and `pictureEnd` are the picture's
   * own time and end. `slot` is a slot already holding the same field.
   * Returns the slot, or null.
   */
  #vi(e, t, i, s, r, o, h, a, u, A, f, d) {
    if (!this.#rr(e, t, i, A, f))
      return e > this.#mt && this.#$t(t) && this.#F.late++, null;
    if (h === null) {
      if (h = this.#as(), h === null)
        return this.#F.late++, null;
      this.#f = h, this.#me(a, o, this.#h[h].framebuffer);
    }
    return this.#Ye.push({
      slot: h,
      time: e,
      picture: s,
      // A film frame is due past the time of the picture after it.
      end: Number.isFinite(i) ? Math.max(t, i) : r,
      scan: u,
      cadence: d,
      duration: (Number.isFinite(t) ? t : r) - e
    }), h;
  }
  /**
   * Whether an output due from `at` until `until` can be shown. A refresh
   * shows the newest output due by then, so one whose successor is due before
   * the next refresh after `at` is never seen: at rates the display cannot
   * keep up with, or after it was due. Refreshes are predicted from `time`,
   * the media time of the coming one, with a margin for the hysteresis.
   * While the clock stands still every output of a current picture is kept,
   * since the one due at a paused position can be any of them.
   */
  #rr(e, t, i, s, r) {
    if (r === 0) return i > s;
    const o = 2 * O * r / 1e3;
    if (t <= s - o) return !1;
    const h = this.#c * r / 1e3;
    if (t - e + 2 * o >= h) return !0;
    const a = Math.max(0, Math.ceil((e - o - s) / h));
    return s + a * h < t + o;
  }
  /**
   * Put up the output the coming refresh should show.
   *
   * Playing, that is the newest output due by `time`. Where an output's time
   * is within the hysteresis of `time`, the refresh advances by as many
   * outputs as the last one did if it can, so that the one slip a field rate
   * the refresh does not divide into must make happens once rather than
   * flapping.
   *
   * Paused, it is the same: the output due at the paused position, which is
   * where play resumes from, so that the outputs after it follow on. (The
   * first output of the picture the position falls in instead would step
   * the picture back a field at a pause in the second, and pass that field
   * over when play resumes.) Where nothing newer than the output up is due,
   * that output stays, and outputs passed over are no lateness: a seek
   * while paused jumps over them.
   */
  #nr(e, t, i) {
    const s = this.#Ye;
    let r = 0;
    for (; r < s.length && !this.#Ki(s[r].end + this.#Vt(s[r])); )
      r++;
    r > 0 && s.splice(0, r);
    let o = 0;
    for (; o < s.length && s[o].time <= this.#mt; )
      o++;
    o > 0 && s.splice(0, o);
    const h = M * (i ? 1 : t);
    let a = -1;
    for (let A = 0; A < s.length && s[A].time + this.#Vt(s[A]) <= e + h; A++)
      a = A;
    if (a < 0) {
      const A = s[0];
      A && !i && this.#ar(A, e);
      return;
    }
    if (!i)
      for (let A = 0; A < a; A++) {
        const f = s[A + 1].time + this.#Vt(s[A + 1]);
        f <= e && this.#$t(f) && this.#F.late++;
      }
    const u = s[a];
    s.splice(0, a + 1), this.#Ar(u);
  }
  /**
   * The media time of the refresh being filled while playing, from `reading`,
   * the clock's.
   *
   * Decoded pictures are shown against a count of refreshes, as mpv's display
   * sync shows frames, rather than against the clock read at each refresh,
   * which near the boundary of two outputs would show one twice and skip the
   * other at the smallest wobble. The count moves the media time on by a
   * fixed step each refresh. Where the screen runs within
   * `MAX_DISPLAY_DEVIATION` of a whole number of picture steps a refresh (or
   * of refreshes a step), the step is exactly that, so that every refresh
   * moves by whole steps and none is ever shown twice or passed over; the
   * sound is then played that much faster or slower to stay with the
   * pictures (`avDifference`, which the player steers the sound's speed by).
   * Elsewhere it is the refresh at this speed.
   *
   * The clock starts the count, so that the output due at the refresh being
   * filled goes up first, and moves it by whole steps where it has come
   * further from the count than `AV_CORRECTION_SECONDS` and a step: that
   * much is no wobble or rounding but a stall, which the sound's speed would
   * take too long over. While the clock stands -- a seek, a wait for data --
   * there is nothing to count, and the time is the reading.
   */
  #hr(e, t) {
    const i = this.#y + this.#c, s = this.#As(t);
    if (!(t > 0))
      return this.#he = !1, e;
    if (!this.#he)
      this.#he = !0, this.#ue = e, this.#pt = t, this.#ce = Number.NaN;
    else {
      this.#Ti(i, t);
      const r = e - this.#ue;
      Math.abs(r) >= Math.max(Ft * t, s) && (this.#ue += Math.trunc(r / s) * s);
    }
    return this.#Ze = i, this.#or(e - this.#ue), this.#ue;
  }
  /** Move the count on to the refresh at `target`: by the refreshes since its last. */
  #Ti(e, t) {
    const i = Math.max(
      1,
      Math.round((e - this.#Ze) / this.#c)
    );
    return this.#ue += i * this.#As(t), this.#Ze = e, this.#pt = t, this.#ue;
  }
  /**
   * Media seconds the count of `#countRefresh` moves by a refresh at speed
   * `rate`: whole picture steps where the screen is close enough to them,
   * else the refresh itself.
   */
  #As(e) {
    const t = this.#c * e / 1e3, i = this.#Oi;
    if (!(i > 0)) return t;
    const s = t / i, r = s >= 1 ? Math.round(s) : 1 / Math.round(1 / s);
    return Math.abs(s / r - 1) <= Rt ? r * i : t;
  }
  /** Average the sound's lead over the output just shown, for `avDifference`. */
  #or(e) {
    const t = performance.now(), i = t - this.#Gi;
    this.#Gi = t, Number.isNaN(this.#ce) || i > oe ? this.#ce = e : this.#ce += (e - this.#ce) * i / oe;
  }
  /** How late an output goes up: one picture, when the pictures after it are made. */
  #Vt(e) {
    return this.#Pe?.wants(e.cadence) ? e.duration : 0;
  }
  /**
   * Put up a picture between the output up and `next`, at `time`, the media
   * time of the coming refresh. Nothing is drawn where the two are not
   * neighbours: a new cadence, a gap, or a moment outside them.
   */
  #ar(e, t) {
    const i = this.#Pe, s = this.#Pt;
    if (!i || !s || this.#S || s.cadence !== e.cadence || !i.wants(e.cadence))
      return;
    const r = this.#h[s.slot], o = this.#h[e.slot], h = this.#l;
    if (!r || !o || h?.kind !== "texture" || h.texture !== r.texture)
      return;
    const a = e.time - s.time;
    if (a <= 0 || a > s.duration * 1.5) return;
    const u = (t - this.#Vt(s) - s.time) / a;
    if (u <= 0 || u >= 1) return;
    const A = this.#s;
    (this.#ft?.previous !== s || this.#ft.next !== e) && (i.pair(A, r.texture, o.texture, this.#C, this.#G), this.#ft = { previous: s, next: e });
    const f = performance.now(), d = this.#dt ? this.#gs() : null, l = d?.framebuffer ?? null;
    A.bindFramebuffer(A.FRAMEBUFFER, l);
    let c = !1;
    try {
      c = i.draw(A, u, this.#C, this.#G, l);
    } catch (m) {
      console.warn("yadif: the interpolator failed", m), this.#Pe = null, this.#ft = null;
    }
    c && (d && this.#Si(d.texture, !1), this.#De += performance.now() - f, this.#be++);
  }
  #Ar(e) {
    const t = performance.now(), i = this.#Wt(!0);
    this.#Ni(e.slot), this.#we(i), this.#De += performance.now() - t, this.#be++, this.#Pt = e, this.#mt = e.time;
  }
  /** Take one input into the shared history and decide its YADIF or IVTC presentation. */
  #Yt(e, t) {
    const i = this.#ne;
    if (this.#V(i) && (this.#ur(t.mediaTime), !!this.#V(i) && t.width > 0 && t.height > 0)) {
      let s = !1;
      if (!this.#tt && this.#e.seeking) {
        const c = this.#e.buffered, m = this.#L >= ne ? this.#L / 1e3 : k / 1e3;
        for (let E = 0; E < c.length; E++)
          if (t.mediaTime >= c.start(E) && t.mediaTime < c.end(E) && Math.abs(t.mediaTime - this.#e.currentTime) <= m) {
            s = !0;
            break;
          }
      }
      if (s && (this.#tt = !0), (this.#C === 0 || this.#G === 0) && this.#Bi(t.width, t.height), !this.#V(i)) return;
      if (this.#k && !this.#k.interlaced) {
        const c = performance.now();
        this.#Pi(c), this.#ht = c, this.#W > 0 && this.#W--, this.#ut = !0, this.#mr(), this.#ye += performance.now() - c, this.#Ae++, this.#be++, this.#Ci(c);
        return;
      }
      const r = t.mediaTime - this.#et, o = t.mozTiming, h = s || (o ? o.discontinuity || r < 0 || r > V : r < 0 || r > V);
      h && (this.#N = 0, this.#L = 0, this.#F.discontinuities++, this.#U(), this.#q(), (s || !this.#ut) && (this.#W = 0)), this.#ut = !1;
      const a = this.#pr(t.presentedFrames, h);
      if (this.#N > 0 && t.mediaTime === this.#et && (!o || t.presentedFrames === this.#Jt))
        return;
      if (!h) {
        const c = o?.periodMs ?? 0;
        c > 0 ? this.#cs(
          c * (this.#e.playbackRate || 1) / 1e3,
          1
        ) : this.#N > 0 && r > 0 && this.#cs(r, a + 1);
      }
      this.#et = t.mediaTime, this.#Jt = t.presentedFrames;
      const u = performance.now();
      this.#Pi(u), this.#ht = u;
      const A = performance.now(), f = this.#Wt(!1);
      if (this.#bs(), this.#us(), !this.#V(i)) {
        this.#J || this.#we(f);
        return;
      }
      if (this.#ls(
        a,
        t.presentedFrames,
        this.#P && this.#gt()
      ), !this.#V(i)) {
        this.#J || this.#we(f);
        return;
      }
      const l = this.#Xs(t, e) + this.#c;
      if (this.#Z)
        if (this.#gt()) {
          const c = this.#z.phase;
          if (c === z)
            this.#Nt++, this.#Y++;
          else {
            const m = this.#L * G / ue, E = this.#wi(1, e, m), v = le[c] ?? 0, b = E || this.#se === null ? l + m : l + v * this.#L;
            this.#Qt(
              "film",
              !1,
              this.#xi("film", b, m),
              m
            );
          }
        } else
          this.#me(!1, !1, null);
      else if (this.#P && this.#gt()) {
        const c = this.#L / 2, E = this.#wi(2, e, c) || this.#se === null ? l + c * 2 : l, v = this.#xi("field", E, c);
        this.#Qt("field", !1, v, c), this.#Qt("field", !0, v + c, c);
      } else if (this.#gt()) {
        const c = this.#L, E = this.#wi(1, e, c) || this.#se === null ? l + c : l;
        this.#Qt(
          "frame",
          !1,
          this.#xi("frame", E, c),
          c
        ) || (this.#F.late++, this.#me(!1, !1, null));
      } else
        this.#F.late += this.#r.length, this.#U(), this.#me(!1, !1, null);
      this.#ve = Math.max(
        this.#ve,
        this.#r.length
      ), this.#we(f), this.#ye += performance.now() - A, this.#Ae++, this.#Ci(u);
    }
  }
  /** Rebuild the detector a retry point stood the engine back up for. */
  #us() {
    if (this.#X && !this.#Me && !this.#n)
      try {
        this.#Vi();
      } catch (e) {
        this.#Xt(
          `film detector unavailable: ${e instanceof Error ? e.message : String(e)}`
        );
      }
  }
  /**
   * Run the pulldown detection for the frame behind the newest, which is the
   * one about to be filtered. `missed` is how many frames went unseen before
   * the newest, `frame` labels the debug overlay, and `secondFields` is
   * whether the frame's second field is drawn too.
   */
  #ls(e, t, i) {
    if (this.#_t = !1, this.#_e = !1, this.#hi = !1, this.#X && !this.#Me) {
      let s = !1;
      if (this.#N === T && e === 0)
        try {
          this.#Os(), this.#Ls(), s = !0;
        } catch (r) {
          this.#Xt(
            `film detection failed: ${r instanceof Error ? r.message : String(r)}`
          );
        }
      else
        this.#q(!0);
      this.#Mt[this.#z.phase] = (this.#Mt[this.#z.phase] ?? 0) + 1, this.#Z = this.#z.phase !== 0 && this.#z.run >= j, this.#Z ? this.#W = _t : this.#W > 0 && this.#W--, this.#_t = s && !this.#oi, this.#_e = this.#_t && this.#W > 0, this.#hi = this.#_e && !this.#Z && i && this.#ks(), this.#_e && !this.#Z && this.#F.matched++, this.#ae && (this.#Li = this.#Gs(t));
    }
    this.#oi = e > 0;
  }
  #V(e) {
    return !this.#J && this.#I && e === this.#ne;
  }
  #ur(e) {
    const t = this.#ne;
    let i;
    for (let o = this.#ke.length - 1; o >= 0; o--) {
      const h = this.#ke[o];
      if (h.start <= e + 1e-6) {
        i = h;
        break;
      }
    }
    if (i?.codedSize && (i.codedSize.width !== this.#C || i.codedSize.height !== this.#G) && this.#Bi(i.codedSize.width, i.codedSize.height), !this.#V(t)) return;
    const s = i?.scan;
    if (!s || this.#k?.interlaced === s.interlaced && this.#k.topFieldFirst === s.topFieldFirst)
      return;
    const r = this.#k?.interlaced;
    this.#k = s, this.#N = 0, this.#U(), this.#qe() && (r !== s.interlaced && (this.#L = 0), s.interlaced && (this.#S || this.#w === "main") ? this.#Be() : this.#Fi(), this.#q());
  }
  /**
   * Whether pictures are being filtered ahead of time and queued, rather than
   * drawn as their frame arrives.
   *
   * A picture for every frame has nothing to schedule -- there is one of them
   * and it goes up now -- and neither has a filter that has yet to see two
   * frames go by, since until then there is no idea how long a frame lasts.
   */
  #gt() {
    return (this.#P || this.#X) && this.#L > 0 && this.#h.length === w;
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
  #cs(e, t) {
    const s = e * 1e3 / (this.#e.playbackRate || 1) / t;
    s < ne || s > k || (this.#L = this.#L > 0 && s > this.#L * vt ? this.#L + (s - this.#L) * pt : s);
  }
  /**
   * Filter one field into an output texture and put it in the queue.
   *
   * The three frames the filter reads are only the right three between one
   * frame arriving and the next, so both fields of a frame are built here and
   * held as pictures. What is queued after that is a copy waiting for a
   * moment, which no later frame can take away.
   */
  #Qt(e, t, i, s) {
    const r = this.#fs();
    if (r === null) return !1;
    const o = this.#h[r];
    if (!o) return !1;
    for (this.#f = r; this.#r.length > 0 && this.#r[0]?.slot === r; )
      this.#r.shift(), this.#F.late++;
    this.#me(!1, t, o.framebuffer);
    const h = {
      slot: r,
      at: i,
      duration: s,
      cadence: e,
      phase: e === "film" ? this.#z.phase : e === "field" ? t ? 2 : 1 : 0,
      droppedBefore: this.#Y
    };
    return this.#Y = 0, this.#r.push(h), this.#se = h, !0;
  }
  /**
   * When a picture goes up: one duration after the last one of its cadence,
   * nudged towards the clock by a fraction of the gap so that the schedule
   * follows it without a picture ever moving across a refresh. (otya)
   *
   * `ideal` is when this frame would go up if it were the only one. Frames
   * do not arrive evenly. Where the frame period and the refresh nearly
   * divide, a frame arrives a refresh early or late now and then, and for
   * seconds at a time while the two drift past each other. An early frame is
   * harmless because its pictures wait. A late frame has missed its moment,
   * so the chain restarts from where the frame is. That costs a refresh with
   * nothing new to show, which cannot be avoided. The cost should be paid
   * only once. The frames after it arrive on time, which now counts as a
   * refresh early, and pulling the chain back to them would skip a picture to
   * undo a delay nobody sees. So the chain keeps the lateness it was pushed
   * to (`#scheduleLag`), treats frames that arrive at that phase as on time,
   * and puts frames that are late by no more than the lag covers on the chain.
   *
   * Frames arrive on refreshes. At whatever phase a frame arrives, the
   * remainder of its distance from the chain after removing whole refreshes
   * is the chain's own drift against the refresh grid, and each frame
   * corrects part of it. The lag is kept in whole refreshes for the same
   * reason. A chain held half a refresh off the grid makes each pair of
   * fields come due together, and one of each pair is dropped.
   *
   * The lag follows what frames actually do. If no frame arrives at the
   * lag's phase within SCHEDULE_WINDOW pictures, the lag moves to the nearest
   * phase they did arrive at. If that phase is earlier than the chain allows
   * for, the chain moves up to it, which skips one picture. A frame more than
   * a refresh off on either side is not jitter, and the chain restarts from
   * the clock.
   */
  #xi(e, t, i) {
    if (!(i > 0)) return t;
    const s = this.#se, r = this.#c;
    if (s !== null && s.cadence === e) {
      const o = s.at + s.duration, h = o - t, a = Math.round(h / r), u = a - this.#Q, A = h - a * r;
      if (a >= 0 && u <= 1) {
        const f = o + Math.max(
          -he,
          Math.min(he, -A * Tt)
        );
        if (u === 0)
          return this.#O = 0, f;
        if (this.#O === 0 || u > 0 != this.#$ > 0 ? (this.#O = 0, this.#$ = u) : Math.abs(u) < Math.abs(this.#$) && (this.#$ = u), ++this.#O < xt) return f;
        const d = this.#$;
        return this.#O = 0, d < 0 ? (this.#Q += d, f) : (this.#F.resynced++, this.#gi(f - d * r));
      }
      if (a < 0 && this.#Q - a <= 1)
        return this.#Q -= a, this.#O = 0, this.#F.resynced++, this.#gi(t);
    }
    return this.#Q = 0, this.#O = 0, s !== null && this.#F.resynced++, this.#gi(t);
  }
  /** Start the chain again at `at`, which is where the caller's picture goes. */
  #gi(e) {
    for (let t = this.#r.at(-1); t && t.at >= e; )
      this.#r.pop(), this.#F.late++, t = this.#r.at(-1);
    return e;
  }
  /** Make room without treating ordinary capacity pressure as clock divergence. */
  #wi(e, t, i) {
    const s = this.#r.at(-1), r = (Q + 1) * Math.max(this.#c, i);
    if (s && s.at - t > r)
      return this.#U(), this.#F.queueResetted++, !0;
    const o = Math.max(
      0,
      this.#r.length + e - Q
    );
    let h = 0, a = 0;
    for (; a < o; ) {
      const u = this.#r.shift();
      if (!u) break;
      h += u.duration, a++;
    }
    for (const u of this.#r) u.at -= h;
    return this.#F.late += a, !1;
  }
  /** Select an output whose pixels are not still represented by the canvas or queue. */
  #fs() {
    const e = this.#l?.kind === "texture" ? this.#l.texture : null, t = new Set(this.#r.map(({ slot: s }) => s));
    for (let s = 1; s <= w; s++) {
      const r = (this.#f + s) % w, o = this.#h[r];
      if (o && o.texture !== e && !t.has(r))
        return r;
    }
    const i = this.#r[0];
    if (i) {
      const s = this.#h[i.slot];
      if (s && s.texture !== e) return i.slot;
    }
    return null;
  }
  /** The loop that puts filtered fields up, and the only thing that draws. */
  #Be() {
    this.#d === null && (!this.#I || this.#te || (this.#H = 0, this.#d = this.#Ke(this.#Ri)));
  }
  #Fi() {
    this.#Zt(this.#d), this.#d = null, this.#ss(), this.#U();
  }
  #Ri = (e) => {
    if (this.#d = null, !this.#I || this.#te) return;
    this.#lr(e);
    const t = this.#ne;
    if (this.#b)
      this.#S || this.#w === "main" ? this.#Ks() : this.#js();
    else {
      if (this.#u.flush(e), !this.#V(t)) return;
      this.#w === "main" && this.#fr(this.#y, e);
    }
    this.#V(t) && (this.#d = this.#Ke(this.#Ri));
  };
  /**
   * Fit a grid of refreshes (period and phase) to the animation frames. rAF
   * timestamps wander by a millisecond or so, which is more than the
   * nearest-refresh decision in #present can take; the grid is what it
   * compares against. A frame far off the grid restarts it. (otya)
   */
  #lr(e) {
    const t = e - this.#H;
    this.#H = e;
    const i = Math.max(1, Math.round(t / this.#c)), s = this.#y + i * this.#c, r = e - s;
    if (this.#y === 0 || t <= 0 || t > k || Math.abs(r) > this.#c / 4) {
      t >= 1 && t <= k && (this.#c = t), this.#y = e;
      return;
    }
    this.#c += r / i * gt, this.#y = s + r * wt;
  }
  /**
   * Where animation frames come from, and the clock their timestamps carry.
   *
   * For Worker rendering it is the host the Worker entry supplied, whose
   * frames are already this realm's. Otherwise it is the window of the
   * document the canvas is in, which is the window that composites it -- and
   * which a document picture-in-picture window becomes by adopting the
   * element. The element is consulted as well, because the two move together
   * and either may be read first while a move is in progress.
   */
  #ds() {
    const e = this.#S;
    if (e)
      return { frames: e, origin: performance.timeOrigin };
    const t = this.#ps();
    return t === null ? { frames: Ct, origin: performance.timeOrigin } : { frames: t, origin: t.performance.timeOrigin };
  }
  /**
   * Ask the current source for one frame, on this module's clock.
   *
   * An animation frame is timestamped against the clock of the window that
   * served it, and after an adoption that is not the clock the presentation
   * deadlines are on. The conversion belongs to the request rather than to
   * the callback: the source that was asked is the source that answers, so
   * the origin captured here is the right one however often the owner moves.
   */
  #Ke(e) {
    const { frames: t, origin: i } = this.#ds(), s = t.requestAnimationFrame(
      (r) => e(this.#xt(r, i))
    );
    return { frames: t, handle: s };
  }
  /** Timers of the window the loop's animation frames come from. */
  #ms() {
    return this.#ps() ?? It;
  }
  /** The window that composites the canvas, or null in none. */
  #ps() {
    return this.#t.ownerDocument?.defaultView ?? this.#e.ownerDocument?.defaultView ?? null;
  }
  /** Cancel a pending animation frame through the source that issued it. */
  #Zt(e) {
    e?.frames.cancelAnimationFrame(e.handle);
  }
  /**
   * Follow the window that composites the canvas.
   *
   * A request already outstanding on the previous window is not merely late.
   * A window that has gone hidden serves no animation frames at all, so
   * waiting for that callback to notice the move waits for something that
   * will not happen; and cancelling it anywhere but on that same window
   * leaves it outstanding. Every other thing still being observed -- a frame
   * arriving, a layout, the old window saying it is going away -- re-points
   * both requests instead.
   *
   * Nothing queued moves: presentation deadlines are on this module's clock,
   * which no adoption changes. Only the refresh grid starts again, because a
   * grid describes one compositor and this is a different one.
   */
  #yi() {
    if (this.#S) return;
    this.#cr(), this.#p && this.#Ji(this.#p.hidden);
    const { frames: e } = this.#ds(), t = this.#d !== null && this.#d.frames !== e, i = this.#x !== null && this.#x.frames !== e;
    !t && !i || (this.#H = 0, t && (this.#Zt(this.#d), this.#d = this.#Ke(this.#Ri)), i && (this.#Zt(this.#x), this.#x = this.#Ke(this.#Mi)));
  }
  /** Watch the current document for visibility changes and DOM adoption. */
  #cr() {
    const e = this.#S ? null : this.#t.ownerDocument ?? null;
    e !== this.#p && (this.#p?.removeEventListener(
      "visibilitychange",
      this.#Di
    ), this.#p = e, e?.addEventListener("visibilitychange", this.#Di), this.#M?.disconnect(), e && this.#M?.observe(e, { childList: !0, subtree: !0 }));
  }
  #Di = () => {
    this.#J || this.#yi();
  };
  /** Deliver faster-playback captures even when a Worker owns the drawing loop. */
  #jt() {
    this.#S || this.#b || this.#x !== null || !this.#u.captureDriven || !this.#I || this.#te || (this.#x = this.#Ke(this.#Mi));
  }
  /** Cancel the page's pending capture delivery. */
  #_i() {
    this.#Zt(this.#x), this.#x = null;
  }
  /** Flush captured pictures on the refresh that will present them. */
  #Mi = (e) => {
    if (this.#x = null, !this.#I || this.#te) return;
    const t = this.#ne;
    this.#u.flush(e), this.#V(t) && this.#u.captureDriven && (this.#x = this.#Ke(this.#Mi));
  };
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
  #fr(e, t) {
    const i = this.#c / 2, s = (a) => {
      const u = a.at - e;
      return u <= i - O ? !0 : u > i + O ? !1 : this.#oe > 0;
    };
    for (; this.#r[1] && s(this.#r[1]); )
      this.#F.late++, this.#r.shift();
    const r = this.#r[0];
    if (!r || !s(r)) return;
    this.#r.shift(), this.#oe = r.at - e;
    const o = performance.now(), h = this.#Wt(!0);
    this.#Ni(r.slot), this.#we(h), this.#De += performance.now() - o, this.#be++, this.#ae && this.#dr(r, t), this.#ie = t;
  }
  /** Preserve upstream's opt-in presentation timing log in either renderer. */
  #dr(e, t) {
    const i = this.#ie === 0 ? 0 : t - this.#ie, s = i / this.#c, r = e.cadence === "film" ? `phase ${e.phase}` : e.cadence === "field" ? `field ${e.phase}` : "frame", o = e.phase === 0 ? "duplicate" : `phase ${z}`, h = e.droppedBefore > 0 ? `, ${o} dropped before it` + (e.droppedBefore > 1 ? ` (${e.droppedBefore})` : "") : "";
    console.log(
      `yadif: +${i.toFixed(2)} ms (${s.toFixed(2)} refreshes) ${e.cadence} ${r}, due ${(e.at - t).toFixed(2)} ms${h}`
    );
  }
  /** Copy one of the filtered pictures onto the canvas. */
  #Ni(e) {
    const t = this.#h[e];
    t && this.#qt(t.texture);
  }
  /** Put a progressive frame through unchanged, keeping one display surface. */
  #mr() {
    this.#bs();
    const e = this.#a[this.#D];
    e && this.#qt(e, !0), this.#N = 0;
  }
  /** Apply visibility on the page; a Worker reports only the desired state. */
  #Fe(e) {
    if (this.#S) {
      this.#S.onVisibility(e);
      return;
    }
    this.#t.style.visibility = e ? "visible" : "hidden";
  }
  #qt(e, t = !1, i = !0) {
    this.#Si(e, t), this.#l = { kind: "texture", texture: e, flip: t }, this.#Fe(!0), i && this.#pe++;
  }
  /** Put a texture onto the canvas: through the scaler, or copied as it is. */
  #Si(e, t) {
    const i = this.#xe, s = this.#dt;
    if (i && s) {
      const r = this.#s;
      r.bindFramebuffer(r.FRAMEBUFFER, null);
      let o = !1;
      try {
        o = i.draw(
          r,
          e,
          t,
          this.#C,
          this.#G,
          s.width,
          s.height
        );
      } catch (h) {
        console.warn(
          "yadif: the scaler failed; showing pictures as they are",
          h
        );
      }
      if (o) return;
      this.#xs();
    }
    this.#Es(e, t, null);
  }
  /** Copy a texture onto the canvas or into an output. */
  #Es(e, t, i) {
    const s = this.#s;
    s.bindFramebuffer(s.FRAMEBUFFER, i), s.useProgram(this.#v), s.activeTexture(s.TEXTURE0), s.bindTexture(s.TEXTURE_2D, e), s.uniform1i(this.#T, 0), s.uniform1i(this.#B, t ? 1 : 0), s.viewport(0, 0, this.#C, this.#G), s.drawArrays(s.TRIANGLES, 0, 3);
  }
  /**
   * Account for the frames between this one and the last one seen.
   *
   * There is no event for a frame the callback was not run for; the only sign
   * of one is that the count of frames went up by more than one. That count
   * comes from the compositor where it provides one, and otherwise from
   * VideoFrames. Frames thrown away either side of a discontinuity are not
   * counted: the held frames were being dropped anyway, and a seek presents
   * what it passes over.
   *
   * Returns how many frames were missed between the last one and this one.
   */
  #pr(e, t) {
    let i = 0;
    return this.#nt !== 0 && !t && (i = Math.max(0, e - this.#nt - 1), this.#F.missed += i), this.#nt = e, i;
  }
  /**
   * Frames stopped arriving for a while -- a pause, a stall, a tab in the
   * background -- and a rate averaged over time nothing was asked of the
   * filter says nothing about it. Begin the interval at this frame.
   */
  #Pi(e) {
    e - this.#ht <= re || (this.#yt = e, this.#Ae = 0, this.#ye = 0, this.#be = 0, this.#De = 0, this.#ve = 0, this.#pe = 0, this.#ot = 0, this.#We = 0, this.#at = 0, this.#He = 0);
  }
  #Ci(e) {
    const t = e - this.#yt;
    if (t < re) return;
    const i = !this.#b && this.#gt() && (this.#P || this.#Z) ? this.#be : this.#Ae, s = this.#Ae ? (this.#ye + this.#De) / this.#Ae : 0;
    let r;
    this.#Xe != null && (r = 0, this.#We !== 0 && (r += this.#ot / 1e6 / this.#We), this.#He !== 0 && (r += this.#at / 1e6 / this.#He / 2));
    const o = {
      ...this.#F,
      avDifference: this.#ce,
      // The element's own count of what its decoder could not keep up with,
      // which is the machine being behind rather than this filter.
      dropped: this.#e.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
      fps: i * 1e3 / t,
      frameMs: s,
      maxQueuedFields: this.#ve,
      outputFps: this.#pe * 1e3 / t,
      gpuMs: r,
      film: this.#Z,
      oneInstant: this.#ni,
      filmError: this.#Me
    };
    this.dispatchEvent(new CustomEvent("stats", { detail: o })), this.#ei?.(o), this.#yt = e, this.#Ae = 0, this.#ye = 0, this.#be = 0, this.#De = 0, this.#ve = 0, this.#pe = 0, this.#ot = 0, this.#We = 0, this.#at = 0, this.#He = 0;
  }
  /** Take the newest frame into the ring. */
  #bs() {
    const e = this.#s;
    this.#D = (this.#D + 1) % T;
    const t = this.#R, i = this.#a[this.#D];
    t !== null && this.#E !== null && i !== void 0 ? (this.#a[this.#D] = this.#E.adopt(t, i), this.#R = null) : (e.bindTexture(e.TEXTURE_2D, i ?? null), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      e.RGBA,
      e.UNSIGNED_BYTE,
      this.#Oe
    )), this.#N = Math.min(this.#N + 1, T);
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
  #me(e, t, i, s = !0) {
    if (this.#N === 0 || this.#te) return;
    const r = i === null && this.#dt ? this.#gs() : null, o = r !== null;
    r && (i = r.framebuffer), s && (this.#N === T && !e ? this.#F.filtered++ : this.#F.degraded++);
    const h = this.#s, { prev: a, cur: u, next: A } = this.#Ii(e);
    h.bindFramebuffer(h.FRAMEBUFFER, i), h.useProgram(this.#A);
    for (const [v, b] of [a, u, A].entries())
      h.activeTexture(h.TEXTURE0 + v), h.bindTexture(h.TEXTURE_2D, this.#a[b] ?? null);
    h.uniform1i(this.#o.prev, 0), h.uniform1i(this.#o.cur, 1), h.uniform1i(this.#o.next, 2);
    const f = this.#X ? this.#n?.texture ?? null : null, d = this.#X ? this.#n?.combTexture ?? null : null, l = f !== null && d !== null;
    f !== null && d !== null && (h.activeTexture(h.TEXTURE0 + 3), h.bindTexture(h.TEXTURE_2D, f), h.uniform1i(this.#o.fieldMetrics, 3), h.activeTexture(h.TEXTURE0 + 4), h.bindTexture(h.TEXTURE_2D, d), h.uniform1i(this.#o.comb, 4));
    const c = this.#hi ? this.#n?.combAheadTexture ?? null : null, m = l && !e;
    h.activeTexture(h.TEXTURE0 + 5), h.bindTexture(h.TEXTURE_2D, c), h.uniform1i(this.#o.combAhead, 5), h.activeTexture(h.TEXTURE0), h.uniform2i(this.#o.size, this.#C, this.#G);
    const E = this.#kt ? 0 : 1;
    h.uniform1i(this.#o.parity, t ? 1 - E : E), h.uniform1i(this.#o.tff, this.#kt ? 1 : 0), h.uniform1i(this.#o.second, t ? 1 : 0), h.uniform1i(this.#o.spatialCheck, this.#K ? 1 : 0), h.uniform1i(this.#o.debug, this.#ae ? 1 : 0), h.uniform1i(this.#o.film, l ? 1 : 0), h.uniform1i(
      this.#o.match,
      m && this.#_e && (!t || c !== null) ? 1 : 0
    ), h.uniform1i(this.#o.instant, m && this.#_t ? 1 : 0), h.uniform1i(this.#o.phase, this.#z.phase), h.viewport(0, 0, this.#C, this.#G), h.drawArrays(h.TRIANGLES, 0, 3), this.#ae && l && this.#Us(this.#Li, 0, 90), r && this.#Si(r.texture, !1), (i === null || o) && (this.#l = { kind: "yadif", flush: e, second: t }, this.#Fe(!0), s && this.#pe++);
  }
  #Ii(e) {
    const t = (i) => (this.#D + T - i) % T;
    return this.#N === 1 ? { prev: this.#D, cur: this.#D, next: this.#D } : e ? { prev: t(1), cur: this.#D, next: this.#D } : this.#N === 2 ? { prev: t(1), cur: t(1), next: this.#D } : { prev: t(2), cur: t(1), next: this.#D };
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
  #Je() {
    if (this.#yi(), !this.#ee) return;
    const e = this.#e, t = this.#b ? this.#Et : e.videoWidth, i = this.#b ? this.#bt : e.videoHeight;
    if (t === 0 || i === 0) return;
    const s = Math.min(
      e.offsetWidth / t,
      e.offsetHeight / i
    ), r = t * s, o = i * s;
    this.#t.style.left = `${e.offsetLeft + (e.offsetWidth - r) / 2}px`, this.#t.style.top = `${e.offsetTop + (e.offsetHeight - o) / 2}px`, this.#t.style.width = `${r}px`, this.#t.style.height = `${o}px`;
  }
  #Bi(e, t) {
    const i = this.#s;
    this.#C = e, this.#G = t, this.#N = 0, this.#W = 0, this.#l = null, this.#Kt(), this.#Ui(), this.#U(), this.#Je();
    for (const s of this.#a) i.deleteTexture(s);
    this.#a = [];
    for (let s = 0; s < T; s++) {
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
      ), this.#a.push(r);
    }
    this.#wt(), (this.#P || this.#X || this.#b) && this.#vs(), this.#lt?.resize(e, t), this.#n?.resize(e, t), this.#qe();
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
  #vs() {
    const e = this.#s;
    if (!(this.#h.length === w || this.#C === 0)) {
      this.#wt();
      for (let t = 0; t < w; t++) {
        const i = e.createTexture();
        e.bindTexture(e.TEXTURE_2D, i), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
          e.TEXTURE_2D,
          0,
          e.RGBA,
          this.#C,
          this.#G,
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
        const r = e.checkFramebufferStatus(e.FRAMEBUFFER) === e.FRAMEBUFFER_COMPLETE;
        if (e.bindFramebuffer(e.FRAMEBUFFER, null), !r) {
          e.deleteFramebuffer(s), e.deleteTexture(i), this.#wt();
          return;
        }
        this.#h.push({ texture: i, framebuffer: s });
      }
      this.#f = w - 1;
    }
  }
  #wt() {
    const e = this.#s, t = this.#l?.kind === "texture" ? this.#l.texture : null;
    this.#h.some((i) => i.texture === t) && (this.#l = null), this.#Pt = null;
    for (const { texture: i, framebuffer: s } of this.#h)
      e.deleteFramebuffer(s), e.deleteTexture(i);
    this.#h = [], this.#U();
  }
  /**
   * Wrap the element in a `<div>` of this one's own and put the canvas over
   * it. The wrapper is what the canvas is positioned against; moving the
   * element out of the tree and back within the one task leaves playback
   * alone, which is what makes turning this on mid-stream free.
   */
  #Er() {
    if (this.#ee) return;
    const e = this.#e.parentElement;
    if (!e) return;
    const t = document.createElement("div");
    t.style.cssText = "position:relative;display:inline-block;line-height:0;max-width:100%", e.insertBefore(t, this.#e), t.appendChild(this.#e), t.appendChild(this.#t), this.#ee = t, this.#re?.observe(this.#e);
    try {
      this.#Ct?.observe(this.#t, {
        box: "device-pixel-content-box"
      });
    } catch {
      this.#Ct?.observe(this.#t);
    }
    this.#Je();
  }
  #br() {
    if (this.#S) return;
    const e = this.#ee;
    this.#ee = null, this.#re?.disconnect(), this.#Ct?.disconnect(), this.#t.remove(), e?.parentElement && (e.parentElement.insertBefore(this.#e, e), e.remove());
  }
  #Ts = () => this.#Je();
  /**
   * Size the canvas's buffer for what puts the pictures up: the picture's own
   * size for the copy, or whatever the scaler asks for. Resizing the buffer
   * clears it, so the last picture goes up again.
   */
  #Ui() {
    const e = this.#C, t = this.#G;
    if (e === 0 || t === 0 || this.#te || this.#J) return;
    let i = null;
    if (this.#xe && !this.#S && this.#Qe && this.#Qe.width > 0 && this.#Qe.height > 0)
      try {
        i = this.#xe.size(
          this.#s,
          e,
          t,
          this.#Qe.width,
          this.#Qe.height
        );
      } catch (h) {
        console.warn(
          "yadif: the scaler failed; showing pictures as they are",
          h
        ), this.#xe = null;
      }
    this.#dt = i;
    const s = i ?? { width: e, height: t }, r = this.#i, o = r.width !== s.width || r.height !== s.height;
    o && (r.width = s.width, r.height = s.height), i || this.#Kt(), (o || i) && this.#vr();
  }
  /** Put the picture up again, on a canvas that has just been cleared. */
  #vr() {
    const e = this.#l;
    !this.#I || this.#te || !e || (e.kind === "texture" ? this.#qt(e.texture, e.flip, !1) : this.#me(e.flush, e.second, null, !1));
  }
  /** Let go of the scaler that failed, and copy pictures as they are again. */
  #xs() {
    this.#xe = null, this.#dt = null, this.#Kt(), this.#i.width = this.#C, this.#i.height = this.#G;
  }
  /** The picture-sized texture the scaler reads, made when it is first wanted. */
  #gs() {
    if (this.#ge) return this.#ge;
    const e = this.#s, t = e.createTexture();
    e.bindTexture(e.TEXTURE_2D, t), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MIN_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_MAG_FILTER, e.NEAREST), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_S, e.CLAMP_TO_EDGE), e.texParameteri(e.TEXTURE_2D, e.TEXTURE_WRAP_T, e.CLAMP_TO_EDGE), e.texImage2D(
      e.TEXTURE_2D,
      0,
      e.RGBA,
      this.#C,
      this.#G,
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
    return e.bindFramebuffer(e.FRAMEBUFFER, null), s ? (this.#ge = { texture: t, framebuffer: i }, this.#ge) : (e.deleteFramebuffer(i), e.deleteTexture(t), this.#xs(), null);
  }
  #Kt() {
    this.#ge && (this.#s.deleteFramebuffer(this.#ge.framebuffer), this.#s.deleteTexture(this.#ge.texture), this.#ge = null);
  }
  /** Forward a media event with the page's playback state that gives it meaning. */
  #Ue(e) {
    return !this.#m || this.#w === "main" ? !1 : (this.#m.postMessage({
      type: "event",
      name: e,
      video: this.#Ie()
    }), !0);
  }
  #ws = () => {
    if (this.#b) {
      this.#j.hold(), this.#Ue("emptied");
      return;
    }
    if (this.#Ue("emptied")) {
      this.#fe(), this.#Fe(!1);
      return;
    }
    this.#N = 0, this.#et = 0, this.#Jt = 0, this.#U(), this.#q(), this.#W = 0, this.#L = 0, this.#Fs(), this.#l = null, this.#Fe(!1);
  };
  #Fs() {
    this.#F = {
      filtered: 0,
      missed: 0,
      degraded: 0,
      discontinuities: 0,
      resynced: 0,
      late: 0,
      queueResetted: 0,
      matched: 0
    }, this.#mi = 0, this.#pi = 0, this.#Mt.fill(0), this.#Nt = 0, this.#nt = 0, this.#yt = 0, this.#ht = 0, this.#Ae = 0, this.#ye = 0, this.#be = 0, this.#De = 0, this.#ve = 0, this.#pe = 0, this.#U(), this.#ot = 0, this.#We = 0, this.#at = 0, this.#He = 0;
  }
  /**
   * A new seek invalidates any destination frame remembered for the last one.
   */
  #Rs = () => {
    if (this.#b) {
      this.#j.hold(), this.#Ue("seeking");
      return;
    }
    if (this.#Ue("seeking")) {
      this.#fe();
      return;
    }
    this.#tt = !1;
  };
  /**
   * Playback stopped, so the frame being held back goes up now. One picture,
   * whatever the rate: a still frame stands for a moment, and the moment is
   * the one the first field was taken at.
   */
  #Re = (e) => {
    if (this.#b) {
      this.#j.hold(), e.type === "ratechange" && this.#he && this.#e.playbackRate > 0 && (this.#le = this.#Ze), this.#Ue(
        e.type
      );
      return;
    }
    if (e.type === "ratechange" && (this.#u.captureDriven ? this.#jt() : this.#_i()), (e.type === "pause" || e.type === "ended" || e.type === "seeked" || e.type === "ratechange") && this.#Ue(e.type)) {
      this.#fe();
      return;
    }
    if (e.type === "seeked") {
      const i = this.#tt;
      if (this.#tt = !1, i) return;
      this.#N = 0, this.#U(), this.#q(), this.#l = null, this.#Fe(!1);
      return;
    }
    const t = e.type === "ratechange";
    if (t && (this.#L = 0, this.#et = this.#e.currentTime), this.#U(), this.#I && this.#N > 0) {
      const i = this.#fs(), s = i === null ? void 0 : this.#h[i];
      i !== null && s ? (this.#f = i, this.#me(!0, !1, s.framebuffer), this.#Ni(i)) : this.#me(!0, !1, null);
    }
    t && (this.#N = 0, this.#nt = 0, this.#q());
  };
  /** The element ran out of data. Its readings stop moving until it has more. */
  #ys = () => {
    this.#b && (this.#j.hold(), this.#Ue("waiting"));
  };
  /**
   * A lost context takes the textures and the program with it. Rebuilding
   * them is possible, but a page that has lost its context has bigger
   * problems; getting out of the way leaves the element's own picture showing.
   */
  #Ds = (e) => {
    e.preventDefault(), !(!this.#S && this.#w === "active") && (this.#te = !0, this.#Ot("the deinterlacer WebGL context was lost"));
  };
}
function fe(n, e) {
  const t = n.createProgram(), i = de(n, n.VERTEX_SHADER, bt), s = de(n, n.FRAGMENT_SHADER, e);
  if (n.attachShader(t, i), n.attachShader(t, s), n.linkProgram(t), n.deleteShader(i), n.deleteShader(s), !n.getProgramParameter(t, n.LINK_STATUS)) {
    const r = n.getProgramInfoLog(t);
    throw n.deleteProgram(t), new Error(
      `the deinterlacer failed to link: ${r ?? "no reason given"}`
    );
  }
  return t;
}
function de(n, e, t) {
  const i = n.createShader(e);
  if (!i) throw new Error("the deinterlacer could not create a shader");
  if (n.shaderSource(i, t), n.compileShader(i), !n.getShaderParameter(i, n.COMPILE_STATUS)) {
    const s = n.getShaderInfoLog(i);
    throw n.deleteShader(i), new Error(
      `the deinterlacer failed to compile: ${s ?? "no reason given"}`
    );
  }
  return i;
}
const me = "data:video/mp4;base64,AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAu9tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAB8nRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAFoAAABDgAAAAAAY5tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAHUwAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE5bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+XN0YmwAAACtc3RzZAAAAAAAAAABAAAAnWF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAFoAQ4AEgAAABIAAAAAAAAAAEVTGF2YzYxLjE5LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA3YXZjQwFkACn/4QAZZ2QAKazZQFoET94CIAAAfSAAHUwD4sWywAEAB2j5KBLLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABBzdHRzAAAAAAAAAAAAAAAQc3RzYwAAAAAAAAAAAAAAFHN0c3oAAAAAAAAAAAAAAAAAAAAQc3RjbwAAAAAAAAAAAAAAKG12ZXgAAAAgdHJleAAAAAAAAAABAAAAAQAAAAAAAAAAAAAAAAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhkbHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjEuNy4xMDAAAACYbW9vZgAAABBtZmhkAAAAAAAAAAEAAACAdHJhZgAAABx0ZmhkAAIAOAAAAAEAAAPpAAAEJwEBAAAAAAAUdGZkdAEAAAAAAAAAAAAAAAAAAEh0cnVuAAAKBQAAAAYAAACgAgAAAAAABCcAAAfSAAAAQgAAE40AAAA/AAAH0gAAAgAAAAAAAAAARAAAA+kAAAG7AAAH0gAACK9tZGF0AAACrwYF//+r3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTQgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDEzMyBtZT11bWggc3VibWU9MTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MSBtZV9yYW5nZT0yNCBjaHJvbWFfbWU9MSB0cmVsbGlzPTIgOHg4ZGN0PTEgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9LTIgdGhyZWFkcz0xNSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9dGZmIGJsdXJheV9jb21wYXQ9MCBjb25zdHJhaW5lZF9pbnRyYT0wIGJmcmFtZXM9MyBiX3B5cmFtaWQ9MiBiX2FkYXB0PTIgYl9iaWFzPTAgZGlyZWN0PTMgd2VpZ2h0Yj0xIG9wZW5fZ29wPTAgd2VpZ2h0cD0wIGtleWludD0zMCBrZXlpbnRfbWluPTMgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD0zMCByYz1jcmYgbWJ0cmVlPTEgY3JmPTguMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAAUGAQEygAAAAWdliIICAj/+/76ivgU3edyfbbnP6kzu1BfFPXa9rMu/FCi/GMk76JT20AAAAwAAAwAAAwAAAwAAAwAAAwEJmrWZnq7KhXxVTgAAAwAAAwAAAwAABJ9gAAADAAAKtgAAAwAAAwCi4AAAAwAAHQgAAAMAAAiqAAADAAADA7EAAAMAAAMCCgAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAL+QAAAAUGAQEygAAAADVBmiIWQj/51kP//f3t2AAPsAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAS8AAAAAUGAQEygAAAADJBnkETiEf/hv/80gAJcAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAkIQAAAAUGAQEygAAAAfMBnmCTRCP/9ZJR/1zH/6vL5qeSOTmASFdQlObW+4YAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAxvEAAAAwAAAwAAAwAAE4wAAAMAAAMAAAMAAFuAAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAAADAMuAAAAABQYBATKAAAAANwGeYZakI//1bXH/Een/+rAALngAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAAAMAN+EAAAAFBgEBMoAAAAGuQZpileloiEf/2XyP/Fn/6mXyw21/v4X7ly3FFO60AAADAAADAAADAAADAAADAAADAAADADKWVJAQiFeS9HQZhFSJuVc/HAAAAwAAAwAAAwAAAwAAAwAAAwAAj8AAAAMAAAMABTIAAAMAAAMAAD+QAAADAAADAAQkAAADAAADAABJgAAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAAAwAXUQAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAB9IAAAAAAAADCwEBAQAAABBtZnJvAAAAAAAAAEM=", Bt = 0.5, Ut = 3e3, pe = 0.1, C = 16, Ee = 'video/mp4; codecs="avc1.640029"';
let Z = null;
function Lt(n = {}) {
  return Z ??= kt(n), Z;
}
async function $t(n = {}) {
  return (await Lt(n)).deinterlaces;
}
function Vt() {
  Z = null;
}
async function kt(n) {
  const e = n.tolerance ?? Bt, t = n.timeoutMs ?? Ut, i = performance.now(), s = (h) => ({
    deinterlaces: !1,
    survives: null,
    tookMs: performance.now() - i,
    error: h instanceof Error ? h.message : String(h)
  });
  if (typeof document > "u")
    return s(new Error("there is no document to decode in"));
  const r = document.createElement("video");
  r.muted = !0, r.defaultMuted = !0, r.playsInline = !0, r.preload = "auto";
  let o = null;
  try {
    o = Gt(r, t);
    const h = H(W(r, "loadeddata"), t), a = r.play().then(
      () => !0,
      () => !1
    );
    if (await o.ready, await h, await Xt(r, t, await a), r.videoWidth === 0 || r.videoHeight === 0)
      return s(new Error("the probe clip decoded to nothing"));
    const u = zt(r);
    return {
      deinterlaces: u < 1 - e,
      survives: u,
      tookMs: performance.now() - i
    };
  } catch (h) {
    return s(h);
  } finally {
    r.pause(), r.removeAttribute("src"), r.replaceChildren(), r.load(), o && URL.revokeObjectURL(o.url);
  }
}
const Y = typeof MediaSource > "u" ? globalThis.ManagedMediaSource : MediaSource, Ot = typeof MediaSource > "u";
function Gt(n, e) {
  if (!Y || !Y.isTypeSupported(Ee))
    throw new Error("the probe clip needs Media Source Extensions");
  const t = me.indexOf(","), i = atob(me.slice(t + 1)), s = new Uint8Array(i.length);
  for (let a = 0; a < i.length; a++) s[a] = i.charCodeAt(a);
  const r = new Y(), o = URL.createObjectURL(r);
  if (Ot) {
    n.disableRemotePlayback = !0;
    const a = document.createElement("source");
    a.type = "video/mp4", a.src = o, n.append(a), n.load();
  } else
    n.src = o;
  const h = (async () => {
    await H(W(r, "sourceopen"), e);
    const a = r.addSourceBuffer(Ee), u = H(W(a, "updateend"), e);
    a.appendBuffer(s), await u, r.endOfStream();
  })();
  return { url: o, ready: h };
}
async function Xt(n, e, t) {
  if (t) {
    const i = performance.now();
    for (; n.currentTime < pe && performance.now() - i < e; )
      await new Promise((s) => requestAnimationFrame(s));
    n.pause();
  } else
    n.currentTime = pe, await H(W(n, "seeked"), e);
}
function zt(n) {
  const e = n.videoHeight, t = document.createElement("canvas");
  t.width = C, t.height = e;
  const i = t.getContext("2d", { willReadFrequently: !0 });
  if (!i) throw new Error("there is no 2d context to read the clip with");
  i.imageSmoothingEnabled = !1, i.drawImage(n, 0, 0, C, e);
  const s = i.getImageData(0, 0, C, e).data, r = (A) => {
    let f = 0;
    for (let d = 0; d < C; d++)
      f += s[(A * C + d) * 4 + 1] ?? 0;
    return f / C;
  };
  let o = 0;
  const h = 2, a = e - 3;
  let u = r(h);
  for (let A = h + 1; A <= a; A++) {
    const f = r(A);
    o += Math.abs(f - u), u = f;
  }
  return o / (a - h) / 255;
}
function W(n, e) {
  return new Promise((t, i) => {
    n.addEventListener(e, () => t(), { once: !0 }), n.addEventListener(
      "error",
      () => {
        const s = n instanceof HTMLMediaElement ? n.error : null, r = s ? ` (MediaError ${s.code}${s.message ? `: ${s.message}` : ""})` : "";
        i(new Error(`the probe clip ${e} failed${r}`));
      },
      { once: !0 }
    );
  });
}
function H(n, e) {
  return Promise.race([
    n,
    new Promise(
      (t, i) => setTimeout(
        () => i(new Error("the probe clip took too long")),
        e
      )
    )
  ]);
}
mt(Fe);
export {
  Ht as Deinterlacer,
  Ze as YADIF_FRAGMENT_SHADER,
  Qe as YADIF_UNIFORMS,
  $t as decoderDeinterlaces,
  Vt as forgetDecoderProbe,
  Lt as probeDecoder,
  Wt as supportsDeinterlace
};
