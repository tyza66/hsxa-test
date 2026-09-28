---
name: fofa-query-builder
description: 把自然语言资产搜索需求转换成合法、完整、可执行的 FOFA 查询语句（FOFA Query Syntax），并对写好的查询做语法体检。当用户要求自然语言转 FOFA、FOFA 查询语句转换、字段或运算符怎么选、修正一条 FOFA 查询、把 Shodan / Google Dork / Censys / Hunter / urlscan 的搜索意图改写成 FOFA、或生成并校验答案 JSON 时使用。
---

# FOFA 查询语句构造

把一句自然语言需求变成一个正确的 FOFA Query。核心不是"把词换成等号"，而是**判断哪些需求 FOFA 表达不了**——判错方向比答不出更糟，所以本 Skill 宁可给出固定答案，也不硬凑一条漏条件的查询。

## 任务边界

本 Skill 只产出**查询语句本身**，不做联网检索、不做结果筛选、不解释推理过程。它替代的是人手工写 Query 的环节，不是替代人去看搜索结果。

## 三步流程

### 第一步：机械层 `scripts/convert.py`

```bash
python3 skill/scripts/convert.py "请查询 IP 地址为 20.247.40.92 的资产。"
python3 skill/scripts/convert.py --题目 docs/题目.txt --输出 skill/tests/convert.json
```

只接管"照着字面就能映射"的需求：单个 IP、单个网段、单个端口、单个协议、单个国家。它刻意保守，出现任何没建模的条件（证书、标题、蜜罐、多选端口……）就返回空并说明原因，**把题目交回给模型**。

机械层还负责把四类一眼无解的需求钉死成固定答案：非法 IPv4、非法 CIDR、端口越界、同一字段自相矛盾。

### 第二步：模型判据

机械层弃权的题目，按下面的判据逐一分析，参考 `reference/` 下的四份文档：

| 要判断什么 | 看哪份 |
| --- | --- |
| 该用哪个字段、支持哪些运算符 | `reference/fofa_fields.md` |
| `=` `==` `!=` `*=` `&&` `||` `()` 的准确语义 | `reference/syntax.md` |
| 自然语言里的哪个说法对应哪个字段 | `reference/mapping.md` |
| 什么情况下必须给固定答案 | `reference/cannot_convert.md` |

判据的优先级是：**先判能不能转，再判用哪个字段，最后才组装**。顺序反了就会为了凑一条查询而忽略根本表达不了的条件。

### 第三步：组装与校验 `scripts/build_answers.py` / `scripts/validate.py`

```bash
python3 skill/scripts/build_answers.py
python3 skill/scripts/validate.py skill/answers/答案.json
```

`build_answers.py` 产出答卷、golden 快照和待确认清单；`validate.py` 在上传前做最后一道格式校验，**不通过就不要提交**。

## 答案硬约束

违反任一条，整份答案会被拒绝。详见 `reference/answer_format.md`。

- 顶层恰三个键：`选手名称`、`参赛包编号`、`答案`。
- 题号形如 `M001-S009`，必须与题目**逐一对应**：缺题、重复、未知题号都算失败。
- 每题只允许 `题号` 和 `查询语句` 两个字段。
- 可转换时填**一条**最终 Query，不带解释、不带 Markdown 代码块。
- 不可转换时填固定串：`该需求不能直接转换为FOFA搜索语句`。
- UTF-8（允许 BOM），不超过 8 MB，JSON 而非 JSONL。

## 脚本清单

| 脚本 | 作用 |
| --- | --- |
| `scripts/lookup.py` | 纯标准库工具库：转义、条件拼装、字段白名单、语法体检。`selftest` 44 例 |
| `scripts/paths.py` | 路径解析：定位工作区、题目、规则源与输出目录，支持仓库/软链/拷贝三种形态 |
| `scripts/convert.py` | 机械层：字面可映射的直译 + 无解判定。`--selftest` 21 例 |
| `scripts/build_answers.py` | 模型判据的落盘实现，生成答卷与待确认清单 |
| `scripts/validate.py` | 答卷格式与题号完整性校验 |
| `scripts/gen_field_doc.py` | 从 `rules/3.txt` 生成字段表并与 `FIELD_OPERATORS` 对账 |
| `tests/regress.py` | 全量回归：两组 selftest + 三份交付物逐字节可复现 + 字段表同步 + 答卷格式校验 |

## 本地安装与调用

本 Skill 已用软链装到 `~/.codex/skills/fofa-query-builder`，指向仓库里的 `skill/`，
两边是同一份文件，改完立即生效，不会出现两份判据各写各的：

```bash
ln -sfn "$(pwd)/skill" ~/.codex/skills/fofa-query-builder
```

新装的 Skill 要到**下一次会话**才会被加载；当前会话里按上面的命令直接跑脚本即可。

三种形态下调用都不用在意当前目录：

- 仓库形态，脚本在 `<repo>/skill/scripts/`，直接按仓库根解析；
- 软链形态，经 `realpath` 还原出真实仓库，行为与仓库形态一致；
- 拷贝形态，整份拷走后从当前目录向上找带 `docs/题目.txt` 的工作区。

路径解析优先级：环境变量 `FOFA_WORKSPACE` > 脚本所属仓库 > 当前目录向上查找。
题目固定取 `<工作区>/docs/题目.txt`；答卷默认写 `<仓库>/skill/answers/答案.json`，
拷贝形态写 `<工作区>/answers/答案.json`。换参赛包、换题目文件时传 `--题目` 覆盖，
不必改脚本。
方案要求单独出一份答卷时，用 `--输出 <路径>` 指定落点：

```bash
python3 skill/scripts/build_answers.py --输出 test/答案.json
```

## 红线

- `rules/` 是持续维护的权威规则源，**只读**。字段表以 `rules/3.txt` 为准。
- `docs/` 是题目与提交规范，**只读**。
- `test/` 目录归用户管理，除用户明确指定输出位置外，不要往里写。
- 不要为了让题目"有答案"而编造字段或放宽条件。

## 新建或更新 Skill 后必做

```bash
python3 skill/tests/regress.py
```

全绿才算完成。任何一条红了都说明判据或实现漂移了，先修再交付。
