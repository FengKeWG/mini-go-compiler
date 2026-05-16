package main

import (
	"fmt"
	"os"
	"strconv"
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
	"minigo/internal/vm"
)

type tableView struct {
	headers []string      // 表头文字
	rows    [][]string    // 表格数据，每个内层切片是一行
	table   *widget.Table // Fyne 表格控件
}

type pageSet struct {
	lexerSummary   *widget.Label // 词法分析数量摘要
	keywordTable   *tableView    // 关键字表 K
	delimiterTable *tableView    // 界符和运算符表 P
	tokenSequence  *widget.Entry // Token 序列文本，支持复制
	tokenTable     *tableView    // Token 详细表
	identTable     *tableView    // 标识符表 I
	constTable     *tableView    // 常数表 C

	parserText *widget.Entry // 语法分析文本输出

	symbolHint  *widget.Label // 符号种类说明
	symbolTable *tableView    // 符号表总表 SYNBL
	typelTable  *tableView    // 类型表 TYPEL
	ainflTable  *tableView    // 数组表 AINFL
	rinflTable  *tableView    // 结构表 RINFL
	pfinflTable *tableView    // 函数表 PFINFL
	paramTable  *tableView    // 参数表 PARAM
	conslTable  *tableView    // 常量表 CONSL
	lenlTable   *tableView    // 长度表 LENL
	vallTable   *tableView    // 活动记录表 VALL

	quadTable *tableView // 语义分析生成的四元式表

	blockTable     *tableView // 基本块划分表
	stepTable      *tableView // 优化步骤统计表
	optimizedTable *tableView // 优化后的四元式表

	instructionSetTable *tableView // 目标代码指令集合
	liveTable           *tableView // 活跃信息表
	targetTable         *tableView // 目标代码表
	runtimeTable        *tableView // 目标指令运行后的变量表
	traceTable          *tableView // 目标指令运行轨迹表

	errorText *widget.Entry // 错误信息文本输出
}

type analyzeView struct {
	lexResult      lexer.Result     // 词法分析结构化结果
	parseResult    parser.Result    // 语法分析结构化结果
	semanticResult semantic.Result  // 语义分析结构化结果
	optimizeResult optimizer.Result // 优化结构化结果
	codegenResult  codegen.Result   // 目标代码结构化结果
	runtimeResult  vm.Result        // 目标指令运行平台结构化结果

	lexerText    string // 词法分析完整文本，复制按钮使用
	parserText   string // 语法分析完整文本，复制按钮使用
	symbolText   string // 符号表完整文本，复制按钮使用
	quadText     string // 四元式完整文本，复制按钮使用
	optimizeText string // 优化结果完整文本，复制按钮使用
	codegenText  string // 目标代码完整文本，复制按钮使用
	runtimeText  string // 目标指令运行平台完整文本，复制按钮使用
	errorText    string // 错误信息完整文本，复制按钮使用
	statusText   string // 底部状态栏文字
}

const guiLocalVarStartAddr = 5

