package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"minigo/internal/lexer"
)

// Symbol 表示语义分析阶段的符号表表项
type Symbol struct {
	Index    int    // 符号表中的序号
	Name     string // 符号名，局部变量会带上函数名前缀
	Type     string // 符号类型，例如 int、float、[10]int、Student
	Category string // 符号种类，f=函数，p=参数，v=变量，c=常量，type=类型，t=临时变量
	Addr     int    // 活动记录中的相对地址，函数和类型没有地址时为 -1
	Length   int    // 类型占用的值单元长度
	Value    string // 附加信息，函数保存参数列表，常量保存初值，结构体保存字段列表
}

// Quad 表示四元式，格式为 Op、Arg1、Arg2、Result
type Quad struct {
	Index  int    // 四元式编号
	Op     string // 操作符，例如 +、=、jfalse、call
	Arg1   string // 第一个操作数
	Arg2   string // 第二个操作数，没有时使用 _
	Result string // 结果位置或跳转目标
}

// Result 保存语义分析阶段的输出结果
type Result struct {
	Symbols []Symbol // 符号表总表
	Quads   []Quad   // 语义动作生成的四元式序列
	Errors  []string // 语义错误信息
}

type operand struct {
	Name string // 表达式结果的名字，可能是变量、常量或临时变量
	Type string // 表达式结果的类型
}

type loopLabel struct {
	Begin string // continue 跳转到的循环开始标签
	End   string // break 跳转到的循环结束标签
}

type typeInfo struct {
	Name   string            // 类型名
	Kind   string            // 类型种类，目前主要是 struct
	Length int               // 该类型占用的值单元长度
	Fields map[string]string // 结构体字段名到字段类型的映射
}

type paramInfo struct {
	Name string // 参数名
	Type string // 参数类型
}

type functionInfo struct {
	ReturnType string      // 函数返回类型，void 表示无返回值
	Params     []paramInfo // 函数形参列表
}

// Analyzer 是带语义动作的递归下降分析器
// 它复用语法分析的结构，在识别语法成分时填写符号表并生成四元式
type Analyzer struct {
	tokens []lexer.Token // 词法分析输出的 Token 序列
	pos    int           // 当前分析到的 Token 下标

	symbols     []Symbol                // 符号表总表 SYNBL
	symbolIndex map[string]int          // 符号名到符号表序号的索引
	types       map[string]typeInfo     // 已声明类型信息，用于检查结构体和计算长度
	functions   map[string]functionInfo // 已声明函数信息，用于检查调用参数
	offset      int                     // 当前函数活动记录中下一个可分配地址

	quads      []Quad      // 四元式序列
	tempCount  int         // 临时变量编号计数器，用来生成 t1、t2
	labelCount int         // 标签编号计数器，用来生成 L1、L2
	loopStack  []loopLabel // 当前嵌套循环的标签栈

	currentFuncName      string // 当前正在分析的函数名
	currentFuncReturn    string // 当前函数声明的返回类型
	currentFuncHasReturn bool   // 当前函数是否已经出现 return

	errors []string // 语义错误信息
}

const localVarStartAddr = 5

// Analyze 对 Token 序列进行语义分析，生成符号表和四元式
func Analyze(tokens []lexer.Token) Result {
	a := Analyzer{
		tokens:      tokens,
		symbolIndex: map[string]int{},
		types:       map[string]typeInfo{},
		functions:   map[string]functionInfo{},
		offset:      localVarStartAddr,
	}
	a.parseProgram()
	return Result{Symbols: a.symbols, Quads: a.quads, Errors: a.errors}
}

// <程序> -> { <类型声明语句> | <函数定义> }
func (a *Analyzer) parseProgram() {
	// 顶层按顺序处理类型声明和函数声明
	for !a.isEOF() {
		if a.checkKeyword("type") {
			a.parseTypeDecl()
		} else {
			a.parseFuncDecl()
		}
	}
}

