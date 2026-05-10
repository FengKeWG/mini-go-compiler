package lexer

import "fmt"

// PrintResult 按课程设计要求输出词法分析阶段的各类表和 Token 序列。
func PrintResult(filename string, result Result) {
	fmt.Println("源程序文件:", filename)
	fmt.Println()

	printSimpleTable("关键字表 K", keywords)
	printSimpleTable("界符表 P", delimiters)
	printSimpleTable("标识符表 I", result.Identifiers)
	printSimpleTable("常数表 C", result.Constants)
	printTokens(result.Tokens)
	printTokenDetails(result.Tokens)
	printErrors(result.Errors)
}

func printSimpleTable(title string, table []string) {
	fmt.Println(title + ":")
	if len(table) == 0 {
		fmt.Println("  空")
		fmt.Println()
		return
	}
	for i, item := range table {
		fmt.Printf("  %2d  %s\n", i+1, item)
	}
	fmt.Println()
}

func printTokens(tokens []Token) {
	fmt.Println("Token序列:")
	for i, token := range tokens {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("(%s,%d)", token.Kind, token.Value)
	}
	fmt.Println()
	fmt.Println()
}

func printTokenDetails(tokens []Token) {
	fmt.Println("Token详细信息:")
	fmt.Printf("  %-4s %-6s %-6s %-12s %-8s\n", "序号", "类别", "编号", "单词", "位置")
	for i, token := range tokens {
		pos := fmt.Sprintf("%d:%d", token.Line, token.Column)
		fmt.Printf("  %-4d %-6s %-6d %-12s %-8s\n", i+1, token.Kind, token.Value, token.Text, pos)
	}
	fmt.Println()
}

func printErrors(errors []string) {
	if len(errors) == 0 {
		return
	}
	fmt.Println("错误信息:")
	for _, err := range errors {
		fmt.Println(err)
	}
}
