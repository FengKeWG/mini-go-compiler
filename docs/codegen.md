# MiniGo 目标代码生成设计

目标代码生成阶段以优化后的四元式为输入，生成课件风格的寄存器-内存指令。为了让代码简单易懂，当前只使用一个工作寄存器 `R0`，每条表达式计算完立即存回结果变量。

## 目标代码指令集合

```txt
LD Ri, X      Ri := X
ST Ri, X      X := Ri
ADD Ri, X     Ri := Ri + X
SUB Ri, X     Ri := Ri - X
MUL Ri, X     Ri := Ri * X
DIV Ri, X     Ri := Ri / X
FJ Ri, L      若 Ri 为 false，则跳转到 L
TJ Ri, L      若 Ri 为 true，则跳转到 L
JMP _, L      无条件跳转到 L
LT/GT/EQ/LE/GE/NE Ri, X  关系运算
AND/OR/NO Ri, X          逻辑运算
PARAM _, X               传递一个实参
CALL _, F                调用函数 F，返回值默认放在 R0
```

因为 MiniGo 支持位运算，所以额外增加以下扩展指令：

```txt
MOD   Ri, X    取模
BAND  Ri, X    按位与
BOR   Ri, X    按位或
XOR   Ri, X    按位异或
BCLR  Ri, X    按位清除
SHL   Ri, X    左移
SHR   Ri, X    右移
```

## 四元式翻译规则

赋值：

```txt
(=, a, _, b)

LD R0, a
ST R0, b
```

算术表达式：

```txt
(+, a, b, t1)

LD  R0, a
ADD R0, b
ST  R0, t1
```

关系表达式：

```txt
(<, a, b, t1)

LD R0, a
LT R0, b
ST R0, t1
```

条件跳转：

```txt
(jfalse, t1, _, L1)

LD R0, t1
FJ R0, L1
```

无条件跳转：

```txt
(j, _, _, L1)

JMP _, L1
```

返回语句：

```txt
(return, 0, _, _)

LD  R0, 0
RET R0, _
```

函数调用：

```txt
(param, a, 1, _)
(param, b, 2, _)
(call, add, 2, t1)

PARAM _, a
PARAM _, b
CALL  _, add
ST    R0, t1
```

## 活跃信息

目标代码生成前会在每个基本块内倒序计算活跃变量。为了避免输出太长，程序只打印每个基本块的摘要：

```txt
基本块
四元式范围
入口活跃变量
出口活跃变量
```

当前采用课件中的简化约定：

```txt
临时变量在基本块出口后默认非活跃
非临时变量在基本块出口后默认活跃
```

这份活跃信息用于展示目标代码生成依据；当前没有实现复杂寄存器分配，因此不会因为活跃信息改变生成结果。

## 目标指令运行平台

目标指令运行平台放在 `internal/vm` 中，它不是操作系统虚拟机，而是课程设计用的小解释器。它读取目标代码生成阶段输出的指令，一条一条执行，并输出返回值、最终变量表和执行轨迹。

运行平台的核心数据：

```txt
PC        当前执行到第几条指令
R0        当前唯一工作寄存器
frame     当前函数的活动记录，保存参数、局部变量、临时变量
callStack 函数调用栈，保存返回地址和调用者活动记录
labels    标签表，用于 JMP、FJ、TJ 跳转
procs     函数入口表，用于 CALL 找到函数开始位置
```

执行规则示例：

```txt
LD R0, a      从活动记录读取 a，放入 R0
ADD R0, b     从活动记录读取 b，加到 R0 上
ST R0, t1     把 R0 写回活动记录中的 t1
FJ R0, L1     如果 R0 为 false，就跳转到标签 L1
PARAM _, x    把实参 x 暂存到参数列表
CALL _, add   创建 add 的活动记录，参数写入形参，跳到 add 入口
RET R0, _     函数返回，恢复调用者活动记录
```

输出内容：

```txt
返回值        main 函数最终返回的值
最终变量表    main 函数活动记录中的变量和值
执行轨迹      每一步执行的 PC、指令文本和 R0 值
```

这样可以对应课程设计表格中的“目标指令运行平台”，证明目标指令不仅能生成，也可以被解释执行。