func main() {
	// 创建 Fyne 应用和主窗口
	a := app.New()
	w := a.NewWindow("MiniGo 编译器可视化")
	w.Resize(fyne.NewSize(1260, 780))

	// 左侧源程序输入框，使用等宽字体方便看代码
	sourceInput := widget.NewMultiLineEntry()
	sourceInput.TextStyle = fyne.TextStyle{Monospace: true}
	sourceInput.Wrapping = fyne.TextWrapOff
	sourceInput.Scroll = fyne.ScrollBoth
	sourceInput.SetText(loadExample())

	pages := newPageSet()
	statusLabel := widget.NewLabel("已载入示例程序")
	current := emptyAnalyzeView()

	var analyzeButton *widget.Button
	analyzeButton = widget.NewButton("开始分析", func() {
		// 点击分析时读取当前输入框内容
		source := sourceInput.Text
		analyzeButton.Disable()
		statusLabel.SetText("正在分析...")

		// 编译过程放到后台执行，避免点击按钮后界面卡住
		go func() {
			result := analyzeSource(source)
			fyne.Do(func() {
				// Fyne 控件必须回到 UI 线程刷新
				current = result
				applyAnalyzeView(pages, result)
				statusLabel.SetText(result.statusText)
				analyzeButton.Enable()
			})
		}()
	})

	loadButton := widget.NewButton("重新载入示例", func() {
		// 重新读取示例源程序，方便修改示例后快速刷新
		sourceInput.SetText(loadExample())
		statusLabel.SetText("已重新载入 examples/basic.mg")
	})

	leftTitle := widget.NewLabel("源程序")
	leftTitle.TextStyle = fyne.TextStyle{Bold: true}
	leftPanel := container.NewBorder(
		container.NewVBox(leftTitle, container.NewHBox(analyzeButton, loadButton)),
		nil,
		nil,
		nil,
		sourceInput,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("词法分析", buildLexerPage(w, statusLabel, pages, &current)),
		container.NewTabItem("语法分析", buildTextPage(w, statusLabel, pages.parserText, "复制语法分析", func() string {
			return current.parserText
		})),
		container.NewTabItem("符号表", buildSymbolPage(w, statusLabel, pages, &current)),
		container.NewTabItem("四元式", buildTablePage(w, statusLabel, pages.quadTable, "复制四元式", func() string {
			return current.quadText
		})),
		container.NewTabItem("优化", buildOptimizerPage(w, statusLabel, pages, &current)),
		container.NewTabItem("目标代码", buildCodegenPage(w, statusLabel, pages, &current)),
		container.NewTabItem("错误信息", buildTextPage(w, statusLabel, pages.errorText, "复制错误信息", func() string {
			return current.errorText
		})),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// 左边放源程序，右边放各阶段结果
	split := container.NewHSplit(leftPanel, tabs)
	split.SetOffset(0.42)

	w.SetContent(container.NewBorder(nil, statusLabel, nil, nil, split))
	w.ShowAndRun()
}

// newPageSet 创建所有页面控件，后续分析只刷新数据
func newPageSet() *pageSet {
	return &pageSet{
		lexerSummary:   widget.NewLabel("等待分析"),
		keywordTable:   newTableView([]string{"编号", "关键字"}, []float32{80, 180}),
		delimiterTable: newTableView([]string{"编号", "界符/运算符"}, []float32{80, 180}),
		tokenSequence:  newOutputEntry("等待分析"),
		tokenTable:     newTableView([]string{"序号", "类别", "编号", "单词", "行", "列"}, []float32{64, 72, 72, 180, 72, 72}),
		identTable:     newTableView([]string{"编号", "标识符"}, []float32{80, 260}),
		constTable:     newTableView([]string{"编号", "常数"}, []float32{80, 260}),

		parserText: newOutputEntry("等待分析"),

		symbolHint:  widget.NewLabel("CAT说明：f=函数，p=形式参数，v=变量，c=常量，type=类型，t=临时变量。t1/t2/t3 是四元式生成时产生的中间结果。"),
		symbolTable: newTableView([]string{"序号", "NAME", "TYPE", "CAT", "ADDR", "LEN", "VALUE"}, []float32{64, 180, 150, 82, 82, 82, 220}),
		typelTable:  newTableView([]string{"序号", "TYPE", "TVAL", "LEN", "TPOINT"}, []float32{70, 160, 100, 90, 220}),
		ainflTable:  newTableView([]string{"序号", "数组类型", "LOW", "UP", "CTP", "CLEN"}, []float32{70, 180, 80, 80, 160, 90}),
		rinflTable:  newTableView([]string{"结构名", "ID", "OFF", "TP"}, []float32{150, 150, 90, 150}),
		pfinflTable: newTableView([]string{"函数名", "LEVEL", "OFF", "FN", "ENTRY", "RETURN", "PARAM"}, []float32{130, 90, 90, 90, 130, 110, 220}),
		paramTable:  newTableView([]string{"函数名", "序号", "参数名", "类型", "CAT", "ADDR"}, []float32{130, 70, 130, 130, 90, 90}),
		conslTable:  newTableView([]string{"序号", "NAME", "TYPE", "VALUE"}, []float32{70, 160, 130, 180}),
		lenlTable:   newTableView([]string{"TYPE", "LEN"}, []float32{180, 90}),
		vallTable:   newTableView([]string{"地址", "内容", "类型", "种类"}, []float32{90, 220, 150, 130}),

		quadTable: newTableView([]string{"序号", "OP", "ARG1", "ARG2", "RESULT"}, []float32{70, 100, 180, 180, 180}),

		blockTable:     newTableView([]string{"基本块", "开始四元式", "结束四元式"}, []float32{100, 130, 130}),
		stepTable:      newTableView([]string{"步骤", "修改条数", "优化前", "优化后"}, []float32{190, 110, 100, 100}),
		optimizedTable: newTableView([]string{"序号", "OP", "ARG1", "ARG2", "RESULT"}, []float32{70, 100, 180, 180, 180}),

		instructionSetTable: newTableView([]string{"序号", "指令说明"}, []float32{70, 620}),
		liveTable:           newTableView([]string{"基本块", "四元式范围", "位置", "变量"}, []float32{90, 120, 90, 180}),
		targetTable:         newTableView([]string{"序号", "OP", "ARG1", "ARG2"}, []float32{70, 120, 180, 180}),
		runtimeTable:        newTableView([]string{"名称", "值"}, []float32{180, 260}),
		traceTable:          newTableView([]string{"步数", "PC", "指令", "R0"}, []float32{80, 80, 260, 160}),

		errorText: newOutputEntry("暂无错误"),
	}
}

// newTableView 创建一个带固定列宽的表格
func newTableView(headers []string, widths []float32) *tableView {
	view := &tableView{headers: headers}
	view.table = widget.NewTable(
		func() (int, int) {
			// 多加一行用于显示表头
			return len(view.rows) + 1, len(view.headers)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			if id.Row == 0 {
				// 第 0 行是表头
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText(view.headers[id.Col])
				return
			}

			label.TextStyle = fyne.TextStyle{Monospace: true}
			// 数据行比表格行号少 1，因为第 0 行被表头占用
			row := id.Row - 1
			text := ""
			if row < len(view.rows) && id.Col < len(view.rows[row]) {
				text = view.rows[row][id.Col]
			}
			label.SetText(text)
		},
	)
	for i, width := range widths {
		view.table.SetColumnWidth(i, width)
	}
	return view
}

// setRows 替换表格数据并刷新控件
func (view *tableView) setRows(rows [][]string) {
	view.rows = rows
	view.table.Refresh()
}

// buildLexerPage 构造词法分析页
func buildLexerPage(w fyne.Window, status *widget.Label, pages *pageSet, current *analyzeView) fyne.CanvasObject {
	tables := container.NewAppTabs(
		container.NewTabItem("关键字表K", pages.keywordTable.table),
		container.NewTabItem("界符表P", pages.delimiterTable.table),
		container.NewTabItem("Token序列", pages.tokenSequence),
		container.NewTabItem("Token表", pages.tokenTable.table),
		container.NewTabItem("标识符表", pages.identTable.table),
		container.NewTabItem("常数表", pages.constTable.table),
	)
	tables.SetTabLocation(container.TabLocationTop)

	top := container.NewVBox(
		pages.lexerSummary,
		copyButton(w, status, "复制词法分析", func() string { return current.lexerText }),
	)
	return container.NewBorder(top, nil, nil, nil, tables)
}

// buildSymbolPage 构造符号表页，按 PPT 中的各类表拆成多个页签
func buildSymbolPage(w fyne.Window, status *widget.Label, pages *pageSet, current *analyzeView) fyne.CanvasObject {
	tables := container.NewAppTabs(
		container.NewTabItem("SYNBL", pages.symbolTable.table),
		container.NewTabItem("TYPEL", pages.typelTable.table),
		container.NewTabItem("AINFL", pages.ainflTable.table),
		container.NewTabItem("RINFL", pages.rinflTable.table),
		container.NewTabItem("PFINFL", pages.pfinflTable.table),
		container.NewTabItem("PARAM", pages.paramTable.table),
		container.NewTabItem("CONSL", pages.conslTable.table),
		container.NewTabItem("LENL", pages.lenlTable.table),
		container.NewTabItem("VALL", pages.vallTable.table),
	)
	tables.SetTabLocation(container.TabLocationTop)
	top := container.NewVBox(
		pages.symbolHint,
		copyButton(w, status, "复制符号表", func() string { return current.symbolText }),
	)
	return container.NewBorder(
		top,
		nil,
		nil,
		nil,
		tables,
	)
}

// buildOptimizerPage 构造优化页
func buildOptimizerPage(w fyne.Window, status *widget.Label, pages *pageSet, current *analyzeView) fyne.CanvasObject {
	tables := container.NewAppTabs(
		container.NewTabItem("基本块", pages.blockTable.table),
		container.NewTabItem("优化步骤", pages.stepTable.table),
		container.NewTabItem("优化后四元式", pages.optimizedTable.table),
	)
	tables.SetTabLocation(container.TabLocationTop)
	return container.NewBorder(
		copyButton(w, status, "复制优化结果", func() string { return current.optimizeText }),
		nil,
		nil,
		nil,
		tables,
	)
}

// buildCodegenPage 构造目标代码页
func buildCodegenPage(w fyne.Window, status *widget.Label, pages *pageSet, current *analyzeView) fyne.CanvasObject {
	tables := container.NewAppTabs(
		container.NewTabItem("指令集合", pages.instructionSetTable.table),
		container.NewTabItem("活跃信息", pages.liveTable.table),
		container.NewTabItem("目标代码", pages.targetTable.table),
		container.NewTabItem("运行结果", pages.runtimeTable.table),
		container.NewTabItem("运行轨迹", pages.traceTable.table),
	)
	tables.SetTabLocation(container.TabLocationTop)
	return container.NewBorder(
		copyButton(w, status, "复制目标代码", func() string { return current.codegenText }),
		nil,
		nil,
		nil,
		tables,
	)
}

// buildTablePage 构造只有一个表格的通用页面
func buildTablePage(w fyne.Window, status *widget.Label, table *tableView, label string, getText func() string) fyne.CanvasObject {
	return container.NewBorder(copyButton(w, status, label, getText), nil, nil, nil, table.table)
}

// buildTextPage 构造只有一个文本框的通用页面
func buildTextPage(w fyne.Window, status *widget.Label, entry *widget.Entry, label string, getText func() string) fyne.CanvasObject {
	return container.NewBorder(copyButton(w, status, label, getText), nil, nil, nil, entry)
}

// copyButton 创建复制按钮，复制完整文本而不是表格可见区域
func copyButton(w fyne.Window, status *widget.Label, text string, getText func() string) *widget.Button {
	return widget.NewButton(text, func() {
		w.Clipboard().SetContent(getText())
		status.SetText(text + "：已复制")
	})
}

// applyAnalyzeView 把一次完整分析结果刷新到所有页面
func applyAnalyzeView(pages *pageSet, view analyzeView) {
	// 词法分析页刷新
	pages.lexerSummary.SetText(formatLexerSummary(view.lexResult))
	pages.keywordTable.setRows(stringRows(lexer.KeywordTable()))
	pages.delimiterTable.setRows(stringRows(lexer.DelimiterTable()))
	pages.tokenSequence.SetText(formatTokenSequence(view.lexResult.Tokens))
	pages.tokenTable.setRows(tokenRows(view.lexResult.Tokens))
	pages.identTable.setRows(stringRows(view.lexResult.Identifiers))
	pages.constTable.setRows(stringRows(view.lexResult.Constants))

	pages.parserText.SetText(view.parserText)

	// 符号表页刷新
	pages.symbolTable.setRows(symbolRows(view.semanticResult.Symbols))
	pages.typelTable.setRows(typelRows(view.semanticResult.Symbols))
	pages.ainflTable.setRows(ainflRows(view.semanticResult.Symbols))
	pages.rinflTable.setRows(rinflRows(view.semanticResult.Symbols))
	pages.pfinflTable.setRows(pfinflRows(view.semanticResult.Symbols))
	pages.paramTable.setRows(paramRows(view.semanticResult.Symbols))
	pages.conslTable.setRows(conslRows(view.semanticResult.Symbols))
	pages.lenlTable.setRows(lenlRows(view.semanticResult.Symbols))
	pages.vallTable.setRows(vallRows(view.semanticResult.Symbols))

	pages.quadTable.setRows(quadRows(view.semanticResult.Quads))

	// 优化页刷新
	pages.blockTable.setRows(blockRows(view.optimizeResult.Blocks))
	pages.stepTable.setRows(stepRows(view.optimizeResult.Steps))
	pages.optimizedTable.setRows(quadRows(view.optimizeResult.Optimized))

	// 目标代码页刷新
	pages.instructionSetTable.setRows(instructionSetRows(view.codegenResult.InstructionSet))
	pages.liveTable.setRows(liveRows(view.codegenResult.LiveBlocks))
	pages.targetTable.setRows(instructionRows(view.codegenResult.Instructions))
	pages.runtimeTable.setRows(runtimeRows(view.runtimeResult))
	pages.traceTable.setRows(traceRows(view.runtimeResult.Trace))

	pages.errorText.SetText(view.errorText)
}

// newOutputEntry 创建可横向滚动且可复制的等宽文本框
func newOutputEntry(text string) *widget.Entry {
	entry := widget.NewMultiLineEntry()
	entry.TextStyle = fyne.TextStyle{Monospace: true}
	entry.Wrapping = fyne.TextWrapOff
	entry.Scroll = fyne.ScrollBoth
	entry.SetText(text)
	return entry
}

// emptyAnalyzeView 返回还没有分析时的默认显示内容
func emptyAnalyzeView() analyzeView {
	return analyzeView{
		lexerText:    "等待分析",
		parserText:   "等待分析",
		symbolText:   "等待分析",
		quadText:     "等待分析",
		optimizeText: "等待分析",
		codegenText:  "等待分析",
		runtimeText:  "等待分析",
		errorText:    "暂无错误",
		statusText:   "等待分析",
	}
}

// loadExample 读取示例源程序，读取失败时使用一个最小可运行示例
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

// analyzeSource 串起词法、语法、语义、优化、目标代码和运行平台六个阶段
func analyzeSource(source string) analyzeView {
	view := emptyAnalyzeView()
	// 默认认为后续阶段未执行，只有上一阶段成功后才覆盖
	view.parserText = "未执行：上一阶段存在错误"
	view.symbolText = "未执行：上一阶段存在错误"
	view.quadText = "未执行：上一阶段存在错误"
	view.optimizeText = "未执行：上一阶段存在错误"
	view.codegenText = "未执行：上一阶段存在错误"
	view.runtimeText = "未执行：上一阶段存在错误"

	lexResult := lexer.Scan(source)
	view.lexResult = lexResult
	view.lexerText = formatLexer(lexResult)
	if len(lexResult.Errors) > 0 {
		// 词法有错时不继续语法分析
		view.errorText = formatErrors("词法错误", lexResult.Errors)
		view.statusText = fmt.Sprintf("词法分析发现 %d 个错误，后续阶段未执行", len(lexResult.Errors))
		return view
	}

	parseResult := parser.Parse(lexResult.Tokens)
	view.parseResult = parseResult
	view.parserText = formatParser(parseResult)
	if len(parseResult.Errors) > 0 {
		// 语法有错时不继续语义分析
		view.errorText = formatErrors("语法错误", parseResult.Errors)
		view.statusText = fmt.Sprintf("语法分析发现 %d 个错误，后续阶段未执行", len(parseResult.Errors))
		return view
	}

	semanticResult := semantic.Analyze(lexResult.Tokens)
	view.semanticResult = semanticResult
	view.symbolText = formatSymbols(semanticResult.Symbols)
	view.quadText = formatQuads("语义分析生成的四元式", semanticResult.Quads)
	if len(semanticResult.Errors) > 0 {
		// 语义有错时不继续优化和目标代码生成
		view.errorText = formatErrors("语义错误", semanticResult.Errors)
		view.statusText = fmt.Sprintf("语义分析发现 %d 个错误，后续阶段未执行", len(semanticResult.Errors))
		return view
	}

	optimizeResult := optimizer.Optimize(semanticResult.Quads)
	view.optimizeResult = optimizeResult
	view.optimizeText = formatOptimizer(optimizeResult)

	codegenResult := codegen.Generate(optimizeResult.Optimized)
	view.codegenResult = codegenResult
	runtimeResult := vm.Run(codegenResult.Instructions, semanticResult.Symbols)
	view.runtimeResult = runtimeResult
	view.runtimeText = formatRuntime(runtimeResult)
	view.codegenText = formatCodegen(codegenResult) + "\n\n" + view.runtimeText

	if len(runtimeResult.Errors) > 0 {
		view.errorText = formatErrors("目标指令运行错误", runtimeResult.Errors)
		view.statusText = fmt.Sprintf("目标指令运行发现 %d 个错误", len(runtimeResult.Errors))
		return view
	}

	view.errorText = "暂无错误"
	view.statusText = "分析完成：词法、语法、语义、优化、目标代码和运行结果均已生成"
	return view
}

// tokenRows 把 Token 序列转换成 GUI 表格行
func tokenRows(tokens []lexer.Token) [][]string {
	rows := make([][]string, 0, len(tokens))
	for i, token := range tokens {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			token.Kind,
			fmt.Sprintf("%d", token.Value),
			token.Text,
			fmt.Sprintf("%d", token.Line),
			fmt.Sprintf("%d", token.Column),
		})
	}
	return rows
}

