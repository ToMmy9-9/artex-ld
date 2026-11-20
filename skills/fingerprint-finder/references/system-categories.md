# 系统类别视角（指纹识别的第一步）

> ⚠️ **核心原则**：所有指纹识别完成后，第一时间回答：
> "这是 **自研系统** / **二开系统** / **通用系统**？"
> 不同类别对应完全不同的识别策略与测试入口。

---

## 1. 系统形态分类（自研 / 二开 / 通用）

| 类别 | 含义 | 识别信号 | 测试入口偏好 |
|---|---|---|---|
| **自研系统** | 业务公司自研，无通用指纹 | 无 meta generator、无明显框架头、JS 高度业务化 | 鉴权结构 / 注册功能 / token 接口 / 前端硬编码 |
| **二开系统** | 在 Spring Boot / ThinkPHP / Jeecg / 若依基础上二次开发 | 看到 /actuator 又出现 /ureport、/druid、/webroot | 框架组件 + 默认口令 + 历史 nday |
| **通用系统** | WordPress / Confluence / Jira / 泛微 / 用友 等公开产品 | meta generator、CMS 标识、明确版本号 | nday / 0day 直接打 |

> 二开系统的指纹识别重点在 **关联路径而非表面**；
> 自研系统的指纹识别重点在 **认证流而非组件**。

---

## 2. 系统形态 → 测试策略映射（A/B/C/D/E 判定）

形态分类（自研/二开/通用）决定技术栈，策略分类（A/B/C/D/E）决定测试路径。
fingerprint-finder 在形态识别后**必须**给出 A/B/C/D/E 判定结果，再交接给 pentest-workflow 编排。

### 2.1 形态 ↔ 测试策略对照表

> ⚠️ **A类判定前提条件（极易误判）**
>
> **A类 = 第三方商业产品**（用友/致远/泛微/Jenkins/Nacos等），不是"有CVE的系统"。
>
> 以下情况**不能判A类**（即使命中CVE/memory记录）：
> - "memory有CVE记录" → ❌ 不能反推A类
> - "目标有已知漏洞" → ❌ 不能反推A类  
> - "自研+商业组件嵌入" → ❌ 应判B类（双线并行）
> - "系统建设方自研" → ❌ 按自研/二开判定，不是A类
> - "集成MQTT/支付宝支付/高德地图" → ❌ 集成第三方服务 ≠ 商业产品
>
> **A类必须同时满足**：① 目标属于第三方商业产品厂商 ② URL/版权直接指向该厂商 ③ 不是系统建设方自研

| 系统形态 | 测试策略类别 | 含义 | 测试路径 | 推荐场景 |
|---------|------------|------|---------|---------|
| **通用** | **A**（成熟商业） | 商业 OA/中间件/运维工具等公开产品 | **POC 主路径** + 嵌入组件层 + 轻量业务逻辑兜底（≤30min） | 场景 6（商业 OA 定向）→ 回退场景 1 |
| **自研 + 嵌入 Spring Boot/Fastjson/Shiro 等** | **B**（自研含组件） | 自研业务 + 嵌入第三方组件（高频情况：政务/医疗/金融自研系统） | **双线并行**（业务逻辑 60% + POC 40%）| 场景 1（完整渗透测试） |
| **二开** | **C**（开源二开） | Metabase/Superset/JeecgBoot/若依 改造 | **POC 主路径** + 二开代码业务逻辑 | 场景 1（完整渗透测试） |
| **自研 + 无嵌入组件** | **D**（纯自研） | 嵌入式 web / 冷门技术栈 | **业务逻辑主路径** | 场景 4（API 安全测试）→ 回退场景 1 |
| **混合架构** | **E**（混合） | 商业产品 + 自研扩展 / 自研 + 商业组件嵌入（如政务 OA 含财务用友模块） | 按主导部分归类，附属部分轻量测试 | 按主导部分对应场景 |

