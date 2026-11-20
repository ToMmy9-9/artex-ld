# 可用技能模块

> 本文件是 SKILL.md 中"可用技能模块"一节的可执行展开。
> 本主编排器整合以下 10 个专项技能。每个 skill 都有独立 SKILL.md，可单独调用。

---

## 1. 🔍 指纹识别 (fingerprint-finder) ⚠️ 每次必须调用

- **功能**：识别目标网站的技术栈、框架、CMS、组件、**认证结构**（OAuth2/SSO/JWT）
- **适用场景**：
  - 识别 Web 框架（Spring Boot、ThinkPHP、Django 等）
  - 识别 CMS 系统（WordPress、DedeCMS、Discuz 等）
  - 识别前端技术（Vue、React、jQuery 等）
  - 识别服务器组件（nginx、Apache、Tomcat 等）
  - 深度探测二次开发系统的隐藏指纹
  - **识别自研 / 二开 / 通用三类系统的差异化入口**
  - **识别 JS 加解密 / OAuth2 / SSO 等认证侧指纹**
  - **识别 WAF 品牌（阿里云/玄武盾/腾讯云/Cloudflare）**
  - **子域名探测（主站加固时子域名往往裸奔）**
- **触发词**：
  - "识别指纹"
  - "这是什么框架"
  - "技术侦察"
  - "识别 CMS"
  - "深度指纹"
  - "识别认证结构"
  - "识别 WAF"
  - "探测子域名"

---

## 2. 🌐 API 测试 (api-tester) ⚠️ 每次必须调用

- **功能**：深度测试 API 接口，包括接口发现、fuzz 测试、凭据获取
- **适用场景**：
  - 测试 RESTful API 接口
  - 发现隐藏的 API 端点
  - JS 逆向分析获取 API 参数
  - 注册 + 登录获取测试凭据
  - 越权测试和逻辑漏洞测试
  - 绕过 WAF 的 fuzz 测试
- **触发词**：
  - "测试 API 接口"
  - "接口发现"
  - "API fuzz"
  - "获取凭据"
  - "越权测试"
  - "尝试注册"

---

## 3. 💉 SQL 注入 (sqli-tester)

- **功能**：检测和利用 SQL 注入漏洞
- **适用场景**：
  - 数字型 / 字符型注入检测
  - 布尔盲注和时间盲注
  - 报错注入和联合查询
  - WAF 绕过技术
  - 数据库信息提取
  - Order By 注入测试
- **触发词**：
  - "SQL 注入"
  - "注入测试"
  - "数据库注入"
  - "盲注测试"
  - "报错注入"

---

## 4. 🔄 响应篡改 (response-modifier)

- **功能**：拦截和修改 HTTP 响应包，配合 yakit MITM（`start_mitm_v2` + `set_current_rules`）实现响应篡改测试
- **适用场景**：
  - 登录状态伪造（将登录失败改为成功）
  - 注册验证绕过（绕过邮箱 / 手机号验证）
  - 短信验证码劫持（修改返回的手机号）
  - 权限绕过（绕过 403 权限验证）
  - 密码重置攻击（劫持他人密码重置流程）
- **触发词**：
  - "修改返回包"
  - "篡改响应"
  - "绕过登录"
  - "验证码劫持"
  - "密码重置"

---

## 5. 📚 POC 库匹配 (poc-scan) ⭐ 关键

- **功能**：从本地 POC 库（约 2.5 万文件，跨源未去重，已冻结）中匹配目标系统的已知漏洞，弥补 agent 内置 POC 不足
- **适用场景**：
  - 商业 OA 系统（用友 / 致远 / 泛微 / 通达 / 万户 / 金蝶 / 蓝凌 / JEPaaS 等）已知漏洞匹配
  - 二开框架（人人权限框架 / JeecgBoot / 若依 RuoYi / CAS 单点登录）历史 nday + Shiro 反序列化
  - 通用组件（Struts2 / Shiro / Weblogic / Spring / Solr / Log4j / Fastjson 等）CVE POC
  - 开源 CMS（WordPress / Drupal / Joomla / DedeCMS / Discuz 等）历史漏洞
  - 短链接系统 / 表单系统（手动测试无通用 POC → api-tester 联动）
  - 命中 POC 后通过 WAF 适配做绕过测试
