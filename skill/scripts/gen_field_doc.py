#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""从权威规则源 rules/3.txt 生成字段速查表 skill/reference/fofa_fields.md。

为什么不手写这份文档：字段的运算符支持情况极易抄错，而 lookup.py 里的
FIELD_OPERATORS 本来就是从 3.txt 逐列抄来的。这里让文档直接由 3.txt 解析
生成，再和 FIELD_OPERATORS 对账——两边一旦不一致就宁可失败，也不让一份
过期的字段表继续误导人。

用法::

    python3 skill/scripts/gen_field_doc.py            # 生成并校验
    python3 skill/scripts/gen_field_doc.py --check    # 只校验，不写文件
"""

import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import lookup  # noqa: E402
import paths  # noqa: E402

RULES_3 = paths.rules_path()
OUT_MD = paths.doc_path("fofa_fields.md")

#: rules/3.txt 用 ✓/- 表示"支持/不支持"。
_OPS = ("=", "!=", "*=")

#: 真正的字段名形如 ip / cert.subject.cn；3.txt 里有 "after&before" 这种
#: 把两个字段写在一行的示例条目，它不是字段，应当跳过而不是当成新字段。
_FIELD_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_.]*$")


def _is_field(name):
    return bool(_FIELD_RE.match(name))


def parse_rules(path):
    """解析 3.txt 的字段表。

    返回 ``[(小节名, [(字段, [(示例, 描述, 运算符元组), ...]), ...]), ...]``。
    六列的行是新字段，五列的行是上一字段的追加示例。
    """
    with open(path, "r", encoding="utf-8") as fh:
        lines = [ln.rstrip("\n") for ln in fh]

    sections = []
    current_section = None
    rows_by_field = {}
    last_field = None

    for idx, line in enumerate(lines):
        stripped = line.strip()
        if not stripped:
            continue
        cols = [c.strip() for c in stripped.split("\t")]

        # 表头行（Syntax Example Description = != *=）标识一个小节的开始
        if cols[0] == "Syntax" and len(cols) >= 6:
            if current_section is not None:
                sections.append((current_section, rows_by_field))
            current_section = _pending_title(lines, idx)
            rows_by_field = {}
            last_field = None
            continue
        if current_section is None:
            continue

        tail = cols[-3:]
        if len(tail) != 3 or any(t not in ("\u2713", "-") for t in tail):
            # 表头之上那行是小节标题，例如 " General"
            continue
        if len(cols) >= 6:
            field = cols[0]
            example, desc = cols[1], cols[2]
        elif len(cols) == 5:
            # 追加示例行，没有字段名
            if last_field is None:
                continue
            field = last_field
            example, desc = cols[0], cols[1]
        else:
            continue

        # 3.txt 把 product.version 这类带点字段写在语法列，但也存在
        # "cert.subject" 这种示例里出现别的字段名的情况，一律按语法列取。
        field = field.strip()
        if _is_field(field):
            last_field = field
        rows_by_field.setdefault(field, []).append(
            (example, desc, tuple(
                (op if t == "\u2713" else None) for op, t in zip(_OPS, tail))))

    if current_section is not None:
        sections.append((current_section, rows_by_field))
    return sections


def _pending_title(lines, idx):
    """小节标题在表头行的前一行，例如 " General" / " Special Label"。

    必须传入表头行的真实下标：表头行在各个小节里逐字相同，用 lines.index
    去找只会永远命中第一个小节。
    """
    for back in range(idx - 1, -1, -1):
        cand = lines[back].strip()
        if cand and not cand.startswith("Syntax"):
            return cand
    return "未命名小节"


def render(sections):
    out = []
    out.append("# FOFA 字段速查表")
    out.append("")
    out.append("本文件由 `python3 skill/scripts/gen_field_doc.py` 从 "
               "`rules/3.txt` 解析生成，**请勿手改**。")
    out.append("")
    out.append("末三列是 `rules/3.txt` 字段表原样的运算符支持情况：")
    out.append("")
    # 用单引号写 Python 字符串，避免 "" 被当成相邻字面量隐式拼接而消失
    out.append('- `=` 相等匹配。`字段=""` 表示该字段不存在或值为空。')
    out.append('- `==` 完全匹配。`字段==""` 表示该字段存在。')
    out.append('- `!=` 不匹配。`字段!=""` 表示该字段值不为空。')
    out.append("- `*=` 模糊匹配，用 `*` 匹配任意串、`?` 匹配单字符。")
    out.append("")
    out.append("下表按 3.txt 的小节组织；字段名后的运算符取自三列交叉核对结果。")
    out.append("")

    total_fields = 0
    for title, rows_by_field in sections:
        out.append("## " + title)
        out.append("")
        out.append("| 字段 | 支持运算符 | 示例 | 说明 |")
        out.append("| --- | --- | --- | --- |")
        for field, rows in rows_by_field.items():
            if not _is_field(field):
                continue
            ops = None
            for _ex, _desc, cand in rows:
                ops = cand
                break
            # 同一字段的多行示例运算符应一致，取任一即可
            for _ex, _desc, cand in rows:
                if cand != ops:
                    raise SystemExit(
                        "3.txt 中字段 %s 的运算符列前后不一致: %r vs %r"
                        % (field, ops, cand))
            op_str = " ".join(op for op in ops if op) if any(ops) else "(无)"
            total_fields += 1
            for i, (example, desc, _cand) in enumerate(rows):
                shown = field if i == 0 else "&nbsp;&nbsp;↳"
                out.append("| `%s` | %s | `%s` | %s |"
                           % (shown, op_str if i == 0 else "",
                              _md_cell(example), _md_cell(desc)))
        out.append("")

    out.append("## 对账")
    out.append("")
    out.append("`rules/3.txt` 共解析出 **%d** 个字段。" % total_fields)
    out.append("`lookup.FIELD_OPERATORS` 收录 **%d** 个字段。"
               % len(lookup.FIELD_OPERATORS))
    out.append("")
    return "\n".join(out) + "\n"


def _md_cell(text):
    """去掉会破坏 Markdown 表格的竖线。"""
    return text.replace("|", "\\|").strip()


def main(argv):
    if not os.path.exists(RULES_3):
        print("找不到规则源: " + RULES_3)
        return 2
    sections = parse_rules(RULES_3)

    # 与 lookup.FIELD_OPERATORS 对账：写法不同就报错，避免文档悄悄过期
    problems = []
    parsed_fields = set()
    for _title, rows_by_field in sections:
        for field, rows in rows_by_field.items():
            if not _is_field(field):
                continue
            parsed_fields.add(field)
            ops = tuple(op for op in rows[0][2] if op)
            allowed = lookup.FIELD_OPERATORS.get(field)
            if allowed is None:
                continue  # 解析出的非字段行，忽略
            # lookup 里 == 随 = 放行，3.txt 没有 == 列，故只比 = / != / *=
            if tuple(op for op in ops) != tuple(op for op in allowed):
                problems.append("字段 %s: 3.txt=%s  lookup=%s"
                                % (field, " ".join(ops), " ".join(allowed)))

    known = set(lookup.FIELD_OPERATORS)
    missing = sorted(known - parsed_fields)
    extra = sorted(parsed_fields - known)
    if problems:
        for p in problems:
            print("[不一致] " + p)
    if missing:
        print("[lookup 有而 3.txt 未解析出] " + "、".join(missing))
    if extra:
        print("[3.txt 有而 lookup 未收录] " + "、".join(extra))

    if "--check" in argv[1:]:
        if problems or missing or extra:
            print("校验未通过")
            return 1
        print("校验通过：%d 个字段，运算符一致" % len(parsed_fields))
        return 0

    doc = render(sections)
    os.makedirs(os.path.dirname(OUT_MD), exist_ok=True)
    with open(OUT_MD, "w", encoding="utf-8") as fh:
        fh.write(doc)
    print("字段表已写入: " + OUT_MD)
    print("小节 %d 个，字段 %d 个" % (len(sections), len(parsed_fields)))
    if problems or missing or extra:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
