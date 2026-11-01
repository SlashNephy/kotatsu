// 静的なデモ (#832、#1050) の映像の場面を、ASS (libass で描く字幕の形式) で書き、標準出力に出す。
// scripts/build-demo-stream.sh が、ffmpeg の ass フィルターで、空のグラデーションの上に描く。
// 絵は、すべてこのファイルの図形と文字で描く。写真や他人の素材は使わない。
// 局と地名は、デモの局 (src/demo/epg.ts) に合わせた架空のもの。文字は、字幕を描くフォント (vendor/caption-font、Unlicense) で書く。
//
// 座標は 1920x1080 で書き、ffmpeg で 1440x1080 (放送と同じ、横に縮めた 16:9) にする。
// ASS の色は &HBBGGRR& の順で、透明度は &H00& (不透明) から &HFF& (透明) で書く。

const duration = 20;
const font = 'Rounded M+ 1m WadaLab mix';

// color は、#RRGGBB を ASS の &HBBGGRR& にする。
function color(hex) {
  const [r, g, b] = [hex.slice(1, 3), hex.slice(3, 5), hex.slice(5, 7)];

  return `&H${b}${g}${r}&`.toUpperCase();
}

// time は、秒を ASS の H:MM:SS.cc にする。
function time(seconds) {
  const cs = Math.round(seconds * 100);
  const s = Math.floor(cs / 100);

  return `0:${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}.${String(cs % 100).padStart(2, '0')}`;
}

// circle は、中心 (cx, cy)、半径 r の円を、ベジェ曲線の描画命令で返す。
function circle(cx, cy, r) {
  const k = r * 0.5523;
  const p = (x, y) => `${Math.round(cx + x)} ${Math.round(cy + y)}`;

  return [
    `m ${p(0, -r)}`,
    `b ${p(k, -r)} ${p(r, -k)} ${p(r, 0)}`,
    `b ${p(r, k)} ${p(k, r)} ${p(0, r)}`,
    `b ${p(-k, r)} ${p(-r, k)} ${p(-r, 0)}`,
    `b ${p(-r, -k)} ${p(-k, -r)} ${p(0, -r)}`,
  ].join(' ');
}

// rect は、角を丸めない四角を返す。
function rect(x, y, w, h) {
  return `m ${x} ${y} l ${x + w} ${y} ${x + w} ${y + h} ${x} ${y + h}`;
}

// roundRect は、角を半径 r で丸めた四角を返す。
function roundRect(x, y, w, h, r) {
  const k = r * 0.4477;

  return [
    `m ${x + r} ${y}`,
    `l ${x + w - r} ${y}`,
    `b ${x + w - k} ${y} ${x + w} ${y + k} ${x + w} ${y + r}`,
    `l ${x + w} ${y + h - r}`,
    `b ${x + w} ${y + h - k} ${x + w - k} ${y + h} ${x + w - r} ${y + h}`,
    `l ${x + r} ${y + h}`,
    `b ${x + k} ${y + h} ${x} ${y + h - k} ${x} ${y + h - r}`,
    `l ${x} ${y + r}`,
    `b ${x} ${y + k} ${x + k} ${y} ${x + r} ${y}`,
  ].join(' ');
}

// polygon は、点を結んだ多角形を返す。
function polygon(points) {
  const [first, ...rest] = points;

  return `m ${first[0]} ${first[1]} l ${rest.map(([x, y]) => `${x} ${y}`).join(' ')}`;
}

const events = [];

// placed は、\move で動かさないものを、(x, y) に置く。\pos と \move は、同じ行に書けない。
function placed(tags, x, y) {
  return tags.includes('\\move') ? '' : `\\pos(${x},${y})`;
}

// shape は、図形を 1 つ置く。tags は、ASS の上書きタグ (\move や \fad など)。
function shape({ layer, path, fill, alpha = '&H00&', tags = '', from = 0, to = duration }) {
  events.push(
    `Dialogue: ${layer},${time(from)},${time(to)},Shape,,0,0,0,,{\\an7${placed(tags, 0, 0)}\\bord0\\shad0\\1c${fill}\\1a${alpha}${tags}\\p1}${path}{\\p0}`,
  );
}

