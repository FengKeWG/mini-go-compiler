package codegen

import (
	"sort"
	"strconv"
	"strings"

	"minigo/internal/optimizer"
	"minigo/internal/semantic"
)

// Instruction 表示一条目标代码指令。
type Instruction struct {
	Index int
	Op    string
	Arg1  string
	Arg2  string
}

// LiveBlock 表示一个基本块的活跃信息摘要。
type LiveBlock struct {
	BlockIndex int
	Start      int
	End        int
	Entry      []string
	Exit       []string
}

// Result 保存目标代码生成阶段的全部输出。
type Result struct {
	InstructionSet []string
	LiveBlocks     []LiveBlock
	Instructions   []Instruction
}

// Generate 根据优化后的四元式生成活跃信息和目标代码。
func Generate(quads []semantic.Quad) Result {
	return Result{
		InstructionSet: buildInstructionSet(),
		LiveBlocks:     BuildLiveInfo(quads),
		Instructions:   Translate(quads),
	}
}

// BuildLiveInfo 在每个基本块内倒序计算变量活跃信息。
func BuildLiveInfo(quads []semantic.Quad) []LiveBlock {
	blocks := optimizer.BuildBasicBlocks(quads)
	nonTempNames := collectNonTempNames(quads)
	var liveBlocks []LiveBlock

	for _, block := range blocks {
		live := copyNameSet(nonTempNames)
		exit := sortedNames(live)

		for i := len(block.Quads) - 1; i >= 0; i-- {
			q := block.Quads[i]

			if defName := definedName(q); defName != "" {
				delete(live, defName)
			}
			for _, name := range usedNames(q) {
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

// Translate 把四元式翻译为课件风格的目标代码。
func Translate(quads []semantic.Quad) []Instruction {
	var instructions []Instruction
	for _, q := range quads {
		switch q.Op {
		case "program":
			instructions = appendInstruction(instructions, "PROC", "_", q.Arg1)
		case "end":
			instructions = appendInstruction(instructions, "END", "_", q.Arg1)
		case "label":
			instructions = appendInstruction(instructions, "LABEL", "_", q.Result)
		case "=":
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "j":
			instructions = appendInstruction(instructions, "JMP", "_", q.Result)
		case "jfalse":
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "FJ", "R0", q.Result)
		case "param":
			instructions = appendInstruction(instructions, "PARAM", "_", q.Arg1)
		case "call":
			instructions = appendInstruction(instructions, "CALL", "_", q.Arg1)
			if q.Result != "_" {
				instructions = appendInstruction(instructions, "ST", "R0", q.Result)
			}
		case "return":
			if q.Arg1 != "_" {
				instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
				instructions = appendInstruction(instructions, "RET", "R0", "_")
			} else {
				instructions = appendInstruction(instructions, "RET", "_", "_")
			}
		case "uminus":
			instructions = appendInstruction(instructions, "LD", "R0", "0")
			instructions = appendInstruction(instructions, "SUB", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "uplus":
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		case "!":
			instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
			instructions = appendInstruction(instructions, "NO", "R0", "_")
			instructions = appendInstruction(instructions, "ST", "R0", q.Result)
		default:
			if op := targetOp(q.Op); op != "" {
				instructions = appendInstruction(instructions, "LD", "R0", q.Arg1)
				instructions = appendInstruction(instructions, op, "R0", q.Arg2)
				instructions = appendInstruction(instructions, "ST", "R0", q.Result)
			}
		}
	}
	return instructions
}

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

func appendInstruction(instructions []Instruction, op string, arg1 string, arg2 string) []Instruction {
	return append(instructions, Instruction{
		Index: len(instructions) + 1,
		Op:    op,
		Arg1:  arg1,
		Arg2:  arg2,
	})
}

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

func allNames(q semantic.Quad) []string {
	if q.Op == "program" || q.Op == "end" || q.Op == "label" || q.Op == "j" {
		return nil
	}
	if q.Op == "call" {
		name := baseName(q.Result)
		if name == "" || isLiteral(q.Result) || isLabelName(name) {
			return nil
		}
		return []string{name}
	}
	names := []string{}
	for _, text := range []string{q.Arg1, q.Arg2, q.Result} {
		name := baseName(text)
		if name != "" && !isLiteral(text) && !isLabelName(name) {
			names = append(names, name)
		}
	}
	return names
}

func usedNames(q semantic.Quad) []string {
	if q.Op == "program" || q.Op == "end" || q.Op == "label" || q.Op == "j" {
		return nil
	}
	if q.Op == "call" {
		return nil
	}
	names := []string{}
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
		name := baseName(q.Result)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func definedName(q semantic.Quad) string {
	if q.Result == "" || q.Result == "_" || strings.Contains(q.Result, "[") || strings.Contains(q.Result, ".") {
		return ""
	}
	if q.Op == "=" || q.Op == "call" || isTargetExpression(q.Op) {
		return baseName(q.Result)
	}
	return ""
}

func isTargetExpression(op string) bool {
	return targetOp(op) != "" || op == "uminus" || op == "uplus" || op == "!"
}

func copyNameSet(names map[string]bool) map[string]bool {
	result := map[string]bool{}
	for name := range names {
		result[name] = true
	}
	return result
}

func sortedNames(names map[string]bool) []string {
	result := []string{}
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
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
	if text == "" || text == "_" || text == "true" || text == "false" {
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
