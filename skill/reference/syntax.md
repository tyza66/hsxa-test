# FOFA 运算语义速查

语义一律以 `rules/3.txt` 顶部的 Logic Operator 表为准。这里补的是那张表没讲清、
但实战里最容易踩的部分。

## 四个比较运算符

| 写法 | 语义 | 空值行为 |
| --- | --- | --- |
| `字段="值"` | 相等匹配，实为**包含**匹配 | `字段=""` 查"该字段不存在或值为空" |
| `字段=="值"` | 完全匹配，整串相等 | `字段==""` 查"该字段存在" |
| `字段!="值"` | 不匹配 | `字段!=""` 查"该字段值不为空" |
| `字段*="模式"` | 模糊匹配 | `*` 匹配任意串，`?` 匹配单字符 |

两个关键推论：

1. **`=` 本身就是包含匹配**，所以想"包含河北"直接 `region="Hebei"`，不要写
   `region*="*Hebei*"`——`region` 压根不支持 `*=`，写了就是非法查询。
2. **`!=` 会连带把空值排掉**。`server!="nginx"` 的真实含义是"有 server 字段、
   且不是 nginx"，所以"不要 Nginx"这类需求用 `!=` 通常正是想要的，但如果需求
   只要求"排除某一类"而不在乎空值，就要意识到结果集比想象的窄。

`==` 不在 3.txt 的三列表里，FOFA 视作 `=` 的精确版。`lookup.lint` 把它随 `=`
一起放行。

## 连接与优先级

| 写法 | 作用 |
| --- | --- |
| `&&` | 与 |
| `\|\|` | 或 |
| `()` | 分组，**优先级最高** |

3.txt 顶部明写："If the query syntax has multiple AND & OR relationships, try
including it with ()"。**只要一条查询里同时出现 `&&` 和 `||`，就必须把 `||`
那几项用括号括起来**，否则连接关系取决于引擎的默认优先级而非你的意图。

示例，"找 80 或 443 端口、且是 Apache 的资产"：

```text
((port="80" || port="443") && server=="Apache")
```

## 本 Skill 不使用的语法

`rules/1.txt` 提到了下面这些，但权威的 `rules/3.txt` 里完全没有，因此**不要**
写进答案：

- `cidr=` / `service=` / `keyword=` / `cms=` / `vuln=` / `hardware=`
  —— 这些字段在 3.txt 的字段表里不存在，用了就是非法查询。
- `regex(body, "...")` —— 3.txt 未涉及。**FOFA 没有正则字段**。
- `order by` / `limit` —— 3.txt 未涉及，答案要求单条 Query。
- 字段间的比较（如 `port > 80`）—— 没有区间与大小比较语法。

## 独有约束：Unique IP View

`rules/3.txt` 末节那族字段（`port_size`、`port_size_gt`、`port_size_lt`、
`ip_ports`、`ip_country`、`ip_region`、`ip_city`、`ip_after`、`ip_before`）
表头原文即声明：

> These unique IP series syntax cannot be used together with the other syntax
> mentioned above.

也就是说它们**必须单独成句**。`ip_country="US" && port="443"` 是错的。
`lookup.lint` 已实现这条检查。

## 字符串转义

值里的 `\` 和 `"` 必须分别转成 `\\` 和 `\"`。`lookup.escape()` 统一处理，
拼查询时**永远不要手写字符串字面量**。