// <函数定义> -> func <标识符> ( <参数列表> ) <返回类型> <复合语句>
func (a *Analyzer) parseFuncDecl() {
	a.expectKeyword("func")
	nameTok := a.expectKind("i", "函数名")

	// 进入新函数时，局部地址从活动记录固定区后面开始分配
	a.currentFuncName = nameTok.Text
	a.offset = localVarStartAddr

	a.expectText("(")
	params := a.parseParamList()
	a.expectText(")")
	returnType := a.parseReturnType() // 分析返回类型
	a.functions[nameTok.Text] = functionInfo{ReturnType: returnType, Params: params}
	// 函数本身进入符号表，参数信息保存在 Value 字段中
	a.enterSymbol(nameTok.Text, returnType, "f", formatParams(params))
	a.emit("program", nameTok.Text, "_", "_")

	a.currentFuncReturn = returnType
	a.currentFuncHasReturn = false

	// 形参也进入符号表，并在活动记录里分配地址
	for _, param := range params {
		a.enterSymbol(param.Name, param.Type, "p", "")
	}

	a.parseBlock()
	// 有返回值的函数必须至少出现一次 return
	if returnType != "void" && !a.currentFuncHasReturn {
		a.addError("函数 " + nameTok.Text + " 缺少 return 语句")
	}

	a.emit("end", nameTok.Text, "_", "_")
	// 函数结束后回到顶层状态
	a.currentFuncName = ""
	a.currentFuncReturn = ""
	a.currentFuncHasReturn = false
	a.offset = localVarStartAddr
}

// <参数列表> -> <参数> { , <参数> } | ε
func (a *Analyzer) parseParamList() []paramInfo {
	var params []paramInfo
	// 右括号表示空参数列表
	if a.checkText(")") {
		return params
	}
	params = append(params, a.parseParam())
	for a.matchText(",") {
		params = append(params, a.parseParam())
	}
	return params
}

// <参数> -> <标识符> <类型>
func (a *Analyzer) parseParam() paramInfo {
	nameTok := a.expectKind("i", "参数名")
	typ := a.parseType()
	return paramInfo{Name: nameTok.Text, Type: typ}
}

// <复合语句> -> { <语句表> }
func (a *Analyzer) parseBlock() {
	a.expectText("{")
	for !a.checkText("}") && !a.isEOF() {
		a.parseStmt()
	}
	a.expectText("}")
}

// <语句> -> 各类声明语句、执行语句、控制语句或空语句
func (a *Analyzer) parseStmt() {
	if a.checkKeyword("var") {
		a.parseVarDecl()
		return
	}
	if a.checkKeyword("const") {
		a.parseConstDecl()
		return
	}
	if a.checkKeyword("type") {
		a.parseTypeDecl()
		return
	}
	if a.checkKeyword("if") {
		a.parseIfStmt()
		return
	}
	if a.checkKeyword("for") {
		a.parseForStmt()
		return
	}
	if a.checkKeyword("break") {
		a.parseBreakStmt()
		return
	}
	if a.checkKeyword("continue") {
		a.parseContinueStmt()
		return
	}
	if a.checkKeyword("return") {
		a.parseReturnStmt()
		return
	}
	if a.checkText("{") {
		a.parseBlock()
		return
	}
	if a.checkText(";") {
		a.advance()
		return
	}
	if a.checkKind("i") {
		a.parseIDStartStmt()
		return
	}
	a.addError("无法识别的语句开头")
	a.advance()
}

// <变量声明语句> -> var <标识符> <类型> ;
func (a *Analyzer) parseVarDecl() {
	a.expectKeyword("var")
	nameTok := a.expectKind("i", "变量名")
	typ := a.parseType()
	// 普通变量进入符号表，种类为 v
	a.enterSymbol(nameTok.Text, typ, "v", "")
	a.expectText(";")
}

// <类型声明语句> -> type <标识符> struct { <字段声明表> }
func (a *Analyzer) parseTypeDecl() {
	a.expectKeyword("type")
	nameTok := a.expectKind("i", "类型名")
	a.expectKeyword("struct")
	a.expectText("{")

	fields := map[string]string{}
	var fieldNames []string
	totalLen := 0
	// 逐个读取结构体字段，并累加结构体总长度
	for !a.checkText("}") && !a.isEOF() {
		fieldTok := a.expectKind("i", "字段名")
		fieldType := a.parseType()
		a.expectText(";")
		if _, ok := fields[fieldTok.Text]; ok {
			a.addError("结构体字段重复声明 " + fieldTok.Text)
			continue
		}
		fields[fieldTok.Text] = fieldType
		fieldNames = append(fieldNames, fieldTok.Text)
		totalLen += a.typeLength(fieldType)
	}
	a.expectText("}")

	// 结构体类型保存到 types 表，后续变量声明和字段访问都要查它
	a.types[nameTok.Text] = typeInfo{
		Name:   nameTok.Text,
		Kind:   "struct",
		Length: totalLen,
		Fields: fields,
	}
	sym := a.enterSymbol(nameTok.Text, "struct", "type", formatFields(fieldNames, fields))
	if sym.Index > 0 {
		// 类型长度在 enterSymbol 后回填，方便 SYNBL 和 LENL 展示
		a.symbols[sym.Index-1].Length = totalLen
	}
}

