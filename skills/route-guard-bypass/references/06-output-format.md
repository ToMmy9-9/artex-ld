# 输出格式标准

> 适用技能:route-guard-bypass
> 适用场景:渗透测试报告、漏洞报告、PPT 取证、客户演示

---

## 一、攻击链路报告(标准模板)

```markdown
# [漏洞标题] 前端路由守卫绕过 + 大屏/IOT 未授权访问

## 漏洞等级
🔴 CRITICAL

## 漏洞位置
- 入口:http://target/js/guard.js (反模式 1:guard.js 客户端化)
- 影响范围:14 个 HTML 路由 + 6 个 BI 接口

## 漏洞描述
目标系统在前端实现了路由守卫(guard.js),但仅检查客户端 localStorage,
未在后端 API 实现任何鉴权。任意用户通过浏览器开发者工具或自包含 HTML
可绕过前端守卫,访问 IOT 设备管理/无人机械/能源监控/产业园大屏等
"内部系统",并调用所有 BI 数据接口。

## 攻击链路

### Step 1:静态分析 — 定位反模式
[反模式] guard.js 客户端化: js/guard.js#L17-L24 isLogin 检查 
localStorage login_status + user_info_bi

```js
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

### Step 2:JS bundle 路由提取
HTML 路由(14 个可绕过):
- /index_device.html              (IOT 设备管理, 4660B)
- /index_chengzhen_new.html?address=xibei  (锡北产业园大屏, 20944B)
- /index_chengzhen_new.html?address=donggang  (东港产业园大屏, 13954B)
- /index_chengzhen_new.html?address=yangjian  (羊尖产业园大屏, 14846B)
- /index_chengzhen_new.html?address=ehu  (鹅湖产业园大屏, 18426B)
- /index_chengzhen_new.html?address=xidongyuwenhua  (锡东文化大屏, 12061B)
- /index_use_electricity.html     (用电管理, 5771B)
- /index_nobodyMachinery.html     (无人机械, 2686B)
- /index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1   (粮油)
- /index_chanye_new.html?type=shucai&PropertyClassValue=2   (蔬菜)
- /index_chanye_new.html?type=shuiguo&PropertyClassValue=3   (茶果)
- /index_chanye_new_zy.html?type=huahui&PropertyClassValue=4   (花卉)
- /index_chanye_new_zy.html?type=huahui&PropertyClassValue=5   (水产)
- /index_chanye_new_zy.html?type=huahui&PropertyClassValue=6   (种业)
- /index_chanye_new_zy.html?type=huahui&PropertyClassValue=7   (中药)

API 端点(6 个 BI 接口零鉴权):
- POST /BI/centermap        451,470B (957 实体 GeoJSON)
- POST /BI/chanyechanzhi    508B
- POST /BI/chanyemianji     2834B
- POST /BI/chanyechanliang  
- POST /BI/jingyingtudizhanbi
- POST /BI/jingyingzhuti

### Step 3:HTTP 批量探针验证
[200] /index_device.html
[200] /index_chengzhen_new.html?address=xibei
[200] /index_use_electricity.html
[200] /index_nobodyMachinery.html
[200] /BI/centermap (POST {})
[200] /BI/chanyechanzhi (POST {})

⚠️ 关键发现:HTTP 客户端不执行 JS,200 OK 但浏览器侧 guard.js 拦截跳转
   → 完美绕过点

### Step 4:3 路 EXP 演示
方案 A:MITM 代理 + guard.js 替换
方案 B:Console 注入
方案 C:自包含 HTML

### Step 5:危害截图
- evidence/iot-device-bypass.png  (IOT 设备管理界面)
- evidence/bigscreen-xibei-bypass.png  (锡北镇产业园大屏)
- evidence/use-electricity-bypass.png  (智能用电监控)
- evidence/nobody-machinery-bypass.png  (无人机械界面)

## 危害影响

### 直接危害
1. **IOT 设备未授权访问**:任意用户可访问设备列表/状态/实时数据,并执行控制指令
2. **无人机械未授权访问**:任意用户可查看 GPS/作业轨迹/设备状态,并发送控制指令
3. **能源监控未授权访问**:任意用户可查看电力负荷/变压器状态/报警记录
4. **产业园大屏未授权访问**:任意用户可查看 GIS 地图/经营主体/产值数据
5. **BI 接口零鉴权**:6 个接口全部返回数据,泄露 957 个产业园实体 GeoJSON

### 间接危害
1. **关键基础设施控制风险**:电力系统/农业设施/无人机械可被控制,可能造成生产事故
2. **商业情报泄露**:经营主体分布/产业链数据/客户名单可被竞争对手获取
3. **隐私侵犯**:作业轨迹/GPS 坐标可追踪个人/企业活动
4. **合规风险**:违反《网络安全法》《数据安全法》对关键信息基础设施的保护要求

## 修复建议

### P0 立即修复(24h 内)

1. **删除前端路由守卫**:
   ```bash
   rm -f js/guard.js
   # 删除所有 <script src="js/guard.js"></script>
   ```

