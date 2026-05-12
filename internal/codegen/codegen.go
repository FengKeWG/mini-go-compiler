package codegen

import (
	"sort"
	"strconv"
	"strings"

	"minigo/internal/optimizer"
	"minigo/internal/semantic"
)

// Instruction 表示一条目标代码指令
type Instruction struct {
	Index int    // 指令编号
	Op    string // 目标指令操作码，例如 LD、ST、ADD、JMP
	Arg1  string // 第一个目标指令操作数
	Arg2  string // 第二个目标指令操作数
}

// LiveBlock 表示一个基本块的活跃信息摘要
type LiveBlock struct {
	BlockIndex int      // 基本块编号
	Start      int      // 基本块第一条四元式编号
	End        int      // 基本块最后一条四元式编号
	Entry      []string // 基本块入口活跃变量
	Exit       []string // 基本块出口活跃变量
}

// Result 保存目标代码生成阶段的全部输出
type Result struct {
	InstructionSet []string      // 当前虚拟目标机支持的指令集合
	LiveBlocks     []LiveBlock   // 基本块活跃信息
	Instructions   []Instruction // 由四元式翻译出的目标代码
}

// Generate 根据优化后的四元式生成活跃信息和目标代码
func Generate(quads []semantic.Quad) Result {
	return Result{
		InstructionSet: buildInstructionSet(),
		LiveBlocks:     BuildLiveInfo(quads),
		Instructions:   Translate(quads),
	}
}

// BuildLiveInfo 在每个基本块内倒序计算变量活跃信息
func BuildLiveInfo(quads []semantic.Quad) []LiveBlock {
	blocks := optimizer.BuildBasicBlocks(quads)
	// 非临时变量默认作为可能对外有用的变量
	nonTempNames := collectNonTempNames(quads)
	var liveBlocks []LiveBlock

	for _, block := range blocks {
		// live 表示从当前位置往后还可能被使用的变量集合
		live := copyNameSet(nonTempNames)
		exit := sortedNames(live)

		// 活跃信息从基本块末尾向前推导
		for i := len(block.Quads) - 1; i >= 0; i-- {
			q := block.Quads[i]

			if defName := definedName(q); defName != "" {
				// 当前四元式定义了变量，继续向前看时该旧值不再活跃
				delete(live, defName)
			}
			for _, name := range usedNames(q) {
				// 当前四元式使用了变量，继续向前看时该变量必须活跃
				live[name] = true
			}
		}

		liveBlocks = append(liveBlocks, LiveBlock{
			BlockIndex: block.Index,
			Start:      block.Start,
			End:        block.End,
			Entry:      sortedNames(live),
			Exit:       exit,
		})
	}

	return liveBlocks
}

// Translate 把四元式翻译为目标代码
func Translate(quads []semantic.Quad) []Instruction {
	var instructions []Instruction
	for _, q := range quads {
		switch q.Op {
		case "program":
			// 函数开始，对应过程入口
			instructions = appendInstruction(instructions, "PROC", "_", q.Arg1)
		case "end":
			// 函数结束，对应过程出口
			instructions = appendInstruction(instructions, "END", "_", q.Arg1)
		case "label":
			// 标签直接变成目标代码中的 LABEL
			instructions = appendInstruction(instructions, "LABEL", "_", q.Result)
		case "=":
			// 赋值先把右值取到 R0，再把 R0 存到目标位置
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "j":
			// 无条件跳转
			instructions = appendInstruction(instructions, "JMP", "_", q.Result)
		case "jfalse":
			// 条件为 false 时跳转
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "FJ", "R0", q.Result)
		case "param":
			// 参数压入调用序列
			instructions = appendInstruction(instructions, "PARAM", "_", q.Arg1)
		case "call":
			// 调用后默认返回值放在 R0
			instructions = appendInstruction(instructions, "CALL", "_", q.Arg1)
			if q.Result != "_" {
				instructions = appendInstruction(instructions, "ST", "R0", q.Result)
			}
		case "return":
			// 有返回值时先放入 R0，再执行 RET
			if q.Arg1 != "_" {
				instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
				instructions = appendInstruction(instructions, "RET", "R0", "_")
			} else {
				instructions = appendInstruction(instructions, "RET", "_", "_")
			}
		case "uminus":
			// 一元负号等价于 0 - x
			instructions = appendInstruction(instructions, "LD", "R0", "0")
			instructions = appendInstruction(instructions, "SUB", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "uplus":
			// 一元正号直接复制值
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "!":
			// 逻辑非使用 NO 指令
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "NO", "R0", "_")
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		default:
			if op := targetOp(q.Op); op != "" {
				// 普通二元运算统一翻译为 LD、运算、ST 三条指令
				instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
				instructions = appendInstruction(instructions, op, "R0", q.Arg2)
				instructions = appendInstruction(instructions, "ST", "R0", q.Result)
			}
		}
	}
	return instructions
}

