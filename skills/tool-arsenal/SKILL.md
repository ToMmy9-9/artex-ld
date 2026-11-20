---
name: tool-arsenal
description: "外部工具库选型与调用指南。当需要批量资产侦察、子域与端口枚举、目录与参数爆破、指纹与 WAF 判定、POC 模板批量扫描、注入类定向深挖（命令注入/SSTI/请求走私/CRLF）、后渗透与域渗透、内网隧道代理、JSON/URL 编解码批处理时，先查本表选工具再执行。⚠️ 边界：本 skill 只覆盖**批量、无状态、离线**的 CLI 工具；需要看回包细节的漏洞验证请求仍必须走 MCP（流量录制/拦截工具），禁止用 CLI 工具或 curl 替代 MCP 做请求测试。"
---

# 外部工具库（tool arsenal）

> 工具本体由管理员配置在平台的**工具目录**里，模型在 Bash 工具描述中能看到实时的目录清单。
> 本 skill 负责**选型与用法**：什么时候用哪个、怎么调、边界在哪。

## 一、先分清三层，别搞混

| 层 | 是什么 | 干什么 |
|---|---|---|
| **skill** | 方法论 / SOP | 怎么测、按什么顺序测 |
| **MCP / 流量录制** | 执行原语 | 发请求（可抓包 / 改包 / 观察拦截） |
| **工具目录** | CLI 原子工具 | **批量、无状态、离线**的活 |

**✅ 该用 CLI 工具**：子域/端口/目录枚举、URL 收集去重、指纹与 WAF 判定、POC 模板批量匹配、
字典与 payload 批处理、JSON/编解码、报告后处理、内网隧道。

**❌ 不该用 CLI 工具**：需要保持登录态的漏洞验证、需要看完整报文/改包的重放、
需要观察 WAF 拦截特征的试探 —— **这些必须走 MCP**。

> **一句话判据**：这一枪**要不要看回包细节**？
> 要（判断 payload 是否被拦、响应是否变形、是否需要改包）→ **MCP**
> 不要（只要一份结果列表）→ **CLI 工具**

---

## 二、快速选型表