// <常量声明语句> -> const <标识符> <赋值运算符> <表达式> ;
func (a *Analyzer) parseConstDecl() {
	a.expectKeyword("const")
	nameTok := a.expectKind("i", "常量名")
	if a.matchText(":=") || a.matchText("=") {
		value := a.parseExpr()
		// 常量不分配活动记录地址，初值保存在 Value 字段中
		a.enterSymbol(nameTok.Text, value.Type, "c", value.Name)
	} else {
		a.addError("常量声明缺少 := 或 =")
	}
	a.expectText(";")
}

// <类型> -> <基本类型> | <数组类型> | <标识符>
func (a *Analyzer) parseType() string {
	// 基本类型直接返回类型名
	for _, typ := range []string{"int", "bool", "float", "char", "string"} {
		if a.matchKeyword(typ) {
			return typ
		}
	}
	// 数组类型保存为 [长度]元素类型，例如 [10]int
	if a.matchText("[") {
		lenTok := a.expectKind("c", "数组长度")
		a.expectText("]")
		elemType := a.parseType()
		return "[" + lenTok.Text + "]" + elemType
	}
	if a.checkKind("i") {
		// 标识符类型必须已经通过 type 声明
		typeTok := a.expectKind("i", "类型名")
		if _, ok := a.types[typeTok.Text]; !ok {
			a.addError("未声明的类型 " + typeTok.Text)
		}
		return typeTok.Text
	}
	a.addError("缺少类型名")
	return "unknown"
}

// <返回类型> -> <类型> | ε
func (a *Analyzer) parseReturnType() string {
	if a.isTypeKeyword() {
		return a.parseType()
	}
	// 没写返回类型时按 void 处理
	return "void"
}

// <赋值语句> -> <左值> <赋值运算符> <表达式> ;
func (a *Analyzer) parseIDStartStmt() {
	a.parseSimpleStmt(true)
}

// <简单语句> -> <函数调用> | <左值> <赋值运算符> <表达式> | <左值> (++ | --)
func (a *Analyzer) parseSimpleStmt(needSemicolon bool) {
	// 标识符后面是左括号，说明这是函数调用语句
	if a.nextText("(") {
		a.parseCallExpr()
		if needSemicolon {
			a.expectText(";")
		}
		return
	}

	left := a.parseDesignator(false)

	// MiniGo 同时支持 = 和 :=，:= 可以声明当前作用域中的新变量
	assignOp := ""
	if a.matchText("=") {
		assignOp = "="
	} else if a.matchText(":=") {
		assignOp = ":="
	}

	if assignOp != "" {
		value := a.parseExpr()
		if left.Type == "unknown" && left.Name != "" {
			if assignOp == ":=" {
				// := 允许第一次出现的名字成为局部变量
				a.enterSymbol(left.Name, value.Type, "v", "")
				left.Type = value.Type
			} else {
				a.addError("未声明的标识符 " + left.Name)
			}
		} else if !canAssign(left.Type, value.Type) {
			a.addError("赋值类型不匹配：" + left.Type + " <- " + value.Type)
		}
		a.emit("=", value.Name, "_", left.Name)
		if needSemicolon {
			a.expectText(";")
		}
		return
	}

	if a.matchText("++") {
		a.emitSelfUpdate(left, "+")
		if needSemicolon {
			a.expectText(";")
		}
		return
	}
	if a.matchText("--") {
		a.emitSelfUpdate(left, "-")
		if needSemicolon {
			a.expectText(";")
		}
		return
	}

	a.addError("标识符后面应为 =、:=、++ 或 --")
	if needSemicolon {
		a.skipToStmtEnd()
	}
}

// emitSelfUpdate 把 i++ 或 i-- 翻译成普通四元式
func (a *Analyzer) emitSelfUpdate(left operand, op string) {
	if left.Type == "unknown" {
		a.addError("未声明的标识符 " + left.Name)
		return
	}
	if left.Type != "int" && left.Type != "float" {
		a.addError("自增自减只能用于数字类型")
		return
	}
	temp := a.makeBinary(op, left, operand{Name: "1", Type: "int"})
	a.emit("=", temp.Name, "_", left.Name)
}

