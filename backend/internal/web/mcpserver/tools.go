package mcpserver

import "github.com/SlashNephy/kotatsu/internal/usecase/oauth"

// tool は、MCP のツール。中身は GraphQL の操作で、画面と同じ処理と権限の確認を通る (設計書 9.10)。
// 入力の JSON Schema は、操作の変数の型から作る。
type tool struct {
	name        string
	title       string
	description string
	// scope は、ツールを使うのに要るスコープ。
	scope string
	// readOnly は、ツールが何も変えないかどうか。destructive は、消すツールかどうか (応答から前の状態に戻せない)。
	readOnly    bool
	destructive bool
	// query は、ツールが実行する GraphQL の操作。空なら、handle で答える。
	query string
	// handle は、GraphQL を使わずに答えるツールの処理。
	handle func() (any, error)
}

const (
	programFields = `id name description startAt endAt isFree archived
		service { id name channelType { type displayName } }
		genres { lv1 lv2 }
		reservation { id enabled }`
	reservationFields = `id kind name description startAt endAt enabled followEventRelay eventId
		service { id name }
		genres { lv1 lv2 }
		assignment { state plannedStartAt plannedEndAt }
		settings { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority joinSegments detectCm }
		rules { id name }
		createdAt updatedAt`
	ruleFields = `id name memo enabled validFrom validUntil avoidDuplicates duplicateWindowDays followEventRelay upcomingReservationCount
		conditions {
			keywords { text regexp caseSensitive fields }
			keywordMatch
			excludeKeywords { text regexp caseSensitive fields }
			channelTypes
			services { id name }
			excludeServices { id name }
			genres { lv1 lv2 }
			excludeGenres { lv1 lv2 }
			timeRanges { weekdays startMinute durationMinutes }
			minDurationSeconds maxDurationSeconds freeOnly marks excludeMarks
		}
		settings { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority joinSegments detectCm }
		createdAt updatedAt`
	recordingFields = `id name description startAt endAt state failureReason protected drops errors scrambled ruleName
		service { id name }
		genres { lv1 lv2 }
		tags { id name }`
)

// readTool、writeTool、deleteTool は、読む、書く、消すツールを作る。
func readTool(name, title, description, query string) *tool {
	return &tool{name: name, title: title, description: description, scope: oauth.ScopeRead, readOnly: true, destructive: false, query: query, handle: nil}
}

func writeTool(name, title, description, query string) *tool {
	return &tool{name: name, title: title, description: description, scope: oauth.ScopeWrite, readOnly: false, destructive: false, query: query, handle: nil}
}

func deleteTool(name, title, description, query string) *tool {
	return &tool{name: name, title: title, description: description, scope: oauth.ScopeWrite, readOnly: false, destructive: true, query: query, handle: nil}
}

