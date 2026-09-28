r"""FOFA 查询构造工具库。

仅依赖 Python 标准库，面向 FOFA Skill 提供三类能力：

1. Query 片段构造与组合（转义、AND/OR 分组、布尔标记、值校验）；
2. 需要实算的两种特征：证书序列号十六进制转十进制、favicon 的 icon_hash；
3. 语法体检（lint），把全角符号、括号引号不配对等低级错误拦在提交之前。

语法依据来自 workspace 里 ``rules/3.txt``（最权威的字段表）与 ``rules/1.txt``
（总纲）。本模块不访问网络，可离线运行、可重复执行、可回滚。

用法::

    python3 lookup.py selftest      # 跑内置自检向量
    python3 lookup.py lint 'app="gitlab"'
"""

from __future__ import annotations

import base64
import re
import struct
import sys

# ---------------------------------------------------------------------------
# 常量
# ---------------------------------------------------------------------------

#: 无法转换时必须原样填写的固定串（docs/README.md 规定）
FIXED_ANSWER = "该需求不能直接转换为FOFA搜索语句"

#: 常用国家/地区中文名 -> FOFA country 字段要求的 ISO 3166-1 alpha-2 代码
COUNTRY_CODES = {
    "中国": "CN",
    "中国大陆": "CN",
    "中华人民共和国": "CN",
    "大不列颠及北爱尔兰联合王国": "GB",
    "英国": "GB",
    "美国": "US",
    "美利坚合众国": "US",
    "日本": "JP",
    "韩国": "KR",
    "德国": "DE",
    "法国": "FR",
    "荷兰": "NL",
    "加拿大": "CA",
    "土耳其": "TR",
    "土耳其共和国": "TR",
    "新西兰": "NZ",
    "香港": "HK",
    "中国香港": "HK",
    "澳门": "MO",
    "中国澳门": "MO",
    "台湾": "TW",
    "中国台湾": "TW",
    "俄罗斯": "RU",
    "新加坡": "SG",
    "印度": "IN",
    "澳大利亚": "AU",
    "巴西": "BR",
    "意大利": "IT",
    "西班牙": "ES",
    "瑞士": "CH",
    "瑞典": "SE",
    "挪威": "NO",
    "芬兰": "FI",
    "丹麦": "DK",
    "比利时": "BE",
    "奥地利": "AT",
    "波兰": "PL",
    "乌克兰": "UA",
    "墨西哥": "MX",
    "阿根廷": "AR",
    "南非": "ZA",
    "阿联酋": "AE",
    "以色列": "IL",
    "泰国": "TH",
    "越南": "VN",
    "马来西亚": "MY",
    "印度尼西亚": "ID",
    "菲律宾": "PH",
    "爱尔兰": "IE",
    "葡萄牙": "PT",
    "希腊": "GR",
    "捷克": "CZ",
    "匈牙利": "HU",
    "罗马尼亚": "RO",
}

#: “中国大陆 / 不含港澳台”类需求需要显式排除的地区代码
CN_EXCLUDE = ("HK", "MO", "TW")

#: rules/3.txt 中出现过的“形似而实非”的全角/弯引号字符及其修正提示
LOOKALIKE = {
    "\u201c": "弯双引号，应使用英文 \"",
    "\u201d": "弯双引号，应使用英文 \"",
    "\u2018": "弯单引号，应使用英文 '",
    "\u2019": "弯单引号，应使用英文 '",
    "\uff02": "全角双引号，应使用英文 \"",
    "\u2033": "双撇号，应使用英文 \"",
    "\uff3c": "全角反斜线，应使用 \\",
    "\uff5c": "全角竖线，应使用 ||",
    "\uff08": "全角左括号，应使用 (",
    "\uff09": "全角右括号，应使用 )",
    "\u3001": "全角顿号，应使用 &&",
    "\uff0b": "全角加号",
    "\uff1d": "全角等号，应使用 =",
    "\uff06": "全角 and 符，应使用 &&",
}


# ---------------------------------------------------------------------------
# 转义与片段构造
# ---------------------------------------------------------------------------


