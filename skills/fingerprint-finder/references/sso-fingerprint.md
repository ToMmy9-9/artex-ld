# SSO 指纹识别（精简版）

> 本文件是 SKILL.md 中"五、SSO 绕过指纹"的识别信号展开。
> **职责边界**：只负责识别信号采集。**验证执行**归 `api-tester` → [08-sso-bypass.md](../../api-tester/references/08-sso-bypass.md)

---

## 1. 识别信号

### 1.1 URL / 参数指纹

| 信号 | 示例 |
|------|------|
| `service` 参数 (CAS) | `service=https://app.com/login` |
| `TGT` / `ST` 票据参数 | `ticket=ST-xxx` |
| `SAMLRequest` / `SAMLResponse` | SAML 协议交互 |
| `RelayState` 参数 | `RelayState=xxx` |
| `returnUrl` / `redirect_uri` 参数 | `returnUrl=https://app.com` |

### 1.2 JS 关键字指纹

在 JS 文件中搜索以下关键字：

```javascript
"SSO"          "CAS"          "saml"         "SAML"
"单点登录"     "sso_token"    "sso_user"
"login?service="
"_sso_"        "_cas_"        "_saml_"
```

### 1.3 Cookie / Header 指纹

**典型 Cookie 字段名**（加密或明文）：
```
u, _u, _sso_user, _sso_token, sso_token, token, uid, _uid, _token_user
```

**特征**：
- 字段名通常是单个字母或带下划线（开发者刻意模糊加密）
- 值为加密长字符串（Base64 / AES 密文），看似乱码
- 多个端（Web / 小程序 / APP）Cookie 字段名相同

### 1.4 接口路径指纹

```
/login?service=xxx
/sso/login
/sso/callback
/cas/login
/cas/serviceValidate
/saml/acs
/saml/sso
/sso/acs
```

---

## 2. 识别后路由

识别到 SSO 指纹后：

1. 采集到的信号详情（Cookie 字段名、加密特征、endpoint 等）写入识别报告
2. **调度 `api-tester`** 加载 [08-sso-bypass.md](../../api-tester/references/08-sso-bypass.md) 执行接口级验证
3. 验证结果回写到 `06-self-dev-auth-matrix.md §g`

**禁止**：在识别阶段做任何测试 / 字段篡改 / 凭据验证。

---

## 3. 输出格式（识别阶段）

```yaml
sso_fingerprint:
  detected: true
  signals:
    - url: /login?service=https://app.com/callback
    - js: "SSO" found in static/js/auth.js
    - cookie: "_sso_user" with encrypted value
  product: 自研SSO / CAS / SAML / 未知
  cookie_fields:
    - name: _sso_user
      encrypted: yes / no
      algo: 未知 (需 api-tester 解码确认)
  endpoints:
    login: /login?service=xxx
    callback: /sso/callback
    validate: /cas/serviceValidate
  next_step: api-tester/references/08-sso-bypass.md
```