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
	Start int             // 基本块第一条四元式编号，用来在界面里显示范围
	End   int             // 基本块最后一条四元式编号，用来在界面里显示范围
	Quads []semantic.Quad // 基本块内包含的四元式，优化时按块处理
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
	Result string // 已经计算过的结果名，例如 t1
	Arg1   string // 表达式第一个操作数，用于清除失效表达式
	Arg2   string // 表达式第二个操作数，用于清除失效表达式
}

// Optimize 对语义分析生成的四元式做基础优化
func Optimize(quads []semantic.Quad) Result {
	// 保留原四元式，方便界面展示优化前后对比
	original := copyQuads(quads)
	// 基本块划分是优化和活跃分析都要用的基础信息
	blocks := BuildBasicBlocks(original)

	// 第一步做常量表达式节省，把编译期能算出的表达式提前算好
	folded, foldCount := foldConstants(original)
	// 第二步做公共子表达式节省，复用同一基本块里已经算过的表达式
	commonSaved, commonCount := saveCommonExpressions(folded)
	// 第三步做循环优化，把循环中不变的临时表达式移到循环入口前
	loopMoved, loopCount := optimizeLoops(commonSaved)
	// 第四步删除无用赋值，减少不会被后续使用的中间结果
	deadRemoved, deadCount := removeDeadAssignments(loopMoved)
	// 删除和移动后重新编号，让输出的四元式序号连续
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
	// result 是当前正在优化的四元式列表
	result := copyQuads(quads)
	// count 统计被外提的四元式条数
	count := 0

	for {
		// changed 表示这一轮有没有找到可以外提的表达式
		// 如果没有变化，说明循环优化已经稳定
		changed := false
		// labelIndex 用于从 j 指令快速找到回跳目标
		labelIndex := buildLabelIndex(result)
		for i, q := range result {
			// 循环结尾通常是一条无条件跳转，跳回循环入口标签
			if q.Op != "j" {
				continue
			}
			// start 是这条跳转目标标签所在的四元式下标
			start, ok := labelIndex[q.Result]
			if !ok || start >= i {
				// 目标不存在或不是向前跳转，就不是这里处理的循环回边
				continue
			}

			// 一个向前面的标签跳转的 j，通常表示循环回边
			bodyStart := findLoopBodyStart(result, start, i)
			if bodyStart < 0 {
				// 找不到条件跳转时，说明这个回边结构不符合当前简化循环模型
				continue
			}
			// indexes 是循环体内可以移动到循环入口前的四元式下标
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
	// 第一条四元式一定是入口
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
	// 入口下标排序后，相邻两个入口之间就是一个基本块
	sort.Ints(indexes)

	var blocks []Block
	for i, start := range indexes {
		// 默认最后一个基本块一直延伸到四元式末尾
		end := len(quads) - 1
		if i+1 < len(indexes) {
			// 当前基本块结束于下一个入口前一条四元式
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
	// result 是折叠后的四元式副本
	result := copyQuads(quads)
	// count 统计成功折叠的表达式条数
	count := 0
	for i, q := range result {
		// 只有两个操作数都是常量时才可以在编译期求值
		value, ok := evalQuad(q)
		if !ok {
			// 不能求值说明操作数里有变量或操作本身不适合折叠
			continue
		}
		// 折叠后保留原来的 Result，只把操作改成直接赋值
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
	// 公共子表达式只在基本块内做，避免跨跳转路径产生错误复用
	blocks := BuildBasicBlocks(quads)
	var result []semantic.Quad
	count := 0

	for _, block := range blocks {
		// exprs 记录当前基本块内还有效的表达式
		// key 是表达式内容，value 是第一次计算这个表达式的结果
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
					// 例如 t2 又要算 a+b，可以改成 t2=t1
					q.Op = "="
					q.Arg1 = old.Result
					q.Arg2 = "_"
					count++
				} else {
					// 第一次见到这个表达式时，把它保存起来
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
	// 删除无用赋值也按基本块处理，规则简单且不容易误删跨块结果
	blocks := BuildBasicBlocks(quads)
	// 用户变量默认认为在基本块出口仍可能被使用，避免误删最终结果
	initialLive := collectInitialLiveNames(quads)
	var result []semantic.Quad
	count := 0

	for _, block := range blocks {
		// live 表示从当前扫描位置往后还可能被使用的变量
		live := copyNameSet(initialLive)
		// keep 标记每条四元式是否保留
		keep := make([]bool, len(block.Quads))

		// 从后往前扫描，符合活跃变量分析的基本做法
		for i := len(block.Quads) - 1; i >= 0; i-- {
			q := block.Quads[i]
			if hasAssignedResult(q) && canDeleteAssignment(q) {
				name := baseName(q.Result)
				if !live[name] {
					// 结果后面没有再用到，这条赋值可以删除
					// 例如 t3 后面从未出现，就不需要生成 t3
					count++
					continue
				}
				// 当前四元式定义了 name，往前看时 name 不再活跃
				delete(live, name)
				addUsedNames(live, q)
				keep[i] = true
				continue
			}

			// 不是普通赋值的四元式一般保守保留
			// 同时把它读取到的变量加入活跃集合
			addUsedNames(live, q)
			keep[i] = true
		}

		// 按原顺序收集需要保留的四元式
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
			// label 的 Result 字段保存标签名，例如 L1
			// 记录切片下标后，跳转指令就能快速找到目标位置
			labelIndex[q.Result] = i
		}
	}
	return labelIndex
}

// findLoopBodyStart 找到 jfalse 后面的第一条循环体四元式
func findLoopBodyStart(quads []semantic.Quad, start int, end int) int {
	// 当前生成的 for 循环大致形状是 label 条件 jfalse 循环体 j
	// start 是 label 的位置，end 是回跳 j 的位置
	for i := start + 1; i < end; i++ {
		if quads[i].Op == "jfalse" {
			// jfalse 后面第一条就是循环体开始
			return i + 1
		}
	}
	return -1
}

// findLoopInvariants 找出循环体中可以外提的循环不变表达式
func findLoopInvariants(quads []semantic.Quad, bodyStart int, bodyEnd int) []int {
	// assigned 表示循环体内会被改写的名字
	// 如果某个变量会在循环里被赋值，那么依赖它的表达式不能算作不变
	assigned := collectAssignedNames(quads[bodyStart:bodyEnd])
	// invariantTemps 表示已经确认是不变表达式结果的临时变量
	// 这样 t1 是不变表达式时，后面依赖 t1 的 t2 也可能是不变表达式
	invariantTemps := map[string]bool{}
	var indexes []int

	for i := bodyStart; i < bodyEnd; i++ {
		q := quads[i]
		if !canHoist(q) {
			// 不是安全的表达式就不参与外提
			continue
		}
		if !isLoopInvariantValue(q.Arg1, assigned, invariantTemps) {
			// 第一个操作数会随循环变化，不能外提
			continue
		}
		if !isLoopInvariantValue(q.Arg2, assigned, invariantTemps) {
			// 第二个操作数会随循环变化，不能外提
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
		// 只有表达式计算才考虑外提，跳转和赋值不外提
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
	// 带数组下标或结构体字段的名字可能涉及别名和地址变化，保守不外提
	return !hasComplexName(q.Arg1) && !hasComplexName(q.Arg2) && !hasComplexName(q.Result)
}

// isLoopInvariantValue 判断一个操作数在当前循环中是否保持不变
func isLoopInvariantValue(text string, assigned map[string]bool, invariantTemps map[string]bool) bool {
	name := baseName(text)
	if name == "" || isLiteral(text) {
		// 常量和空占位符天然不随循环变化
		return true
	}
	if isLabelName(name) || hasComplexName(text) {
		// 标签不是普通数据，复杂名字保守认为可能变化
		return false
	}
	if isTempName(name) {
		// 临时变量要么本身不在循环中被赋值，要么来自前面已经确认的不变表达式
		return invariantTemps[name] || !assigned[name]
	}
	// 普通变量只要没有在循环体内被赋值，就认为循环不变
	return !assigned[name]
}

// collectAssignedNames 收集一段四元式中被赋值的名字
func collectAssignedNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		if hasAssignedResult(q) {
			name := baseName(q.Result)
			if name != "" {
				// 只记录基础名字，数组下标和字段访问会被简化成基础变量
				names[name] = true
			}
		}
	}
	return names
}

// moveQuadsBefore 把若干条四元式按原顺序移动到指定位置前面
func moveQuadsBefore(quads []semantic.Quad, indexes []int, before int) []semantic.Quad {
	// moved 用来标记哪些四元式已经被外提前移
	moved := map[int]bool{}
	for _, index := range indexes {
		moved[index] = true
	}

	var result []semantic.Quad
	// 先复制移动目标位置之前的四元式
	for i := 0; i < before; i++ {
		result = append(result, quads[i])
	}
	// 再按原顺序插入要外提的四元式
	for _, index := range indexes {
		result = append(result, quads[index])
	}
	// 最后复制剩余四元式，并跳过已经移动过的部分
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
	// 跳转、条件跳转、返回和函数结束都会改变顺序执行流
	return op == "j" || op == "jfalse" || op == "return" || op == "end"
}

// isExpressionOp 判断操作符是否属于表达式计算
func isExpressionOp(op string) bool {
	// 表达式运算包括二元运算和一元运算
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
	// 只有赋值和表达式计算会定义一个新结果
	return q.Result != "" && q.Result != "_" && (q.Op == "=" || isExpressionOp(q.Op))
}

// canDeleteAssignment 判断一条赋值是否允许被死代码删除处理
func canDeleteAssignment(q semantic.Quad) bool {
	if q.Result == "" || q.Result == "_" {
		// 没有实际结果位置的四元式不能按赋值删除
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
	// 使用操作符和两个操作数组成 key，保证同一表达式能命中缓存
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
			// 表达式结果或操作数涉及这个变量时，说明缓存已经不可靠
			delete(exprs, key)
		}
	}
}

// evalQuad 尝试在编译期计算一条表达式四元式
func evalQuad(q semantic.Quad) (string, bool) {
	if isBinaryOp(q.Op) {
		// 二元运算需要同时检查两个操作数是不是常量
		return evalBinary(q.Op, q.Arg1, q.Arg2)
	}
	if isUnaryOp(q.Op) {
		// 一元运算只需要检查第一个操作数
		return evalUnary(q.Op, q.Arg1)
	}
	// 非表达式四元式不能做常量折叠
	return "", false
}

// evalBinary 计算二元常量表达式
func evalBinary(op string, left string, right string) (string, bool) {
	if op == "&&" || op == "||" {
		// 逻辑运算只接受 true 和 false
		leftBool, ok1 := parseBool(left)
		rightBool, ok2 := parseBool(right)
		if !ok1 || !ok2 {
			// 只要有一边不是布尔常量，就不能折叠
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
			// 位运算中出现非整数常量时不折叠
			return "", false
		}
		return evalIntBinary(op, leftInt, rightInt)
	}

	if !strings.Contains(left, ".") && !strings.Contains(right, ".") {
		// 两边都是整数文本时优先用整数运算，避免结果变成小数形式
		leftInt, ok1 := parseInt(left)
		rightInt, ok2 := parseInt(right)
		if ok1 && ok2 {
			// 纯整数表达式保持整数结果
			return evalNormalIntBinary(op, leftInt, rightInt)
		}
	}

	// 走到这里说明可能是浮点运算或浮点比较
	leftFloat, ok1 := parseNumber(left)
	rightFloat, ok2 := parseNumber(right)
	if !ok1 || !ok2 {
		// 有变量或字符串字符常量时不能当成数字计算
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
			// 除零不在编译期折叠，保留给后续阶段处理
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
			// 整数除零不折叠，避免编译器自身报错
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
			// 取模除数为零时不能折叠
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
			// 负数移位没有在这里定义，保守不折叠
			return "", false
		}
		return strconv.FormatInt(left<<uint(right), 10), true
	case ">>":
		if right < 0 {
			// 负数移位没有在这里定义，保守不折叠
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
			// 逻辑非只折叠布尔常量
			return "", false
		}
		return formatBool(!boolValue), true
	}

	number, ok := parseNumber(value)
	if !ok {
		// 正号和负号只折叠数字常量
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
		// 第一个返回值是真实布尔值，第二个返回值表示解析成功
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
	// 第二个返回值表示是否解析成功
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

// parseNumber 解析整数或浮点数文本
func parseNumber(text string) (float64, bool) {
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		// 字符串常量和字符常量不能参与数字折叠
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil
}

// formatNumber 把常量折叠结果格式化成合适的文本
func formatNumber(value float64, left string, right string) string {
	if !strings.Contains(left, ".") && !strings.Contains(right, ".") && value == float64(int64(value)) {
		// 原操作数都是整数且结果也是整数时，输出整数文本
		return strconv.FormatInt(int64(value), 10)
	}
	// 其他情况输出浮点文本，并去掉多余的零
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// collectUserNames 收集用户变量名，用于保护最终可能需要的结果
func collectUserNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		// 用户变量可能出现在两个操作数或结果位置
		addUserName(names, q.Arg1)
		addUserName(names, q.Arg2)
		addUserName(names, q.Result)
	}
	return names
}

// collectInitialLiveNames 计算死代码删除的初始活跃集合
func collectInitialLiveNames(quads []semantic.Quad) map[string]bool {
	// 先把用户变量加入集合，避免把最终结果变量删掉
	names := collectUserNames(quads)
	for _, q := range quads {
		// 再把所有被读取过的名字加入集合
		// 这样某些跨基本块使用的临时变量也能被保守保护
		addLiveName(names, q.Arg1)
		addLiveName(names, q.Arg2)
	}
	return names
}

// copyNameSet 复制名字集合
func copyNameSet(names map[string]bool) map[string]bool {
	result := map[string]bool{}
	for name := range names {
		// map 是引用类型，优化每个基本块时必须复制一份
		result[name] = true
	}
	return result
}

// addUsedNames 把一条四元式读取到的名字加入活跃集合
func addUsedNames(names map[string]bool, q semantic.Quad) {
	// 普通四元式读取 Arg1 和 Arg2
	addLiveName(names, q.Arg1)
	addLiveName(names, q.Arg2)
	if q.Op != "=" && strings.Contains(q.Result, "[") {
		// 写数组元素时，数组基础名也需要保持活跃
		addLiveName(names, q.Result)
	}
}

// addUserName 收集用户写出来的变量名，不包含临时变量和标签
func addUserName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isTempName(name) || isLabelName(name) {
		// 常量、临时变量、标签都不是用户变量
		return
	}
	names[name] = true
}

// addLiveName 把一个操作数中的名字加入活跃集合
func addLiveName(names map[string]bool, text string) {
	name := baseName(text)
	if name == "" || isLiteral(text) || isLabelName(name) {
		// 常量和标签不参与变量活跃分析
		return
	}
	names[name] = true
}

// baseName 从数组访问或字段访问中取出基础变量名
func baseName(text string) string {
	if text == "" || text == "_" {
		// 空字符串和下划线都是占位符
		return ""
	}
	name := text
	if index := strings.Index(name, "["); index >= 0 {
		// scores[i] 的基础名是 scores
		name = name[:index]
	}
	if index := strings.Index(name, "."); index >= 0 {
		// stu 的 age 字段访问基础名是 stu
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
		// 空字符串和下划线表示没有实际变量
		return true
	}
	if text == "true" || text == "false" {
		// 布尔字面量不是变量
		return true
	}
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		// 字符串和字符字面量不是变量
		return true
	}
	// 能被解析成数字的文本也不是变量
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// isTempName 判断名字是否为编译器生成的临时变量
func isTempName(name string) bool {
	if len(name) < 2 || name[0] != 't' {
		// 临时变量统一命名为 t 加数字
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			// t 后面只要出现非数字，就不是临时变量
			return false
		}
	}
	return true
}

// isLabelName 判断名字是否为编译器生成的标签
func isLabelName(name string) bool {
	if len(name) < 2 || name[0] != 'L' {
		// 标签统一命名为 L 加数字
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			// L 后面只要出现非数字，就不是标签
			return false
		}
	}
	return true
}
