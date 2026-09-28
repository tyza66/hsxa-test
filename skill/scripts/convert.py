#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""自然语言 -> FOFA 查询语句的机械模式引擎。

定位：这是 SKILL.md 流程里的第一道工序，只处理"照着字面就能映射"的需求，
例如单个 IP、单个网段、单个端口、单个协议、单个国家。它刻意做得保守：一旦
需求里出现它没有建模的条件（证书、标题、蜜罐、端口多选……），或者同一字段出现
多个不同取值，``convert()`` 就返回 ``None`` 把题目交回给模型——宁可不答，
也不给一个看似精确实则漏条件的错答案。

另外，机械层还负责把"一看就无解"的需求钉死成固定答案：非法 IPv4、非法 CIDR、
越界端口、同一字段自相矛盾（既等于又不等于）这四类不需要模型再判。

用法::

    python3 skill/scripts/convert.py "请查询 IP 地址为 20.247.40.92 的资产。"
    python3 skill/scripts/convert.py --题目 docs/题目.txt
    python3 skill/scripts/convert.py --selftest
"""

import argparse
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import lookup  # noqa: E402
import paths  # noqa: E402


# ---------------------------------------------------------------------------
# 取值词典
# ---------------------------------------------------------------------------

#: 国别名 -> ISO 3166-1 alpha-2。FOFA 的 country 用两位国别代码。
COUNTRY_ALIASES = {
    "中国": "CN", "中国大陆": "CN", "中华人民共和国": "CN",
    "china": "CN", "mainland china": "CN",
    "美国": "US", "美利坚合众国": "US", "united states": "US", "usa": "US",
    "日本": "JP", "japan": "JP",
    "德国": "DE", "germany": "DE", "deutschland": "DE",
    "英国": "GB", "大不列颠": "GB", "united kingdom": "GB",
    "荷兰": "NL", "netherlands": "NL", "holland": "NL",
    "新西兰": "NZ", "new zealand": "NZ",
    "韩国": "KR", "南韩": "KR", "south korea": "KR", "korea": "KR",
    "法国": "FR", "france": "FR",
    "俄罗斯": "RU", "russia": "RU",
    "加拿大": "CA", "canada": "CA",
    "土耳其": "TR", "turkey": "TR", "turkiye": "TR",
    "新加坡": "SG", "singapore": "SG",
    "印度": "IN", "india": "IN",
    "巴西": "BR", "brazil": "BR",
    "澳大利亚": "AU", "澳洲": "AU", "australia": "AU",
    "意大利": "IT", "italy": "IT",
    "西班牙": "ES", "spain": "ES",
    "越南": "VN", "vietnam": "VN",
    "泰国": "TH", "thailand": "TH",
    "印度尼西亚": "ID", "印尼": "ID", "indonesia": "ID",
    "马来西亚": "MY", "malaysia": "MY",
    "菲律宾": "PH", "philippines": "PH",
    "墨西哥": "MX", "mexico": "MX",
    "阿根廷": "AR", "argentina": "AR",
    "智利": "CL", "chile": "CL",
    "南非": "ZA", "south africa": "ZA",
    "埃及": "EG", "egypt": "EG",
    "以色列": "IL", "israel": "IL",
    "阿联酋": "AE", "uae": "AE",
    "沙特": "SA", "saudi arabia": "SA",
    "瑞典": "SE", "sweden": "SE",
    "挪威": "NO", "norway": "NO",
    "芬兰": "FI", "finland": "FI",
    "波兰": "PL", "poland": "PL",
    "乌克兰": "UA", "ukraine": "UA",
    "瑞士": "CH", "switzerland": "CH",
    "奥地利": "AT", "austria": "AT",
    "比利时": "BE", "belgium": "BE",
    "爱尔兰": "IE", "ireland": "IE",
    "丹麦": "DK", "denmark": "DK",
    "葡萄牙": "PT", "portugal": "PT",
    "希腊": "GR", "greece": "GR",
    "捷克": "CZ", "czech": "CZ",
    "匈牙利": "HU", "hungary": "HU",
    "罗马尼亚": "RO", "romania": "RO",
    "香港": "HK", "hong kong": "HK",
    "澳门": "MO", "macao": "MO", "macau": "MO",
    "台湾": "TW", "taiwan": "TW",
}

#: 把 ISO 代码本身也登记成别名，例如题目直接写 "US"。
#: 全大写两位码只按词边界匹配，避免把 "us" / "in" 这类英文词当成国家。
for _code in sorted(set(COUNTRY_ALIASES.values())):
    COUNTRY_ALIASES[_code] = _code

#: 服务名 -> FOFA protocol 取值。rules/3.txt 只给了 "quic" 这类示例，没有完整
#: 枚举表，所以命中协议一律降级为中置信度，提醒人工核对真实取值。
PROTOCOL_ALIASES = {
    "ssh": "ssh", "ftp": "ftp", "dns": "dns", "snmp": "snmp",
    "memcached": "memcached", "redis": "redis", "mysql": "mysql",
    "socks5": "socks5", "socks4": "socks4", "telnet": "telnet",
    "smtp": "smtp", "pop3": "pop3", "imap": "imap", "http": "http",
    "https": "https", "rdp": "rdp", "vnc": "vnc", "ntp": "ntp",
    "tftp": "tftp", "rsync": "rsync", "ldap": "ldap", "sip": "sip",
    "rtsp": "rtsp", "mqtt": "mqtt", "coap": "coap", "modbus": "modbus",
    "bacnet": "bacnet", "mongodb": "mongodb", "postgresql": "postgresql",
    "amqp": "rabbitmq", "kafka": "kafka", "zookeeper": "zookeeper",
    "elasticsearch": "elasticsearch", "docker": "docker",
    "websocket": "websocket",
}

#: 产品名 -> FOFA app 指纹名。只收写法稳定的几个，其余留给模型判断。
APP_ALIASES = {
    "jenkins": "Jenkins",
    "grafana": "Grafana",
    "wordpress": "WordPress",
    "gitlab": "GitLab",
    "hfs": "HFS",
    "海康威视": "HIKVISION", "海康": "HIKVISION", "hikvision": "HIKVISION",
    "大华": "dahua", "dahua": "dahua",
    "tomcat": "Tomcat",
    "zabbix": "Zabbix",
    "prometheus": "Prometheus",
    "kibana": "Kibana",
}


# ---------------------------------------------------------------------------
# 出现这些词说明需求里有机械层没有建模的维度，直接交回模型
# ---------------------------------------------------------------------------

DEFER_MARKERS = (
    "证书", "cert", "标题", "title", "intitle", "正文", "body", "intext",
    "响应头", "响应信息", "header", "蜜罐", "honeypot", "欺诈", "仿冒",
    "fraud", "已绑定", "绑定域名", "子域", "subdomain", "图标", "favicon",
    "icon_hash", "jarm", "ja3s", "tls", "ssl", "组织", "org", "asn",
    "云", "cloud", "版本", "version", "产品", "product", "cve",
    "指纹", "fid", "操作系统", "网站名", "城市", "地区", "省", "州",
    "更新", "after", "before", "排除", "不要", "以外", "除了", "不等于",
    "不是", "不含", "或者", "任一", "任意", "都算", "都要", "服务响应",
    "服务器", "server", "运营商", "备案",
    # 地域后缀。单字，只收在本批题目里确实指向行政区划的几个；
    # 刻意不收"都"——它在需求里几乎总是"全部"的意思（如"每一级都只有一个
    # 字符"），收了会大面积误弃权。
    "市", "县", "道",
    # 否定式措辞。"不含"匹配不到"不包含"（中间隔着"包"），必须单独列。
    "不包含", "不包括", "不含有", "港澳台",
    # 平台来源词。这类需求只是"把别处的搜索意图搬过来"，域名是来源不是目标，
    # 例如"把 urlscan.io 搜索意图转换成 FOFA"里的 urlscan.io 不能被检索。
    # 特意不收 google —— M041/M050 里 google.com 恰恰是证书要查的目标。
    "urlscan", "shodan", "censys", "zoomeye",
)

#: 需求在提"网段/区间"但没给出 CIDR 时的兜底标记——给了 CIDR 就不再拦，
#: 因为那时词已经被 CIDR 本身消化掉了。
_CIDR_MARKERS = ("网段", "B 段", "b 段", "C 段", "c 段", "整个", "区间", "范围")


# ---------------------------------------------------------------------------
# 基础识别器
# ---------------------------------------------------------------------------

#: 合法形态的 CIDR：四段 + "/" + 一到两位掩码。
_CIDR_RE = re.compile(
    r"(?<![\d./])(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})/(\d{1,2})(?![\d.])")

#: 形态像 CIDR 但掩码位数或地址段不合法的形态，用于把"给了个坏网段"识别出来。
_BAD_CIDR_RE = re.compile(
    r"(?<![\d./])(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})/\d{1,3}(?![\d.])")

#: 四段以上点分数字串。故意不收三段，免得把 "2.4.49" 这类版本号当地址。
_DOTTED_RE = re.compile(r"(?<![\d.])(\d{1,3}(?:\.\d{1,3}){3,6})(?!\d)")

_VERSION_RE = re.compile(r"(?<![\d.])\d+\.\d+\.\d+(?![\d.])")
_DOMAIN_RE = re.compile(
    r"(?<![\w.-])(?:[a-z0-9\u4e00-\u9fff]"
    r"(?:[a-z0-9\u4e00-\u9fff-]{0,61}[a-z0-9\u4e00-\u9fff])?\.)"
    r"{1,4}[a-z\u4e00-\u9fff]{2,24}(?![\w.-])", re.IGNORECASE)
_TLD_RE = re.compile(
    r"\.(com|cn|net|org|io|edu|gov|mil|int|info|biz|co|me|tv|cc|xyz|top|pro|"
    r"ar|br|de|es|fr|in|it|jp|kr|nl|ru|us|vn|th|id|my|ph|sg|tw|hk|mo|"
    r"au|ca|ch|se|no|fi|dk|pl|cz|gr|pt|ie|be|at|hu|ro|ua|za|eg|il|ae|sa|"
    r"mx|cl|pe|tr)(?![\w.-])", re.IGNORECASE)


def extract_addresses(text):
    """识别地址类 token。

    返回 ``[(token, 合法与否), ...]``。四段点分串逐段校验：段数不是四段
    （例如 1.2.3.4.5）、某段大于 255、或掩码越界，都算不合法。这是为了把
    "给了个坏地址"和"没给地址"区分开——前者必须判无解，后者只是换字段。
    """
    raw = str(text)
    found = []
    valid = set()
    for m in _CIDR_RE.finditer(raw):
        ok = lookup.is_valid_cidr(m.group(0))
        found.append((m.group(0), ok))
        if ok:
            valid.add(m.group(0))
    for m in _BAD_CIDR_RE.finditer(raw):
        if m.group(0) not in valid:
            found.append((m.group(0), False))
    masked = _CIDR_RE.sub(" ", raw)
    masked = _BAD_CIDR_RE.sub(" ", masked)
    for m in _DOTTED_RE.finditer(masked):
        token = m.group(0)
        parts = token.split(".")
        ok = (len(parts) == 4
              and all(p.isdigit() and int(p) <= 255 for p in parts))
        found.append((token, ok))
    return found


def extract_hostnames(text):
    """挑出看着像域名的串，排除地址、CIDR 和纯版本号。"""
    masked = _CIDR_RE.sub(" ", str(text))
    masked = _BAD_CIDR_RE.sub(" ", masked)
    masked = _DOTTED_RE.sub(" ", masked)
    masked = _VERSION_RE.sub(" ", masked)
    out = []
    for m in _DOMAIN_RE.finditer(masked):
        host = m.group(0).lower()
        if not _TLD_RE.search(host):
            continue
        if host not in out:
            out.append(host)
    return out


def extract_ports(text):
    """取"端口 N"与"N 端口"两种写法，越界值原样返回以便判无解。"""
    raw = str(text)
    out = []
    for m in re.finditer(r"端口\s*(?:为|是|开放|开|用|于|：|:)?\s*(\d{2,7})", raw):
        out.append(m.group(1))
    for m in re.finditer(r"(?<![\d.])(\d{2,9})(?![\d.])\s*端口", raw):
        out.append(m.group(1))
    seen = []
    for port in out:
        if port not in seen:
            seen.append(port)
    return seen


def extract_status_codes(text):
    out = []
    for m in re.finditer(
            r"(?:状态码|响应码|响应状态码|状态代码)\s*(?:为|是|等于|：|:)?\s*(\d{3})",
            str(text)):
        out.append(m.group(1))
    return out


def extract_countries(text):
    """按别名表找国家。中文别名直接包含匹配，两位码要求大写词边界。"""
    raw = str(text)
    low = raw.lower()
    found = []
    for alias, code in COUNTRY_ALIASES.items():
        if len(alias) == 2 and alias.isascii() and alias.isupper():
            hit = re.search(r"(?<![A-Za-z])" + alias + r"(?![A-Za-z])", raw)
        elif len(alias) <= 2:
            hit = re.search(r"(?<![a-z])" + re.escape(alias) + r"(?![a-z])", low)
        else:
            hit = alias in low if alias.isascii() else alias in raw
        if hit and code not in found:
            found.append(code)
    return found


def extract_protocols(text):
    low = str(text).lower()
    found = []
    for name, value in sorted(PROTOCOL_ALIASES.items()):
        if re.search(r"(?<![a-z0-9])" + re.escape(name) + r"(?![a-z0-9])", low):
            if value not in found:
                found.append(value)
    return found


def extract_apps(text):
    low = str(text).lower()
    found = []
    for name, value in sorted(APP_ALIASES.items()):
        if name in low and value not in found:
            found.append(value)
    return found


def find_country_conflict(text):
    """同一国家既被要求等于又被要求不等于时返回国别码，否则返回 None。"""
    raw = str(text)
    tail = r"[^。；;，,]{0,12}?"
    positive = re.findall(
        # (?<!不) 是必需的：否则惰性 tail 会把"国家**不**是中国"里的"不"
        # 吞掉再去匹配"是"，凭空造出一个 country="CN" 的正向条件，把原本
        # 合法的需求误判成自相矛盾（M013 曾因此被错判为固定答案）。
        r"国家(?:代码)?" + tail + r"(?<!\u4e0d)(?:等于|是|为)\s*"
        r"([A-Za-z\u4e00-\u9fff][A-Za-z\u4e00-\u9fff ]{0,7})", raw)
    negative = re.findall(
        r"国家(?:代码)?" + tail + r"(?:不等于|不是|不为|排除)\s*"
        r"([A-Za-z\u4e00-\u9fff][A-Za-z\u4e00-\u9fff ]{0,7})", raw)
    pos = set()
    for token in positive:
        code = COUNTRY_ALIASES.get(token.strip().lower()) \
            or COUNTRY_ALIASES.get(token.strip())
        if code:
            pos.add(code)
    for token in negative:
        code = COUNTRY_ALIASES.get(token.strip().lower()) \
            or COUNTRY_ALIASES.get(token.strip())
        if code and code in pos:
            return code
    return None


# ---------------------------------------------------------------------------
# 转换主入口
# ---------------------------------------------------------------------------

#: 条件组装顺序：网络 -> 域名 -> 地域 -> 服务 -> 响应。
#: 固定顺序让同类题目产出稳定字符串，回归测试才有意义。
_TERM_ORDER = ("ip", "domain", "country", "protocol", "app", "port",
               "status_code")


def convert(text):
    """把一句自然语言需求转成 FOFA 查询。

    返回 ``(查询语句, 置信度, 说明)``。查询语句为 None 表示机械层不接这题，
    应交给模型按 SKILL.md 的判据分析。置信度 ``H``/``M``/``L`` 与
    build_answers.py 一致。
    """
    if not text or not str(text).strip():
        return None, None, "输入为空"
    raw = str(text)
    low = raw.lower()
    notes = []

    # --- 一、非法取值：先于一切正常分支，能判无解就判 -------------------
    addresses = extract_addresses(raw)
    bad = [token for token, ok in addresses if not ok]
    if bad:
        return (lookup.FIXED_ANSWER, "H",
                "%s 不是合法 IPv4 或 CIDR，无法解析成地址或网段" % bad[0])
    for port in extract_ports(raw):
        if not lookup.is_valid_port(port):
            return (lookup.FIXED_ANSWER, "H",
                    "端口 %s 超出 TCP/UDP 合法范围 1-65535" % port)
    conflict = find_country_conflict(raw)
    if conflict:
        return (lookup.FIXED_ANSWER, "H",
                '同一字段既要求 country="%s" 又要求 country!="%s"，逻辑自相矛盾'
                % (conflict, conflict))

    # --- 二、收集可确定的条件 ------------------------------------------
    cidrs = [t for t, ok in addresses if ok and "/" in t]
    ipv4s = [t for t, ok in addresses if ok and "/" not in t]
    ports = extract_ports(raw)
    codes = extract_status_codes(raw)
    countries = extract_countries(raw)
    protocols = extract_protocols(raw)
    apps = extract_apps(raw)
    hosts = extract_hostnames(raw)

    # 同族多个不同取值多半是"或"关系，机械层不猜连接关系
    for name, values in (("IP", ipv4s), ("网段", cidrs), ("端口", ports),
                         ("状态码", codes), ("国家", countries),
                         ("协议", protocols), ("产品", apps),
                         ("域名", hosts)):
        if len(values) > 1:
            return None, None, ("识别到 %d 个不同的%s，需模型判断是 AND 还是 OR"
                                % (len(values), name))

    # --- 三、有没有机械层没建模的条件 ----------------------------------
    markers = []
    if not cidrs:
        # "网段/B 段/区间"这类词在已给出 CIDR 时已经被消化，不再是未知维度
        markers.extend(_CIDR_MARKERS)
    markers.extend(DEFER_MARKERS)
    unmodeled = []
    for marker in markers:
        if marker in raw or (marker.isascii() and marker in low):
            unmodeled.append(marker)
    if unmodeled:
        return None, None, "出现机械层未建模的条件: " + "、".join(unmodeled)

    # --- 四、组装 ------------------------------------------------------
    terms = []
    if ipv4s:
        terms.append(lookup.eq("ip", ipv4s[0]))
    if cidrs:
        terms.append(lookup.eq("ip", cidrs[0]))
    if hosts:
        terms.append(lookup.eq("domain", hosts[0]))
        notes.append("域名映射到 domain")
    if countries:
        terms.append(lookup.eq("country", countries[0]))
    if protocols:
        terms.append(lookup.eq("protocol", protocols[0]))
        notes.append("协议取值需对照站点实测核对")
    if apps:
        terms.append(lookup.eq("app", apps[0]))
        notes.append("app 指纹名需对照站点实测核对")
    if ports:
        terms.append(lookup.eq("port", ports[0]))
    if codes:
        terms.append(lookup.eq("status_code", codes[0]))

    if not terms:
        return None, None, "没有识别到可直接映射的条件，需模型分析"

    if len(terms) == 1:
        # 协议和 app 的取值没有权威枚举表兜底，单独命中也压成中置信度
        conf = "M" if (protocols or apps) else "H"
        return terms[0], conf, "；".join(notes) or "单一条件直译"

    query = lookup.AND(*terms)
    issues = lookup.lint(query)
    if issues:
        return None, None, "生成的查询未通过语法体检: " + "; ".join(issues)
    notes.append("%d 个条件做 AND" % len(terms))
    return query, "M", "；".join(notes)


# ---------------------------------------------------------------------------
# 批处理与自检
# ---------------------------------------------------------------------------

def load_questions(path):
    with open(path, "rb") as fh:
        raw = fh.read()
    if raw.startswith(b"\xef\xbb\xbf"):
        raw = raw[3:]
    return [(row["题号"], row["自然语言输入"])
            for row in json.loads(raw.decode("utf-8"))]


def run_batch(questions, out_path):
    """机械层尽力而为：能转的转，不能转的留空并说明原因。"""
    result = {}
    for qid, text in questions:
        query, confidence, note = convert(text)
        if query is None:
            result[qid] = None
        else:
            result[qid] = {"查询语句": query, "置信度": confidence, "说明": note}
    with open(out_path, "w", encoding="utf-8") as fh:
        json.dump(result, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    return result


def selftest():
    """内置用例：该给的给，不该管的别管。"""
    cases = [
        ("请查询 IP 地址为 20.247.40.92 的资产。", 'ip="20.247.40.92"', "H"),
        ("请查询 124.255.254.0/24 网段内的全部资产。", 'ip="124.255.254.0/24"', "H"),
        ("请查询开放 3000 端口的资产。", 'port="3000"', "H"),
        ("请查询主域是 zendesk.com 的全部资产。", 'domain="zendesk.com"', "H"),
        ("我想找中国的 DNS 服务。", '(country="CN" && protocol="dns")', "M"),
        ("搜索 SSH 协议且端口为 22022 的资产。", '(protocol="ssh" && port="22022")', "M"),
        ("我想找互联网上提供 SNMP 网络管理服务的资产，不限定端口。",
         'protocol="snmp"', "M"),
        # 非法取值 -> 固定答案
        ("搜索开放 1000000 端口的资产。", lookup.FIXED_ANSWER, "H"),
        ("搜索地址或网段为 1.2.3.4.5 的资产。", lookup.FIXED_ANSWER, "H"),
        ("搜索网段 10.0.0.0/999 内的资产。", lookup.FIXED_ANSWER, "H"),
        ("查同一条资产记录，国家代码必须等于 US，同时国家代码又必须不等于 US。",
         lookup.FIXED_ANSWER, "H"),
        # 拿不准 -> 交回模型
        ("帮我查最安全的网站。", None, None),
        ("找日本已绑定域名的网站：443 端口、响应码 200、证书有效。", None, None),
        ("我想要搜索这个 IP 的整个 B 段：40.88.14.254。", None, None),
        ("我想找 113.247.228.0/24 网段里开放 80、443、8080 或 8443 任意一个端口的资产。",
         None, None),
        ("我想找美国 OVH SAS 网络里开放 2087 端口、标题完整为 WHM 登录。", None, None),
        ("我想找证书信息里包含 google.com 这个域名的资产，蜜罐和欺诈网站都不要。",
         None, None),
        # 回归：正向正则曾被"不是"骗出一个 country="CN"，误判自相矛盾
        ("搜索标题包含 Jenkins 或正文包含 jenkins、国家不是中国，且更新时间在 2026-08-01 之后的资产。",
         None, None),
        # 回归：来源平台域名不是检索目标
        ("把这个 urlscan.io 搜索意图转换成 FOFA：HTML 包含 login 的资产。", None, None),
        ("搜索厦门市 80 端口且网页状态码为 200 的资产。", None, None),
        ("请帮我查询不包含港澳台的中国 Grafana 资产。", None, None),
    ]
    failures = 0
    for text, want_query, want_conf in cases:
        query, conf, note = convert(text)
        if query != want_query or conf != want_conf:
            failures += 1
            print("[FAIL] " + text[:38])
            print("       got  query=%r conf=%r note=%s" % (query, conf, note))
            print("       want query=%r conf=%r" % (want_query, want_conf))
        else:
            print("[ ok ] %-38s -> %r" % (text[:38], query))
    if failures:
        print("selftest: %d 个用例失败" % failures)
        return 1
    print("selftest: 全部通过（%d 个用例）" % len(cases))
    return 0


def main(argv):
    parser = argparse.ArgumentParser(description="自然语言 -> FOFA 查询（机械模式）")
    parser.add_argument("文本", nargs="?", help="一句自然语言需求")
    parser.add_argument("--题目", help="题目 JSON 路径，批量模式用")
    parser.add_argument("--输出", default=os.path.join(paths.tests_dir(), "convert.json"),
                        help="批量模式输出路径")
    parser.add_argument("--selftest", action="store_true", help="跑内置用例")
    args = parser.parse_args(argv[1:])

    if args.selftest:
        return selftest()
    if args.题目:
        questions = load_questions(args.题目)
        result = run_batch(questions, args.输出)
        auto = sum(1 for v in result.values() if v)
        print("题目 %d 道，机械层直接转换 %d 道，交回模型 %d 道"
              % (len(questions), auto, len(result) - auto))
        print("结果已写入: " + args.输出)
        return 0
    if args.文本:
        query, conf, note = convert(args.文本)
        if query is None:
            print("机械层不处理: " + note)
            return 1
        print(query)
        print("置信度: %s  说明: %s" % (conf, note))
        # 固定答案表示“此题无解”，不是真实查询语句，跳过语法体检（否则必然误报）
        issues = [] if query == lookup.FIXED_ANSWER else lookup.lint(query)
        if issues:
            print("语法体检未通过: " + "; ".join(issues))
            return 1
        return 0
    parser.print_help()
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