// stringRows 把普通字符串切片转换成带编号的两列表格
func stringRows(items []string) [][]string {
	rows := make([][]string, 0, len(items))
	for i, item := range items {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), item})
	}
	return rows
}

// symbolRows 把 SYNBL 符号表转换成 GUI 表格行
func symbolRows(symbols []semantic.Symbol) [][]string {
	rows := make([][]string, 0, len(symbols))
	for _, sym := range symbols {
		addr := fmt.Sprintf("%d", sym.Addr)
		if sym.Addr < 0 {
			addr = "-"
		}
		value := sym.Value
		if value == "" {
			value = "-"
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", sym.Index),
			sym.Name,
			sym.Type,
			sym.Category,
			addr,
			fmt.Sprintf("%d", sym.Length),
			value,
		})
	}
	return rows
}

// typelRows 根据符号表推导类型表 TYPEL
func typelRows(symbols []semantic.Symbol) [][]string {
	typeNames := []string{"int", "float", "char", "bool", "string"}
	for _, sym := range symbols {
		if sym.Category == "type" {
			typeNames = addUnique(typeNames, sym.Name)
			continue
		}
		if sym.Type != "" && sym.Type != "void" && sym.Type != "struct" {
			typeNames = addUnique(typeNames, sym.Type)
		}
	}

	structs := collectStructs(symbols)
	rows := make([][]string, 0, len(typeNames))
	for i, typ := range typeNames {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			typ,
			typeCode(typ, structs),
			fmt.Sprintf("%d", typeLength(typ, structs)),
			typePointer(typ, structs),
		})
	}
	return rows
}