**重要原则**：
- 形态分类（自研/二开/通用）由 fingerprint-finder 完成
- 测试策略（A/B/C/D/E）由 fingerprint-finder 在形态识别后立即给出
- pentest-workflow 消费 A/B/C/D/E 决策结果执行对应编排（详见 [system-category-routing.md](system-category-routing.md)）
- 两者不冲突，而是**正交维度**（形态决定技术栈，策略决定测试路径）
- **⚠️ 负面证据校验（必须）**：A类判定前必须检查"是否是系统建设方自研"，若存在以下任一信号 → **禁止判A类**：
  - 目标ICP备案/版权声明指向实际运营单位（不是软件厂商）
  - JS中包含目标专属业务术语（停车/充电/物业等垂直行业特征）
  - 系统名称含目标单位名称（如"xx智慧停车平台"）
  - 技术栈为ASP.NET+Vue（通用商业产品不用这套组合）
  - 自研认证结构（LoginHandler.ashx等自定义接口）
- 证据不足时统一按 **B 类**（双线并行）保守处理，避免激进跳过业务逻辑
- **场景映射**：fingerprint-finder 在输出 A/B/C/D/E 时必须同时输出**推荐场景编号**，供 pentest-workflow 做场景二次确认（详见 §2.4）

### 2.2 A 类（成熟商业产品）扩展定义

**原有的 A 类覆盖**：商业 OA（用友/致远/泛微/通达/万户/金蝶/蓝凌）、CMS（WordPress/Discuz/Drupal）、商业软件（Confluence/Jira）

**⚠️ A类的本质**：A类是"第三方厂商的成熟产品被部署在目标处"，不是"目标自己开发的有漏洞的系统"。

**不属于A类的典型误判**：
- 目标自研系统使用了Spring Boot（是B类不是A类）
- 目标自研系统使用了Fastjson/Shiro（是B类不是A类）
- 目标集成MQTT/支付宝支付/高德地图（是B类不是A类）
- 目标系统在memory中有CVE记录（不能反推A类，必须重新从指纹判定）

**扩展 A 类覆盖——独立中间件/基础设施/运维工具**（以下组件暴露时，测试策略应走 A 类 POC 主路径，而非归入 B 类的自研范畴）：

| 类别 | 典型组件 | 漏洞模式 |
|---|---|---|
| **CI/CD 平台** | Jenkins, GitLab, TeamCity | 未授权访问、RCE、凭据泄露 |
| **数据中间件** | Nacos, Redis, MinIO, Kafka, ZooKeeper | 默认口令、未授权、配置泄露 |
| **可视化平台** | Grafana, Superset, Metabase, Kibana | SSRF 到数据库、未授权仪表盘 |
| **运维管理面板** | 1panel, phpmyadmin, pgadmin, adminer, h2database, mongo-express | 默认口令、SQL 执行、文件管理 RCE |
| **API 网关/代理** | APISIX, Nginx-UI, Envoy | 配置绕过、Dashboard 未授权 |
| **文档/协作工具** | Showdoc, YApi, kkfileview, ownCloud | 文件上传、未授权访问、SQL 注入 |
| **监控/日志** | Zabbix, Ntopng, Cacti, Grafana | 默认口令、命令注入、SSRF |
| **消息队列** | ActiveMQ, RocketMQ, RabbitMQ | 反序列化 RCE、未授权 |
| **注册中心/配置中心** | Nacos, Eureka, Apollo | 配置泄露、未授权写 |
| **工作流引擎** | Flowable, Activiti, Camunda | 部署 RCE、未授权操作 |

**判定规则**：命中以上任一组件且独立部署（非嵌入自研系统）→ 直接判定 **A 类**，走 POC 主路径，推荐场景 1（中间件专项子路径）。

### 2.3 B 类（自研含嵌入组件）识别信号

**边界场景**：原"自研系统"分类未明确包含"自研但嵌入第三方组件"的情况。实际中：
- 政务/医疗/金融系统经常是业务自研 + 后端用 Spring Boot/Fastjson/Shiro
- 看似自研，但实际继承了框架的全部 nday 风险

**识别信号**：
```
- JS 高度业务化（自研特征）
- 但后端响应头/错误页/技术特征显示 Spring Boot / ThinkPHP / Fastjson 等
- API 路径自定义（自研特征）
- 但 /actuator/、/druid/、/swagger-ui/ 等框架路径同时存在
- 满足自研 Checklist ≥3 项 + 嵌入组件特征 ≥1 项 → B 类
```