def escape(value) -> str:
    r"""把字面值转成可安全放进英文双引号的形式。

    rules/3.txt 的取值一律包在英文双引号内，因此只有两种字符必须转义：
    反斜杠 ``\`` 与双引号 ``"``。其余字符（空格、点、冒号、斜杠等）
    不必转义，FOFA 按字面串处理。
    """
    text = str(value)
    text = text.replace("\\", "\\\\")
    text = text.replace('"', '\\"')
    return text


def eq(field: str, value) -> str:
    """``field="value"``：匹配，对文本类字段表现为包含。"""
    return field + '="' + escape(value) + '"'


def exact(field: str, value) -> str:
    """``field=="value"``：完整匹配，逐字符相等。"""
    return field + '=="' + escape(value) + '"'


def ne(field: str, value) -> str:
    """``field!="value"``：不匹配。"""
    return field + '!="' + escape(value) + '"'


def not_empty(field: str) -> str:
    """``field!=""``：字段值非空。"""
    return field + '!=""'


def is_empty(field: str) -> str:
    """``field=""``：字段不存在或取值为空。"""
    return field + '=""'


def fz(field: str, value) -> str:
    """``field*="*value*"``：模糊包含，等价于“包含 value”。"""
    return field + '*="*' + escape(value) + '*"'


def pattern(field: str, pattern_text: str) -> str:
    """``field*="pat"``：通配符模糊匹配，``*`` 任意长度，``?`` 单字符。"""
    return field + '*="' + pattern_text + '"'


def flag(field: str, value: bool) -> str:
    """布尔字段，例如 ``is_domain="true"``。"""
    return field + '="' + ("true" if value else "false") + '"'


def AND(*terms: str) -> str:
    """用 && 连接并整体加括号；单元素或空输入时保持原样。"""
    parts = [t for t in terms if t]
    if not parts:
        return ""
    if len(parts) == 1:
        return parts[0]
    return "(" + " && ".join(parts) + ")"


def OR(*terms: str) -> str:
    """用 || 连接并整体加括号；单元素或空输入时保持原样。"""
    parts = [t for t in terms if t]
    if not parts:
        return ""
    if len(parts) == 1:
        return parts[0]
    return "(" + " || ".join(parts) + ")"


def cn_exclude_terms() -> list:
    """返回排除港澳台的三个 country 条件，用于显式排除场景。"""
    return [ne("country", code) for code in CN_EXCLUDE]


# ---------------------------------------------------------------------------
# 值校验
# ---------------------------------------------------------------------------

_IPV4_RE = re.compile(r"^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$")


def is_valid_ipv4(text) -> bool:
    """严格校验 IPv4：四段，每段 0-255。"""
    m = _IPV4_RE.match(str(text).strip())
    if not m:
        return False
    return all(0 <= int(g) <= 255 for g in m.groups())


def is_valid_cidr(text) -> bool:
    """校验 ``a.b.c.d/prefix``，prefix 取 0-32。"""
    raw = str(text).strip()
    if "/" not in raw:
        return False
    addr, _, prefix = raw.partition("/")
    if not is_valid_ipv4(addr):
        return False
    if not prefix.isdigit():
        return False
    return 0 <= int(prefix) <= 32


def is_valid_port(text) -> bool:
    """端口必须是 1-65535 的十进制整数。"""
    raw = str(text).strip()
    if not raw.isdigit():
        return False
    return 1 <= int(raw) <= 65535


def b_segment(ip: str) -> str:
    """把 IPv4 归一到所在 B 段，返回 ``a.b.0.0/16``。"""
    raw = str(ip).strip()
    if not is_valid_ipv4(raw):
        raise ValueError("不是合法 IPv4: " + raw)
    a, b, _c, _d = raw.split(".")
    return a + "." + b + ".0.0/16"


def ip_range_to_or(start: str, end: str, limit: int = 4096) -> str:
    """把闭区间 IP 展开成 ``(ip="..." || ip="...")``。

    地址数超过 ``limit`` 时直接拒绝，避免生成超长的穷举串（FOFA 没有
    区间语法，只能逐个 OR）。
    """
    def to_int(text) -> int:
        if not is_valid_ipv4(text):
            raise ValueError("不是合法 IPv4: " + str(text))
        a, b, c, d = (int(x) for x in str(text).strip().split("."))
        return (a << 24) | (b << 16) | (c << 8) | d

    lo, hi = to_int(start), to_int(end)
    if lo > hi:
        lo, hi = hi, lo
    if hi - lo + 1 > limit:
        raise ValueError("区间含 %d 个地址，超过上限 %d" % (hi - lo + 1, limit))
    terms = []
    for n in range(lo, hi + 1):
        terms.append(eq("ip", ".".join(str((n >> s) & 0xFF) for s in (24, 16, 8, 0))))
    return OR(*terms)