// ainflRows 根据数组类型生成数组表 AINFL
func ainflRows(symbols []semantic.Symbol) [][]string {
	arrays := collectArrayTypes(symbols)
	structs := collectStructs(symbols)
	rows := make([][]string, 0, len(arrays))
	for i, typ := range arrays {
		size, elemType, ok := parseArrayType(typ)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			typ,
			"0",
			fmt.Sprintf("%d", size-1),
			elemType,
			fmt.Sprintf("%d", typeLength(elemType, structs)),
		})
	}
	return rows
}

// rinflRows 根据结构体类型生成结构表 RINFL
func rinflRows(symbols []semantic.Symbol) [][]string {
	structs := collectStructs(symbols)
	rows := [][]string{}
	for _, sym := range symbols {
		if sym.Category != "type" || sym.Type != "struct" {
			continue
		}
		offset := 0
		for _, field := range strings.Split(sym.Value, ",") {
			name, typ, ok := parseField(field)
			if !ok {
				continue
			}
			rows = append(rows, []string{
				sym.Name,
				name,
				fmt.Sprintf("%d", offset),
				typ,
			})
			offset += typeLength(typ, structs)
		}
	}
	return rows
}

// pfinflRows 根据函数符号生成函数表 PFINFL
func pfinflRows(symbols []semantic.Symbol) [][]string {
	rows := [][]string{}
	for _, sym := range symbols {
		if sym.Category != "f" {
			continue
		}
		params := sym.Value
		if params == "" {
			params = "-"
		}
		rows = append(rows, []string{
			sym.Name,
			"0",
			fmt.Sprintf("%d", guiLocalVarStartAddr),
			fmt.Sprintf("%d", countParams(sym.Value)),
			sym.Name,
			sym.Type,
			params,
		})
	}
	return rows
}

