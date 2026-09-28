# 自然语言到字段的映射判据

第二步（模型判据）的对照表。左列是需求里的常见说法，右列是 FOFA 侧落点。
**先在这一步决定整个需求表达不表达得了，再决定每个说法用哪个字段。**

## 地址与网络

| 自然语言 | 落点 |
| --- | --- |
| IP 地址为 X | `ip="X"` |
| X/24 网段、C 段、B 段 | `ip="X/24"`（B 段按 /16 展开）|
| IP 从 A 到 B 的闭区间 | 逐 IP 用 `\|\|` 穷举，量太大就判不可转换 |
| 主域、根域、域名 | `domain="x.com"` |
| 主机名、访问地址 | `host="www.x.com"` |
| 前面恰好两级子域，每级一个字符 | `host*="?.?.163.com"` |
| 以 .cn 结尾的域名、任意后缀 | `domain*="*.cn"`、`domain*="????.com"` |

## 服务与响应

| 自然语言 | 落点 |
| --- | --- |
| 开放 N 端口 | `port="N"` |
| 开放 80 或 443 | `(port="80" \|\| port="443")` |
| SSH / FTP / SNMP / SOCKS5 等服务 | `protocol="ssh"` 等，取值需实测核对 |
| 真正跑在 TCP / UDP 上 | `base_protocol="tcp"`、`base_protocol="udp"` |
| 响应信息、Banner、返回的文本 | `banner="..."` |
| 网页标题、标题完整为 | `title="..."` / `title=="..."` |
| 响应头含某键值 | `header="Key: Value"` |
| 响应码、状态码 | `status_code="200"` |
| 正文、HTML、页面内容 | `body="..."` |
| 某某产品 / 版本号 | `product="X"` / `product.version="X"` |
| 具体应用指纹 | `app="GitLab"` 等，指纹名需实测核对 |

## 位置与归属

| 自然语言 | 落点 |
| --- | --- |
| 国家、国家代码 | `country="CN"` 用两位码 |
| 省、州、地区 | `region="Hebei"` |
| 城市 | `city="Xiamen"` |
| 运营商 / 组织 | `org="..."` |
| AS 号 | `asn="4134"` |
| 云厂商 | `cloud_name="Amazon AWS"` |
| 是否云资产 | `is_cloud="true"` |
| 操作系统 | `os="Linux"` |
| 网站类 / 服务类资产 | `type="subdomain"` / `type="service"` |
| 是否绑定域名 | `is_domain="true"` / `is_domain="false"` |

## 证书

| 自然语言 | 落点 |
| --- | --- |
| 证书包含某域名 | `cert.domain="x.com"` |
| 证书持有者、CN | `cert.subject.cn="x.com"` |
| 证书颁发者 | `cert.issuer.cn="..."` |
| 证书全文 | `cert="..."` |
| 证书有效 / 已失效 | `cert.is_valid="true"` / `cert.is_expired="true"` |
| 证书与域名匹配 | `cert.is_match="true"` |
| 签发者与持有者相同 | `cert.is_equal="true"` |
| 证书到期时间 | `cert.not_after.before/after="..."`（见 `sources.md` 的暗病说明）|
| 证书序列号 | `cert.sn="十进制"`（题目给十六进制要先换算）|

## 时与会话

| 自然语言 | 落点 |
| --- | --- |
| 更新时间在 X 之后 | `after="2026-08-01"` |
| 更新时间在 X 之前 | `before="..."` |
| 更新于某个区间 | `after="..." && before="..."` |

## 三个反复出错的判断点

**1. 来源平台不是检索目标。** "把 urlscan.io 搜索意图转换成 FOFA：HTML 包含
login"里的 urlscan.io 只是出处，答案应只写 `body="login" && ...`，绝不能把
urlscan.io 本身变成 `domain=`。凡出现 Shodan / Google Dork / Censys / Hunter /
urlscan 这些平台名，先确认它是来源还是目标。

**2. "不要 X" 用 `!=` 还是补一层限定？** `server!="nginx"` 会连带排除空值。
若需求是"没说是 Nginx 和 WAF 的都不要"，用两个 `!=` 是合理的（M072）；若需求
是"排除某一个具体值且必须保留其余"，`!=` 正是所需。

**3. 模糊域名要分清 `domain` 和 `host`。** `domain*="*.io"` 匹配主域后缀，
`host*="?.?.163.com"` 匹配主机名的层级结构。两者不可互换。

## 拿去 `lint` 之前先自查

- 字段是否支持你要用的运算符？不支持就换语义等效的写法（如 `=` 代替 `*=`）。
- 同时有 `&&` 和 `||` 时，`||` 那组是否已加括号？
- 同一字段出现多个不同取值，是 AND 还是 OR？需求没说清就不要猜。
- 值里的引号和反斜杠是否已通过 `lookup.escape()` 转义？
