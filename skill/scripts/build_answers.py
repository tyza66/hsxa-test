r"""把 docs/题目.txt 的 100 道自然语言需求映射成 FOFA Query 并生成答卷。

映射表 ``A`` 是“题目号 -> (查询语句, 置信度, 说明)”的字典。所有查询语句都
通过 lookup 的构造器拼装，转义与括号由工具保证，避免手写引号出错。

置信度取值：

* ``high``   规则明确、字段/运算符直接对应，可直接提交；
* ``medium`` 方向确定，但个别取值（地域英文名、指纹 app 名、字段归属）
              依赖外部知识，需要抽查复核；
* ``low``     需求本身含糊，答案是一次权衡，建议人工过目。

低/中置信度条目会在 ``skill/tests/review.md`` 里汇总成待确认清单。

用法::

    python3 build_answers.py                 # 按默认路径生成
    python3 build_answers.py --题目 <路径>    # 指定题目文件
"""

from __future__ import annotations

import argparse
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import lookup  # noqa: E402
import paths  # noqa: E402

PLAYER = "tyza66"
PKG = "pkg-08e82c8d"

# 复用频率高的取值
AMAZON_AWS = "Amazon AWS"
HFS_JAR = "21d21d00021d21d00021d21d21d21dce319294781f0e12867fd06896a000ea"
JA3S_TR = "f4febc55ea12b31ae17cfb7e614afda8"
SERIAL_HEX = "63:BD:83:71:62:14:83:6E:F8:00:75:35:61:61:EB:06"
SERIAL_DEC = "132577581667764526475870473477991557894"

# M082 的六个主域
M082_DOMAINS = [
    "kuaihuoyun.com",
    "wlhyos.cn",
    "wlhyos.com",
    "56ctms.com",
    "xiaokuaikeji.com",
    "zhihuiwuliu.org.cn",
]

# M090 的响应头：前七个只要求字段名存在，后两个保留材料里的原值
M090_NAME_ONLY = [
    "Accept-Ranges",
    "Cache-Control",
    "Content-Type",
    "Date",
    "Last-Modified",
    "Server",
    "Vary",
]

H = "high"
M = "medium"
L = "low"


# ---------------------------------------------------------------------------
# 映射表
# ---------------------------------------------------------------------------

