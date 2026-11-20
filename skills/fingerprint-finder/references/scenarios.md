# 7 个智能工作场景

> 本文件是 [workflow-stages.md](workflow-stages.md) 中"四、7 智能场景调度"的可执行展开。
> 7 个场景的调度模板全部展开，agent 可直接复制执行。

> ⚠️ **`memory-search-mcp` 在每个场景中按触发条件动态调用**（参见 [workflow-stages.md §一](workflow-stages.md#一memory-动态加载触发点)）。
> 以下每个场景中的步骤只是该场景的主干流程，memory 检索由触发的特征决定，不由步骤顺序决定。
> 测试过程中发现独立子系统入口时，走 [system-category-routing.md §子目标发现与路由](system-category-routing.md#子目标发现与路由) 独立编排。

---

## 场景 1：完整渗透测试

**触发条件**：
- **用户输入关键词**：完整 / 全量 / 综合测试 / 全面渗透 / 默认（无明确场景匹配时）
- **fingerprint 输出驱动**：B 类 / C 类 / 无法判定时自动回退；A 类（独立中间件/运维工具）时作为中间件专项子路径
- **默认回退**：所有场景无法匹配时，回退到场景 1

```
用户："对 example.com 进行完整的渗透测试"
主编排器：
1. [fingerprint-finder] → 指纹识别 + 系统分类判定 A/B/C/D/E
   ↳ 识别出 Spring Boot → 触发 memory_get_methodology(category="02_java")
   ↳ 识别出 edu 行业 → 触发 memory_get_industry_case(tag="edu")
   ↳ 识别出"用友 NC" → 触发 [poc-scan] 匹配用友 POC
   ↓
【分类检查点】→ 按 A/B/C/D/E 分支：
   │
   ├─ 【A 类（成熟商业产品）】
   │  2. [poc-scan] ⭐ 硬钩子 → 多角度匹配 POC
   │  3. [sensitive-info-leak] → 扫描 JS / 配置文件提取敏感信息
   │  4. [api-tester] → 仅凭据获取环节（跳过深度测试）
   │  5. 轻量业务逻辑兜底（用户列表/数据导出/密码重置/支付/管理后台）
   │  6. 综合报告生成
   │
   ├─ 【B 类（自研含组件）】
   │  2. [sensitive-info-leak] → 扫描 JS / 配置文件提取敏感信息
   │  3. [api-tester] → 接口发现 + 自研认证矩阵 a-i + 凭据获取
   │  4. [poc-scan] ⭐ 软触发 → 仅嵌入组件识别后触发硬钩子
   │  5. [api-tester] → 越权和逻辑漏洞测试
   │  6. [sqli-tester] → 数据库接口时触发
   │  7. [response-modifier] → 响应篡改（登录绕过/验证码劫持等）
   │  8. 综合报告生成
   │
   ├─ 【C 类（开源二开）】
   │  2. [poc-scan] ⭐ 硬钩子 → 原项目 CVE POC 匹配
   │  3. [sensitive-info-leak] → 扫描敏感信息
   │  4. [api-tester] → 聚焦二开代码业务逻辑 + 凭据获取
   │  5. [sqli-tester] + [response-modifier] → 视情况
   │  6. 综合报告生成
   │
   ├─ 【D 类（纯自研无组件）】
   │  2. [sensitive-info-leak] → 扫描 JS / 配置文件提取敏感信息
   │  3. [api-tester] → 全流程：模块穷举 + 鉴权 + 越权 + 逻辑漏洞
   │  4. [sqli-tester] → 数据库接口时触发
   │  5. [response-modifier] → 响应篡改
   │  6. 综合报告生成
   │
   └─ 【E 类（混合架构）】
      按主导部分归入对应分支，附属部分轻量执行（≤30 分钟）

【子目标发现检查点】→ 贯穿所有分支，测试过程中若发现独立子系统入口：
   ↳ api-tester 接口发现 / sensitive-info-leak JS 分析时触发
   ↳ 判断标准：不同技术栈 / 不同响应头 / 独立认证入口（≥2 项）
   ↳ 触发后走 [system-category-routing.md §子目标发现与路由](system-category-routing.md#子目标发现与路由)
```

---

## 场景 2：快速漏洞扫描（⭐ 含 POC 库匹配）

**触发条件**：
- **用户输入关键词**：快速 / 常见漏洞 / 快速扫 / 轻量扫描
- **fingerprint 输出驱动**：A 类（已知商业产品）、C 类（已知开源二开）→ 快速 POC 扫描；D 类 → 跳过 POC；B 类 → 仅组件 POC
- **适用场景**：已知目标性质、快速验证已知漏洞

```
用户："快速扫描 example.com 的常见漏洞"
主编排器：
1. [fingerprint-finder] → 快速指纹识别 + 系统分类判定 A/B/C/D/E
   ↳ 触发 memory_get_methodology(category=对应技术栈分类)
   ↓
【分类检查点】→ 按 A/B/C/D/E 分支：
   │
   ├─ 【A 类 / C 类】
   │  2. [poc-scan] ⭐ 硬钩子 → 快速匹配指纹对应的已知漏洞 POC
   │  3. [sqli-tester] → SQL 注入快速检测
   │  4. [api-tester] → 关键接口测试（A类仅凭据获取）
   │  5. 生成漏洞报告（含 POC 命中清单）
   │
   ├─ 【B 类】
   │  2. [poc-scan] ⭐ 软触发 → 仅组件识别后触发
   │  3. [api-tester] → 接口快速发现 + 凭据获取
   │  4. [sqli-tester] → SQL 注入快速检测
   │  5. 生成漏洞报告
   │
   └─ 【D 类】
      2. [api-tester] → 接口快速发现 + 凭据获取（跳过 poc-scan）
      3. [sqli-tester] → SQL 注入快速检测
      4. 生成漏洞报告
```

---

## 场景 3：定向漏洞测试

**触发条件**：
- **用户输入关键词**：SQL 注入 / XXE / SSRF / 命令执行 / 定向 / 某个漏洞类型
- **fingerprint 输出驱动**：fingerprint-finder 识别出已知漏洞组件（如 struts2、log4j、fastjson）→ 自动定向到该类漏洞测试
- **调用方式**：用户指定漏洞类型 → 直接调对应 skill；指纹驱动 → fingerprint-finder 输出带漏洞类型推荐

```
用户："测试 example.com 的 SQL 注入漏洞"
主编排器：
1. [fingerprint-finder] → 快速识别数据库类型 + 系统分类判定
   ↳ 触发 memory_get_methodology(category="06_vuln")
   ↓
【分类检查点】→ 按 A/B/C/D/E 分支：
   │
   ├─ 【A 类 / C 类】
   │  2. [poc-scan] ⭐ → 检索 SQL 注入类 POC（如 yonyou sqli POC）
   │  3. [sqli-tester] → 针对性 SQL 注入测试（结合 POC + 自动化）
   │
   ├─ 【B 类】
   │  2. [poc-scan] ⭐ 软触发 → 仅组件识别后触发
   │  3. [sqli-tester] → 针对性 SQL 注入测试
   │
   └─ 【D 类】
      2. [sqli-tester] → 针对性 SQL 注入测试（跳过 poc-scan）

4. 如发现注入 → 数据提取
5. 生成漏洞报告
```

---

## 场景 4：API 安全测试

**触发条件**：
- **用户输入关键词**：API 接口测试 / 接口安全 / API 安全 / 接口测试
- **fingerprint 输出驱动**：D 类（纯自研无组件）→ 默认推荐场景 4；B 类（自研含组件）→ api-tester 作为并行线之一
- **子目标发现**：接口发现过程中若发现独立子系统入口 → 走子目标路由

```
用户："测试 example.com 的 API 接口安全性"
主编排器：
1. [fingerprint-finder] → 识别 API 框架 + 系统分类判定
   ↳ 触发 memory_get_methodology(category=对应分类)
   ↓
【分类检查点】→ 按 A/B/C/D/E 分支：
   │
   ├─ 【A 类】
   │  2. [api-tester] → 仅凭据获取（注册流程利用）
   │  3. 生成 API 安全报告
   │
   ├─ 【B 类 / D 类】
   │  2. [api-tester] → 接口发现
   │  3. [api-tester] → JS 逆向分析
   │  4. [api-tester] → 凭据获取
   │  5. [api-tester] → 越权和逻辑漏洞测试
   │     ↳ 触发 memory_get_methodology(category="07_auth" / "08_logic")
   │  6. 【子目标发现检查点】→ 接口发现 / JS 分析中若发现独立子系统入口
   │     ↳ 不同 API 路径前缀 / 不同认证入口 / 不同技术栈
   │     ↳ 触发后走 [system-category-routing.md §子目标发现与路由](system-category-routing.md#子目标发现与路由)
   │  7. 生成 API 安全报告
   │
   └─ 【C 类】
      2. [api-tester] → 接口发现
      3. [api-tester] → JS 逆向分析
      4. [api-tester] → 凭据获取
      5. [api-tester] → 聚焦二开代码业务逻辑
      6. 【子目标发现检查点】→ 接口发现 / JS 分析中若发现独立子系统入口
         ↳ 触发后走 [system-category-routing.md §子目标发现与路由](system-category-routing.md#子目标发现与路由)
      7. 生成 API 安全报告
```

---

## 场景 5：深度指纹探测 + 商业系统 POC 匹配

**触发条件**：
- **用户输入关键词**：深度探测 / 深度指纹 / 二开系统指纹 / 识别什么系统
- **fingerprint 输出驱动**：A 类产品名模糊（如"类似用友但不确定"）→ 深度指纹确认；首次 fingerprint-finder 判定证据不足 → 自动升级到场景 5
- **适用场景**：目标指纹不明确、需要深度 Fuzz 确认、二开系统组件发现

```
用户："深度探测 example.com 的指纹，特别是二次开发系统"
主编排器：
1. [fingerprint-finder] → 基础指纹识别 + 系统分类判定
   ↓
【分类检查点】→ 按 A/B/C/D/E 分支：
   │
   ├─ 【A 类 / C 类】
   │  2. [api-tester] → JS 文件分析
   │  3. [fingerprint-finder] → 基于框架特征的深度 Fuzz
   │  4. [fingerprint-finder] → 接口指纹挖掘
   │  5. [poc-scan] ⭐ 硬钩子 → 根据深度指纹匹配 POC
   │  6. 生成深度指纹 + POC 命中报告
   │
   └─ 【B 类 / D 类】
      2. [api-tester] → JS 文件分析
      3. [fingerprint-finder] → 基于框架特征的深度 Fuzz
      4. [fingerprint-finder] → 接口指纹挖掘
      5. [poc-scan] ⭐ → 仅 B 类组件识别后触发（D 类跳过）
      6. 【子目标发现检查点】→ 深度 Fuzz / JS 分析中若发现独立子系统入口
         ↳ 触发后走 [system-category-routing.md §子目标发现与路由](system-category-routing.md#子目标发现与路由)
      7. 生成深度指纹报告
```

---

## 场景 6：商业 OA 系统定向测试（⭐ poc-scan 主战场）

**触发条件**：
- **用户输入关键词**：用友 / 致远 / 泛微 / 通达 / 万户 / 金蝶 / 蓝凌 / 商业 OA / OA 测试
- **fingerprint 输出驱动**：fingerprint-finder 识别出商业 OA 产品名（用友 NC、致远 A8、泛微 e-cology 等）→ 自动切换到场景 6
- **自动触发**：即使输入是"完整渗透测试"，若 fingerprint-finder 判定为 A 类 + 命中商业产品名 → 自动重定向到场景 6

```
用户："用友 NC / 致远 OA / 泛微 e-cology 定向测试"
主编排器：
1. [fingerprint-finder] → 确认系统（用友 NC / 致远 / 泛微 ...）
   → 用户已指定目标为商业 OA，隐含 A 类判定，无需分支
2. [poc-scan] ⭐ → 多角度匹配 POC（grep 只读语料，不跑脚本）：
   ↳ rg -l -i --no-ignore -e "用友" -e "yonyou" POC/
   ↳ rg -l -i --no-ignore -e "致远" -e "seeyon" POC/
   ↳ rg -l -i --no-ignore -e "泛微" -e "weaver" POC/
   ↳ 命中后 Read 正文确认版本/前提；oa/ 与 custom/ 子目录优先看
3. 若有 WAF（阿里云等）→ waf-adapt.md 改造 payload
4. [yakit MCP http_fuzzer] → 执行 POC（走 127.0.0.1:8080 MITM）
5. 命中 → work-space/<target>/findings/<system>-<cve>.md
6. [sqli-tester / api-tester] → 后续深度利用
```

---

## 6 场景速查表（agent 调度用）

| 场景 | 用户大致输入 | 触发条件（指纹驱动） | 主调用 skill | 分类分支 | 子目标发现 |
|---|---|---|---|---|---|
| 1. 完整渗透测试 | "完整 / 全量 / 综合测试 xxx.com" | **默认回退**：B类/C类/无法判定时自动回退；A类中间件时走POC主路径 | 全部 | fingerprint → **分类检查点** → A类(poc-scan主路径) / B类(双线并行) / C类(poc-scan硬钩子) / D类(业务逻辑主路径) / E类(按主导归并) | 是（贯穿全程） |
| 2. 快速漏洞扫描 | "快速扫一下常见漏洞" | A类/C类 → 快速POC；D类 → 跳过POC；B类 → 仅组件POC | fingerprint + poc-scan + sqli + api | fingerprint → **分类检查点** → A/C类(poc-scan硬钩子) / B类(软触发) / D类(跳过poc-scan) | 低概率 |
| 3. 定向漏洞测试 | "测一下 SQL 注入 / XXE / SSRF" | fingerprint识别出已知漏洞组件(struts2/log4j)→自动定向 | sqli-tester（专项）| fingerprint → **分类检查点** → A/C类(poc-scan+sqli) / B类(软触发+sqli) / D类(直接sqli) | 低概率 |
| 4. API 安全测试 | "API 接口测试 / 接口安全" | **D类（纯自研）→ 默认推荐**；B类 → api-tester并行线 | api-tester | fingerprint → **分类检查点** → A类(仅凭据获取) / B/D类(全流程) / C类(聚焦二开逻辑) | **是**（接口发现时触发） |
| 5. 深度指纹 + 商业系统 | "深度探测 / 二开系统指纹" | A类产品名模糊 → 深度确认；首次fingerprint证据不足 → 自动升级 | fingerprint 深度 + poc-scan | fingerprint → **分类检查点** → A/C类(深度指纹+poc-scan) / B/D类(深度指纹，仅B类组件命中时poc-scan) | **是**（深度Fuzz时触发） |
| 6. 商业 OA 定向 | "用友 / 致远 / 泛微 / 通达 定向测试" | **指纹命中商业OA产品名 → 自动切换**（即使输入是场景1） | poc-scan 主战场 | 用户已指定 → 隐含 A 类，直接走 poc-scan 主路径 | 低概率 |

> 用户输入超出 6 个场景时，回退到 **场景 1（完整渗透测试）** 作为默认入口。