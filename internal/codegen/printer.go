package codegen

import (
	"fmt"
	"strings"
)

// PrintResult 输出目标代码阶段的指令集合、活跃信息和目标代码。
func PrintResult(result Result) {
	fmt.Println("目标代码生成结果:")
	printInstructionSet(result.InstructionSet)
	printLiveBlocks(result.LiveBlocks)
	printInstructions(result.Instructions)
}

func printInstructionSet(instructionSet []string) {
	fmt.Println("目标代码指令集合:")
	for _, item := range instructionSet {
		fmt.Println("  " + item)
	}
	fmt.Println()
}

func printLiveBlocks(blocks []LiveBlock) {
	fmt.Println("活跃信息摘要:")
	printCodegenRow("基本块", "四元式范围", "入口活跃", "出口活跃")
	for _, block := range blocks {
		printCodegenRow(
			fmt.Sprintf("B%d", block.BlockIndex),
			fmt.Sprintf("%d-%d", block.Start, block.End),
			formatNames(block.Entry),
			formatNames(block.Exit),
		)
	}
	fmt.Println()
}

func printInstructions(instructions []Instruction) {
	fmt.Println("目标代码序列:")
	for _, inst := range instructions {
		fmt.Printf("  %-4d %-8s %-8s %s\n", inst.Index, inst.Op, inst.Arg1+",", inst.Arg2)
	}
	fmt.Println()
}

func formatNames(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	showCount := len(names)
	if showCount > 4 {
		showCount = 4
	}
	text := fmt.Sprintf("%d个:%s", len(names), strings.Join(names[:showCount], ","))
	if len(names) > showCount {
		text += "..."
	}
	return text
}

// printCodegenRow 按显示宽度输出一行，避免中文列名导致表格歪。
func printCodegenRow(block string, quad string, before string, after string) {
	fmt.Println("  " + padDisplay(block, 8) + padDisplay(quad, 14) + padDisplay(before, 28) + after)
}

func padDisplay(text string, width int) string {
	spaceCount := width - displayWidth(text)
	if spaceCount < 1 {
		spaceCount = 1
	}
	return text + strings.Repeat(" ", spaceCount)
}

func displayWidth(text string) int {
	width := 0
	for _, ch := range text {
		if ch > 127 {
			width += 2
		} else {
			width++
		}
	}
	return width
}