A = {

    # ---- M001 ~ M012：IP、域名、地理位置、端口 --------------------------
    "M001-S009": (lookup.eq("ip", "20.247.40.92"), H, "单个 IPv4"),
    "M002-S004": (lookup.eq("ip", "124.255.254.0/24"), H, "24 位网段"),
    "M003-S002": (lookup.eq("domain", "zendesk.com"), H, "主域用 domain"),
    "M004-S003": (lookup.eq("host", "www.amazon.com"), H, "网站名用 host"),
    "M005-S004": (lookup.fz("domain", "vpn"), H, "域名包含 vpn"),
    "M006-S002": (lookup.eq("country", "GB"), H, "英国 -> 国别代码 GB"),
    "M007-S007": (lookup.AND(lookup.eq("country", "KR"),
                             lookup.eq("region", "Gangwon")),
                 M, "江原道英文名取 Gangwon；region 不支持 *=，= 的包含语义可兜底 Gangwon-do 一类写法"),
    "M008-S009": (lookup.AND(lookup.eq("country", "DE"),
                             lookup.eq("region", "Bavaria"),
                             lookup.eq("city", "Munich")),
                  M, "München -> Munich，Bavaria -> Bayern，均为英文地名待核"),
    "M009-S008": (lookup.eq("asn", "4134"), H, "自治系统号"),
    "M010-S009": (lookup.AND(lookup.eq("country", "NL"),
                             lookup.eq("cloud_name", AMAZON_AWS)),
                  H, "亚马逊的云计算/对象存储/应用托管都归 Amazon AWS"),
    "M011-S005": (lookup.AND(lookup.eq("region", "Hebei"),
                             lookup.eq("cloud_name", "Huawei Cloud")),
                 M, "华为云的 cloud_name 取值待核；region 不支持 *=，= 已是包含匹配"),
    "M012-S009": (lookup.eq("port", "3000"), H, "端口"),

    # ---- M013 ~ M025：协议、指纹、版本、分类 ---------------------------
    "M013-S003": (lookup.AND(lookup.ne("country", "CN"),
                             lookup.eq("after", "2026-08-01"),
                             lookup.OR(lookup.eq("title", "Jenkins"),
                                       lookup.eq("body", "jenkins"))),
                  H, "“不是中国”按字面取 country!=\"CN\""),
    "M014-S002": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.eq("protocol", "dns")),
                  H, "DNS 服务用 protocol"),
    "M015-S001": (lookup.eq("protocol", "socks5"),
                  M, "SOCKS v5 在 FOFA 里的 protocol 取值待核"),
    "M016-S006": (lookup.AND(lookup.eq("protocol", "ssh"),
                             lookup.eq("port", "22022")), H, "协议 + 端口"),
    "M017-S001": (lookup.eq("base_protocol", "tcp"), H, "传输层协议用 base_protocol"),
    "M018-S006": (lookup.eq("protocol", "snmp"), H, "SNMP 网络管理服务，不限定端口"),
    "M019-S010": (lookup.AND(lookup.eq("country", "US"),
                             lookup.eq("protocol", "memcached")),
                  M, "memcached 的 protocol 取值待核"),
    "M020-S005": (lookup.AND(lookup.eq("protocol", "ftp"),
                             lookup.eq("base_protocol", "tcp")),
                  H, "“真正跑在 TCP 上”= base_protocol=\"tcp\""),
    "M021-S010": (lookup.AND(lookup.eq("protocol", "ssh"),
                             lookup.eq("banner", "OpenSSH_10.4")),
                  H, "服务返回信息用 banner"),
    "M022-S007": (lookup.AND(lookup.eq("country", "JP"),
                             lookup.eq("app", "HIKVISION")),
                  M, "海康威视的 app 指纹名待核"),
    "M023-S002": (lookup.AND(lookup.eq("domain", "163.com"),
                             lookup.pattern("host", "?.?.163.com")),
                  M, "host 通配 ? 匹配单字符，构成两级单字符子域"),
    "M024-S005": (lookup.eq("product.version", "10.0.0"), H, "不限定产品则只查版本"),
    "M025-S010": (lookup.eq("category", "Middleware"),
                  L, "中间层软件的 category 英文取值为猜测，需实测核对"),

    # ---- M026 ~ M045：正文、响应头、证书 -------------------------------
    "M026-S002": (lookup.eq("body", "NoSuchUpload"), H, "正文包含"),
    "M027-S008": (lookup.eq("header", "Content-Security-Policy-Report-Only"),
                  H, "响应头包含字段名"),
    "M028-S011": (lookup.eq("status_code", "308"),
                  M, "“网站类资产”是否要补 type=\"subdomain\" 待核"),
    "M029-S010": (lookup.eq("banner", "SSH-2.0-OpenSSH_6.1"),
                  M, "“响应信息”取 banner，未叠加 protocol"),
    "M030-S003": (lookup.eq("header", "Set-Cookie:PHPSESSID"), M,
                  "题目原文无空格；真实 PHP 会话头多为 \"Set-Cookie: PHPSESSID=\"，必要时改拆成两项 AND"),
    "M031-S009": (lookup.eq("js_name", "jquery.min.js"), H, "引用 JS 用 js_name"),
    "M032-S010": (lookup.AND(lookup.eq("country", "NL"),
                             lookup.eq("city", "Amsterdam"),
                             lookup.eq("port", "554"),
                             lookup.eq("banner", "200 OK")),
                  M, "阿姆斯特丹的英文城市名待核"),
    "M033-S003": (lookup.AND(lookup.eq("cert.issuer.org", "GlobalSign"),
                             lookup.flag("cert.is_expired", False)),
                  H, "签发组织 + 未过期"),
    "M034-S007": (lookup.AND(lookup.flag("is_domain", True),
                             lookup.eq("cert.subject.cn", "apple.com"),
                             lookup.ne("domain", "apple.com")),
                  M, "“域名数据”补 is_domain=true"),
    "M035-S010": (lookup.eq("cert", "TrustAsia"), H, "证书全文包含"),
    "M036-S007": (lookup.eq("cert.subject.org", "Alibaba"), H, "持有者组织"),
    "M037-S008": (lookup.AND(lookup.eq("domain", "sohu.com"),
                             lookup.not_empty("banner"),
                             lookup.ne("banner", "apache")),
                  H, "banner 非空且不含 apache"),
    "M038-S009": (lookup.eq("cert.domain", "adobe.com"),
                  M, "“根域名”落到 cert.domain"),
    "M039-S001": (lookup.eq("cert.subject.cn", "qq.com"), H, "持有者名称 CN"),
    "M040-S011": (lookup.AND(lookup.eq("cert", "godaddy"),
                             lookup.ne("domain", "godaddy.com")),
                  M, "“根域名不是”取资产侧 domain，而非 cert.domain"),
    "M041-S001": (lookup.AND(lookup.eq("cert", "google.com"),
                             lookup.flag("is_honeypot", False),
                             lookup.flag("is_fraud", False)),
                  H, "蜜罐与欺诈都排除"),
    "M042-S003": (lookup.OR(lookup.eq("cert.subject.cn", "example.com"),
                            lookup.eq("cert.domain", "example.com")),
                  M, "裸域与子域由 = 的包含语义覆盖"),
    "M043-S004": (lookup.AND(lookup.eq("tls.version", "TLS 1.0"),
                             lookup.flag("cert.is_expired", False)),
                  H, "TLS 版本 + 未过期"),
    "M044-S007": (lookup.AND(lookup.eq("jarm", HFS_JAR),
                             lookup.eq("status_code", "307"),
                             lookup.eq("header", "Content-Length: 0")),
                  H, "三个条件并列"),
    "M045-S001": (lookup.eq("os", "Linux"),
                  M, "os 大小写（Linux / linux）待核"),

    # ---- M046 ~ M060 ---------------------------------------------------
    "M046-S013": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.eq("type", "subdomain"),
                             lookup.flag("is_domain", False)),
                  M, "“网站类资产”补 type=\"subdomain\""),
    "M047-S011": (lookup.flag("cert.is_equal", False),
                  H, "颁发者与持有者不匹配"),
    "M048-S001": (lookup.AND(lookup.eq("country", "CA"),
                             lookup.flag("cert.is_match", False)),
                  H, "证书与域名不匹配"),
    "M049-S010": (lookup.AND(lookup.eq("country", "TR"),
                             lookup.eq("tls.ja3s", JA3S_TR)), H, "JA3S 指纹"),
    "M050-S004": (lookup.eq("org", "Inc"), H, "网络组织名称包含 Inc"),
    "M051-S001": (lookup.AND(lookup.eq("country", "US"),
                             lookup.eq("org", "OVH SAS"),
                             lookup.eq("port", "2087"),
                             lookup.exact("title", "WHM 登录")),
                  H, "标题完整等于用 =="),
    "M052-S002": (lookup.AND(lookup.pattern("domain", "*.io"),
                             lookup.eq("app", "WordPress")), H, ".io 顶级域 + WordPress"),
    "M053-S009": (lookup.AND(lookup.eq("city", "Xiamen"),
                             lookup.eq("port", "80"),
                             lookup.eq("status_code", "200")),
                  M, "厦门的英文城市名待核"),
    "M054-S009": (lookup.AND(lookup.eq("country", "GB"),
                             lookup.eq("org", "DigitalOcean"),
                             lookup.eq("body", "/api/show")),
                  M, "“DigitalOcean 上”取 org，也可试 cloud_name"),
    "M055-S003": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.eq("header", "Set-Cookie"),
                             lookup.eq("body", "/admin")), H, "两类条件并列"),
    "M056-S008": (lookup.AND(lookup.eq("domain", "baidu.com"),
                             lookup.OR(lookup.eq("body", "Login"),
                                       lookup.eq("body", "admin"),
                                       lookup.eq("body", "Dashboard"),
                                       lookup.eq("body", "管理"),
                                       lookup.eq("header", "Set-Cookie"))),
                  H, "正文四项或响应头，任意命中"),
    "M057-S012": (lookup.AND(lookup.eq("ip", "13.0.0.0/8"),
                             lookup.eq("status_code", "200"),
                             lookup.OR(lookup.eq("body", "Welcome"),
                                       lookup.eq("body", "Sign in"),
                                       lookup.eq("body", "Dashboard"))),
                  H, "状态码必须，正文三项任一"),
    "M058-S012": (lookup.AND(lookup.eq("country", "DE"),
                             lookup.OR(lookup.eq("host", "amazonaws.com"),
                                       lookup.eq("domain", "amazonaws.com"),
                                       lookup.eq("cname_domain", "amazonaws.com"),
                                       lookup.eq("cert", "amazonaws.com"),
                                       lookup.eq("cert.domain", "amazonaws.com"))),
                  H, "访问地址/主域/CNAME 根域/证书全文/证书域名五处任一"),
    "M059-S006": (lookup.AND(lookup.eq("port", "143"),
                             lookup.eq("cert.sn", "1"),
                             lookup.OR(lookup.eq("cert", "TAK"),
                                       lookup.eq("cert", "ATAK"),
                                       lookup.eq("cert", "WinTAK"),
                                       lookup.eq("cert", "Squad"),
                                       lookup.eq("cert", "CoT"))),
                  H, "序列号 + 证书五个关键词任一"),
    "M060-S010": (lookup.AND(lookup.eq("ip", "113.247.228.0/24"),
                             lookup.OR(lookup.eq("port", "80"),
                                       lookup.eq("port", "443"),
                                       lookup.eq("port", "8080"),
                                       lookup.eq("port", "8443"))),
                  H, "四个端口任一"),


    # ---- M061 ~ M080 ---------------------------------------------------
    "M061-S005": (lookup.AND(lookup.eq("port", "80"),
                             lookup.flag("is_ipv6", False),
                             lookup.OR(lookup.eq("title", "Sign in"),
                                       lookup.eq("title", "Login"))),
                  H, "不是 IPv6 用 is_ipv6=false"),
    "M062-S010": (lookup.AND(lookup.eq("country", "NZ"),
                             lookup.flag("is_cloud", True),
                             lookup.OR(lookup.eq("port", "80"),
                                       lookup.eq("port", "443"))),
                  H, "“使用了云服务商”= is_cloud=true"),
    "M063-S001": (lookup.OR(lookup.AND(lookup.eq("banner", "IIS"),
                                       lookup.eq("banner", "1.")),
                            lookup.eq("header", "IIS")),
                  H, "banner 同时两项，或 header 命中"),
    "M064-S004": (lookup.AND(lookup.eq("country", "JP"),
                             lookup.flag("is_domain", True),
                             lookup.eq("port", "443"),
                             lookup.eq("status_code", "200"),
                             lookup.flag("cert.is_valid", True),
                             lookup.OR(lookup.eq("title", "大学"),
                                       lookup.eq("title", "学部"),
                                       lookup.eq("title", "研究"),
                                       lookup.eq("body", "入学案内"),
                                       lookup.eq("body", "学生"))),
                  H, "五个必填条件 + 标题三项或正文两项"),
    "M065-S004": (lookup.AND(lookup.eq("domain", "baidu.com"),
                             lookup.OR(lookup.eq("title", "系统"),
                                       lookup.eq("title", "平台"),
                                       lookup.eq("title", "登录"),
                                       lookup.eq("title", "管理"),
                                       lookup.eq("title", "API"),
                                       lookup.eq("host", "api"),
                                       lookup.eq("host", "admin"),
                                       lookup.eq("host", "dev"),
                                       lookup.eq("host", "test"),
                                       lookup.eq("host", "uat"),
                                       lookup.eq("host", "pre"))),
                  H, "子域名项用 host 包含匹配，pre 一类短词的误命中风险已记录"),
    "M066-S006": (lookup.AND(lookup.eq("category", "Mail"),
                             lookup.eq("title", "Mail"),
                             lookup.ne("country", "US")),
                  L, "“邮件系统”对应的 category 英文取值为猜测"),
    "M067-S008": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.ne("country", "HK"),
                             lookup.ne("country", "MO"),
                             lookup.ne("country", "TW"),
                             lookup.eq("port", "8888")),
                  H, "显式排除港澳台，与 country=\"CN\" 叠加不改变结果"),
    "M068-S005": (lookup.AND(lookup.eq("body", "Jenkins"),
                             lookup.ne("title", "Jenkins")),
                  H, "标题不含同一关键词"),
    "M069-S001": (lookup.AND(lookup.eq("country", "TW"),
                             lookup.eq("asn", "20940"),
                             lookup.eq("port", "443"),
                             lookup.eq("tls.version", "TLS 1.3"),
                             lookup.ne("cert.issuer.org", "Let's Encrypt"),
                             lookup.ne("cert.issuer.org", "ZeroSSL")),
                  H, "两个签发组织都排除"),
    "M070-S003": (lookup.AND(lookup.eq("app", "HFS"),
                             lookup.eq("status_code", "200"),
                             lookup.exact("title", "HFS /"),
                             lookup.ne("country", "JP"),
                             lookup.ne("cloud_name", AMAZON_AWS)),
                  M, "“亚马逊云”取 cloud_name，未用 is_cloud 以免误伤其他云"),
    "M071-S006": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.ne("country", "HK"),
                             lookup.ne("country", "MO"),
                             lookup.ne("country", "TW"),
                             lookup.eq("app", "Grafana")),
                  H, "中国但不含港澳台"),
    "M072-S008": (lookup.AND(lookup.pattern("domain", "*.edu.ar"),
                             lookup.OR(lookup.eq("port", "80"),
                                       lookup.eq("port", "443")),
                             lookup.ne("server", "nginx"),
                             lookup.ne("server", "WAF")),
                  M, "“服务是 Nginx 和 WAF 的不要”取 server 负向匹配"),
    "M073-S003": (lookup.AND(lookup.eq("ip", "1.1.1.1"),
                             lookup.flag("is_domain", True)),
                  H, "已绑定域名用 is_domain=true"),
    "M074-S008": (lookup.AND(lookup.eq("cert.domain", "aliyun.com"),
                             lookup.flag("cert.is_valid", True),
                             lookup.flag("is_domain", False)),
                  M, "“根域 aliyun.com”取 cert.domain；未绑定域名取 is_domain=false"),
    "M075-S006": (lookup.AND(lookup.eq("country", "CN"),
                             lookup.flag("is_domain", True),
                             lookup.eq("header", "x-qiniu-zone"),
                             lookup.ne("domain", "qiniudn.com")),
                  H, "主域不是取资产侧 domain"),
    "M076-S004": (lookup.AND(lookup.is_empty("title"),
                             lookup.eq("status_code", "200"),
                             lookup.not_empty("fid"),
                             lookup.not_empty("icon_hash"),
                             lookup.fz("domain", "admin"),
                             lookup.OR(lookup.pattern("domain", "*.cn"),
                                       lookup.pattern("domain", "????.com"))),
                  H, "标题为空用 title=\"\"，两个根域形态用通配 OR"),
    "M077-S009": (lookup.eq("body", 'class="container"'),
                  H, "双引号由 lookup.escape 转义"),
    "M078-S009": (lookup.eq("body",
                            '"exception": "Symfony\\Component\\ErrorHandler\\Error\\FatalError"'),
                  H, "保留双引号与反斜杠，由 lookup.escape 转义"),
    "M079-S006": (lookup.eq("ip", lookup.b_segment("40.88.14.254")),
                  H, "整个 B 段即 /16"),
    "M080-S006": (lookup.AND(lookup.ip_range_to_or("47.96.0.1", "47.96.0.5"),
                             lookup.OR(lookup.eq("port", "80"),
                                       lookup.eq("port", "443"))),
                  H, "闭区间穷举 + 端口任一"),

    # ---- M081 ~ M089 之前的七条跨引擎转换 --------------------------------
    "M083-S008": (lookup.AND(lookup.eq("header", "Server: nginx"),
                             lookup.eq("title", "Welcome")),
                  H, "Shodan 的 http.title 对应 FOFA title；裸关键词按 \"Key: Value\" 归一为 header"),
    "M084-S004": (lookup.AND(lookup.eq("title", "Grafana"),
                             lookup.eq("body", "JavaScript")),
                  H, "intitle→title、intext→body"),
    "M085-S001": (lookup.AND(lookup.eq("body", "login"),
                             lookup.OR(lookup.pattern("domain", "*bet*"),
                                       lookup.pattern("domain", "*casino*"),
                                       lookup.pattern("domain", "*kasino*"))),
                  H, "urlscan 的 domain 是可注册主域，对应 FOFA domain 模糊匹配；body 不支持 *="),
    "M086-S006": (lookup.AND(lookup.pattern("domain", "*google*"),
                             lookup.ne("domain", "google.com"),
                             lookup.is_empty("title")),
                  H, "FOFA 无 domain.suffix，排除后缀改用 domain!=；web.title 为空用 title=\"\""),
    "M087-S009": (lookup.OR(lookup.eq("header", "Server: Tengine"),
                            lookup.eq("header", "X-Cache: MISS")),
                  M, "header 不支持 *=，只能用 \"Key: Value\" 子串匹配；HTTP 头名大小写是否归一需抽查"),
    "M088-S004": (lookup.AND(lookup.ne("asn", "4134"),
                             lookup.eq("country", "HK"),
                             lookup.exact("server", "Apache")),
                  H, "全角引号归一；香港属国家级地区应取 country=\"HK\" 而非 region=\"HK\"；server 精确匹配用 =="),
    "M089-S010": (lookup.OR(lookup.AND(lookup.eq("app", "HFS"),
                                       lookup.eq("port", "80")),
                            lookup.AND(lookup.eq("app", "HFS"),
                                       lookup.eq("port", "8080"),
                                       lookup.eq("status_code", "200"))),
                  H, "全角引号与全角竖线归一；两组条件各自加括号保证 || 优先级"),
}


