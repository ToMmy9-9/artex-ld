---
name: "api-tester"
description: "API 接口发现、认证结构分析、访问控制验证、参数行为测试与结果归因。"
---

# API 深度测试技能

此技能帮助你深度测试和探索 API 接口，包括接口发现、fuzz 测试、凭据获取和数据提取。

## 核心功能

- **接口发现**：深度搜索网站的隐藏 API 接口
- **Fuzz 测试**：对 API 端点进行模糊测试（端点、参数、请求方法、头部）,需获取对应站点接口前缀
- **凭据获取**：⚠️ **重点发现**能获取凭据的接口（注册、Token生成等），一旦获取凭据立即对其他接口进行**组合测试**
- **深度测试**：利用获取的凭据进行更深入的接口测试
- **数据提取**：从响应中提取敏感数据
- **自研系统认证矩阵** ⚠️：系统类型判定为自研后，**必须**按 a–i 九项执行（未授权 / 硬编码 / 路由守卫 / 注册 / 发 token 全接口 / 垂直越权 / SSO·OAuth2 / 鉴权字段 / 签名伪造），并先从 JS / 菜单 / 路由 / store 反推出权限模型与可获取权限，详见 [references/06-self-dev-auth-matrix.md](references/06-self-dev-auth-matrix.md)
- 逻辑漏洞测试
- 越权测试（水平越权、垂直越权）
- 凭据获取（注册流程优先）
- JS逆向分析（提取API端点、参数、密钥）



## 测试流程

### 第一阶段：接口发现与参数提取
接口fuzz策略（！！！！！必须执行）
- 从路由页面加载(全量提取)->全量路由访问->访问后触发全流量->流量全量加载保存(全量提取)
- 从js中分析提取全量接口+header头（可能存在鉴权绕过）+对应参数
- 根据不同接口+组合对应参数+可能存在的特殊header头进行fuzzing
1. **前端资源分析**：提取所有 JS 文件（内联、外链、动态加载）
2. **JS 逆向分析**：搜索 API 端点路径、提取请求参数、识别加密算法、查找硬编码密钥、提取 routes / menus / permissions / roles / tenant 结构
3. **综合信息收集**：查找 API 文档、分析 robots.txt、检查备份文件
4. **参数发现**：分析请求参数、测试隐藏参数
5. 深度测试API接口，包括接口发现、fuzz测试、凭据获取和数据提取。

1. 接口发现与参数提取（前端资源分析、JS逆向分析）
2. Fuzz测试（端点Fuzz、JSON格式Fuzz、参数逻辑跳过Fuzz）
3. 凭据获取（注册流程利用、登录接口利用、Token生成接口）
4. 组合测试（横向扩散、权限提升、数据扩展、链式挖掘）


**强制调用场景（按系统类别条件触发）**：
- **A 类（成熟商业产品）**：仅"凭据获取"环节必调用（其他深度测试跳过，避免无效投入）
- **B 类（自研含组件）**：⚠️ 每次必调用（双线并行的核心）
- **C 类（开源二开）**：必调用，但聚焦"二开代码业务逻辑"
- **D 类（纯自研）**：⚠️ 每次必调用（业务逻辑主路径）
- **E 类（混合架构）**：按主导类别决定
- **通用触发**：发现任何API接口时 / 需要获取测试凭据时 / 进行越权测试时 / 进行逻辑漏洞测试时


**重要原则**：
- 禁止登录接口爆破
- 优先使用注册流程获取凭据
- ⚠️ **role参数必须测试（0,1,2等值）**
- ⚠️ **获取凭据后立即进行组合测试**
- 无需在暴力破解上浪费过多时间

**详细文档**：[api-tester/SKILL.md](references/../SKILL.md)



> 详细说明请参考 [references/01-js-analysis.md](references/01-js-analysis.md)

### 第二阶段：Fuzz 测试

#### 1. 端点 Fuzz
```
/api/v1/{users,admin,config,data}
/api/v1/users/{1..100}
/ureport /ureport/designer### /durid
```

#### 2. JSON 格式 Fuzz（重点）

**核心原理**：通过改变 JSON 对象的嵌套结构，测试服务器的多重解析行为。