- **支持的 POC 格式**：
  - nuclei YAML 模板（13,000+）
  - afrog YAML POC（1,700+）
  - Markdown POC（Awesome-POC 中文 + 商业 OA 专项，4,400+）
  - 本地语料：**约 2.5 万文件（跨源未去重，已冻结只读）**
- **核心能力**：
  - 6 维指纹：CVE / CNVD / product+type+endpoint+parameter / 标题标准化 / 端点 / 参数
  - WAF 适配：支持阿里云 / 玄武盾 / 腾讯云 WAF 绕过
  - ⚠️ **语料已冻结（2026-09）**：同步 / 去重 / 建索引管线已停用，`POC/` 为只读语料
- **使用方式**（直接 grep 语料，**不执行 `core/` 下任何脚本**）：
  ```bash
  rg -l -i --no-ignore -e "用友" -e "yongyou" POC/
  rg -l -i --no-ignore "CVE-2021-44228" POC/
  ```
- **触发词**：
  - "匹配 POC"
  - "POC 库检索"
  - "已知漏洞"
  - "商业系统漏洞"
  - "找 POC"

### ⭐ poc-scan 调度硬钩子（强制，不依赖模型判断）

`poc-scan` 的 description 触发是"软触发"，模型在某些 post-fingerprint 场景（用户已识别系统、用户给具体 CVE、用户描述方案）下会漏判。为消除 4 类已知 false negative，编排器**强制按以下步骤调用**，无论模型当前判断如何。

**触发条件（满足任一即执行硬钩子）**：

- `fingerprint-finder` 输出命中 **商业 OA**：用友 / 致远 / 泛微 / 通达 / 万户 / 金蝶 / 蓝凌 / JEPaaS
- `fingerprint-finder` 输出命中 **二开框架**：人人权限框架(renren-fast/security) / JeecgBoot / 若依 RuoYi / CAS 单点登录
- `fingerprint-finder` 输出命中 **通用组件**：Struts2 / Shiro / Weblogic / Solr / ActiveMQ / Fastjson / Spring / Log4j
- `fingerprint-finder` 输出命中 **开源 CMS**：WordPress / Discuz / DedeCMS / Drupal / Joomla
- 用户在 query 中直接给出 **CVE 编号**（独立复现场景）
- 识别出 **WAF**（阿里云 / 玄武盾 / 腾讯云 / Cloudflare / AWS WAF）

**二开框架关键词映射**：
| 系统 | 指纹关键词 | poc-scan 检索词 |
|---|---|---|
| 人人权限框架 | `renren`、`rememberMe`、`/admin`、`/generator` | `renren`、`shiro` |
| JeecgBoot | `jeecg-boot`、`jmreport`、`Ant Design Vue` | `jeecg` |
| 若依 RuoYi | `ry-ui`、`/login`+`/captchaImage` | `ruoyi`、`ruoyi` |
| JEPaaS 低代码 | `JEPaaS`、`j_spring_security_check` | `jepaas` |
| CAS 单点登录 | `CAS`、`serviceValidate`、`TGC` | `cas`、`nuclei-cves` |

**执行步骤（顺序执行，不可跳步）**：

1. **本地检索**：先读 `INDEX.md` 定位子目录，再用 `rg` 检索语料（**不执行 `core/` 下脚本**，管线已冻结）：
   ```bash
   rg -l -i --no-ignore -e "<系统名>" -e "<英文别名>" POC/
   ```
   把结果（命中数、命中 POC 路径列表）落库到 `work-space/<target>/findings/poc-search-result.md`
   - 关键词映射：用友 → `yongyou`；致远 → `seeyon`；泛微 → `weaver`；通达 → `tongda`；万户 → `wanhu`；金蝶 → `kingdee`；蓝凌 → `landray`