**典型代表**：大华 DSS（非 ICC 版）等带自研业务壳的 IoT 系统、客户自研 OA 但用了若依/JeecgBoot 部分模块

**测试策略**：**双线并行**（业务逻辑 60% + POC 40%）

### 2.4 E 类（混合架构）识别信号

**边界场景**：原三类未覆盖的混合情况：
- 商业产品 + 自研扩展（如政务 OA 含用友 NC 财务模块）
- 自研系统 + 商业组件嵌入（如自研 CRM 嵌入第三方 SSO）

**识别信号**：
```
- 同时识别到通用商业产品指纹（meta generator/CMS 标识）
- 同时识别到自研业务特征（自定义 API 路径/业务化 JS）
- 客户端下载链接指向多个产品
- API 端点混合标准路径与自定义路径
```

**判断方法**：核心业务模块 vs 边缘模块的占比
- 核心模块是商业产品 → 按 A 类，附属模块轻量测试
- 核心模块是自研 → 按 B 类，附属模块轻量测试

**测试策略**：按主导部分归类，附属部分轻量测试（≤30 分钟）

> → A/B/C/D/E 各类的具体调用顺序（编排执行）见 [system-category-routing.md](system-category-routing.md)

### 2.5 测试策略 → 推荐场景映射

fingerprint-finder 完成 A/B/C/D/E 判定后，必须同时输出**推荐场景编号**，供 pentest-workflow 决策。

#### 2.5.1 分类 → 场景映射表

| 测试策略 | 推荐场景 | 描述 | 回退场景 |
|---------|---------|------|---------|
| **A 类** | 场景 6（已知商业产品名）→ 场景 1（中间件/POC 主路径） | 命中商业 OA 产品名 → 场景 6；命中独立中间件/运维工具 → 场景 1 | 场景 1 |
| **B 类** | 场景 1（完整渗透测试） | 双线并行：业务逻辑 + POC 同时覆盖 | 场景 1 |
| **C 类** | 场景 1（完整渗透测试） | POC 主路径 + 二开业务逻辑 | 场景 1 |
| **D 类** | 场景 4（API 安全测试）→ 场景 1 | 纯自研优先走 API 专项，发现组件后回退场景 1 | 场景 1 |
| **E 类** | 按主导部分 | 主导 A 类 → 场景 6/1；主导 B 类 → 场景 1 | 场景 1 |
| **无法判定** | 场景 1（默认回退） | 证据不足 → B 类保守处理 → 场景 1 | - |

#### 2.5.2 指纹信号 → 场景映射规则

| 指纹信号 | 判定条件 | 推荐场景 | 说明 |
|---------|---------|---------|------|
| 商业 OA 产品名 | 用友/致远/泛微/通达/万户/金蝶/蓝凌 | 场景 6 | 直接走 poc-scan 主战场 |
| 独立中间件 | Jenkins/GitLab/Nacos/Redis/MinIO/ActiveMQ 等 | 场景 1 | A 类 POC 主路径，含中间件专项子路径 |
| 运维管理面板 | phpmyadmin/1panel/grafana/nginx-ui 等 | 场景 1 | A 类 POC 主路径，默认口令优先 |
| 自研技术栈 | Spring Boot + ThinkPHP + 自定义 API | 场景 1 | B 类双线并行 |
| 纯自研前端 | Vue/React + 自定义 API + 无组件指纹 | 场景 4 | D 类 API 专项优先 |
| 开源二开 | JeecgBoot/若依/Metabase 改造 | 场景 1 | C 类 POC + 业务逻辑 |
| AI/ML 平台 | comfyui/gradio/langflow/n8n | 场景 1 | A 类 POC + 配置审计 + 工作流分析 |
| 无法判定 | 信号不足 ≥3 项 | 场景 1 | B 类保守处理 |

#### 2.5.3 fingerprint-finder 输出格式