**常见场景**：
- **优惠券叠加**：同键重复、数组形式、嵌套形式
- **数量叠加**：同键重复值不同
- **价格修改**：价格字段重复或嵌套
- **IDOR 绕过**：用户 ID 重复或嵌套

#### 3. 参数逻辑跳过 Fuzz（重点）

**核心原理**：通过猜测和枚举参数值，跳过多步骤流程中的前置步骤。

**命名规律**：数字递增型、功能编号型、操作类型型、状态机型等

> 详细说明请参考 [references/02-json-fuzz.md](references/02-json-fuzz.md) 和 [references/03-parameter-logic-skip.md](references/03-parameter-logic-skip.md)

### 第三阶段：凭据获取 ⚠️（重点）

⚠️ **凭据接口优先级最高**：任何能获取凭据的接口（注册、Token生成、OAuth等）都是**重点发现目标**，必须优先测试！

⚠️ **组合测试原则**：获取凭据后**立即**使用该凭据对已发现的所有接口进行组合测试，不等待！

1. **注册流程利用**（重点）：发现注册接口 → 分析参数 → ⚠️**测试role/permission参数（0,1,2等值）** → 注册新用户 → 绕过验证 → 获取凭据

⚠️ **role参数测试要点**：
   - **0值必须测试**：很多系统0代表最高管理员权限，漏掉0会错过严重漏洞
   - **枚举所有值**：0, 1, 2, 3, admin, super, owner等
   - **注册时测试**：注册接口中尝试添加role参数
   - **修改时测试**：用户资料修改接口中尝试修改role参数
   - **常见参数名**：role, permission, level, user_type, is_admin, is_super, auth_level, group_id

2. **登录接口利用**：发现登录接口 → 按业务逻辑构造弱口令字典（禁止无脑公开账号爆破）→ 获取凭据
3. **Token生成接口**：发现 token 生成接口 → 分析算法 → 生成有效 token
4. **OAuth/SSO接口**：发现第三方登录 → 测试授权流程 → 获取凭据
5. **未授权访问测试**：测试无需认证即可访问的接口
6. **信息泄露获取凭据**：查找响应中的 token、key、secret
7. **一切可发 token 的接口**（自研 P0）：刷新 / 游客 / 短信登录 / 重置密码 / 切换租户 / 调试发牌 / 小程序 code2session 等，见矩阵 **e**

> 详细说明请参考 [references/04-test-scenarios.md](references/04-test-scenarios.md)

### 第三阶段-B：二开系统 API 漏洞 ⚠️（判定二开后激活）

当 系统识别能力 输出 `system_type=二开` 时，**二开系统的 API 攻击面和自研完全不同**——重点不在认证流，而在**组件路径 + 历史漏洞**。

**二开系统 vs 自研系统的 API 测试差异**：

| 维度 | 自研系统 | 二开系统 |
|---|---|---|
| 攻击面 | 认证流（注册/登录/token/越权） | 组件漏洞（druid/swagger/ureport） |
| API 特征 | 完全自定义 | 继承父框架（Spring Boot/ThinkPHP） |
| 重点 | 鉴权矩阵 a–i | 关联路径探测 + nday 命中 |
| 凭据获取 | 优先（注册拿 token） | 次要（后台默认口令） |

**二开系统 API 探测优先级（按 ROI）**：

```
① Druid 未授权（★极高）
   /druid/index.html           → 数据库可视化
   /druid/datasource.html     → 查所有数据源密码
   /druid/sql.html             → 直接执行 SQL
   /druid/wall-console         → 防火墙状态

② Swagger / API 文档（★极高）
   /swagger-ui.html            → 未授权 API 端点清单
   /v2/api-docs                → OpenAPI JSON
   /doc.html                   → SpringDoc
   → 从中找未授权接口

③ Ureport 报表（★高）
   /ureport/designer/          → 报表设计器
   /ureport/preview            → 预览（可能有注入）
   /ureport/datasource         → 数据源配置

④ actuator 端点（★高）
   /actuator/env               → 环境变量（含 AK/SK/DB 密码）
   /actuator/heapdump          → 堆转储（找密码/JWT）
   /actuator/beans             → Bean 列表（找认证类）

⑤ 后台管理 API（★中）
   /admin/*, /manage/*, /system/*
   → 后台往往仅前端校验，直打后端可能绕过

⑥ 二开框架特有接口
   JeecgBoot:  /jmreport/*, /qurest/*
   RuoYi:      /monitor/job/*, /tool/gen/*
   人人:       /generator/*, /modules/*
   JEPaaS:     /document/*, /module/*
```

