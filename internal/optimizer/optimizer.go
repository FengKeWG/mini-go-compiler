package optimizer

import (
	"sort"
	"strconv"
	"strings"

	"minigo/internal/semantic"
)

// Block 表示一个基本块
type Block struct {
	Index int             // 基本块编号
	Start int             // 基本块第一条四元式编号
	End   int             // 基本块最后一条四元式编号
	Quads []semantic.Quad // 基本块内包含的四元式
}

// Step 表示一次优化的统计信息
type Step struct {
	Name    string // 优化名称
	Before  int    // 优化前四元式数量
	After   int    // 优化后四元式数量
	Changed int    // 本轮优化影响的四元式条数
}

// Result 保存优化阶段的全部输出
type Result struct {
	Blocks    []Block         // 基本块划分结果
	Original  []semantic.Quad // 优化前四元式
	Optimized []semantic.Quad // 优化后四元式
	Steps     []Step          // 各优化步骤的统计信息
}

type exprItem struct {
	Result string // 已经计算过的结果名
	Arg1   string // 表达式第一个操作数
	Arg2   string // 表达式第二个操作数
}

// Optimize 对语义分析生成的四元式做基础优化
func Optimize(quads []semantic.Quad) Result {
	// 保留原四元式，方便界面展示优化前后对比
	original := copyQuads(quads)
	blocks := BuildBasicBlocks(original)

	// 优化顺序按课程设计表格中的几类基础优化排列
	folded, foldCount := foldConstants(original)
	commonSaved, commonCount := saveCommonExpressions(folded)
	loopMoved, loopCount := optimizeLoops(commonSaved)
	deadRemoved, deadCount := removeDeadAssignments(loopMoved)
	optimized := reindexQuads(deadRemoved)

	steps := []Step{
		{Name: "常量表达式节省", Before: len(original), After: len(folded), Changed: foldCount},
		{Name: "公共子表达式节省", Before: len(folded), After: len(commonSaved), Changed: commonCount},
		{Name: "循环优化", Before: len(commonSaved), After: len(loopMoved), Changed: loopCount},
		{Name: "删除无用赋值", Before: len(loopMoved), After: len(deadRemoved), Changed: deadCount},
	}

	return Result{
		Blocks:    blocks,
		Original:  original,
		Optimized: optimized,
		Steps:     steps,
	}
}

// optimizeLoops 做基础循环优化，把循环体中的不变表达式提前到循环入口前
func optimizeLoops(quads []semantic.Quad) ([]semantic.Quad, int) {
	result := copyQuads(quads)
	count := 0

	for {
		changed := false
		// labelIndex 用于从 j 指令快速找到回跳目标
		labelIndex := buildLabelIndex(result)
		for i, q := range result {
			if q.Op != "j" {
				continue
			}
			start, ok := labelIndex[q.Result]
			if !ok || start >= i {
				continue
			}

			// 一个向前面的标签跳转的 j，通常表示循环回边
			bodyStart := findLoopBodyStart(result, start, i)
			if bodyStart < 0 {
				continue
			}
			indexes := findLoopInvariants(result, bodyStart, i)
			if len(indexes) == 0 {
				continue
			}

			// 每轮只移动一组，移动后重新分析标签和下标
			result = moveQuadsBefore(result, indexes, start)
			count += len(indexes)
			changed = true
			break
		}
		if !changed {
			break
		}
	}

	return result, count
}