```
输出格式（必含）：
  system_type: 自研 / 二开 / 通用
  test_strategy: A / B / C / D / E
  recommended_scenario: 1-7    # ← 新增：推荐场景编号
  scenario_reason: "自研系统 + 无嵌入组件 → 场景 4"  # ← 新增：映射理由
  detail: 具体指纹信息...
```

---

## 3. 自研系统 → 认证结构指纹库

自研系统的指纹 = 它的认证流本身。详见 [auth-fingerprint.md](auth-fingerprint.md)：

- **a. 接口未授权**：去掉 Cookie 后仍返回 200 → 越权
- **b. 前端硬编码**：默认账号、调试开关、内部 IP → 弱口令 / 调试接口
- **c. 路由守卫**：仅前端校验、未在接口层校验 → 直接请求接口
- **d. 注册功能**：找到注册入口 → 注册账号 → 拿正式 token
- **e. 可获取 token 的接口**：找回密码 / 短信验证码 / 邮箱验证码 / 游客 token / 刷新 / 调试发牌
- **f. 权限提升**：role / group / isAdmin 参数 → yakit http_fuzzer 批量 fuzz
- **g. 认证结构**：OAuth2 / SSO / SAML → 走对应绕过文档
- **h. 鉴权字段绕过**：识别请求头中的 `X-User-Id`、Cookie 中的加密 `userid=` → 替换 / 置空
- **i. 签名字段逆向伪造**：`sign` / `X-Sign` + 前端 secret → 删签 / 重放 / 重签名篡改

> ⚠️ **识别后强制 handoff**：`system_type=自研` 一经判定，必须调度 **`api-tester`** 执行完整 a–i 矩阵（[06-self-dev-auth-matrix.md](../../api-tester/references/06-self-dev-auth-matrix.md)），禁止只写指纹报告结束。

**自研系统识别 Checklist**：

```
[ ] 是否存在独立 meta generator？
[ ] HTML 中是否有 framework 关键字（Vue/React/Spring 标识）？
[ ] JS 文件是否有业务域特征（无通用库名）？
[ ] API 路径是否完全自定义（如 /api/v1/user 而不是 /api/auth）？
[ ] 管理后台路径是否自定义（如 /manage 而不是 /admin）？
满足 ≥3 项 → 自研系统
```

---

## 4. 二开系统 → 关联组件字典

二开系统的指纹 = 默认组件的关联路径。详见 [deep-discovery.md](deep-discovery.md)。常见组合速查：

| 看到 | 必试 |
|---|---|
| `/actuator` | `/druid`、`/ureport`、`/webroot`、`/api-docs`、`/swagger-ui` |
| `/admin` | `/druid`、`/datasource`、`/ureport`、`/actuator` |
| `/datasource` | `/ureport`、`/phpmyadmin`、`/druid`、`/webroot` |
| `/webroot` | `demo / 123456`、`demo / demo`（默认口令） |
| `/jeecg` | `/druid`、`/ureport`、`/actuator`（JeecgBoot 特征） |
| `/ruoyi` | `/druid`、`/swagger-ui`、`/actuator`（RuoYi 特征） |
| `/login.html` + ThinkPHP 头 | `/admin.php`、`/manage/` |

> ⚠️ `/ureport/` 属 **Java** 报表组件（UReport2 · bstek），**不要**与 PHP 框架（ThinkPHP / FastAdmin）关联；上表中它与 `/druid`、`/actuator` 并列的用法才是正确的。

**二开系统识别 Checklist**：

```
[ ] 是否识别到 Spring Boot 特征（/actuator、/env、Java 异常）？
[ ] 是否识别到 ThinkPHP 特征（X-ThinkPHP-Version、/index.php?s=）？
[ ] 是否存在 ureport / druid / webroot 等"业务组件"但签名被改？
[ ] 框架版本号是否过期（如 Spring Boot 2.0.x）？
[ ] 默认账号是否还能登录（admin/123456、demo/demo）？
满足 ≥3 项 → 二开系统
```

