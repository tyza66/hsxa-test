# FOFA 字段速查表

本文件由 `python3 skill/scripts/gen_field_doc.py` 从 `rules/3.txt` 解析生成，**请勿手改**。

末三列是 `rules/3.txt` 字段表原样的运算符支持情况：

- `=` 相等匹配。`字段=""` 表示该字段不存在或值为空。
- `==` 完全匹配。`字段==""` 表示该字段存在。
- `!=` 不匹配。`字段!=""` 表示该字段值不为空。
- `*=` 模糊匹配，用 `*` 匹配任意串、`?` 匹配单字符。

下表按 3.txt 的小节组织；字段名后的运算符取自三列交叉核对结果。

## General

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `ip` | = != | `ip="1.1.1.1"` | Query by a IPv4 address. |
| `&nbsp;&nbsp;↳` |  | `ip="220.181.111.1/24"` | Query by IPv4 C segment. |
| `&nbsp;&nbsp;↳` |  | `ip="2600:9000:202a:2600:18:4ab7:f600:93a1"` | Query by a IPv6 address. |
| `port` | = != *= | `port="6379"` | Query by open port number. |
| `domain` | = != *= | `domain="github.com"` | Query by domain. |
| `host` | = != *= | `host="s3.amazonaws.com"` | Query by hostname. |
| `os` | = != *= | `os="centos"` | Query by operating system. |
| `server` | = != *= | `server="Microsoft-IIS/10"` | Query by server. |
| `asn` | = != *= | `asn="19551"` | Query by autonomous system number. |
| `org` | = != *= | `org="LLC Baxet"` | Query by affiliated organization. |
| `is_domain` | = | `is_domain=true` | Filter assets that own a domain. |
| `&nbsp;&nbsp;↳` |  | `is_domain=false` | Filter assets that do not own a domain. |
| `is_ipv6` | = | `is_ipv6=true` | Filter assets that are IPv6. |
| `&nbsp;&nbsp;↳` |  | `is_ipv6=false` | Filter assets that are IPv4. |

## Special Label

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `app` | = | `app="Microsoft-Exchange"` | Query by fingerprint organized by FOFA. |
| `fid` | = != | `fid="sSXXGNUO2FefBTcCLIT/2Q=="` | Query by FeatureID aggregated by FOFA. |
| `product` | = != | `product="NGINX"` | Query by product names tagged by FOFA. |
| `product.version` | = != | `product="Roundcube-Webmail" && product.version="1.6.10"` | Query by product version tagged by FOFA. |
| `category` | = != | `category="Service"` | Query by categories marked by FOFA |
| `type` | = | `type="service"` | Filter protocol assets. |
| `&nbsp;&nbsp;↳` |  | `type="subdomain"` | Filter website type assets. |
| `cloud_name` | = != *= | `cloud_name="Amazon AWS"` | Query by cloud service provider. |
| `is_cloud` | = | `is_cloud=true` | Filter assets that are cloud services. |
| `&nbsp;&nbsp;↳` |  | `is_cloud=false` | Filter assets that are not cloud services. |
| `is_fraud` | = | `is_fraud=true` | Filter assets that are phishing spam site. (Professional and above) |
| `&nbsp;&nbsp;↳` |  | `is_fraud=false` | Filter assets that are not phishing spam sites (default filtered) |
| `is_honeypot` | = | `is_honeypot=true` | Filter assets that are honeypots. (Professional and above) |
| `&nbsp;&nbsp;↳` |  | `is_honeypot=false` | Filter assets that are not honeypots (default filtered). |

## None Web (type=service)

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `protocol` | = != *= | `protocol="quic"` | Query by protocol name. |
| `banner` | = != | `banner="users"` | Query by protocol response information. |
| `banner_hash` | = != | `banner_hash="7330105010150477363"` | Query by hash value calculated from protocol response banner. (Personal and above) |
| `banner_fid` | = != | `banner_fid="zRpqmn0FXQRjZpH8MjMX55zpMy9SgsW8"` | Query by hash value calculated from protocol response's structure. (Personal and above） |
| `base_protocol` | = != | `base_protocol="udp"` | Query assets with UDP as the transport layer protocol. |
| `&nbsp;&nbsp;↳` |  | `base_protocol="tcp"` | Query assets with TCP as the transport layer protocol. |

## Website (type=subdomain)

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `title` | = != *= | `title="New York"` | Query by website title. |
| `header` | = != | `header="elastic"` | Query by response headers. |
| `header_hash` | = != *= | `header_hash="1258854265"` | Query by hash value calculated from http/https response headers. (Personal and above) |
| `body` | = != | `body="google"` | Query by HTML body. |
| `body_hash` | = != | `body_hash="-2090962452"` | Query by hash value calculated from HTML body. |
| `js_name` | = != *= | `js_name="js/jquery.js"` | Query by JS included in HTML body. |
| `js_md5` | = != *= | `js_md5="82ac3f14327a8b7ba49baa208d4eaa15"` | Query by JS_md5. |
| `cname` | = != *= | `cname="customers.spektrix.com"` | Query by CName. |
| `cname_domain` | = != *= | `cname_domain="siteforce.com"` | Query by main domain resolved from CName. |
| `icon_hash` | = != | `icon_hash="-247388890"` | Query by hash value of favicon. |
| `status_code` | = != | `status_code="402"` | Filter services with a status code of 402 (website assets) |
| `sdk_hash` | = != | `sdk_hash="Are3qNnP2Eqn7q5kAoUO3l+w3mgVIytO"` | Query by hash value calculated from third-party code embedded in the website (Business and above) |

