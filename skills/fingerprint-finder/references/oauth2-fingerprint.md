# OAuth2 指纹识别（精简版）

> 本文件是 SKILL.md 中"四、OAuth2 绕过指纹"的识别信号展开。
> **职责边界**：只负责识别信号采集。**验证执行**归 `api-tester` → [07-oauth2-bypass.md](../../api-tester/references/07-oauth2-bypass.md)

---

## 1. 识别信号

### 1.1 URL / 参数指纹

| 信号 | 示例 |
|------|------|
| `client_id` 参数 | `client_id=xxx` |
| `redirect_uri` 参数 | `redirect_uri=https://app.com/callback` |
| `response_type=code` | `response_type=code` |
| `scope` 权限参数 | `scope=user_info,orders` |
| `state` / `nonce` 防伪造参数 | `state=abc123` |
| `code_challenge` (PKCE) | `code_challenge=xxxx` |

### 1.2 JS 关键字指纹

在 JS 文件中搜索以下关键字：

```javascript
"OAuth"        "OAuth2"       "access_token"
"client_id"    "client_secret" "appSecret"
"redirect_uri" "response_type" "authorization_code"
"grant_type"   "refresh_token" "Bearer"
```

### 1.3 Cookie / Header 指纹

- 响应中出现 `access_token`、`refresh_token`、`id_token`
- 请求头出现 `Authorization: Bearer xxx`
- Cookie 中出现 `token`、`access_token`、`refresh_token`

### 1.4 接口路径指纹

```
/oauth/authorize
/oauth/token
/oauth/callback
/oauth/login
/api/oauth/authorize
/api/oauth/token
```

---

## 2. 识别后路由

识别到 OAuth2 指纹后：

1. 采集到的信号详情（client_id、endpoint、redirect_uri 等）写入识别报告
2. **调度 `api-tester`** 加载 [07-oauth2-bypass.md](../../api-tester/references/07-oauth2-bypass.md) 执行接口级验证
3. 验证结果回写到 `06-self-dev-auth-matrix.md §g`

**禁止**：在识别阶段做任何测试 / fuzz / 凭据验证。

---

## 3. 输出格式（识别阶段）

```yaml
oauth2_fingerprint:
  detected: true
  signals:
    - url: /oauth/authorize?client_id=xxx&redirect_uri=yyy
    - js: "client_secret" found in static/js/main.js
    - header: "Authorization: Bearer" in requests
  client_id: xxxxx
  endpoints:
    authorize: /oauth/authorize
    token: /oauth/token
    callback: /oauth/callback
  next_step: api-tester/references/07-oauth2-bypass.md
```