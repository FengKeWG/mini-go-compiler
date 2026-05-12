package optimizer

import (
	"fmt"

	"minigo/internal/semantic"
)

// PrintResult 输出优化阶段的基本块、优化统计和优化后的四元式
func PrintResult(result Result) {
	fmt.Println("优化结果:")
	printBasicBlocks(result.Blocks)
	printOptimizeSteps(result.Steps)
	printOptimizedQuads(result.Optimized)
}

// printBasicBlocks 输出每个基本块包含的四元式编号范围
func printBasicBlocks(blocks []Block) {
	fmt.Println("基本块划分:")
	for _, block := range blocks {
		fmt.Printf("  B%d: 四元式 %d - %d\n", block.Index, block.Start, block.End)
	}
	fmt.Println()
}

// printOptimizeSteps 输出每一步优化前后四元式数量变化
func printOptimizeSteps(steps []Step) {
	fmt.Println("优化步骤:")
	for _, step := range steps {
		fmt.Printf("  %-16s 修改 %d 条，四元式数量 %d -> %d\n",
			step.Name, step.Changed, step.Before, step.After)
	}
	fmt.Println()
}

// printOptimizedQuads 输出优化后的四元式序列
func printOptimizedQuads(quads []semantic.Quad) {
	fmt.Println("优化后四元式序列:")
	for _, q := range quads {
		fmt.Printf("  %-4d (%s, %s, %s, %s)\n", q.Index, q.Op, q.Arg1, q.Arg2, q.Result)
	}
	fmt.Println()
}