# ---------------------------------------------------------------------------
# 结构复杂的条目：在映射表之外拼装后并入，保持表格可读
# ---------------------------------------------------------------------------

# M081：header_hash / body_hash / icon_hash / fid 四项中至少两项非空
M081_PAIRS = [
    ("header_hash", "body_hash"),
    ("header_hash", "icon_hash"),
    ("header_hash", "fid"),
    ("body_hash", "icon_hash"),
    ("body_hash", "fid"),
    ("icon_hash", "fid"),
]
A["M081-S011"] = (
    lookup.AND(lookup.eq("app", "Grafana"),
               lookup.OR(*[lookup.AND(lookup.not_empty(p[0]), lookup.not_empty(p[1]))
                           for p in M081_PAIRS])),
    H,
    "四项两两组合共 6 组 OR，表达“至少两项有值”",
)

# M082：六个主域在“资产主域或证书域名”命中，证书需匹配、有效且过期时间在区间内
_m082_terms = []
for _d in M082_DOMAINS:
    _m082_terms.append(lookup.eq("domain", _d))
    _m082_terms.append(lookup.eq("cert.domain", _d))
A["M082-S001"] = (
    lookup.AND(lookup.flag("cert.is_match", True),
               lookup.flag("cert.is_valid", True),
               lookup.eq("cert.not_after.after", "2024-01-01 00:00:00"),
               lookup.eq("cert.not_after.before", "2026-08-30 18:17:49"),
               lookup.OR(*_m082_terms)),
    H,
    "证书被标记为匹配且有效，过期时间晚于 2024-01-01 00:00:00 且早于 2026-08-30 18:17:49",
)

