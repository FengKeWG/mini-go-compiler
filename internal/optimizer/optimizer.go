package optimizer

import (
	"sort"
	"strconv"
	"strings"

	"minigo/internal/semantic"
)

// Block 表示一个基本块。
type Block struct {
	Index int
	Start int
	End   int
	Quads []semantic.Quad
}

// Step 表示一次优化的统计信息。
type Step struct {
	Name    string
	Before  int
	After   int
	Changed int
}

// Result 保存优化阶段的全部输出。
type Result struct {
	Blocks    []Block
	Original  []semantic.Quad
	Optimized []semantic.Quad
	Steps     []Step
}

type exprItem struct {
	Result string
	Arg1   string
	Arg2   string
}

// Optimize 对语义分析生成的四元式做基础优化。
func Optimize(quads []semantic.Quad) Result {
	original := copyQuads(quads)
	blocks := BuildBasicBlocks(original)

	folded, foldCount := foldConstants(original)
	commonSaved, commonCount := saveCommonExpressions(folded)
	deadRemoved, deadCount := removeDeadAssignments(commonSaved)
	optimized := reindexQuads(deadRemoved)

	steps := []Step{
		{Name: "常量表达式节省", Before: len(original), After: len(folded), Changed: foldCount},
		{Name: "公共子表达式节省", Before: len(folded), After: len(commonSaved), Changed: commonCount},
		{Name: "删除无用赋值", Before: len(commonSaved), After: len(deadRemoved), Changed: deadCount},
	}

	return Result{
		Blocks:    blocks,
		Original:  original,
		Optimized: optimized,
		Steps:     steps,
	}
}

