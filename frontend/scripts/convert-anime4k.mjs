// Anime4K の mpv 向けのシェーダー (GLSL の hook) を、WebGL2 のフラグメントシェーダーの並びに直し、
// src/yunomi/anime4kShaders.ts に書く (#1653、設計書 7.2)。
// 元のファイルは bloc97/Anime4K の glsl/Upscale/Anime4K_Upscale_CNN_x2_M.glsl (MIT)。リポジトリには入れず、引数で渡す。
//
//   node scripts/convert-anime4k.mjs <Anime4K_Upscale_CNN_x2_M.glsl>
//
// 畳み込みのステップ (//!SAVE で結果を名前を付けて残すステップ) だけを修正する。最後の Depth-to-Space のステップは、拡大と元の絵への足し込みを
// まとめて src/yunomi/anime4k.ts に書く。mpv の texOff と tex は、テクスチャの端で端の画素を読む (clamp) ので、同じく端に寄せて読む。
// mpv は y を画像の下向きに数えるので、テクスチャの行の向き (uDown) を掛けて読む。
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const [source] = process.argv.slice(2);
if (source === undefined) {
  throw new Error('usage: node scripts/convert-anime4k.mjs <Anime4K_Upscale_CNN_x2_M.glsl>');
}
const text = readFileSync(source, 'utf8').replaceAll('\r\n', '\n');

// 頭のライセンスの文 (// で始まる行の並び) を、そのまま残す。
const license = text
  .split('\n')
  .filter((_, index, lines) =>
    lines.slice(0, index + 1).every((line) => line.startsWith('//') || line === ''),
  )
  .filter((line) => !line.startsWith('//!'))
  .join('\n')
  .trim();

const passes = [];
for (const block of text.split(/^(?=\/\/!DESC )/m).slice(1)) {
  const directives = Object.groupBy(
    block.split('\n').filter((line) => line.startsWith('//!')),
    (line) => line.slice(3).split(' ')[0],
  );
  const save = directives.SAVE?.[0]?.split(' ')[1];
  if (save === undefined || save === 'MAIN') {
    // Depth-to-Space のステップ (元のコマの MAIN に書き戻すステップ) は、anime4k.ts に書く。
    continue;
  }
  const inputs = (directives.BIND ?? []).map((line) => line.split(' ')[1]);
  const body = block
    .split('\n')
    .filter((line) => !line.startsWith('//!'))
    .join('\n')
    // X_texOff(vec2(dx, dy)) と X_tex(X_pos) を、端に寄せて画素を読む形にする。
    .replace(/(\w+)_texOff\(vec2\(([^()]*)\)\)/g, 'A4K_FETCH($1, ivec2($2))')
    .replace(/(\w+)_tex\(\1_pos\)/g, 'A4K_FETCH($1, ivec2(0))')
    .trim();
  if (/_tex|_pos|_pt|_size|HOOKED/.test(body.replace(/\bconv2d\w*_tf\b/g, ''))) {
    throw new Error(`unsupported reference in ${save}`);
  }
  const fragment = [
    '#version 300 es',
    'precision highp float;',
    'precision highp int;',
    ...inputs.map((name) => `uniform highp sampler2D ${name};`),
    // uDown は、画像の下へ 1 画素進むときの、テクスチャの行の向き (行を上から持てば 1、下から持てば -1)。
    // mpv のシェーダーは、y を画像の下向きに数える。重みの向きを合わせないと、縦に裏返した畳み込みになる。
    'uniform int uDown;',
    'out vec4 fragColor;',
    '#define A4K_FETCH(t, o) texelFetch(t, clamp(ivec2(gl_FragCoord.xy) + (o) * ivec2(1, uDown), ivec2(0), textureSize(t, 0) - 1), 0)',
    body,
    'void main() {',
    '  fragColor = hook();',
    '}',
    '',
  ].join('\n');
  passes.push({ output: save, inputs, fragment });
}

const out = `// このファイルは scripts/convert-anime4k.mjs が書いた。手で直さない。
// Anime4K (https://github.com/bloc97/Anime4K) の Anime4K_Upscale_CNN_x2_M.glsl の畳み込みのステップを、WebGL2 に直したもの (#1653)。
// 元のライセンスの文は、そのまま anime4kLicenseText に残す。

// anime4kLicenseText は、元のシェーダーのライセンスの文。/about の一覧に出す (付録 B)。
export const anime4kLicenseText = \`${license.replaceAll(/^\/\/ ?/gm, '')}
\`;

// Anime4KPass は、畳み込みの 1 ステップ。inputs のテクスチャ (同じ名前の sampler) を読み、output の名前で RGBA の 4 つの値を残す。
export type Anime4KPass = { output: string; inputs: readonly string[]; fragment: string };

// anime4kPasses は、畳み込みのステップを、動かす順に並べたもの。最初のステップの入力 MAIN は、拡大する元のコマ。
export const anime4kPasses: readonly Anime4KPass[] = ${JSON.stringify(passes, null, 2)};
`;
const target = fileURLToPath(new URL('../src/yunomi/anime4kShaders.ts', import.meta.url));
writeFileSync(target, out);
// リポジトリの書式に合わせる。書き直すたびに、同じ書式で書く。
execFileSync('pnpm', ['exec', 'vp', 'fmt', target], { stdio: 'inherit' });
console.log(`wrote ${passes.length} passes`);