# M090：七个响应头只要求字段名存在；Content-Length 与 Etag 保留材料原值
_m090_terms = [lookup.eq("header", _n) for _n in M090_NAME_ONLY]
_m090_terms.append(lookup.eq("header", "Content-Length: 1171"))
_m090_terms.append(lookup.eq("header", 'Etag: "6aa522a3-493"'))
A["M090-S008"] = (lookup.AND(*_m090_terms), H,
                  "状态码不限定；Etag 的引号由 lookup.escape 转义")

# M096：Jenkins 官方文档 TCP Agent Listener Port 一节示例响应中的字段名
M096_FIELDS = [
    "Jenkins-Agent-Protocols",
    "Jenkins-Version",
    "Jenkins-Session",
    "Remoting-Minimum-Version",
]
_m096_terms = []
for _f in M096_FIELDS:
    _m096_terms.append(lookup.eq("body", _f))
    _m096_terms.append(lookup.eq("banner", _f))
A["M096-S001"] = (lookup.OR(*_m096_terms), H,
                  "以产品名（Jenkins）或 Remoting 开头的四个字段，正文或响应原文任一命中")


# ---------------------------------------------------------------------------
# 其余条目
# ---------------------------------------------------------------------------

A["M097-S001"] = (lookup.eq("icon_hash", "-1493334710"), H,
                  "murmur3_32(base64.encodebytes(favicon))，已用 fofa.info 的文档向量核对")

