# MiniGo 词法分析模块

这是编译原理课程设计代码，目前实现了 **MiniGo 词法分析器**、**递归下降语法分析器**、**语义分析/四元式生成**、**基础四元式优化** 和 **目标代码生成**。

代码按 Go 项目习惯分成 `cmd` 和 `internal`，但文件数量控制得比较少：

- `cmd/minigo/main.go`：程序入口，读源程序文件，调用扫描器
- `internal/lexer/lexer.go`：Token、关键字表、界符表、词法分析主逻辑
- `internal/lexer/printer.go`：打印 Token 序列和各类表
- `internal/parser/parser.go`：递归下降语法分析
- `internal/semantic/semantic.go`：符号表、活动记录地址、四元式生成
- `internal/semantic/printer.go`：打印语义分析结果
- `internal/optimizer/optimizer.go`：基本块划分和基础四元式优化
- `internal/optimizer/printer.go`：打印优化结果
- `internal/codegen/codegen.go`：活跃信息分析和目标代码生成
- `internal/codegen/printer.go`：打印目标代码生成结果
- 不使用 `interface`
- 不使用 AST
- 不分复杂包
- 输出词法分析、语法分析、语义分析和优化阶段结果

## 运行

```powershell
go run .\cmd\minigo .\examples\basic.mg
```

不传文件时，默认读取：

```txt
examples/basic.mg
```

## 当前完成内容

- 关键字表
- 界符表
- 标识符表
- 常数表
- Token 序列
- 简单错误提示
- 支持 `//` 和 `/* */` 注释
- 支持整数常量、实数常量、字符常量、字符串常量
- 递归下降语法分析
- 符号表
- 活动记录地址分配
- 四元式生成
- 函数单返回值和 return 类型检查
- 函数参数和函数调用
- 数组声明，例如 `var scores [10]int;`
- 结构体声明，例如 `type Student struct { age int; }`
- 基本块划分
- 常量表达式节省
- 公共子表达式节省
- 删除无用赋值
- 循环不变式外提
- 目标代码指令集合定义
- 活跃信息生成
- 四元式到目标指令的翻译

后续如果时间够，可以继续补 LL(1) 或 SLR 分析表展示。