// paramRows 把形参符号单独整理成参数表 PARAM
func paramRows(symbols []semantic.Symbol) [][]string {
	rows := [][]string{}
	for _, sym := range symbols {
		if sym.Category != "p" {
			continue
		}
		funcName, paramName := splitScopedName(sym.Name)
		addr := fmt.Sprintf("%d", sym.Addr)
		if sym.Addr < 0 {
			addr = "-"
		}
		rows = append(rows, []string{
			funcName,
			fmt.Sprintf("%d", len(rows)+1),
			paramName,
			sym.Type,
			sym.Category,
			addr,
		})
	}
	return rows
}

// conslRows 把常量符号整理成常量表 CONSL
func conslRows(symbols []semantic.Symbol) [][]string {
	rows := [][]string{}
	for _, sym := range symbols {
		if sym.Category != "c" {
			continue
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", sym.Index),
			sym.Name,
			sym.Type,
			sym.Value,
		})
	}
	return rows
}

// lenlRows 根据所有出现过的类型生成长度表 LENL
func lenlRows(symbols []semantic.Symbol) [][]string {
	printed := map[string]bool{}
	structs := collectStructs(symbols)
	rows := [][]string{}

	for _, typ := range []string{"int", "float", "char", "bool", "string"} {
		rows = append(rows, []string{typ, fmt.Sprintf("%d", typeLength(typ, structs))})
		printed[typ] = true
	}
	for _, sym := range symbols {
		typ := sym.Type
		if sym.Category == "type" {
			typ = sym.Name
		}
		if typ == "" || typ == "void" || typ == "struct" || printed[typ] {
			continue
		}
		rows = append(rows, []string{typ, fmt.Sprintf("%d", typeLength(typ, structs))})
		printed[typ] = true
	}
	return rows
}

