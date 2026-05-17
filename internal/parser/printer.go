package parser

import (
	"fmt"
	"strings"
)

// printOperatorDetails 输出算符优先分析相关表格
func printOperatorDetails(result Result) {
	fmt.Println("表达式算符文法:")
	for i, p := range result.Productions {
		fmt.Printf("  %-4d %s -> %s\n", i+1, p.Left, strings.Join(p.Right, " "))
	}
	fmt.Println()

	fmt.Println("FIRSTVT / LASTVT:")
	fmt.Printf("  %-10s %-40s %-40s\n", "非终结符", "FIRSTVT", "LASTVT")
	for _, row := range result.SetRows {
		fmt.Printf("  %-10s %-40s %-40s\n", row.NonTerminal, row.FirstVT, row.LastVT)
	}
	fmt.Println()

	fmt.Println("算符优先关系表:")
	fmt.Printf("  %-12s %-12s %-8s\n", "栈顶终结符", "当前输入", "关系")
	for _, row := range result.Relations {
		fmt.Printf("  %-12s %-12s %-8s\n", row.StackTop, row.Input, row.Relation)
	}
	fmt.Println()

	fmt.Println("算符优先分析过程:")
	fmt.Printf("  %-24s %-6s %-24s %-24s %-8s %s\n", "表达式", "步数", "符号栈", "输入串", "关系", "动作")
	for _, step := range result.Steps {
		fmt.Printf("  %-24s %-6d %-24s %-24s %-8s %s\n",
			step.Expression, step.Step, step.Stack, step.Input, step.Relation, step.Action)
	}
	fmt.Println()
}