// <左值> -> <标识符> { [ <表达式> ] | 点 <标识符> }
func (a *Analyzer) parseDesignator(reportUnknown bool) operand {
	nameTok := a.expectKind("i", "标识符")
	name := nameTok.Text
	typ := "unknown"
	// 先查普通变量、参数、常量或函数作用域内的名字
	if sym, ok := a.lookupSymbol(name); ok {
		typ = sym.Type
	}

	for {
		if a.matchText("[") {
			// 数组访问会把类型从数组类型降为元素类型
			index := a.parseExpr()
			name = name + "[" + index.Name + "]"
			elemType, ok := a.arrayElementType(typ)
			if ok {
				typ = elemType
			} else if typ != "unknown" {
				a.addError("非数组类型不能使用下标")
			}
			a.expectText("]")
			continue
		}
		if a.matchText(".") {
			// 结构体字段访问会根据结构体类型查字段类型
			fieldTok := a.expectKind("i", "字段名")
			name = name + "." + fieldTok.Text
			fieldType, ok := a.structFieldType(typ, fieldTok.Text)
			if ok {
				typ = fieldType
			} else if typ != "unknown" {
				a.addError("结构体类型 " + typ + " 中不存在字段 " + fieldTok.Text)
			}
			continue
		}
		break
	}

	if typ == "unknown" && reportUnknown {
		a.addError("未声明的标识符 " + nameTok.Text)
	}
	return operand{Name: name, Type: typ}
}

// <条件语句> -> if <表达式> <复合语句> [ else <复合语句> ]
func (a *Analyzer) parseIfStmt() {
	a.expectKeyword("if")
	cond := a.parseExpr()
	// 条件为 false 时跳到 else 标签，语句结束后跳到 end 标签
	elseLabel := a.newLabel()
	endLabel := a.newLabel()
	a.emit("jfalse", cond.Name, "_", elseLabel)
	a.parseBlock()
	if a.matchKeyword("else") {
		a.emit("j", "_", "_", endLabel)
		a.emit("label", "_", "_", elseLabel)
		a.parseBlock()
		a.emit("label", "_", "_", endLabel)
	} else {
		a.emit("label", "_", "_", elseLabel)
	}
}

// <for循环语句> -> for <表达式> <复合语句> | for <简单语句>? ; <表达式>? ; <简单语句>? <复合语句> | for <复合语句>
func (a *Analyzer) parseForStmt() {
	a.expectKeyword("for")
	if a.checkText("{") {
		a.parseInfiniteForStmt()
		return
	}
	if a.hasTextBeforeBlock(";") {
		a.parseThreePartForStmt()
		return
	}
	a.parseConditionForStmt()
}

// parseConditionForStmt 处理 for 条件 { } 这种类似 while 的循环
func (a *Analyzer) parseConditionForStmt() {
	begin := a.newLabel()
	end := a.newLabel()
	// begin 标号放在条件判断之前，循环体结束后跳回 begin
	a.emit("label", "_", "_", begin)
	cond := a.parseExpr()
	a.emit("jfalse", cond.Name, "_", end)
	// 保存当前循环标签，供 break 和 continue 使用
	a.loopStack = append(a.loopStack, loopLabel{Begin: begin, End: end})
	a.parseBlock()
	a.loopStack = a.loopStack[:len(a.loopStack)-1]
	a.emit("j", "_", "_", begin)
	a.emit("label", "_", "_", end)
}

// parseInfiniteForStmt 处理 for { } 这种无限循环
func (a *Analyzer) parseInfiniteForStmt() {
	begin := a.newLabel()
	end := a.newLabel()
	a.emit("label", "_", "_", begin)
	a.loopStack = append(a.loopStack, loopLabel{Begin: begin, End: end})
	a.parseBlock()
	a.loopStack = a.loopStack[:len(a.loopStack)-1]
	a.emit("j", "_", "_", begin)
	a.emit("label", "_", "_", end)
}

// parseThreePartForStmt 处理 for 初始化 ; 条件 ; 更新 { } 这种三段式循环
func (a *Analyzer) parseThreePartForStmt() {
	if !a.checkText(";") {
		// 初始化语句先执行一次，所以直接生成在循环开始之前
		a.parseSimpleStmt(false)
	}
	a.expectText(";")

	begin := a.newLabel()
	post := a.newLabel()
	end := a.newLabel()
	a.emit("label", "_", "_", begin)

	if !a.checkText(";") {
		// 条件为空时表示永真循环
		cond := a.parseExpr()
		a.emit("jfalse", cond.Name, "_", end)
	}
	a.expectText(";")

	postStart := len(a.quads)
	if !a.checkText("{") {
		// 更新语句源码上在循环体前面，但运行时要放到循环体后面
		a.parseSimpleStmt(false)
	}
	postQuads := a.takeQuads(postStart)

	// 三段式 for 的 continue 应该跳到更新语句，而不是直接跳到条件判断
	a.loopStack = append(a.loopStack, loopLabel{Begin: post, End: end})
	a.parseBlock()
	a.loopStack = a.loopStack[:len(a.loopStack)-1]

	// 循环体结束后执行更新语句，再回到条件判断
	a.emit("label", "_", "_", post)
	a.appendQuads(postQuads)
	a.emit("j", "_", "_", begin)
	a.emit("label", "_", "_", end)
}

