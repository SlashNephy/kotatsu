# アイコンの原本と候補

アプリのアイコンを決めたときの原本と、候補を置いています。アプリが配信するアイコンは `frontend/public/icons/` にあり、ここのファイルは原本として残し、配信しません。配信するのは、採用版と柄の組を `frontend/public/icons/` に写したものです。

## 採用版

| ファイル | 内容 |
| --- | --- |
| `main-v6.svg` | 夕焼け色 (#f3b97a) の背景に、頭にみかんを載せた三毛猫の顔を描いた絵です。インストール用アイコンのすべてのサイズと、ヘッダーのロゴで使います。`frontend/public/icons/icon.svg` と同じものです。 |
| `favicon-mikan.svg` | ブラウザのタブに出す favicon です。16px では猫の顔が潰れて見分けにくいので、`main-v6.svg` と同じ背景にみかんだけを載せています。背景に埋もれないよう、みかんを濃い橙にして右下に影を付けています (#759)。`frontend/public/icons/favicon.svg` と同じものです。 |
| `main-v6-maskable.svg` | マスカブル版です。縮尺は `main-v6.svg` のままで、背景を全面に敷き、顔とみかんを半径 40% のセーフゾーンに収めています。`frontend/public/icons/icon-maskable-512.png` の元です。 |

## 柄の組

採用版の構図 (背景、みかん、顔の形) のまま、猫の柄だけを変えた絵です。設定の「アイコン」で選べます (#1665)。

| ファイル | 柄 |
| --- | --- |
| `coat-chatora.svg` | 茶トラ (猫が夕焼けの背景に埋もれるので、背景をクリーム色 #fbeedd にしています) |
| `coat-kijitora.svg` | キジトラ |
| `coat-sabatora.svg` | サバトラ |
| `coat-kuro.svg` | 黒猫 (目と口を黄色にしています) |
| `coat-shiro.svg` | 白猫 |
| `coat-hachiware.svg` | ハチワレ (頭の上側が黒く、額の真ん中で白が八の字に割れ込む柄) |

## 候補

| ファイル | 内容 |
| --- | --- |
| `p1-mike-night-{app,favicon}.svg` | 三毛猫と夜の部屋の組です。 |
| `p2-mike-sunset-{app,favicon}.svg` | 三毛猫と夕焼けの組です。 |
| `p3-chatora-night-{app,favicon}.svg` | 茶トラと夜の部屋の組です。 |
| `p4-chatora-sunset-{app,favicon}.svg` | 茶トラと夕焼けの組です。 |
| `variant-no-tenban-sleep.svg` | 天板のない、顔だけの版の寝顔です。 |
| `variant-no-tenban-open.svg` | 天板のない、顔だけの版で目を開けたものです。採用版の元になりました。 |
| `candidate-maskable-88pct.svg` | 88% に縮めて余白をとったマスカブル版です。 |

P1〜P4 は、ホーム画面用の `app` と favicon 用の `favicon` の 2 枚で 1 組です。`app` は布団から顔を出す猫の情景を、`favicon` は猫になったこたつを描いています。

## 端末ごとに選べるアイコン

ブラウザのタブの favicon とヘッダーのロゴは、設定の「アイコン」で、端末ごとに採用版と柄の組から選べます (#1665)。次のファイルを、名前を変えずに `frontend/public/icons/` に写して配信しています。原本を直したら、写しも同じものにします。

| 組 | 配信するファイル |
| --- | --- |
| 採用版 (既定) | `icon.svg` (`main-v6.svg` の写し)、`favicon.svg` (`favicon-mikan.svg` の写し) |
| 柄の組 | `coat-*.svg` |

採用版の favicon は、16px でも見分けられるようみかんだけを描いています。柄の組は、柄の違いが見えるよう、ヘッダーのロゴと同じ猫の顔の絵を favicon にも使います。

ホーム画面やアプリの一覧に入れた (インストールした) アイコンは、採用版のままで、選んでも替わりません。柄の組をインストール用にするには、組ごとにマスカブル版と PNG (192、512、apple-touch-icon の 180) が要ります。