// splitScopedName 把带作用域的名字拆成函数名和局部名
func splitScopedName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "-", name
	}
	return parts[0], parts[1]
}

// vallRows 根据符号地址生成活动记录表 VALL
func vallRows(symbols []semantic.Symbol) [][]string {
	rows := [][]string{
		{"0", "Old SP", "-", "连接数据"},
		{"1", "返回地址", "-", "连接数据"},
		{"2", "全局Display地址", "-", "连接数据"},
		{"3", "参数个数", "-", "参数信息"},
		{"4", "Display[0]", "-", "显示区表"},
	}
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
		rows = append(rows, []string{
			fmt.Sprintf("%d", sym.Addr),
			sym.Name,
			sym.Type,
			category,
		})
	}
	return rows
}

// collectStructs 收集结构体类型，便于计算字段和长度
func collectStructs(symbols []semantic.Symbol) map[string]semantic.Symbol {
	structs := map[string]semantic.Symbol{}
	for _, sym := range symbols {
		if sym.Category == "type" && sym.Type == "struct" {
			structs[sym.Name] = sym
		}
	}
	return structs
}

// collectArrayTypes 收集所有数组类型，避免 AINFL 重复展示
func collectArrayTypes(symbols []semantic.Symbol) []string {
	arrays := []string{}
	for _, sym := range symbols {
		if strings.HasPrefix(sym.Type, "[") {
			arrays = addUnique(arrays, sym.Type)
		}
	}
	return arrays
}

// addUnique 追加不重复字符串
func addUnique(items []string, item string) []string {
	for _, old := range items {
		if old == item {
			return items
		}
	}
	return append(items, item)
}

// typeCode 把类型名转换成课件里的类型码
func typeCode(typ string, structs map[string]semantic.Symbol) string {
	switch typ {
	case "int":
		return "i"
	case "float":
		return "r"
	case "char":
		return "c"
	case "bool":
		return "b"
	case "string":
		return "s"
	default:
		if strings.HasPrefix(typ, "[") {
			return "a"
		}
		if _, ok := structs[typ]; ok {
			return "d"
		}
		return typ
	}
}

// typePointer 返回复杂类型对应的辅助表说明
func typePointer(typ string, structs map[string]semantic.Symbol) string {
	if strings.HasPrefix(typ, "[") {
		return "AINFL(" + typ + ")"
	}
	if _, ok := structs[typ]; ok {
		return "RINFL(" + typ + ")"
	}
	return "null"
}

// typeLength 计算 GUI 表格中展示的类型长度
func typeLength(typ string, structs map[string]semantic.Symbol) int {
	switch typ {
	case "int":
		return 4
	case "float":
		return 8
	case "bool", "char":
		return 1
	case "string":
		return 16
	default:
		if sym, ok := structs[typ]; ok {
			return sym.Length
		}
		size, elemType, ok := parseArrayType(typ)
		if ok {
			return size * typeLength(elemType, structs)
		}
		return 0
	}
}

// parseArrayType 解析 [10]int 这种数组类型文本
func parseArrayType(typ string) (int, string, bool) {
	if !strings.HasPrefix(typ, "[") {
		return 0, "", false
	}
	right := strings.Index(typ, "]")
	if right <= 1 || right+1 >= len(typ) {
		return 0, "", false
	}
	size, err := strconv.Atoi(typ[1:right])
	if err != nil {
		return 0, "", false
	}
	return size, typ[right+1:], true
}

// parseField 解析结构体字段文本，例如 age:int
func parseField(field string) (string, string, bool) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

// countParams 统计函数参数个数
func countParams(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}
	return len(strings.Split(params, ","))
}

// quadRows 把四元式转换成 GUI 表格行
func quadRows(quads []semantic.Quad) [][]string {
	rows := make([][]string, 0, len(quads))
	for _, q := range quads {
		rows = append(rows, []string{
			fmt.Sprintf("%d", q.Index),
			q.Op,
			q.Arg1,
			q.Arg2,
			q.Result,
		})
	}
	return rows
}

