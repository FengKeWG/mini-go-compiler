package parser

import (
	"fmt"
	"sort"
	"strings"

	"minigo/internal/lexer"
)

// Production 表示一条表达式算符文法产生式
type Production struct {
	Left  string   // 产生式左部非终结符
	Right []string // 产生式右部符号序列
}

// FirstLastRow 表示一个非终结符的 FIRSTVT 和 LASTVT 集合
type FirstLastRow struct {
	NonTerminal string // 非终结符
	FirstVT     string // FIRSTVT 集合
	LastVT      string // LASTVT 集合
}

// RelationRow 表示算符优先表中的一个非空关系
type RelationRow struct {
	StackTop string // 栈顶终结符
	Input    string // 当前输入终结符
	Relation string // 优先关系，取值为 <、=、>
}

// OpStep 表示算符优先分析过程中的一步
type OpStep struct {
	Expression string // 被分析的表达式
	Step       int    // 步数
	Stack      string // 当前符号栈
	Input      string // 剩余输入串
	Relation   string // 栈顶终结符和当前输入符号的优先关系
	Action     string // 本步动作
}

type operatorResult struct {
	Productions []Production
	SetRows     []FirstLastRow
	Relations   []RelationRow
	Steps       []OpStep
}

// BuildOperatorResult 生成表达式部分的算符优先展示结果
func BuildOperatorResult(tokens []lexer.Token) operatorResult {
	productions := buildOperatorProductions()
	firstVT := buildFirstVT(productions)
	lastVT := buildLastVT(productions)
	table := buildPriorityTable()
	expressions := collectExpressionTexts(tokens)

	var steps []OpStep
	for _, expr := range expressions {
		steps = append(steps, analyzeExpression(expr, table)...)
	}

	return operatorResult{
		Productions: productions,
		SetRows:     buildSetRows(productions, firstVT, lastVT),
		Relations:   buildRelationRows(table),
		Steps:       steps,
	}
}

// buildOperatorProductions 定义表达式算符优先文法
func buildOperatorProductions() []Production {
	return []Production{
		{"E", []string{"E", "||", "A"}},
		{"E", []string{"A"}},
		{"A", []string{"A", "&&", "B"}},
		{"A", []string{"B"}},
		{"B", []string{"B", "==", "C"}},
		{"B", []string{"B", "!=", "C"}},
		{"B", []string{"B", "<", "C"}},
		{"B", []string{"B", "<=", "C"}},
		{"B", []string{"B", ">", "C"}},
		{"B", []string{"B", ">=", "C"}},
		{"B", []string{"C"}},
		{"C", []string{"C", "+", "D"}},
		{"C", []string{"C", "-", "D"}},
		{"C", []string{"C", "|", "D"}},
		{"C", []string{"C", "^", "D"}},
		{"C", []string{"D"}},
		{"D", []string{"D", "*", "F"}},
		{"D", []string{"D", "/", "F"}},
		{"D", []string{"D", "%", "F"}},
		{"D", []string{"D", "<<", "F"}},
		{"D", []string{"D", ">>", "F"}},
		{"D", []string{"D", "&", "F"}},
		{"D", []string{"D", "&^", "F"}},
		{"D", []string{"F"}},
		{"F", []string{"!", "F"}},
		{"F", []string{"-", "F"}},
		{"F", []string{"+", "F"}},
		{"F", []string{"^", "F"}},
		{"F", []string{"&", "F"}},
		{"F", []string{"*", "F"}},
		{"F", []string{"P"}},
		{"P", []string{"(", "E", ")"}},
		{"P", []string{"i"}},
	}
}

// buildFirstVT 按课本规则迭代计算 FIRSTVT
func buildFirstVT(productions []Production) map[string]map[string]bool {
	first := newNonTerminalSet(productions)
	changed := true
	for changed {
		changed = false
		for _, p := range productions {
			if len(p.Right) == 0 {
				continue
			}
			firstSymbol := p.Right[0]
			if isTerminal(firstSymbol) {
				changed = addSetItem(first[p.Left], firstSymbol) || changed
				continue
			}
			if len(p.Right) >= 2 && isTerminal(p.Right[1]) {
				changed = addSetItem(first[p.Left], p.Right[1]) || changed
			}
			for item := range first[firstSymbol] {
				changed = addSetItem(first[p.Left], item) || changed
			}
		}
	}
	return first
}

