package vm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"minigo/internal/codegen"
	"minigo/internal/semantic"
)

// Value 表示运行平台中的一个运行时值
type Value struct {
	Data any // 真正保存的值，可能是整数、浮点数、布尔值或字符串
}

// Variable 表示运行结束后需要展示的变量
type Variable struct {
	Name  string // 变量名
	Value string // 变量当前值
}

// Trace 表示目标指令执行过程中的一步
type Trace struct {
	Step        int    // 执行步数
	PC          int    // 当前执行到的指令下标
	Instruction string // 当前执行的目标指令文本
	R0          string // 执行后寄存器 R0 的值
}

// Result 保存目标指令运行平台的执行结果
type Result struct {
	ReturnValue string     // main 函数返回值
	Variables   []Variable // main 函数运行结束后的变量表
	Trace       []Trace    // 指令执行轨迹
	Errors      []string   // 运行错误
}

type frame struct {
	name string           // 当前函数名
	vars map[string]Value // 当前函数的局部变量、参数、临时变量和常量
}

type callItem struct {
	returnPC int   // 函数返回后继续执行的指令下标
	frame    frame // 调用者的活动记录
}

type machine struct {
	instructions []codegen.Instruction       // 目标指令序列
	labels       map[string]int              // 标签名到指令下标的映射
	procs        map[string]int              // 函数名到函数入口指令下标的映射
	params       map[string][]string         // 函数名到形参名列表的映射
	defaults     map[string]map[string]Value // 每个函数活动记录的初始变量和值

	pc        int        // 当前执行到的指令下标
	r0        Value      // 工作寄存器 R0
	frame     frame      // 当前函数活动记录
	callStack []callItem // 函数调用栈
	pending   []Value    // CALL 前由 PARAM 收集到的实参值
	trace     []Trace    // 指令执行轨迹
	errors    []string   // 运行错误
	returned  bool       // main 函数是否已经返回
}

const maxRunSteps = 10000

// Run 解释执行目标代码指令
func Run(instructions []codegen.Instruction, symbols []semantic.Symbol) Result {
	m := newMachine(instructions, symbols)
	m.run()
	return m.result()
}

// newMachine 根据目标指令和符号表初始化运行平台
func newMachine(instructions []codegen.Instruction, symbols []semantic.Symbol) *machine {
	m := &machine{
		instructions: instructions,
		labels:       map[string]int{},
		procs:        map[string]int{},
		params:       map[string][]string{},
		defaults:     buildFrameDefaults(symbols),
	}

	for i, inst := range instructions {
		if inst.Op == "LABEL" {
			// LABEL 指令只负责记录位置，后面的 JMP/FJ/TJ 根据标签名跳转
			m.labels[inst.Arg2] = i
		}
		if inst.Op == "PROC" {
			// PROC 后面一条指令才是函数体真正入口
			m.procs[inst.Arg2] = i + 1
		}
	}
	for _, sym := range symbols {
		if sym.Category == "p" {
			// 参数在符号表里形如 add.x，需要拆成函数名 add 和局部名 x
			funcName, localName := splitScopedName(sym.Name)
			m.params[funcName] = append(m.params[funcName], localName)
		}
	}

	m.pc = 0
	if start, ok := m.procs["main"]; ok {
		// 程序默认从 main 函数入口开始运行
		m.pc = start
	}
	m.frame = m.newFrame("main")
	return m
}

// buildFrameDefaults 根据符号表给每个函数准备活动记录初值
func buildFrameDefaults(symbols []semantic.Symbol) map[string]map[string]Value {
	defaults := map[string]map[string]Value{}
	for _, sym := range symbols {
		if sym.Category == "f" || sym.Category == "type" {
			// 函数和类型本身不占运行时变量空间
			continue
		}
		funcName, localName := splitScopedName(sym.Name)
		if defaults[funcName] == nil {
			defaults[funcName] = map[string]Value{}
		}
		if sym.Category == "c" {
			// 常量使用符号表中的初值
			defaults[funcName][localName] = parseLiteral(sym.Value)
		} else {
			// 参数、变量和临时变量先按类型填默认零值
			defaults[funcName][localName] = zeroValue(sym.Type)
		}
	}
	return defaults
}

