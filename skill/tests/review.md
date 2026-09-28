# 待确认清单

由 `python3 skill/scripts/build_answers.py` 自动生成，请勿手改；
修改 `skill/scripts/build_answers.py` 里的映射后重新生成即可。

置信度说明：`high` 规则直接对应；`medium` 取值依赖外部知识需抽查；`low` 需求含糊，答案是一次权衡。

## 中低置信度条目

| 题号 | 置信度 | 查询语句 | 说明 |
| --- | --- | --- | --- |
| M007-S007 | medium | `(country="KR" && region="Gangwon")` | 江原道英文名取 Gangwon；region 不支持 *=，= 的包含语义可兜底 Gangwon-do 一类写法 |
| M008-S009 | medium | `(country="DE" && region="Bavaria" && city="Munich")` | München -> Munich，Bavaria -> Bayern，均为英文地名待核 |
| M011-S005 | medium | `(region="Hebei" && cloud_name="Huawei Cloud")` | 华为云的 cloud_name 取值待核；region 不支持 *=，= 已是包含匹配 |
| M015-S001 | medium | `protocol="socks5"` | SOCKS v5 在 FOFA 里的 protocol 取值待核 |
| M019-S010 | medium | `(country="US" && protocol="memcached")` | memcached 的 protocol 取值待核 |
| M022-S007 | medium | `(country="JP" && app="HIKVISION")` | 海康威视的 app 指纹名待核 |
| M023-S002 | medium | `(domain="163.com" && host*="?.?.163.com")` | host 通配 ? 匹配单字符，构成两级单字符子域 |
| M025-S010 | low | `category="Middleware"` | 中间层软件的 category 英文取值为猜测，需实测核对 |
| M028-S011 | medium | `status_code="308"` | “网站类资产”是否要补 type="subdomain" 待核 |
| M029-S010 | medium | `banner="SSH-2.0-OpenSSH_6.1"` | “响应信息”取 banner，未叠加 protocol |
| M030-S003 | medium | `header="Set-Cookie:PHPSESSID"` | 题目原文无空格；真实 PHP 会话头多为 "Set-Cookie: PHPSESSID="，必要时改拆成两项 AND |
| M032-S010 | medium | `(country="NL" && city="Amsterdam" && port="554" && banner="200 OK")` | 阿姆斯特丹的英文城市名待核 |
| M034-S007 | medium | `(is_domain="true" && cert.subject.cn="apple.com" && domain!="apple.com")` | “域名数据”补 is_domain=true |
| M038-S009 | medium | `cert.domain="adobe.com"` | “根域名”落到 cert.domain |
| M040-S011 | medium | `(cert="godaddy" && domain!="godaddy.com")` | “根域名不是”取资产侧 domain，而非 cert.domain |
| M042-S003 | medium | `(cert.subject.cn="example.com" \|\| cert.domain="example.com")` | 裸域与子域由 = 的包含语义覆盖 |
| M045-S001 | medium | `os="Linux"` | os 大小写（Linux / linux）待核 |
| M046-S013 | medium | `(country="CN" && type="subdomain" && is_domain="false")` | “网站类资产”补 type="subdomain" |
| M053-S009 | medium | `(city="Xiamen" && port="80" && status_code="200")` | 厦门的英文城市名待核 |
| M054-S009 | medium | `(country="GB" && org="DigitalOcean" && body="/api/show")` | “DigitalOcean 上”取 org，也可试 cloud_name |
| M066-S006 | low | `(category="Mail" && title="Mail" && country!="US")` | “邮件系统”对应的 category 英文取值为猜测 |
| M070-S003 | medium | `(app="HFS" && status_code="200" && title=="HFS /" && country!="JP" && cloud_name!="Amazon AWS")` | “亚马逊云”取 cloud_name，未用 is_cloud 以免误伤其他云 |
| M072-S008 | medium | `(domain*="*.edu.ar" && (port="80" \|\| port="443") && server!="nginx" && server!="WAF")` | “服务是 Nginx 和 WAF 的不要”取 server 负向匹配 |
| M074-S008 | medium | `(cert.domain="aliyun.com" && cert.is_valid="true" && is_domain="false")` | “根域 aliyun.com”取 cert.domain；未绑定域名取 is_domain=false |
| M087-S009 | medium | `(header="Server: Tengine" \|\| header="X-Cache: MISS")` | header 不支持 *=，只能用 "Key: Value" 子串匹配；HTTP 头名大小写是否归一需抽查 |
| M098-S001 | low | `is_fraud="true"` | “仿冒特征”只能落到 FOFA 的 is_fraud 标签；若要按同一注册局进一步收窄需人工确认 |
| M099-S001 | medium | `(app="Apache" && product.version="2.4.49")` | CVE-2021-41773 对应 Apache HTTP Server；app 指纹名与是否叠加 2.4.50 待核 |

## 统计

- 题目总数：100
- 可转换：95
- 固定答案：5
- 中低置信度：27
