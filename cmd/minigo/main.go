package main

import (
	"fmt"
	"os"

	"minigo/internal/codegen"
	"minigo/internal/lexer"
	"minigo/internal/optimizer"
	"minigo/internal/parser"
	"minigo/internal/semantic"
)

func main() {
	// 默认读取示例程序，也可以从命令行传入源程序路径
	filename := "examples/basic.mg"
	if len(os.Args) >= 2 {
		filename = os.Args[1]
	}

	sourceBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("读取源程序失败:", err)
		return
	}

	// 第一阶段：词法分析，生成 Token 序列、标识符表、常数表和错误信息
	lexResult := lexer.Scan(string(sourceBytes))
	lexer.PrintResult(filename, lexResult)
	if len(lexResult.Errors) > 0 {
		return
	}

	// 第二阶段：语法分析，使用递归下降子程序检查语法
	parseResult := parser.Parse(lexResult.Tokens)
	parser.PrintResult(parseResult)
	if len(parseResult.Errors) > 0 {
		return
	}

	// 第三阶段：语义分析，生成符号表、活动记录和四元式
	semanticResult := semantic.Analyze(lexResult.Tokens)
	semantic.PrintResult(semanticResult)
	if len(semanticResult.Errors) > 0 {
		return
	}

	// 第四阶段：基础优化，对四元式做基本块划分和简单优化
	optimizeResult := optimizer.Optimize(semanticResult.Quads)
	optimizer.PrintResult(optimizeResult)

	// 第五阶段：目标代码生成，输出活跃信息和目标指令序列
	codegenResult := codegen.Generate(optimizeResult.Optimized)
	codegen.PrintResult(codegenResult)
}
