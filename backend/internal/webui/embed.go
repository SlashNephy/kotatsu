// Package webui は、ビルドした SPA を Go のバイナリに埋め込む (設計書 2.3)。
//
// SPA は frontend/ で vp build すると、このパッケージの dist/app に出力される。
// 未ビルドでも go build が通るよう、dist には空の .gitkeep だけを置いてリポジトリに残す。
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// icons は、SPA を未ビルドでも使えるよう、frontend/public/icons から写したものを埋め込む。
// 写すのは mise run //backend:generate で、CI で原本と食い違っていないかを確かめる。
//
//go:embed icons/icon.svg icons/icon-192.png
var icons embed.FS

// Icons は、Kotatsu のアイコン (icon.svg と icon-192.png) を返す。
func Icons() fs.FS {
	sub, err := fs.Sub(icons, "icons")
	if err != nil {
		// "icons" は正しいパスなので、fs.Sub は失敗しない。
		panic(err)
	}

	return sub
}

// App は、ビルドした SPA のファイルを返す。
func App() fs.FS {
	app, err := fs.Sub(dist, "dist/app")
	if err != nil {
		// "dist/app" は正しいパスなので、fs.Sub は失敗しない。
		panic(err)
	}

	return app
}
