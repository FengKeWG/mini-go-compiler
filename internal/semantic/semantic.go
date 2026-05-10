package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"minigo/internal/lexer"
)

// Symbol 表示语义分析阶段的符号表表项。
type Symbol struct {
	Index    int
	Name     string
	Type     string
	Category string
	Addr     int
	Length   int
	Value    string
}

// Quad 表示四元式：(Op, Arg1, Arg2, Result)。
type Quad struct {
	Index  int
	Op     string
	Arg1   string
	Arg2   string
	Result string
}

// Result 保存语义分析阶段的输出结果。
type Result struct {
	Symbols []Symbol
	Quads   []Quad
	Errors  []string
}

type operand struct {
	Name string
	Type string
}

type loopLabel struct {
	Begin string
	End   string
}

type typeInfo struct {
	Name   string
	Kind   string
	Length int
	Fields map[string]string
}

// Analyzer 是带语义动作的递归下降分析器。
// 它复用语法分析的结构，在识别语法成分时填写符号表并生成四元式。
type Analyzer struct {
	tokens []lexer.Token
	pos    int

	symbols     []Symbol
	symbolIndex map[string]int
	types       map[string]typeInfo
	offset      int

	quads      []Quad
	tempCount  int
	labelCount int
	loopStack  []loopLabel

	currentFuncName      string
	currentFuncReturn    string
	currentFuncHasReturn bool

	errors []string
}

const localVarStartAddr = 5

// Analyze 对 Token 序列进行语义分析，生成符号表和四元式。
func Analyze(tokens []lexer.Token) Result {
	a := Analyzer{
		tokens:      tokens,
		symbolIndex: map[string]int{},
		types:       map[string]typeInfo{},
		offset:      localVarStartAddr,
	}
	a.parseProgram()
	return Result{Symbols: a.symbols, Quads: a.quads, Errors: a.errors}
}

// <程序> -> { <类型声明语句> | <函数定义> }
func (a *Analyzer) parseProgram() {
	for !a.isEOF() {
		if a.checkKeyword("type") {
			a.parseTypeDecl()
		} else {
			a.parseFuncDecl()
		}
	}
}

// <函数定义> -> func <标识符> ( ) <返回类型> <复合语句>
func (a *Analyzer) parseFuncDecl() {
	a.expectKeyword("func")
	nameTok := a.expectKind("i", "函数名")
	a.expectText("(")
	a.expectText(")")
	returnType := a.parseReturnType()
	a.enterSymbol(nameTok.Text, returnType, "f", "")
	a.emit("program", nameTok.Text, "_", "_")

	oldFuncName := a.currentFuncName
	oldFuncReturn := a.currentFuncReturn
	oldFuncHasReturn := a.currentFuncHasReturn
	a.currentFuncName = nameTok.Text
	a.currentFuncReturn = returnType
	a.currentFuncHasReturn = false

	a.parseBlock()
	if returnType != "void" && !a.currentFuncHasReturn {
		a.addError("函数 " + nameTok.Text + " 缺少 return 语句")
	}

	a.currentFuncName = oldFuncName
	a.currentFuncReturn = oldFuncReturn
	a.currentFuncHasReturn = oldFuncHasReturn
	a.emit("end", nameTok.Text, "_", "_")
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
	fieldNames := []string{}
	totalLen := 0
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

	a.types[nameTok.Text] = typeInfo{
		Name:   nameTok.Text,
		Kind:   "struct",
		Length: totalLen,
		Fields: fields,
	}
	sym := a.enterSymbol(nameTok.Text, "struct", "type", formatFields(fieldNames, fields))
	if sym.Index > 0 {
		a.symbols[sym.Index-1].Length = totalLen
	}
}

// <常量声明语句> -> const <标识符> <赋值运算符> <表达式> ;
func (a *Analyzer) parseConstDecl() {
	a.expectKeyword("const")
	nameTok := a.expectKind("i", "常量名")
	if a.matchText(":=") || a.matchText("=") {
		value := a.parseExpr()
		a.enterSymbol(nameTok.Text, value.Type, "c", value.Name)
	} else {
		a.addError("常量声明缺少 := 或 =")
	}
	a.expectText(";")
}

// <类型> -> <基本类型> | <数组类型> | <标识符>
func (a *Analyzer) parseType() string {
	for _, typ := range []string{"int", "bool", "float", "char", "string"} {
		if a.matchKeyword(typ) {
			return typ
		}
	}
	if a.matchText("[") {
		lenTok := a.expectKind("c", "数组长度")
		a.expectText("]")
		elemType := a.parseType()
		return "[" + lenTok.Text + "]" + elemType
	}
	if a.checkKind("i") {
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
	return "void"
}

// <赋值语句> -> <左值> <赋值运算符> <表达式> ;
func (a *Analyzer) parseIDStartStmt() {
	left := a.parseDesignator(false)

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
				a.enterSymbol(left.Name, value.Type, "v", "")
				left.Type = value.Type
			} else {
				a.addError("未声明的标识符 " + left.Name)
			}
		} else if !canAssign(left.Type, value.Type) {
			a.addError("赋值类型不匹配：" + left.Type + " <- " + value.Type)
		}
		a.emit("=", value.Name, "_", left.Name)
		a.expectText(";")
		return
	}

	// 扩展语句只用于覆盖冒号和逗号的语法形式，不生成四元式。
	if a.matchText(":") {
		a.parseExpr()
		for a.matchText(",") {
			a.parseExpr()
		}
		a.expectText(";")
		return
	}

	a.addError("标识符后面应为 =、:= 或 :")
	a.skipToStmtEnd()
}

