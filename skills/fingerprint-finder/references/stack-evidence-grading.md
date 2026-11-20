# 定栈证据分级与反证 SOP（防指纹误报）

> 本文件是 [framework-signatures.md](framework-signatures.md) / [deep-discovery.md](deep-discovery.md) 的**使用前提**：
> 那两份文件列的是"信号"，本文件规定"哪些信号能下结论、下到什么程度"。

## 0. 为什么需要这一篇

指纹识别的目标不是"给目标贴一个技术栈标签"，而是**为后续测试选对路径**（POC / 业务逻辑 / 认证矩阵）。贴错标签的代价是整轮打偏：

- Java 系统被误判成 PHP/ThinkPHP → 找错未授权端点与反序列化方向，POC 全打空
- .NET 系统被误判成 Java → 用错 Shiro / Fastjson 方向
- 商业 SaaS 白标反代被误判成"自研" → 走业务逻辑，漏掉 A 类 POC 主路径
- 扫描器 banner 被当结论 → 把停车自助机后台当成 XXL-JOB 调度中心

历史实战中，**误报几乎全部来自"用壳信号下了芯结论"**。

## 1. 信号分级

### 1.1 壳信号（只定"前端 / 表层"，禁止单独用于定后端语言或系统形态）

| 信号 | 说明 |
|---|---|
| 前端框架特征：`<div id="app">` / `<div id="root">`、vue/react/angular 关键字、webpack chunk 命名 | 只证明**前端**构建方式 |
| 前端 UI 库：Element UI / Ant Design / layui / jQuery | 通用组件，非某产品专属 |
| HTTP `Server` 头 | 常被反代 / CDN / WAF 改写，只反映边缘节点 |
| 页面 title / copyright / ICP / 版权署名 | 只反映"谁运营"，可能是白标反代 |
| 品牌名 / 产品名出现在跳转链接、JS 引用、帮助文档 | 只证明"被引用"，不证明"可打" |
| 扫描器 banner（UrlScan / FOFA 等第三方结果） | 未经验证的单点特征 |
| SPA 路由列表、favicon、字体 | 前端资源，无后端指向性 |
| 前端字符串（如页面上写的 "RSA 1024"） | 未解析验证前不可信 |

### 1.2 芯信号（可据以判定后端语言 / 框架 / 组件）

| 信号 | 示例 | 指向 |
|---|---|---|
| 错误回显 / 异常文本 | SQL 报错、JPA `NonUniqueResultException`、PDO 异常、Java 堆栈 | 后端语言 + ORM |
| JWT / 库解析错误消息 | `JWT strings must contain exactly 2 period characters` → jjwt(Java)；`Microsoft IdentityModel` → .NET | 运行时语言 |
| 错误页字节签名 | Spring Whitelabel / Tomcat 404 字节 / nginx 透传 404 / ASP.NET 黄页 | 应用服务器 |
| Cookie 名 | `.AspNetCore.Antiforgery.*`(.NET) / `JSESSIONID`(Java) / `laravel_session` / `PHPSESSID` / `rememberMe`(Shiro) | 框架 / Runtime |
| 响应头优先序 | `X-Powered-By: ASP.NET` 优先于被改写的 `Server` | 运行时 |
| 包名 / 服务名 / 栈帧 | `com.xxx.*` 包路径、Feign 服务名、微服务名 | 语言 + 厂商 |
| 组件专属端点回显 | Druid 监控页、Actuator JSON、Swagger UI HTML、`/ureport/designer/` 页面 | 组件确认 |
| 凭据 / 行为差 | 带 token 401 JSON vs 不带 token 500 HTML | 认证框架 |
| 文件后缀与 handler | `.ashx?method=&compress=`、`.do`、`.action` | 平台 |

### 1.3 判定规则

- **R1**：判定后端语言 / 框架，**至少 1 个芯信号**。
- **R2**：只有壳信号时，结论必须写成"**推测（未证实）**"，并列入待验证项。
- **R3**：前端栈与后端栈**分开记录**，不要合并成一个"技术栈"字段。
- **R4**：按**入口**（端口 / 子域 / URL 前缀）分别定栈——同一目标多后端并存是常态，禁止"整站单一技术栈"。

## 2. 易误报信号 → 反证动作

