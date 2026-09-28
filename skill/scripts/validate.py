r"""答案文件校验器（依据 docs/README.md 与 docs/答题模板.txt 的硬约束）。

校验项逐条如下：

1. 文件不超过 8 MB、UTF-8 解码、允许带 BOM；
2. 顶层必须是 JSON 对象，且恰好含有 选手名称 / 参赛包编号 / 答案；
3. 答案是非空数组，数组元素是对象；
4. 题号集合与题目.json 完全一致：缺题、重复题号、未知题号都判拒绝；
5. 每个元素只能有 题号 与 查询语句 两个字段；
6. 查询语句非空，且不是固定串时须通过 lookup.lint 语法体检；
7. 查询语句里不得出现 Markdown 代码块、多余解释或换行。

用法::

    python3 validate.py <答案.json> [--题目 docs/题目.txt] [--参赛包编号 pkg-xxx]

不传 `--参赛包编号` 时，默认按 `docs/参赛包.txt` 首行的包号校验；
换答题包后无需改脚本或文档，文件是唯一口径。

退出码：0 通过；1 拒绝；2 用法或读取错误。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import lookup  # noqa: E402
import paths  # noqa: E402

#: docs/README.md 规定：无法转换时唯一允许的固定串
FIXED_ANSWER = lookup.FIXED_ANSWER

#: 题号形如 M001-S009
QUESTION_ID_RE = re.compile(r"^M\d{3}-S\d{3}$")

#: 答案文件大小上限 8 MB
MAX_BYTES = 8 * 1024 * 1024

REQUIRED_TOP = ("选手名称", "参赛包编号", "答案")


def _decode(path):
    """读文件并按 UTF-8 解码，允许 BOM。"""
    with open(path, "rb") as fh:
        raw = fh.read()
    if len(raw) > MAX_BYTES:
        raise ValueError("文件大小 %d 字节，超过 8 MB 上限" % len(raw))
    if raw.startswith(b"\xef\xbb\xbf"):
        raw = raw[3:]
    try:
        return raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise ValueError("不是合法 UTF-8 编码: %s" % exc)


def _load_json(text, path):
    try:
        return json.loads(text)
    except json.JSONDecodeError as exc:
        raise ValueError("%s 不是合法 JSON: %s" % (path, exc))


def load_questions(path):
    """读取题目文件，返回按出现顺序排列的题号列表。"""
    if not os.path.isfile(path):
        raise ValueError("题目文件不存在: " + path)
    text = _decode(path)
    data = _load_json(text, path)
    if not isinstance(data, list) or not data:
        raise ValueError("题目文件必须是包含题目的 JSON 数组")
    seen = []
    for idx, item in enumerate(data):
        if not isinstance(item, dict):
            raise ValueError("第 %d 题不是 JSON 对象" % (idx + 1))
        if set(item.keys()) != {"题号", "自然语言输入"}:
            raise ValueError("第 %d 题字段必须恰为 题号/自然语言输入，实际 %r"
                             % (idx + 1, sorted(item.keys())))
        qid = item["题号"]
        if not isinstance(qid, str) or not QUESTION_ID_RE.match(qid):
            raise ValueError("第 %d 题题号非法: %r" % (idx + 1, qid))
        seen.append(qid)
    return seen


def _lint_query(text):
    """查询语句级别的附加检查。"""
    problems = []
    if text != text.strip():
        problems.append("查询语句首尾有空白")
    if "\n" in text or "\r" in text:
        problems.append("查询语句含换行")
    if "```" in text:
        problems.append("查询语句含 Markdown 代码块标记")
    if text and text != FIXED_ANSWER:
        problems.extend(lookup.lint(text))
    return problems


def validate(answer_path, questions_path=None, package_id=None):
    """校验答案文件。返回 (是否通过, 问题列表, 结构化信息)。"""
    problems = []
    info = {"题目数": 0, "答案数": 0, "固定答案数": 0, "可转换数": 0}

    if not os.path.isfile(answer_path):
        return False, ["答案文件不存在: " + answer_path], info

    try:
        text = _decode(answer_path)
    except ValueError as exc:
        return False, [str(exc)], info

    try:
        data = _load_json(text, answer_path)
    except ValueError as exc:
        return False, [str(exc)], info

    if not isinstance(data, dict):
        problems.append("顶层必须是 JSON 对象，实际是 %s" % type(data).__name__)
        return False, problems, info

    keys = set(data.keys())
    if keys != set(REQUIRED_TOP):
        problems.append("顶层字段必须恰为 %r，实际 %r" % (list(REQUIRED_TOP), sorted(keys)))
        return False, problems, info

    if not isinstance(data["选手名称"], str) or not data["选手名称"].strip():
        problems.append("选手名称 必须是非空字符串")
    if not isinstance(data["参赛包编号"], str) or not data["参赛包编号"].strip():
        problems.append("参赛包编号 必须是非空字符串")

    answers = data["答案"]
    if not isinstance(answers, list) or not answers:
        problems.append("答案 必须是非空 JSON 数组")
        return False, problems, info
    info["答案数"] = len(answers)

    # 题号集合一致性：这一步不通过就不必再看内容，直接整体拒绝
    order = []
    for idx, item in enumerate(answers):
        if not isinstance(item, dict):
            problems.append("第 %d 个答案元素不是 JSON 对象" % (idx + 1))
            continue
        if set(item.keys()) != {"题号", "查询语句"}:
            problems.append("第 %d 个答案字段必须恰为 题号/查询语句，实际 %r"
                            % (idx + 1, sorted(item.keys())))
            continue
        qid = item["题号"]
        if not isinstance(qid, str) or not QUESTION_ID_RE.match(qid):
            problems.append("第 %d 个答案题号非法: %r" % (idx + 1, qid))
            continue
        order.append(qid)

    if problems:
        return False, problems, info

    duplicate = sorted({q for q in order if order.count(q) > 1})
    if duplicate:
        problems.append("题号重复: " + ", ".join(duplicate))

    if questions_path:
        try:
            expected = load_questions(questions_path)
        except ValueError as exc:
            problems.append(str(exc))
            return False, problems, info
        info["题目数"] = len(expected)
        missing = sorted(set(expected) - set(order))
        unknown = sorted(set(order) - set(expected))
        if missing:
            problems.append("缺题 %d 道: %s" % (len(missing), ", ".join(missing)))
        if unknown:
            problems.append("未知题号 %d 个: %s" % (len(unknown), ", ".join(unknown)))
        if len(order) != len(expected):
            problems.append("答案条数 %d 与题目数 %d 不等" % (len(order), len(expected)))
        if problems:
            return False, problems, info

    if package_id and data["参赛包编号"] != package_id:
        problems.append("参赛包编号必须是 %s，实际 %s" % (package_id, data["参赛包编号"]))

    # 内容检查
    for idx, item in enumerate(answers):
        qid = item["题号"]
        query = item["查询语句"]
        if not isinstance(query, str) or not query.strip():
            problems.append("%s 查询语句为空" % qid)
            continue
        if query == FIXED_ANSWER:
            info["固定答案数"] += 1
            continue
        info["可转换数"] += 1
        for issue in _lint_query(query):
            problems.append("%s %s" % (qid, issue))

    return (not problems), problems, info


def main(argv=None):
    parser = argparse.ArgumentParser(description="校验 FOFA 答案文件是否符合 docs/README.md 的硬约束")
    parser.add_argument("answer", help="答案 JSON 路径")
    parser.add_argument("--题目", default=paths.questions_path(),
                        help="题目 JSON 路径（默认工作区 docs/题目.txt，FOFA_WORKSPACE 可改）")
    parser.add_argument("--参赛包编号", default=paths.package_id(),
                        help="期望的参赛包编号（默认工作区 docs/参赛包.txt 首行，FOFA_WORKSPACE 可改）")
    args = parser.parse_args(argv)

    ok, problems, info = validate(args.answer, args.题目, args.参赛包编号)
    print("题目数=%d 答案数=%d 可转换=%d 固定答案=%d"
          % (info["题目数"], info["答案数"], info["可转换数"], info["固定答案数"]))
    for issue in problems:
        print("  [拒绝] " + issue)
    print("结果: " + ("通过" if ok else "拒绝"))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