// newFrame 创建一个函数调用时使用的活动记录
func (m *machine) newFrame(name string) frame {
	vars := map[string]Value{}
	for key, value := range m.defaults[name] {
		// 每次调用都复制一份默认变量，避免不同调用之间互相影响
		vars[key] = value
	}
	return frame{name: name, vars: vars}
}

// run 从当前 PC 开始循环执行目标指令
func (m *machine) run() {
	for step := 1; step <= maxRunSteps; step++ {
		if m.returned || len(m.errors) > 0 {
			return
		}
		if m.pc < 0 || m.pc >= len(m.instructions) {
			return
		}

		inst := m.instructions[m.pc]
		m.execute(inst)
		// 记录执行轨迹，方便 GUI 展示每一步 PC、指令和 R0
		m.trace = append(m.trace, Trace{
			Step:        step,
			PC:          inst.Index,
			Instruction: formatInstruction(inst),
			R0:          m.r0.Text(),
		})
	}
	m.errors = append(m.errors, "目标指令执行超过最大步数，可能存在死循环")
}

// execute 执行一条目标指令
func (m *machine) execute(inst codegen.Instruction) {
	switch inst.Op {
	case "PROC", "LABEL":
		// 过程标记和标签本身不改变数据，只移动到下一条
		m.pc++
	case "END":
		m.executeEnd()
	case "LD":
		// LD 把变量或常量读入 R0
		m.r0 = m.readValue(inst.Arg2)
		m.pc++
	case "ST":
		// ST 把 R0 写回变量或临时变量
		m.writeValue(inst.Arg2, m.r0)
		m.pc++
	case "ADD", "SUB", "MUL", "DIV", "MOD":
		// 算术运算把 R0 和第二操作数计算后仍放回 R0
		m.r0 = evalNumberOp(inst.Op, m.r0, m.readValue(inst.Arg2))
		m.pc++
	case "LT", "GT", "EQ", "LE", "GE", "NE":
		// 比较运算结果是 bool，后续可被 FJ/TJ 使用
		m.r0 = evalCompareOp(inst.Op, m.r0, m.readValue(inst.Arg2))
		m.pc++
	case "AND", "OR":
		// 逻辑运算把操作数转换成 bool 后计算
		m.r0 = evalLogicOp(inst.Op, m.r0, m.readValue(inst.Arg2))
		m.pc++
	case "NO":
		// NO 是逻辑非，对 R0 取反
		m.r0 = Value{Data: !toBool(m.r0)}
		m.pc++
	case "BAND", "BOR", "XOR", "BCLR", "SHL", "SHR":
		// 位运算把操作数转换成整数后计算
		m.r0 = evalBitOp(inst.Op, m.r0, m.readValue(inst.Arg2))
		m.pc++
	case "JMP":
		m.jump(inst.Arg2)
	case "FJ":
		if !toBool(m.r0) {
			m.jump(inst.Arg2)
		} else {
			m.pc++
		}
	case "TJ":
		if toBool(m.r0) {
			m.jump(inst.Arg2)
		} else {
			m.pc++
		}
	case "PARAM":
		// PARAM 在 CALL 前收集实参值
		m.pending = append(m.pending, m.readValue(inst.Arg2))
		m.pc++
	case "CALL":
		m.call(inst.Arg2)
	case "RET":
		m.ret(inst.Arg1)
	default:
		m.errors = append(m.errors, "无法执行的目标指令 "+inst.Op)
	}
}

// executeEnd 处理函数体自然结束
func (m *machine) executeEnd() {
	if len(m.callStack) == 0 {
		// main 函数结束时整个程序结束
		m.returned = true
		return
	}
	// 普通函数没有显式 RET 时也能回到调用者
	item := m.callStack[len(m.callStack)-1]
	m.callStack = m.callStack[:len(m.callStack)-1]
	m.frame = item.frame
	m.pc = item.returnPC
}

// jump 根据标签名修改 PC
func (m *machine) jump(label string) {
	target, ok := m.labels[label]
	if !ok {
		m.errors = append(m.errors, "找不到跳转标签 "+label)
		return
	}
	m.pc = target
}