// BuildBasicBlocks 根据跳转和标号划分基本块
func BuildBasicBlocks(quads []semantic.Quad) []Block {
	if len(quads) == 0 {
		return nil
	}

	// leaders 保存每个基本块入口在切片中的下标
	leaders := map[int]bool{0: true}
	for i, q := range quads {
		if q.Op == "label" {
			// 标号所在位置一定是基本块入口
			leaders[i] = true
		}
		if isBlockEnd(q.Op) && i+1 < len(quads) {
			// 跳转、返回和函数结束之后的下一条也是基本块入口
			leaders[i+1] = true
		}
	}

	var indexes []int
	for index := range leaders {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	var blocks []Block
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

// foldConstants 把常量运算直接算出来，例如 (+, 2, 3, t1) 变为 (=, 5, _, t1)
func foldConstants(quads []semantic.Quad) ([]semantic.Quad, int) {
	result := copyQuads(quads)
	count := 0
	for i, q := range result {
		// 只有两个操作数都是常量时才可以在编译期求值
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

// saveCommonExpressions 在每个基本块内复用已经计算过的表达式
func saveCommonExpressions(quads []semantic.Quad) ([]semantic.Quad, int) {
	blocks := BuildBasicBlocks(quads)
	var result []semantic.Quad
	count := 0

	for _, block := range blocks {
		exprs := map[string]exprItem{}
		for _, q := range block.Quads {
			if hasAssignedResult(q) {
				// 变量被重新赋值后，依赖它的表达式缓存全部失效
				clearExprsByName(exprs, baseName(q.Result))
			}

			if isExpressionOp(q.Op) {
				key := expressionKey(q)
				if old, ok := exprs[key]; ok {
					// 找到相同表达式时，直接把旧结果赋给新结果
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

// removeDeadAssignments 删除同一基本块中被覆盖且没有被使用的赋值
func removeDeadAssignments(quads []semantic.Quad) ([]semantic.Quad, int) {
	blocks := BuildBasicBlocks(quads)
	// 用户变量默认认为在基本块出口仍可能被使用，避免误删最终结果
	initialLive := collectInitialLiveNames(quads)
	var result []semantic.Quad
	count := 0

	for _, block := range blocks {
		live := copyNameSet(initialLive)
		keep := make([]bool, len(block.Quads))

		// 从后往前扫描，符合活跃变量分析的基本做法
		for i := len(block.Quads) - 1; i >= 0; i-- {
			q := block.Quads[i]
			if hasAssignedResult(q) && canDeleteAssignment(q) {
				name := baseName(q.Result)
				if !live[name] {
					// 结果后面没有再用到，这条赋值可以删除
					count++
					continue
				}
				// 当前四元式定义了 name，往前看时 name 不再活跃
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

// buildLabelIndex 建立标签名到四元式下标的映射
func buildLabelIndex(quads []semantic.Quad) map[string]int {
	labelIndex := map[string]int{}
	for i, q := range quads {
		if q.Op == "label" {
			labelIndex[q.Result] = i
		}
	}
	return labelIndex
}

// findLoopBodyStart 找到 jfalse 后面的第一条循环体四元式
func findLoopBodyStart(quads []semantic.Quad, start int, end int) int {
	for i := start + 1; i < end; i++ {
		if quads[i].Op == "jfalse" {
			return i + 1
		}
	}
	return -1
}

// findLoopInvariants 找出循环体中可以外提的循环不变表达式
func findLoopInvariants(quads []semantic.Quad, bodyStart int, bodyEnd int) []int {
	// assigned 表示循环体内会被改写的名字
	assigned := collectAssignedNames(quads[bodyStart:bodyEnd])
	// invariantTemps 表示已经确认是不变表达式结果的临时变量
	invariantTemps := map[string]bool{}
	var indexes []int

	for i := bodyStart; i < bodyEnd; i++ {
		q := quads[i]
		if !canHoist(q) {
			continue
		}
		if !isLoopInvariantValue(q.Arg1, assigned, invariantTemps) {
			continue
		}
		if !isLoopInvariantValue(q.Arg2, assigned, invariantTemps) {
			continue
		}
		// 两个操作数都不随循环变化，这条表达式可以外提
		indexes = append(indexes, i)
		invariantTemps[baseName(q.Result)] = true
	}

	return indexes
}

// canHoist 判断一条四元式是否具备外提的基本条件
func canHoist(q semantic.Quad) bool {
	if !isExpressionOp(q.Op) {
		return false
	}
	if !isTempName(baseName(q.Result)) {
		// 只外提写入临时变量的表达式，避免改变用户变量赋值位置
		return false
	}
	if q.Op == "/" || q.Op == "%" {
		// 除法和取模可能涉及除零，保守起见不外提
		return false
	}
	return !hasComplexName(q.Arg1) && !hasComplexName(q.Arg2) && !hasComplexName(q.Result)
}

// isLoopInvariantValue 判断一个操作数在当前循环中是否保持不变
func isLoopInvariantValue(text string, assigned map[string]bool, invariantTemps map[string]bool) bool {
	name := baseName(text)
	if name == "" || isLiteral(text) {
		return true
	}
	if isLabelName(name) || hasComplexName(text) {
		return false
	}
	if isTempName(name) {
		// 临时变量要么本身不在循环中被赋值，要么来自前面已经确认的不变表达式
		return invariantTemps[name] || !assigned[name]
	}
	return !assigned[name]
}

// collectAssignedNames 收集一段四元式中被赋值的名字
func collectAssignedNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		if hasAssignedResult(q) {
			name := baseName(q.Result)
			if name != "" {
				names[name] = true
			}
		}
	}
	return names
}

// moveQuadsBefore 把若干条四元式按原顺序移动到指定位置前面
func moveQuadsBefore(quads []semantic.Quad, indexes []int, before int) []semantic.Quad {
	moved := map[int]bool{}
	for _, index := range indexes {
		moved[index] = true
	}

	var result []semantic.Quad
	for i := 0; i < before; i++ {
		result = append(result, quads[i])
	}
	for _, index := range indexes {
		result = append(result, quads[index])
	}
	for i := before; i < len(quads); i++ {
		if !moved[i] {
			result = append(result, quads[i])
		}
	}
	return result
}

// copyQuads 复制四元式切片，避免优化过程修改原始输入
func copyQuads(quads []semantic.Quad) []semantic.Quad {
	result := make([]semantic.Quad, len(quads))
	copy(result, quads)
	return result
}

// reindexQuads 重新生成连续四元式编号
func reindexQuads(quads []semantic.Quad) []semantic.Quad {
	result := copyQuads(quads)
	for i := range result {
		result[i].Index = i + 1
	}
	return result
}

// isBlockEnd 判断一条四元式是否会结束当前基本块
func isBlockEnd(op string) bool {
	return op == "j" || op == "jfalse" || op == "return" || op == "end"
}

// isExpressionOp 判断操作符是否属于表达式计算
func isExpressionOp(op string) bool {
	return isBinaryOp(op) || isUnaryOp(op)
}

// isBinaryOp 判断操作符是否为二元运算
func isBinaryOp(op string) bool {
	switch op {
	case "+", "-", "*", "/", "%", "<", "<=", ">", ">=", "==", "!=", "&&", "||", "&", "|", "^", "&^", "<<", ">>":
		return true
	default:
		return false
	}
}

// isUnaryOp 判断操作符是否为一元运算
func isUnaryOp(op string) bool {
	return op == "uminus" || op == "uplus" || op == "!"
}

// hasAssignedResult 判断四元式是否会给 Result 赋新值
func hasAssignedResult(q semantic.Quad) bool {
	return q.Result != "" && q.Result != "_" && (q.Op == "=" || isExpressionOp(q.Op))
}

// canDeleteAssignment 判断一条赋值是否允许被死代码删除处理
func canDeleteAssignment(q semantic.Quad) bool {
	if q.Result == "" || q.Result == "_" {
		return false
	}
	if strings.Contains(q.Result, "[") || strings.Contains(q.Result, ".") {
		// 数组元素和结构体字段写入可能有副作用，保守不删
		return false
	}
	return q.Op == "=" || isExpressionOp(q.Op)
}

// expressionKey 生成表达式唯一键，用来判断公共子表达式
func expressionKey(q semantic.Quad) string {
	arg1 := q.Arg1
	arg2 := q.Arg2
	if isCommutative(q.Op) && arg1 > arg2 {
		// 可交换运算中 a+b 和 b+a 视为同一个表达式
		arg1, arg2 = arg2, arg1
	}
	return q.Op + "|" + arg1 + "|" + arg2
}

// isCommutative 判断操作符是否满足交换律
func isCommutative(op string) bool {
	return op == "+" || op == "*" || op == "==" || op == "!=" ||
		op == "&&" || op == "||" || op == "&" || op == "|" || op == "^"
}

// clearExprsByName 删除所有依赖某个变量的表达式缓存
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

// evalQuad 尝试在编译期计算一条表达式四元式
func evalQuad(q semantic.Quad) (string, bool) {
	if isBinaryOp(q.Op) {
		return evalBinary(q.Op, q.Arg1, q.Arg2)
	}
	if isUnaryOp(q.Op) {
		return evalUnary(q.Op, q.Arg1)
	}
	return "", false
}

// evalBinary 计算二元常量表达式
func evalBinary(op string, left string, right string) (string, bool) {
	if op == "&&" || op == "||" {
		// 逻辑运算只接受 true 和 false
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
		// 位运算只接受整数常量
		leftInt, ok1 := parseInt(left)
		rightInt, ok2 := parseInt(right)
		if !ok1 || !ok2 {
			return "", false
		}
		return evalIntBinary(op, leftInt, rightInt)
	}

	if !strings.Contains(left, ".") && !strings.Contains(right, ".") {
		// 两边都是整数文本时优先用整数运算，避免结果变成小数形式
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

// evalNormalIntBinary 计算普通整数算术和比较
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

// evalIntBinary 计算整数位运算和移位运算
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

// evalUnary 计算一元常量表达式
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

// parseBool 解析布尔常量文本
func parseBool(text string) (bool, bool) {
	if text == "true" {
		return true, true
	}
	if text == "false" {
		return false, true
	}
	return false, false
}

// formatBool 把布尔值转回源语言中的文本
func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// parseInt 解析整数文本
func parseInt(text string) (int64, bool) {
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

// parseNumber 解析整数或浮点数文本
func parseNumber(text string) (float64, bool) {
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil
}

// formatNumber 把常量折叠结果格式化成合适的文本
func formatNumber(value float64, left string, right string) string {
	if !strings.Contains(left, ".") && !strings.Contains(right, ".") && value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// collectUserNames 收集用户变量名，用于保护最终可能需要的结果
func collectUserNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		addUserName(names, q.Arg1)
		addUserName(names, q.Arg2)
		addUserName(names, q.Result)
	}
	return names
}

// collectInitialLiveNames 计算死代码删除的初始活跃集合
func collectInitialLiveNames(quads []semantic.Quad) map[string]bool {
	names := collectUserNames(quads)
	for _, q := range quads {
		addLiveName(names, q.Arg1)
		addLiveName(names, q.Arg2)
	}
	return names
}

// copyNameSet 复制名字集合
func copyNameSet(names map[string]bool) map[string]bool {
	result := map[string]bool{}
	for name := range names {
		result[name] = true
	}
	return result
}

// addUsedNames 把一条四元式读取到的名字加入活跃集合
func addUsedNames(names map[string]bool, q semantic.Quad) {
	addLiveName(names, q.Arg1)
	addLiveName(names, q.Arg2)
	if q.Op != "=" && strings.Contains(q.Result, "[") {
		addLiveName(names, q.Result)
	}
}

// addUserName 收集用户写出来的变量名，不包含临时变量和标签
func addUserName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isTempName(name) || isLabelName(name) {
		return
	}
	names[name] = true
}

// addLiveName 把一个操作数中的名字加入活跃集合
func addLiveName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isLabelName(name) {
		return
	}
	names[name] = true
}

// baseName 从数组访问或字段访问中取出基础变量名
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

// hasComplexName 判断名字中是否包含数组下标或结构体字段访问
func hasComplexName(text string) bool {
	return strings.Contains(text, "[") || strings.Contains(text, ".")
}

// isLiteral 判断文本是否为常量或占位符
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

// isTempName 判断名字是否为编译器生成的临时变量
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

// isLabelName 判断名字是否为编译器生成的标签
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