// text は、文字を置く。an は、(x, y) を文字のどこに合わせるか (テンキーの並び)。
function text({
  layer,
  x,
  y,
  size,
  body,
  fill,
  an = 7,
  border = 0,
  borderColor = '&H000000&',
  tags = '',
  from = 0,
  to = duration,
}) {
  events.push(
    `Dialogue: ${layer},${time(from)},${time(to)},Text,,0,0,0,,{\\an${an}${placed(tags, x, y)}\\fs${size}\\1c${fill}\\bord${border}\\3c${borderColor}\\shad0${tags}}${body}`,
  );
}

// 背景の空は、ffmpeg の gradients で描く。ここでは、その上に重ねるものを描く。

// 太陽。ぼかした大きな円を光に見立てて、下に敷く。
shape({
  layer: 0,
  path: circle(1480, 250, 150),
  fill: color('#fff6d0'),
  alpha: '&HB0&',
  tags: '\\blur30',
});
shape({ layer: 0, path: circle(1480, 250, 78), fill: color('#fff3b8'), tags: '\\blur3' });

// 雲。円を重ねて形を作り、20 秒かけてゆっくり右へ流す。
const clouds = [
  { x: 260, y: 210, s: 1.0, dx: 160 },
  { x: 900, y: 140, s: 0.7, dx: 110 },
  { x: 1180, y: 330, s: 0.85, dx: 140 },
];
for (const cloud of clouds) {
  const s = cloud.s;
  const path = [
    circle(0, 0, 70 * s),
    circle(80 * s, -30 * s, 85 * s),
    circle(170 * s, 0, 70 * s),
    circle(90 * s, 30 * s, 60 * s),
  ].join(' ');
  shape({
    layer: 1,
    path,
    fill: color('#ffffff'),
    tags: `\\blur4\\move(${cloud.x},${cloud.y},${cloud.x + cloud.dx},${cloud.y},0,${duration * 1000})`,
  });
}

// 遠くの山並み。
shape({
  layer: 2,
  path: polygon([
    [0, 700],
    [180, 560],
    [330, 620],
    [520, 470],
    [700, 590],
    [860, 540],
    [1040, 640],
    [1260, 500],
    [1440, 600],
    [1620, 530],
    [1920, 650],
    [1920, 1080],
    [0, 1080],
  ]),
  fill: color('#8ea7c6'),
});
// 山頂の雪。
shape({
  layer: 3,
  path: polygon([
    [520, 470],
    [575, 507],
    [545, 500],
    [520, 520],
    [492, 498],
    [468, 512],
  ]),
  fill: color('#f4f7fb'),
});
shape({
  layer: 3,
  path: polygon([
    [1260, 500],
    [1320, 534],
    [1290, 530],
    [1262, 548],
    [1232, 528],
    [1206, 540],
  ]),
  fill: color('#f4f7fb'),
});

// 手前の丘。
shape({
  layer: 4,
  path: `m 0 760 b 300 690 520 700 780 750 b 1040 800 1300 720 1560 700 b 1720 690 1840 710 1920 730 l 1920 1080 0 1080`,
  fill: color('#5d9b54'),
});

// 電波塔。丘の右に立てる。
const tower = { x: 1640, base: 706, top: 450 };
shape({
  layer: 5,
  path: polygon([
    [tower.x - 22, tower.base],
    [tower.x - 4, tower.top],
    [tower.x + 4, tower.top],
    [tower.x + 22, tower.base],
  ]),
  fill: color('#e03131'),
});
for (let i = 1; i < 5; i++) {
  const y = tower.top + ((tower.base - tower.top) * i) / 5;
  shape({ layer: 6, path: rect(tower.x - 24, Math.round(y), 48, 14), fill: color('#f8f9fa') });
}
shape({ layer: 6, path: rect(tower.x - 2, tower.top - 50, 4, 50), fill: color('#adb5bd') });

// 手前の野原と、ふもとへ続く道。
shape({
  layer: 7,
  path: `m 0 860 b 480 820 1100 840 1920 820 l 1920 1080 0 1080`,
  fill: color('#7cb85e'),
});
shape({
  layer: 8,
  path: `m 860 1080 b 900 980 980 900 1060 845 l 1100 843 b 1060 900 1040 980 1080 1080`,
  fill: color('#dcc89a'),
});