// call 根据函数名创建被调函数活动记录
func (m *machine) call(name string) {
	start, ok := m.procs[name]
	if !ok {
		m.errors = append(m.errors, "找不到函数入口 "+name)
		return
	}

	callee := m.newFrame(name)
	for i, paramName := range m.params[name] {
		if i < len(m.pending) {
			// 按参数顺序把实参值填入被调函数的形参变量
			callee.vars[paramName] = m.pending[i]
		}
	}
	m.pending = nil
	// 保存调用者现场，返回时恢复
	m.callStack = append(m.callStack, callItem{returnPC: m.pc + 1, frame: m.frame})
	m.frame = callee
	m.pc = start
}

// ret 执行函数返回并恢复调用者活动记录
func (m *machine) ret(arg string) {
	if arg != "_" {
		// 有返回值时先把返回变量或常量放入 R0
		m.r0 = m.readValue(arg)
	}
	if len(m.callStack) == 0 {
		// main 函数 RET 表示程序执行结束
		m.returned = true
		return
	}
	item := m.callStack[len(m.callStack)-1]
	m.callStack = m.callStack[:len(m.callStack)-1]
	m.frame = item.frame
	m.pc = item.returnPC
}

// readValue 从当前活动记录读取变量，也能直接读取常量字面量
func (m *machine) readValue(text string) Value {
	if text == "" || text == "_" {
		return Value{}
	}
	if value, ok := m.frame.vars[text]; ok {
		// 优先读取活动记录里的变量、参数或临时变量
		return value
	}
	if isLiteral(text) {
		// 目标指令中可能直接出现 1、3.14、true 这种字面量
		return parseLiteral(text)
	}
	return Value{}
}

// writeValue 把 R0 或计算结果写入当前活动记录
func (m *machine) writeValue(name string, value Value) {
	if name == "" || name == "_" {
		return
	}
	m.frame.vars[name] = value
}

// result 把运行平台内部状态整理成对外展示的结果
func (m *machine) result() Result {
	var variables []Variable
	for name, value := range m.frame.vars {
		variables = append(variables, Variable{Name: name, Value: value.Text()})
	}
	sort.Slice(variables, func(i, j int) bool {
		return variables[i].Name < variables[j].Name
	})
	return Result{
		ReturnValue: m.r0.Text(),
		Variables:   variables,
		Trace:       m.trace,
		Errors:      m.errors,
	}
}

// evalNumberOp 执行加减乘除和取模运算
func evalNumberOp(op string, left Value, right Value) Value {
	if isFloatValue(left) || isFloatValue(right) {
		// 只要有一个操作数是 float，就按浮点数运算
		a := toFloat(left)
		b := toFloat(right)
		if op == "ADD" {
			return Value{Data: a + b}
		}
		if op == "SUB" {
			return Value{Data: a - b}
		}
		if op == "MUL" {
			return Value{Data: a * b}
		}
		if op == "DIV" && b != 0 {
			return Value{Data: a / b}
		}
		return Value{}
	}

	// 普通 int 运算统一转成 int64，输出时再格式化
	a := toInt(left)
	b := toInt(right)
	switch op {
	case "ADD":
		return Value{Data: a + b}
	case "SUB":
		return Value{Data: a - b}
	case "MUL":
		return Value{Data: a * b}
	case "DIV":
		if b == 0 {
			return Value{}
		}
		return Value{Data: a / b}
	case "MOD":
		if b == 0 {
			return Value{}
		}
		return Value{Data: a % b}
	default:
		return Value{}
	}
}

// evalCompareOp 执行关系运算，结果一定是 bool
func evalCompareOp(op string, left Value, right Value) Value {
	a := toFloat(left)
	b := toFloat(right)
	switch op {
	case "LT":
		return Value{Data: a < b}
	case "GT":
		return Value{Data: a > b}
	case "LE":
		return Value{Data: a <= b}
	case "GE":
		return Value{Data: a >= b}
	case "EQ":
		return Value{Data: left.Text() == right.Text()}
	case "NE":
		return Value{Data: left.Text() != right.Text()}
	default:
		return Value{}
	}
}