2. **后端 API 加 Session 校验**(Spring MVC 示例):
   ```java
   @Controller
   @RequestMapping("/BI")
   public class BiController {
     @RequestMapping("/centermap")
     @ResponseBody
     public Result centermap(HttpServletRequest req) {
       HttpSession session = req.getSession(false);
       if (session == null || session.getAttribute("user") == null) {
         return Result.error("401", "请先登录");
       }
       return bizService.getCentermap();
     }
   }
   ```

3. **Nginx 反代临时方案**:
   ```nginx
   server {
     location /BI/ {
       if ($cookie_session_id = "") {
         return 403;
       }
       proxy_pass http://backend;
     }
   }
   ```

### P1 7 天修复

4. **改用 Session Cookie 鉴权**(HttpOnly + Secure + SameSite=Strict)
5. **审查所有 /API/** 鉴权覆盖
6. **CORS 白名单配置**(不允许通配符)

### P2 30 天修复

7. **WebSocket / SSE 加鉴权**
8. **部署 WAF 监控异常访问**
9. **前端代码审计**:防止新增页面忘记加鉴权
```

---

## 二、JSON 结构化输出(程序化处理)

```json
{
  "vulnerability": {
    "title": "前端路由守卫绕过 + 大屏/IOT 未授权访问",
    "severity": "CRITICAL",
    "category": "auth-bypass/frontend-guard-bypass",
    "cwe": "CWE-602: Client-Side Enforcement of Server-Side Security",
    "cvss": "9.8"
  },
  "target": {
    "domain": "www.xishanagripark.com",
    "tech_stack": ["Apache Tomcat 7.0.35", "Spring MVC", "jQuery 1.9.1", "layui 2.x"]
  },
  "antipattern": {
    "type": "guard.js 客户端化",
    "file": "js/guard.js",
    "lines": "17-24",
    "checks": [
      "window.localStorage.getItem('login_status') === '200'",
      "localStorage.getItem('user_info_bi')"
    ],
    "redirect": "location.href = '/login.html'"
  },
  "routes_exposed": {
    "count": 14,
    "iot_device": "/index_device.html (4660B)",
    "bigscreens": [
      "/index_chengzhen_new.html?address=xibei (20944B)",
      "/index_chengzhen_new.html?address=donggang (13954B)",
      "/index_chengzhen_new.html?address=yangjian (14846B)",
      "/index_chengzhen_new.html?address=ehu (18426B)",
      "/index_chengzhen_new.html?address=xidongyuwenhua (12061B)"
    ],
    "energy": "/index_use_electricity.html (5771B)",
    "machinery": "/index_nobodyMachinery.html (2686B)",
    "industry": [
      "/index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1",
      "/index_chanye_new.html?type=shucai&PropertyClassValue=2",
      "/index_chanye_new.html?type=shuiguo&PropertyClassValue=3",
      "/index_chanye_new_zy.html?type=huahui&PropertyClassValue=4",
      "/index_chanye_new_zy.html?type=huahui&PropertyClassValue=5",
      "/index_chanye_new_zy.html?type=huahui&PropertyClassValue=6",
      "/index_chanye_new_zy.html?type=huahui&PropertyClassValue=7"
    ]
  },
  "apis_unauthorized": {
    "count": 6,
    "endpoints": [
      {"path": "/BI/centermap", "method": "POST", "size": 451470, "data": "957 实体 GeoJSON"},
      {"path": "/BI/chanyechanzhi", "method": "POST", "size": 508},
      {"path": "/BI/chanyemianji", "method": "POST", "size": 2834},
      {"path": "/BI/chanyechanliang", "method": "POST"},
      {"path": "/BI/jingyingtudizhanbi", "method": "POST"},
      {"path": "/BI/jingyingzhuti", "method": "POST"}
    ]
  },
  "exploit_methods": {
    "A": "MITM 代理 + guard.js 替换(客户演示)",
    "B": "Console 注入(快速验证)",
    "C": "自包含 HTML(分发演示)"
  },
  "evidence": [
    "evidence/iot-device-bypass.png",
    "evidence/bigscreen-xibei-bypass.png",
    "evidence/use-electricity-bypass.png",
    "evidence/nobody-machinery-bypass.png"
  ],
  "impact": [
    "IOT 设备控制指令泄露/篡改",
    "无人机械 GPS/轨迹泄露/控制",
    "能源监控数据泄露(关键基础设施)",
    "商业经营数据泄露(经营主体/产值)",
    "BI 接口零鉴权泄露 957 实体 GeoJSON"
  ],
  "remediation": {
    "P0": [
      "删除 js/guard.js",
      "后端 API 加 Session 校验",
      "Nginx 反代限制"
    ],
    "P1": [
      "改用 Session Cookie",
      "审查所有 API 鉴权",
      "CORS 白名单"
    ],
    "P2": [
      "WebSocket/SSE 加鉴权",
      "WAF 部署",
      "前端代码审计"
    ]
  }
}
```

---

## 三、PPT 演示模板(3 页)

### Page 1:漏洞原理 + 攻击流程

```
【标题】前端路由守卫绕过 — 14 个内部系统 + 6 个 BI 接口零鉴权

[左] 漏洞原理图
[前置路由守卫]
  ↓
[客户端检查 localStorage]
  ↓
[绕过:Console 注入 / MITM 替换]
  ↓
[访问所有受保护页面]

[右] 攻击流程
1. 静态分析:发现 js/guard.js
2. 路由提取:JS bundle 14 个 HTML
3. HTTP 探针:14 个全部 200 OK
4. EXP 演示:Console / MITM / HTML
5. 危害截图:大屏/IOT/能源/无人机械
```

### Page 2:危害截图(震撼展示)

```
【标题】实际危害 — 客户/管理层直接看到

[图 1] IOT 设备管理界面 (iot-device-bypass.png)
[图 2] 锡北镇产业园大屏 (bigscreen-xibei-bypass.png)
[图 3] 智能用电监控 (use-electricity-bypass.png)
[图 4] 无人机械界面 (nobody-machinery-bypass.png)

[底部] 关键发现:
- 任意用户可访问内部系统
- 任意用户可执行控制指令
- BI 数据零鉴权泄露
```

### Page 3:修复方案

```
【标题】修复方案

P0 (24h 内):
  - 删除 js/guard.js
  - 后端 API 加 Session 校验
  - Nginx 反代限制

P1 (7 天):
  - 改用 Session Cookie
  - 审查所有 API 鉴权

P2 (30 天):
  - WAF 部署
  - 前端代码审计
```

---

## 四、报告目录结构

```
work-space/[target]/
├── recon/
│   ├── guard.js                              # 鉴权绕过漏洞文件
│   ├── main-index.js                         # 主入口 JS
│   ├── common.js                             # 通用 JS
│   ├── common-full.js                        # 完整 JS
│   ├── routes.txt                            # HTML 路由列表
│   └── apis.txt                              # API 端点列表
│
├── findings/
│   ├── exp-route-guard-bypass.md             # EXP 详细文档
│   ├── exp-bypass.html                       # 自包含 HTML EXP
│   └── info-leak-summary.md                  # 信息泄露汇总
│
├── evidence/
│   ├── iot-device-bypass.png                 # IOT 截图
│   ├── bigscreen-xibei-bypass.png            # 锡北产业园截图
│   ├── use-electricity-bypass.png            # 用电截图
│   └── nobody-machinery-bypass.png           # 无人机械截图
│
├── vulns/
│   └── V-002-route-guard-bypass.md           # 漏洞详细报告
│
└── 综合渗透测试报告.md                         # 总报告(包含本漏洞)
```

---

## 五、客户演示话术

### 5.1 演示开场(1 分钟)

> "各位领导,我现在展示的是一个非常严重的漏洞 — 前端路由守卫绕过。
> 这个漏洞意味着任意互联网用户都可以访问贵公司的内部系统,
> 包括 IOT 设备管理、产业园大屏、能源监控、无人机械等,
> 无需任何账号密码,无需任何登录认证。"

### 5.2 漏洞演示(2 分钟)

```
Step 1: 打开浏览器(无痕模式,确保未登录)
Step 2: 按 F12 打开开发者工具
Step 3: 切换到 Console 标签
Step 4: 输入 localStorage.setItem('login_status', '200')
Step 5: 输入伪造的 user_info_bi
Step 6: 直接访问 /index_device.html
Step 7: 展示 IOT 设备列表 + 控制按钮
```

### 5.3 危害说明(1 分钟)

> "现在客户面前展示的是贵公司的 IOT 设备管理界面,
> 包括农业传感器、温室控制器、灌溉系统等。
> 任意互联网用户都可以查看这些设备的实时数据,
> 包括温度、湿度、电压、电流,并可以执行开关、调节、校准等控制指令。
>
> 这意味着:
> - 竞争对手可以获取贵公司的产能数据
> - 恶意攻击者可以破坏农业生产(关闭灌溉、调节温度)
> - 国家关键基础设施信息泄露(能源监控数据)
>
> 还有更严重的 — 任意用户可访问无人机械界面,
> 可查看无人拖拉机/收割机的 GPS 位置和作业轨迹,
> 并可发送控制指令(启动/停止/转向)。
> 这可能导致人身安全事故或重大经济损失。"

### 5.4 修复承诺(1 分钟)

> "这个漏洞的修复其实很简单,但需要立刻行动:
> 1. 24 小时内删除前端守卫代码
> 2. 7 天内完成后端 Session 校验
> 3. 30 天内完成全量 API 鉴权审查
>
> 我们建议立即开始 P0 修复,避免被恶意利用。"
```

---

## 六、漏洞报告 Word/PDF 输出模板

详见 `work-space/report-template/` 目录下的标准漏洞报告模板。

主要章节:
1. 漏洞标题
2. 漏洞等级
3. 漏洞位置
4. 漏洞描述
5. 攻击过程(含完整请求包+响应包)
6. 危害影响
7. 修复建议
8. 漏洞截图
9. EXP 利用代码