**二开系统凭据获取**：
- 默认账号优先：`admin/admin`、`admin/123456`、`demo/demo`
- Shiro rememberMe → 反序列化 RCE（不走登录）
- 后台 → 定时任务 RCE

### 第三阶段-C：自研系统认证矩阵 ⚠️（判定自研后强制）

当 系统识别能力 输出 `system_type=自研`（或等价信号）时，**不得**只做通用端点 fuzz，必须加载并执行：

> [references/06-self-dev-auth-matrix.md](references/06-self-dev-auth-matrix.md)

| 项 | 向量 | 本 skill 动作 |
|----|------|----------------|
| a | 接口未授权 | 去 Cookie/Authorization 全量复测 + id 遍历 |
| b | 前端硬编码 | 验证账号/密钥/调试开关是否真正可用 |
| c | 路由守卫绕过 | 前端挡登录、直打后端 API |
| d | 注册功能 | 注册拿 token + role 注入（P0） |
| e | 一切可获取 token 的接口 | 发牌接口全家桶（P0） |
| f | 权限提升（垂直越权） | 管理接口 + role/tenant 参数 + JS 权限码对应关系 |
| g | **认证结构 SSO/OAuth2** | 见下方专项 |
| h | 鉴权字段绕过 | 异常请求头/Cookie 身份字段替换·置空·重加密 + 调试后门头 |
| i | 签名字段逆向伪造 | 还原 sign 算法与 secret → 删签/重放/重签篡改 |

**g 项：OAuth2 绕过（3 种向量）**：

| 向量 | 测试方法 |
|---|---|
| **client_secret** | 暴力破解 / 前端泄露 / 请求包泄露 / JS 中泄露 / 其他接口组合泄露 |
| **userid** | 越权（替换为受害者 userid）/ 加密后置空 |
| **passwd** | 暴力破解 / 直接未验证 / 绕过 / 加密后置空 |

```
OAuth2 测试步骤：
① 找 token 端点 → client_id + 错误 client_secret / 空 secret
② 找 callback 端点 → 改 userid / 置空 / 重放 code
③ 从 JS 中提取 client_secret（静态分析）
④ 暴力破解 client_secret（若可猜测）
⑤ 替换 userid 为受害者 → 水平越权
```

**g 项：SSO 绕过（3 种向量）**：

| 向量 | 测试方法 |
|---|---|
| **userid 置空** | 加密后置空 → 落到默认系统账号（高危） |
| **callback 越权** | 修改 callback 参数指向恶意站点 / callback 域可控 |
| **cookie 复制** | 窃取 SSO Cookie 后复制到目标系统 |

```
SSO 测试步骤：
① 找 SSO 登录入口 → 分析 cookie 名（TGC / JSESSIONID）
② 找 userid 加密字段 → Base64/Hex 解码看是否 userid
③ 置空加密字段 → 看是否落到默认账号
④ 抓取 SSO cookie → 复制到业务系统 cookie 中
```

**优先级**：P0 = d/e/a → P1 = c/b/f/h/i → P2 = g（若存在协议指纹）。任一突破立即进入第四阶段组合测试。

### 第三阶段-D：扩展矩阵（高频有效但易被忽视）⚠️

以下矩阵在实战中**高频命中但容易被忽视**，建议作为常规测试项：

#### j. API 版本差异

```
/v1/ vs /v2/ → 旧版往往更弱
```

**测试方法**：
```
① 枚举 API 版本：/api/v1/*、/api/v2/*、/api/v3/*
② 对比版本权限：v1 无鉴权 / v2 有鉴权
③ 常见场景：
   - /v1/user/info（无鉴权） vs /v2/user/info（需 token）
   - /v1/admin/*（未鉴权） vs /v2/admin/*（已鉴权）
④ 高危模式：旧版 API 往往是遗留代码，补丁不同步
```

#### k. JWT 降级攻击

```
RS256 → HS256 / none algorithm
```

