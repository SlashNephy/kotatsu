// Package logo は、録画の映像の隅の局ロゴが出ているかどうかで、CM の区間を見分けるための部品 (#1505)。
//
// ロゴは、色や濃さではなく、縁の向き (線の向き) で学習して当てる。白いロゴが暗い背景にも明るい背景にも乗るので、
// 勾配の角を 2 倍にして符号の反転を消し、コマをまたいで向きが揃う画素をロゴの縁とみなす。
// このパッケージは、画素の配列を受け取って計算するだけで、FFmpeg や DB には触れない。
package logo

import "time"

// Params は、ロゴの学習と判定のパラメーター。#1505 の「決めるパラメーター」の表に当たる。
// 既定値 (DefaultParams) は仮の値で、実放送の録画で測って決め直す。
type Params struct {
	// EdgeStrength は、勾配を縁として数える強さの下限 (Sobel の 3×3 の、画素値 0〜255 の単位)。
	EdgeStrength float64

	// LearnFrames は、学習で取り出すキーフレームの数の上限。
	LearnFrames int
	// LearnMinInterval は、学習でキーフレームを取り出す間隔の下限。
	LearnMinInterval time.Duration
	// BandRatio は、学習で切り出す上下の帯の高さの、コマの高さに対する割合。
	BandRatio float64
	// CornerRatio は、隅の範囲の幅の、コマの幅に対する割合。
	CornerRatio float64
	// StrongQuantile は、隅の中で「強い縁」とする揃い方の分位 (0〜1。0.99 なら上位 1%)。
	StrongQuantile float64
	// MinCornerCoherence は、強い縁の閾値がこれ未満の隅を諦める揃い方の下限。型の点の下限にも使う。
	MinCornerCoherence float64
	// CornerMedianRatio は、強い縁の閾値が、隅の揃い方の中央値の何倍以上なら採るか。
	CornerMedianRatio float64
	// DilateRadius は、強い縁の印を太らせる正方形の半径 (画素)。
	DilateRadius int
	// TooLargeRatio は、ロゴより大きいとして比較から外す塊の幅と高さの、コマに対する割合。
	TooLargeRatio float64
	// CoreMinRatio は、核にできる塊の幅と高さの下限の、コマに対する割合。
	CoreMinRatio float64
	// BundleDistance は、核と同じロゴとして括る塊の、核からの距離 (画素)。
	BundleDistance int
	// BundleStrengthRatio は、括る塊の強い縁の数の、核に対する割合の下限。
	BundleStrengthRatio float64
	// LogoShareMin は、括った塊の強い縁が、張り合う量に占める割合の下限。
	LogoShareMin float64
	// TrimRatio は、枠を締めるとき、各端から切り落とす強い縁の割合。
	TrimRatio float64
	// MarginMin と MarginRatio は、締めた枠の四方に足す余白 (max(MarginMin, 枠の大きさ × MarginRatio))。
	MarginMin   int
	MarginRatio float64

	// TemplateQuantile と TemplatePointRatio は、型の点とする揃い方の閾値を決める分位と比。
	TemplateQuantile   float64
	TemplatePointRatio float64
	// Shift は、型をずらす量 (画素)。
	Shift int
	// MinTemplatePoints は、これ未満なら型を使わない点の数。
	MinTemplatePoints int
	// AngleCos は、向きが合ったとみなす元の角の余弦 c。
	AngleCos float64

	// AppearScore は、ヒステリシスの上の閾値 (出たとみなす点数)。
	AppearScore float64
	// ContinueScore は、ヒステリシスの下の閾値 (続いているとみなす点数)。AppearScore より低くする。
	ContinueScore float64
	// NearWindow は、1 段目で近いコマとする前後の時間。
	NearWindow time.Duration
	// BrightnessLevels は、1 段目で明るさを分ける段の数。
	BrightnessLevels int
	// LevelMinDuration は、段や格子を確かめるのに要るコマの量。
	LevelMinDuration time.Duration
	// LevelQuantile は、1 段目で段を判断できるとする点数の分位 (0.9 なら上位 10%)。
	LevelQuantile float64
	// FlatTexture と WhiteBrightness は、のっぺりの上書きを掛ける肌理の上限と明るさの上限。
	FlatTexture     float64
	WhiteBrightness float64
	// TextureEdges は、2 段目の肌理の段の境目 (昇順)。段の数は len(TextureEdges) + 1。
	TextureEdges []float64
	// GridMissRatio は、2 段目で格子を判断できるとする、続いているとみなす点数未満のコマの割合の上限。
	GridMissRatio float64
	// WindowHalf は、コマごとに見る前後の時間。
	WindowHalf time.Duration
	// WindowRank は、窓の値に使う順位 (1 なら最高点)。
	WindowRank int
	// WindowMinRatio は、窓の値を出すのに要る、判断できるコマの窓に対する割合。
	WindowMinRatio float64
	// SmoothFrames は、境目の戻しで使う中央値の幅 (コマ)。
	SmoothFrames int
	// PullbackMax は、境目の戻しでずらせる量の上限。
	PullbackMax time.Duration
	// FillGap は、埋める途切れの長さの上限。
	FillGap time.Duration
	// MinInterval は、捨てるロゴ区間の長さの上限。
	MinInterval time.Duration
}

// DefaultParams は、仮の既定値を返す。どれも実放送の録画で測って決め直す (#1505 の「測ること」)。
// 縁とみなす強さ、強い縁の分位、出たとみなす点数、続いているとみなす点数は、地上波の 5 局の録画 (1440×1080) で、
// 半透明のロゴの縁が落ちないこと、ロゴの縁が隅の 1〜2% しか占めないこと、点数がロゴの出ている所で 0.6〜0.8、
// CM の間で 0 付近に分かれることを見て決めた。ほかは仮の値のまま。
func DefaultParams() *Params {
	return &Params{
		EdgeStrength:        20,
		LearnFrames:         300,
		LearnMinInterval:    500 * time.Millisecond,
		BandRatio:           0.2,
		CornerRatio:         0.3,
		StrongQuantile:      0.99,
		MinCornerCoherence:  0.3,
		CornerMedianRatio:   3,
		DilateRadius:        2,
		TooLargeRatio:       0.25,
		CoreMinRatio:        0.01,
		BundleDistance:      24,
		BundleStrengthRatio: 0.2,
		LogoShareMin:        0.6,
		TrimRatio:           0.02,
		MarginMin:           4,
		MarginRatio:         0.1,
		TemplateQuantile:    0.9,
		TemplatePointRatio:  0.5,
		Shift:               3,
		MinTemplatePoints:   30,
		AngleCos:            0.9,
		AppearScore:         0.4,
		ContinueScore:       0.2,
		NearWindow:          10 * time.Second,
		BrightnessLevels:    8,
		LevelMinDuration:    10 * time.Second,
		LevelQuantile:       0.9,
		FlatTexture:         2,
		WhiteBrightness:     240,
		TextureEdges:        []float64{4, 12},
		GridMissRatio:       0.3,
		WindowHalf:          time.Second,
		WindowRank:          3,
		WindowMinRatio:      0.2,
		SmoothFrames:        15,
		PullbackMax:         1500 * time.Millisecond,
		FillGap:             2 * time.Second,
		MinInterval:         5 * time.Second,
	}
}