A["M098-S001"] = (lookup.flag("is_fraud", True), L,
                  "“仿冒特征”只能落到 FOFA 的 is_fraud 标签；若要按同一注册局进一步收窄需人工确认")

A["M099-S001"] = (lookup.AND(lookup.eq("app", "Apache"),
                             lookup.eq("product.version", "2.4.49")),
                  M,
                  "CVE-2021-41773 对应 Apache HTTP Server；app 指纹名与是否叠加 2.4.50 待核")

A["M100-S001"] = (lookup.AND(lookup.eq("cert.issuer.cn", "ZeroSSL ECC DV SSL CA 2"),
                             lookup.eq("cert.sn", lookup.hex_to_dec(SERIAL_HEX))),
                  H,
                  "证书序号按 cert.sn 要求换算为十进制，运行时实算不自带常量")

# 不可转换的五题：端口越界、IP 非法、条件自相矛盾、无可度量安全指标、缺容器运行时数据
for _qid, _why in [
    ("M091-S009", "端口 1000000 超出 TCP/UDP 合法范围 1-65535，FOFA 无该端口数据"),
    ("M092-S007", "1.2.3.4.5 不是合法 IPv4，无法解析成地址或网段"),
    ("M093-S004", "同一记录要求 country=\"US\" 且 country!=\"US\"，逻辑自相矛盾"),
    ("M094-S004", "“最安全”没有可量化、可在 FOFA 字段上表达的标准"),
    ("M095-S010", "容器实时内存使用率需要运行时访问权限，FOFA 只有静态资产指纹"),
]:
    A[_qid] = (lookup.FIXED_ANSWER, H, _why)


# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------

def load_questions(path):
    """读取题目 JSON，返回 [(题号, 自然语言输入), ...] 以及题号顺序列表。"""
    with open(path, "rb") as fh:
        raw = fh.read()
    if raw.startswith(b"\xef\xbb\xbf"):
        raw = raw[3:]
    data = json.loads(raw.decode("utf-8"))
    items = []
    for row in data:
        items.append((row["题号"], row["自然语言输入"]))
    return items, [q for q, _ in items]


def build_answer_document():
    """按 docs/README.md 的格式生成答卷对象。"""
    return {
        "选手名称": PLAYER,
        "参赛包编号": PKG,
        "答案": [{"题号": qid, "查询语句": A[qid][0]} for qid in QUESTION_ORDER],
    }


def write_review(path, items):
    """把待确认清单写成 Markdown，供人工迭代。"""
    lines = [
        "# 待确认清单",
        "",
        "由 `python3 skill/scripts/build_answers.py` 自动生成，请勿手改；",
        "修改 `skill/scripts/build_answers.py` 里的映射后重新生成即可。",
        "",
        "置信度说明：`high` 规则直接对应；`medium` 取值依赖外部知识需抽查；"
        "`low` 需求含糊，答案是一次权衡。",
        "",
        "## 中低置信度条目",
        "",
        "| 题号 | 置信度 | 查询语句 | 说明 |",
        "| --- | --- | --- | --- |",
    ]
    low = [(q, v) for q, v in items if v[1] in (L, M)]
    for qid, value in low:
        query, conf, note = value
        shown = query.replace("|", "\\|")
        if len(shown) > 160:
            shown = shown[:157] + "..."
        lines.append("| %s | %s | `%s` | %s |" % (qid, conf, shown, note))
    lines += [
        "",
        "## 统计",
        "",
        "- 题目总数：%d" % len(QUESTION_ORDER),
        "- 可转换：%d" % sum(1 for q in QUESTION_ORDER if A[q][0] != lookup.FIXED_ANSWER),
        "- 固定答案：%d" % sum(1 for q in QUESTION_ORDER if A[q][0] == lookup.FIXED_ANSWER),
        "- 中低置信度：%d" % len(low),
        "",
    ]
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
    return len(low)


