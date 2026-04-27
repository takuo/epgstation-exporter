package epgstation

// RuleItem はルールの id, ruleName, reservesCnt を含む補完型。
// 生成コードの Rule = AddRuleOption には id 等が含まれないため別途定義する。
type RuleItem struct {
	ID          RuleId  `json:"id"`
	RuleName    *string `json:"ruleName,omitempty"`
	ReservesCnt *int    `json:"reservesCnt,omitempty"`
}

// RulesExtended は id/ruleName/reservesCnt 付きのルール一覧。
// GetRulesWithResponse の Body を再デコードするために使う。
type RulesExtended struct {
	Rules []RuleItem `json:"rules"`
	Total int        `json:"total"`
}
