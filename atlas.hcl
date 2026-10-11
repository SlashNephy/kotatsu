# マイグレーションの生成にだけ使う。実行時は kotatsu migrate が、Go に埋め込んだマイグレーションを適用する。
# 古いバージョンのマネージド DB でも動くよう、開発用の DB には一つ前のメジャーバージョンを使う。
env "local" {
  src = "file://db/schema.sql"
  # Docker Hub の取得の回数の上限に当たらないよう、Google のミラーの公式イメージを使う。
  dev = "docker+postgres://mirror.gcr.io/library/postgres:18/dev?search_path=public"

  migration {
    dir = "file://db/migrations?format=goose"
  }
}
