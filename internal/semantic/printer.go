package semantic

import (
	"fmt"
	"strconv"
	"strings"
)

// PrintResult 输出语义分析阶段的符号表、辅助表、活动记录和四元式
func PrintResult(result Result) {
	fmt.Println("语义分析结果:")
	if len(result.Errors) == 0 {
		fmt.Println("  语义分析成功")
	} else {
		for _, err := range result.Errors {
			fmt.Println(" ", err)
		}
		fmt.Println()
		return
	}
	fmt.Println()

	printSynbl(result.Symbols)
	printTypel(result.Symbols)
	printAinfl(result.Symbols)
	printRinfl(result.Symbols)
	printPfinfl(result.Symbols)
	printParamTable(result.Symbols)
	printConsl(result.Symbols)
	printLenl(result.Symbols)
	printActivityRecord(result.Symbols)
	printQuads(result.Quads)
}

// printSynbl 输出符号表总表，对应 PPT 中的 SYNBL
func printSynbl(symbols []Symbol) {
	fmt.Println("符号表总表 SYNBL:")
	fmt.Printf("  %-4s %-12s %-12s %-8s %-6s\n", "序号", "NAME", "TYPE", "CAT", "ADDR")
	for _, sym := range symbols {
		addr := fmt.Sprintf("%d", sym.Addr)
		if sym.Addr < 0 {
			addr = "-"
		}
		fmt.Printf("  %-4d %-12s %-12s %-8s %-6s\n",
			sym.Index, sym.Name, sym.Type, sym.Category, addr)
	}
	fmt.Println()
}

// printTypel 输出类型表，对应 PPT 中的 TYPEL
func printTypel(symbols []Symbol) {
	fmt.Println("类型表 TYPEL:")
	fmt.Printf("  %-4s %-12s %-8s %-18s\n", "序号", "TVAL", "LEN", "TPOINT")

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
	for i, typ := range typeNames {
		fmt.Printf("  %-4d %-12s %-8d %-18s\n", i+1, typeCode(typ, structs), typeLengthForPrint(typ, structs), typePointer(typ, structs))
	}
	fmt.Println()
}

// printAinfl 输出数组表，对应 PPT 中的 AINFL
func printAinfl(symbols []Symbol) {
	arrays := collectArrayTypes(symbols)
	if len(arrays) == 0 {
		return
	}

	fmt.Println("数组表 AINFL:")
	fmt.Printf("  %-4s %-12s %-8s %-8s %-12s %-8s\n", "序号", "数组类型", "LOW", "UP", "CTP", "CLEN")
	structs := collectStructs(symbols)
	for i, typ := range arrays {
		size, elemType, ok := parseArrayType(typ)
		if !ok {
			continue
		}
		fmt.Printf("  %-4d %-12s %-8d %-8d %-12s %-8d\n",
			i+1, typ, 0, size-1, elemType, typeLengthForPrint(elemType, structs))
	}
	fmt.Println()
}

// printRinfl 输出结构表，对应 PPT 中的 RINFL
func printRinfl(symbols []Symbol) {
	structs := collectStructs(symbols)
	if len(structs) == 0 {
		return
	}

	fmt.Println("结构表 RINFL:")
	fmt.Printf("  %-12s %-12s %-8s %-12s\n", "结构名", "ID", "OFF", "TP")
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
			fmt.Printf("  %-12s %-12s %-8d %-12s\n", sym.Name, name, offset, typ)
			offset += typeLengthForPrint(typ, structs)
		}
	}
	fmt.Println()
}

// printPfinfl 输出函数表，对应 PPT 中的 PFINFL
func printPfinfl(symbols []Symbol) {
	fmt.Println("函数表 PFINFL:")
	fmt.Printf("  %-12s %-8s %-8s %-8s %-12s %-8s %-18s\n", "函数名", "LEVEL", "OFF", "FN", "ENTRY", "RETURN", "PARAM")
	for _, sym := range symbols {
		if sym.Category != "f" {
			continue
		}
		params := sym.Value
		if params == "" {
			params = "-"
		}
		fmt.Printf("  %-12s %-8d %-8d %-8d %-12s %-8s %-18s\n",
			sym.Name, 0, localVarStartAddr, countParams(sym.Value), sym.Name, sym.Type, params)
	}
	fmt.Println()
}

// printParamTable 输出函数形参表，对应 PFINFL 中 PARAM 指向的参数信息
func printParamTable(symbols []Symbol) {
	hasParam := false
	for _, sym := range symbols {
		if sym.Category == "p" {
			hasParam = true
			break
		}
	}
	if !hasParam {
		return
	}

	fmt.Println("参数表 PARAM:")
	fmt.Printf("  %-12s %-8s %-12s %-12s %-8s %-8s\n", "函数名", "序号", "参数名", "TYPE", "CAT", "ADDR")
	index := 1
	for _, sym := range symbols {
		if sym.Category != "p" {
			continue
		}
		funcName, paramName := splitScopedName(sym.Name)
		addr := fmt.Sprintf("%d", sym.Addr)
		if sym.Addr < 0 {
			addr = "-"
		}
		fmt.Printf("  %-12s %-8d %-12s %-12s %-8s %-8s\n",
			funcName, index, paramName, sym.Type, sym.Category, addr)
		index++
	}
	fmt.Println()
}