2. **命中判定**：`hits ≥ 1` → `Read` 命中文件正文确认适用版本/前提 → 进入第 4 步；`hits == 0` → 继续
3. **本地 0 命中 → 网上兜底**（按顺序尝试）：
   - **WebSearch**：`"<系统名> CVE 2024 2025"` 找最新漏洞披露
   - **WebSearch**：`"<系统名> POC github"` 找公开 POC 仓库
   - **WebSearch**：`"<系统名> exploit-db"` / `site:github.com "<系统名> 漏洞"`
   - **yakit mcp `query_cve`**：按系统名查 yakit 内置漏洞库
   - 命中信息 → 写入 `POC/custom/<系统名>-web-found-<日期>.md` + `work-space/<target>/findings/poc-search-result.md`（标注"网络补充"来源）
4. **落库完毕 → 跳到下游 skill**（sqli-tester / api-tester / response-modifier 视命中 POC 类型）

> ⚠️ **语料已冻结后互联网兜底权重上升**：本地语料不再有新 CVE 进入，第 3 步从"兜底"变成**主路径**，不得因为第 1 步命中就跳过。

**与 description 软触发的关系**：

- **软触发**：用户**直接**说"跑 nuclei / 试 afrog / 找 POC"等独立 query 时，description 让模型自主决定
- **硬钩子**：编排器流程**在 fingerprint 之后的下一步强制动作**，不依赖模型
- 两者**不冲突**：硬钩子覆盖 fingerprint 后的自动联动，软触发覆盖用户独立发起 POC 验证

**硬钩子豁免情况**：

- `fingerprint-finder` 输出"未识别"或"自研系统" → 跳过硬钩子，改走 `api-tester` 做接口发现
- 目标为纯 API / 纯前端 SPA，无业务系统特征 → 跳过硬钩子

---

## 6. 🔓 敏感信息泄露 (sensitive-info-leak) ⭐ 贯穿全程

- **功能**：从 JS / HTML / JSON / 配置文件中检测账号密码、身份证、手机号、邮箱、API 密钥、JWT 密钥、云服务密钥、地图 API 密钥等敏感信息
- **适用场景**：
  - **JS 文件分析**：从 JavaScript 代码中提取 API 密钥、数据库连接字符串、JWT Token
  - **前端代码审计**：发现硬编码的密码、Token、配置信息
  - **配置文件扫描**：检测 .env、JSON、YAML 等配置文件中的敏感数据
  - **个人信息识别**：提取身份证号、手机号、邮箱、姓名等
  - **密钥泄露检测**：发现 AWS 密钥、GitHub Token、地图 API 密钥（高德、百度、腾讯、Google Maps）
  - **Git 历史扫描**：从 Git 历史记录中挖掘历史泄露的敏感信息
- **综合利用率**：⭐⭐⭐⭐⭐（极高）
  - 获取的 API 密钥可提供给 [api-tester] 直接调用 API 接口
  - 获取的数据库连接字符串可用于 [api-tester] 直接连接数据库
  - 获取的 JWT 密钥可用于 [api-tester] 伪造任意用户 Token
  - 获取的凭据可提供给 [response-modifier] 进行登录绕过测试
  - 获取的管理员账号密码可用于 [sqli-tester] 进行高权限数据库操作
  - 识别的系统指纹可触发 [poc-scan] 匹配已知漏洞 POC

**poc-scan 调用场景（从敏感信息 / 指纹识别触发）**：

