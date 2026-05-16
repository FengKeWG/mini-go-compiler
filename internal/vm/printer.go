package vm

import "fmt"

// PrintResult 输出目标指令运行平台的执行结果
func PrintResult(result Result) {
	fmt.Println("目标指令运行平台:")
	if len(result.Errors) > 0 {
		fmt.Println("运行错误:")
		for _, err := range result.Errors {
			fmt.Println("  " + err)
		}
		fmt.Println()
		return
	}

	fmt.Println("返回值:", result.ReturnValue)
	fmt.Println()

	fmt.Println("最终变量表:")
	fmt.Printf("  %-18s %-12s\n", "变量", "值")
	for _, variable := range result.Variables {
		fmt.Printf("  %-18s %-12s\n", variable.Name, variable.Value)
	}
	fmt.Println()

	fmt.Println("执行轨迹:")
	fmt.Printf("  %-6s %-6s %-24s %-12s\n", "步数", "PC", "指令", "R0")
	for _, item := range result.Trace {
		fmt.Printf("  %-6d %-6d %-24s %-12s\n", item.Step, item.PC, item.Instruction, item.R0)
	}
	fmt.Println()
}
