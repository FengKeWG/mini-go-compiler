package parser

import (
	"fmt"

	"minigo/internal/lexer"
)

// Result 保存语法分析阶段的结果
type Result struct {
	Errors []string // 语法错误信息
}

// Parser 递归下降语法分析器
// 保存 Token 序列和当前位置
type Parser struct {
	tokens []lexer.Token // 词法分析传来的 Token 序列
	pos    int           // 当前正在分析的 Token 下标
	errors []string      // 递归下降过程中发现的语法错误
}

// Parse 对词法分析生成的 Token 序列做递归下降
func Parse(tokens []lexer.Token) Result {
	p := Parser{tokens: tokens}
	p.parseProgram()
	return Result{Errors: p.errors}
}

// PrintResult 输出语法分析结果
func PrintResult(result Result) {
	fmt.Println("语法分析结果:")
	if len(result.Errors) == 0 {
		fmt.Println("  递归下降语法分析成功")
		fmt.Println()
		return
	}
	for _, err := range result.Errors {
		fmt.Println(" ", err)
	}
	fmt.Println()
}

// <程序> -> { <类型声明语句> | <函数定义> }
func (p *Parser) parseProgram() {
	// 顶层只允许出现结构体类型声明和函数定义
	for !p.isEOF() {
		if p.checkKeyword("type") {
			p.parseTypeDecl()
		} else {
			p.parseFuncDecl()
		}
	}
}

// <函数定义> -> func <标识符> ( <参数列表> ) <返回类型> <复合语句>
func (p *Parser) parseFuncDecl() {
	// 这里按照 Go 风格函数头进行匹配，例如 func add(x int) int
	p.expectKeyword("func")
	p.expectKind("i", "函数名")
	p.expectText("(")
	p.parseParamList()
	p.expectText(")")
	p.parseReturnType()
	p.parseBlock()
}

// <参数列表> -> <参数> { , <参数> } | ε
func (p *Parser) parseParamList() {
	// 右括号说明参数列表为空
	if p.checkText(")") {
		return
	}
	p.parseParam()
	for p.matchText(",") {
		p.parseParam()
	}
}

// <参数> -> <标识符> <类型>
func (p *Parser) parseParam() {
	// 参数形式采用 name type，和 Go 的写法一致
	p.expectKind("i", "参数名")
	p.parseType()
}

// <复合语句> -> { <语句表> }
func (p *Parser) parseBlock() {
	p.expectText("{")
	// 遇到右大括号之前不断识别语句
	for !p.checkText("}") && !p.isEOF() {
		p.parseStmt()
	}
	p.expectText("}")
}

// <语句> -> <变量声明语句> | <常量声明语句> | <赋值语句> | <条件语句>
//
//	| <for循环语句> | <break语句> | <continue语句> | <return语句> | <复合语句> | ;
func (p *Parser) parseStmt() {
	// 通过当前 Token 判断这一句属于哪一种语句
	if p.checkKeyword("var") {
		p.parseVarDecl()
		return
	}
	if p.checkKeyword("const") {
		p.parseConstDecl()
		return
	}
	if p.checkKeyword("type") {
		p.parseTypeDecl()
		return
	}
	if p.checkKeyword("if") {
		p.parseIfStmt()
		return
	}
	if p.checkKeyword("for") {
		p.parseForStmt()
		return
	}
	if p.checkKeyword("break") {
		p.parseBreakStmt()
		return
	}
	if p.checkKeyword("continue") {
		p.parseContinueStmt()
		return
	}
	if p.checkKeyword("return") {
		p.parseReturnStmt()
		return
	}
	if p.checkText("{") {
		p.parseBlock()
		return
	}
	if p.checkText(";") {
		// 空语句直接跳过分号
		p.advance()
		return
	}
	if p.checkKind("i") {
		p.parseIDStartStmt()
		return
	}
	p.addError("无法识别的语句开头")
	p.advance()
}

// <变量声明语句> -> var <标识符> <类型> ;
func (p *Parser) parseVarDecl() {
	p.expectKeyword("var")
	p.expectKind("i", "变量名")
	p.parseType()
	p.expectText(";")
}

// <类型声明语句> -> type <标识符> struct { <字段声明表> }
func (p *Parser) parseTypeDecl() {
	// 当前只支持结构体类型声明，用来覆盖课程设计中的结构体要求
	p.expectKeyword("type")
	p.expectKind("i", "类型名")
	p.expectKeyword("struct")
	p.expectText("{")
	// 字段形式为 字段名 类型 ;
	for !p.checkText("}") && !p.isEOF() {
		p.expectKind("i", "字段名")
		p.parseType()
		p.expectText(";")
	}
	p.expectText("}")
}

// <常量声明语句> -> const <标识符> <赋值运算符> <表达式> ;
func (p *Parser) parseConstDecl() {
	p.expectKeyword("const")
	p.expectKind("i", "常量名")
	if p.matchText(":=") || p.matchText("=") {
		p.parseExpr()
	} else {
		p.addError("常量声明缺少 := 或 =")
	}
	p.expectText(";")
}

