package codegen

import (
	"fmt"
	"strings"
)

// PrintResult 输出目标代码阶段的指令集合、活跃信息和目标代码
func PrintResult(result Result) {
	fmt.Println("目标代码生成结果:")
	// 按课设要求依次输出指令集合、活跃信息和目标代码序列
	printInstructionSet(result.InstructionSet)
	printLiveBlocks(result.LiveBlocks)
	printInstructions(result.Instructions)
}

// printInstructionSet 输出虚拟目标机支持的指令说明
func printInstructionSet(instructionSet []string) {
	fmt.Println("目标代码指令集合:")
	for _, item := range instructionSet {
		// 每一行是一个虚拟目标机指令说明
		fmt.Println("  " + item)
	}
	fmt.Println()
}

// printLiveBlocks 输出每个基本块入口和出口的活跃变量
func printLiveBlocks(blocks []LiveBlock) {
	fmt.Println("活跃信息摘要:")
	printCodegenRow("基本块", "四元式范围", "入口活跃", "出口活跃")
	for _, block := range blocks {
		// 每个基本块显示入口和出口两个集合
		printCodegenRow(
			fmt.Sprintf("B%d", block.BlockIndex),
			fmt.Sprintf("%d-%d", block.Start, block.End),
			formatNames(block.Entry),
			formatNames(block.Exit),
		)
	}
	fmt.Println()
}

// printInstructions 输出由四元式翻译得到的目标代码序列
func printInstructions(instructions []Instruction) {
	fmt.Println("目标代码序列:")
	for _, inst := range instructions {
		// 输出格式接近课件中的 OP R, M 形式
		fmt.Printf("  %-4d %-8s %-8s %s\n", inst.Index, inst.Op, inst.Arg1+",", inst.Arg2)
	}
	fmt.Println()
}

// formatNames 把活跃变量名列表压缩成一段较短的文本
func formatNames(names []string) string {
	if len(names) == 0 {
		// 没有活跃变量时用短横线表示空集合
		return "-"
	}
	showCount := len(names)
	if showCount > 4 {
		// 终端输出太宽会很乱，所以只展示前几个变量
		showCount = 4
	}
	text := fmt.Sprintf("%d个:%s", len(names), strings.Join(names[:showCount], ","))
	if len(names) > showCount {
		text += "..."
	}
	return text
}

// printCodegenRow 按显示宽度输出一行，避免中文列名导致表格歪
func printCodegenRow(block string, quad string, before string, after string) {
	fmt.Println("  " + padDisplay(block, 8) + padDisplay(quad, 14) + padDisplay(before, 28) + after)
}

// padDisplay 根据终端显示宽度补空格
func padDisplay(text string, width int) string {
	// displayWidth 估算显示宽度，中文按两个英文字符宽度计算
	spaceCount := width - displayWidth(text)
	if spaceCount < 1 {
		spaceCount = 1
	}
	return text + strings.Repeat(" ", spaceCount)
}

// displayWidth 估算字符串在终端里占用的显示宽度
func displayWidth(text string) int {
	width := 0
	for _, ch := range text {
		if ch > 127 {
			// 中文字符在大多数终端中占两个英文宽度
			width += 2
		} else {
			width++
		}
	}
	return width
}
