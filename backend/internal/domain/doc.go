// Package domain は、Kotatsu の entity の型と、DB や外部サービスに触れない規則を表現する。
//
// # レイヤの依存関係
//
// backend は次の依存グラフを守る。矢印は import してよい向きを表す。
//
//	cli --> web --> usecase --> query --> infra/database
//	web --> query, domain, lib, infra (infra/database を除く)
//	usecase --> domain, infra, lib
//	query --> domain, lib
//
// # domain の制約
//
//   - lib のほかに、internal のパッケージを import してはならない。
//   - DB や外部サービス、ファイルに触れてはならない。
package domain