**测试方法**：
```
① 抓 JWT token → 分析 alg 字段
② 若 RS256 → 尝试改为 HS256 → 用公钥作为 secret 签名
③ 若 RS256 → 尝试改为 none → 删签名部分
③ 请求与流量分析能力 mcp 编码处理 → 构造恶意 JWT
④ 替换 Authorization 头 → 测试 admin 权限
```

#### l. GraphQL 认证绕过

```
introspection 未关 / batch query 越权
```

**测试方法**：
```
① 找 GraphQL 端点：/graphql、/api/graphql、/query
② POST body: {"query":"{__schema{types{name}}}"}
③ introspection 未关 → 获取完整 schema → 找隐藏字段/接口
④ batch query: [{"query":"...account{id}"},{"query":"...admin{users}"}]
⑤ 水平越权：同一 batch 中混合查询 A 用户和 B 用户数据
```

#### m. 多接口组合越权 ⚠️（骚操作）

**核心原理**：A 接口泄露数据 → B 接口参数为空时不校验 → 组合利用实现越权

```
场景示例：
A 接口 GET /api/user/profile          → 返回 {id: 1001, name: "张三", phone: "138xxxx"}
B 接口 GET /api/order/list?userId=   → userId 为空时不校验，返回空列表

攻击链路：
① 调用 A 接口（无需认证）→ 获取目标用户的 id
② 调用 B 接口 userId=1001         → 查到张三的订单（越权）
```

**测试方法**：
```
① 信息收集：
   - A 接口（无需认证）泄露了哪些字段？ID / 手机号 / 邮箱 / 工号？
   - B 接口的参数默认值是什么？为空时返回什么？
② 参数组合：
   - A 泄露的 ID → 填入 B 接口的 userId
   - A 泄露的订单号 → 填入 B 接口的 orderId
   - A 泄露的文件 ID → 填入 B 接口的 fileId
③ 常见模式：
   - /api/profile（泄露用户ID） + /api/orders（查订单）
   - /api/search?q=关键词（泄露文档ID）+ /api/doc/detail（查文档）
   - /api/chat/list（泄露会话ID）+ /api/chat/history（查聊天记录）
④ 关键：找"泄露接口"和"查询接口"的字段对应关系
```

#### n. 密码找回 token 预测

```
时间戳 / UUID / 顺序号 / 用户名哈希
```

**测试方法**：
```
① 正常找回流程：输入手机号/邮箱 → 收到验证码/token
② 观察 token 格式：
   - 时间戳：reset.php?token=1701234567
   - UUID：reset.php?token=550e8400-e29b-41d4-a716-446655440000
   - 顺序号：reset.php?token=10086
   - Base64(用户名)：reset.php?token=5aW25aW2（admin的base64）
③ 预测 token：
   - 重放自己的 token，观察规律
   - 修改时间戳为历史时间
   - 枚举顺序号（10000-10099）
   - 替换用户名部分为受害者
④ 高危：token 可预测 → 直接重置任意用户密码
```

**优先级**：k > l > j > m > n

---

### 第四阶段：组合测试（凭据获取后立即执行）

获取凭据后**立即**对所有已发现接口进行组合测试：

1. **横向扩散**：使用凭据测试所有已发现的接口，发现更多端点
2. **权限提升**：测试普通凭据是否能访问管理员接口
3. **数据扩展**：凭据 + 已有接口 → 提取更多数据
4. **链式挖掘**：凭据 + 越权 → 获取其他用户数据

## 测试策略

1. **凭据接口优先策略**：⚠️ 任何能获取凭据的接口（登录、注册、Token生成、OAuth等）都是**重点发现目标**，优先测试！
2. **组合测试策略**：⚠️ 获取凭据后**立即**对所有已发现接口进行组合测试，不等待！
3. **role参数枚举策略**：⚠️ **注册/修改时必须测试role等权限参数（0,1,2等值）**，0值往往代表最高权限，漏掉0会错过严重漏洞！
4. **JS 分析优先策略**：必须从网站 JS / sourcemap / 动态 chunk / store / 路由配置中综合搜寻参数、权限码、角色、租户与调试后门
5. **渐进式测试**：匿名访问 → 注册用户 → 普通用户 → 管理员
6. **智能字典**：根据上下文选择字典，从 JS 中提取真实参数名
7. **自研矩阵强制策略**：⚠️ 判定自研后必须跑完 [06-self-dev-auth-matrix.md](references/06-self-dev-auth-matrix.md) a–i Checklist，并在 `work-space/[目标]/findings/self-dev-auth-matrix.md` 留痕；禁止只识别不验证
9. **JS 权限模型策略**：⚠️ 自研目标必须先回答「系统有哪些角色/权限码/租户域？普通注册用户理论上能拿到什么权限？JS 暴露了哪些 debug/backdoor 入口？」再进入 f/h/i 深测