// <break语句> -> break ;
func (a *Analyzer) parseBreakStmt() {
	a.expectKeyword("break")
	if len(a.loopStack) == 0 {
		a.addError("break 不在循环语句中")
	} else {
		// break 跳到当前最内层循环的结束标签
		top := a.loopStack[len(a.loopStack)-1]
		a.emit("j", "_", "_", top.End)
	}
	a.expectText(";")
}

// <continue语句> -> continue ;
func (a *Analyzer) parseContinueStmt() {
	a.expectKeyword("continue")
	if len(a.loopStack) == 0 {
		a.addError("continue 不在循环语句中")
	} else {
		// continue 跳到当前最内层循环的开始标签
		top := a.loopStack[len(a.loopStack)-1]
		a.emit("j", "_", "_", top.Begin)
	}
	a.expectText(";")
}

// <return语句> -> return [ <表达式> ] ;
func (a *Analyzer) parseReturnStmt() {
	a.expectKeyword("return")
	a.currentFuncHasReturn = true
	if !a.checkText(";") {
		value := a.parseExpr()
		// 有返回表达式时检查它能不能赋给函数返回类型
		if a.currentFuncReturn == "void" {
			a.addError("无返回值函数不能返回表达式")
		} else if !canAssign(a.currentFuncReturn, value.Type) {
			a.addError("return 类型不匹配：" + a.currentFuncReturn + " <- " + value.Type)
		}
		a.emit("return", value.Name, "_", "_")
	} else {
		// 没有返回表达式时，只允许 void 函数这样写
		if a.currentFuncReturn != "void" {
			a.addError("函数 " + a.currentFuncName + " 需要返回 " + a.currentFuncReturn)
		}
		a.emit("return", "_", "_", "_")
	}
	a.expectText(";")
}

// <表达式> -> <逻辑或表达式>
func (a *Analyzer) parseExpr() operand {
	return a.parseOr()
}

// <逻辑或表达式> -> <逻辑与表达式> { || <逻辑与表达式> }
func (a *Analyzer) parseOr() operand {
	left := a.parseAnd()
	for a.matchText("||") {
		right := a.parseAnd()
		left = a.makeBinary("||", left, right)
	}
	return left
}

// <逻辑与表达式> -> <关系表达式> { && <关系表达式> }
func (a *Analyzer) parseAnd() operand {
	left := a.parseCompare()
	for a.matchText("&&") {
		right := a.parseCompare()
		left = a.makeBinary("&&", left, right)
	}
	return left
}

// <关系表达式> -> <加法表达式> { <关系运算符> <加法表达式> }
func (a *Analyzer) parseCompare() operand {
	left := a.parseAdd()
	for {
		op := a.current().Text
		if op != "==" && op != "!=" && op != "<" && op != "<=" && op != ">" && op != ">=" {
			break
		}
		a.advance()
		right := a.parseAdd()
		left = a.makeBinary(op, left, right)
	}
	return left
}

// <加法表达式> -> <乘法表达式> { (+ | - | '|' | ^) <乘法表达式> }
func (a *Analyzer) parseAdd() operand {
	left := a.parseMul()
	for {
		op := a.current().Text
		if op != "+" && op != "-" && op != "|" && op != "^" {
			break
		}
		a.advance()
		right := a.parseMul()
		left = a.makeBinary(op, left, right)
	}
	return left
}

// <乘法表达式> -> <一元表达式> { (* | / | % | << | >> | & | &^) <一元表达式> }
func (a *Analyzer) parseMul() operand {
	left := a.parseUnary()
	for {
		op := a.current().Text
		if op != "*" && op != "/" && op != "%" && op != "<<" && op != ">>" && op != "&" && op != "&^" {
			break
		}
		a.advance()
		right := a.parseUnary()
		left = a.makeBinary(op, left, right)
	}
	return left
}