// <类型> -> <基本类型> | <数组类型> | <标识符>
func (p *Parser) parseType() {
	// 基本类型直接消费关键字
	if p.matchKeyword("int") || p.matchKeyword("bool") || p.matchKeyword("float") ||
		p.matchKeyword("char") || p.matchKeyword("string") {
		return
	}
	// 数组类型形如 [10]int，可以递归支持多维数组
	if p.matchText("[") {
		p.expectKind("c", "数组长度")
		p.expectText("]")
		p.parseType()
		return
	}
	// 标识符类型一般是结构体类型名
	if p.matchKind("i") {
		return
	}
	p.addError("缺少类型名")
}

// <返回类型> -> <类型> | ε
func (p *Parser) parseReturnType() {
	// 没有返回类型时表示 void 风格函数
	if p.isTypeKeyword() {
		p.parseType()
	}
}

// 以标识符开头的语句有多种形式：
// <赋值语句> -> <左值> <赋值运算符> <表达式> ;
// <函数调用语句> -> <函数调用> ;
// <扩展语句> -> <标识符> : <表达式> { , <表达式> } ;
func (p *Parser) parseIDStartStmt() {
	// 标识符后面紧跟左括号，说明这是函数调用语句
	if p.nextText("(") {
		p.parseCall()
		p.expectText(";")
		return
	}

	// 否则先按左值读取，左值可能是变量、数组元素或结构体字段
	p.parseDesignator()
	if p.matchText("=") || p.matchText(":=") {
		p.parseExpr()
		p.expectText(";")
		return
	}
	// 这个分支主要用于当前示例中的 pair: a, b;，同时覆盖冒号和逗号的词法测试
	if p.matchText(":") {
		p.parseExpr()
		for p.matchText(",") {
			p.parseExpr()
		}
		p.expectText(";")
		return
	}
	p.addError("标识符后面应为 =、:= 或 :")
	p.skipToStmtEnd()
}

// <左值> -> <标识符> { [ <表达式> ] | 点 <标识符> }
func (p *Parser) parseDesignator() {
	// Designator 表示可以被赋值或被读取的位置
	p.expectKind("i", "标识符")
	p.parseDesignatorSuffix()
}

// <左值后缀> -> [ <表达式> ] <左值后缀> | 点 <标识符> <左值后缀> | ε
func (p *Parser) parseDesignatorSuffix() {
	for {
		if p.matchText("[") {
			// 数组下标访问，例如 scores[i]
			p.parseExpr()
			p.expectText("]")
			continue
		}
		if p.matchText(".") {
			// 结构体字段访问，例如 stu 的 age 字段
			p.expectKind("i", "字段名")
			continue
		}
		break
	}
}

// <函数调用> -> <标识符> ( <实参列表> )
func (p *Parser) parseCall() {
	p.expectKind("i", "函数名")
	p.expectText("(")
	p.parseArgumentList()
	p.expectText(")")
}

// <实参列表> -> <表达式> { , <表达式> } | ε
func (p *Parser) parseArgumentList() {
	if p.checkText(")") {
		return
	}
	p.parseExpr()
	for p.matchText(",") {
		p.parseExpr()
	}
}

// <条件语句> -> if <表达式> <复合语句> [ else <复合语句> ]
func (p *Parser) parseIfStmt() {
	// if 后面直接跟表达式和代码块，不使用括号
	p.expectKeyword("if")
	p.parseExpr()
	p.parseBlock()
	if p.matchKeyword("else") {
		p.parseBlock()
	}
}

// <for循环语句> -> for <表达式> <复合语句>
func (p *Parser) parseForStmt() {
	// 当前 for 只支持条件循环，形式接近 Go 的 for condition { }
	p.expectKeyword("for")
	p.parseExpr()
	p.parseBlock()
}

// <break语句> -> break ;
func (p *Parser) parseBreakStmt() {
	p.expectKeyword("break")
	p.expectText(";")
}

// <continue语句> -> continue ;
func (p *Parser) parseContinueStmt() {
	p.expectKeyword("continue")
	p.expectText(";")
}

// <return语句> -> return [ <表达式> ] ;
func (p *Parser) parseReturnStmt() {
	p.expectKeyword("return")
	// return 后面可以没有表达式，用于无返回值函数
	if !p.checkText(";") {
		p.parseExpr()
	}
	p.expectText(";")
}

// <表达式> -> <逻辑或表达式>
func (p *Parser) parseExpr() {
	// 表达式从最低优先级的逻辑或开始向下递归
	p.parseOr()
}

// <逻辑或表达式> -> <逻辑与表达式> { || <逻辑与表达式> }
func (p *Parser) parseOr() {
	p.parseAnd()
	for p.matchText("||") {
		p.parseAnd()
	}
}

