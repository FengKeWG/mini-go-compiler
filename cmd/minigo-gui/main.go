package main

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"minigo/internal/codegen"
	"minigo/internal/lexer"
	"minigo/internal/optimizer"
	"minigo/internal/parser"
	"minigo/internal/semantic"
)

type pageSet struct {
	lexerText    *widget.Label
	parserText   *widget.Label
	symbolText   *widget.Label
	quadText     *widget.Label
	optimizeText *widget.Label
	codegenText  *widget.Label
	errorText    *widget.Label
}

type analyzeView struct {
	lexerText    string
	parserText   string
	symbolText   string
	quadText     string
	optimizeText string
	codegenText  string
	errorText    string
	statusText   string
}

func main() {
	a := app.New()
	w := a.NewWindow("MiniGo 编译器可视化")
	w.Resize(fyne.NewSize(1200, 760))

	sourceInput := widget.NewMultiLineEntry()
	sourceInput.TextStyle = fyne.TextStyle{Monospace: true}
	sourceInput.Wrapping = fyne.TextWrapOff
	sourceInput.Scroll = fyne.ScrollBoth
	sourceInput.SetText(loadExample())

	pages := pageSet{
		lexerText:    newOutputLabel("等待分析"),
		parserText:   newOutputLabel("等待分析"),
		symbolText:   newOutputLabel("等待分析"),
		quadText:     newOutputLabel("等待分析"),
		optimizeText: newOutputLabel("等待分析"),
		codegenText:  newOutputLabel("等待分析"),
		errorText:    newOutputLabel("暂无错误"),
	}

	statusLabel := widget.NewLabel("已载入示例程序")

	var analyzeButton *widget.Button
	analyzeButton = widget.NewButton("开始分析", func() {
		source := sourceInput.Text
		analyzeButton.Disable()
		statusLabel.SetText("正在分析...")

		// 编译过程放到后台执行，避免点击按钮后界面卡住。
		go func() {
			result := analyzeSource(source)
			fyne.Do(func() {
				pages.lexerText.SetText(result.lexerText)
				pages.parserText.SetText(result.parserText)
				pages.symbolText.SetText(result.symbolText)
				pages.quadText.SetText(result.quadText)
				pages.optimizeText.SetText(result.optimizeText)
				pages.codegenText.SetText(result.codegenText)
				pages.errorText.SetText(result.errorText)
				statusLabel.SetText(result.statusText)
				analyzeButton.Enable()
			})
		}()
	})

	loadButton := widget.NewButton("重新载入示例", func() {
		sourceInput.SetText(loadExample())
		statusLabel.SetText("已重新载入 examples/basic.mg")
	})

	leftTitle := widget.NewLabel("源程序")
	leftTitle.TextStyle = fyne.TextStyle{Bold: true}
	leftTools := container.NewHBox(analyzeButton, loadButton)
	leftPanel := container.NewBorder(
		container.NewVBox(leftTitle, leftTools),
		nil,
		nil,
		nil,
		sourceInput,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("词法分析", container.NewScroll(pages.lexerText)),
		container.NewTabItem("语法分析", container.NewScroll(pages.parserText)),
		container.NewTabItem("符号表", container.NewScroll(pages.symbolText)),
		container.NewTabItem("四元式", container.NewScroll(pages.quadText)),
		container.NewTabItem("优化", container.NewScroll(pages.optimizeText)),
		container.NewTabItem("目标代码", container.NewScroll(pages.codegenText)),
		container.NewTabItem("错误信息", container.NewScroll(pages.errorText)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	split := container.NewHSplit(leftPanel, tabs)
	split.SetOffset(0.43)

	w.SetContent(container.NewBorder(nil, statusLabel, nil, nil, split))
	w.ShowAndRun()
}

func newOutputLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.TextStyle = fyne.TextStyle{Monospace: true}
	label.Wrapping = fyne.TextWrapOff
	return label
}

func loadExample() string {
	data, err := os.ReadFile("examples/basic.mg")
	if err == nil {
		return string(data)
	}
	return `func main() int {
    var a int;
    a = 1 + 2;
    return a;
}`
}

func analyzeSource(source string) analyzeView {
	view := analyzeView{
		parserText:   "未执行：上一阶段存在错误",
		symbolText:   "未执行：上一阶段存在错误",
		quadText:     "未执行：上一阶段存在错误",
		optimizeText: "未执行：上一阶段存在错误",
		codegenText:  "未执行：上一阶段存在错误",
		errorText:    "暂无错误",
	}

	lexResult := lexer.Scan(source)
	view.lexerText = limitLines(formatLexer(lexResult), 260)
	if len(lexResult.Errors) > 0 {
		view.errorText = formatErrors("词法错误", lexResult.Errors)
		view.statusText = fmt.Sprintf("词法分析发现 %d 个错误，后续阶段未执行", len(lexResult.Errors))
		return view
	}

	parseResult := parser.Parse(lexResult.Tokens)
	view.parserText = limitLines(formatParser(parseResult), 120)
	if len(parseResult.Errors) > 0 {
		view.errorText = formatErrors("语法错误", parseResult.Errors)
		view.statusText = fmt.Sprintf("语法分析发现 %d 个错误，后续阶段未执行", len(parseResult.Errors))
		return view
	}

	semanticResult := semantic.Analyze(lexResult.Tokens)
	view.symbolText = limitLines(formatSymbols(semanticResult.Symbols), 260)
	view.quadText = limitLines(formatQuads("语义分析生成的四元式", semanticResult.Quads), 260)
	if len(semanticResult.Errors) > 0 {
		view.errorText = formatErrors("语义错误", semanticResult.Errors)
		view.statusText = fmt.Sprintf("语义分析发现 %d 个错误，后续阶段未执行", len(semanticResult.Errors))
		return view
	}

	optimizeResult := optimizer.Optimize(semanticResult.Quads)
	view.optimizeText = limitLines(formatOptimizer(optimizeResult), 320)

	codegenResult := codegen.Generate(optimizeResult.Optimized)
	view.codegenText = limitLines(formatCodegen(codegenResult), 320)

	view.statusText = "分析完成：词法、语法、语义、优化、目标代码均已生成"
	return view
}

func formatLexer(result lexer.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Token 总数: %d\n", len(result.Tokens))
	fmt.Fprintf(&b, "标识符数量: %d\n", len(result.Identifiers))
	fmt.Fprintf(&b, "常数数量: %d\n", len(result.Constants))
	fmt.Fprintf(&b, "错误数量: %d\n\n", len(result.Errors))

	b.WriteString("Token 序列预览:\n")
	previewCount := len(result.Tokens)
	if previewCount > 80 {
		previewCount = 80
	}
	for i := 0; i < previewCount; i++ {
		token := result.Tokens[i]
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "(%s,%d)", token.Kind, token.Value)
	}
	if len(result.Tokens) > previewCount {
		fmt.Fprintf(&b, "\n... 还有 %d 个 Token，命令行版本可查看完整序列", len(result.Tokens)-previewCount)
	}

	b.WriteString("\n\nToken 详细信息:\n")
	fmt.Fprintf(&b, "%-5s %-6s %-6s %-16s %-8s\n", "序号", "类别", "编号", "单词", "位置")
	detailCount := len(result.Tokens)
	if detailCount > 160 {
		detailCount = 160
	}
	for i := 0; i < detailCount; i++ {
		token := result.Tokens[i]
		pos := fmt.Sprintf("%d:%d", token.Line, token.Column)
		fmt.Fprintf(&b, "%-5d %-6s %-6d %-16s %-8s\n", i+1, token.Kind, token.Value, token.Text, pos)
	}
	if len(result.Tokens) > detailCount {
		fmt.Fprintf(&b, "... 还有 %d 行 Token 详细信息未在界面中展开\n", len(result.Tokens)-detailCount)
	}

	b.WriteString("\n标识符表 I:\n")
	writeStringList(&b, result.Identifiers)

	b.WriteString("\n常数表 C:\n")
	writeStringList(&b, result.Constants)

	if len(result.Errors) > 0 {
		b.WriteString("\n词法错误:\n")
		writeErrors(&b, result.Errors)
	}
	return b.String()
}