// buildInstructionSet 返回课设虚拟目标机支持的指令说明
func buildInstructionSet() []string {
	return []string{
		"LD Ri, X      Ri := X",
		"ST Ri, X      X := Ri",
		"ADD Ri, X     Ri := Ri + X",
		"SUB Ri, X     Ri := Ri - X",
		"MUL Ri, X     Ri := Ri * X",
		"DIV Ri, X     Ri := Ri / X",
		"FJ Ri, L      Ri 为 false 时跳转到 L",
		"TJ Ri, L      Ri 为 true 时跳转到 L",
		"JMP _, L      无条件跳转到 L",
		"LT/GT/EQ/LE/GE/NE Ri, X  关系运算",
		"AND/OR/NO Ri, X          逻辑运算",
		"MOD/BAND/BOR/XOR/BCLR/SHL/SHR Ri, X  扩展位运算",
		"PARAM/CALL               参数传递和函数调用",
		"PROC/END/RET/LABEL       过程、返回和标号",
	}
}

// appendInstruction 追加一条目标指令并自动生成编号
func appendInstruction(instructions []Instruction, op string, arg1 string, arg2 string) []Instruction {
	return append(instructions, Instruction{
		Index: len(instructions) + 1,
		Op:    op,
		Arg1:  arg1,
		Arg2:  arg2,
	})
}

// targetOp 把四元式操作符映射到目标机操作码
func targetOp(op string) string {
	switch op {
	case "+":
		return "ADD"
	case "-":
		return "SUB"
	case "*":
		return "MUL"
	case "/":
		return "DIV"
	case "%":
		return "MOD"
	case "<":
		return "LT"
	case ">":
		return "GT"
	case "==":
		return "EQ"
	case "<=":
		return "LE"
	case ">=":
		return "GE"
	case "!=":
		return "NE"
	case "&&":
		return "AND"
	case "||":
		return "OR"
	case "&":
		return "BAND"
	case "|":
		return "BOR"
	case "^":
		return "XOR"
	case "&^":
		return "BCLR"
	case "<<":
		return "SHL"
	case ">>":
		return "SHR"
	default:
		return ""
	}
}

// collectNonTempNames 收集所有非临时变量名
func collectNonTempNames(quads []semantic.Quad) map[string]bool {
	names := map[string]bool{}
	for _, q := range quads {
		for _, name := range allNames(q) {
			if !isTempName(name) {
				names[name] = true
			}
		}
	}
	return names
}

// allNames 收集一条四元式中出现的所有变量名
func allNames(q semantic.Quad) []string {
	if q.Op == "program" || q.Op == "end" || q.Op == "label" || q.Op == "j" {
		return nil
	}
	if q.Op == "call" {
		// 函数名不是变量，call 只关心返回结果
		name := baseName(q.Result)
		if name == "" || isLiteral(q.Result) || isLabelName(name) {
			return nil
		}
		return []string{name}
	}
	var names []string
	for _, text := range []string{q.Arg1, q.Arg2, q.Result} {
		name := baseName(text)
		if name != "" && !isLiteral(text) && !isLabelName(name) {
			names = append(names, name)
		}
	}
	return names
}

// usedNames 收集一条四元式读取到的变量名
func usedNames(q semantic.Quad) []string {
	if q.Op == "program" || q.Op == "end" || q.Op == "label" || q.Op == "j" {
		return nil
	}
	if q.Op == "call" {
		return nil
	}
	var names []string
	for _, text := range []string{q.Arg1, q.Arg2} {
		name := baseName(text)
		if name != "" && !isLiteral(text) && !isLabelName(name) {
			names = append(names, name)
		}
	}
	if q.Op == "jfalse" || q.Op == "return" {
		name := baseName(q.Arg1)
		if name != "" && !isLiteral(q.Arg1) {
			names = append(names, name)
		}
	}
	if strings.Contains(q.Result, "[") || strings.Contains(q.Result, ".") {
		// 写数组元素或字段时，基础对象本身也算被使用
		name := baseName(q.Result)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// definedName 返回一条四元式定义的变量名
func definedName(q semantic.Quad) string {
	if q.Result == "" || q.Result == "_" || strings.Contains(q.Result, "[") || strings.Contains(q.Result, ".") {
		return ""
	}
	if q.Op == "=" || q.Op == "call" || isTargetExpression(q.Op) {
		return baseName(q.Result)
	}
	return ""
}

// isTargetExpression 判断操作符是否能翻译成目标机表达式指令
func isTargetExpression(op string) bool {
	return targetOp(op) != "" || op == "uminus" || op == "uplus" || op == "!"
}

// copyNameSet 复制变量名集合
func copyNameSet(names map[string]bool) map[string]bool {
	result := map[string]bool{}
	for name := range names {
		result[name] = true
	}
	return result
}

// sortedNames 把集合转换为有序切片，保证每次输出顺序稳定
func sortedNames(names map[string]bool) []string {
	var result []string
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// baseName 从数组访问或字段访问中提取基础变量名
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

// isLiteral 判断文本是否为常量或占位符
func isLiteral(text string) bool {
	if text == "" || text == "_" || text == "true" || text == "false" {
		return true
	}
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return true
	}
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// isTempName 判断名字是否为临时变量
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

// isLabelName 判断名字是否为跳转标签
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