- fingerprint-finder 识别出"用友 NC" → [poc-scan] `--keyword "用友"` 匹配 300+ POC
- fingerprint-finder 识别出"Shiro" → [poc-scan] `--keyword "shiro"` 匹配反序列化 POC
- 识别出"Log4j" 关键词 → [poc-scan] `--cve "CVE-2021-44228"` 匹配 53 跨源 POC
- 识别出 WAF → [poc-scan → waf-adapt.md] 改造 POC payload

- **输出成果**：
  - 账号密码对（可用于登录测试）
  - API 密钥和 Token（可直连 API）
  - 数据库连接字符串（可连接数据库）
  - JWT 签名密钥（可伪造任意 Token）
  - 云服务密钥（可访问云资源）
  - 地图 API 密钥（可调用地图服务）
- **触发词**：
  - "敏感信息泄露"
  - "检测敏感信息"
  - "扫描 JS 文件"
  - "查找密钥"
  - "信息泄露检测"
  - "提取 API 密钥"
  - "扫描配置文件"

---

## 7. 📸 漏洞证据截图 (evidence-capture) ⭐ 报告阶段

- **功能**：专门处理"漏洞成立但截图拿不出"的疑难场景，覆盖 5 条证据路径：
  1. **前端 JS 复用解密**（首选）：用 js-reverse-mcp `evaluate_script` 调用前端解密函数
  2. **yakit MITM 飞行中解密**：`start_mitm_v2` + `set_current_rules` 全站解密
  3. **自实现解密**（拿到密钥后）：AES/RSA/SM4/XOR Python 解密模板
  4. **OOB 盲注证据**：`require_dnslog_domain` + DNSLog/HTTP 反连平台
  5. **长度/时间侧信道**：length/timing oracle 模板
- **配套能力**：
  - 通用证据拼装格式（多路径合并 + 注释 + 元信息）
  - 失败截图重做 SOP（PNG 渲染失败 / 文件过大 / 关键字段模糊）
  - 补证（单证据被质疑时加独立路径 / 完整攻击链展示）
  - 复杂场景方案（双层加密 / 字段级加密 / WS 加密 / 签名校验 / 混合架构）
- **适用场景**：
  - **加密响应**：AES/RSA/SM4 等对称或非对称加密的 API 响应（明文不可见）
  - **盲注无回显**：DNSLog/xss.pt/bxss/反连平台出网回调
  - **侧信道**：明文+密文都拿不到，只有长度/时间差能证明漏洞
  - **截图失败**：PNG 渲染异常、文件过大、关键字段看不清
  - **补证**：单一证据被审计质疑、缺完整攻击链
- **强制调用场景**：
  - Phase 7 报告生成阶段**所有漏洞出图必调**
  - 响应是 base64 / hex / 加密但漏洞已确认成立
  - 盲注无回显，需要 DNSLog 截图
  - 截图失败 / 模糊 / 缺要素，需重做或补证
- **综合利用率**：⭐⭐⭐⭐⭐
  - 上游 sqli-tester / api-tester / sensitive-info-leak / poc-scan / response-modifier 等所有漏洞验证 skill 出图时 → 全部交由本 skill 处理
  - 加密场景下，与 decryption-strategies + complex-scenarios 协同
  - 与 MCP `evidence-screenshot-mcp` `take_evidence` 强绑定（最终截图）
- **与 MCP 协同**：
  - `evidence-screenshot-mcp` `take_evidence`：最终截图工具（Burp 白底风格）
  - `yakit` `start_mitm_v2` + `set_current_rules`：MITM 飞行中解密 / OOB 反连平台 / DNSLog
  - `js-reverse-mcp` `evaluate_script`：复用前端 JS 解密函数
  - `memory-search-mcp` `memory_get_methodology`：检索历史"加密场景"打法
- **触发词**：
  - "出证据" / "截一张图" / "出图" / "报告截图"
  - "加密响应怎么证明" / "盲注怎么出图" / "OOB 证据"
  - "截图不清楚，重做" / "补一张"
  - "save evidence" / "capture proof"