| 框架 | 关键指纹 | 必试组件 |
|---|---|---|
| **JeecgBoot** | `/jeecg-boot/`、`Ant Design Vue`、`@Jeecg` | `/druid`、`/ureport`、`/actuator`、`swagger-ui` |
| **RuoYi (若依)** | `/login`+`/captchaImage`、`ry-ui`、`若依` | `/druid`、`/swagger-ui`、`/actuator` |
| **人人权限框架** | `/login`、`renren`、`人人`、`renren.io` | `/admin`、`/generator`、`/modules/`、`/ext`**（Shiro rememberMe 高危）** |
| **JEPaaS 低代码** | `JEPaaS`、`低代码平台`、`j_spring_security_check` | `/a/main`、`/document/`、`/module/` |
| **CAS 单点登录** | `CAS`、`x509`、`cas-server`、`ServiceValidate` | `/login`、`/logout`、`/serviceValidate` |
| **短链接系统** | 302 重定向、短 URL、`/s/`、`/short/`、`/t/` | 预测/枚举、Referer 泄露、后台入口 |
| **表单系统** | `表单`、`form`、`@FormBody`、`easy-form` | 未授权访问、导出功能、字段注入 |
| **Guns** | `/guns/`、`@Controller` 命名规范 | `/druid`、`/swagger-ui` |
| **iBase4J** | `/iBase4J-SYS-Web/`、`@Permission` 注解 | `/druid`、`/swagger-ui` |
| **Torna** | `/torna/`、`/swagger-ui/` | `/druid`（若集成了） |
| **FastAdmin** | `FastAdmin`、`/admin.php`、`/index.php` | `/phpmyadmin` |
| **WordPress** | `/wp-admin/`、`/wp-content/`、`/wp-includes/` | `/xmlrpc.php`、`/wp-config.php` |
| **Discuz** | `/forum.php`、`/uc_server/` | `/uc_server/admin/` |

#### 4.1.1 人人权限框架 (renren-fast / renren-security)

**识别信号**：
```
- HTML/JS 中出现 "renren" 关键字
- Cookie 中出现 renren_cookie 或类似命名
- `/login` 返回 JSON 含 "今天" 或随机背景图
- Shiro rememberMe Cookie（高频共存）
```

**必试路径**：
```
/admin                              → 后台管理（权限绕过高危）
/generator/                        → 代码生成器（SQL 泄露表结构）
/modules/sys/login.htm             → 登录页
/sys/user/list                     → 用户列表（未授权）
/tool/gen/test/{tableName}         → 代码生成测试
```

**高危漏洞模式**：
- Shiro 反序列化 → rememberMe Cookie 可被利用
- 权限绕过 → `/modules/*` 接口往往仅前端校验
- 代码生成器 → 获取数据库表结构 + 潜在 SQL 注入

#### 4.1.2 JEPaaS 低代码平台

**识别信号**：
```
- 页面出现 "JEPaaS" 字样或 Logo
- Cookie 中含 `jeepai` 或 `jepaas`
- `/j_spring_security_check` 登录接口
- 低代码表单设计器 UI 特征
```

**必试路径**：
```
/a/main                             → 管理后台
/document/                          → 文档上传（文件上传 RCE）
/module/                            → 模块管理
/j_spring_security_check           → 登录接口（SQL 注入 CVE）
```

**高危漏洞**：
- `/document/` 文件上传 → RCE
- `/j_spring_security_check` SQL 注入
- 后台任意文件读取

#### 4.1.3 CAS 单点登录

**识别信号**：
```
- 302 重定向到 /login?service=xxx
- 响应含 "CAS" 或 "Central Authentication Service"
- /serviceValidate 接口存在
- 登录后出现 TGC Ticket Granting Cookie
```

**必试路径**：
```
/login?service=xxx                 → 登录入口
/serviceValidate                   → 票据验证（可伪造）
/p3/serviceValidate                → P3 协议（支持属性）
/logout                            → 登出（可能不过期）
```

**高危漏洞**：
- 票据伪造 → 任意用户登录
- Service 参数可控 → 钓鱼链接构造
- TGT 会话劫持 → 横向移动

#### 4.1.4 短链接系统

**识别信号**：
```
- 访问 URL 直接 302 重定向
- URL 极短（如 /s/abc、/t/xyz、/l/123）
- 响应 Location 头含原始 URL
- 短码为纯字母或数字，6-8 位
```