// printConsl 输出常量表，对应 PPT 中的 CONSL
func printConsl(symbols []Symbol) {
	hasConst := false
	for _, sym := range symbols {
		if sym.Category == "c" {
			hasConst = true
			break
		}
	}
	if !hasConst {
		return
	}

	fmt.Println("常量表 CONSL:")
	fmt.Printf("  %-4s %-12s %-12s %-12s\n", "序号", "NAME", "TYPE", "VALUE")
	for _, sym := range symbols {
		if sym.Category == "c" {
			fmt.Printf("  %-4d %-12s %-12s %-12s\n", sym.Index, sym.Name, sym.Type, sym.Value)
		}
	}
	fmt.Println()
}

// printLenl 输出长度表，对应 PPT 中的 LENL
func printLenl(symbols []Symbol) {
	fmt.Println("长度表 LENL:")
	fmt.Printf("  %-12s %-8s\n", "TYPE", "LEN")
	printed := map[string]bool{}
	structs := collectStructs(symbols)
	for _, typ := range []string{"int", "float", "char", "bool", "string"} {
		fmt.Printf("  %-12s %-8d\n", typ, typeLengthForPrint(typ, structs))
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
		fmt.Printf("  %-12s %-8d\n", typ, typeLengthForPrint(typ, structs))
		printed[typ] = true
	}
	fmt.Println()
}

// printActivityRecord 输出活动记录表，对应 PPT 中的 VALL
func printActivityRecord(symbols []Symbol) {
	fmt.Println("活动记录表 VALL:")
	printVallRow("地址", "内容", "类型", "种类")
	printVallRow("0", "Old SP", "-", "连接数据")
	printVallRow("1", "返回地址", "-", "连接数据")
	printVallRow("2", "全局Display地址", "-", "连接数据")
	printVallRow("3", "参数个数", "-", "参数信息")
	printVallRow("4", "Display[0]", "-", "显示区表")
	for _, sym := range symbols {
		if sym.Addr >= 0 {
			area := "局部变量"
			if sym.Category == "t" {
				area = "临时变量"
			} else if sym.Category == "p" {
				area = "形式参数"
			}
			printVallRow(fmt.Sprintf("%d", sym.Addr), sym.Name, sym.Type, area)
		}
	}
	fmt.Println()
}

// printQuads 输出语义动作生成的四元式序列
func printQuads(quads []Quad) {
	fmt.Println("四元式序列:")
	for _, q := range quads {
		fmt.Printf("  %-4d (%s, %s, %s, %s)\n", q.Index, q.Op, q.Arg1, q.Arg2, q.Result)
	}
	fmt.Println()
}

// collectStructs 收集所有结构体类型，方便后面计算字段长度
func collectStructs(symbols []Symbol) map[string]Symbol {
	structs := map[string]Symbol{}
	for _, sym := range symbols {
		if sym.Category == "type" && sym.Type == "struct" {
			structs[sym.Name] = sym
		}
	}
	return structs
}

// collectArrayTypes 收集程序中出现过的数组类型
func collectArrayTypes(symbols []Symbol) []string {
	arrays := []string{}
	for _, sym := range symbols {
		if strings.HasPrefix(sym.Type, "[") {
			arrays = addUnique(arrays, sym.Type)
		}
	}
	return arrays
}

// addUnique 向字符串切片中加入一个不重复的元素
func addUnique(items []string, item string) []string {
	for _, old := range items {
		if old == item {
			return items
		}
	}
	return append(items, item)
}

// typeCode 把 MiniGo 类型转换成 PPT 中常见的类型码
func typeCode(typ string, structs map[string]Symbol) string {
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

// typePointer 给数组和结构体类型生成对应辅助表位置说明
func typePointer(typ string, structs map[string]Symbol) string {
	if strings.HasPrefix(typ, "[") {
		return "AINFL(" + typ + ")"
	}
	if _, ok := structs[typ]; ok {
		return "RINFL(" + typ + ")"
	}
	return "null"
}

// typeLengthForPrint 计算打印辅助表时需要展示的类型长度
func typeLengthForPrint(typ string, structs map[string]Symbol) int {
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
			return size * typeLengthForPrint(elemType, structs)
		}
		return 0
	}
}

// parseArrayType 解析形如 [10]int 的数组类型
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

// parseField 解析结构体字段保存形式，如 age:int
func parseField(field string) (string, string, bool) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

// splitScopedName 把带函数作用域的名字拆成函数名和局部名
func splitScopedName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "-", name
	}
	return parts[0], parts[1]
}

// countParams 统计函数参数列表中的参数个数
func countParams(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}
	return len(strings.Split(params, ","))
}

// printVallRow 按显示宽度输出一行，避免中文占两个字符宽度导致列歪
func printVallRow(addr string, content string, typ string, category string) {
	fmt.Println("  " + padDisplay(addr, 8) + padDisplay(content, 24) + padDisplay(typ, 14) + category)
}

// padDisplay 按终端显示宽度补空格，中文通常占两个英文宽度
func padDisplay(text string, width int) string {
	spaceCount := width - displayWidth(text)
	if spaceCount < 1 {
		spaceCount = 1
	}
	return text + strings.Repeat(" ", spaceCount)
}

// displayWidth 简单估算字符串在终端中的显示宽度
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
