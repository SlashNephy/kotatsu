(function(){"use strict";const M=`#version 300 es
void main() {
  // From the vertex index alone. There is no geometry here worth a buffer.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;function N(h,e,t){const i=h.createProgram(),s=J(h,h.VERTEX_SHADER,t),r=J(h,h.FRAGMENT_SHADER,e);if(h.attachShader(i,s),h.attachShader(i,r),h.linkProgram(i),h.deleteShader(s),h.deleteShader(r),!h.getProgramParameter(i,h.LINK_STATUS)){const o=h.getProgramInfoLog(i);throw h.deleteProgram(i),new Error(`the deinterlacer failed to link: ${o??"no reason given"}`)}return i}function w(h,e,t,i=1){const s=h.createTexture();h.activeTexture(h.TEXTURE0),h.bindTexture(h.TEXTURE_2D,s),h.texStorage2D(h.TEXTURE_2D,1,h.RGBA8,1,1),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MIN_FILTER,h.NEAREST),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MAG_FILTER,h.NEAREST);const r=h.createFramebuffer();h.bindFramebuffer(h.FRAMEBUFFER,r);const o=[];for(let n=0;n<i;n++){const u=h.createTexture();h.bindTexture(h.TEXTURE_2D,u),h.texStorage2D(h.TEXTURE_2D,1,t,1,1),h.framebufferTexture2D(h.FRAMEBUFFER,h.COLOR_ATTACHMENT0+n,h.TEXTURE_2D,u,0),o.push(u)}h.drawBuffers(o.map((n,u)=>h.COLOR_ATTACHMENT0+u)),h.bindTexture(h.TEXTURE_2D,s),h.useProgram(e),h.viewport(0,0,1,1),h.drawArrays(h.TRIANGLES,0,3),h.bindFramebuffer(h.FRAMEBUFFER,null),h.deleteFramebuffer(r);for(const n of o)h.deleteTexture(n);h.deleteTexture(s)}function J(h,e,t){const i=h.createShader(e);if(!i)throw new Error("the deinterlacer could not create a shader");if(h.shaderSource(i,t),h.compileShader(i),!h.getShaderParameter(i,h.COMPILE_STATUS)){const s=h.getShaderInfoLog(i);throw h.deleteShader(i),new Error(`the deinterlacer failed to compile: ${s??"no reason given"}`)}return i}const xe=`#version 300 es
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
`,ge=`#version 300 es
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
`;function Fe(h,e){const t=N(h,xe,ge),i=h.getAttribLocation(t,"aVertexPosition"),s=h.getAttribLocation(t,"aTextureCoord"),r=h.getUniformLocation(t,"uTexture"),o=h.getUniformLocation(t,"uMatrix"),n=h.getUniformLocation(t,"uUvMatrix"),u=h.getUniformLocation(t,"uTextColor"),c=h.getUniformLocation(t,"uBackColor");if(r==null||o==null||n==null||u==null||c==null)throw new Error("failed to initialize DEBUG_FRAGMENT_SHADER, DEBUG_VERTEX_SHADER");const a=h.createBuffer(),d=h.createBuffer();return{gl:h,...Re(h,e),program:t,programUniforms:{vertex:i,textureCoord:s,texture:r,matrix:o,uvMatrix:n,textColor:u,backColor:c},positionBuffer:a,textureBuffer:d}}function Re(h,e){const t=new OffscreenCanvas(0,0),i=t.getContext("2d"),s=new Map;let r=0;const o=0;let n=1;i.font=e,i.fillStyle="white";for(let c=32;c<128;c++){const a=String.fromCharCode(c),d=i.measureText(a),m=Math.ceil(d.actualBoundingBoxDescent+d.actualBoundingBoxAscent+1),l=Math.ceil(d.actualBoundingBoxLeft+d.actualBoundingBoxRight+1);s.set(a,{x:r,y:o,width:l,height:m,metrics:d}),n=Math.max(n,m),r+=l}t.width=r,t.height=n,i.font=e,i.fillStyle="white";for(const[c,a]of s)i.fillText(c,Math.floor(a.x+a.metrics.actualBoundingBoxLeft+1),Math.floor(a.metrics.actualBoundingBoxAscent+1));const u=h.createTexture();return h.bindTexture(h.TEXTURE_2D,u),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MIN_FILTER,h.LINEAR),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MAG_FILTER,h.LINEAR),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_WRAP_S,h.CLAMP_TO_EDGE),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_WRAP_T,h.CLAMP_TO_EDGE),h.texImage2D(h.TEXTURE_2D,0,h.RGBA,h.RGBA,h.UNSIGNED_BYTE,t),{fontTexture:u,chars:s,textureSize:{width:r,height:n}}}function ye(h){const e=h.gl;e.deleteBuffer(h.positionBuffer),e.deleteBuffer(h.textureBuffer),e.deleteTexture(h.fontTexture),e.deleteProgram(h.program)}function _e(h,e,t,i,s,r,o){const n=[],u=[],c=t;for(const m of e){if(m===`
`){t=c,i+=o;continue}const l=h.chars.get(m);if(l==null)continue;if(l.width===1){t+=l.metrics.width;continue}const f=Math.floor(t-l.metrics.actualBoundingBoxLeft),p=Math.floor(i-l.metrics.actualBoundingBoxAscent),b=f+l.width,x=p+l.height;n.push(f,p),u.push(l.x,l.y),n.push(f,x),u.push(l.x,l.y+l.height),n.push(f+l.width,x),u.push(l.x+l.width,l.y+l.height),n.push(b,x),u.push(l.x+l.width,l.y+l.height),n.push(f,p),u.push(l.x,l.y),n.push(b,p),u.push(l.x+l.width,l.y),t+=l.metrics.width}const a=h.gl;a.useProgram(h.program),a.bindBuffer(a.ARRAY_BUFFER,h.positionBuffer),a.bufferData(a.ARRAY_BUFFER,new Float32Array(n),a.STATIC_DRAW),a.vertexAttribPointer(h.programUniforms.vertex,2,a.FLOAT,!1,0,0),a.enableVertexAttribArray(h.programUniforms.vertex),a.bindBuffer(a.ARRAY_BUFFER,h.textureBuffer),a.bufferData(a.ARRAY_BUFFER,new Float32Array(u),a.STATIC_DRAW),a.vertexAttribPointer(h.programUniforms.textureCoord,2,a.FLOAT,!1,0,0),a.enableVertexAttribArray(h.programUniforms.textureCoord),a.activeTexture(a.TEXTURE0),a.bindTexture(a.TEXTURE_2D,h.fontTexture),a.uniform1i(h.programUniforms.texture,0),a.uniform3fv(h.programUniforms.textColor,[1,1,1]),a.uniform3fv(h.programUniforms.backColor,[0,0,0]);function d(m,l,f){const p=[];for(let b=0;b<l;b++)for(let x=0;x<m;x++)p.push(f[x*m+b]);return p}a.uniformMatrix4fv(h.programUniforms.matrix,!1,d(4,4,[1/(s/2),0,0,-1,0,-2/r,0,1,0,0,1,0,0,0,0,1])),a.uniformMatrix3fv(h.programUniforms.uvMatrix,!1,d(3,3,[1/h.textureSize.width,0,0,0,1/h.textureSize.height,0,0,0,1])),a.viewport(0,0,s,r),a.enable(a.BLEND),a.blendFunc(a.SRC_ALPHA,a.ONE_MINUS_SRC_ALPHA),a.drawArrays(a.TRIANGLES,0,n.length/2),a.disable(a.BLEND)}const E={firstRepeatsPrevious:0,secondRepeatsNext:1,secondRepeatsPrevious:2,previousSecondRepeated:3,firstRepeatsNext:4,previousFirstRepeated:5,phase:6,instant:7},U=8,y=16,G=8,Ae=15,Ne=6,we=8,Se=30,Me=.01,Pe=.002,Z=2,z=1,De=2,q=10,Ce={a:"uA",b:"uB",fieldMetrics:"uFieldMetrics",first:"uFirst",size:"uSize"},ee=16,te=8,Ie=`#version 300 es

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
  const int BLOCK_W = ${ee};
  const int BLOCK_H = ${te};

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

  float run = texelFetch(uFieldMetrics, ivec2(${E.phase}, 0), 0)[1];
  float threshold = run == 0.0 ? 0.025 : (run <= 10.0 ? 0.11 : 0.15);

  vec4 even = measure(diffEven, float(totalEven), threshold);
  vec4 odd = measure(diffOdd, float(totalOdd), threshold);
  outFirst = uFirst == 0 ? even : odd;
  outSecond = uFirst == 0 ? odd : even;
}
`,Ue={second:"uSecond",first:"uFirst",size:"uSize"},L=8,ke=`#version 300 es
precision highp float;

uniform sampler2D uSecond;
uniform sampler2D uFirst;

uniform ivec2 uSize;

layout(location = 0) out vec4 outSecond;
layout(location = 1) out vec4 outFirst;

void main()
{
  ivec2 dst = ivec2(gl_FragCoord.xy);
  ivec2 base = dst * ${L};

  vec4 second = vec4(0.0);
  vec4 first = vec4(0.0);

  for (int y = 0; y < ${L}; ++y) {
    for (int x = 0; x < ${L}; ++x) {
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
`,Le={previous:"uPrevious",second:"uSecond",first:"uFirst",comb:"uComb",size:"uSize"},Be=`#version 300 es
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
  return differing <= ${Z}.0;
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
  bool believed = run >= ${q}.0;
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
  if (max(dSecondPrevious[2], dFirstPrevious[2]) <= ${Pe}) return vec4(score, 0.0, 0.0, last[3]);
  ivec2 blocks = textureSize(uComb, 0);
  float combed = 0.0;
  for (int y = 0; y < blocks.y; ++y) {
    for (int x = 0; x < blocks.x; ++x) {
      if (texelFetch(uComb, ivec2(x, y), 0)[0] > ${G}.0) combed += 1.0;
    }
  }
  bool clean = combed <= ${Me} * float(blocks.x * blocks.y);
  score = clean ? min(score + 1.0, ${Se}.0) : max(score - ${we}.0, 0.0);
  bool on = last[3] > 0.5 ? score >= ${Ne}.0 : score >= ${Ae}.0;
  return vec4(score, clean ? 1.0 : 0.0, combed, on ? 1.0 : 0.0);
}

void main()
{
  int metric = int(gl_FragCoord.x);
  // The comparisons against the next frame become, a frame later, the ones
  // against the previous frame, and those the ones before.
  if (metric == ${E.firstRepeatsPrevious}) {
    outValue = previous(${E.firstRepeatsNext});
  } else if (metric == ${E.secondRepeatsNext}) {
    outValue = fold(uSecond);
  } else if (metric == ${E.secondRepeatsPrevious}) {
    outValue = previous(${E.secondRepeatsNext});
  } else if (metric == ${E.previousSecondRepeated}) {
    outValue = previous(${E.secondRepeatsPrevious});
  } else if (metric == ${E.firstRepeatsNext}) {
    outValue = fold(uFirst);
  } else if (metric == ${E.previousFirstRepeated}) {
    outValue = previous(${E.firstRepeatsPrevious});
  } else if (metric == ${E.instant}) {
    outValue = instant(
      previous(${E.instant}),
      previous(${E.secondRepeatsNext}),
      previous(${E.firstRepeatsNext})
    );
  } else {
    outValue = decide(
      previous(${E.phase}),
      previous(${E.firstRepeatsNext}),
      fold(uSecond),
      previous(${E.secondRepeatsNext}),
      previous(${E.secondRepeatsPrevious}),
      fold(uFirst),
      previous(${E.firstRepeatsPrevious})
    );
  }
}
`,Oe={prev:"uPrev",cur:"uCur",first:"uFirst",size:"uSize"},Xe=`#version 300 es
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
`,Ge={flags:"uFlags",size:"uSize"},ze=`#version 300 es
precision highp float;
precision highp int;

uniform sampler2D uFlags;
/** The size of the marks. */
uniform ivec2 uSize;

out vec4 outValue;

void main() {
  ivec2 block = ivec2(gl_FragCoord.xy);
  ivec2 base = block * ivec2(${y}, ${y/2});
  ivec2 end = min(base + ivec2(${y}, ${y/2}), uSize);
  vec2 count = vec2(0.0);
  for (int y = base.y; y < end.y; ++y)
    for (int x = base.x; x < end.x; ++x)
      count += texelFetch(uFlags, ivec2(x, y), 0).rg;
  outValue = vec4(count, 0.0, 0.0);
}
`,$e={prev:"uPrev",cur:"uCur",next:"uNext",size:"uSize",parity:"uParity",tff:"uTff",spatialCheck:"uSpatialCheck",debug:"uDebug",film:"uFilm",second:"uSecond",phase:"uPhase",fieldMetrics:"uFieldMetrics",comb:"uComb",combAhead:"uCombAhead",match:"uMatch",instant:"uInstant"},Ve=`#version 300 es
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
  return texelFetch(uFieldMetrics, ivec2(metric, 0), 0)[1] <= ${Z}.0;
}

/** The pulldown phase the detection gave this frame, or 0. See film-shader.ts. */
int detectedPhase() {
  vec4 phase = texelFetch(uFieldMetrics, ivec2(${E.phase}, 0), 0);
  // Deinterlace each field normally until the cadence is confirmed.
  return phase[1] >= ${q}.0 ? int(phase[0]) : 0;
}

bool isMixedPhase(int phase) {
  return phase == ${z} || phase == ${De};
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
  ivec2 block = ivec2(x / ${y}, y / ${y});
  float standing = texelFetch(uComb, block, 0)[0];
  if (!neighbours) {
    if (standing > ${G}.0) return false;
    rgb = texelFetch(uCur, ivec2(x, y), 0).rgb;
    return true;
  }
  float adjoining = uSecond ? texelFetch(uCombAhead, block, 0)[1]
                            : texelFetch(uComb, block, 0)[1];
  if (min(standing, adjoining) > ${G}.0) return false;
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
  return texelFetch(uFieldMetrics, ivec2(${E.instant}, 0), 0)[3] > 0.5;
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
    vec4 comb = texelFetch(uComb, ivec2(x / ${y}, y / ${y}), 0);
    bool combedBlock = (mixed ? comb[1] : comb[0]) > ${G}.0;
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
`,P={phase:0,run:0};function B(h,e,t){return Object.fromEntries(Object.entries(t).map(([i,s])=>[i,h.getUniformLocation(e,s)]))}class ie{#t;#i;#e;#l;#v;#_;#s;#h;#u;#o;#T;#x=null;#U=null;#a=null;#n=null;#d=null;#c=null;#r=0;#m=null;#F=null;#w=0;#V=0;metrics=new Float32Array(U*4);#g=0;#E=0;constructor(e){this.#t=e,this.#i=N(e,Ie,M),this.#e=B(e,this.#i,Ce),this.#l=N(e,ke,M),this.#v=B(e,this.#l,Ue),this.#_=N(e,Be,M),this.#s=B(e,this.#_,Le),this.#h=N(e,Xe,M),this.#u=B(e,this.#h,Oe),this.#o=N(e,ze,M),this.#T=B(e,this.#o,Ge),w(e,this.#i,e.RGBA32F,2),w(e,this.#l,e.RGBA32F,2),w(e,this.#_,e.RGBA32F),w(e,this.#h,e.RG8),w(e,this.#o,e.RGBA32F)}get texture(){return this.#c?.[this.#r]?.textures[0]??null}get combTexture(){return this.#n?.textures[0]??null}get combAheadTexture(){return this.#d?.textures[0]??null}resize(e,t){e===this.#g&&t===this.#E||(this.#g=e,this.#E=t,this.#f())}reset(e=!1){const t=this.#t;t.deleteSync(this.#F),this.#F=null,this.#w=0;const i=this.#c?.[this.#r];if(!i)return;const s=e?E.instant:U,r=new Float32Array(s*4);for(let o=0;o<E.phase;o++)r[o*4+1]=1;t.bindTexture(t.TEXTURE_2D,i.textures[0]??null),t.texSubImage2D(t.TEXTURE_2D,0,0,0,s,1,t.RGBA,t.FLOAT,r)}detect(e,t,i){const s=this.#t;if(this.#g===0||this.#E===0)return;this.#A();const r=this.#x,o=this.#U,n=this.#c;if(r===null||o===null||n===null)return;const u=n[this.#r],c=n[1-this.#r];if(s.bindFramebuffer(s.FRAMEBUFFER,r.framebuffer),s.useProgram(this.#i),this.#S(0,e,this.#e.a),this.#S(1,t,this.#e.b),this.#S(2,u.textures[0],this.#e.fieldMetrics),s.uniform1i(this.#e.first,i),s.uniform2i(this.#e.size,this.#g,this.#E),s.viewport(0,0,r.width,r.height),s.drawArrays(s.TRIANGLES,0,3),s.bindFramebuffer(s.FRAMEBUFFER,o.framebuffer),s.useProgram(this.#l),this.#S(0,r.textures[0],this.#v.second),this.#S(1,r.textures[1],this.#v.first),s.uniform2i(this.#v.size,r.width,r.height),s.viewport(0,0,o.width,o.height),s.drawArrays(s.TRIANGLES,0,3),s.bindFramebuffer(s.FRAMEBUFFER,c.framebuffer),s.useProgram(this.#_),this.#S(0,u.textures[0],this.#s.previous),this.#S(1,o.textures[0],this.#s.second),this.#S(2,o.textures[1],this.#s.first),this.#S(3,this.#n?.textures[0],this.#s.comb),s.uniform2i(this.#s.size,o.width,o.height),s.viewport(0,0,U,1),s.drawArrays(s.TRIANGLES,0,3),this.#r=1-this.#r,this.#w++,this.#F!==null){s.bindFramebuffer(s.FRAMEBUFFER,null);return}this.#V=this.#w,s.bindBuffer(s.PIXEL_PACK_BUFFER,this.#m),s.readPixels(0,0,U,1,s.RGBA,s.FLOAT,0),s.bindBuffer(s.PIXEL_PACK_BUFFER,null),s.bindFramebuffer(s.FRAMEBUFFER,null),this.#F=s.fenceSync(s.SYNC_GPU_COMMANDS_COMPLETE,0),s.flush()}measureComb(e,t,i,s=!1){const r=this.#t;if(this.#g===0||this.#E===0)return;this.#A();const o=s?this.#d:this.#n,n=this.#a;o===null||n===null||(r.bindFramebuffer(r.FRAMEBUFFER,n.framebuffer),r.useProgram(this.#h),this.#S(0,e,this.#u.prev),this.#S(1,t,this.#u.cur),r.uniform1i(this.#u.first,i),r.uniform2i(this.#u.size,this.#g,this.#E),r.viewport(0,0,n.width,n.height),r.drawArrays(r.TRIANGLES,0,3),r.bindFramebuffer(r.FRAMEBUFFER,o.framebuffer),r.useProgram(this.#o),this.#S(0,n.textures[0],this.#T.flags),r.uniform2i(this.#T.size,n.width,n.height),r.viewport(0,0,o.width,o.height),r.drawArrays(r.TRIANGLES,0,3),r.bindFramebuffer(r.FRAMEBUFFER,null))}poll(){const e=this.#t,t=this.#F;if(t===null||this.#m===null)return null;switch(e.clientWaitSync(t,0,0)){case e.ALREADY_SIGNALED:case e.CONDITION_SATISFIED:return e.bindBuffer(e.PIXEL_PACK_BUFFER,this.#m),e.getBufferSubData(e.PIXEL_PACK_BUFFER,0,this.metrics),e.bindBuffer(e.PIXEL_PACK_BUFFER,null),e.deleteSync(t),this.#F=null,{phase:this.metrics[E.phase*4]??0,run:this.metrics[E.phase*4+1]??0,instant:(this.metrics[E.instant*4+3]??0)>.5,age:this.#w-this.#V};default:return null}}destroy(){const e=this.#t;if(this.#f(),this.#c!==null){for(const t of this.#c)C(e,t);this.#c=null}e.deleteSync(this.#F),this.#F=null,e.deleteBuffer(this.#m),this.#m=null,e.deleteProgram(this.#i),e.deleteProgram(this.#l),e.deleteProgram(this.#_),e.deleteProgram(this.#h),e.deleteProgram(this.#o)}#S(e,t,i){const s=this.#t;s.activeTexture(s.TEXTURE0+e),s.bindTexture(s.TEXTURE_2D,t??null),s.uniform1i(i,e)}#f(){const e=this.#t;this.#x!==null&&C(e,this.#x),this.#U!==null&&C(e,this.#U),this.#a!==null&&C(e,this.#a),this.#n!==null&&C(e,this.#n),this.#d!==null&&C(e,this.#d),this.#x=null,this.#U=null,this.#a=null,this.#n=null,this.#d=null}#A(){const e=this.#t;if(this.#x===null||this.#U===null||this.#a===null||this.#n===null||this.#d===null){this.#f();const t=Math.ceil(this.#g/ee),i=Math.ceil(this.#E/(te*2));this.#x=D(e,t,i,2),this.#U=D(e,Math.ceil(t/L),Math.ceil(i/L),2),this.#a=D(e,this.#g,Math.ceil(this.#E/2),1,"marks"),this.#n=D(e,Math.ceil(this.#g/y),Math.ceil(this.#E/y),1),this.#d=D(e,Math.ceil(this.#g/y),Math.ceil(this.#E/y),1)}this.#c===null&&(this.#c=[D(e,U,1,1),D(e,U,1,1)],this.#r=0,this.reset()),this.#m===null&&(this.#m=e.createBuffer(),e.bindBuffer(e.PIXEL_PACK_BUFFER,this.#m),e.bufferData(e.PIXEL_PACK_BUFFER,this.metrics.byteLength,e.STREAM_COPY),e.bindBuffer(e.PIXEL_PACK_BUFFER,null))}}function D(h,e,t,i,s="floats"){const r=s==="marks",o=h.createFramebuffer();h.bindFramebuffer(h.FRAMEBUFFER,o);const n=[];for(let a=0;a<i;a++){const d=h.createTexture();h.bindTexture(h.TEXTURE_2D,d),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MIN_FILTER,h.NEAREST),h.texParameteri(h.TEXTURE_2D,h.TEXTURE_MAG_FILTER,h.NEAREST),h.texImage2D(h.TEXTURE_2D,0,r?h.RG8:h.RGBA32F,e,t,0,r?h.RG:h.RGBA,r?h.UNSIGNED_BYTE:h.FLOAT,null),h.framebufferTexture2D(h.FRAMEBUFFER,h.COLOR_ATTACHMENT0+a,h.TEXTURE_2D,d,0),n.push(d)}h.drawBuffers(n.map((a,d)=>h.COLOR_ATTACHMENT0+d));const u=h.checkFramebufferStatus(h.FRAMEBUFFER)===h.FRAMEBUFFER_COMPLETE;h.bindFramebuffer(h.FRAMEBUFFER,null);const c={framebuffer:o,textures:n,width:e,height:t};if(!u)throw C(h,c),new Error("failed to allocate framebuffer");return c}function C(h,{framebuffer:e,textures:t}){h.deleteFramebuffer(e);for(const i of t)h.deleteTexture(i)}function se(h){return Number.isInteger(h)&&h>0}function We(h){const{timestamp:e,duration:t,width:i,height:s,displayWidth:r,displayHeight:o,data:n,planes:u,matrix:c}=h;if(!Number.isFinite(e)||!(t>0)||!Number.isFinite(t))throw new TypeError(`invalid picture timing ${e}+${t}`);if(!se(i)||!se(s))throw new TypeError(`invalid picture size ${i}x${s}`);if(!(r>0)||!(o>0))throw new TypeError(`invalid display size ${r}x${o}`);if(c!=="bt709"&&c!=="bt601")throw new TypeError(`unknown matrix ${String(c)}`);if(u.length!==3)throw new TypeError("a picture has three planes");for(let a=0;a<3;a++){const d=a===0?i:Math.ceil(i/2),m=a===0?s:Math.ceil(s/2),{offset:l,stride:f}=u[a];if(!Number.isInteger(l)||l<0||!Number.isInteger(f)||f<d||l+f*(m-1)+d>n.byteLength)throw new TypeError(`plane ${a} at ${l} with stride ${f} does not fit ${d}x${m} samples in ${n.byteLength} bytes`)}}function re(h,e){const t=1-h-e;return Float32Array.of(2*(1-h),2*(1-e)*e/t,2*(1-h)*h/t,2*(1-e))}const He={bt709:re(.2126,.0722),bt601:re(.299,.114)},qe=`#version 300 es
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
`;class Ke{#t;#i;#e;#l;#v;#_;#s;#h;#u=[];#o=0;#T=0;#x=!1;constructor(e){this.#t=e,this.#i=N(e,qe,M),this.#e=e.getUniformLocation(this.#i,"uLuma"),this.#l=e.getUniformLocation(this.#i,"uCb"),this.#v=e.getUniformLocation(this.#i,"uCr"),this.#_=e.getUniformLocation(this.#i,"uInterlaced"),this.#s=e.getUniformLocation(this.#i,"uMatrix"),this.#h=e.createFramebuffer(),w(e,this.#i,e.RGBA8)}resize(e,t){const i=this.#t;for(const s of this.#u)i.deleteTexture(s);this.#u=[];for(let s=0;s<3;s++){const r=i.createTexture();i.bindTexture(i.TEXTURE_2D,r),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MIN_FILTER,i.NEAREST),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MAG_FILTER,i.NEAREST),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_WRAP_S,i.CLAMP_TO_EDGE),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_WRAP_T,i.CLAMP_TO_EDGE),i.texStorage2D(i.TEXTURE_2D,1,i.R8,s===0?e:Math.ceil(e/2),s===0?t:Math.ceil(t/2)),this.#u.push(r)}this.#o=e,this.#T=t,this.#x=!1}convert(e,t){const i=this.#t,s=new Uint8Array(e.data);i.pixelStorei(i.UNPACK_ALIGNMENT,1);for(let r=0;r<3;r++){const{offset:o,stride:n}=e.planes[r];i.bindTexture(i.TEXTURE_2D,this.#u[r]),i.pixelStorei(i.UNPACK_ROW_LENGTH,n),i.texSubImage2D(i.TEXTURE_2D,0,0,0,r===0?this.#o:Math.ceil(this.#o/2),r===0?this.#T:Math.ceil(this.#T/2),i.RED,i.UNSIGNED_BYTE,s,o)}if(i.pixelStorei(i.UNPACK_ROW_LENGTH,0),i.pixelStorei(i.UNPACK_ALIGNMENT,4),i.bindFramebuffer(i.FRAMEBUFFER,this.#h),i.framebufferTexture2D(i.FRAMEBUFFER,i.COLOR_ATTACHMENT0,i.TEXTURE_2D,t,0),!this.#x){if(i.checkFramebufferStatus(i.FRAMEBUFFER)!==i.FRAMEBUFFER_COMPLETE)throw i.bindFramebuffer(i.FRAMEBUFFER,null),new Error("decoded pictures cannot be drawn into RGBA8");this.#x=!0}i.useProgram(this.#i);for(let r=0;r<3;r++)i.activeTexture(i.TEXTURE0+r),i.bindTexture(i.TEXTURE_2D,this.#u[r]);i.uniform1i(this.#e,0),i.uniform1i(this.#l,1),i.uniform1i(this.#v,2),i.uniform1i(this.#_,e.interlaced?1:0),i.uniform4fv(this.#s,He[e.matrix]),i.viewport(0,0,this.#o,this.#T),i.drawArrays(i.TRIANGLES,0,3),i.activeTexture(i.TEXTURE0),i.bindFramebuffer(i.FRAMEBUFFER,null)}destroy(){const e=this.#t;for(const t of this.#u)e.deleteTexture(t);this.#u=[],e.deleteFramebuffer(this.#h),e.deleteProgram(this.#i)}}const he=250,F=4,Ye=.01,je=100,Qe=250,ne=.5,S=2;class Je{#t=0;#i=Number.NaN;#e=0;#l=1;#v=!0;#_=Number.NaN;#s=0;#h=!1;#u=0;#o=0;#T=Number.NaN;#x=0;#U=new Float64Array(F);#a=0;#n=!0;#d=new Float64Array(F).fill(Number.NaN);#c=0;#r=0;#m=!1;#F=!1;#w=!1;#V=new Float64Array(3).fill(Number.NaN);#g=0;#E=Number.NaN;#S=Number.NaN;#f=Number.NaN;#A=Number.NaN;#oe=!1;#ie=Number.NEGATIVE_INFINITY;at(e){return this.#v&&!Number.isNaN(this.#A)&&!Number.isNaN(this.#i)?this.#q(e):this.#i+this.#e*(e-this.#t)/1e3}#q(e){const t=this.#l,i=this.#S+t*(e-this.#f-this.#A)/1e3,s=this.#_+t*(e-this.#o)/1e3;return Math.max(this.#ie,Math.min(s,Math.max(this.#E,i)))}get rate(){return this.#e}get origin(){return this.#T}hold(){this.#n=!0,this.#i=Number.NaN,this.#e=0}observe(e,t,i,s){const r=e-this.#o;if(this.#o=e,!s||i===0||this.#n||i!==this.#l){const T=i!==0&&(!s||this.#l===0||i===this.#l);return this.#n=!1,this.#l=i,this.#D(e,t),T&&(this.#E=t),!1}const o=this.#r!==0;if(!this.#v&&!o&&this.#se(r),t===this.#_){if(this.#v)return Number.isNaN(this.#A)||(this.#oe=!0),!1;if(o)return e>this.#r&&(this.#m=!0,this.#O(e,t,!0)),!1;const T=this.#ee(i),X=Number.isNaN(T)?Qe:T+r+S;return e-this.#s>X&&this.#O(e,t,!Number.isNaN(T)),!1}const n=(t-this.#_)*1e3/i,u=e-this.#s,c=u-r,a=this.#u;this.#_=t,this.#s=e,this.#u=r;const d=n<=u+a+S,m=n>=u*ne&&d;if(this.#v){const T=!this.#h;if(this.#h=!0,T&&!Number.isNaN(this.#E)&&c<=S){this.#S=t,this.#f=e;const Te=this.#re();this.#A=Te>r+S?Te:Number.NaN}else this.#oe&&n>=r*ne&&n<=r+a+S&&(this.#ie=t);this.#w&&!T&&m&&this.#W(c,i),this.#w=!1;const X=this.#k(i,r);if(!T&&m){if(X||this.#F)return this.#m=!1,this.#K(e,t,i,r,X),!0;this.#F=!0}else if(this.#F=!1,T&&!this.#m&&X&&n>0&&d)return this.#K(e,t,i,r,!0),!0;return this.#t=e,this.#i=t,!1}if(o){if(!m)return this.#m=!0,this.#D(e,t),!1;this.#r=0,this.#m=!1}else m&&this.#W(c,i);const l=t-this.at(e);if(Math.abs(l)>je/1e3*i)return this.#T=t,this.#J(e,t),!0;const f=this.#U;e-this.#a>=he?(f.copyWithin(0,1),f[F-1]=l,this.#a=e):f[F-1]=Math.max(f[F-1],l);let p=f[0];for(let T=1;T<F;T++)p=Math.max(p,f[T]);const b=Ye*i*(e-this.#x)/1e3,x=Math.max(-b,Math.min(b,p));this.#x=e,this.#i+=x;for(let T=0;T<F;T++)f[T]=f[T]-x;return!1}#se(e){this.#c+=e,!(this.#c<he)&&(this.#d.copyWithin(0,1),this.#d[F-1]=0,this.#c=0)}#K(e,t,i,s,r){Number.isNaN(this.#f)||(this.#V[this.#g++%this.#V.length]=(this.#S-t)*1e3/i+(e-this.#f)),this.#J(e,t),r&&(this.#r=e+this.#ee(i)+s+S)}#O(e,t,i){const s=this.#s;this.#D(e,t),i&&(this.#s=s,this.#h=!0,this.#w=!0)}#W(e,t){const i=this.#d[F-1],s=e*t/1e3;this.#d[F-1]=Number.isNaN(i)?s:Math.max(i,s)}#k(e,t){let i=Number.POSITIVE_INFINITY;for(let r=0;r<F-1;r++){const o=this.#d[r];if(Number.isNaN(o))return!1;i=Math.min(i,o)}const s=i*1e3/e;return s>S&&s+S>=t}#ee(e){let t=Number.NaN;for(let s=0;s<F-1;s++){const r=this.#d[s];Number.isNaN(r)||(t=Number.isNaN(t)?r:Math.max(t,r))}if(Number.isNaN(t))return t;const i=this.#d[F-1];return Number.isNaN(i)||(t=Math.max(t,i)),t*1e3/e}#re(){const[e,t,i]=this.#V;return Number.isNaN(e)||Number.isNaN(t)||Number.isNaN(i)?Number.NaN:Math.max(Math.min(e,t),Math.min(Math.max(e,t),i))}#D(e,t){this.#E=Number.NaN,this.#S=Number.NaN,this.#f=Number.NaN,this.#A=Number.NaN,this.#oe=!1,this.#ie=Number.NEGATIVE_INFINITY,this.#w=!1,this.#F=!1,this.#r=0,this.#t=e,this.#i=t,this.#e=0,this.#v=!0,this.#h=!1,this.#_=t,this.#s=e,this.#T=t}#J(e,t){this.#f=Number.NaN,this.#A=Number.NaN,this.#r=0,this.#t=e,this.#i=t,this.#e=this.#l,this.#v=!1,this.#x=e,this.#a=e,this.#U.fill(Number.NEGATIVE_INFINITY),this.#U[F-1]=0}}const O=16,Ze=6,et=`#version 300 es
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
  ivec2 origin = ivec2(gl_FragCoord.xy) * ${O};
  vec3 difference = vec3(0.0);
  for (int y = 0; y < ${O}; y++) {
    for (int x = 0; x < ${O}; x++) {
      ivec2 at = min(origin + ivec2(x, y), last);
      difference = max(
        difference,
        abs(texelFetch(uTake, at, 0).rgb - texelFetch(uReference, at, 0).rgb)
      );
    }
  }
  fragColor = vec4(difference, 1.0);
}
`;class tt{#t;#i;#e=null;#l=null;#v=[];#_=0;#s=[];#h=new Uint8Array(0);#u=[];#o=null;#T=0;constructor(e,t){this.#t=e,this.#i=t}take(){const e=this.#t,t=this.#v.pop()??this.#n();if(t===null)return null;const i=this.#i.videoWidth,s=this.#i.videoHeight;e.bindTexture(e.TEXTURE_2D,t);try{e.texImage2D(e.TEXTURE_2D,0,e.RGBA,e.RGBA,e.UNSIGNED_BYTE,this.#i)}catch(n){throw this.#v.push(t),n}const r={id:++this.#T,texture:t,width:i,height:s,changed:null,fence:null,buffer:null,reported:!1},o=this.#o;return o===null||o.width!==i||o.height!==s?r.changed=!0:this.#x(r,o),this.#o=r,this.#u.push(r),r.id}settle(){const e=this.#t,t=this.#u.find(i=>!i.reported);if(t===void 0||t===this.#o)return null;if(t.changed===null){const{fence:i,buffer:s}=t;if(i===null||s===null)return null;const r=e.clientWaitSync(i,0,0);if(r!==e.ALREADY_SIGNALED&&r!==e.CONDITION_SATISFIED)return null;this.#h.byteLength<s.bytes&&(this.#h=new Uint8Array(s.bytes));const o=this.#h.subarray(0,s.bytes);e.bindBuffer(e.PIXEL_PACK_BUFFER,s.buffer),e.getBufferSubData(e.PIXEL_PACK_BUFFER,0,o),e.bindBuffer(e.PIXEL_PACK_BUFFER,null),t.changed=it(o),this.#U(t)}return t.reported=!0,{id:t.id,changed:t.changed}}adopt(e,t){const i=this.#u.findIndex(r=>r.id===e),s=this.#u[i];if(s===void 0||!s.reported)throw new Error(`picture ${e} is not available to adopt`);return this.#u.splice(i,1),this.#v.push(t),s.texture}release(e){const t=this.#u.findIndex(s=>s.id===e),i=this.#u[t];i!==void 0&&(this.#u.splice(t,1),this.#a(i))}reset(){const e=this.#u;this.#u=[],this.#o=null;for(const t of e)this.#U(t),this.#a(t)}destroy(){const e=this.#t;this.reset();for(const t of this.#v)e.deleteTexture(t);this.#v=[],this.#_=0;for(const{buffer:t}of this.#s)e.deleteBuffer(t);this.#s=[],this.#r(),this.#e!==null&&e.deleteProgram(this.#e),this.#e=null}#x(e,t){const i=this.#t,s=Math.ceil(e.width/O),r=Math.ceil(e.height/O),o=this.#c(s,r),n=this.#m(s*r*4);this.#e??=this.#d(),i.bindFramebuffer(i.FRAMEBUFFER,o.framebuffer),i.useProgram(this.#e),i.activeTexture(i.TEXTURE1),i.bindTexture(i.TEXTURE_2D,t.texture),i.activeTexture(i.TEXTURE0),i.bindTexture(i.TEXTURE_2D,e.texture),i.viewport(0,0,s,r),i.drawArrays(i.TRIANGLES,0,3),i.bindBuffer(i.PIXEL_PACK_BUFFER,n.buffer),i.readPixels(0,0,s,r,i.RGBA,i.UNSIGNED_BYTE,0),i.bindBuffer(i.PIXEL_PACK_BUFFER,null),i.bindFramebuffer(i.FRAMEBUFFER,null),e.fence=i.fenceSync(i.SYNC_GPU_COMMANDS_COMPLETE,0),e.buffer=n,i.flush()}#U(e){const t=this.#t;e.fence!==null&&t.deleteSync(e.fence),e.fence=null;const i=e.buffer;if(e.buffer=null,i===null)return;const s=this.#l;s!==null&&i.bytes===s.width*s.height*4?this.#s.push(i):t.deleteBuffer(i.buffer)}#a(e){this.#v.push(e.texture)}#n(){if(this.#_===Ze)return null;const e=this.#t,t=e.createTexture();return e.bindTexture(e.TEXTURE_2D,t),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),this.#_++,t}#d(){const e=this.#t,t=N(e,et,M);return e.useProgram(t),e.uniform1i(e.getUniformLocation(t,"uTake"),0),e.uniform1i(e.getUniformLocation(t,"uReference"),1),t}#c(e,t){const i=this.#t,s=this.#l;if(s?.width===e&&s.height===t)return s;this.#r();for(const{buffer:u}of this.#s)i.deleteBuffer(u);this.#s=[];const r=i.createTexture();i.bindTexture(i.TEXTURE_2D,r),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MIN_FILTER,i.NEAREST),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MAG_FILTER,i.NEAREST),i.texImage2D(i.TEXTURE_2D,0,i.RGBA8,e,t,0,i.RGBA,i.UNSIGNED_BYTE,null);const o=i.createFramebuffer();i.bindFramebuffer(i.FRAMEBUFFER,o),i.framebufferTexture2D(i.FRAMEBUFFER,i.COLOR_ATTACHMENT0,i.TEXTURE_2D,r,0),i.bindFramebuffer(i.FRAMEBUFFER,null);const n={framebuffer:o,texture:r,width:e,height:t};return this.#l=n,n}#r(){const e=this.#t;this.#l!==null&&(e.deleteFramebuffer(this.#l.framebuffer),e.deleteTexture(this.#l.texture),this.#l=null)}#m(e){const t=this.#s.pop();if(t!==void 0)return t;const i=this.#t,s=i.createBuffer();return i.bindBuffer(i.PIXEL_PACK_BUFFER,s),i.bufferData(i.PIXEL_PACK_BUFFER,e,i.STREAM_COPY),i.bindBuffer(i.PIXEL_PACK_BUFFER,null),{buffer:s,bytes:e}}}function it(h){for(let e=0;e<h.length;e+=4)if(h[e]!==0||h[e+1]!==0||h[e+2]!==0)return!0;return!1}const oe=["mozParsedFrames","mozDecodedFrames","mozPresentedFrames","mozPaintedFrames"];function st(h){return oe.every(e=>e in h)}function rt(h){return h.ownerDocument?.defaultView?.performance.timeOrigin??performance.timeOrigin}const ht=250,K=500,nt=1.75,ot=3,at=200;class ut{#t;#i;#e;#l=null;#v=[];#_=0;#s=null;#h=0;#u=null;#o=null;#T=null;#x=null;#U=null;#a=null;#n=!0;#d=null;#c=null;#r=0;#m;#F;#w=null;#V=null;#g=null;#E=null;#S=0;#f=[];#A=[];constructor(e,t){if(this.#t=e,this.#e=t,this.#i=st(e)?e:null,this.#m=this.#i===null&&typeof VideoFrame<"u",this.#F=this.#m&&e.playbackRate>1,this.#i){for(const i of["emptied","seeking","seeked"])e.addEventListener(i,this.#W);for(const i of["pause","playing","waiting","ratechange"])e.addEventListener(i,this.#O)}if(this.#m)for(const i of["loadeddata","playing","pause","ended","seeking","seeked","emptied","ratechange"])e.addEventListener(i,this.#ie)}get captureDriven(){return this.#F}request(e){if(this.#x===null){if(this.#x=e,this.#F){this.#q();return}this.#T=this.#i?requestAnimationFrame(this.#D):this.#t.requestVideoFrameCallback(this.#ee)}}cancel(){this.#T!==null&&(this.#i?cancelAnimationFrame(this.#T):this.#t.cancelVideoFrameCallback(this.#T)),this.#T=null,this.#x=null,this.#w?.(),this.#w=null,this.#g!==null&&this.#t.cancelVideoFrameCallback(this.#g),this.#g=null,this.#oe(),this.#E=null,this.#f=[],this.#F=this.#m&&this.#t.playbackRate>1,this.#W()}destroy(){this.cancel();for(const e of["emptied","seeking","seeked"])this.#t.removeEventListener(e,this.#W);for(const e of["pause","playing","waiting","ratechange"])this.#t.removeEventListener(e,this.#O);for(const e of["loadeddata","playing","pause","ended","seeking","seeked","emptied","ratechange"])this.#t.removeEventListener(e,this.#ie)}flush(e){if(this.#F)for(this.#t.ownerDocument!==this.#V&&(this.#w?.(),this.#w=null,this.#q());this.#A.length>0&&this.#x!==null;){const t=this.#A.shift(),i=t.frame;try{this.#re(e,{width:i.visibleRect?.width??i.codedWidth,height:i.visibleRect?.height??i.codedHeight,mediaTime:i.timestamp/1e6,presentedFrames:t.count,expectedDisplayTime:Math.max(t.at,e),timeOrigin:performance.timeOrigin,frame:i})}finally{i.close()}}}#oe(){for(const e of this.#A)e.frame.close();this.#A=[]}#ie=e=>{const t=this.#m&&this.#t.playbackRate>1;if(t!==this.#F){const i=this.#x;this.cancel(),this.#F=t,i!==null&&this.request(i);return}["seeking","seeked","emptied","ratechange"].includes(e.type)&&(this.#E=null,this.#f=[]),this.#F&&((e.type==="pause"||e.type==="ended")&&this.flush(performance.now()),this.#oe(),this.#w?.(),this.#w=null,this.#x!==null&&this.#q())};#q(){if(!this.#F||this.#w!==null||this.#x===null||(this.#t.paused||this.#t.ended)&&this.#E!==null)return;this.#g===null&&typeof this.#t.requestVideoFrameCallback=="function"&&(this.#g=this.#t.requestVideoFrameCallback(this.#se));const e=Math.max(4,8/Math.max(1,this.#t.playbackRate)),t=this.#t.ownerDocument?.defaultView;if(this.#V=this.#t.ownerDocument,t){const i=t.setTimeout(this.#K,e);this.#w=()=>t.clearTimeout(i)}else{const i=setTimeout(this.#K,e);this.#w=()=>clearTimeout(i)}}#se=()=>{this.#g=null,this.#w?.(),this.#w=null,this.#K()};#K=()=>{this.#w=null;const e=this.#t;if(this.#x===null)return;if(e.readyState<2||e.seeking){this.#q();return}let t;try{t=new VideoFrame(e)}catch(i){if(!(i instanceof DOMException)||i.name!=="InvalidStateError")throw i;this.#q();return}if(t.timestamp===this.#E)t.close();else{let i=1;if(this.#E!==null){const s=t.timestamp-this.#E;if(s>1e3&&s<25e4&&(this.#f.push(s),this.#f.length>7&&this.#f.shift()),s>0&&this.#f.length>0){const r=[...this.#f].sort((n,u)=>n-u),o=r[Math.floor(r.length/2)];i=Math.max(1,Math.round(s/o))}}for(this.#E=t.timestamp,this.#S+=i,this.#A.push({frame:t,at:performance.now(),count:this.#S});this.#A.length>4;)this.#A.shift().frame.close()}this.#q()};#O=()=>{this.#d=null,this.#c=null,this.#r=0};#W=()=>{this.#U=null,this.#a=null,this.#n=!0,this.#k(),this.#O()};#k(){this.#l?.reset(),this.#v=[],this.#h=0}#ee=(e,t)=>{this.#re(e,{width:t.width,height:t.height,mediaTime:t.mediaTime,presentedFrames:t.presentedFrames,expectedDisplayTime:t.expectedDisplayTime,timeOrigin:rt(this.#t)})};#re=(e,t)=>{const i=this.#x;this.#T=null,this.#x=null,i?.(e,t)};#D=e=>{const t=this.#i,i=oe.map(u=>t[u]);this.#U?.some((u,c)=>i[c]<u)&&this.#W(),this.#U=i;const s=this.#e?.()??null;s!==this.#l&&(this.#W(),this.#l=s);const r=t.mozPaintedFrames,o=!t.seeking&&t.readyState>=2&&t.videoWidth>0&&t.videoHeight>0,n=this.#a===null&&(r>0||t.paused&&(t.mozPresentedFrames>0||t.mozDecodedFrames>0));if(s!==null){this.#Ee(e,t,s,o,n);return}if(o&&(n||this.#a!==null&&r!==this.#a)){this.#d!==null&&e-this.#d>K&&(this.#O(),this.#n=!0),this.#J(e,t),this.#d=e,this.#a=r;const u=this.#n;this.#n=!1,this.#re(e,{width:t.videoWidth,height:t.videoHeight,mediaTime:t.currentTime,presentedFrames:r,expectedDisplayTime:e,timeOrigin:performance.timeOrigin,mozTiming:{periodMs:this.#r,discontinuity:u}})}else this.#T=requestAnimationFrame(this.#D)};#J(e,t){if(t.paused||t.ended)return;const i=t.mozPaintedFrames,s=this.#c;if(s&&e-s.at>=ht){const r=i-s.frames;if(r>0){const o=(e-s.at)/r;o>=4&&o<=200&&(this.#r=this.#r?this.#r+(o-this.#r)*.25:o)}this.#c=null}this.#c??={at:e,frames:i}}#Ee(e,t,i,s,r){const o=t.mozPaintedFrames,n=this.#a!==null&&o!==this.#a;this.#s!==null&&e-this.#s>K&&(this.#k(),this.#O(),this.#n=!0),this.#s=e,n&&this.#J(e,t);const u=t.paused||t.ended;(!u||n)&&(this.#h=0),u&&this.#u!==null&&(this.#u=e);const c=u&&this.#h>=ot;if(s&&(r||this.#a!==null)){this.#a=o;const l=c?null:i.take();l!==null&&this.#v.push({id:l,at:e,mediaTime:t.currentTime,width:t.videoWidth,height:t.videoHeight,resting:u})}let a=null;for(let l=i.settle();l!==null;l=i.settle()){const f=this.#v.shift();if(f?.id!==l.id)throw new Error("picture verdicts arrived out of order");let p=l.changed;if(p?this.#h=0:(this.#h=f.resting?this.#h+1:0,p=a===null&&!f.resting&&this.#u!==null&&(f.at-this.#u)*(t.playbackRate||1)>=at&&o!==this.#o),!p){i.release(f.id);continue}a!==null&&i.release(a.id),a=f}if(a===null){this.#T=requestAnimationFrame(this.#D);return}let d=1;if(this.#d!==null){const l=a.at-this.#d;l>K?(this.#O(),this.#n=!0):this.#r>0&&l>this.#r*nt&&(d=Math.round(l/this.#r))}this.#_+=d,this.#d=a.at,this.#u=a.at,this.#o=o;const m=this.#n;this.#n=!1;try{this.#re(e,{width:a.width,height:a.height,mediaTime:a.mediaTime,presentedFrames:this.#_,expectedDisplayTime:e,timeOrigin:performance.timeOrigin,mozTiming:{periodMs:this.#r,discontinuity:m},picture:a.id})}finally{i.release(a.id)}}}let lt=null;const Y=.5,g=4,j=5,_=j+1,ae=1e3,ue=4,$=200,ct=.25,ft=1e3/60,dt=`#version 300 es
void main() {
  // One triangle over the whole viewport, from the vertex index alone. There
  // is no geometry here worth a buffer: every pixel is the fragment shader's.
  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`,mt=.75,pt=.1,le=1,Et=60,vt=.02,bt=.1,V=1,Tt=.02,xt=.005,gt=250,ce=1e3,fe=16,Ft=4,Rt=90,yt=1.5,de=4,_t=24,I=.001,W=5,me=4,pe={2:0,3:.25,4:.5,5:.75};function Ee(h){return Nt[(h.interlaced?2:0)+(h.topFieldFirst?1:0)]}const At=`#version 300 es
precision highp float;
uniform sampler2D uField;
uniform bool uFlip;
out vec4 fragColor;
void main() {
  ivec2 position = ivec2(gl_FragCoord.xy);
  if (uFlip) position.y = textureSize(uField, 0).y - 1 - position.y;
  fragColor = texelFetch(uField, position, 0);
}
`,Nt=[{interlaced:!1,topFieldFirst:!1},{interlaced:!1,topFieldFirst:!0},{interlaced:!0,topFieldFirst:!1},{interlaced:!0,topFieldFirst:!0}],wt={requestAnimationFrame:h=>requestAnimationFrame(h),cancelAnimationFrame:h=>cancelAnimationFrame(h)},St={setTimeout:(h,e)=>setTimeout(h,e),clearTimeout:h=>clearTimeout(h)};class Mt extends EventTarget{decodedVideo;#t;#i;#e;#l;#v;#_=null;#s;#h=null;#u;#o;#T;#x;#U;#a=[];#n=[];#d=_-1;#c=null;#r=[];#m=null;#F=null;#w=Number.NEGATIVE_INFINITY;#V=0;#g=null;#E=null;#S;#f=ft;#A=0;#oe=0;#ie=0;#q=0;#se=null;#K=0;#O=0;#W=0;#k(){this.#r.length=0,this.#qe.length=0,this.#se=null,this.#K=0,this.#O=0,this.#q=0}#ee=null;#re;#D;#J;#Ee=0;#ae;#G;#L=0;#C=0;#X=0;#N=g-1;#M=0;#et=0;#Zt=0;#tt=!1;#I=!1;#he=0;#Le=!1;#Z=!1;#B=null;#Be=[];#te=!1;#yt=null;#ei;#ti;#P;#Oe;#ve;#ii;#p=null;#R;#it=!1;#si=0;#ri=!1;#ws=0;#st=!1;#_t=!1;#Xe=null;#Ss=0;#rt=new Map;#y={filtered:0,missed:0,degraded:0,discontinuities:0,resynced:0,late:0,queueResetted:0,matched:0};#ht=0;#nt=0;#At=0;#ue=0;#Ae=0;#be=0;#Ne=0;#Te=0;#Ge;#Nt=[];#ze=[];#ot=0;#$e=0;#at=0;#Ve=0;#ut=P;#We=0;#z=P;#Y=!1;#$=0;#wt=!1;#we=!1;#hi=!1;#ni=!1;#oi=!1;#lt=!1;#He=null;#Li="";#St=[0,0,0,0,0,0];#Mt=0;#Se=null;#ai=null;#ui=0;#b=!1;#Me=[];#li=!1;#j=new Je;#ct;#Bi=!1;#xe=Array.from({length:g},()=>({timestamp:0,duration:0,interlaced:!0,topFieldFirst:!0,repeatFirstField:!1}));#ci=!1;#Pt=Number.NEGATIVE_INFINITY;#Pe=!1;#ft=!1;#qe=[];#Dt=null;#De=null;#dt=null;#ge=null;#mt=null;#Ke=null;#Fe=null;#Ct;#pt=Number.NEGATIVE_INFINITY;#ne=!1;#le=0;#Ye=0;#Et=1;#Ce=null;#ce=Number.NaN;#Oi=0;#fe=Number.NaN;#Xi=0;#fi=Number.NaN;#di=!0;#vt=0;#bt=0;#mi=null;#It=!1;#pi=0;#Ei=0;#Ut=!1;#kt=!1;#Gi=Number.NaN;#zi=Number.NaN;#Lt=new Map;#Ms=0;constructor(e,t={},i=null){super(),this.#e=e,this.#D=t.doubleRate??!1,this.#J=t.spatialCheck??!0,this.#ae=t.debug??!1,this.#G=t.film??!1,this.#De=t.interpolator??null,this.#ge=t.scaler??null,this.#ei=t.onStats,this.#ti=t.onFailure,this.#P=i,this.#ve=i?"main":t.rendering??"main",this.#ii=t.workerUrl??lt,this.#R=this.#ve==="main"?"main":"idle",this.#i=i?i.canvas:document.createElement("canvas"),this.#t=i?.canvas??(this.#ve==="main"?this.#i:document.createElement("canvas")),this.#Oe=e,i||(this.#i.style.cssText="position:absolute;pointer-events:none;visibility:hidden");const s=this.#t.getContext("webgl2",{alpha:!0,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!1,powerPreference:"high-performance"});if(!(s instanceof WebGL2RenderingContext))throw new Error("this browser has no WebGL2");this.#s=s,this.#G&&(this.#Wi(),this.#h=new ie(s)),this.#u=ve(s,Ve);const r=this.#u;this.#o=Object.fromEntries(Object.entries($e).map(([o,n])=>[o,s.getUniformLocation(r,n)])),this.#T=ve(s,At),this.#x=s.getUniformLocation(this.#T,"uField"),this.#U=s.getUniformLocation(this.#T,"uFlip"),w(s,this.#u,s.RGBA8),w(s,this.#T,s.RGBA8),this.#ct=new Ke(s),this.#Ds(),this.#Ge=s.getExtension("EXT_disjoint_timer_query_webgl2"),this.#t.addEventListener("webglcontextlost",this.#Ns),this.#re=i?null:new ResizeObserver(()=>this.#Ze()),this.#Ct=i?null:new ResizeObserver(([o])=>{if(!o)return;const n=window.devicePixelRatio||1,u={width:Math.round(o.contentRect.width*n),height:Math.round(o.contentRect.height*n)},c=o.devicePixelContentBoxSize?.[0];this.#Ke=c&&Math.abs(c.inlineSize-u.width)<=1&&Math.abs(c.blockSize-u.height)<=1?{width:c.inlineSize,height:c.blockSize}:u,this.#ki()}),this.#S=i?null:new MutationObserver(()=>{this.#i.ownerDocument!==this.#E&&this.#Ze()}),this.decodedVideo={push:o=>this.#Vs(o),reset:()=>this.#Qi(),finish:()=>{this.#li=!0,this.#p?.postMessage({type:"pictures",action:"finish"})},onrelease:null,avDifference:()=>this.#p?this.#fi:this.#fe,startsAt:(o,n)=>{if(!Number.isFinite(o)||!Number.isFinite(n))throw new RangeError(`start at ${o} s, ${n} ms`);if(this.#p){this.#p.postMessage({type:"start",media:o,at:n,timeOrigin:performance.timeOrigin,video:this.#Ie()});return}this.#Ce={media:o,wall:n}},rendererStatus:()=>this.#yt!==null?{state:"failed",reason:this.#yt}:{state:(this.#R==="main"?this.#Bi:this.#R==="active")?"prepared":"preparing"},onrendererstatus:null},this.#v=i?null:new tt(s,e),this.#l=new ut(e,()=>this.#R==="main"?this.#v:null),e.addEventListener("emptied",this.#Rs),e.addEventListener("resize",this.#xs),e.addEventListener("pause",this.#_e),e.addEventListener("ended",this.#_e),e.addEventListener("seeking",this.#_s),e.addEventListener("seeked",this.#_e),e.addEventListener("ratechange",this.#_e),e.addEventListener("waiting",this.#As)}get running(){return this.#b?this.#I:this.#I&&(this.#B?.interlaced??!0)}get canvas(){return this.#i}get#Bt(){return this.#b?this.#di:this.#B?.topFieldFirst!==!1}#$i(){return{doubleRate:this.#D,spatialCheck:this.#J,film:this.#G,debug:this.#ae}}get enabled(){return this.#Le}set enabled(e){this.#Le=e,this.#Xt(),this.#p?.postMessage({type:"enabled",enabled:e})}set scan(e){if(this.#b)return;const t=this.#B?.interlaced!==e?.interlaced,i=t||this.#B?.topFieldFirst!==e?.topFieldFirst;this.#B=e,!(i&&(!this.#Qe()||this.#B!==e))&&(this.#p?.postMessage({type:"scan",scan:e}),i&&(this.#M=0,this.#k(),this.#Q(),t&&(this.#L=0),this.#c=null,this.#ye(!1)),this.#Xt(),i&&((e?.interlaced??!0)&&(this.#P||this.#R==="main")?this.#Ue():this.#yi()))}get scan(){return this.#b?this.#Dt?.scan??null:this.#B}set videoTimeline(e){this.#Be=e,this.#p?.postMessage({type:"timeline",videoTimeline:e}),e.length===0&&(this.#B=null),this.#Xt()}get videoTimeline(){return this.#Be}get container(){return this.#ee??this.#e}get doubleRate(){return this.#D}set doubleRate(e){e!==this.#D&&(this.#D=e,this.#je(),!this.#b&&(this.#k(),this.#Vi()))}get spatialCheck(){return this.#J}set spatialCheck(e){e!==this.#J&&(this.#J=e,this.#je())}get film(){return this.#G}set film(e){const t=this.#p?this.#ai:this.#Se;if(!(e===this.#G&&(e===!1||t===null))){if(this.#p){this.#G=e,this.#je(e?"film":void 0);return}if(e){if(!this.#Qe())return;try{this.#Hi()}catch(i){this.#G=!0,this.#je(),this.#Gt(`film detector unavailable: ${i instanceof Error?i.message:String(i)}`);return}}if(this.#G=e,this.#je(),!e){if(!this.#Qe())return;this.#Y=!1,this.#$=0,this.#we=!1,this.#z=P,this.#Q(),this.#h?.destroy(),this.#h=null}this.#Vi()}}get debug(){return this.#ae}set debug(e){e!==this.#ae&&(this.#ae=e,this.#je())}get interpolator(){return this.#De}set interpolator(e){e!==this.#De&&(this.#De=e,this.#dt=null)}get scaler(){return this.#ge}set scaler(e){this.#ge=e,this.#ki()}get#Ps(){return this.#D||this.#G}#Vi(){this.#Z||this.#b||(this.#Ps?(this.#C>0&&this.#Ts(),(this.#B?.interlaced??!0)&&(this.#P||this.#R==="main")&&this.#Ue()):this.#G||(this.#c=null,this.#ye(!1),this.#Rt()))}#je(e){this.#p?.postMessage({type:"settings",options:this.#$i(),retryFilm:e})}#Wi(){if(this.#s.getExtension("EXT_color_buffer_float")===null)throw new Error("film needs EXT_color_buffer_float")}#Ds(){const e=this.#s,t=e.fenceSync(e.SYNC_GPU_COMMANDS_COMPLETE,0);e.flush();const i=this.#ps(),s=()=>{if(e.clientWaitSync(t,0,0)===e.TIMEOUT_EXPIRED){i.setTimeout(s,fe);return}e.deleteSync(t),this.#Bi=!0,this.#R==="main"&&this.decodedVideo.onrendererstatus?.()};i.setTimeout(s,fe)}#Ot(e){if(this.#yt=e,this.#P){this.#P.onFailure(e),this.stop();return}this.#Ki(e),this.stop(),this.decodedVideo.onrendererstatus?.()}#Hi(){this.#Wi(),this.#h??=new ie(this.#s),this.#C>0&&this.#h.resize(this.#C,this.#X)}#Xt(){this.#b||this.#Le&&(this.#Be.length>0||(this.#B?.interlaced??!0))?this.start():this.stop()}#Cs(){return this.#P||this.#ve==="main"?!1:this.#R==="starting"||this.#R==="active"?!0:typeof Worker<"u"&&typeof VideoFrame<"u"&&typeof OffscreenCanvas<"u"&&this.#ii!==null&&"transferControlToOffscreen"in HTMLCanvasElement.prototype?(this.#qi(),!0):this.#ve==="auto"?(this.#zt(),!1):(this.#R="failed",this.#Ot("rendering in a Worker needs Worker, VideoFrame, OffscreenCanvas transfer and a worker URL, and this page lacks one"),!0)}#qi(){this.#de(),this.#p?.terminate(),this.#p=null,this.#Vt(),this.#st=!1,this.#_t=!1,this.#ai=null,this.#ui=0;let e=this.#i;if(this.#ri){e=document.createElement("canvas"),e.className=this.#i.className;const r=this.#i.getAttribute("style");r===null?e.removeAttribute("style"):e.setAttribute("style",r),e.style.visibility="hidden",this.#i.parentElement&&this.#i.replaceWith(e),this.#i=e}const t=++this.#si;this.#R="starting",this.decodedVideo.onrendererstatus?.();let i,s;try{s=e.transferControlToOffscreen(),this.#ri=!0,i=new Worker(this.#ii,{type:"module"})}catch(r){this.#Tt(r instanceof Error?r.message:String(r));return}this.#p=i,i.onmessage=r=>{t===this.#si&&this.#Is(r.data)},i.onerror=r=>{t===this.#si&&(r.preventDefault(),this.#Tt(r.message||"the deinterlacer worker failed"))},i.postMessage({type:"initialize",canvas:s,options:this.#$i(),scan:this.#B,videoTimeline:this.#Be,enabled:this.#I,video:this.#Ie()},[s]),this.#Ut&&i.postMessage({type:"pageVisibility",hidden:!0,video:this.#Ie()})}#Is(e){switch(e.type){case"ready":this.#R="active",this.decodedVideo.onrendererstatus?.(),this.#I&&(this.#xt(),this.#jt());break;case"failed":this.#Tt(e.message);break;case"consumed":{this.#st=!1,this.#_t=!0;const t=this.#Xe;this.#Xe=null,t&&this.#ji(t);break}case"visibility":this.#i.style.visibility=e.visible?"visible":"hidden";break;case"stats":{this.#b&&(this.#Ei=e.stats.missed+e.stats.queueResetted),this.#fi=e.stats.avDifference;const t={...e.stats,dropped:this.#e.getVideoPlaybackQuality?.().droppedVideoFrames??0},i=t.filmError??null,s=this.#ui;if(this.#ai=i,this.#ui=e.filmFailure,i!==null&&e.filmFailure!==s){this.dispatchEvent(new CustomEvent("failure",{detail:i}));try{this.#ti?.(i)}catch{}}this.dispatchEvent(new CustomEvent("stats",{detail:t})),this.#ei?.(t);break}case"capture":{const t=this.#rt.get(e.id);if(this.#rt.delete(e.id),!t){e.image?.close();break}e.image?t.resolve(e.image):this.#b?t.reject(new Error("no rendered picture is available")):createImageBitmap(this.#e).then(t.resolve,t.reject);break}case"release":this.#Lt.delete(e.id)&&this.#me(e.data);break}}#Ki(e){if(this.dispatchEvent(new CustomEvent("failure",{detail:e})),!this.#P)try{this.#ti?.(e)}catch{}}#Gt(e){this.#Se!==e&&(this.#Se=e,this.#Y=!1,this.#$=0,this.#we=!1,this.#z=P,this.#Q(),this.#h?.destroy(),this.#h=null,this.#Ki(e))}#Qe(){return this.#Z?!1:(this.#p||(this.#Se=null),!0)}#Tt(e){if(this.#R==="starting"&&this.#ve==="auto"&&!this.#it){this.#zt();return}if(this.#Yi(e),!this.#it){this.#it=!0,this.#qi();return}console.error(`Deinterlacer Worker stopped: ${e}`),this.#R="failed",this.#p?.terminate(),this.#p=null,this.#de(),this.#Vt(),this.#Ot(`deinterlacer worker stopped: ${e}`)}#zt(){const e=this.#t;e.className=this.#i.className;const t=this.#i.getAttribute("style");t===null?e.removeAttribute("style"):e.setAttribute("style",t),e.style.visibility="hidden",this.#i.parentElement&&this.#i.replaceWith(e),this.#i=e,this.#ri=!1,this.#p?.terminate(),this.#p=null,this.#R="main",this.#de(),this.#Vt(),this.decodedVideo.onrendererstatus?.(),this.#I&&(this.#xt(),this.#jt(),(this.#B?.interlaced??!0)&&this.#Ue())}#de(){this.#Xe?.frame.close(),this.#Xe=null}#Yi(e){for(const t of this.#rt.values())t.reject(new Error(e));this.#rt.clear()}start(){if(!(this.#I||this.#Z||this.#yt!==null)&&(this.#he++,this.#I=!0,this.#ys(),!!this.#Qe())){if(this.#vr(),this.#jt(),this.#Cs()){if(this.#b){this.#p?.postMessage({type:"running",running:!0}),this.#Ue();return}this.#p?.postMessage({type:"enabled",enabled:!0}),this.#R==="active"&&this.#xt();return}this.#xt(),(this.#b||(this.#B?.interlaced??!0))&&this.#Ue()}}stop(){this.#he++,this.#I&&(this.#I=!1,this.#b&&this.#Qi(),this.#l.cancel(),this.#wi(),this.#yi(),this.#M=0,this.#c=null,this.#ye(!1),this.#de(),this.#p?.postMessage(this.#b?{type:"running",running:!1}:{type:"enabled",enabled:!1}))}destroy(){if(!this.#Z){this.#Z=!0,this.#Le=!1,this.stop(),this.#p?.postMessage({type:"destroy"}),this.#p?.terminate(),this.#p=null,this.#de(),this.#Vt(),this.#Yi("the deinterlacer was destroyed"),this.#E?.removeEventListener("visibilitychange",this.#Ni),this.#E=null,this.#S?.disconnect(),this.#t.removeEventListener("webglcontextlost",this.#Ns),this.#l.destroy(),this.#v?.destroy(),this.#e.removeEventListener("emptied",this.#Rs),this.#e.removeEventListener("resize",this.#xs),this.#e.removeEventListener("pause",this.#_e),this.#e.removeEventListener("ended",this.#_e),this.#e.removeEventListener("seeking",this.#_s),this.#e.removeEventListener("seeked",this.#_e),this.#e.removeEventListener("ratechange",this.#_e),this.#e.removeEventListener("waiting",this.#As),this.#mi!==null&&(this.#e.style.aspectRatio=this.#mi),this.#qs(),this.#br();for(const e of this.#a)this.#s.deleteTexture(e);this.#a=[],this.#Rt(),this.#Jt();for(const e of[...this.#Nt,...this.#ze.map(({q:t})=>t)])this.#s.deleteQuery(e);this.#Nt.length=0,this.#ze.length=0,this.#h?.destroy(),this.#h=null,this.#ct?.destroy(),this.#ct=null,this.#He!==null&&(ye(this.#He),this.#He=null),this.#s.deleteProgram(this.#u),this.#s.deleteProgram(this.#T),this.#s.getExtension("WEBGL_lose_context")?.loseContext()}}capture(){if(this.#R==="active"&&this.#i.style.visibility==="visible"&&this.#p){const s=++this.#Ss,r=new Promise((o,n)=>{this.#rt.set(s,{resolve:o,reject:n})});return this.#p.postMessage({type:"capture",id:s,width:this.#e.videoWidth,height:this.#e.videoHeight}),r}if(!this.#b&&(this.#R==="starting"||this.#R==="failed"))return createImageBitmap(this.#e);const e=this.#c;if((this.#P||this.#b)&&(!this.#I||this.#te||!e))return Promise.reject(new Error("no rendered picture is available"));if(!this.#I||this.#te||!e)return createImageBitmap(this.#e);e.kind==="texture"?this.#Qt(e.texture,e.flip,!1):this.#pe(e.flush,e.second,null,!1);const t=this.#b?this.#vt:this.#e.videoWidth,i=this.#b?this.#bt:this.#e.videoHeight;return t>0&&i>0&&(t!==this.#t.width||i!==this.#t.height)?createImageBitmap(this.#t,{resizeWidth:t,resizeHeight:i,resizeQuality:"high"}):createImageBitmap(this.#t)}addEventListener(e,t,i){super.addEventListener(e,t,i)}removeEventListener(e,t,i){super.removeEventListener(e,t,i)}#xt(){this.#P||!this.#I||this.#b||this.#l.request(this.#zs)}#Ie(){const e=[];for(let t=0;t<this.#e.buffered.length;t++)e.push({start:this.#e.buffered.start(t),end:this.#e.buffered.end(t)});return{currentTime:this.#e.currentTime,playbackRate:this.#e.playbackRate,seeking:this.#e.seeking,paused:this.#e.paused,ended:this.#e.ended,readyState:this.#e.readyState,videoWidth:this.#e.videoWidth,videoHeight:this.#e.videoHeight,buffered:e}}#Us(e,t,i){let s;try{s=i?.clone()??new VideoFrame(this.#e,{timestamp:Math.max(0,Math.round(t.mediaTime*1e6))})}catch(o){const n=o instanceof Error?o.message:String(o);this.#ve==="auto"&&!this.#_t&&!this.#it?(this.#zt(),this.#qt(e,t)):this.#Tt(n);return}const r={id:++this.#ws,frame:s,now:e,metadata:t,video:this.#Ie()};if(this.#st){this.#Xe?.frame.close(),this.#Xe=r;return}this.#ji(r)}#ji(e){const t=this.#p;if(!t||this.#R!=="active"){e.frame.close();return}this.#st=!0;const i={type:"frame",id:e.id,frame:e.frame,metadata:e.metadata,video:e.video};try{t.postMessage(i,[e.frame])}catch(s){this.#st=!1,e.frame.close();const r=s instanceof Error?s.message:String(s);this.#ve==="auto"&&!this.#_t&&!this.#it?(this.#zt(),this.#qt(e.now,e.metadata)):this.#Tt(r)}}#ks(e,t,i){this.#He==null&&(this.#He=Fe(this.#s,"20px monospace")),_e(this.#He,e,t,i,this.#C,this.#X,20)}#$t(e){if(this.#Ge==null||this.#ze.length>30)return;const t=this.#Nt.pop()??this.#s.createQuery();return this.#s.beginQuery(this.#Ge.TIME_ELAPSED_EXT,t),this.#ze.push({q:t,isField:e}),t}#Re(e){this.#Ge!=null&&(e!=null&&this.#s.endQuery(this.#Ge.TIME_ELAPSED_EXT),this.#ze=this.#ze.filter(({q:t,isField:i})=>{if(this.#s.getQueryParameter(t,this.#s.QUERY_RESULT_AVAILABLE)){const s=this.#s.getQueryParameter(t,this.#s.QUERY_RESULT);return i?(this.#at+=s,this.#Ve++):(this.#ot+=s,this.#$e++),this.#Nt.push(t),!1}return!0}))}#Q(e=!1){this.#z=P,this.#ut=P,this.#We=0,e||(this.#hi=!1),this.#Y=!1,this.#h?.reset(e)}#Ls(){const{prev:e,cur:t,next:i}=this.#Ii(!1),s=this.#a[e],r=this.#a[t],o=this.#a[i];if(!s||!r||!o)return;const n=this.#Bt?0:1;this.#h?.measureComb(s,r,n),this.#h?.detect(r,o,n)}#Bs(){const{cur:e,next:t}=this.#Ii(!1),i=this.#a[e],s=this.#a[t],r=this.#h;if(!i||!s||!r)return!1;const o=this.#Bt?0:1;try{r.measureComb(i,s,o,!0)}catch(n){return this.#Gt(`film detection failed: ${n instanceof Error?n.message:String(n)}`),!1}return!0}#Os(){const e=this.#h?.poll()??null;e!==null&&(this.#ut=e,this.#We=e.age,this.#hi=e.instant),this.#We++;const{phase:t,run:i}=this.#ut;t===0||this.#We>Math.ceil(W*Math.max(1,this.#e.playbackRate))?this.#z=P:this.#z={phase:(t-1+this.#We)%W+1,run:i}}#Xs(e){const t=[],i=this.#h?.metrics??new Float32Array(0);for(let r=0;r<E.phase;r++){const o=i[r*4]??0,n=i[r*4+1]??0,u=i[r*4+2]??0;t.push(`${o.toFixed(3)},${n.toString().padStart(4)},${u.toFixed(3)}`)}const s=this.#St.map((r,o)=>`${o===0?"-":o}:${r}`).join(" ");return`frame=${e} phase=${this.#z.phase} run=${this.#z.run} known=${this.#ut.phase}/${this.#ut.run} age=${this.#We} ${this.#Y?"film":"video"} period=${this.#L.toFixed(3)}
${t.join(" ")}
${s} dropped:${this.#Mt}`}#gt(e,t){const i=t-performance.timeOrigin;return Number.isFinite(i)&&i!==0?e+i:e}#Gs(e,t){const i=e.expectedDisplayTime;if(!Number.isFinite(i)||i<=0||!Number.isFinite(e.timeOrigin))return t;const s=this.#gt(i,e.timeOrigin),r=Ft*Math.max(this.#f,this.#L);return s<t-r||s>t+r?t:s}#zs=(e,t)=>{if(!this.#I||this.#te)return;this.#Ai();const i=this.#gt(e,t.timeOrigin),{frame:s,picture:r,...o}=t;this.#Oe=s??this.#e,this.#_=r??null;try{this.#$s(i,o,s)}finally{this.#Oe=this.#e,this.#_=null}this.#xt()};#$s(e,t,i){if(this.#R==="active"){this.#Us(e,t,i);return}this.#R!=="starting"&&this.#qt(e,t)}ingestExternalFrame(e,t,i){this.#Oe=i;try{this.#qt(e,t)}finally{this.#Oe=this.#e}}#Vs(e){try{if(We(e),e.timestamp<this.#Pt+I)throw new RangeError(`a picture at ${e.timestamp} s came after one at ${this.#Pt} s; pictures come in presentation order`);this.#b||this.#Ys()}catch(s){throw this.#me(e.data),s}if(this.#Pt=e.timestamp,this.#pi++,this.#Ws(e.displayWidth,e.displayHeight),!this.#I){this.#me(e.data);return}if(this.#R==="starting"||this.#R==="active"){this.#Ks(e);return}const t=this.#Me;t.push(e),this.#vi(this.#j.at(performance.now()));const i=Math.ceil(_t*Math.max(1,this.#e.playbackRate));for(;t.length>i;){const s=t.pop();this.#Wt(s.timestamp+s.duration)&&this.#y.queueResetted++,this.#me(s.data)}this.#Te=Math.max(this.#Te,t.length)}#Ws(e,t){if(!(e===this.#vt&&t===this.#bt)){if(this.#vt=e,this.#bt=t,this.#P||this.#Z){this.#Ze();return}this.#mi??=this.#e.style.aspectRatio,this.#e.style.aspectRatio=`${e} / ${t}`,this.#Hs(),this.#e.dispatchEvent(new Event("resize"))}}#Hs(){if(this.#It)return;this.#It=!0;const e=this.#e;Object.defineProperty(e,"videoWidth",{configurable:!0,get:()=>this.#vt}),Object.defineProperty(e,"videoHeight",{configurable:!0,get:()=>this.#bt}),Object.defineProperty(e,"getVideoPlaybackQuality",{configurable:!0,value:()=>({creationTime:performance.now(),totalVideoFrames:this.#pi,droppedVideoFrames:this.#y.missed+this.#y.queueResetted+this.#Ei,corruptedVideoFrames:0})})}#qs(){if(!this.#It)return;this.#It=!1;const e=this.#e;delete e.videoWidth,delete e.videoHeight,delete e.getVideoPlaybackQuality}#Ks(e){const t=++this.#Ms;try{this.#p.postMessage({type:"picture",id:t,picture:e},[e.data])}catch(i){throw this.#me(e.data),i}this.#Lt.set(t,e.data)}#Vt(){const e=[...this.#Lt.values()];this.#Lt.clear();for(const t of e)this.#me(t)}#Ys(){this.#b=!0,this.#de(),this.#l.cancel(),this.#wi(),this.#M=0,this.#k(),this.#Q(),this.#c=null,this.#C=0,this.#X=0,this.#j.hold(),this.#Xt(),this.#Ue()}#me(e){const t=this.decodedVideo.onrelease;if(t!==null)try{t(e)}catch(i){queueMicrotask(()=>{throw i})}}#Qi(){if(!this.#b)return;this.#p?.postMessage({type:"pictures",action:"reset"});const e=this.#Me;this.#Me=[],this.#li=!1,this.#ci=!1,this.#Pt=Number.NEGATIVE_INFINITY,this.#fi=Number.NaN,this.#Pe=!1,this.#ft=!1,this.#M=0,this.#k(),this.#pt=Number.NEGATIVE_INFINITY,this.#ne=!1,this.#Ce=null,this.#ce=Number.NaN,this.#fe=Number.NaN,this.#Q(),this.#$=0,this.#j.hold();for(const t of e)this.#me(t.data)}#vi(e){const t=this.#Me;for(;t.length>2&&t[2].timestamp<=e;){const i=t.shift();this.#es(i.timestamp+i.duration),this.#me(i.data)}}#Ji(e){return!(e<=this.#j.origin+I)}#Wt(e){return this.#kt||!this.#Ji(e)?!1:!(this.#j.origin===this.#zi&&e<=this.#Gi+I)}#Zi(e){e!==this.#Ut&&(this.#Ut=e,this.#p?.postMessage({type:"pageVisibility",hidden:e,video:this.#Ie()}),e&&(this.#kt=!0))}observePageVisibility(e){this.#Zi(e)}#es(e){this.#Wt(e)&&this.#y.missed++}#ts(e){const t=this.#e;this.#j.observe(e,t.currentTime,t.playbackRate,!t.paused&&!t.seeking&&!t.ended)&&(this.#y.resynced++,Number.isNaN(this.#ce)&&(this.#ne=!1))}observeMediaClock(e,t){this.#ts(this.#gt(e,t))}observeStart(e,t,i){this.#Ce={media:e,wall:this.#gt(t,i)}}#js(){this.#F!==null&&(this.#ss(),this.#is());const e=this.#ps(),t=this.#A+this.#f-de,i=e.setTimeout(()=>{this.#F=null,this.#is()},Math.max(0,t-performance.now()));this.#F={timers:e,handle:i}}#is(){performance.now()-this.#w<this.#f-de||this.#Qs()}#ss(){const e=this.#F;e!==null&&(this.#F=null,e.timers.clearTimeout(e.handle))}#Qs(){this.#w=performance.now(),this.#p?.postMessage({type:"clock",video:this.#Ie(),at:performance.now(),timeOrigin:performance.timeOrigin})}#Js(){const e=this.#e;this.#P||this.#ts(performance.now());const t=this.#A+this.#f;if(e.paused)this.#Ce=null,this.#ce=Number.NaN,this.#ne=!1;else if(this.#Ce!==null){const o=this.#Ce;if(t<o.wall){if(!this.#ne)return;const n=this.#Et;this.#rs(this.#xi(t,n),n,!1);return}this.#Ce=null,this.#ne=!0,this.#le=o.media+(t-o.wall)*e.playbackRate/1e3,this.#Ye=t,this.#Et=e.playbackRate,this.#ce=t,this.#fe=Number.NaN}let i=this.#j.rate;const s=this.#j.at(t);let r;if(!Number.isNaN(this.#ce)&&(Number.isNaN(s)||!(i>0))){if(t-this.#ce>gt){this.#ce=Number.NaN,this.#ne=!1;return}i=this.#Et,r=this.#Ye===t?this.#le:this.#xi(t,i)}else{if(this.#ce=Number.NaN,Number.isNaN(s))return;r=e.paused?s:this.#nr(s,i)}this.#rs(r,i,e.paused)}#rs(e,t,i){this.#kt&&!this.#Ut&&(this.#kt=!1,this.#Gi=Math.max(e,this.#e.currentTime),this.#zi=this.#j.origin);const s=this.#he;this.#Zs(e,t),this.#H(s)&&this.#hr(e,t,i)}#Zs(e,t){const i=this.#he,s=yt*this.#f*Math.max(1,t)/1e3;for(this.#vi(e);;){const u=this.#Me[0];if(u===void 0||(this.#Pe?this.#xe[this.#N].timestamp:u.timestamp-u.duration)>e+s||(u.width===this.#C&&u.height===this.#X?this.#os():_)<this.#er(u))break;if(this.#Me.shift(),this.#tr(u,e,t),!this.#H(i))return;this.#vi(e)}if(!this.#Pe||this.#ft||this.#os()<this.#hs())return;const r=this.#xe[this.#N],o=this.#Me[0];if(o!==void 0&&o.timestamp-(r.timestamp+r.duration)<=I)return;const n=V*t/1e3;(this.#li||r.timestamp<=e+n)&&(this.#bi(!0,e,t),this.#ft=!0)}#hs(){return this.#D&&!this.#Y?2:1}#er(e){const t=!e.interlaced||!this.#Le;return(this.#Pe?this.#hs():0)+(t?1:0)}#ns(e){const t=this.#n[e];if(t===void 0)return!1;const i=this.#c;if(i?.kind==="texture"&&i.texture===t.texture)return!1;for(const s of this.#qe)if(s.slot===e)return!1;return!0}#os(){let e=0;for(let t=0;t<this.#n.length;t++)this.#ns(t)&&e++;return e}#as(){for(let e=1;e<=_;e++){const t=(this.#d+e)%_;if(this.#ns(t))return t}return null}#tr(e,t,i){this.#Oi=e.duration/(e.interlaced?e.repeatFirstField?3:2:1);const s=this.#ci?this.#xe[this.#N]:null,r=s===null?0:s.timestamp+s.duration,o=e.width!==this.#C||e.height!==this.#X,n=s===null||o||e.timestamp<r-I||e.timestamp-r>Y,u=!n&&e.timestamp-r>I,c=!e.interlaced||!this.#Le,a=this.#Pe,d=this.#ft;this.#Pe=!1,this.#ft=!1,a&&!d&&(n||c)&&!o&&this.#bi(!0,t,i),n?(s!==null&&this.#y.discontinuities++,this.#M=0,this.#Q(),this.#$=0):s!==null&&!c&&s.interlaced&&s.topFieldFirst!==e.topFieldFirst&&this.#Q(!0);const m=performance.now();this.#Di(m),this.#nt=m;const l=this.#he,f=this.#$t(!1);try{if(o&&(this.#Ui(e.width,e.height),this.#n.length!==_))throw new Error("decoded pictures have no output textures");this.#ir(e)}catch(p){this.#Re(f),this.#Ot(p instanceof Error?p.message:String(p));return}finally{this.#me(e.data)}if(c)e.interlaced||(this.#$>0&&this.#$--,this.#lt=!0),this.#sr(),this.#M=0;else{if(a&&!n&&!(d&&u)){const p=this.#xe[(this.#N+g-1)%g];this.#di=p.topFieldFirst,this.#ls(),this.#H(l)&&this.#cs(u?1:0,Math.round(p.timestamp*1e3),this.#D),this.#H(l)&&this.#bi(!1,t,i)}else this.#oi=u;this.#lt=!1,this.#Pe=!0}if(!this.#H(l)){this.#Z||this.#Re(f);return}this.#Re(f),this.#Ae+=performance.now()-m,this.#ue++,this.#Ci(m)}#ir(e){this.#N=(this.#N+1)%g,this.#ct.convert(e,this.#a[this.#N]);const t=this.#xe[this.#N];t.timestamp=e.timestamp,t.duration=e.duration,t.interlaced=e.interlaced,t.topFieldFirst=e.topFieldFirst,t.repeatFirstField=e.repeatFirstField,this.#M=Math.min(this.#M+1,g),this.#ci=!0}#sr(){const e=this.#xe[this.#N],t=this.#as();if(t===null){this.#y.late++;return}this.#d=t,this.#vs(this.#a[this.#N],!0,this.#n[t].framebuffer),this.#qe.push({slot:t,time:e.timestamp,picture:e.timestamp,end:e.timestamp+e.duration,scan:Ee(e),cadence:"frame",duration:e.duration})}#bi(e,t,i){const s=this.#xe[e?this.#N:(this.#N+g-1)%g],r=this.#qe;for(;r.length>0&&r.at(-1).picture===s.timestamp;)r.pop();this.#di=s.topFieldFirst;const o=e?Number.POSITIVE_INFINITY:this.#xe[this.#N].timestamp,n=Ee(s),u=s.timestamp+s.duration;let c=!1;if(this.#Y&&!e){const a=this.#z.phase;if(a===z){this.#Mt++;return}const d=s.timestamp+(pe[a]??0)*s.duration;c=this.#Ti(d,d+s.duration*W/me,o,s.timestamp,u,!1,null,e,n,t,i,"film")!==null}else if(!this.#D)c=this.#Ti(s.timestamp,o,o,s.timestamp,u,!1,null,e,n,t,i,"frame")!==null;else{const a=s.repeatFirstField?3:2,d=s.duration/a;let m=null;for(let l=0;l<a;l++){const f=s.timestamp+l*d,p=this.#Ti(f,l+1<a?f+d:o,o,s.timestamp,u,l===1,l===2?m:null,e,n,t,i,"field");l===0&&(m=p),p!==null&&(c=!0)}}!c&&o<=t&&s.timestamp>this.#pt&&this.#es(u)}#Ti(e,t,i,s,r,o,n,u,c,a,d,m){if(!this.#rr(e,t,i,a,d))return e>this.#pt&&this.#Wt(t)&&this.#y.late++,null;if(n===null){if(n=this.#as(),n===null)return this.#y.late++,null;this.#d=n,this.#pe(u,o,this.#n[n].framebuffer)}return this.#qe.push({slot:n,time:e,picture:s,end:Number.isFinite(i)?Math.max(t,i):r,scan:c,cadence:m,duration:(Number.isFinite(t)?t:r)-e}),n}#rr(e,t,i,s,r){if(r===0)return i>s;const o=2*V*r/1e3;if(t<=s-o)return!1;const n=this.#f*r/1e3;if(t-e+2*o>=n)return!0;const u=Math.max(0,Math.ceil((e-o-s)/n));return s+u*n<t+o}#hr(e,t,i){const s=this.#qe;let r=0;for(;r<s.length&&!this.#Ji(s[r].end+this.#Ht(s[r]));)r++;r>0&&s.splice(0,r);let o=0;for(;o<s.length&&s[o].time<=this.#pt;)o++;o>0&&s.splice(0,o);const n=I*(i?1:t);let u=-1;for(let a=0;a<s.length&&s[a].time+this.#Ht(s[a])<=e+n;a++)u=a;if(u<0){const a=s[0];a&&!i&&this.#ar(a,e);return}if(!i)for(let a=0;a<u;a++){const d=s[a+1].time+this.#Ht(s[a+1]);d<=e&&this.#Wt(d)&&this.#y.late++}const c=s[u];s.splice(0,u+1),this.#ur(c)}#nr(e,t){const i=this.#A+this.#f,s=this.#us(t);if(!(t>0))return this.#ne=!1,e;if(!this.#ne)this.#ne=!0,this.#le=e,this.#Et=t,this.#fe=Number.NaN;else{this.#xi(i,t);const r=e-this.#le;Math.abs(r)>=Math.max(Tt*t,s)&&(this.#le+=Math.trunc(r/s)*s)}return this.#Ye=i,this.#or(e-this.#le),this.#le}#xi(e,t){const i=Math.max(1,Math.round((e-this.#Ye)/this.#f));return this.#le+=i*this.#us(t),this.#Ye=e,this.#Et=t,this.#le}#us(e){const t=this.#f*e/1e3,i=this.#Oi;if(!(i>0))return t;const s=t/i,r=s>=1?Math.round(s):1/Math.round(1/s);return Math.abs(s/r-1)<=xt?r*i:t}#or(e){const t=performance.now(),i=t-this.#Xi;this.#Xi=t,Number.isNaN(this.#fe)||i>ce?this.#fe=e:this.#fe+=(e-this.#fe)*i/ce}#Ht(e){return this.#De?.wants(e.cadence)?e.duration:0}#ar(e,t){const i=this.#De,s=this.#Dt;if(!i||!s||this.#P||s.cadence!==e.cadence||!i.wants(e.cadence))return;const r=this.#n[s.slot],o=this.#n[e.slot],n=this.#c;if(!r||!o||n?.kind!=="texture"||n.texture!==r.texture)return;const u=e.time-s.time;if(u<=0||u>s.duration*1.5)return;const c=(t-this.#Ht(s)-s.time)/u;if(c<=0||c>=1)return;const a=this.#s;(this.#dt?.previous!==s||this.#dt.next!==e)&&(i.pair(a,r.texture,o.texture,this.#C,this.#X),this.#dt={previous:s,next:e});const d=performance.now(),m=this.#mt?this.#Fs():null,l=m?.framebuffer??null;a.bindFramebuffer(a.FRAMEBUFFER,l);let f=!1;try{f=i.draw(a,c,this.#C,this.#X,l)}catch(p){console.warn("yadif: the interpolator failed",p),this.#De=null,this.#dt=null}f&&(m&&this.#Pi(m.texture,!1),this.#Ne+=performance.now()-d,this.#be++)}#ur(e){const t=performance.now(),i=this.#$t(!0);this.#Mi(e.slot),this.#Re(i),this.#Ne+=performance.now()-t,this.#be++,this.#Dt=e,this.#pt=e.time}#qt(e,t){const i=this.#he;if(this.#H(i)&&(this.#lr(t.mediaTime),!!this.#H(i)&&t.width>0&&t.height>0)){let s=!1;if(!this.#tt&&this.#e.seeking){const f=this.#e.buffered,p=this.#L>=ue?this.#L/1e3:$/1e3;for(let b=0;b<f.length;b++)if(t.mediaTime>=f.start(b)&&t.mediaTime<f.end(b)&&Math.abs(t.mediaTime-this.#e.currentTime)<=p){s=!0;break}}if(s&&(this.#tt=!0),(this.#C===0||this.#X===0)&&this.#Ui(t.width,t.height),!this.#H(i))return;if(this.#B&&!this.#B.interlaced){const f=performance.now();this.#Di(f),this.#nt=f,this.#$>0&&this.#$--,this.#lt=!0,this.#pr(),this.#Ae+=performance.now()-f,this.#ue++,this.#be++,this.#Ci(f);return}const r=t.mediaTime-this.#et,o=t.mozTiming,n=s||(o?o.discontinuity||r<0||r>Y:r<0||r>Y);n&&(this.#M=0,this.#L=0,this.#y.discontinuities++,this.#k(),this.#Q(),(s||!this.#lt)&&(this.#$=0)),this.#lt=!1;const u=this.#Er(t.presentedFrames,n);if(this.#M>0&&t.mediaTime===this.#et&&(!o||t.presentedFrames===this.#Zt))return;if(!n){const f=o?.periodMs??0;f>0?this.#fs(f*(this.#e.playbackRate||1)/1e3,1):this.#M>0&&r>0&&this.#fs(r,u+1)}this.#et=t.mediaTime,this.#Zt=t.presentedFrames;const c=performance.now();this.#Di(c),this.#nt=c;const a=performance.now(),d=this.#$t(!1);if(this.#bs(),this.#ls(),!this.#H(i)){this.#Z||this.#Re(d);return}if(this.#cs(u,t.presentedFrames,this.#D&&this.#Ft()),!this.#H(i)){this.#Z||this.#Re(d);return}const l=this.#Gs(t,e)+this.#f;if(this.#Y)if(this.#Ft()){const f=this.#z.phase;if(f===z)this.#Mt++,this.#q++;else{const p=this.#L*W/me,b=this.#Ri(1,e,p),x=pe[f]??0,T=b||this.#se===null?l+p:l+x*this.#L;this.#Kt("film",!1,this.#gi("film",T,p),p)}}else this.#pe(!1,!1,null);else if(this.#D&&this.#Ft()){const f=this.#L/2,b=this.#Ri(2,e,f)||this.#se===null?l+f*2:l,x=this.#gi("field",b,f);this.#Kt("field",!1,x,f),this.#Kt("field",!0,x+f,f)}else if(this.#Ft()){const f=this.#L,b=this.#Ri(1,e,f)||this.#se===null?l+f:l;this.#Kt("frame",!1,this.#gi("frame",b,f),f)||(this.#y.late++,this.#pe(!1,!1,null))}else this.#y.late+=this.#r.length,this.#k(),this.#pe(!1,!1,null);this.#Te=Math.max(this.#Te,this.#r.length),this.#Re(d),this.#Ae+=performance.now()-a,this.#ue++,this.#Ci(c)}}#ls(){if(this.#G&&!this.#Se&&!this.#h)try{this.#Hi()}catch(e){this.#Gt(`film detector unavailable: ${e instanceof Error?e.message:String(e)}`)}}#cs(e,t,i){if(this.#wt=!1,this.#we=!1,this.#ni=!1,this.#G&&!this.#Se){let s=!1;if(this.#M===g&&e===0)try{this.#Os(),this.#Ls(),s=!0}catch(r){this.#Gt(`film detection failed: ${r instanceof Error?r.message:String(r)}`)}else this.#Q(!0);this.#St[this.#z.phase]=(this.#St[this.#z.phase]??0)+1,this.#Y=this.#z.phase!==0&&this.#z.run>=q,this.#Y?this.#$=Rt:this.#$>0&&this.#$--,this.#wt=s&&!this.#oi,this.#we=this.#wt&&this.#$>0,this.#ni=this.#we&&!this.#Y&&i&&this.#Bs(),this.#we&&!this.#Y&&this.#y.matched++,this.#ae&&(this.#Li=this.#Xs(t))}this.#oi=e>0}#H(e){return!this.#Z&&this.#I&&e===this.#he}#lr(e){const t=this.#he;let i;for(let o=this.#Be.length-1;o>=0;o--){const n=this.#Be[o];if(n.start<=e+1e-6){i=n;break}}if(i?.codedSize&&(i.codedSize.width!==this.#C||i.codedSize.height!==this.#X)&&this.#Ui(i.codedSize.width,i.codedSize.height),!this.#H(t))return;const s=i?.scan;if(!s||this.#B?.interlaced===s.interlaced&&this.#B.topFieldFirst===s.topFieldFirst)return;const r=this.#B?.interlaced;this.#B=s,this.#M=0,this.#k(),this.#Qe()&&(r!==s.interlaced&&(this.#L=0),s.interlaced&&(this.#P||this.#R==="main")?this.#Ue():this.#yi(),this.#Q())}#Ft(){return(this.#D||this.#G)&&this.#L>0&&this.#n.length===_}#fs(e,t){const s=e*1e3/(this.#e.playbackRate||1)/t;s<ue||s>$||(this.#L=this.#L>0&&s>this.#L*mt?this.#L+(s-this.#L)*ct:s)}#Kt(e,t,i,s){const r=this.#ds();if(r===null)return!1;const o=this.#n[r];if(!o)return!1;for(this.#d=r;this.#r.length>0&&this.#r[0]?.slot===r;)this.#r.shift(),this.#y.late++;this.#pe(!1,t,o.framebuffer);const n={slot:r,at:i,duration:s,cadence:e,phase:e==="film"?this.#z.phase:e==="field"?t?2:1:0,droppedBefore:this.#q};return this.#q=0,this.#r.push(n),this.#se=n,!0}#gi(e,t,i){if(!(i>0))return t;const s=this.#se,r=this.#f;if(s!==null&&s.cadence===e){const o=s.at+s.duration,n=o-t,u=Math.round(n/r),c=u-this.#K,a=n-u*r;if(u>=0&&c<=1){const d=o+Math.max(-le,Math.min(le,-a*pt));if(c===0)return this.#O=0,d;if(this.#O===0||c>0!=this.#W>0?(this.#O=0,this.#W=c):Math.abs(c)<Math.abs(this.#W)&&(this.#W=c),++this.#O<Et)return d;const m=this.#W;return this.#O=0,m<0?(this.#K+=m,d):(this.#y.resynced++,this.#Fi(d-m*r))}if(u<0&&this.#K-u<=1)return this.#K-=u,this.#O=0,this.#y.resynced++,this.#Fi(t)}return this.#K=0,this.#O=0,s!==null&&this.#y.resynced++,this.#Fi(t)}#Fi(e){for(let t=this.#r.at(-1);t&&t.at>=e;)this.#r.pop(),this.#y.late++,t=this.#r.at(-1);return e}#Ri(e,t,i){const s=this.#r.at(-1),r=(j+1)*Math.max(this.#f,i);if(s&&s.at-t>r)return this.#k(),this.#y.queueResetted++,!0;const o=Math.max(0,this.#r.length+e-j);let n=0,u=0;for(;u<o;){const c=this.#r.shift();if(!c)break;n+=c.duration,u++}for(const c of this.#r)c.at-=n;return this.#y.late+=u,!1}#ds(){const e=this.#c?.kind==="texture"?this.#c.texture:null,t=new Set(this.#r.map(({slot:s})=>s));for(let s=1;s<=_;s++){const r=(this.#d+s)%_,o=this.#n[r];if(o&&o.texture!==e&&!t.has(r))return r}const i=this.#r[0];if(i){const s=this.#n[i.slot];if(s&&s.texture!==e)return i.slot}return null}#Ue(){this.#m===null&&(!this.#I||this.#te||(this.#V=0,this.#m=this.#Je(this.#_i)))}#yi(){this.#Yt(this.#m),this.#m=null,this.#ss(),this.#k()}#_i=e=>{if(this.#m=null,!this.#I||this.#te)return;this.#cr(e);const t=this.#he;if(this.#b)this.#P||this.#R==="main"?this.#Js():this.#js();else{if(this.#l.flush(e),!this.#H(t))return;this.#R==="main"&&this.#dr(this.#A,e)}this.#H(t)&&(this.#m=this.#Je(this.#_i))};#cr(e){const t=e-this.#V;this.#V=e;const i=Math.max(1,Math.round(t/this.#f)),s=this.#A+i*this.#f,r=e-s;if(this.#A===0||t<=0||t>$||Math.abs(r)>this.#f/4){t>=1&&t<=$&&(this.#f=t),this.#A=e;return}this.#f+=r/i*vt,this.#A=s+r*bt}#ms(){const e=this.#P;if(e)return{frames:e,origin:performance.timeOrigin};const t=this.#Es();return t===null?{frames:wt,origin:performance.timeOrigin}:{frames:t,origin:t.performance.timeOrigin}}#Je(e){const{frames:t,origin:i}=this.#ms(),s=t.requestAnimationFrame(r=>e(this.#gt(r,i)));return{frames:t,handle:s}}#ps(){return this.#Es()??St}#Es(){return this.#i.ownerDocument?.defaultView??this.#e.ownerDocument?.defaultView??null}#Yt(e){e?.frames.cancelAnimationFrame(e.handle)}#Ai(){if(this.#P)return;this.#fr(),this.#E&&this.#Zi(this.#E.hidden);const{frames:e}=this.#ms(),t=this.#m!==null&&this.#m.frames!==e,i=this.#g!==null&&this.#g.frames!==e;!t&&!i||(this.#V=0,t&&(this.#Yt(this.#m),this.#m=this.#Je(this.#_i)),i&&(this.#Yt(this.#g),this.#g=this.#Je(this.#Si)))}#fr(){const e=this.#P?null:this.#i.ownerDocument??null;e!==this.#E&&(this.#E?.removeEventListener("visibilitychange",this.#Ni),this.#E=e,e?.addEventListener("visibilitychange",this.#Ni),this.#S?.disconnect(),e&&this.#S?.observe(e,{childList:!0,subtree:!0}))}#Ni=()=>{this.#Z||this.#Ai()};#jt(){this.#P||this.#b||this.#g!==null||!this.#l.captureDriven||!this.#I||this.#te||(this.#g=this.#Je(this.#Si))}#wi(){this.#Yt(this.#g),this.#g=null}#Si=e=>{if(this.#g=null,!this.#I||this.#te)return;const t=this.#he;this.#l.flush(e),this.#H(t)&&this.#l.captureDriven&&(this.#g=this.#Je(this.#Si))};#dr(e,t){const i=this.#f/2,s=u=>{const c=u.at-e;return c<=i-V?!0:c>i+V?!1:this.#oe>0};for(;this.#r[1]&&s(this.#r[1]);)this.#y.late++,this.#r.shift();const r=this.#r[0];if(!r||!s(r))return;this.#r.shift(),this.#oe=r.at-e;const o=performance.now(),n=this.#$t(!0);this.#Mi(r.slot),this.#Re(n),this.#Ne+=performance.now()-o,this.#be++,this.#ae&&this.#mr(r,t),this.#ie=t}#mr(e,t){const i=this.#ie===0?0:t-this.#ie,s=i/this.#f,r=e.cadence==="film"?`phase ${e.phase}`:e.cadence==="field"?`field ${e.phase}`:"frame",o=e.phase===0?"duplicate":`phase ${z}`,n=e.droppedBefore>0?`, ${o} dropped before it`+(e.droppedBefore>1?` (${e.droppedBefore})`:""):"";console.log(`yadif: +${i.toFixed(2)} ms (${s.toFixed(2)} refreshes) ${e.cadence} ${r}, due ${(e.at-t).toFixed(2)} ms${n}`)}#Mi(e){const t=this.#n[e];t&&this.#Qt(t.texture)}#pr(){this.#bs();const e=this.#a[this.#N];e&&this.#Qt(e,!0),this.#M=0}#ye(e){if(this.#P){this.#P.onVisibility(e);return}this.#i.style.visibility=e?"visible":"hidden"}#Qt(e,t=!1,i=!0){this.#Pi(e,t),this.#c={kind:"texture",texture:e,flip:t},this.#ye(!0),i&&this.#Ee++}#Pi(e,t){const i=this.#ge,s=this.#mt;if(i&&s){const r=this.#s;r.bindFramebuffer(r.FRAMEBUFFER,null);let o=!1;try{o=i.draw(r,e,t,this.#C,this.#X,s.width,s.height)}catch(n){console.warn("yadif: the scaler failed; showing pictures as they are",n)}if(o)return;this.#gs()}this.#vs(e,t,null)}#vs(e,t,i){const s=this.#s;s.bindFramebuffer(s.FRAMEBUFFER,i),s.useProgram(this.#T),s.activeTexture(s.TEXTURE0),s.bindTexture(s.TEXTURE_2D,e),s.uniform1i(this.#x,0),s.uniform1i(this.#U,t?1:0),s.viewport(0,0,this.#C,this.#X),s.drawArrays(s.TRIANGLES,0,3)}#Er(e,t){let i=0;return this.#ht!==0&&!t&&(i=Math.max(0,e-this.#ht-1),this.#y.missed+=i),this.#ht=e,i}#Di(e){e-this.#nt<=ae||(this.#At=e,this.#ue=0,this.#Ae=0,this.#be=0,this.#Ne=0,this.#Te=0,this.#Ee=0,this.#ot=0,this.#$e=0,this.#at=0,this.#Ve=0)}#Ci(e){const t=e-this.#At;if(t<ae)return;const i=!this.#b&&this.#Ft()&&(this.#D||this.#Y)?this.#be:this.#ue,s=this.#ue?(this.#Ae+this.#Ne)/this.#ue:0;let r;this.#Ge!=null&&(r=0,this.#$e!==0&&(r+=this.#ot/1e6/this.#$e),this.#Ve!==0&&(r+=this.#at/1e6/this.#Ve/2));const o={...this.#y,avDifference:this.#fe,dropped:this.#e.getVideoPlaybackQuality?.().droppedVideoFrames??0,fps:i*1e3/t,frameMs:s,maxQueuedFields:this.#Te,outputFps:this.#Ee*1e3/t,gpuMs:r,film:this.#Y,oneInstant:this.#hi,filmError:this.#Se};this.dispatchEvent(new CustomEvent("stats",{detail:o})),this.#ei?.(o),this.#At=e,this.#ue=0,this.#Ae=0,this.#be=0,this.#Ne=0,this.#Te=0,this.#Ee=0,this.#ot=0,this.#$e=0,this.#at=0,this.#Ve=0}#bs(){const e=this.#s;this.#N=(this.#N+1)%g;const t=this.#_,i=this.#a[this.#N];t!==null&&this.#v!==null&&i!==void 0?(this.#a[this.#N]=this.#v.adopt(t,i),this.#_=null):(e.bindTexture(e.TEXTURE_2D,i??null),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,e.RGBA,e.UNSIGNED_BYTE,this.#Oe)),this.#M=Math.min(this.#M+1,g)}#pe(e,t,i,s=!0){if(this.#M===0||this.#te)return;const r=i===null&&this.#mt?this.#Fs():null,o=r!==null;r&&(i=r.framebuffer),s&&(this.#M===g&&!e?this.#y.filtered++:this.#y.degraded++);const n=this.#s,{prev:u,cur:c,next:a}=this.#Ii(e);n.bindFramebuffer(n.FRAMEBUFFER,i),n.useProgram(this.#u);for(const[x,T]of[u,c,a].entries())n.activeTexture(n.TEXTURE0+x),n.bindTexture(n.TEXTURE_2D,this.#a[T]??null);n.uniform1i(this.#o.prev,0),n.uniform1i(this.#o.cur,1),n.uniform1i(this.#o.next,2);const d=this.#G?this.#h?.texture??null:null,m=this.#G?this.#h?.combTexture??null:null,l=d!==null&&m!==null;d!==null&&m!==null&&(n.activeTexture(n.TEXTURE0+3),n.bindTexture(n.TEXTURE_2D,d),n.uniform1i(this.#o.fieldMetrics,3),n.activeTexture(n.TEXTURE0+4),n.bindTexture(n.TEXTURE_2D,m),n.uniform1i(this.#o.comb,4));const f=this.#ni?this.#h?.combAheadTexture??null:null,p=l&&!e;n.activeTexture(n.TEXTURE0+5),n.bindTexture(n.TEXTURE_2D,f),n.uniform1i(this.#o.combAhead,5),n.activeTexture(n.TEXTURE0),n.uniform2i(this.#o.size,this.#C,this.#X);const b=this.#Bt?0:1;n.uniform1i(this.#o.parity,t?1-b:b),n.uniform1i(this.#o.tff,this.#Bt?1:0),n.uniform1i(this.#o.second,t?1:0),n.uniform1i(this.#o.spatialCheck,this.#J?1:0),n.uniform1i(this.#o.debug,this.#ae?1:0),n.uniform1i(this.#o.film,l?1:0),n.uniform1i(this.#o.match,p&&this.#we&&(!t||f!==null)?1:0),n.uniform1i(this.#o.instant,p&&this.#wt?1:0),n.uniform1i(this.#o.phase,this.#z.phase),n.viewport(0,0,this.#C,this.#X),n.drawArrays(n.TRIANGLES,0,3),this.#ae&&l&&this.#ks(this.#Li,0,90),r&&this.#Pi(r.texture,!1),(i===null||o)&&(this.#c={kind:"yadif",flush:e,second:t},this.#ye(!0),s&&this.#Ee++)}#Ii(e){const t=i=>(this.#N+g-i)%g;return this.#M===1?{prev:this.#N,cur:this.#N,next:this.#N}:e?{prev:t(1),cur:this.#N,next:this.#N}:this.#M===2?{prev:t(1),cur:t(1),next:this.#N}:{prev:t(2),cur:t(1),next:this.#N}}#Ze(){if(this.#Ai(),!this.#ee)return;const e=this.#e,t=this.#b?this.#vt:e.videoWidth,i=this.#b?this.#bt:e.videoHeight;if(t===0||i===0)return;const s=Math.min(e.offsetWidth/t,e.offsetHeight/i),r=t*s,o=i*s;this.#i.style.left=`${e.offsetLeft+(e.offsetWidth-r)/2}px`,this.#i.style.top=`${e.offsetTop+(e.offsetHeight-o)/2}px`,this.#i.style.width=`${r}px`,this.#i.style.height=`${o}px`}#Ui(e,t){const i=this.#s;this.#C=e,this.#X=t,this.#M=0,this.#$=0,this.#c=null,this.#Jt(),this.#ki(),this.#k(),this.#Ze();for(const s of this.#a)i.deleteTexture(s);this.#a=[];for(let s=0;s<g;s++){const r=i.createTexture();i.bindTexture(i.TEXTURE_2D,r),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MIN_FILTER,i.NEAREST),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_MAG_FILTER,i.NEAREST),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_WRAP_S,i.CLAMP_TO_EDGE),i.texParameteri(i.TEXTURE_2D,i.TEXTURE_WRAP_T,i.CLAMP_TO_EDGE),i.texImage2D(i.TEXTURE_2D,0,i.RGBA,e,t,0,i.RGBA,i.UNSIGNED_BYTE,null),this.#a.push(r)}this.#Rt(),(this.#D||this.#G||this.#b)&&this.#Ts(),this.#ct?.resize(e,t),this.#h?.resize(e,t),this.#Qe()}#Ts(){const e=this.#s;if(!(this.#n.length===_||this.#C===0)){this.#Rt();for(let t=0;t<_;t++){const i=e.createTexture();e.bindTexture(e.TEXTURE_2D,i),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,this.#C,this.#X,0,e.RGBA,e.UNSIGNED_BYTE,null);const s=e.createFramebuffer();e.bindFramebuffer(e.FRAMEBUFFER,s),e.framebufferTexture2D(e.FRAMEBUFFER,e.COLOR_ATTACHMENT0,e.TEXTURE_2D,i,0);const r=e.checkFramebufferStatus(e.FRAMEBUFFER)===e.FRAMEBUFFER_COMPLETE;if(e.bindFramebuffer(e.FRAMEBUFFER,null),!r){e.deleteFramebuffer(s),e.deleteTexture(i),this.#Rt();return}this.#n.push({texture:i,framebuffer:s})}this.#d=_-1}}#Rt(){const e=this.#s,t=this.#c?.kind==="texture"?this.#c.texture:null;this.#n.some(i=>i.texture===t)&&(this.#c=null),this.#Dt=null;for(const{texture:i,framebuffer:s}of this.#n)e.deleteFramebuffer(s),e.deleteTexture(i);this.#n=[],this.#k()}#vr(){if(this.#ee)return;const e=this.#e.parentElement;if(!e)return;const t=document.createElement("div");t.style.cssText="position:relative;display:inline-block;line-height:0;max-width:100%",e.insertBefore(t,this.#e),t.appendChild(this.#e),t.appendChild(this.#i),this.#ee=t,this.#re?.observe(this.#e);try{this.#Ct?.observe(this.#i,{box:"device-pixel-content-box"})}catch{this.#Ct?.observe(this.#i)}this.#Ze()}#br(){if(this.#P)return;const e=this.#ee;this.#ee=null,this.#re?.disconnect(),this.#Ct?.disconnect(),this.#i.remove(),e?.parentElement&&(e.parentElement.insertBefore(this.#e,e),e.remove())}#xs=()=>this.#Ze();#ki(){const e=this.#C,t=this.#X;if(e===0||t===0||this.#te||this.#Z)return;let i=null;if(this.#ge&&!this.#P&&this.#Ke&&this.#Ke.width>0&&this.#Ke.height>0)try{i=this.#ge.size(this.#s,e,t,this.#Ke.width,this.#Ke.height)}catch(n){console.warn("yadif: the scaler failed; showing pictures as they are",n),this.#ge=null}this.#mt=i;const s=i??{width:e,height:t},r=this.#t,o=r.width!==s.width||r.height!==s.height;o&&(r.width=s.width,r.height=s.height),i||this.#Jt(),(o||i)&&this.#Tr()}#Tr(){const e=this.#c;!this.#I||this.#te||!e||(e.kind==="texture"?this.#Qt(e.texture,e.flip,!1):this.#pe(e.flush,e.second,null,!1))}#gs(){this.#ge=null,this.#mt=null,this.#Jt(),this.#t.width=this.#C,this.#t.height=this.#X}#Fs(){if(this.#Fe)return this.#Fe;const e=this.#s,t=e.createTexture();e.bindTexture(e.TEXTURE_2D,t),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,this.#C,this.#X,0,e.RGBA,e.UNSIGNED_BYTE,null);const i=e.createFramebuffer();e.bindFramebuffer(e.FRAMEBUFFER,i),e.framebufferTexture2D(e.FRAMEBUFFER,e.COLOR_ATTACHMENT0,e.TEXTURE_2D,t,0);const s=e.checkFramebufferStatus(e.FRAMEBUFFER)===e.FRAMEBUFFER_COMPLETE;return e.bindFramebuffer(e.FRAMEBUFFER,null),s?(this.#Fe={texture:t,framebuffer:i},this.#Fe):(e.deleteFramebuffer(i),e.deleteTexture(t),this.#gs(),null)}#Jt(){this.#Fe&&(this.#s.deleteFramebuffer(this.#Fe.framebuffer),this.#s.deleteTexture(this.#Fe.texture),this.#Fe=null)}#ke(e){return!this.#p||this.#R==="main"?!1:(this.#p.postMessage({type:"event",name:e,video:this.#Ie()}),!0)}#Rs=()=>{if(this.#b){this.#j.hold(),this.#ke("emptied");return}if(this.#ke("emptied")){this.#de(),this.#ye(!1);return}this.#M=0,this.#et=0,this.#Zt=0,this.#k(),this.#Q(),this.#$=0,this.#L=0,this.#ys(),this.#c=null,this.#ye(!1)};#ys(){this.#y={filtered:0,missed:0,degraded:0,discontinuities:0,resynced:0,late:0,queueResetted:0,matched:0},this.#pi=0,this.#Ei=0,this.#St.fill(0),this.#Mt=0,this.#ht=0,this.#At=0,this.#nt=0,this.#ue=0,this.#Ae=0,this.#be=0,this.#Ne=0,this.#Te=0,this.#Ee=0,this.#k(),this.#ot=0,this.#$e=0,this.#at=0,this.#Ve=0}#_s=()=>{if(this.#b){this.#j.hold(),this.#ke("seeking");return}if(this.#ke("seeking")){this.#de();return}this.#tt=!1};#_e=e=>{if(this.#b){this.#j.hold(),e.type==="ratechange"&&this.#ne&&this.#e.playbackRate>0&&(this.#ce=this.#Ye),this.#ke(e.type);return}if(e.type==="ratechange"&&(this.#l.captureDriven?this.#jt():this.#wi()),(e.type==="pause"||e.type==="ended"||e.type==="seeked"||e.type==="ratechange")&&this.#ke(e.type)){this.#de();return}if(e.type==="seeked"){const i=this.#tt;if(this.#tt=!1,i)return;this.#M=0,this.#k(),this.#Q(),this.#c=null,this.#ye(!1);return}const t=e.type==="ratechange";if(t&&(this.#L=0,this.#et=this.#e.currentTime),this.#k(),this.#I&&this.#M>0){const i=this.#ds(),s=i===null?void 0:this.#n[i];i!==null&&s?(this.#d=i,this.#pe(!0,!1,s.framebuffer),this.#Mi(i)):this.#pe(!0,!1,null)}t&&(this.#M=0,this.#ht=0,this.#Q())};#As=()=>{this.#b&&(this.#j.hold(),this.#ke("waiting"))};#Ns=e=>{e.preventDefault(),!(!this.#P&&this.#R==="active")&&(this.#te=!0,this.#Ot("the deinterlacer WebGL context was lost"))}}function Pt(h,e,t,i,s,r,o){return new Mt(h,t,{canvas:e,onFailure:i,onVisibility:s,requestAnimationFrame:r,cancelAnimationFrame:o})}function ve(h,e){const t=h.createProgram(),i=be(h,h.VERTEX_SHADER,dt),s=be(h,h.FRAGMENT_SHADER,e);if(h.attachShader(t,i),h.attachShader(t,s),h.linkProgram(t),h.deleteShader(i),h.deleteShader(s),!h.getProgramParameter(t,h.LINK_STATUS)){const r=h.getProgramInfoLog(t);throw h.deleteProgram(t),new Error(`the deinterlacer failed to link: ${r??"no reason given"}`)}return t}function be(h,e,t){const i=h.createShader(e);if(!i)throw new Error("the deinterlacer could not create a shader");if(h.shaderSource(i,t),h.compileShader(i),!h.getShaderParameter(i,h.COMPILE_STATUS)){const s=h.getShaderInfoLog(i);throw h.deleteShader(i),new Error(`the deinterlacer failed to compile: ${s??"no reason given"}`)}return i}const k=self;class Dt extends EventTarget{currentTime=0;playbackRate=1;seeking=!1;paused=!0;ended=!1;readyState=0;videoWidth=0;videoHeight=0;parentElement=null;offsetWidth=0;offsetHeight=0;offsetLeft=0;offsetTop=0;#t=[];update(e){this.currentTime=e.currentTime,this.playbackRate=e.playbackRate,this.seeking=e.seeking,this.paused=e.paused,this.ended=e.ended,this.readyState=e.readyState,this.videoWidth=e.videoWidth,this.videoHeight=e.videoHeight,this.#t=e.buffered}get buffered(){return{length:this.#t.length,start:e=>{const t=this.#t[e];if(!t)throw new DOMException("Invalid range index","IndexSizeError");return t.start},end:e=>{const t=this.#t[e];if(!t)throw new DOMException("Invalid range index","IndexSizeError");return t.end}}}getVideoPlaybackQuality(){return{creationTime:performance.now(),droppedVideoFrames:0,totalVideoFrames:0,corruptedVideoFrames:0}}requestVideoFrameCallback(){return 0}cancelVideoFrameCallback(){}}let R=null,v=null,H=!1;const Q=new Map;function Ct(h){return k.requestAnimationFrame(h)}function It(h){k.cancelAnimationFrame(h)}function A(h,e=[]){k.postMessage(h,e)}function Ut(h,e,t){h.doubleRate=e.doubleRate,h.spatialCheck=e.spatialCheck,(h.film!==e.film||t==="film")&&(h.film=e.film),h.debug=e.debug}k.onmessage=h=>{const e=h.data;try{if(e.type==="initialize"){if(typeof k.requestAnimationFrame!="function")throw new Error("requestAnimationFrame is unavailable in this Worker");R=new Dt,R.update(e.video),v=Pt(R,e.canvas,e.options,r=>{H||A({type:"failed",message:r})},r=>A({type:"visibility",visible:r}),Ct,It);const t=v;t.decodedVideo.onrelease=r=>{const o=Q.get(r);Q.delete(r),o!==void 0&&!H&&A({type:"release",id:o,data:r},[r])};let i=0;v.addEventListener("failure",()=>i++),v.addEventListener("stats",r=>{const{dropped:o,...n}=r.detail;A({type:"stats",stats:n,filmFailure:i})}),v.scan=e.scan,v.videoTimeline=e.videoTimeline,v.enabled=e.enabled;let s=!1;t.decodedVideo.onrendererstatus=()=>{s||t.decodedVideo.rendererStatus().state!=="prepared"||(s=!0,H||A({type:"ready"}))};return}if(!R||!v)return;switch(e.type){case"frame":R.update(e.video);try{v.ingestExternalFrame(performance.now(),e.metadata,e.frame)}finally{e.frame.close(),A({type:"consumed",id:e.id})}break;case"settings":Ut(v,e.options,e.retryFilm);break;case"scan":v.scan=e.scan;break;case"timeline":v.videoTimeline=e.videoTimeline;break;case"enabled":v.enabled=e.enabled;break;case"running":e.running?v.start():v.stop();break;case"picture":Q.set(e.picture.data,e.id),v.decodedVideo.push(e.picture);break;case"pictures":e.action==="reset"?v.decodedVideo.reset():v.decodedVideo.finish();break;case"start":R.update(e.video),v.observeStart(e.media,e.at,e.timeOrigin);break;case"clock":R.update(e.video),v.observeMediaClock(e.at,e.timeOrigin);break;case"pageVisibility":R.update(e.video),v.observePageVisibility(e.hidden);break;case"event":R.update(e.video),R.dispatchEvent(new Event(e.name));break;case"capture":R.videoWidth=e.width,R.videoHeight=e.height,v.capture().then(t=>A({type:"capture",id:e.id,image:t},[t])).catch(()=>A({type:"capture",id:e.id,image:null}));break;case"destroy":H=!0,v.destroy(),v=null,R=null,k.close();break}}catch(t){const i=t instanceof Error?t.message:String(t);A({type:"failed",message:i})}}})();
