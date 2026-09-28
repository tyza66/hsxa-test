r"""Skill 全量回归套件。

一条命令核对整条流水线的一致性，任何一条红了都说明判据或实现漂移了，先修再交付。

用法::

    python3 skill/tests/regress.py
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "..", "scripts"))

import paths  # noqa: E402

TESTS_DIR = paths.tests_dir()
SKILL_DIR = paths.skill_root()
ANSWERS = os.path.join(paths.answers_dir(), "答案.json")
QUESTIONS = paths.questions_path()
ROOT = paths.find_workspace() or SKILL_DIR
SCRIPTS = os.path.join(SKILL_DIR, "scripts")

PY = sys.executable or "python3"


def _run(script, *args):
    """跑一个 Skill 脚本，返回 (退出码, stdout, stderr)。"""
    proc = subprocess.run(
        [PY, os.path.join(SCRIPTS, script)] + list(args),
        capture_output=True, text=True, cwd=ROOT, timeout=600,
    )
    return proc.returncode, proc.stdout, proc.stderr


def _tail(text, limit=400):
    text = text.strip()
    return text if len(text) <= limit else text[-limit:]


def check_selftests(failures):
    """两个内置自检向量组。"""
    for script, args, label, expect in (
        ("lookup.py", ["selftest"], "lookup selftest", "全部通过"),
        ("convert.py", ["--selftest"], "convert selftest", "全部通过"),
    ):
        code, out, err = _run(script, *args)
        ok = code == 0 and expect in out
        print("[%s] %s" % ("ok" if ok else "FAIL", label))
        if not ok:
            failures.append(label)
            print("       退出码=%d" % code)
            for line in _tail(out).splitlines():
                print("       " + line)
            if err.strip():
                print("       stderr: " + _tail(err).replace("\n", "\n       "))


def _same(path_a, path_b):
    """两个文件内容逐字节一致。"""
    with open(path_a, "rb") as fh:
        left = fh.read()
    with open(path_b, "rb") as fh:
        right = fh.read()
    return left == right


def check_reproducible(tmp, failures):
    """临时目录重跑流水线，与已提交交付物逐字节对账。"""
    questions = QUESTIONS
    answer_tmp = os.path.join(tmp, "答案.json")
    golden_tmp = os.path.join(tmp, "golden.json")
    convert_tmp = os.path.join(tmp, "convert.json")

    code, out, err = _run(
        "build_answers.py",
        "--题目", questions,
        "--输出", answer_tmp,
        "--golden", golden_tmp,
        "--review", os.path.join(tmp, "review.md"),
    )
    ok = code == 0
    print("[%s] build_answers 重跑" % ("ok" if ok else "FAIL"))
    if not ok:
        failures.append("build_answers 重跑")
        for line in _tail(out).splitlines():
            print("       " + line)
        return None

    code, out, err = _run("convert.py", "--题目", questions, "--输出", convert_tmp)
    ok = code == 0
    print("[%s] convert 重跑" % ("ok" if ok else "FAIL"))
    if not ok:
        failures.append("convert 重跑")
        for line in _tail(out).splitlines():
            print("       " + line)
        return None

    committed = [
        (answer_tmp, ANSWERS),
        (golden_tmp, os.path.join(TESTS_DIR, "golden.json")),
        (convert_tmp, os.path.join(TESTS_DIR, "convert.json")),
    ]
    for regenerated, target in committed:
        ok = _same(regenerated, target)
        label = "复现 " + os.path.relpath(target, ROOT)
        print("[%s] %s" % ("ok" if ok else "FAIL", label))
        if not ok:
            failures.append(label)
            print("       临时产出与已提交文件不一致")
    return answer_tmp


def check_doc_sync(failures):
    """字段表与 rules/3.txt 对账。"""
    code, out, err = _run("gen_field_doc.py", "--check")
    ok = code == 0
    print("[%s] 字段表同步" % ("ok" if ok else "FAIL"))
    if not ok:
        failures.append("字段表同步")
        for line in _tail(out).splitlines():
            print("       " + line)


def check_answer_schema(answer_path, failures):
    """对答卷跑提交前校验。"""
    pkg = "pkg-08e82c8d"
    code, out, err = _run(
        "validate.py", answer_path,
        "--题目", QUESTIONS,
        "--参赛包编号", pkg,
    )
    ok = code == 0
    print("[%s] 答卷格式校验" % ("ok" if ok else "FAIL"))
    if not ok:
        failures.append("答卷格式校验")
        for line in _tail(out).splitlines():
            print("       " + line)


def check_stats(failures):
    """题目、golden、答卷三处规模对齐，防止悄悄掉题。"""
    with open(os.path.join(TESTS_DIR, "golden.json"), encoding="utf-8") as fh:
        golden = json.load(fh)
    with open(ANSWERS, encoding="utf-8") as fh:
        answer = json.load(fh)
    with open(QUESTIONS, encoding="utf-8") as fh:
        raw = fh.read()
    if raw.startswith("\ufeff"):
        raw = raw[1:]
    questions = json.loads(raw)
    counts = (len(questions), len(golden), len(answer["答案"]))
    ok = counts == (100, 100, 100)
    print("[%s] 题量对齐 题目/golden/答卷 = %d/%d/%d"
          % ("ok" if ok else "FAIL", counts[0], counts[1], counts[2]))
    if not ok:
        failures.append("题量对齐")


def main():
    failures = []
    print("== FOFA Skill 回归 ==")
    print("仓库根: " + ROOT)
    print("")
    check_selftests(failures)
    with tempfile.TemporaryDirectory(prefix="fofa-regress-") as tmp:
        answer = check_reproducible(tmp, failures)
        check_doc_sync(failures)
        if answer:
            check_answer_schema(answer, failures)
    check_stats(failures)
    print("")
    if failures:
        print("回归失败 %d 项: %s" % (len(failures), ", ".join(failures)))
        return 1
    print("回归全部通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
