r"""路径解析：让 Skill 的脚本从任意工作目录、任意安装形态下都能定位输入输出。

支持三种形态，行为一致：

* 仓库形态 —— ``<repo>/skill/scripts/``，即 git 里的源码副本；
* 链接形态 —— ``~/.codex/skills/fofa-query-builder`` 软链到 ``<repo>/skill``；
* 拷贝形态 —— 整份拷进 ``~/.codex/skills/fofa-query-builder/``。

前两种形态通过 ``realpath`` 还原真实仓库根，``docs/题目.txt``、``rules/3.txt``、
``skill/`` 下的交付物全部就位。拷贝形态没有仓库可依附，回退到从当前目录向上
寻找带 ``docs/题目.txt`` 的工作区；都找不到就要求调用方显式传路径，不猜。
"""

from __future__ import annotations

import os

#: 工作区标记：本仓库的题目文件
MARKER = os.path.join("docs", "题目.txt")

#: 环境变量，可显式指定工作区，优先级最高
ENV_WORKSPACE = "FOFA_WORKSPACE"


def _real_script_dir():
    return os.path.dirname(os.path.realpath(__file__))


def skill_root():
    """skill/ 目录，含 scripts/、reference/、tests/。"""
    return os.path.dirname(_real_script_dir())


def repo_root():
    """脚本所属仓库根；不是仓库形态时返回 None。"""
    candidate = os.path.dirname(skill_root())
    if os.path.isfile(os.path.join(candidate, MARKER)):
        return candidate
    return None


def find_workspace(start=None):
    """定位带 docs/题目.txt 的工作区。

    顺序：环境变量 FOFA_WORKSPACE、脚本所属仓库、从 start（默认 CWD）向上找。
    找不到返回 None，此时调用方应要求显式传参而不是猜一个路径。
    """
    env = os.environ.get(ENV_WORKSPACE)
    if env:
        env = os.path.abspath(env)
        if os.path.isfile(os.path.join(env, MARKER)):
            return env
    root = repo_root()
    if root:
        return root
    current = os.path.abspath(start if start is not None else os.getcwd())
    while True:
        if os.path.isfile(os.path.join(current, MARKER)):
            return current
        parent = os.path.dirname(current)
        if parent == current:
            return None
        current = parent


def questions_path():
    """题目文件。找不到工作区时返回标记路径，由调用方报清晰的错。"""
    workspace = find_workspace()
    if workspace:
        return os.path.join(workspace, MARKER)
    return MARKER


def rules_path():
    """rules/3.txt，字段表的唯一权威源。"""
    workspace = find_workspace()
    if workspace:
        return os.path.join(workspace, "rules", "3.txt")
    return os.path.join("rules", "3.txt")


def tests_dir():
    """golden、快照等回归基线所在目录。"""
    return os.path.join(skill_root(), "tests")


def answers_dir():
    """答卷输出目录。

    仓库与链接形态下落在 ``<repo>/skill/answers``，与 git 里的布局一致；
    拷贝形态下跟着当前工作区走，落到 ``<workspace>/answers``。
    """
    root = repo_root()
    if root:
        return os.path.join(root, "skill", "answers")
    workspace = find_workspace()
    if workspace:
        return os.path.join(workspace, "answers")
    return os.path.join(skill_root(), "answers")


def doc_path(name):
    """reference/ 下的参考文档，如 doc_path("fofa_fields.md")。"""
    return os.path.join(skill_root(), "reference", name)
