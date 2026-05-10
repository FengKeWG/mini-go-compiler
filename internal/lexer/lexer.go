package lexer

import (
	"fmt"
	"unicode"
)

// Token 表示词法分析得到的一个单词
type Token struct {
	Kind   string // 单词类别，k=关键字，p=界符/运算符，i=标识符，c=常数，eof=文件结束
	Value  int    // 单词编号
	Text   string // 单词原文
	Line   int    // 单词所在行号
	Column int    // 单词所在列号
}

// Result 保存词法分析阶段的全部输出结果
type Result struct {
	Tokens      []Token  // Token 序列
	Identifiers []string // 标识符表
	Constants   []string // 常数表
	Errors      []string // 词法错误信息
}

// keywords 关键字表
var keywords = []string{
	"func", "var", "int", "bool", "if", "else", "for", "true", "false",
	"return", "string", "char", "float", "const", "break", "continue", "type", "struct",
}

// delimiters 界符表和运算符表
var delimiters = []string{
	"(", ")", "{", "}", ";", "=", "+", "-", "*", "/", "<", ">",
	"<=", ">=", "==", "!=", "&&", "||", ",",
	"!", "%", "[", "]", ":", ":=", ".",
	"&", "|", "^", "&^", "<<", ">>",
}

// Scan 从左到右扫描源程序，生成 Token 序列、标识符表、常数表和错误信息
func Scan(source string) Result {
	var result Result

	runes := []rune(source)
	i := 0
	line := 1
	column := 1

	for i < len(runes) {
		ch := runes[i]

		// 跳过换行，并维护行号和列号
		if ch == '\n' {
			i++
			line++
			column = 1
			continue
		}

		// 跳过空格、制表符等空白字符
		if unicode.IsSpace(ch) {
			i++
			column++
			continue
		}

		// 跳过单行注释：// ...
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '/' {
			i, column = skipLineComment(runes, i, column)
			continue
		}

		// 跳过块注释：/* ... */
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			var ok bool
			startLine := line
			startColumn := column
			i, line, column, ok = skipBlockComment(runes, i, line, column)
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("第%d行第%d列: 块注释没有结束", startLine, startColumn))
			}
			continue
		}

		startLine := line
		startColumn := column

		// 识别关键字或标识符
		if isLetter(ch) {
			var word string
			word, i, column = readIdentifier(runes, i, column)
			if index := findIndex(keywords, word); index > 0 {
				result.Tokens = append(result.Tokens, Token{"k", index, word, startLine, startColumn})
			} else {
				index := addToTable(&result.Identifiers, word)
				result.Tokens = append(result.Tokens, Token{"i", index, word, startLine, startColumn})
			}
			continue
		}

		// 识别整数常量或简单实数常量
		if unicode.IsDigit(ch) {
			var number string
			number, i, column = readNumber(runes, i, column)
			index := addToTable(&result.Constants, number)
			result.Tokens = append(result.Tokens, Token{"c", index, number, startLine, startColumn})
			continue
		}

		// 识别字符串常量
		if ch == '"' {
			var text string
			var ok bool
			text, i, column, ok = readQuoted(runes, i, column, '"')
			index := addToTable(&result.Constants, text)
			result.Tokens = append(result.Tokens, Token{"c", index, text, startLine, startColumn})
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("第%d行第%d列: 字符串常量没有结束", startLine, startColumn))
			}
			continue
		}

		// 识别字符常量
		if ch == '\'' {
			var text string
			var ok bool
			text, i, column, ok = readQuoted(runes, i, column, '\'')
			index := addToTable(&result.Constants, text)
			result.Tokens = append(result.Tokens, Token{"c", index, text, startLine, startColumn})
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("第%d行第%d列: 字符常量没有结束", startLine, startColumn))
			}
			continue
		}

		// 先尝试识别两个字符的界符，<=、>=、==、!=、&&、||
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			if index := findIndex(delimiters, two); index > 0 {
				result.Tokens = append(result.Tokens, Token{"p", index, two, startLine, startColumn})
				i += 2
				column += 2
				continue
			}
		}

		// 再识别单字符界符，+、-、*、/、{、}
		one := string(ch)
		if index := findIndex(delimiters, one); index > 0 {
			result.Tokens = append(result.Tokens, Token{"p", index, one, startLine, startColumn})
			i++
			column++
			continue
		}

		result.Errors = append(result.Errors, fmt.Sprintf("第%d行第%d列: 无法识别的字符 %q", line, column, ch))
		i++
		column++
	}

	result.Tokens = append(result.Tokens, Token{"eof", 0, "#", line, column})
	return result
}

// skipLineComment 跳过单行注释
func skipLineComment(runes []rune, i int, column int) (int, int) {
	i += 2
	column += 2

	// 一直跳到换行符前，换行符交给 Scan 主循环统一处理
	for i < len(runes) && runes[i] != '\n' {
		i++
		column++
	}
	return i, column
}

// skipBlockComment 跳过块注释
func skipBlockComment(runes []rune, i int, line int, column int) (int, int, int, bool) {
	i += 2
	column += 2

	// 在注释内部寻找结束标志 */
	for i < len(runes) {
		if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
			i += 2
			column += 2
			return i, line, column, true
		}

		// 块注释可能跨行，所以要维护行号和列号
		if runes[i] == '\n' {
			i++
			line++
			column = 1
		} else {
			i++
			column++
		}
	}

	// 扫到文件末尾仍然没有遇到 */，说明块注释没有正常结束
	return i, line, column, false
}

func readIdentifier(runes []rune, i int, column int) (string, int, int) {
	// 标识符规则：字母或下划线开头，后面可以跟字母、数字或下划线
	start := i
	for i < len(runes) && (isLetter(runes[i]) || unicode.IsDigit(runes[i])) {
		i++
		column++
	}
	// 返回识别出的单词、扫描结束后的下标和列号
	return string(runes[start:i]), i, column
}

func readNumber(runes []rune, i int, column int) (string, int, int) {
	// 支持整数和简单小数，例如 123、3.14
	start := i
	hasDot := false
	for i < len(runes) {
		if unicode.IsDigit(runes[i]) {
			i++
			column++
		} else if runes[i] == '.' && !hasDot && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) {
			hasDot = true
			i++
			column++
		} else {
			break
		}
	}

	return string(runes[start:i]), i, column
}

func readQuoted(runes []rune, i int, column int, quote rune) (string, int, int, bool) {
	// quote 可以是双引号 "，也可以是单引号 '。
	// 因此这个函数同时用于读取字符串常量和字符常量。
	start := i

	// 先跳过开头的引号
	i++
	column++

	for i < len(runes) {
		if runes[i] == quote {
			i++
			column++
			return string(runes[start:i]), i, column, true
		}

		// 遇到转义字符时，把反斜杠和后面的字符一起跳过
		if runes[i] == '\\' && i+1 < len(runes) {
			i += 2
			column += 2
		} else if runes[i] == '\n' {
			break
		} else {
			i++
			column++
		}
	}

	// 没有读到结尾引号，返回 false，外层负责记录错误
	return string(runes[start:i]), i, column, false
}

func isLetter(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func findIndex(table []string, text string) int {
	// 表的编号从 1 开始；返回 0 表示没有找到
	for i, item := range table {
		if item == text {
			return i + 1
		}
	}
	return 0
}

func addToTable(table *[]string, text string) int {
	// 如果表中已经存在该单词，直接返回原编号
	for i, item := range *table {
		if item == text {
			return i + 1
		}
	}
	// 否则插入表尾，并返回新编号
	*table = append(*table, text)
	return len(*table)
}
