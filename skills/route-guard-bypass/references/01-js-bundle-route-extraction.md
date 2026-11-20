# JS bundle 路由与 API 端点提取 SOP

> 适用技能:route-guard-bypass / api-tester
> 适用场景:从 main-index.js / app.js / main.js 等主入口 JS 中提取所有 HTML 路由 + API 端点 + 鉴权关键字

---

## 一、HTML 路由提取(4 种模式)

### 模式 1:window.location.href 跳转

```bash
grep -oE "location\.href\s*=\s*['\"][^'\"]+\.html[^'\"]*" main-index.js
```

**典型输出**:
```
location.href = 'index_chengzhen_new.html?address=xibei'
location.href = 'index_device.html'
location.href = 'index_use_electricity.html'
location.href = 'index_nobodyMachinery.html'
location.href = 'login.html'
```

### 模式 2:window.open 跳转

```bash
grep -oE "window\.open\(['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js
```

### 模式 3:routes / pageMap / routeMap 数组

```bash
grep -nE "(routes|pageMap|routeMap|navMap)\s*[:=]\s*\\{?\\[" main-index.js

# 或 Python 正则
grep -oE "\[['\"][a-z_]+\.html[^'\"]*['\"]" main-index.js
```

**典型输出(恺易 main-index.js#L12)**:
```js
[
  ['liangyou', 'index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1'],
  ['shucai', 'index_chanye_new.html?type=shucai&PropertyClassValue=2'],
  ['shuiguo', 'index_chanye_new.html?type=shuiguo&PropertyClassValue=3'],
  ['huahui', 'index_chanye_new_zy.html?type=huahui&PropertyClassValue=4'],
  ...
]
```

### 模式 4:<a href> 链接(从 HTML 文件提取)

```bash
grep -oE "href=['\"][^'\"]+\.html[^'\"]*['\"]" *.html
```

**典型输出**:
```html
<a href="index_device.html">设备管理</a>
<a href="index_chengzhen.html">产业园</a>
```

### 提取 SOP(完整版)

```bash
# 1. 抓取主入口 JS
curl -s http://target/main-index.js -o main-index.js

# 2. 提取所有跳转目标
grep -oE "(location\.href|window\.open|href)\s*[=(]\s*['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js | \
  grep -oE "['\"][^'\"]+\.html[^'\"]*['\"]" | \
  tr -d "'\"" | sort -u > routes.txt

# 3. 去重 + 排序
sort -u routes.txt

# 4. 分类(大屏/IOT/产业/登录)
grep -E "(chengzhen|chanye|device|electricity|nobodyMachinery|login)" routes.txt
```

---

## 二、API 端点提取(6 种模式)

### 模式 1:url 字段硬编码

```bash
grep -oE "url\s*[:=]\s*['\"][^'\"]+['\"]" main-index.js | grep -E "(BI|api|data|chart|server)"
```

**典型输出(恺易 main-index.js)**:
```js
url: window.LocalServerUrl + 'BI/chanyechanzhi'
url: window.LocalServerUrl + 'BI/centermap'
url: window.LocalServerUrl + 'BI/jingyingtudizhanbi'
```

### 模式 2:$.ajax / $.post / $.get

```bash
grep -oE "\\\$\.(ajax|get|post|getJSON)\(['\"][^'\"]+['\"]" *.js
```

### 模式 3:fetch() 调用

```bash
grep -oE "fetch\(['\"][^'\"]+['\"]" *.js
```

### 模式 4:XMLHttpRequest.open

```bash
grep -oE "\.open\(['\"][A-Z]+['\"]\s*,\s*['\"][^'\"]+['\"]" *.js
```

### 模式 5:ServerUrl / BaseUrl / ApiUrl 域名

```bash
grep -nE "(ServerUrl|BaseUrl|ApiUrl|LocalServerUrl)\s*[:=]" *.js

# 典型输出
window.LocalServerUrl = 'http://36.156.143.17:8095/kyzhyq/'
```

### 模式 6:WebSocket / SSE 连接

```bash
grep -oE "(WebSocket|EventSource)\(['\"][^'\"]+['\"]" *.js
```

### 提取 SOP(完整版)

```bash
# 1. 提取所有 URL 字符串
grep -oE "['\"](https?:)?//[^'\"]+|['\"]/?[a-zA-Z][a-zA-Z0-9_/]*['\"]" main-index.js | \
  tr -d "'\"" | grep -vE "(css|js|jpg|png|gif|woff|svg)" | \
  grep -E "(BI|api|data|chart|server|/yw|/ys|/admin|/system)" > apis.txt

# 2. 去重 + 排序
sort -u apis.txt

# 3. 分类
echo "--- BI 端点 ---"
grep "BI/" api.txt
echo "--- API 端点 ---"
grep "/api" api.txt
echo "--- 后台端点 ---"
grep -E "(/admin|/system|/manage)" api.txt
```

---

## 三、鉴权关键字提取(5 类)

### 关键字 1:localStorage 检查

```bash
grep -nE "localStorage\.(get|set)Item" *.js | head -50
```

**典型输出**:
```js
localStorage.getItem('login_status') === '200'
localStorage.getItem('user_info_bi')
localStorage.setItem('token', ...)
```

### 关键字 2:Session 检查

```bash
grep -nE "(session|cookie)\.(get|set|read)" *.js
```

### 关键字 3:鉴权跳转

```bash
grep -nE "(location\.href|next\()\s*[\(=]\s*['\"][^'\"]*login" *.js
```

**典型输出**:
```js
window.location.href = '/login.html'
next('/login')
```

### 关键字 4:Token 验证

```bash
grep -nE "(verifyToken|validateToken|checkAuth|isAuthenticated|isLogin)" *.js
```

### 关键字 5:guard.js 独立文件

```bash
# 直接探测
find . -name "guard.js" -o -name "auth.js" -o -name "interceptor.js" -o -name "authGuard.js"

# 或 HTTP 探针
for path in js/guard.js js/auth.js js/interceptor.js js/authGuard.js; do
  status=$(curl -s -o /dev/null -w "%{http_code}" "http://target/$path")
  echo "$status  $path"
done
```

### 提取 SOP(完整版)

```bash
# 1. 提取所有鉴权相关行
grep -nE "(localStorage|cookie|session|token|auth|login|guard|interceptor|jwt)" main-index.js common.js guard.js 2>/dev/null | \
  grep -vE "(//|/\*|\*)" | head -100 > auth_patterns.txt

# 2. 重点关注
echo "--- 鉴权函数定义 ---"
grep -nE "function.*(isLogin|isAuth|guard|checkAuth)" auth_patterns.txt

echo "--- 鉴权检查调用 ---"
grep -nE "(isLogin|isAuth|guard)\(\)" auth_patterns.txt

echo "--- 鉴权跳转 ---"
grep -nE "location\.(href|replace)" auth_patterns.txt
```

---

## 四、完整提取模板(实战推荐)

```bash
#!/bin/bash
# extract_routes.sh - JS bundle 路由/API/鉴权 提取器
TARGET="$1"
if [ -z "$TARGET" ]; then
  echo "Usage: $0 <target-domain>"
  exit 1
fi

WORK_DIR="route-guard-extract"
mkdir -p $WORK_DIR
cd $WORK_DIR

# 1. 抓取主入口 JS
echo "[*] 抓取主入口 JS..."
for js in main-index.js app.js main.js common.js common-full.js guard.js auth.js; do
  curl -s "http://$TARGET/js/$js" -o "$js" 2>/dev/null
  if [ -s "$js" ]; then
    echo "  ✓ $js ($(wc -c < $js) bytes)"
  else
    rm -f "$js"
  fi
done

# 2. 提取 HTML 路由
echo "[*] 提取 HTML 路由..."
grep -hoE "(location\.href|window\.open|href)\s*[=(]\s*['\"][^'\"]+\.html[^'\"]*['\"]" *.js 2>/dev/null | \
  grep -oE "['\"][^'\"]+\.html[^'\"]*['\"]" | tr -d "'\"" | sort -u > routes.txt
echo "  ✓ $(wc -l < routes.txt) 个 HTML 路由"

# 3. 提取 API 端点
echo "[*] 提取 API 端点..."
grep -hoE "['\"](https?:)?//[^'\"]+['\"]|['\"]/?[a-zA-Z][a-zA-Z0-9_/]*['\"]" *.js 2>/dev/null | \
  tr -d "'\"" | grep -vE "\.(css|js|jpg|png|gif|woff|svg)" | \
  grep -E "(BI|api|data|chart|server|/yw|/ys|/admin|/system)" | sort -u > apis.txt
echo "  ✓ $(wc -l < apis.txt) 个 API 端点"

# 4. 提取鉴权关键字
echo "[*] 提取鉴权关键字..."
grep -hnE "(isLogin|login_status|guard\(|router\.beforeEach|next\(['\"]login|location\.href.*login)" *.js 2>/dev/null | sort -u > auth_patterns.txt
echo "  ✓ $(wc -l < auth_patterns.txt) 个鉴权模式"

# 5. 输出汇总
echo ""
echo "===== HTML 路由 (前 30) ====="
head -30 routes.txt
echo ""
echo "===== API 端点 (前 30) ====="
head -30 apis.txt
echo ""
echo "===== 鉴权模式 (前 30) ====="
head -30 auth_patterns.txt
```

**使用方法**:
```bash
chmod +x extract_routes.sh
./extract_routes.sh www.xishanagripark.com
```

---

## 五、实战案例(锡山区农业平台 Round 18)

**输入**:http://www.xishanagripark.com

**Step 1 — 抓取 JS**:
```bash
curl -s http://www.xishanagripark.com/js/main-index.js -o main-index.js  # 45KB
curl -s http://www.xishanagripark.com/js/guard.js -o guard.js             # 1KB
curl -s http://www.xishanagripark.com/js/common.js -o common.js           # 93KB
curl -s http://www.xishanagripark.com/js/common-full.js -o common-full.js # 108KB
```

**Step 2 — HTML 路由提取**:
```bash
grep -hoE "location\.href\s*=\s*['\"][^'\"]+\.html[^'\"]*" main-index.js
```

**输出(14 个)**:
```
'index_chengzhen_new.html?address=xibei'
'index_chengzhen_new.html?address=donggang'
'index_chengzhen_new.html?address=yangjian'
'index_chengzhen_new.html?address=ehu'
'index_chengzhen_new.html?address=xidongyuwenhua'
'index_device.html'                          # IOT
'index_use_electricity.html'                 # 用电
'index_nobodyMachinery.html'                 # 无人机械
'index_chanye_new_ly.html?type=liangyou&PropertyClassValue=1'   # 粮油
'index_chanye_new.html?type=shucai&PropertyClassValue=2'        # 蔬菜
'index_chanye_new.html?type=shuiguo&PropertyClassValue=3'        # 茶果
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=4'     # 花卉
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=5'     # 水产
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=6'     # 种业
'index_chanye_new_zy.html?type=huahui&PropertyClassValue=7'     # 中药
```

**Step 3 — 鉴权关键字提取**:
```bash
grep -hnE "(isLogin|login_status|guard\()" guard.js main-index.js
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

**Step 4 — API 端点提取**:
```bash
grep -hoE "url\s*[:=]\s*['\"][^'\"]+['\"]" main-index.js | grep "BI/"
```

**输出(6 个)**:
```
'BI/chanyechanzhi'
'BI/centermap'
'BI/chanyemianji'
'BI/chanyechanliang'
'BI/jingyingtudizhanbi'
'BI/jingyingzhuti'
```

**Step 5 — HTTP 批量探针**:
```bash
for html in $(grep -oE "['\"][^'\"]+\.html[^'\"]*['\"]" main-index.js | tr -d "'\"" | sort -u); do
  status=$(curl -s -o /dev/null -w "%{http_code}" "http://www.xishanagripark.com/$html")
  echo "$status  $html"
done
```

**输出**:全部 200 OK(其中 index_chengzhen.html 旧版 404,但其他 14 个全部可访问)

**关键发现 — HTTP 200 OK 但浏览器会跳转到 login.html** = **完美绕过点**

---

## 六、命令速查

| 任务 | 命令 |
|---|---|
| HTML 路由 | `grep -hoE "location\.href\s*=\s*['\"][^'\"]+\.html[^'\"]*" *.js` |
| API 端点(BI) | `grep -hoE "url\s*[:=]\s*['\"][^'\"]+['\"]" *.js \| grep "BI/"` |
| localStorage 检查 | `grep -nE "localStorage\.(get\|set)Item" *.js` |
| 鉴权跳转 | `grep -nE "location\.href.*login\|next\(['\"]login" *.js` |
| guard.js 探测 | `for f in js/guard.js js/auth.js; do curl -sI http://target/$f; done` |
| ServerUrl 域名 | `grep -nE "(ServerUrl\|BaseUrl\|ApiUrl)\s*[:=]" *.js` |
| 完整提取模板 | `./extract_routes.sh www.target.com` |