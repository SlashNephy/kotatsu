# アイコンの原本と候補

アプリのアイコンを決めたときの原本と、採用しなかった候補を置いています。アプリが配信するアイコンは `frontend/public/icons/` にあり、ここのファイルは配信しません。

## 採用版

| ファイル | 内容 |
| --- | --- |
| `main-v6.svg` | 夕焼け色 (#f3b97a) の背景に、頭にみかんを載せた三毛猫の顔を描いた絵です。インストール用アイコンのすべてのサイズと、ヘッダーのロゴで使います。`frontend/public/icons/icon.svg` と同じものです。 |
| `favicon-mikan.svg` | ブラウザのタブに出す favicon です。16px では猫の顔が潰れて見分けにくいので、`main-v6.svg` と同じ背景にみかんだけを載せています。背景に埋もれないよう、みかんを濃い橙にして右下に影を付けています (#759)。`frontend/public/icons/favicon.svg` と同じものです。 |
| `main-v6-maskable.svg` | マスカブル版です。縮尺は `main-v6.svg` のままで、背景を全面に敷き、顔とみかんを半径 40% のセーフゾーンに収めています。`frontend/public/icons/icon-maskable-512.png` の元です。 |

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
