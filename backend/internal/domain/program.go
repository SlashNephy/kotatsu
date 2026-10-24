package domain

// Genre は番組のジャンル。ARIB STD-B10 の content_nibble をそのまま持つ。
type Genre struct {
	Lv1 int `json:"lv1"`
	Lv2 int `json:"lv2"`
	Un1 int `json:"un1"`
	Un2 int `json:"un2"`
}

// Audio は、番組の音声。ARIB STD-B10 の音声コンポーネント記述子をそのまま持つ。
type Audio struct {
	// ComponentType は component_type。1 はシングルモノ、2 はデュアルモノ、3 はステレオ、9 は 5.1ch など。
	ComponentType int `json:"componentType"`
	// ComponentTag は component_tag。mirakc は返さない。
	ComponentTag *int `json:"componentTag,omitempty"`
	IsMain       bool `json:"isMain"`
	SamplingRate int  `json:"samplingRate"`
	// Langs は、言語のコード (ISO 639-2)。デュアルモノでは、主音声と副音声の 2 つになる。
	Langs []string `json:"langs"`
}

// RelatedItem は、番組に関連づけられた別のサービスの番組。
type RelatedItem struct {
	// Type は shared (同じイベントを複数のサービスで放送する)、relay (別のサービスに引き継ぐ)、movement のどれか。
	Type      string `json:"type"`
	NetworkID int    `json:"networkId,omitempty"`
	ServiceID int    `json:"serviceId"`
	EventID   int    `json:"eventId"`
}