def hex_to_dec(text) -> str:
    """把带冒号或空格分隔的十六进制串转成十进制字符串（cert.sn 需要）。"""
    raw = re.sub(r"[^0-9A-Fa-f]", "", str(text))
    if not raw:
        raise ValueError("十六进制串为空")
    return str(int(raw, 16))


# ---------------------------------------------------------------------------
# icon_hash
# ---------------------------------------------------------------------------


def murmur3_32(key, seed: int = 0) -> int:
    """MurmurHash3 x86_32，返回有符号整数。

    FOFA 的 icon_hash 正是 ``murmur3_32(base64.encodebytes(favicon))``。
    自检向量：``murmur3_32(b"foo") == -156908512``。
    """
    data = bytes(key)
    length = len(data)
    h = seed & 0xFFFFFFFF
    c1 = 0xCC9E2D51
    c2 = 0x1B873593
    blocks = length // 4
    for i in range(blocks):
        k = int.from_bytes(data[i * 4:i * 4 + 4], "little")
        k = (k * c1) & 0xFFFFFFFF
        k = ((k << 15) | (k >> 17)) & 0xFFFFFFFF
        k = (k * c2) & 0xFFFFFFFF
        h ^= k
        h = ((h << 13) | (h >> 19)) & 0xFFFFFFFF
        h = (h * 5 + 0xE6546B64) & 0xFFFFFFFF
    tail = data[blocks * 4:]
    k = 0
    for j in range(len(tail) - 1, -1, -1):
        k = (k << 8) ^ tail[j]
    if tail:
        k = (k * c1) & 0xFFFFFFFF
        k = ((k << 15) | (k >> 17)) & 0xFFFFFFFF
        k = (k * c2) & 0xFFFFFFFF
        h ^= k
    h ^= length
    h ^= h >> 16
    h = (h * 0x85EBCA6B) & 0xFFFFFFFF
    h ^= h >> 13
    h = (h * 0xC2B2AE35) & 0xFFFFFFFF
    h ^= h >> 16
    if h >= 0x80000000:
        return h - 0x100000000
    return h


def favicon_hash(content: bytes) -> str:
    """按 FOFA 既定算法计算 favicon 的 icon_hash 字符串。"""
    return str(murmur3_32(base64.encodebytes(bytes(content))))


# ---------------------------------------------------------------------------
# 语法体检
# ---------------------------------------------------------------------------

_STRING_RE = re.compile(r'"(?:[^"\\]|\\.)*"')

#: 一个合法查询项：field=value / field==value / field!=value / field*=pattern
_TERM_RE = re.compile(r'([A-Za-z_][A-Za-z0-9_.]*)(==|\*=|!=|=)("(?:[^"\\]|\\.)*")')

#: rules/3.txt 里出现过、我们可能用到的字段名
KNOWN_FIELDS = frozenset([
    "ip", "port", "domain", "host", "os", "server", "asn", "org",
    "is_domain", "is_ipv6", "app", "fid", "product", "product.version",
    "category", "type", "cloud_name", "is_cloud", "is_fraud", "is_honeypot",
    "protocol", "banner", "banner_hash", "banner_fid", "base_protocol",
    "title", "header", "header_hash", "body", "body_hash", "js_name", "js_md5",
    "cname", "cname_domain", "icon_hash", "status_code", "sdk_hash",
    "country", "region", "city", "cert", "cert.subject", "cert.issuer",
    "cert.subject.org", "cert.subject.cn", "cert.issuer.org", "cert.issuer.cn",
    "cert.domain", "cert.is_equal", "cert.is_valid", "cert.is_match",
    "cert.is_expired", "jarm", "tls.version", "tls.ja3s", "cert.sn",
    "cert.not_after.after", "cert.not_after.before", "cert.not_before.after",
    "cert.not_before.before", "after", "before", "port_size", "port_size_gt",
   "port_size_lt", "ip_ports", "ip_country", "ip_region", "ip_city",
   "ip_after", "ip_before",
])