| 疑似信号 | 可能的错误结论 | 反证动作 |
|---|---|---|
| 扫描器 banner 报 XXL-JOB / Struts2 / Jenkins | 该产品存在 | 主动请求产品特征路径 + 验证 UI/接口回显；banner 单点不采信 |
| Spring 特征路径（/actuator、/druid） | 一定是 Spring Boot | 请求实际端点看回显；注意 Spring Boot 1.x 的 Actuator 在**根路径**；注意 ABP(.NET) 也有大 JSON 配置端点 |
| `/ureport/`、`/datasource/` | ThinkPHP（PHP） | ureport 是 **Java** 组件（bstek UReport2）；结合 `/ureport/designer/` 页面与 Java 报错确认 |
| 前端 Vue / uni-app SPA | 后端是 Java / 自研 | 触发错误回显、看响应头与 Cookie、找 API 错误格式 |
| Element UI / Ant Design Vue | 就是 RuoYi / 某中台 | RuoYi 需 `/prod-api/` + `/captchaImage` + Element UI **三件套**同时成立 |
| 大 JSON 配置端点 | Spring Actuator | 检查 `.AspNetCore.Antiforgery.*` cookie / `X-Powered-By: ASP.NET`（ABP ≠ Spring） |
| 某品牌名出现 | 该产品可达 | 区分"引用"与"可达"；逐个入口验证 |
| 界面像政务 / 园区自研平台 | 自研（B/D 类，走业务逻辑） | 找产品底座（标准化客户端 / 统一 API 前缀 / 厂商版权），可能是商业 SaaS 白标反代（A 类） |
| `Server: nginx` / `Server: elb` | 纯 nginx | 找 `X-Powered-By`、应用错误页；可能反代到 Tomcat/.NET 后端，或存在旁路端口 |
| 同域某子栈为 .NET | 全站 .NET | 逐子域 / 端口验证——同品牌常跨 .NET / Java / Go |

## 3. "看起来是 A 其实是 B"矩阵（历史已证实）

| 表面信号 | 错误结论 | 实际可能是 |
|---|---|---|
| Vue2 管理台 SPA | 后端 Java | ASP.NET (IIS) |
| uni-app / Vue H5 | 后端 Java | ASP.NET Core / Kestrel |
| Vue3 + chunk | 后端同栈 | Go 双服务 |
| Vue2 + Element UI | RuoYi | Spring MVC + JPA 等 |
| `/ureport/` 命中 | ThinkPHP | Java（UReport2） |
| 大配置 JSON 端点 | Spring Actuator | ABP(.NET) |
| 某停车 / 品牌家族名 | 家族同栈 | 家族横跨 .NET / Java |
| 政务 / 园区界面 | 自研 | 商业 SaaS 白标反代 |
| `Server` 头 | 后端栈 | 已被反代改写 |
| 大体积 71KB 配置 JSON | Spring Boot | 需按 Cookie 区分 ABP / Spring |

## 4. 输出规范

定栈结论必须写成"信号 → 置信度"的形式，禁止只给一个裸结论：

```
入口：<host:port / path 前缀>
- 前端（壳）：Vue2 + Element UI            [芯信号: 否] 证据：chunk 命名 + 组件 class
- 后端（芯）：Java / Spring Boot           [置信度: 高] 证据：/actuator/env 返回 JSON + 异常堆栈 com.xxx
- 数据库：MySQL                            [置信度: 中] 证据：SQL 报错语法
- 待验证：是否存在第二后端（.NET / 旁路端口）
```

## 5. 关联

- [system-categories.md](system-categories.md) — 系统形态（自研 / 二开 / 通用）与 A/B/C/D/E 分类（**分类前必须先完成定栈**）
- [framework-signatures.md](framework-signatures.md) — 框架信号库（本文件是它的使用前提）
- [deep-discovery.md](deep-discovery.md) — 关联路径 Fuzz
- [auth-fingerprint.md](auth-fingerprint.md) — 认证结构指纹
- memory 关联：用 `memory_search` 按特征检索——系统分类方法论（关键词：system-classification / 独立证据判定）、组件签名恢复（aspnet-handler-signature）、nginx 旁路绕过（tomcat-bypass-nginx）、框架默认配置（abp-framework）、指纹误报处理（fingerprint-false-positive）