// <一元表达式> -> (! | - | + | ^ | & | *) <一元表达式> | <基本表达式>
func (a *Analyzer) parseUnary() operand {
	op := a.current().Text
	if op == "!" || op == "-" || op == "+" || op == "^" || op == "&" || op == "*" {
		a.advance()
		value := a.parseUnary()
		return a.makeUnary(op, value)
	}
	return a.parsePrimary()
}

// <基本表达式> -> <标识符> | <函数调用> | <常数> | true | false | ( <表达式> )
func (a *Analyzer) parsePrimary() operand {
	if a.checkKind("i") {
		// 标识符既可能是函数调用，也可能是变量、数组元素或结构体字段
		if a.nextText("(") {
			return a.parseCallExpr()
		}
		return a.parseDesignator(true)
	}
	if a.checkKind("c") {
		tok := a.current()
		a.advance()
		// 常量的类型根据文本形态简单推断
		return operand{Name: tok.Text, Type: literalType(tok.Text)}
	}
	if a.matchKeyword("true") {
		return operand{Name: "true", Type: "bool"}
	}
	if a.matchKeyword("false") {
		return operand{Name: "false", Type: "bool"}
	}
	if a.matchText("(") {
		// 括号不生成四元式，只改变表达式优先级
		value := a.parseExpr()
		a.expectText(")")
		return value
	}
	a.addError("缺少表达式")
	a.advance()
	return operand{Name: "?", Type: "unknown"}
}

// <函数调用> -> <标识符> ( <实参列表> )
func (a *Analyzer) parseCallExpr() operand {
	nameTok := a.expectKind("i", "函数名")
	a.expectText("(")
	args := a.parseArgumentList()
	a.expectText(")")

	// 调用前先检查函数是否已经声明
	info, ok := a.functions[nameTok.Text]
	if !ok {
		a.addError("未声明的函数 " + nameTok.Text)
		return operand{Name: "?", Type: "unknown"}
	}
	if len(args) != len(info.Params) {
		a.addError(fmt.Sprintf("函数 %s 参数个数不匹配：需要 %d 个，实际 %d 个",
			nameTok.Text, len(info.Params), len(args)))
	} else {
		// 参数个数一致后，再逐个检查参数类型
		for i, arg := range args {
			if !canAssign(info.Params[i].Type, arg.Type) {
				a.addError(fmt.Sprintf("函数 %s 第 %d 个参数类型不匹配：%s <- %s",
					nameTok.Text, i+1, info.Params[i].Type, arg.Type))
			}
		}
	}

	// 每个实参先生成 param 四元式
	for i, arg := range args {
		a.emit("param", arg.Name, strconv.Itoa(i+1), "_")
	}
	result := "_"
	if info.ReturnType != "void" {
		// 有返回值的函数调用需要一个临时变量接住结果
		result = a.newTemp(info.ReturnType).Name
	}
	a.emit("call", nameTok.Text, strconv.Itoa(len(args)), result)
	return operand{Name: result, Type: info.ReturnType}
}

// <实参列表> -> <表达式> { , <表达式> } | ε
func (a *Analyzer) parseArgumentList() []operand {
	var args []operand
	if a.checkText(")") {
		return args
	}
	args = append(args, a.parseExpr())
	for a.matchText(",") {
		args = append(args, a.parseExpr())
	}
	return args
}

func (a *Analyzer) makeBinary(op string, left operand, right operand) operand {
	// 二元表达式统一生成一个临时变量保存结果
	typ := binaryResultType(op, left.Type, right.Type)
	temp := a.newTemp(typ)
	a.emit(op, left.Name, right.Name, temp.Name)
	return temp
}

func (a *Analyzer) makeUnary(op string, value operand) operand {
	// 一元表达式也生成临时变量，方便后续优化和目标代码生成
	resultType := value.Type
	quadOp := op
	if op == "!" {
		resultType = "bool"
	} else if op == "-" {
		quadOp = "uminus"
	} else if op == "+" {
		quadOp = "uplus"
	}
	temp := a.newTemp(resultType)
	a.emit(quadOp, value.Name, "_", temp.Name)
	return temp
}

func (a *Analyzer) enterSymbol(name string, typ string, category string, value string) Symbol {
	if name == "" {
		return Symbol{}
	}
	key := a.symbolKey(name, category)
	// 同一作用域内同名符号不能重复声明
	if _, ok := a.symbolIndex[key]; ok {
		a.addError("重复声明标识符 " + name)
		return Symbol{}
	}

	length := a.typeLength(typ)
	if category == "f" {
		length = 0
	}
	addr := -1
	if category == "v" || category == "p" || category == "t" {
		// 变量、形参和临时变量需要进入活动记录并占用地址
		addr = a.offset
		a.offset += length
	}

	displayName := name
	if category != "f" && category != "type" && a.currentFuncName != "" {
		// 局部符号显示为函数名加变量名，方便区分不同函数作用域
		displayName = a.currentFuncName + "." + name
	}

	sym := Symbol{
		Index:    len(a.symbols) + 1,
		Name:     displayName,
		Type:     typ,
		Category: category,
		Addr:     addr,
		Length:   length,
		Value:    value,
	}
	a.symbols = append(a.symbols, sym)
	a.symbolIndex[key] = sym.Index
	return sym
}