#: rules/3.txt 每张字段表末三列（= / != / *=）抄录成的运算符白名单。
#: 表里标 ``-`` 即该字段不支持此运算符，例如 region 只支持 ``=`` 与 ``!=``，
#: 写成 ``region*="*Hebei*"`` 属非法（``=`` 本身已是包含匹配）。
#: ``==`` 未出现在表内，FOFA 视作 ``=`` 的精确版，故随 ``=`` 一起放行。
FIELD_OPERATORS = {
    "ip": ("=", "!="),
    "port": ("=", "!=", "*="),
    "domain": ("=", "!=", "*="),
    "host": ("=", "!=", "*="),
    "os": ("=", "!=", "*="),
    "server": ("=", "!=", "*="),
    "asn": ("=", "!=", "*="),
    "org": ("=", "!=", "*="),
    "is_domain": ("=",),
    "is_ipv6": ("=",),
    "app": ("=",),
    "fid": ("=", "!="),
    "product": ("=", "!="),
    "product.version": ("=", "!="),
    "category": ("=", "!="),
    "type": ("=",),
    "cloud_name": ("=", "!=", "*="),
    "is_cloud": ("=",),
    "is_fraud": ("=",),
    "is_honeypot": ("=",),
    "protocol": ("=", "!=", "*="),
    "banner": ("=", "!="),
    "banner_hash": ("=", "!="),
    "banner_fid": ("=", "!="),
    "base_protocol": ("=", "!="),
    "title": ("=", "!=", "*="),
    "header": ("=", "!="),
    "header_hash": ("=", "!=", "*="),
    "body": ("=", "!="),
    "body_hash": ("=", "!="),
    "js_name": ("=", "!=", "*="),
    "js_md5": ("=", "!=", "*="),
    "cname": ("=", "!=", "*="),
    "cname_domain": ("=", "!=", "*="),
    "icon_hash": ("=", "!="),
    "status_code": ("=", "!="),
    # rules/3.txt 的 sdk_hash 一行是三列 "✓ ✓ -"，即不支持 *=
    "sdk_hash": ("=", "!="),
    "country": ("=", "!="),
    "region": ("=", "!="),
    "city": ("=", "!="),
    "cert": ("=", "!="),
    "cert.subject": ("=", "!=", "*="),
    "cert.issuer": ("=", "!=", "*="),
    "cert.subject.org": ("=", "!=", "*="),
    "cert.subject.cn": ("=", "!=", "*="),
    "cert.issuer.org": ("=", "!=", "*="),
    "cert.issuer.cn": ("=", "!=", "*="),
    "cert.domain": ("=", "!=", "*="),
    "cert.is_equal": ("=",),
    "cert.is_valid": ("=",),
    "cert.is_match": ("=",),
    "cert.is_expired": ("=",),
    "jarm": ("=", "!=", "*="),
    "tls.version": ("=", "!="),
    "tls.ja3s": ("=", "!=", "*="),
    "cert.sn": ("=", "!="),
    "cert.not_after.after": ("=",),
    "cert.not_after.before": ("=",),
    "cert.not_before.after": ("=",),
    "cert.not_before.before": ("=",),
    "after": ("=",),
    "before": ("=",),
    "port_size": ("=", "!="),
    "port_size_gt": ("=",),
    "port_size_lt": ("=",),
    "ip_ports": ("=",),
    "ip_country": ("=",),
    "ip_region": ("=",),
    "ip_city": ("=",),
    "ip_after": ("=",),
    "ip_before": ("=",),
}

assert set(FIELD_OPERATORS) == set(KNOWN_FIELDS), \
    "FIELD_OPERATORS 与 KNOWN_FIELDS 的字段集合不一致"


#: rules/3.txt 末节 "Unique IP View" 的字段。该节表头原文即声明：
#: "These unique IP series syntax cannot be used together with the other
#: syntax mentioned above." —— 这一族字段必须单独成句，不能和其他字段混写。
UNIQUE_IP_FIELDS = frozenset([
    "port_size", "port_size_gt", "port_size_lt", "ip_ports", "ip_country",
    "ip_region", "ip_city", "ip_after", "ip_before",
])


