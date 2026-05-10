func add(x int, y int) int {
    var result int;
    result = x + y;
    return result;
}

func main() int {
    /* 结构体类型声明 */
    type Student struct {
        age int;
        score float;
        name string;
    }

    /* 变量和常量声明 */
    var a int;
    var b int;
    var ok bool;
    var rate float;
    var ch char;
    var msg string;
    var scores [10]int;
    var pair int;
    var stu Student;
    const limit := 10;

    /* 整数、小数、字符、字符串常量 */
    a = 2;
    b = 5;
    a = 2 + 3;
    b = a + 1;
    pair = a + 1;
    pair = add(a, b);
    rate = 3.14;
    ch = 'x';
    msg = "hello";
    ok = true;

    /* 算术运算符：+ - * / % */
    a = a + b;
    b = b - 1;
    a = a * 2;
    b = b / 2;
    a = a % 3;

    /* 位运算符：& | ^ &^ << >> */
    a = a & b;
    a = a | b;
    a = a ^ b;
    a = a &^ b;
    a = a << 1;
    b = b >> 1;

    /* 关系运算符：< > <= >= == != */
    if a < b {
        a = a + 1;
    } else {
        b = b + 1;
    }

    if a <= limit && b >= 0 {
        ok = true;
    }

    if a == b || a != limit {
        ok = !false;
    }

    /* for 条件循环 */
    for a > 0 {
        pair = limit + 1;
        a = a - 1;
        continue;
    }

    for b > 0 {
        b = b - 1;
        break;
    }

    /* 数组下标、结构体字段、冒号、逗号、:= */
    scores[0] = a;
    pair: a, b;
    stu.age = a;
    stu.name = msg;
    temp := rate;

    return 0;
}