// <逻辑与表达式> -> <关系表达式> { && <关系表达式> }
func (p *Parser) parseAnd() {
	p.parseCompare()
	for p.matchText("&&") {
		p.parseCompare()
	}
}

// <关系表达式> -> <加法表达式> { <关系运算符> <加法表达式> }
func (p *Parser) parseCompare() {
	p.parseAdd()
	for p.matchText("==") || p.matchText("!=") || p.matchText("<") ||
		p.matchText("<=") || p.matchText(">") || p.matchText(">=") {
		p.parseAdd()
	}
}

// <加法表达式> -> <乘法表达式> { (+ | - | '|' | ^) <乘法表达式> }
func (p *Parser) parseAdd() {
	p.parseMul()
	for p.matchText("+") || p.matchText("-") || p.matchText("|") || p.matchText("^") {
		p.parseMul()
	}
}

// <乘法表达式> -> <一元表达式> { (* | / | % | << | >> | & | &^) <一元表达式> }
func (p *Parser) parseMul() {
	p.parseUnary()
	for p.matchText("*") || p.matchText("/") || p.matchText("%") ||
		p.matchText("<<") || p.matchText(">>") || p.matchText("&") || p.matchText("&^") {
		p.parseUnary()
	}
}

// <一元表达式> -> (! | - | + | ^ | & | *) <一元表达式> | <基本表达式>
func (p *Parser) parseUnary() {
	if p.matchText("!") || p.matchText("-") || p.matchText("+") ||
		p.matchText("^") || p.matchText("&") || p.matchText("*") {
		p.parseUnary()
		return
	}
	p.parsePrimary()
}

// <基本表达式> -> <标识符> | <函数调用> | <常数> | true | false | ( <表达式> )
func (p *Parser) parsePrimary() {
	if p.checkKind("i") {
		// 标识符后面是左括号时作为函数调用，否则作为普通左值
		if p.nextText("(") {
			p.parseCall()
		} else {
			p.parseDesignator()
		}
		return
	}
	if p.matchKind("c") {
		return
	}
	if p.matchKeyword("true") || p.matchKeyword("false") {
		return
	}
	if p.matchText("(") {
		// 括号表达式会重新进入完整表达式分析
		p.parseExpr()
		p.expectText(")")
		return
	}
	p.addError("缺少表达式")
	p.advance()
}

func (p *Parser) current() lexer.Token {
	// 越界时返回 eof，避免错误恢复时数组越界
	if p.pos >= len(p.tokens) {
		return lexer.Token{Kind: "eof", Text: "#"}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() {
	// 向前移动一个 Token
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *Parser) isEOF() bool {
	// eof 是词法分析主动补上的结束标记
	return p.current().Kind == "eof"
}

func (p *Parser) checkKind(kind string) bool {
	// 只检查类别，不移动当前位置
	return p.current().Kind == kind
}

func (p *Parser) checkText(text string) bool {
	// 只检查原文，不移动当前位置
	return p.current().Text == text
}

func (p *Parser) checkKeyword(word string) bool {
	tok := p.current()
	return tok.Kind == "k" && tok.Text == word
}

func (p *Parser) nextText(text string) bool {
	// 查看下一个 Token，用于区分函数调用和普通标识符
	if p.pos+1 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Text == text
}

func (p *Parser) isTypeKeyword() bool {
	return p.checkKeyword("int") || p.checkKeyword("bool") || p.checkKeyword("float") ||
		p.checkKeyword("char") || p.checkKeyword("string") || p.checkText("[") || p.checkKind("i")
}

func (p *Parser) matchKind(kind string) bool {
	// 匹配成功就消费 Token，匹配失败不报错
	if p.checkKind(kind) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) matchText(text string) bool {
	if p.checkText(text) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) matchKeyword(word string) bool {
	if p.checkKeyword(word) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expectKind(kind string, name string) {
	// expect 表示当前位置必须满足要求，不满足就记录错误
	if p.checkKind(kind) {
		p.advance()
		return
	}
	p.addError("缺少" + name)
}

func (p *Parser) expectText(text string) {
	if p.checkText(text) {
		p.advance()
		return
	}
	p.addError("缺少 " + text)
}

func (p *Parser) expectKeyword(word string) {
	if p.checkKeyword(word) {
		p.advance()
		return
	}
	p.addError("缺少关键字 " + word)
}

func (p *Parser) addError(message string) {
	// 错误信息带上当前 Token 的行列，方便在源程序中定位
	tok := p.current()
	p.errors = append(p.errors, fmt.Sprintf("第%d行第%d列: %s，当前单词为 %q", tok.Line, tok.Column, message, tok.Text))
}

func (p *Parser) skipToStmtEnd() {
	// 出错后跳到分号或右大括号，避免一个错误引发大量连锁错误
	for !p.isEOF() && !p.checkText(";") && !p.checkText("}") {
		p.advance()
	}
	if p.checkText(";") {
		p.advance()
	}
}