- **输出规范**：截图必须落 `work-space/[目标]/evidence/` 目录，每张图附：
  - `EVD-{ID}-{type}.png`（截图本体）
  - `EVD-{ID}-source.txt`（原始报文）
  - `EVD-{ID}-decrypt.py`（解密脚本，如有）
  - `EVD-{ID}-notes.md`（路径选择理由）
  - `INDEX.md`（证据清单）

**与 description 触发对比**：

- 本 skill 的 description 触发词包含"出证据/截一张图/加密响应/盲注截图"——任何出报告场景都应激活
- 硬钩子由编排器在 Phase 7 强制激活（不依赖模型）
- 软触发覆盖用户独立发起的"加密响应怎么证明"等求助

---

## 8. 📚 POC 库匹配 (poc-scan) ⭐ 关键

- **功能**：从本地 POC 库（约 2.5 万文件，跨源未去重，已冻结）中匹配目标系统的已知漏洞
- **适用场景**：
  - 商业 OA 系统（用友/致远/泛微/通达/万户/金蝶/蓝凌/JEPaaS 等）
  - 二开框架（人人权限框架/JeecgBoot/若依 RuoYi/CAS 单点登录）
  - 通用组件（Struts2/Shiro/Weblogic/Spring/Solr/Log4j/Fastjson 等）
  - 开源 CMS（WordPress/Drupal/Joomla/DedeCMS/Discuz 等）
- **触发词**：
  - "匹配 POC"
  - "POC 库检索"
  - "已知漏洞"
  - "商业系统漏洞"
  - "找 POC"

---

## 8. 🖥️ 浏览器自动化 (browser-skill)

- **功能**：通过 `bsk` CLI 驱动用户的真实 Chromium 浏览器，支持通用模式和渗透模式
- **适用场景**：
  - 通用模式：页面浏览、数据抓取、表单填写、UI 回归验证
  - 渗透模式：强制 MITM 代理（127.0.0.1:8080）、流量捕获、API 端点提取、凭据注入
- **触发词**：
  - "打开浏览器"
  - "浏览页面"
  - "抓取数据"
  - "渗透测试浏览器"

---

## 9. 🧰 原子化工具库 (tool-arsenal) ⭐ 侦察/批量层

- **功能**：`tools/` 下 14 类原子化命令行工具的**选型表 + 命令速查**（单文件单用途、免安装、全部实测可运行）。
  覆盖资产侦察 / 端口 / 目录与参数爆破 / 指纹与 WAF 判定 / POC 模板批量扫描 /
  注入类定向深挖（命令注入·SSTI·请求走私·CRLF）/ 反序列化 / Windows 提权 / 域渗透 / 凭证 /
  隧道代理 / JSON·URL·编解码批处理。
- **⚠️ 边界（重要）**：本 skill 只覆盖**侦察与批量层**的 CLI 工具。
  **需要看回包细节的漏洞验证请求仍必须走 MCP**（yakit http_fuzzer / js-reverse-mcp / response-modifier），
  不得用 CLI 工具或 curl 替代流量录制/MCP 做请求测试。
  判据：这一枪**要不要看回包细节**？要 → MCP；不要（只要结果列表）→ CLI 工具。
- **适用场景**：
  - 需要批量枚举、扫描、跑模板，而不是逐条请求验证
  - 不知道某个能力该用哪个工具（查选型表）
  - WAF 判定（`cdncheck` → `wafw00f`）
  - 拿到 shell 后的后渗透 / 域渗透 / 建隧道
  - URL·JSON·编解码批处理
- **触发词**：
  - "用什么工具"
  - "工具选型"
  - "批量扫描"
  - "目录爆破"
  - "参数发现"
  - "提权"
  - "内网穿透"
  - "建隧道"
  - "WAF 识别"
- **详细内容**：`../../tool-arsenal/SKILL.md`；工具台账见 `tools/INDEX.md`