// <左值> -> <标识符> { [ <表达式> ] | . <标识符> }
func (a *Analyzer) parseDesignator(reportUnknown bool) operand {
	nameTok := a.expectKind("i", "标识符")
	name := nameTok.Text
	typ := "unknown"
	if sym, ok := a.lookupSymbol(name); ok {
		typ = sym.Type
	}

	for {
		if a.matchText("[") {
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

// <for循环语句> -> for <表达式> <复合语句>
func (a *Analyzer) parseForStmt() {
	a.expectKeyword("for")
	begin := a.newLabel()
	end := a.newLabel()
	a.emit("label", "_", "_", begin)
	cond := a.parseExpr()
	a.emit("jfalse", cond.Name, "_", end)
	a.loopStack = append(a.loopStack, loopLabel{Begin: begin, End: end})
	a.parseBlock()
	a.loopStack = a.loopStack[:len(a.loopStack)-1]
	a.emit("j", "_", "_", begin)
	a.emit("label", "_", "_", end)
}

// <break语句> -> break ;
func (a *Analyzer) parseBreakStmt() {
	a.expectKeyword("break")
	if len(a.loopStack) == 0 {
		a.addError("break 不在循环语句中")
	} else {
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
		if a.currentFuncReturn == "void" {
			a.addError("无返回值函数不能返回表达式")
		} else if !canAssign(a.currentFuncReturn, value.Type) {
			a.addError("return 类型不匹配：" + a.currentFuncReturn + " <- " + value.Type)
		}
		a.emit("return", value.Name, "_", "_")
	} else {
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

// <基本表达式> -> <标识符> | <常数> | true | false | ( <表达式> )
func (a *Analyzer) parsePrimary() operand {
	if a.checkKind("i") {
		return a.parseDesignator(true)
	}
	if a.checkKind("c") {
		tok := a.current()
		a.advance()
		return operand{Name: tok.Text, Type: literalType(tok.Text)}
	}
	if a.matchKeyword("true") {
		return operand{Name: "true", Type: "bool"}
	}
	if a.matchKeyword("false") {
		return operand{Name: "false", Type: "bool"}
	}
	if a.matchText("(") {
		value := a.parseExpr()
		a.expectText(")")
		return value
	}
	a.addError("缺少表达式")
	a.advance()
	return operand{Name: "?", Type: "unknown"}
}

func (a *Analyzer) makeBinary(op string, left operand, right operand) operand {
	typ := binaryResultType(op, left.Type, right.Type)
	temp := a.newTemp(typ)
	a.emit(op, left.Name, right.Name, temp.Name)
	return temp
}

func (a *Analyzer) makeUnary(op string, value operand) operand {
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
	if _, ok := a.symbolIndex[name]; ok {
		a.addError("重复声明标识符 " + name)
		return Symbol{}
	}

	length := a.typeLength(typ)
	if category == "f" {
		length = 0
	}
	addr := -1
	if category == "v" || category == "t" {
		addr = a.offset
		a.offset += length
	}

	sym := Symbol{
		Index:    len(a.symbols) + 1,
		Name:     name,
		Type:     typ,
		Category: category,
		Addr:     addr,
		Length:   length,
		Value:    value,
	}
	a.symbols = append(a.symbols, sym)
	a.symbolIndex[name] = sym.Index
	return sym
}

func (a *Analyzer) lookupSymbol(name string) (Symbol, bool) {
	index, ok := a.symbolIndex[name]
	if !ok {
		return Symbol{}, false
	}
	return a.symbols[index-1], true
}

func (a *Analyzer) newTemp(typ string) operand {
	a.tempCount++
	name := fmt.Sprintf("t%d", a.tempCount)
	a.enterSymbol(name, typ, "t", "")
	return operand{Name: name, Type: typ}
}

func (a *Analyzer) newLabel() string {
	a.labelCount++
	return fmt.Sprintf("L%d", a.labelCount)
}

func (a *Analyzer) emit(op string, arg1 string, arg2 string, result string) {
	a.quads = append(a.quads, Quad{
		Index:  len(a.quads) + 1,
		Op:     op,
		Arg1:   arg1,
		Arg2:   arg2,
		Result: result,
	})
}

func (a *Analyzer) current() lexer.Token {
	if a.pos >= len(a.tokens) {
		return lexer.Token{Kind: "eof", Text: "#"}
	}
	return a.tokens[a.pos]
}

func (a *Analyzer) advance() {
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
	tok := a.current()
	a.errors = append(a.errors, fmt.Sprintf("第%d行第%d列: %s，当前单词为 %q", tok.Line, tok.Column, message, tok.Text))
}

func (a *Analyzer) skipToStmtEnd() {
	for !a.isEOF() && !a.checkText(";") && !a.checkText("}") {
		a.advance()
	}
	if a.checkText(";") {
		a.advance()
	}
}

func (a *Analyzer) typeLength(typ string) int {
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
	info, ok := a.types[typ]
	if !ok || info.Kind != "struct" {
		return "", false
	}
	fieldType, ok := info.Fields[fieldName]
	return fieldType, ok
}

func formatFields(fieldNames []string, fields map[string]string) string {
	parts := []string{}
	for _, name := range fieldNames {
		parts = append(parts, name+":"+fields[name])
	}
	return strings.Join(parts, ",")
}

func literalType(text string) string {
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
	if op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" ||
		op == "&&" || op == "||" {
		return "bool"
	}
	if leftType == "float" || rightType == "float" {
		return "float"
	}
	if leftType == "unknown" || rightType == "unknown" {
		return "unknown"
	}
	return "int"
}

func canAssign(leftType string, rightType string) bool {
	if leftType == "unknown" || rightType == "unknown" {
		return true
	}
	if leftType == rightType {
		return true
	}
	return leftType == "float" && rightType == "int"
}