func formatParser(result parser.Result) string {
	var b strings.Builder
	b.WriteString("递归下降语法分析结果:\n\n")
	if len(result.Errors) == 0 {
		b.WriteString("通过：源程序符合当前 MiniGo 文法。\n")
		return b.String()
	}
	writeErrors(&b, result.Errors)
	return b.String()
}

func formatSymbols(symbols []semantic.Symbol) string {
	var b strings.Builder
	b.WriteString("符号表总表 SYNBL:\n")
	fmt.Fprintf(&b, "%-5s %-18s %-14s %-8s %-8s %-8s %-18s\n",
		"序号", "NAME", "TYPE", "CAT", "ADDR", "LEN", "VALUE")
	for _, sym := range symbols {
		addr := fmt.Sprintf("%d", sym.Addr)
		if sym.Addr < 0 {
			addr = "-"
		}
		value := sym.Value
		if value == "" {
			value = "-"
		}
		fmt.Fprintf(&b, "%-5d %-18s %-14s %-8s %-8s %-8d %-18s\n",
			sym.Index, sym.Name, sym.Type, sym.Category, addr, sym.Length, value)
	}

	b.WriteString("\n活动记录表 VALL:\n")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "地址", "内容", "类型", "种类")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "0", "Old SP", "-", "连接数据")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "1", "返回地址", "-", "连接数据")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "2", "全局Display地址", "-", "连接数据")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "3", "参数个数", "-", "参数信息")
	fmt.Fprintf(&b, "%-8s %-24s %-14s %-10s\n", "4", "Display[0]", "-", "显示区表")
	for _, sym := range symbols {
		if sym.Addr < 0 {
			continue
		}
		category := "局部变量"
		if sym.Category == "p" {
			category = "形式参数"
		} else if sym.Category == "t" {
			category = "临时变量"
		}
		fmt.Fprintf(&b, "%-8d %-24s %-14s %-10s\n", sym.Addr, sym.Name, sym.Type, category)
	}
	return b.String()
}

