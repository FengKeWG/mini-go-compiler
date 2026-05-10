# MiniGo 词法分析模块

这是编译原理课程设计的前三阶段代码，目前实现了 **MiniGo 词法分析器**、**递归下降语法分析器** 和 **语义分析/四元式生成**。

代码按 Go 项目习惯分成 `cmd` 和 `internal`，但文件数量控制得比较少：

- `cmd/minigo/main.go`：程序入口，读源程序文件，调用扫描器
- `internal/lexer/lexer.go`：Token、关键字表、界符表、词法分析主逻辑
- `internal/lexer/printer.go`：打印 Token 序列和各类表
- `internal/parser/parser.go`：递归下降语法分析
- `internal/semantic/semantic.go`：符号表、活动记录地址、四元式生成
- `internal/semantic/printer.go`：打印语义分析结果
- 不使用 `interface`
- 不使用 AST
- 不使用递归下降
- 不分复杂包
- 只输出词法分析阶段需要的表和 Token 序列

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
- 数组声明，例如 `var scores [10]int;`
- 结构体声明，例如 `type Student struct { age int; }`

后面的优化、目标代码生成先不写，等语义分析稳定后再加。