| 我要干的事 | 用哪个 | 目录位置（以工具目录根为准） |
|---|---|---|
| 子域枚举（被动/主动） | `subfinder` / `ksubdomain` / `httpx` | `domain/SubDomain-brute/` |
| 历史 URL 收集 | `gau` / `katana -jc` | `domain/gau`、`domain/katana` |
| 批量探活 | `httprobe` / `httpx` | `util/httprobe`、`domain/SubDomain-brute/httpx` |
| TLS 证书挖域名 | `tlsx -san -cn` | `domain/tlsx` |
| ASN → IP 段 | `asnmap` / `mapcidr` | `domain/asnmap`、`domain/mapcidr` |
| 空间测绘（FOFA/Quake） | `uncover -e fofa` | `domain/uncover` |
| 端口扫描（快） | `naabu` / `gogo` | `portscan/` |
| 内网综合扫描 | `fscan` | `portscan/fscan` |
| 目录/文件爆破 | `feroxbuster` / `ffuf` | `dirscan/` |
| 参数发现（隐藏攻击面） | `x8` / `arjun` | `dirscan/x8`、`dirscan/arjun` |
| WAF 判定 | `cdncheck` → `wafw00f` | `domain/cdncheck`、`vulnscan/wafw00f` |
| POC 模板批量扫描 | `nuclei` + templates | `vulnscan/nuclei` |
| CVE / 组件漏洞检索 | `cvemap` | `vulnscan/cvemap` |
| XSS 批量检测 | `dalfox` | `vulnscan/dalfox` |
| 密钥泄露（源码/仓库） | `gitleaks` | `vulnscan/gitleaks` |
| 盲注 OOB 回显 | `interactsh-client` | `vulnscan/interactsh-client` |
| GraphQL 指纹 | `graphw00f` | `vulnscan/graphw00f` |
| 命令注入 | `commix` | `inject/commix` |
| SSTI 模板注入 | `SSTImap` | `inject/SSTImap` |
| HTTP 请求走私 | `smuggler` | `inject/smuggler` |
| CRLF 注入 | `crlfuzz` | `inject/crlfuzz` |
| JS 文件地址批量提取 | `subjs` | `js/subjs` |
| Java 反序列化 payload | `ysoserial` | `exploit/ysoserial` |
| .NET 反序列化 payload | `ysoserial.net` | `exploit/ysoserial.net` |
| Shiro 利用 | `shiro_attack` | `exploit/shiro_attack` |
| Log4Shell 扫描 | `Log4j2Scan` | `exploit/Log4j2Scan` |
| Windows 提权（SeImpersonate） | `GodPotato` / `JuicyPotatoNG` / `PrintSpoofer` | `privesc/` |
| Windows 提权枚举 | `winPEASx64` | `privesc/PEASS-ng` |
| 主机信息收集 | `Seatbelt` | `dotnet/SharpCollection/` |
| 域数据采集（BloodHound） | `SharpHound` | `ad/SharpHound` |
| Kerberos 攻击 | `Rubeus` / `kerbrute` | `dotnet/`、`ad/kerbrute` |
| ADCS 证书滥用 | `Certify` / `Whisker` | `dotnet/` |
| LSASS 凭证 | `mimikatz` | `cred/mimikatz` |
| 本机软件明文密码 | `LaZagne` | `cred/LaZagne` |
| 内网隧道（首选） | `ligolo-ng` | `tunnel/ligolo-ng` |
| 内网隧道（兜底/HTTP） | `chisel` / `frp` / `gost` / `Stowaway` | `tunnel/`、`pivot/` |
| JSON 处理 | `jq` | `util/jq` |
| URL 去重 / 替换 / 拆解 | `anew` / `qsreplace` / `unfurl` | `util/` |
| JWT 测试 | `jwt_tool` | `util/jwt_tool` |

> 上表的路径是**相对工具目录根**的。工具目录根的绝对路径会出现在 Bash 工具描述里，
> 调用时拼成完整路径（例如 `D:\PTS\tools\dirscan\ffuf\ffuf.exe`）。

---

## 三、通用调用约定

1. **绝对路径调用**：工具不在 PATH 里，必须用工具目录根拼出的完整路径。
   Windows 下路径用反斜杠，调用时建议用 PowerShell 的调用运算符：
   `& 'D:\PTS\tools\util\jq\jq.exe' --version`
2. **注意实际 shell**：本平台的 Bash 工具在 Windows 上底层是 **PowerShell**
   （`powershell -NoProfile -NonInteractive -Command`），不是 bash。
   不要写 `ls -la`、`&&` 串联、`/dev/null` 这类 bash 语法；改用
   `Get-ChildItem`、`;`、`$null`。Linux 部署上才是真正的 bash。
3. **结果落文件再处理**：这类工具普遍输出一大堆行，先重定向到文件，再用
   `anew` 去重、`httprobe` 过滤存活，不要把原始输出灌进上下文。
4. **字典按技术栈选**：JSP / PHP / ASPX / Spring 各有专用字典，别拿通用字典硬打。
5. **速率控制**：批量扫描一律加 `-rl` / `-c` / `-t` 之类的限速参数，避免打挂目标或触发防护。
6. **工具自身失败不等于漏洞**：工具报错只说明工具没跑通，要换路径或换工具重试，
   不要直接下结论。

---

## 四、什么时候**不要**用这里的工具

- 需要判断响应内容是否被 WAF 改写 → 用 MCP 观察原始响应。
- 需要带登录态做越权差分 → 用 MCP / 会话工具，不要用 CLI 工具。
- 需要看请求是否被拦截、变形 → 用 MCP 拦截器。

工具目录是**广度工具**，MCP 是**深度工具**。先用广度铺开攻击面，再用深度验证，
这是最省 token 的顺序。