// buildLastVT 按课本规则迭代计算 LASTVT
func buildLastVT(productions []Production) map[string]map[string]bool {
	last := newNonTerminalSet(productions)
	changed := true
	for changed {
		changed = false
		for _, p := range productions {
			if len(p.Right) == 0 {
				continue
			}
			lastIndex := len(p.Right) - 1
			lastSymbol := p.Right[lastIndex]
			if isTerminal(lastSymbol) {
				changed = addSetItem(last[p.Left], lastSymbol) || changed
				continue
			}
			if lastIndex >= 1 && isTerminal(p.Right[lastIndex-1]) {
				changed = addSetItem(last[p.Left], p.Right[lastIndex-1]) || changed
			}
			for item := range last[lastSymbol] {
				changed = addSetItem(last[p.Left], item) || changed
			}
		}
	}
	return last
}

// buildPriorityTable 按优先级和结合性生成表达式算符优先表
func buildPriorityTable() map[string]map[string]string {
	terms := operatorTerms()
	table := map[string]map[string]string{}
	for _, left := range terms {
		table[left] = map[string]string{}
		for _, right := range terms {
			if rel := priorityRelation(left, right); rel != "" {
				table[left][right] = rel
			}
		}
	}
	return table
}

// priorityRelation 返回两个终结符之间的优先关系
func priorityRelation(left string, right string) string {
	if left == "#" && right == "#" {
		return "="
	}
	if left == "#" {
		if right != ")" {
			return "<"
		}
		return ""
	}
	if right == "#" {
		if left == "(" {
			return ""
		}
		return ">"
	}
	if left == "(" && right == ")" {
		return "="
	}
	if left == "(" {
		if canStartOperand(right) {
			return "<"
		}
		return ""
	}
	if right == ")" {
		if left == "(" {
			return "="
		}
		return ">"
	}
	if left == "i" {
		return ">"
	}
	if right == "i" || right == "(" || isPrefixOperator(right) {
		return "<"
	}
	if isOperator(left) && isOperator(right) {
		leftLevel := operatorLevel(left)
		rightLevel := operatorLevel(right)
		if leftLevel < rightLevel {
			return "<"
		}
		return ">"
	}
	return ""
}

// analyzeExpression 使用算符优先表分析一个表达式
func analyzeExpression(expr string, table map[string]map[string]string) []OpStep {
	input := strings.Fields(expr)
	if len(input) == 0 {
		return nil
	}
	input = append(input, "#")
	stack := []string{"#"}
	pos := 0
	step := 1
	var steps []OpStep

	for step <= 200 {
		current := input[pos]
		if isAcceptState(stack, current) {
			steps = append(steps, OpStep{
				Expression: expr,
				Step:       step,
				Stack:      strings.Join(stack, " "),
				Input:      strings.Join(input[pos:], " "),
				Relation:   "=",
				Action:     "接受",
			})
			return steps
		}

		top := topTerminal(stack)
		relation := table[top][current]
		if relation == "" {
			steps = append(steps, OpStep{
				Expression: expr,
				Step:       step,
				Stack:      strings.Join(stack, " "),
				Input:      strings.Join(input[pos:], " "),
				Relation:   "空",
				Action:     "出错：无优先关系",
			})
			return steps
		}

		if relation == "<" || relation == "=" {
			steps = append(steps, OpStep{
				Expression: expr,
				Step:       step,
				Stack:      strings.Join(stack, " "),
				Input:      strings.Join(input[pos:], " "),
				Relation:   relation,
				Action:     "移进 " + current,
			})
			stack = append(stack, current)
			pos++
		} else {
			nextStack, action, ok := reduceStack(stack)
			steps = append(steps, OpStep{
				Expression: expr,
				Step:       step,
				Stack:      strings.Join(stack, " "),
				Input:      strings.Join(input[pos:], " "),
				Relation:   relation,
				Action:     action,
			})
			if !ok {
				return steps
			}
			stack = nextStack
		}
		step++
	}

	steps = append(steps, OpStep{
		Expression: expr,
		Step:       step,
		Stack:      strings.Join(stack, " "),
		Input:      strings.Join(input[pos:], " "),
		Relation:   "",
		Action:     "出错：分析步骤过多",
	})
	return steps
}