def main(argv=None):
    global QUESTION_ORDER

    parser = argparse.ArgumentParser(description="生成 FOFA 答卷与待确认清单")
    parser.add_argument("--题目", default=paths.questions_path(),
                        help="题目 JSON 路径（默认工作区 docs/题目.txt，FOFA_WORKSPACE 可改）")
    parser.add_argument("--输出", default=os.path.join(paths.answers_dir(), "答案.json"),
                        help="答卷输出路径")
    parser.add_argument("--golden", default=os.path.join(paths.tests_dir(), "golden.json"),
                        help="golden 输出路径")
    parser.add_argument("--review", default=os.path.join(paths.tests_dir(), "review.md"),
                        help="待确认清单输出路径")
    args = parser.parse_args(argv)

    items, QUESTION_ORDER = load_questions(args.题目)
    expected = set(QUESTION_ORDER)
    got = set(A)
    if expected != got:
        for qid in sorted(expected - got):
            print("缺少映射: " + qid)
        for qid in sorted(got - expected):
            print("多余映射: " + qid)
        print("映射表与题目不一致，已终止")
        return 1

    # 每条查询都要过一遍语法体检，FIX 串除外
    failures = 0
    for qid in QUESTION_ORDER:
        query = A[qid][0]
        if query == lookup.FIXED_ANSWER:
            continue
        issues = lookup.lint(query)
        if issues:
            failures += 1
            print("[lint] %s %s -> %s" % (qid, query, "; ".join(issues)))
    if failures:
        print("%d 条查询未通过语法体检，已终止" % failures)
        return 1

    doc = build_answer_document()
    os.makedirs(os.path.dirname(args.输出), exist_ok=True)
    os.makedirs(os.path.dirname(args.golden), exist_ok=True)
    os.makedirs(os.path.dirname(args.review), exist_ok=True)
    with open(args.输出, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    with open(args.golden, "w", encoding="utf-8") as fh:
        json.dump({qid: {"查询语句": A[qid][0], "置信度": A[qid][1], "说明": A[qid][2]}
                   for qid in QUESTION_ORDER}, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    low_count = write_review(args.review, [(q, A[q]) for q in QUESTION_ORDER])

    total = len(QUESTION_ORDER)
    converted = sum(1 for q in QUESTION_ORDER if A[q][0] != lookup.FIXED_ANSWER)
    print("题目 %d 道，可转换 %d 条，固定答案 %d 条，待确认 %d 条" %
          (total, converted, total - converted, low_count))
    print("答卷已写入: " + args.输出)
    print("golden 已写入: " + args.golden)
    print("待确认清单已写入: " + args.review)
    return 0


QUESTION_ORDER = []


if __name__ == "__main__":
    sys.exit(main())