func formatQuads(title string, quads []semantic.Quad) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	for _, q := range quads {
		fmt.Fprintf(&b, "%-5d (%s, %s, %s, %s)\n", q.Index, q.Op, q.Arg1, q.Arg2, q.Result)
	}
	return b.String()
}

func formatOptimizer(result optimizer.Result) string {
	var b strings.Builder
	b.WriteString("基本块划分:\n")
	for _, block := range result.Blocks {
		fmt.Fprintf(&b, "B%d: 四元式 %d - %d\n", block.Index, block.Start, block.End)
	}

	b.WriteString("\n优化步骤:\n")
	fmt.Fprintf(&b, "%-18s %-8s %-8s %-8s\n", "步骤", "修改", "优化前", "优化后")
	for _, step := range result.Steps {
		fmt.Fprintf(&b, "%-18s %-8d %-8d %-8d\n", step.Name, step.Changed, step.Before, step.After)
	}

	b.WriteString("\n")
	b.WriteString(formatQuads("优化后四元式", result.Optimized))
	return b.String()
}

func formatCodegen(result codegen.Result) string {
	var b strings.Builder
	b.WriteString("目标代码指令集合:\n")
	for _, item := range result.InstructionSet {
		b.WriteString("  " + item + "\n")
	}

	b.WriteString("\n活跃信息摘要:\n")
	fmt.Fprintf(&b, "%-8s %-12s %-24s %-24s\n", "基本块", "范围", "入口活跃", "出口活跃")
	for _, block := range result.LiveBlocks {
		fmt.Fprintf(&b, "%-8s %-12s %-24s %-24s\n",
			fmt.Sprintf("B%d", block.BlockIndex),
			fmt.Sprintf("%d-%d", block.Start, block.End),
			formatNameList(block.Entry),
			formatNameList(block.Exit),
		)
	}

	b.WriteString("\n目标代码序列:\n")
	for _, inst := range result.Instructions {
		fmt.Fprintf(&b, "%-5d %-8s %-8s %s\n", inst.Index, inst.Op, inst.Arg1+",", inst.Arg2)
	}
	return b.String()
}

func formatErrors(title string, errors []string) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	writeErrors(&b, errors)
	return b.String()
}

func writeErrors(b *strings.Builder, errors []string) {
	for _, err := range errors {
		b.WriteString(err + "\n")
	}
}

func writeStringList(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("空\n")
		return
	}
	for i, item := range items {
		fmt.Fprintf(b, "%-4d %s\n", i+1, item)
	}
}

func formatNameList(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	if len(names) <= 4 {
		return strings.Join(names, ",")
	}
	return fmt.Sprintf("%d个:%s...", len(names), strings.Join(names[:4], ","))
}

func limitLines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	hidden := len(lines) - maxLines
	lines = lines[:maxLines]
	lines = append(lines, fmt.Sprintf("\n... 为了保持界面流畅，已隐藏 %d 行；完整输出请运行命令行版本。", hidden))
	return strings.Join(lines, "\n")
}