// collectExpressionTexts 从完整 Token 序列中抽取表达式片段
func collectExpressionTexts(tokens []lexer.Token) []string {
	var expressions []string
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind == "eof" {
			break
		}
		if tok.Text == "=" || tok.Text == ":=" {
			expr, next := collectUntil(tokens, i+1, map[string]bool{";": true})
			expressions = appendNormalizedExpression(expressions, expr)
			i = next
			continue
		}
		if tok.Kind == "k" && tok.Text == "return" {
			expr, next := collectUntil(tokens, i+1, map[string]bool{";": true})
			expressions = appendNormalizedExpression(expressions, expr)
			i = next
			continue
		}
		if tok.Kind == "k" && tok.Text == "if" {
			expr, next := collectUntil(tokens, i+1, map[string]bool{"{": true})
			expressions = appendNormalizedExpression(expressions, expr)
			i = next
			continue
		}
		if tok.Kind == "k" && tok.Text == "for" {
			header, next := collectUntil(tokens, i+1, map[string]bool{"{": true})
			expressions = appendForExpressions(expressions, header)
			i = next
		}
	}
	return uniqueStrings(expressions)
}

// appendForExpressions 抽取 for 头部中的条件表达式
func appendForExpressions(expressions []string, header []lexer.Token) []string {
	parts := splitBySemicolon(header)
	if len(parts) == 3 {
		return appendNormalizedExpression(expressions, parts[1])
	}
	return appendNormalizedExpression(expressions, header)
}

// normalizeExpression 把复杂操作数统一抽象成 i
func normalizeExpression(tokens []lexer.Token) string {
	var parts []string
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind == "i" {
			parts = append(parts, "i")
			i = skipOperandSuffix(tokens, i)
			continue
		}
		if tok.Kind == "c" || tok.Text == "true" || tok.Text == "false" {
			parts = append(parts, "i")
			continue
		}
		if isOperatorToken(tok.Text) || tok.Text == "(" || tok.Text == ")" {
			parts = append(parts, tok.Text)
		}
	}
	return strings.Join(parts, " ")
}

// skipOperandSuffix 跳过函数调用、数组下标和结构体字段后缀
func skipOperandSuffix(tokens []lexer.Token, i int) int {
	for i+1 < len(tokens) {
		next := tokens[i+1].Text
		if next == "(" {
			i = skipBalanced(tokens, i+1, "(", ")")
			continue
		}
		if next == "[" {
			i = skipBalanced(tokens, i+1, "[", "]")
			continue
		}
		if next == "." && i+2 < len(tokens) {
			i += 2
			continue
		}
		break
	}
	return i
}

// reduceStack 执行一次算符优先归约
func reduceStack(stack []string) ([]string, string, bool) {
	n := len(stack)
	if n >= 1 && stack[n-1] == "i" {
		return append(stack[:n-1], "N"), "归约 i -> N", true
	}
	if n >= 3 && stack[n-3] == "(" && stack[n-2] == "N" && stack[n-1] == ")" {
		return append(stack[:n-3], "N"), "归约 ( N ) -> N", true
	}
	if n >= 3 && stack[n-3] == "N" && isOperator(stack[n-2]) && stack[n-1] == "N" {
		action := fmt.Sprintf("归约 N %s N -> N", stack[n-2])
		return append(stack[:n-3], "N"), action, true
	}
	if n >= 2 && isPrefixOperator(stack[n-2]) && stack[n-1] == "N" {
		action := fmt.Sprintf("归约 %s N -> N", stack[n-2])
		return append(stack[:n-2], "N"), action, true
	}
	return stack, "出错：无法归约", false
}