// BuildBasicBlocks 根据跳转和标号划分基本块。
func BuildBasicBlocks(quads []semantic.Quad) []Block {
	if len(quads) == 0 {
		return nil
	}

	leaders := map[int]bool{0: true}
	for i, q := range quads {
		if q.Op == "label" {
			leaders[i] = true
		}
		if isBlockEnd(q.Op) && i+1 < len(quads) {
			leaders[i+1] = true
		}
	}

	indexes := []int{}
	for index := range leaders {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	blocks := []Block{}
	for i, start := range indexes {
		end := len(quads) - 1
		if i+1 < len(indexes) {
			end = indexes[i+1] - 1
		}
		blocks = append(blocks, Block{
			Index: len(blocks) + 1,
			Start: quads[start].Index,
			End:   quads[end].Index,
			Quads: copyQuads(quads[start : end+1]),
		})
	}
	return blocks
}

// foldConstants 把常量运算直接算出来，例如 (+, 2, 3, t1) 变为 (=, 5, _, t1)。
func foldConstants(quads []semantic.Quad) ([]semantic.Quad, int) {
	result := copyQuads(quads)
	count := 0
	for i, q := range result {
		value, ok := evalQuad(q)
		if !ok {
			continue
		}
		result[i] = semantic.Quad{
			Index:  q.Index,
			Op:     "=",
			Arg1:   value,
			Arg2:   "_",
			Result: q.Result,
		}
		count++
	}
	return result, count
}

// saveCommonExpressions 在每个基本块内复用已经计算过的表达式。
func saveCommonExpressions(quads []semantic.Quad) ([]semantic.Quad, int) {
	blocks := BuildBasicBlocks(quads)
	result := []semantic.Quad{}
	count := 0

	for _, block := range blocks {
		exprs := map[string]exprItem{}
		for _, q := range block.Quads {
			if hasAssignedResult(q) {
				clearExprsByName(exprs, baseName(q.Result))
			}

			if isExpressionOp(q.Op) {
				key := expressionKey(q)
				if old, ok := exprs[key]; ok {
					q.Op = "="
					q.Arg1 = old.Result
					q.Arg2 = "_"
					count++
				} else {
					exprs[key] = exprItem{Result: q.Result, Arg1: q.Arg1, Arg2: q.Arg2}
				}
			}
			result = append(result, q)
		}
	}

	return result, count
}

// removeDeadAssignments 删除同一基本块中被覆盖且没有被使用的赋值。
func removeDeadAssignments(quads []semantic.Quad) ([]semantic.Quad, int) {
	blocks := BuildBasicBlocks(quads)
	allUserNames := collectUserNames(quads)
	result := []semantic.Quad{}
	count := 0

	for _, block := range blocks {
		live := copyNameSet(allUserNames)
		keep := make([]bool, len(block.Quads))

		for i := len(block.Quads) - 1; i >= 0; i-- {
			q := block.Quads[i]
			if hasAssignedResult(q) && canDeleteAssignment(q) {
				name := baseName(q.Result)
				if !live[name] {
					count++
					continue
				}
				delete(live, name)
				addUsedNames(live, q)
				keep[i] = true
				continue
			}

			addUsedNames(live, q)
			keep[i] = true
		}

		for i, q := range block.Quads {
			if keep[i] {
				result = append(result, q)
			}
		}
	}

	return result, count
}

func copyQuads(quads []semantic.Quad) []semantic.Quad {
	result := make([]semantic.Quad, len(quads))
	copy(result, quads)
	return result
}

func reindexQuads(quads []semantic.Quad) []semantic.Quad {
	result := copyQuads(quads)
	for i := range result {
		result[i].Index = i + 1
	}
	return result
}

func isBlockEnd(op string) bool {
	return op == "j" || op == "jfalse" || op == "return" || op == "end"
}

func isExpressionOp(op string) bool {
	return isBinaryOp(op) || isUnaryOp(op)
}

func isBinaryOp(op string) bool {
	switch op {
	case "+", "-", "*", "/", "%", "<", "<=", ">", ">=", "==", "!=", "&&", "||", "&", "|", "^", "&^", "<<", ">>":
		return true
	default:
		return false
	}
}

func isUnaryOp(op string) bool {
	return op == "uminus" || op == "uplus" || op == "!"
}

func hasAssignedResult(q semantic.Quad) bool {
	return q.Result != "" && q.Result != "_" && (q.Op == "=" || isExpressionOp(q.Op))
}

func canDeleteAssignment(q semantic.Quad) bool {
	if q.Result == "" || q.Result == "_" {
		return false
	}
	if strings.Contains(q.Result, "[") || strings.Contains(q.Result, ".") {
		return false
	}
	return q.Op == "=" || isExpressionOp(q.Op)
}

func expressionKey(q semantic.Quad) string {
	arg1 := q.Arg1
	arg2 := q.Arg2
	if isCommutative(q.Op) && arg1 > arg2 {
		arg1, arg2 = arg2, arg1
	}
	return q.Op + "|" + arg1 + "|" + arg2
}

func isCommutative(op string) bool {
	return op == "+" || op == "*" || op == "==" || op == "!=" ||
		op == "&&" || op == "||" || op == "&" || op == "|" || op == "^"
}

func clearExprsByName(exprs map[string]exprItem, name string) {
	if name == "" {
		return
	}
	for key, item := range exprs {
		if baseName(item.Result) == name || baseName(item.Arg1) == name || baseName(item.Arg2) == name {
			delete(exprs, key)
		}
	}
}

func evalQuad(q semantic.Quad) (string, bool) {
	if isBinaryOp(q.Op) {
		return evalBinary(q.Op, q.Arg1, q.Arg2)
	}
	if isUnaryOp(q.Op) {
		return evalUnary(q.Op, q.Arg1)
	}
	return "", false
}

func evalBinary(op string, left string, right string) (string, bool) {
	if op == "&&" || op == "||" {
		leftBool, ok1 := parseBool(left)
		rightBool, ok2 := parseBool(right)
		if !ok1 || !ok2 {
			return "", false
		}
		if op == "&&" {
			return formatBool(leftBool && rightBool), true
		}
		return formatBool(leftBool || rightBool), true
	}

	if op == "%" || op == "&" || op == "|" || op == "^" || op == "&^" || op == "<<" || op == ">>" {
		leftInt, ok1 := parseInt(left)
		rightInt, ok2 := parseInt(right)
		if !ok1 || !ok2 {
			return "", false
		}
		return evalIntBinary(op, leftInt, rightInt)
	}

	if !strings.Contains(left, ".") && !strings.Contains(right, ".") {
		leftInt, ok1 := parseInt(left)
		rightInt, ok2 := parseInt(right)
		if ok1 && ok2 {
			return evalNormalIntBinary(op, leftInt, rightInt)
		}
	}

	leftFloat, ok1 := parseNumber(left)
	rightFloat, ok2 := parseNumber(right)
	if !ok1 || !ok2 {
		return "", false
	}

	switch op {
	case "+":
		return formatNumber(leftFloat+rightFloat, left, right), true
	case "-":
		return formatNumber(leftFloat-rightFloat, left, right), true
	case "*":
		return formatNumber(leftFloat*rightFloat, left, right), true
	case "/":
		if rightFloat == 0 {
			return "", false
		}
		return formatNumber(leftFloat/rightFloat, left, right), true
	case "<":
		return formatBool(leftFloat < rightFloat), true
	case "<=":
		return formatBool(leftFloat <= rightFloat), true
	case ">":
		return formatBool(leftFloat > rightFloat), true
	case ">=":
		return formatBool(leftFloat >= rightFloat), true
	case "==":
		return formatBool(leftFloat == rightFloat), true
	case "!=":
		return formatBool(leftFloat != rightFloat), true
	default:
		return "", false
	}
}

func evalNormalIntBinary(op string, left int64, right int64) (string, bool) {
	switch op {
	case "+":
		return strconv.FormatInt(left+right, 10), true
	case "-":
		return strconv.FormatInt(left-right, 10), true
	case "*":
		return strconv.FormatInt(left*right, 10), true
	case "/":
		if right == 0 {
			return "", false
		}
		return strconv.FormatInt(left/right, 10), true
	case "<":
		return formatBool(left < right), true
	case "<=":
		return formatBool(left <= right), true
	case ">":
		return formatBool(left > right), true
	case ">=":
		return formatBool(left >= right), true
	case "==":
		return formatBool(left == right), true
	case "!=":
		return formatBool(left != right), true
	default:
		return "", false
	}
}

func evalIntBinary(op string, left int64, right int64) (string, bool) {
	switch op {
	case "%":
		if right == 0 {
			return "", false
		}
		return strconv.FormatInt(left%right, 10), true
	case "&":
		return strconv.FormatInt(left&right, 10), true
	case "|":
		return strconv.FormatInt(left|right, 10), true
	case "^":
		return strconv.FormatInt(left^right, 10), true
	case "&^":
		return strconv.FormatInt(left&^right, 10), true
	case "<<":
		if right < 0 {
			return "", false
		}
		return strconv.FormatInt(left<<uint(right), 10), true
	case ">>":
		if right < 0 {
			return "", false
		}
		return strconv.FormatInt(left>>uint(right), 10), true
	default:
		return "", false
	}
}

func evalUnary(op string, value string) (string, bool) {
	if op == "!" {
		boolValue, ok := parseBool(value)
		if !ok {
			return "", false
		}
		return formatBool(!boolValue), true
	}

	number, ok := parseNumber(value)
	if !ok {
		return "", false
	}
	if op == "uminus" {
		return formatNumber(-number, value, ""), true
	}
	if op == "uplus" {
		return formatNumber(number, value, ""), true
	}
	return "", false
}

func parseBool(text string) (bool, bool) {
	if text == "true" {
		return true, true
	}
	if text == "false" {
		return false, true
	}
	return false, false
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func parseInt(text string) (int64, bool) {
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

func parseNumber(text string) (float64, bool) {
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil
}

func formatNumber(value float64, left string, right string) string {
	if !strings.Contains(left, ".") && !strings.Contains(right, ".") && value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func collectUserNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		addUserName(names, q.Arg1)
		addUserName(names, q.Arg2)
		addUserName(names, q.Result)
	}
	return names
}

func copyNameSet(names map[string]bool) map[string]bool {
	result := map[string]bool{}
	for name := range names {
		result[name] = true
	}
	return result
}

func addUsedNames(names map[string]bool, q semantic.Quad) {
	addLiveName(names, q.Arg1)
	addLiveName(names, q.Arg2)
	if q.Op != "=" && strings.Contains(q.Result, "[") {
		addLiveName(names, q.Result)
	}
}

func addUserName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isTempName(name) || isLabelName(name) {
		return
	}
	names[name] = true
}

func addLiveName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isLabelName(name) {
		return
	}
	names[name] = true
}

func baseName(text string) string {
	if text == "" || text == "_" {
		return ""
	}
	name := text
	if index := strings.Index(name, "["); index >= 0 {
		name = name[:index]
	}
	if index := strings.Index(name, "."); index >= 0 {
		name = name[:index]
	}
	return name
}

func isLiteral(text string) bool {
	if text == "" || text == "_" {
		return true
	}
	if text == "true" || text == "false" {
		return true
	}
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return true
	}
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

func isTempName(name string) bool {
	if len(name) < 2 || name[0] != 't' {
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func isLabelName(name string) bool {
	if len(name) < 2 || name[0] != 'L' {
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