def lint(query) -> list:
    """体检一条 FOFA Query，返回人类可读的问题列表（空列表表示通过）。"""
    problems = []
    if query is None:
        return ["查询为 None"]
    text = str(query)
    if not text:
        return ["查询为空字符串"]
    if text != text.strip():
        problems.append("首尾存在空白字符")
    if "\n" in text or "\r" in text or "\t" in text:
        problems.append("查询中含换行或制表符，必须是单行")

    for ch, hint in LOOKALIKE.items():
        if ch in text:
            problems.append("含" + hint)

    if text.count("(") != text.count(")"):
        problems.append("圆括号不配对：左 %d 右 %d" % (text.count("("), text.count(")")))

    in_string = False
    escaped = False
    for ch in text:
        if escaped:
            escaped = False
            continue
        if ch == "\\":
            escaped = True
            continue
        if ch == '"':
            in_string = not in_string
    if in_string:
        problems.append("双引号不成对")
    if escaped:
        problems.append("查询以孤立反斜杠结尾")

    # 剥掉所有合法查询项后，剩余内容只应是指定运算符与圆括号。
    # 注意顺序：先把运算符和括号换成空格，最后再统一去空白；
    # 若先删空白，替换产生的空格会被当成残留内容而误报。
    residue = _TERM_RE.sub(" ", text)
    for token in ("&&", "||", "(", ")"):
        residue = residue.replace(token, " ")
    residue = re.sub(r"\s+", "", residue)
    if residue:
        problems.append("存在无法解析的片段: " + repr(residue))

    # 字段与运算符要搭得拢，例如 region 不支持 *=
    for m in _TERM_RE.finditer(text):
        field, op = m.group(1), m.group(2)
        allowed = FIELD_OPERATORS.get(field)
        if allowed is None:
            continue  # 未知字段名由下面的标识符扫描统一报告
        check = "=" if op == "==" else op
        if check not in allowed:
            problems.append("字段 %s 不支持运算符 %s（可用：%s）" %
                            (field, m.group(2), " ".join(allowed)))

    # Unique IP View 一族的字段必须单独成句，不能和其他语法混写
    used = [m.group(1) for m in _TERM_RE.finditer(text)]
    unique_used = sorted(set(f for f in used if f in UNIQUE_IP_FIELDS))
    mixed = sorted(set(f for f in used if f not in UNIQUE_IP_FIELDS))
    if unique_used and mixed:
        problems.append("Unique IP View 字段 %s 不能与 %s 等其他语法混用"
                        % ("、".join(unique_used), "、".join(mixed)))

    # 只识别查询项左侧的标识符：先把双引号内的字面值挖空，避免把
    # body="class=\"container\"" 里的 class 误判成字段名
    bare = _STRING_RE.sub('""', text)
    for m in re.finditer(r"([A-Za-z_][A-Za-z0-9_.]*)", bare):
        token = m.group(1)
        low = token.lower()
        if low in ("and", "or", "not"):
            problems.append("使用了英文逻辑词 " + token + "，应使用 && 或 ||")
        elif token not in KNOWN_FIELDS:
            problems.append("未知字段名: " + token)
    return problems


# ---------------------------------------------------------------------------
# 自检
# ---------------------------------------------------------------------------

_SELFTEST_BLOB = b"fofa-icon-selftest-vector\x00\x01\x02"