func (a *Analyzer) lookupSymbol(name string) (Symbol, bool) {
	if a.currentFuncName != "" {
		// 先查当前函数作用域
		index, ok := a.symbolIndex[a.currentFuncName+"."+name]
		if ok {
			return a.symbols[index-1], true
		}
	}
	// 再查全局作用域
	index, ok := a.symbolIndex[name]
	if ok {
		return a.symbols[index-1], true
	}
	return Symbol{}, false
}

func (a *Analyzer) symbolKey(name string, category string) string {
	// 函数名和类型名保持全局唯一
	if category == "f" || category == "type" || a.currentFuncName == "" {
		return name
	}
	// 局部变量、参数、临时变量都挂到当前函数下面
	return a.currentFuncName + "." + name
}

func (a *Analyzer) newTemp(typ string) operand {
	// 临时变量用于保存表达式中间结果，例如 t1、t2
	a.tempCount++
	name := fmt.Sprintf("t%d", a.tempCount)
	a.enterSymbol(name, typ, "t", "")
	return operand{Name: name, Type: typ}
}

func (a *Analyzer) newLabel() string {
	// 标签用于 if 和 for 的跳转目标，例如 L1、L2
	a.labelCount++
	return fmt.Sprintf("L%d", a.labelCount)
}

func (a *Analyzer) emit(op string, arg1 string, arg2 string, result string) {
	// 每生成一条四元式就自动分配递增编号
	a.quads = append(a.quads, Quad{
		Index:  len(a.quads) + 1,
		Op:     op,
		Arg1:   arg1,
		Arg2:   arg2,
		Result: result,
	})
}

// takeQuads 取出从 start 开始新生成的四元式，并从原列表里临时删除
func (a *Analyzer) takeQuads(start int) []Quad {
	if start >= len(a.quads) {
		return nil
	}
	result := make([]Quad, len(a.quads[start:]))
	copy(result, a.quads[start:])
	a.quads = a.quads[:start]
	return result
}

// appendQuads 把暂存的四元式重新追加到当前位置，并重新分配编号
func (a *Analyzer) appendQuads(quads []Quad) {
	for _, q := range quads {
		a.emit(q.Op, q.Arg1, q.Arg2, q.Result)
	}
}

func (a *Analyzer) current() lexer.Token {
	// 越界时返回 eof，避免错误恢复时数组越界
	if a.pos >= len(a.tokens) {
		return lexer.Token{Kind: "eof", Text: "#"}
	}
	return a.tokens[a.pos]
}

func (a *Analyzer) advance() {
	// 向前消费一个 Token
	if a.pos < len(a.tokens) {
		a.pos++
	}
}

func (a *Analyzer) isEOF() bool {
	return a.current().Kind == "eof"
}

func (a *Analyzer) checkKind(kind string) bool {
	return a.current().Kind == kind
}

func (a *Analyzer) checkText(text string) bool {
	return a.current().Text == text
}

func (a *Analyzer) checkKeyword(word string) bool {
	tok := a.current()
	return tok.Kind == "k" && tok.Text == word
}

func (a *Analyzer) nextText(text string) bool {
	if a.pos+1 >= len(a.tokens) {
		return false
	}
	return a.tokens[a.pos+1].Text == text
}

// hasTextBeforeBlock 判断当前 for 头部到代码块之前是否出现指定符号
// 用它区分 for 条件循环 和 for init; cond; post 三段式循环
func (a *Analyzer) hasTextBeforeBlock(text string) bool {
	for i := a.pos; i < len(a.tokens); i++ {
		if a.tokens[i].Text == "{" || a.tokens[i].Kind == "eof" {
			return false
		}
		if a.tokens[i].Text == text {
			return true
		}
	}
	return false
}

func (a *Analyzer) isTypeKeyword() bool {
	return a.checkKeyword("int") || a.checkKeyword("bool") || a.checkKeyword("float") ||
		a.checkKeyword("char") || a.checkKeyword("string") || a.checkText("[") || a.checkKind("i")
}

