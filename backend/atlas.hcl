# マイグレーションの生成にだけ使う。実行時は kotatsu migrate が、Go に埋め込んだマイグレーションを適用する。
# 古いバージョンのマネージド DB でも動くよう、開発用の DB には一つ前のメジャーバージョンを使う。
env "local" {
  src = "file://db/schema.sql"
  dev = "docker+postgres://docker.io/library/postgres:17/dev?search_path=public"

  migration {
    dir = "file://db/migrations?format=goose"
  }
}
