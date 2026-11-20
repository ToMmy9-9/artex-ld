# 认证结构指纹库（自研系统专项）

> 自研系统通常没有可识别的版本号和路径指纹，**唯一可靠的指纹就是认证流本身**。
> 一旦识别出认证结构，对应的绕过测试入口就出来了。
> 本文件只负责**识别信号采集**，**验证执行**归 `api-tester`。
> 本文件是 SKILL.md 中"系统类别视角 → 1. 自研系统"的可执行展开。

---

## a. 接口未授权指纹

**识别信号**
- 浏览器访问业务接口，去掉 `Cookie` / `Authorization` 头后仍返回 `200` + 数据
- yakit `query_http_flow` 过滤"200 且请求头无鉴权字段"的请求
- 在未登录状态下，直接复制别人页面触发的 API URL

**典型表现**
```
GET /api/user/profile            → 200 + JSON（无 Cookie）
GET /api/admin/list              → 200 + 明文数据
GET /api/order/detail?id=1001    → 200 + 订单详情
```

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §a](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## b. 前端硬编码指纹

**识别信号**
HTML / JS 源码中暴露：
- 默认账号：`admin / 123456`、`test / test`、`demo / demo`
- 测试手机号 / 邮箱：`13800138000`、`test@test.com`
- 调试开关：`isDebug=true`、`DEBUG = true`
- 注释中的内部地址：`http://192.168.x.x`、`http://10.x.x.x`

**搜索关键词**
```javascript
// 硬编码特征
"admin" "123456"
"password" "secret" "key"
"13800138000" "10001"
"TODO" "FIXME"
"http://192.168" "http://10." "http://172."
"console.log("  // 调试日志
"isDebug" "DEBUG_MODE"
```

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §b](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## c. 路由守卫指纹

**识别信号**
- 前端路由（如 Vue Router 的 `beforeEach`、`meta.requiresAuth`、React 的 `<ProtectedRoute>`）
- **关键判定：只在路由层校验登录态，未在接口层校验**

**典型表现**
```
访问 https://target.com/admin → 前端弹登录 → 直接请求 https://target.com/api/admin → 200
```

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §c](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## d. 注册功能指纹（获取 token 的最高优先级入口）

**识别信号**
- 存在 `/api/register`、`/api/signup`、`/api/user/signup` 等接口
- 存在 `/api/sendSmsCode`、`/api/sendEmailCode`
- 注册流程不需要邀请码 / 实名 / 公司授权

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §d](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## e. 可获取 token 的接口

**识别信号**
响应中出现 token/accessToken/Authorization 的接口，包括但不限于：

| 类型 | 路径特征 |
|------|----------|
| 登录 | /login /signin /oauth/token |
| 注册 | /register /signup |
| 刷新 | /refresh /token/refresh |
| 游客/匿名 token | /guest /anonymous /visitor/token |
| 短信/邮箱登录 | /sms/login /email/login |
| 找回/重置密码 | /forgot /resetPassword /retrieve |
| 第三方回调 | /callback /oauth/callback /sso/acs |
| 切换租户/身份 | /switchTenant /impersonate /sudo |
| 开发/调试发 token | /dev/token /test/login /mock/auth |

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §e](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## f. 权限提升（垂直越权）指纹

**识别信号**
业务请求中携带：
- `role`、`group`、`isAdmin`、`userType`、`level`
- `X-User-Role`、`X-Admin`、`X-Tenant-Id`（请求头形式）
- Body 参数：`{"role":0,"username":"test"}`

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §f](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## g. 认证结构（SSO / OAuth2 / JWT）指纹

**识别信号**
URL / Cookie / Header 中：
- `client_id`、`redirect_uri`、`scope`、`response_type=code`
- `state`、`nonce`、`code_challenge`（PKCE）
- `access_token`、`refresh_token`、`id_token`

JS 文件中搜索：
```javascript
"OAuth"        "OAuth2"     "access_token"
"SSO"          "CAS"        "saml"        "SAML"
"client_secret" "appSecret"  "app_id"
"sign"          "signature"  "hmac"
"jwt"           "Bearer "
```

**路由**：
- 识别 OAuth2 → 走 [oauth2-fingerprint.md](oauth2-fingerprint.md)（识别信号细化）
- 识别 SSO → 走 [sso-fingerprint.md](sso-fingerprint.md)（识别信号细化）
- 识别 JWT → 走 `api-tester` 矩阵 [§g](../../api-tester/references/06-self-dev-auth-matrix.md#g-认证结构sso--oauth2密钥与方式)

---

## h. 鉴权字段绕过指纹（**实战最强指纹**）

**识别信号**
登录后的请求中携带非常规字段做身份标识：
- 请求头：`X-User-Id`、`X-Role`、`X-Tenant-Id`、`X-Auth-User`、`X-Account`
- Cookie：`userid=`、`uid=`、`account=`、`role=`、`is_admin=`（加密后字段名未变）
- Body：`userId=1001`、`loginUser=xxx`

**关键判定**：很多系统把鉴权字段 **AES / DES 加密后** 写入 Cookie，看上去是乱码，但解密后字段名就是 `userid`、`uid`。**字段名就是攻击面**。

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §h](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## i. 签名字段逆向伪造指纹（**自研高发**）

**识别信号**
请求参数或头中出现签名字段，且前端 JS 可还原算法与密钥：

```
参数：sign, signature, signData, msg_signature, sig, hash, mac, s
请求头：X-Sign, X-Signature, X-Ca-Signature, X-Hmac-Signature, X-Request-Sign
JS 关键字：
  md5(  sha1(  sha256(  hmac  HmacSHA  CryptoJS  JSEncrypt  sm3
  sign(  getSign(  makeSign(  createSignature(  doSign(
  secret  appSecret  appKey  salt  iv  key  signKey
```

**典型表现**
```
POST /api/order/create
{"userId":1001,"amount":100,"timestamp":1710000000,"sign":"a1b2c3..."}

GET /api/list?page=1&ts=1710000000&sign=md5(sorted_params+secret)
```

**指纹采集要点（识别阶段必做）**
1. 从 MITM 流量标记所有带 `sign*` 的接口（不止登录）
2. 在 JS 中定位签名函数：进入签名的字段集合、是否含 raw body、时间窗字段名
3. 记录密钥来源：硬编码 / 配置接口下发 / 登录响应 / 与 appKey 拼接
4. 记录算法：MD5 / SHA* / HMAC / RSA / SM3 — **禁止停在"发现有 sign"**
5. 输出到 `work-space/[target]/recon/auth.md` 的 `crypto.sign` 段后，**强制调度 api-tester 矩阵 i**

**与 h 的区别**
| | h 鉴权字段 | i 签名字段 |
|--|-----------|-----------|
| 作用 | 声明"我是谁" | 证明"请求未被改 / 来自客户端" |
| 伪造方式 | 换 userid / 置空 / 重加密身份 | 还原算法后重算 sign，或删签绕过 |
| 组合 | 伪身份后访问他人数据 | 伪签后改 amount/role/userid 仍通过校验 |

**验证执行**：见 `api-tester` → [06-self-dev-auth-matrix.md §i](../../api-tester/references/06-self-dev-auth-matrix.md)

---

## 自研系统指纹采集模板

每次识别出某自研系统，把以下字段补全，沉淀到 `work-space/[target]/recon/auth.md`：

```yaml
target: https://xxx.com
system_type: 自研
auth_flow:
  login_url: /api/login
  register_url: /api/register   # 注册入口
  reset_url: /api/forgot        # 密码重置入口
  token_field: Authorization    # token 字段名
  auth_headers:                 # 鉴权专用头
    - X-User-Id
    - X-Tenant-Id
  body_params:                  # 业务请求中携带身份字段
    - role
    - userId
crypto:
  algo: AES   # JS 中识别到的加解密算法
  field: userid
  bypass: 置空 / 重加密
  sign:                        # i. 签名字段指纹
    field: sign                # 或 X-Sign
    algo: MD5 / HMAC-SHA256 / RSA
    secret_source: js_hardcode / config_api / login_resp
    signed_params: [userId, amount, timestamp]
    api_tester_handoff: matrix_i_required
sso_or_oauth:
  type: OAuth2
  client_id: xxxxx
  client_secret: found_in_js   # 是否在前端泄露
test_entries:
  - 注册 → 拿 token → 全量接口
  - userid 字段替换 + 重加密
  - client_secret 弱口令
  - reset token 越权重置
  - 签名字段逆向伪造（api-tester §i）
  - 异常鉴权请求头绕过（api-tester §h）
handoff:
  skill: api-tester
  doc: references/06-self-dev-auth-matrix.md
  checklist: work-space/[target]/findings/self-dev-auth-matrix.md
```