func buildSetRows(productions []Production, firstVT map[string]map[string]bool, lastVT map[string]map[string]bool) []FirstLastRow {
	var rows []FirstLastRow
	for _, nt := range nonTerminals(productions) {
		rows = append(rows, FirstLastRow{
			NonTerminal: nt,
			FirstVT:     strings.Join(sortedSet(firstVT[nt]), ", "),
			LastVT:      strings.Join(sortedSet(lastVT[nt]), ", "),
		})
	}
	return rows
}

func buildRelationRows(table map[string]map[string]string) []RelationRow {
	var rows []RelationRow
	for _, left := range operatorTerms() {
		for _, right := range operatorTerms() {
			if rel := table[left][right]; rel != "" {
				rows = append(rows, RelationRow{StackTop: left, Input: right, Relation: rel})
			}
		}
	}
	return rows
}

func collectUntil(tokens []lexer.Token, start int, stop map[string]bool) ([]lexer.Token, int) {
	var result []lexer.Token
	depth := 0
	for i := start; i < len(tokens); i++ {
		text := tokens[i].Text
		if text == "(" || text == "[" {
			depth++
		}
		if text == ")" || text == "]" {
			depth--
		}
		if depth == 0 && stop[text] {
			return result, i
		}
		result = append(result, tokens[i])
	}
	return result, len(tokens)
}

func splitBySemicolon(tokens []lexer.Token) [][]lexer.Token {
	var parts [][]lexer.Token
	start := 0
	for i, tok := range tokens {
		if tok.Text == ";" {
			parts = append(parts, tokens[start:i])
			start = i + 1
		}
	}
	parts = append(parts, tokens[start:])
	return parts
}

func appendNormalizedExpression(expressions []string, tokens []lexer.Token) []string {
	expr := normalizeExpression(tokens)
	if expr == "" {
		return expressions
	}
	return append(expressions, expr)
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func skipBalanced(tokens []lexer.Token, start int, left string, right string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Text == left {
			depth++
		}
		if tokens[i].Text == right {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(tokens) - 1
}

func topTerminal(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != "N" {
			return stack[i]
		}
	}
	return "#"
}

func isAcceptState(stack []string, current string) bool {
	return current == "#" && len(stack) == 2 && stack[0] == "#" && stack[1] == "N"
}

func canStartOperand(text string) bool {
	return text == "i" || text == "(" || text == "!"
}

func isOperatorToken(text string) bool {
	return isOperator(text) || text == "!"
}

func isPrefixOperator(text string) bool {
	return text == "!" || text == "+" || text == "-" || text == "^" || text == "&" || text == "*"
}

func isOperator(text string) bool {
	return operatorLevel(text) > 0
}

func operatorLevel(text string) int {
	switch text {
	case "||":
		return 1
	case "&&":
		return 2
	case "==", "!=", "<", "<=", ">", ">=":
		return 3
	case "+", "-", "|", "^":
		return 4
	case "*", "/", "%", "<<", ">>", "&", "&^":
		return 5
	case "!":
		return 6
	default:
		return 0
	}
}

func operatorTerms() []string {
	return []string{
		"#", "i", "(", ")", "!",
		"||", "&&",
		"==", "!=", "<", "<=", ">", ">=",
		"+", "-", "|", "^",
		"*", "/", "%", "<<", ">>", "&", "&^",
	}
}

func isTerminal(symbol string) bool {
	for _, term := range operatorTerms() {
		if symbol == term {
			return true
		}
	}
	return false
}

func newNonTerminalSet(productions []Production) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	for _, p := range productions {
		if result[p.Left] == nil {
			result[p.Left] = map[string]bool{}
		}
	}
	return result
}

func nonTerminals(productions []Production) []string {
	seen := map[string]bool{}
	var result []string
	for _, p := range productions {
		if !seen[p.Left] {
			seen[p.Left] = true
			result = append(result, p.Left)
		}
	}
	return result
}

func addSetItem(set map[string]bool, item string) bool {
	if set[item] {
		return false
	}
	set[item] = true
	return true
}

func sortedSet(set map[string]bool) []string {
	var result []string
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