## Location

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `country` | = != | `country="US"` | Query by country code. |
| `region` | = != | `region="New York"` | Query by province/region name. |
| `city` | = != | `city="Manhattan"` | Query by city name. |

## Certificate

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `cert` | = != | `cert="baidu"` | Query by certificate. |
| `cert.subject` | = != *= | `cert.subject.cn="baidu.com"` | Query by certificate subject. |
| `cert.issuer` | = != *= | `cert.issuer="DigiCert"` | Query by certificate issuer. |
| `cert.subject.org` | = != *= | `cert.subject.org="Oracle Corporation"` | Query by certificate subject's organization. |
| `cert.subject.cn` | = != *= | `cert.subject.cn="baidu.com"` | Query by certificate subject's common name. |
| `cert.issuer.org` | = != *= | `cert.issuer.org="cPanel, Inc."` | Query by certificate issuer's organization. |
| `cert.issuer.cn` | = != *= | `cert.issuer.cn="Synology Inc. CA"` | Query by certificate issuer's common name. |
| `cert.domain` | = != *= | `cert.domain="amazon.com"` | Query by certificate subject's domain. |
| `cert.is_equal` | = | `cert.is_equal=true` | Filter assets where certificate issuer matches certificate subject. (Personal and above) |
| `&nbsp;&nbsp;↳` |  | `cert.is_equal=false` | Filter assets where certificate issuer does not matches certificate subject. (Personal and above) |
| `cert.is_valid` | = | `cert.is_valid=true` | Filter assets that have valid certificates. (Personal and above) |
| `&nbsp;&nbsp;↳` |  | `cert.is_valid=false` | Filter assets that have invalid certificates. (Personal and above) |
| `cert.is_match` | = | `cert.is_match=true` | Filter assets where certificate matches the domain. (Personal and above) |
| `&nbsp;&nbsp;↳` |  | `cert.is_match=false` | Filter assets where certificate does not match the domain. (Personal and above) |
| `cert.is_expired` | = | `cert.is_expired=true` | Filter assets with expired certificates. (Personal and above) |
| `&nbsp;&nbsp;↳` |  | `cert.is_expired=false` | Filter assets with non-expired certificates. (Personal and above) |
| `jarm` | = != *= | `jarm="2ad2ad0002ad2ad22c2ad2ad2ad2ad2eac92ec34bcc0cf7520e97547f83e81"` | Query by JARM fingerprint. |
| `tls.version` | = != | `tls.version="TLS 1.3"` | Query by tls protocol version, |
| `tls.ja3s` | = != *= | `tls.ja3s="15af977ce25de452b96affa2addb1036"` | Query by tls ja3s fingerprint. |
| `cert.sn` | = != | `cert.sn="356078156165546797850343536942784588840297"` | Query by certificate Serial Number. |
| `cert.not_after.after` | = | `cert.not_after.after="2025-03-01"` | Filter certificates where the validity start date (not_before) is after. |
| `cert.not_after.before` | = | `cert.not_after.before="2025-03-01"` | Filter certificates where the validity start date (not_before) is before. |
| `cert.not_before.after` | = | `cert.not_before.after="2025-03-01"` | Filter certificates where the validity end date (not_after) is after. |
| `cert.not_before.before` | = | `cert.not_before.before="2025-03-01"` | Filter certificates where the validity end date (not_after) is before. |

## Last Update Time

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `after` | = | `after="2023-01-01"` | Filter assets that have been updated after a time. |
| `before` | = | `before="2023-12-01"` | Filter assets that have been updated before a time. |

## Unique IP View (These unique IP series syntax cannot be used together with the other syntax mentioned above.)

| 字段 | 支持运算符 | 示例 | 说明 |
| --- | --- | --- | --- |
| `port_size` | = != | `port_size="6"` | Filter Unique IPs with exactly 6 open ports. (Personal and above) |
| `port_size_gt` | = | `port_size_gt="6"` | Filter Unique IPs with more than 6 open ports. (Personal and above) |
| `port_size_lt` | = | `port_size_lt="12"` | Filter Unique IPs with less than 12 open ports. (Personal and above) |
| `ip_ports` | = | `ip_ports="80,161"` | Filter Unique IPs that have different ports open. |
| `ip_country` | = | `ip_country="US"` | Query Unique IPs by country code. |
| `ip_region` | = | `ip_region="Zhejiang"` | Query Unique IPs by province/region name. |
| `ip_city` | = | `ip_city="Seattle"` | Query Unique IPs by city name. |
| `ip_after` | = | `ip_after="2021-03-18"` | Filter Unique IPs that have been updated after a time |
| `ip_before` | = | `ip_before="2019-09-09"` | Filter Unique IPs that have been updated before a time. |

## 对账

`rules/3.txt` 共解析出 **71** 个字段。
`lookup.FIELD_OPERATORS` 收录 **71** 个字段。