**必试路径**：
```
枚举短码（6位 = 26^6 ≈ 3 亿组合，优先社工/规律猜测）
/admin                             → 后台入口（管理系统）
/api/shorten                      → 短链接生成接口
```

**高危漏洞**：
- 短码预测/枚举 → 撞库获取他人短链接
- Referer 泄露 → 原始 URL 在日志/Referer 中暴露
- 短链接后台弱口令 → 管理所有链接数据

#### 4.1.5 表单系统（自建 / 通用表单引擎）

**识别信号**：
```
- 路径含 /form/、/formdesign/、/form-builder/
- 大量 POST JSON 含 fieldName、fieldType 参数
- Export/Import 功能
- 第三方表单引擎关键字（FormCreate、EasyForm、form-generator）
```

**必试路径**：
```
/form/list                        → 表单列表（未授权）
/form/{id}/data                  → 表单数据（未授权）
/form/export                      → 数据导出
/form/submit                      → 提交（可能有 XSS/存储型漏洞）
```

**高危漏洞**：
- 存储型 XSS → 表单提交时触发
- 未授权数据访问 → 查看他人填报数据
- SQL 注入 → 动态字段拼接
- 文件导出 → 模板注入（SSTI）

---

## 5. 通用系统 → nday / 0day

通用系统的指纹 = 组件名 + 版本号。识别后**直接调本地 POC 库**：

### 5.1 必调 skill

```
poc-scan
poc-scan-skill-1d8b7c33
poc-scan-skill-208c9611
poc-scan-skill-3625e2a9
poc-scan-skill-cf55985f
```

上述 skill 使用本地约 2.5 万文件 POC 语料（nuclei-templates / afrog-pocs / Awesome-POC / 商业 OA 专项，跨源未去重，已冻结只读），覆盖：
- 商业 OA：用友 / 致远 / 泛微 / 通达 / 万户 / 金蝶 / 蓝凌
- 通用组件：Struts2 / Shiro / Weblogic / Solr / ActiveMQ / Fastjson
- 开源 CMS：WordPress / Discuz / DedeCMS

### 5.2 用 yakit 查询

- `query_fingerprint` —— 输入组件名查已知指纹
- `query_cve` —— 输入组件 + 版本查对应 CVE

### 5.3 通用系统识别 Checklist

```
[ ] HTML 中是否有 meta generator？
    <meta name="generator" content="WordPress 5.8">
    <meta name="generator" content="DedeCMS V57">
[ ] 是否识别到明确的 CMS / OA 标识？
[ ] 默认管理后台路径是否被保留（如 /wp-admin/、/dede/）？
[ ] 版本号是否可直接读出（X-Powered-By、X-Generator）？
满足 ≥2 项 → 通用系统
```

---

## 6. 三类系统的测试入口汇总

| 类别 | 第一手测试 | 第二手测试 | 必报告产物 |
|---|---|---|---|
| **自研** | 注册功能 → 拿 token → 全量接口 | 鉴权字段绕过 / 前端硬编码 / 接口未授权 | 完整认证流图、token 复用证据 |
| **二开** | 关联组件 Fuzz（/druid / /ureport / 默认口令） | 框架 nday + 自研部分 | 框架版本 + 组件清单 + nday 命中情况 |
| **通用** | POC 库直接打 | 0day / 未公开漏洞 | POC 名称 + 命中 payload + 复现截图 |

---

## 7. 系统类别采集模板（沉淀到 work-space）

```yaml
target: https://xxx.com
system_type: 自研 / 二开 / 通用
sub_type: # 二开系统填写
  framework: JeecgBoot / RuoYi / Guns / iBase4J / FastAdmin / 其它
  version: 2.4.0
test_strategy: A / B / C / D / E # 由 §2 判定规则给出
signals:
  self_dev:
    - 无 meta generator
    - JS 完全业务化
  secondary_dev:
    - /actuator 命中 + /druid 命中
    - Spring Boot 异常栈泄露
  generic:
    - meta generator: WordPress 5.8
    - /wp-admin/ 保留
test_entries_plan:
  - 第一手：xx
  - 第二手：yy
evidence_files:
  - work-space/[target]/recon/category-decision.md
```