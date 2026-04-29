package epgstation

// RuleItem はルールの id, ruleName, reservesCnt を含む補完型。
// 生成コードの Rule = AddRuleOption には id 等が含まれないため別途定義する。
type RuleItem struct {
	ID            RuleId  `json:"id"`
	RuleName      *string `json:"ruleName,omitempty"`
	ReservesCnt   *int    `json:"reservesCnt,omitempty"`
	SearchOption  `json:"searchOption,inline"`
	ReserveOption `json:"reserveOption,inline"`
}

// SearchOption はルール検索のオプション
// Keyword 以外は不要
type SearchOption struct {
	Keyword *string `json:"keyword,omitempty"`
}

// ReserveOptions は予約のオプション
// Enable 以外は不要
type ReserveOption struct {
	Enable bool `json:"enable"`
}

// RulesExtended は id/ruleName/reservesCnt 付きのルール一覧。
// GetRulesWithResponse の Body を再デコードするために使う。
type RulesExtended struct {
	Rules []RuleItem `json:"rules"`
	Total int        `json:"total"`
}

// genreNames はARIB STD-B10 大分類ジャンルコードの日本語名マッピング。
// ProgramGenreLv1 (= int) の値をテキストに変換する。
var genreNames = map[int]string{
	0x0: "ニュース/報道",
	0x1: "スポーツ",
	0x2: "情報/ワイドショー",
	0x3: "ドラマ",
	0x4: "音楽",
	0x5: "バラエティ",
	0x6: "映画",
	0x7: "アニメ/特撮",
	0x8: "ドキュメンタリー/教養",
	0x9: "劇場/公演",
	0xa: "趣味/教育",
	0xb: "福祉",
	0xf: "その他",
}

// GenreName は genre コードを日本語名に変換する。不明な場合は空文字を返す。
func GenreName(code int) string {
	return genreNames[code]
}