// toolList は、公開するツールを返す。tools/list では、SDK が名前の順に並べて返す (MCP 2026-07-28 は、決まった順に返すことを勧める)。
func toolList() []*tool {
	return []*tool{
		readTool("list_services", "サービスとチャンネル種別の一覧",
			"番組表に出すサービス (放送局) と、チャンネル種別 (GR、BS、CS など) の一覧を返す。"+
				"番組の検索やルールの条件で、サービスの ID とチャンネル種別を指定するのに使う。",
			`query { channelTypes { type displayName } services { id name channel channelType { type displayName } } }`),
		{
			name: "list_genres", title: "ジャンルの一覧", scope: oauth.ScopeRead, readOnly: true, destructive: false, query: "",
			description: "番組のジャンルの大分類 (lv1) の番号と名前の一覧を返す。番組の検索やルールの条件で、ジャンルを指定するのに使う。",
			handle:      listGenres,
		},
		readTool("search_programs", "番組の検索",
			"条件に合う番組を、放送の早い順に返す。既定では、これから放送する番組と放送中の番組を探す。"+
				"archive を渡すと、その期間の放送済みの番組を探す。続きは pageInfo.endCursor を after に渡して読む。",
			`query($conditions: ProgramConditionsInput!, $archive: ArchiveRangeInput, $first: Int = 20, $after: String) {
			searchPrograms(conditions: $conditions, archive: $archive, first: $first, after: $after) {
				edges { node { `+programFields+` } }
				pageInfo { hasNextPage endCursor }
			}
		}`),
		readTool("list_reservations", "予約の一覧",
			"from から to までに重なる録画の予約を、開始の早い順に返す。assignment.state は、チューナーの割り当ての結果。"+
				"続きは pageInfo.endCursor を after に渡して読む。",
			`query($from: Time!, $to: Time!, $states: [AssignmentState!], $enabled: Boolean, $rule: ID, $first: Int = 50, $after: String) {
			reservations(from: $from, to: $to, states: $states, enabled: $enabled, rule: $rule, first: $first, after: $after) {
				edges { node { `+reservationFields+` } }
				pageInfo { hasNextPage endCursor }
			}
		}`),
		readTool("get_reservation", "予約の取得",
			"予約を ID で返す。",
			`query($id: ID!) { node(id: $id) { ... on Reservation { `+reservationFields+` } } }`),
		writeTool("create_reservation", "予約の作成",
			"番組を録画する予約を作る。program には、番組の検索で得た番組の ID を渡す。",
			`mutation($input: ProgramReservationInput!) { createProgramReservation(input: $input) { `+reservationFields+` } }`),
		writeTool("update_reservation", "予約の変更",
			"予約の有効と無効、イベントリレーへの追従、録画設定を変える。省いた項目は変えない。",
			`mutation($id: ID!, $input: ReservationUpdateInput!) { updateReservation(id: $id, input: $input) { `+reservationFields+` } }`),
		deleteTool("delete_reservation", "予約の削除",
			"予約を消す。ルールで作られた予約は、消しても、ルールに一致する限り作り直される。無効にするなら update_reservation を使う。",
			`mutation($id: ID!) { deleteReservation(id: $id) { id } }`),
		readTool("list_rules", "ルールの一覧",
			"録画のルール (条件に合う番組を自動で予約する) の一覧を返す。",
			`query { rules { `+ruleFields+` } }`),
		readTool("get_rule", "ルールの取得",
			"ルールを ID で返す。",
			`query($id: ID!) { node(id: $id) { ... on Rule { `+ruleFields+` } } }`),
		writeTool("create_rule", "ルールの作成",
			"録画のルールを作る。条件は search_programs の conditions と同じ形で、作る前に search_programs で一致する番組を確かめるとよい。",
			`mutation($input: RuleInput!) { createRule(input: $input) { `+ruleFields+` } }`),
		writeTool("update_rule", "ルールの変更",
			"ルールを変える。input には、変えない項目も含めたルールの全体を渡す。",
			`mutation($id: ID!, $input: RuleInput!) { updateRule(id: $id, input: $input) { `+ruleFields+` } }`),
		deleteTool("delete_rule", "ルールの削除",
			"ルールを消す。ルールで作った予約のうち、ほかのルールに一致しないものも消える。",
			`mutation($id: ID!) { deleteRule(id: $id) }`),
		readTool("search_recordings", "録画の検索",
			"録画を、録画を始めた日時の新しい順に返す。filter.keyword で番組名を絞れる。続きは pageInfo.endCursor を after に渡して読む。",
			`query($filter: RecordingFilter, $first: Int = 20, $after: String) {
			recordings(filter: $filter, first: $first, after: $after) {
				edges { node { `+recordingFields+` } }
				pageInfo { hasNextPage endCursor }
			}
		}`),
	}
}

// genre は、ジャンルの大分類。
type genre struct {
	Lv1  int    `json:"lv1"`
	Name string `json:"name"`
}

// listGenres は、ジャンルの大分類の一覧を返す (ARIB STD-B10 の content_nibble_level_1)。
func listGenres() (any, error) {
	return map[string]any{"genres": []genre{
		{Lv1: 0, Name: "ニュース・報道"},
		{Lv1: 1, Name: "スポーツ"},
		{Lv1: 2, Name: "情報・ワイドショー"},
		{Lv1: 3, Name: "ドラマ"},
		{Lv1: 4, Name: "音楽"},
		{Lv1: 5, Name: "バラエティ"},
		{Lv1: 6, Name: "映画"},
		{Lv1: 7, Name: "アニメ・特撮"},
		{Lv1: 8, Name: "ドキュメンタリー・教養"},
		{Lv1: 9, Name: "劇場・公演"},
		{Lv1: 10, Name: "趣味・教育"},
		{Lv1: 11, Name: "福祉"},
		{Lv1: 15, Name: "その他"},
	}}, nil
}