func (a *Analyzer) matchText(text string) bool {
	if a.checkText(text) {
		a.advance()
		return true
	}
	return false
}

func (a *Analyzer) matchKeyword(word string) bool {
	if a.checkKeyword(word) {
		a.advance()
		return true
	}
	return false
}

func (a *Analyzer) expectKind(kind string, name string) lexer.Token {
	// 返回当前 Token，方便语义阶段拿到标识符名字或常量文本
	tok := a.current()
	if a.checkKind(kind) {
		a.advance()
		return tok
	}
	a.addError("缺少" + name)
	return lexer.Token{}
}

func (a *Analyzer) expectText(text string) {
	if a.checkText(text) {
		a.advance()
		return
	}
	a.addError("缺少 " + text)
}

func (a *Analyzer) expectKeyword(word string) {
	if a.checkKeyword(word) {
		a.advance()
		return
	}
	a.addError("缺少关键字 " + word)
}

func (a *Analyzer) addError(message string) {
	// 语义错误也记录行列，便于在源程序中定位
	tok := a.current()
	a.errors = append(a.errors, fmt.Sprintf("第%d行第%d列: %s，当前单词为 %q", tok.Line, tok.Column, message, tok.Text))
}

func (a *Analyzer) skipToStmtEnd() {
	// 发生错误后跳过当前语句，减少连锁报错
	for !a.isEOF() && !a.checkText(";") && !a.checkText("}") {
		a.advance()
	}
	if a.checkText(";") {
		a.advance()
	}
}

func (a *Analyzer) typeLength(typ string) int {
	// 基本类型长度按课设中值单元分配的简化规则计算
	switch typ {
	case "int":
		return 4
	case "float":
		return 8
	case "bool", "char":
		return 1
	case "string":
		return 16
	default:
		if info, ok := a.types[typ]; ok {
			return info.Length
		}
		if strings.HasPrefix(typ, "[") {
			// 数组长度等于元素个数乘以元素类型长度
			right := strings.Index(typ, "]")
			if right <= 1 {
				return 0
			}
			size, err := strconv.Atoi(typ[1:right])
			if err != nil {
				return 0
			}
			elemType := typ[right+1:]
			return size * a.typeLength(elemType)
		}
		return 0
	}
}

func (a *Analyzer) arrayElementType(typ string) (string, bool) {
	// 从 [10]int 中取出 int
	if !strings.HasPrefix(typ, "[") {
		return "", false
	}
	right := strings.Index(typ, "]")
	if right <= 1 || right+1 >= len(typ) {
		return "", false
	}
	return typ[right+1:], true
}

func (a *Analyzer) structFieldType(typ string, fieldName string) (string, bool) {
	// 根据结构体类型名和字段名查字段类型
	info, ok := a.types[typ]
	if !ok || info.Kind != "struct" {
		return "", false
	}
	fieldType, ok := info.Fields[fieldName]
	return fieldType, ok
}

func formatFields(fieldNames []string, fields map[string]string) string {
	// 保持字段声明顺序，便于输出结构表
	var parts []string
	for _, name := range fieldNames {
		parts = append(parts, name+":"+fields[name])
	}
	return strings.Join(parts, ",")
}

func formatParams(params []paramInfo) string {
	// 参数列表保存到函数符号的 Value 字段中
	var parts []string
	for _, param := range params {
		parts = append(parts, param.Name+":"+param.Type)
	}
	return strings.Join(parts, ",")
}

func literalType(text string) string {
	// 字符串以双引号开头，字符以单引号开头
	if strings.HasPrefix(text, "\"") {
		return "string"
	}
	if strings.HasPrefix(text, "'") {
		return "char"
	}
	if strings.Contains(text, ".") {
		return "float"
	}
	return "int"
}

func binaryResultType(op string, leftType string, rightType string) string {
	// 比较运算和逻辑运算的结果一定是 bool
	if op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" ||
		op == "&&" || op == "||" {
		return "bool"
	}
	// float 和 int 混合运算时结果提升为 float
	if leftType == "float" || rightType == "float" {
		return "float"
	}
	if leftType == "unknown" || rightType == "unknown" {
		return "unknown"
	}
	return "int"
}

func canAssign(leftType string, rightType string) bool {
	// unknown 已经有其他错误提示，这里放行以减少连锁错误
	if leftType == "unknown" || rightType == "unknown" {
		return true
	}
	if leftType == rightType {
		return true
	}
	// 允许 int 赋给 float
	return leftType == "float" && rightType == "int"
}
