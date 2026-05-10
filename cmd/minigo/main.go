package main

import (
	"fmt"
	"os"

	"minigo/internal/lexer"
	"minigo/internal/parser"
	"minigo/internal/semantic"
)

func main() {
	// 默认读取示例程序，也可以从命令行传入源程序路径。
	filename := "examples/basic.mg"
	if len(os.Args) >= 2 {
		filename = os.Args[1]
	}

	sourceBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("读取源程序失败:", err)
		return
	}

	// 第一阶段：词法分析，生成 Token 序列、标识符表、常数表和错误信息。
	lexResult := lexer.Scan(string(sourceBytes))
	lexer.PrintResult(filename, lexResult)

	// 第二阶段：语法分析。只有词法分析无错误时才继续做递归下降分析。
	if len(lexResult.Errors) == 0 {
		parseResult := parser.Parse(lexResult.Tokens)
		parser.PrintResult(parseResult)
		if len(parseResult.Errors) == 0 {
			semanticResult := semantic.Analyze(lexResult.Tokens)
			semantic.PrintResult(semanticResult)
		}
	}
}