// evalLogicOp 执行 && 和 || 逻辑运算
func evalLogicOp(op string, left Value, right Value) Value {
	a := toBool(left)
	b := toBool(right)
	if op == "AND" {
		return Value{Data: a && b}
	}
	return Value{Data: a || b}
}

// evalBitOp 执行整数位运算
func evalBitOp(op string, left Value, right Value) Value {
	a := toInt(left)
	b := toInt(right)
	if b < 0 && (op == "SHL" || op == "SHR") {
		// 负数位移没有意义，这里返回 0 避免移位异常
		return Value{}
	}
	switch op {
	case "BAND":
		return Value{Data: a & b}
	case "BOR":
		return Value{Data: a | b}
	case "XOR":
		return Value{Data: a ^ b}
	case "BCLR":
		return Value{Data: a &^ b}
	case "SHL":
		return Value{Data: a << uint(b)}
	case "SHR":
		return Value{Data: a >> uint(b)}
	default:
		return Value{}
	}
}

// zeroValue 根据类型生成运行时默认值
func zeroValue(typ string) Value {
	switch typ {
	case "float":
		return Value{Data: float64(0)}
	case "bool":
		return Value{Data: false}
	case "string", "char":
		return Value{Data: ""}
	default:
		return Value{Data: int64(0)}
	}
}

// parseLiteral 把源码中的常量文本转换成运行时值
func parseLiteral(text string) Value {
	if text == "true" {
		return Value{Data: true}
	}
	if text == "false" {
		return Value{Data: false}
	}
	if strings.HasPrefix(text, "\"") && strings.HasSuffix(text, "\"") {
		return Value{Data: strings.Trim(text, "\"")}
	}
	if strings.HasPrefix(text, "'") && strings.HasSuffix(text, "'") {
		return Value{Data: strings.Trim(text, "'")}
	}
	if strings.Contains(text, ".") {
		value, err := strconv.ParseFloat(text, 64)
		if err == nil {
			return Value{Data: value}
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err == nil {
		return Value{Data: value}
	}
	return Value{}
}

// Text 把运行时值转换成界面和终端可以展示的字符串
func (v Value) Text() string {
	switch data := v.Data.(type) {
	case int:
		return strconv.Itoa(data)
	case int64:
		return strconv.FormatInt(data, 10)
	case float64:
		return strconv.FormatFloat(data, 'f', -1, 64)
	case bool:
		if data {
			return "true"
		}
		return "false"
	case string:
		return data
	default:
		return "0"
	}
}

// toInt 把运行时值转换成整数，位运算和整数运算使用
func toInt(value Value) int64 {
	switch data := value.Data.(type) {
	case int:
		return int64(data)
	case int64:
		return data
	case float64:
		return int64(data)
	case bool:
		if data {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toFloat 把运行时值转换成浮点数，比较和浮点运算使用
func toFloat(value Value) float64 {
	switch data := value.Data.(type) {
	case int:
		return float64(data)
	case int64:
		return float64(data)
	case float64:
		return data
	case bool:
		if data {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toBool 把运行时值转换成布尔值，条件跳转和逻辑运算使用
func toBool(value Value) bool {
	switch data := value.Data.(type) {
	case bool:
		return data
	case int:
		return data != 0
	case int64:
		return data != 0
	case float64:
		return data != 0
	case string:
		return data != ""
	default:
		return false
	}
}

// isFloatValue 判断运行时值是否为浮点数
func isFloatValue(value Value) bool {
	_, ok := value.Data.(float64)
	return ok
}

// isLiteral 判断文本是否为常量字面量
func isLiteral(text string) bool {
	if text == "true" || text == "false" {
		return true
	}
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return true
	}
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// splitScopedName 把 main.a 拆成函数名 main 和局部名 a
func splitScopedName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "main", name
	}
	return parts[0], parts[1]
}

// formatInstruction 把目标指令整理成执行轨迹里的文本
func formatInstruction(inst codegen.Instruction) string {
	return fmt.Sprintf("%s %s, %s", inst.Op, inst.Arg1, inst.Arg2)
}