// blockRows 把基本块划分结果转换成 GUI 表格行
func blockRows(blocks []optimizer.Block) [][]string {
	rows := make([][]string, 0, len(blocks))
	for _, block := range blocks {
		rows = append(rows, []string{
			fmt.Sprintf("B%d", block.Index),
			fmt.Sprintf("%d", block.Start),
			fmt.Sprintf("%d", block.End),
		})
	}
	return rows
}

// stepRows 把优化步骤统计转换成 GUI 表格行
func stepRows(steps []optimizer.Step) [][]string {
	rows := make([][]string, 0, len(steps))
	for _, step := range steps {
		rows = append(rows, []string{
			step.Name,
			fmt.Sprintf("%d", step.Changed),
			fmt.Sprintf("%d", step.Before),
			fmt.Sprintf("%d", step.After),
		})
	}
	return rows
}

// instructionSetRows 把目标机指令集合转换成 GUI 表格行
func instructionSetRows(items []string) [][]string {
	rows := make([][]string, 0, len(items))
	for i, item := range items {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), item})
	}
	return rows
}

// liveRows 把每个基本块的活跃信息展开成多行
func liveRows(blocks []codegen.LiveBlock) [][]string {
	rows := [][]string{}
	for _, block := range blocks {
		blockName := fmt.Sprintf("B%d", block.BlockIndex)
		quadRange := fmt.Sprintf("%d-%d", block.Start, block.End)
		rows = appendLiveRows(rows, blockName, quadRange, "入口", block.Entry)
		rows = appendLiveRows(rows, blockName, quadRange, "出口", block.Exit)
	}
	return rows
}

// appendLiveRows 把入口或出口活跃变量逐个追加到表格中
func appendLiveRows(rows [][]string, blockName string, quadRange string, position string, names []string) [][]string {
	if len(names) == 0 {
		return append(rows, []string{blockName, quadRange, position, "-"})
	}
	for _, name := range names {
		rows = append(rows, []string{blockName, quadRange, position, name})
	}
	return rows
}

// instructionRows 把目标代码指令转换成 GUI 表格行
func instructionRows(instructions []codegen.Instruction) [][]string {
	rows := make([][]string, 0, len(instructions))
	for _, inst := range instructions {
		rows = append(rows, []string{
			fmt.Sprintf("%d", inst.Index),
			inst.Op,
			inst.Arg1,
			inst.Arg2,
		})
	}
	return rows
}

// runtimeRows 把目标指令运行结果转换成 GUI 表格行
func runtimeRows(result vm.Result) [][]string {
	rows := [][]string{}
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			rows = append(rows, []string{"运行错误", err})
		}
		return rows
	}

	rows = append(rows, []string{"返回值", result.ReturnValue})
	for _, variable := range result.Variables {
		rows = append(rows, []string{variable.Name, variable.Value})
	}
	return rows
}

// traceRows 把目标指令执行轨迹转换成 GUI 表格行
func traceRows(trace []vm.Trace) [][]string {
	rows := make([][]string, 0, len(trace))
	for _, item := range trace {
		rows = append(rows, []string{
			fmt.Sprintf("%d", item.Step),
			fmt.Sprintf("%d", item.PC),
			item.Instruction,
			item.R0,
		})
	}
	return rows
}

// formatLexerSummary 生成词法分析顶部摘要
func formatLexerSummary(result lexer.Result) string {
	return fmt.Sprintf(
		"Token总数：%d    标识符：%d    常数：%d    错误：%d",
		len(result.Tokens),
		len(result.Identifiers),
		len(result.Constants),
		len(result.Errors),
	)
}