## 输出格式

```
[+] 发现接口：/api/v1/users
[!] 越权漏洞：水平越权/垂直越权
[+] 用户数据：100 条记录
[+] 敏感文件：backup.sql、config.json
```

自研目标额外输出 Checklist 与向量结论：

```
[自研矩阵] a 未授权: hit /api/order/detail
[自研矩阵] d 注册 token: ok
[自研矩阵] h 鉴权字段: X-User-Id 替换成功 → 用户B
[自研矩阵] i 签名伪造: MD5+secret 重签成功
```

> 详细输出格式请参考 [references/05-output-format.md](references/05-output-format.md)
> 自研矩阵模板请参考 [references/06-self-dev-auth-matrix.md](references/06-self-dev-auth-matrix.md)

## 参考文档

| 文档 | 内容 |
|------|------|
| [01-js-analysis.md](references/01-js-analysis.md) | JS 资源提取、参数与加解密逆向 |
| [02-json-fuzz.md](references/02-json-fuzz.md) | JSON 结构 / 同键重复 fuzz |
| [03-parameter-logic-skip.md](references/03-parameter-logic-skip.md) | 多步骤参数逻辑跳过 |
| [04-test-scenarios.md](references/04-test-scenarios.md) | 注册/越权/逻辑/凭据组合/role 场景 |
| [05-output-format.md](references/05-output-format.md) | 输出格式 |
| [**06-self-dev-auth-matrix.md**](references/06-self-dev-auth-matrix.md) | **自研系统 a–i 强制测试矩阵**（未授权/硬编码/路由守卫/注册/发token/垂直越权/SSO·OAuth2/鉴权字段/签名伪造） |


## 攻破后台后(已有后台权限情况下)->测试方法（拿下后台后必须测试！！！）
0、低权限的话必须尝试越权到高权限
1、尝试获取后台用户名等关键,组合字典尝试爆破，获取多个弱口令
2、获取全量后台接口(从后台的js/流量中全量抓取)，去除鉴权字段，进行fuzzing，尝试未授权访问
3、尝试获取配置文件等一且可进行辅助横向的配置，便于后续进行横向
4、横向攻破其他管理系统(子系统)
5、后台相关漏洞,比如sql注入(getshell)->一般情况下老系统存在比如net站点,ssrf(打内网/云上)，后台云AKSK，存储筒等各种密钥泄露等等，打关联测试系统(横向其他系统/接口)
6、测试其他一且高危漏洞(根据后台功能进行判断)
7、在测试时需注意自身权限(高权限有高权限的测试方法，低权限有低权限的测试方法)
8、后台有權限情況下，必須刪除权限检验字段，尝试未授权获取后台数据(这是未授权访问的测试方法论),比如post包删除鉴权字段但是需要保留滞空参数等
9、若为最高权限下,无需再测试垂直越权，必须尝试接口未授权访问


## 最佳实践

- **隐蔽性**：控制请求频率，避免触发防护
- **完整性**：记录所有测试步骤和响应；自研矩阵 Checklist 必须落盘
- **安全性**：妥善保管获取的凭据和数据
负责任地披露发现的漏洞
- **闭环性**：识别（fingerprint）→ 验证（本 skill）→ 组合利用 → findings；禁止停在指纹描述

## 执行能力适配

本 Skill 只定义分析目标、验证逻辑和结果要求。请求发送、页面交互、流量处理、脚本分析、证据采集等执行能力由宿主环境按当前任务选择，不影响本 Skill 的判断流程。

## 注意事项

⚠️ **重要警告**：不要对生产环境造成破坏性影响
⚠️ **自研警告**：无通用 POC 可打时，攻击面在认证流与业务 API——必须走 06 矩阵，而非空跑 poc-scan 后结束








