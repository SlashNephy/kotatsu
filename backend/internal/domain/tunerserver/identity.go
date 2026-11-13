package tunerserver

//go:generate go tool go-enum --values --nocomments

// Kind は、チューナーサーバーの種類 (設計書 3.1)。
//
// ENUM(
// mirakurun
// mirakc
// mahiron
// )
type Kind string

// Identity は、チューナーサーバーの種類と版。システムの状態の画面に出す (#1538)。
type Identity struct {
	Kind Kind
	// Version は、チューナーサーバーが名乗る版。名乗らなければ空。
	Version string
}
