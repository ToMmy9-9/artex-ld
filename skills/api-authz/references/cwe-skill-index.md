# CWE → 技能索引

给规划者与 worker 用的一张对照表：看到某类漏洞证据时，应该调用哪个方法论技能。
`vulnclass` 是自由文本，所以这里用关键词与 CWE 双通道匹配——两者任一命中即可。

## 访问控制类（最高频）

| CWE | 名称 | 技能 | 关键判据 |
| --- | --- | --- | --- |
| CWE-284 | Improper Access Control | `api-authz` | 用他人凭据或匿名拿到了非本人资源 |
| CWE-639 | Authorization Bypass Through User-Controlled Key (IDOR) | `idor-testing`、`api-authz` | 替换 id 后仍返回他人数据 |
| CWE-862 | Missing Authorization | `api-authz` | 匿名直接 200 + 业务数据 |
| CWE-863 | Incorrect Authorization | `api-authz`、`route-guard-bypass` | 有鉴权但判定错误（角色/租户不校验） |
| CWE-601 | URL Redirection to Untrusted Site | `api-authz`、`api-tester` | 跳转参数可被替换为外域 |

## 注入类

| CWE | 名称 | 技能 |
| --- | --- | --- |
| CWE-89 | SQL Injection | `sql-injection-testing` |
| CWE-79 | Cross-site Scripting | `xss-testing` |
| CWE-78 / CWE-77 | OS Command Injection | `command-injection-testing` |
| CWE-90 | LDAP Injection | `ldap-injection-testing` |
| CWE-91 | XML Injection / XPath | `xpath-injection-testing` |
| CWE-611 | XML External Entity (XXE) | `xxe-testing` |
| CWE-502 | Deserialization of Untrusted Data | `deserialization-testing` |
| CWE-918 | Server-Side Request Forgery (SSRF) | `ssrf-testing` |
| CWE-943 | Improper Neutralization in NoSQL | `sql-injection-testing`、`api-tester` |

## 文件与上传

| CWE | 名称 | 技能 |
| --- | --- | --- |
| CWE-434 | Unrestricted Upload of File with Dangerous Type | `file-upload-testing` |
| CWE-22 | Path Traversal | `file-upload-testing`、`sensitive-info-leak` |
| CWE-538 | File and Directory Information Exposure | `sensitive-info-leak` |

## 信息泄露与配置

| CWE | 名称 | 技能 |
| --- | --- | --- |
| CWE-200 | Exposure of Sensitive Information | `sensitive-info-leak` |
| CWE-532 | Insertion of Sensitive Information into Log | `sensitive-info-leak`、`incident-response` |
| CWE-16 / CWE-2 | Configuration | `vulnerability-assessment`、`cloud-security-audit` |
| CWE-1104 | Use of Unmaintained Third Party Components | `fingerprint-finder`、`vulnerability-assessment` |

## 会话与凭据

| CWE | 名称 | 技能 |
| --- | --- | --- |
| CWE-287 | Improper Authentication | `api-authz`、`route-guard-bypass` |
| CWE-384 | Session Fixation | `api-authz` |
| CWE-352 | Cross-Site Request Forgery | `csrf-testing` |
| CWE-798 | Use of Hard-coded Credentials | `sensitive-info-leak` |
| CWE-521 | Weak Password Requirements | `vulnerability-assessment` |

## 逻辑与业务

| CWE | 名称 | 技能 |
| --- | --- | --- |
| CWE-840 | Business Logic Errors | `business-logic-testing` |
| CWE-841 | Improper Enforcement of Behavioral Workflow | `business-logic-testing` |
| CWE-770 | Allocation of Resources Without Limits (DoS 风险) | `business-logic-testing`（注意：仅记录，不主动打 DoS） |

## 移动端 / 云 / 容器

| 范围 | 技能 |
| --- | --- |
| Android / iOS 客户端、越狱检测、本地存储 | `mobile-app-security-testing` |
| 云 IAM、对象存储、元数据服务 | `cloud-security-audit` |
| 容器逃逸、K8s RBAC、镜像 | `container-security-testing` |
| 内网横向、域渗透、隧道 | `network-penetration-testing` |

## 使用方法

1. 拿到 finding 的 `vulnclass` 或证据关键词，先在表中找 CWE；
2. 命中后调用对应技能（`Skill` 工具），按其方法论补测，避免凭记忆猜；
3. 表里没有的类别，退回 `vulnerability-assessment` 做通用评估，并在结论里写明"未匹配到专用方法论"。

> 这张表只解决"该用哪个方法"，不改变证据标准：结论仍必须满足对应技能的复现与证据要求。