// formatLexer 生成完整词法分析文本，复制按钮使用
func formatLexer(result lexer.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Token 总数: %d\n", len(result.Tokens))
	fmt.Fprintf(&b, "标识符数量: %d\n", len(result.Identifiers))
	fmt.Fprintf(&b, "常数数量: %d\n", len(result.Constants))
	fmt.Fprintf(&b, "错误数量: %d\n\n", len(result.Errors))

	appendTextTable(&b, "关键字表 K", []string{"编号", "关键字"}, stringRows(lexer.KeywordTable()))
	appendTextTable(&b, "界符表 P", []string{"编号", "界符/运算符"}, stringRows(lexer.DelimiterTable()))

	b.WriteString("Token 序列:\n")
	b.WriteString(formatTokenSequence(result.Tokens))
	b.WriteString("\n\n")

	b.WriteString("Token 详细信息:\n")
	fmt.Fprintf(&b, "%-5s %-6s %-6s %-16s %-8s\n", "序号", "类别", "编号", "单词", "位置")
	for i, token := range result.Tokens {
		pos := fmt.Sprintf("%d:%d", token.Line, token.Column)
		fmt.Fprintf(&b, "%-5d %-6s %-6d %-16s %-8s\n", i+1, token.Kind, token.Value, token.Text, pos)
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

// formatTokenSequence 生成形如 (k,1), (i,2) 的 Token 序列文本
func formatTokenSequence(tokens []lexer.Token) string {
	var b strings.Builder
	for i, token := range tokens {
		if i > 0 && i%12 == 0 {
			b.WriteString("\n")
		} else if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "(%s,%d)", token.Kind, token.Value)
	}
	return b.String()
}

// formatParser 生成语法分析文本
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

// formatSymbols 生成符号表相关全部文本，复制按钮使用
func formatSymbols(symbols []semantic.Symbol) string {
	var b strings.Builder
	appendTextTable(&b, "符号表总表 SYNBL", []string{"序号", "NAME", "TYPE", "CAT", "ADDR", "LEN", "VALUE"}, symbolRows(symbols))
	appendTextTable(&b, "类型表 TYPEL", []string{"序号", "TYPE", "TVAL", "LEN", "TPOINT"}, typelRows(symbols))
	appendTextTable(&b, "数组表 AINFL", []string{"序号", "数组类型", "LOW", "UP", "CTP", "CLEN"}, ainflRows(symbols))
	appendTextTable(&b, "结构表 RINFL", []string{"结构名", "ID", "OFF", "TP"}, rinflRows(symbols))
	appendTextTable(&b, "函数表 PFINFL", []string{"函数名", "LEVEL", "OFF", "FN", "ENTRY", "RETURN", "PARAM"}, pfinflRows(symbols))
	appendTextTable(&b, "参数表 PARAM", []string{"函数名", "序号", "参数名", "类型", "CAT", "ADDR"}, paramRows(symbols))
	appendTextTable(&b, "常量表 CONSL", []string{"序号", "NAME", "TYPE", "VALUE"}, conslRows(symbols))
	appendTextTable(&b, "长度表 LENL", []string{"TYPE", "LEN"}, lenlRows(symbols))
	appendTextTable(&b, "活动记录表 VALL", []string{"地址", "内容", "类型", "种类"}, vallRows(symbols))
	return b.String()
}

// formatQuads 生成四元式文本
func formatQuads(title string, quads []semantic.Quad) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	for _, q := range quads {
		fmt.Fprintf(&b, "%-5d (%s, %s, %s, %s)\n", q.Index, q.Op, q.Arg1, q.Arg2, q.Result)
	}
	return b.String()
}

// formatOptimizer 生成优化结果文本
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

// formatCodegen 生成目标代码阶段文本
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
			wrapText(formatNameList(block.Entry), 22),
			wrapText(formatNameList(block.Exit), 22),
		)
	}

	b.WriteString("\n目标代码序列:\n")
	for _, inst := range result.Instructions {
		fmt.Fprintf(&b, "%-5d %-8s %-8s %s\n", inst.Index, inst.Op, inst.Arg1+",", inst.Arg2)
	}
	return b.String()
}

// formatRuntime 生成目标指令运行平台文本
func formatRuntime(result vm.Result) string {
	var b strings.Builder
	b.WriteString("目标指令运行平台:\n")
	if len(result.Errors) > 0 {
		b.WriteString("运行错误:\n")
		writeErrors(&b, result.Errors)
		return b.String()
	}

	fmt.Fprintf(&b, "返回值: %s\n\n", result.ReturnValue)
	appendTextTable(&b, "最终变量表", []string{"变量", "值"}, runtimeRows(result)[1:])
	appendTextTable(&b, "执行轨迹", []string{"步数", "PC", "指令", "R0"}, traceRows(result.Trace))
	return b.String()
}

// formatErrors 生成错误信息文本
func formatErrors(title string, errors []string) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	writeErrors(&b, errors)
	return b.String()
}

// appendTextTable 把表格数据拼成等宽文本
func appendTextTable(b *strings.Builder, title string, headers []string, rows [][]string) {
	b.WriteString(title + ":\n")
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len([]rune(header))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len([]rune(cell)) > widths[i] {
				widths[i] = len([]rune(cell))
			}
		}
	}

	writeTextRow(b, headers, widths)
	if len(rows) == 0 {
		b.WriteString("空\n\n")
		return
	}
	for _, row := range rows {
		writeTextRow(b, row, widths)
	}
	b.WriteString("\n")
}

// writeTextRow 输出等宽文本表格的一行
func writeTextRow(b *strings.Builder, row []string, widths []int) {
	for i := 0; i < len(widths); i++ {
		cell := ""
		if i < len(row) {
			cell = row[i]
		}
		b.WriteString(cell)
		b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(cell))+3))
	}
	b.WriteString("\n")
}

// writeErrors 把错误列表逐行写入字符串
func writeErrors(b *strings.Builder, errors []string) {
	for _, err := range errors {
		b.WriteString(err + "\n")
	}
}

// writeStringList 把字符串表按编号输出
func writeStringList(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("空\n")
		return
	}
	for i, item := range items {
		fmt.Fprintf(b, "%-4d %s\n", i+1, item)
	}
}

// formatNameList 把变量名列表拼成逗号分隔文本
func formatNameList(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ",")
}

// wrapText 按指定宽度给逗号分隔文本换行
func wrapText(text string, width int) string {
	if len([]rune(text)) <= width {
		return text
	}

	var b strings.Builder
	lineWidth := 0
	for _, part := range strings.Split(text, ",") {
		partWidth := len([]rune(part))
		if lineWidth > 0 && lineWidth+partWidth+1 > width {
			b.WriteString("\n")
			lineWidth = 0
		} else if lineWidth > 0 {
			b.WriteString(",")
			lineWidth++
		}
		b.WriteString(part)
		lineWidth += partWidth
	}
	return b.String()
}