def selftest() -> int:
    """跑内置向量。全部通过返回 0，否则返回 1 并打印差异。"""
    cases = [
        ("escape 反斜杠与引号", escape('a\\b"c'), 'a\\\\b\\"c'),
        ("escape 纯文本", escape("NoSuchUpload"), "NoSuchUpload"),
        ("eq", eq("app", "gitlab"), 'app="gitlab"'),
        ("exact", exact("title", "HFS /"), 'title=="HFS /"'),
        ("ne", ne("country", "HK"), 'country!="HK"'),
        ("not_empty", not_empty("banner"), 'banner!=""'),
        ("is_empty", is_empty("title"), 'title=""'),
        ("flag false", flag("is_domain", False), 'is_domain="false"'),
        ("AND", AND(eq("port", "80"), eq("country", "CN")), '(port="80" && country="CN")'),
        ("OR", OR(eq("port", "80"), eq("port", "443")), '(port="80" || port="443")'),
        ("AND 单元素", AND(eq("port", "80")), 'port="80"'),
        ("ipv4 合法", is_valid_ipv4("20.247.40.92"), True),
        ("ipv4 非法", is_valid_ipv4("1.2.3.4.5"), False),
        ("port 合法", is_valid_port("3000"), True),
        ("port 越界", is_valid_port("1000000"), False),
        ("cidr 合法", is_valid_cidr("124.255.254.0/24"), True),
        ("cidr 非法", is_valid_cidr("1.2.3.4.5/24"), False),
        ("b_segment", b_segment("40.88.14.254"), "40.88.0.0/16"),
        ("hex_to_dec", hex_to_dec("63:BD:83:71:62:14:83:6E:F8:00:75:35:61:61:EB:06"),
         "132577581667764526475870473477991557894"),
        ("murmur3 foo", murmur3_32(b"foo"), -156908512),
        ("murmur3 seeded", murmur3_32(b"foo", 42), -1322301282),
        ("murmur3 empty", murmur3_32(b""), 0),
        ("favicon_hash", favicon_hash(_SELFTEST_BLOB), "1936783277"),
        ("ip_range_to_or", ip_range_to_or("47.96.0.1", "47.96.0.2"),
         '(ip="47.96.0.1" || ip="47.96.0.2")'),
    ]
    failures = 0
    for name, got, want in cases:
        if got != want:
            failures += 1
            print("[FAIL] %-20s got=%r want=%r" % (name, got, want))
        else:
            print("[ ok ] %-20s %r" % (name, got))

    lint_cases = [
        ('app="gitlab"', True),
        ('(a="1" && b="2")', False),        # 未知字段，应报错
        ('country="CN" AND title="x"', False),
        ('app="gitlab" && title="A', False),
        ('title="\uff11\uff12"', False),       # 全角数字也属可疑残留
        ('domain*="??.cn"', True),
        ('title=="HFS /"', True),
        ('body="class=\\"container\\""', True),
    ]
    lint_cases[4] = ('"', False)   # 半个引号
    # 字段与运算符要搭对：rules/3.txt 的 = / != / *= 三列里的 - 就是不支持
    lint_cases.extend([
        ('region*="*Hebei*"', False),   # region 只支持 = / !=
        ('region="Hebei"', True),
        ('header*="Server: nginx"', False),  # header 只支持 = / !=
        ('body*="login"', False),       # body 只支持 = / !=
        ('app*="Apache"', False),       # app 只支持 =
        ('is_domain="false"', True),
        ('title=""', True),
        ('server=="Apache"', True),     # == 随 = 放行
        # Unique IP View 一族的字段必须单独成句，不得与其他语法混写
        ('ip_country="US"', True),
        ('(ip_country="US" && ip_ports="80,443")', True),
        ('ip_country="US" && port="443"', False),
        ('port_size="6" && country="CN"', False),
    ])
    for sample, should_be_clean in lint_cases:
        issues = lint(sample)
        if should_be_clean and issues:
            failures += 1
            print("[FAIL] lint 误报 %r -> %r" % (sample, issues))
        elif not should_be_clean and not issues:
            failures += 1
            print("[FAIL] lint 漏报 %r" % sample)
        else:
            print("[ ok ] lint %-5s -> %r" % ("clean" if should_be_clean else "dirty", issues))

    if failures:
        print("selftest: %d 个用例失败" % failures)
        return 1
    print("selftest: 全部通过（%d 个用例）" % (len(cases) + len(lint_cases)))
    return 0


def main(argv) -> int:
    if len(argv) < 2:
        print((__doc__ or "").strip())
        return 2
    cmd = argv[1]
    if cmd == "selftest":
        return selftest()
    if cmd == "lint":
        if len(argv) < 3:
            print("用法: lookup.py lint <query>")
            return 2
        issues = lint(argv[2])
        for item in issues:
            print("问题: " + item)
        print("lint 通过" if not issues else "lint 未通过")
        return 1 if issues else 0
    print("未知子命令: " + cmd)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