// ふもとの家並み。
const houses = [
  { x: 420, y: 790, w: 90, h: 60, roof: '#c92a2a' },
  { x: 540, y: 800, w: 70, h: 50, roof: '#1971c2' },
  { x: 640, y: 785, w: 100, h: 66, roof: '#e8590c' },
  { x: 1240, y: 770, w: 80, h: 56, roof: '#2f9e44' },
  { x: 1350, y: 780, w: 96, h: 60, roof: '#c92a2a' },
];
for (const house of houses) {
  shape({ layer: 9, path: rect(house.x, house.y, house.w, house.h), fill: color('#fdf6e3') });
  shape({
    layer: 9,
    path: polygon([
      [house.x - 10, house.y],
      [house.x + house.w / 2, house.y - house.h * 0.6],
      [house.x + house.w + 10, house.y],
    ]),
    fill: color(house.roof),
  });
  shape({
    layer: 10,
    path: rect(house.x + house.w / 2 - 12, house.y + house.h - 34, 24, 34),
    fill: color('#8d6e63'),
  });
}

// 局のロゴ (右上)。放送の局のロゴのように、少し透かして重ねる。
shape({ layer: 20, path: roundRect(1636, 54, 64, 64, 12), fill: color('#e8590c'), alpha: '&H30&' });
text({
  layer: 21,
  x: 1668,
  y: 86,
  an: 5,
  size: 44,
  body: 'こ',
  fill: color('#ffffff'),
  tags: '\\b1\\alpha&H30&',
});
text({
  layer: 21,
  x: 1712,
  y: 86,
  an: 4,
  size: 34,
  body: 'こたつ\\Nテレビ',
  fill: color('#ffffff'),
  border: 2,
  borderColor: color('#333333'),
  tags: '\\b1\\alpha&H30&\\q2',
});

// 番組のコーナー名 (左上)。
shape({ layer: 20, path: roundRect(60, 56, 380, 60, 30), fill: color('#1f2a44'), alpha: '&H50&' });
shape({ layer: 21, path: circle(98, 86, 11), fill: color('#ff4d4f') });
text({
  layer: 21,
  x: 122,
  y: 86,
  an: 4,
  size: 34,
  body: 'ぬくぬくライブカメラ',
  fill: color('#ffffff'),
});

// テロップ (画面の下)。左から差し込み、入れ替える。字幕は、テロップに重ならないよう、この上に出す。
const telops = [
  { from: 1, to: 9.6, tag: 'こたつ山のふもと', main: 'けさは冷え込み　霜が降りました' },
  { from: 10.4, to: 19.4, tag: 'きょうの天気', main: '晴れ　日中は　ぽかぽか陽気に' },
];
for (const telop of telops) {
  const slide = (x, y) => `\\move(${x - 60},${y},${x},${y},0,300)\\fad(300,300)`;
  shape({
    layer: 22,
    path: roundRect(0, 0, 1040, 112, 14),
    fill: color('#ffffff'),
    tags: slide(120, 900),
    from: telop.from,
    to: telop.to,
  });
  shape({
    layer: 23,
    path: roundRect(0, 0, 400, 56, 10),
    fill: color('#e8590c'),
    tags: slide(120, 856),
    from: telop.from,
    to: telop.to,
  });
  text({
    layer: 24,
    x: 0,
    y: 0,
    size: 38,
    body: telop.tag,
    fill: color('#ffffff'),
    tags: `\\b1${slide(144, 864)}`,
    from: telop.from,
    to: telop.to,
  });
  text({
    layer: 24,
    x: 0,
    y: 0,
    size: 64,
    body: telop.main,
    fill: color('#1f2a44'),
    tags: `\\b1${slide(160, 920)}`,
    from: telop.from,
    to: telop.to,
  });
}

const header = [
  '[Script Info]',
  '; 静的なデモの映像の場面。frontend/scripts/demo-stream-scene.mjs が書く。',
  'ScriptType: v4.00+',
  'PlayResX: 1920',
  'PlayResY: 1080',
  'WrapStyle: 2',
  'ScaledBorderAndShadow: yes',
  '',
  '[V4+ Styles]',
  'Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding',
  `Style: Shape,${font},20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,0,0,7,0,0,0,1`,
  `Style: Text,${font},40,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,0,0,7,0,0,0,1`,
  '',
  '[Events]',
  'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text',
];

process.stdout.write(`${[...header, ...events].join('\n')}\n`);
