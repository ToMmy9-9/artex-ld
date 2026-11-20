# JS 指纹识别专项

> 本文件是 SKILL.md 中"JS 加解密指纹识别"节的可执行展开。
> 调用本文件的前提：已经从 HTML/响应头/Packet 中获取到 JS 资源 URL。

---

## 一、JS 加解密指纹识别（AI 跑核心）

### 1.1 核心原则

看见 JS 文件后，**不是先看路径，而是先搜加解密 / 认证关键字**。这些关键字是判断"系统是否有可绕过面"的最快指纹。

### 1.2 关键字与测试入口速查

| 关键词 | 含义 | 测试入口 |
|---|---|---|
| `RSA`、`JSEncrypt`、`encryptlong` | RSA 非对称加密 | 找公钥 → 加密任意 userid 替换身份 |
| `AES`、`CryptoJS.AES`、`SM4` | 对称加密 | 替换明文 → 用前端 key 重新加密 |
| `signature`、`sign=`、`hmac` | HMAC 签名 | 算法识别 → key 提取 → 重放 / 越权 |
| `OAuth`、`OAuth2`、`access_token` | OAuth2 认证 | 走 [OAuth2 指纹识别](oauth2-fingerprint.md) → 交 `api-tester` 验证 |
| `SSO`、`CAS`、`saml`、`SAML` | 单点登录 | 走 [SSO 指纹识别](sso-fingerprint.md) → 交 `api-tester` 验证 |
| `jwt`、`Bearer ` | JWT 认证 | alg=none / 弱密钥破解 |

### 1.3 提取 SOP

```
1. 用 yakit query_http_flow 列出所有 .js 资源
2. 优先看 chunk-vendors.* / app.* / main.* 这类主入口
3. 用 sensitive-info-leak 跑以下 grep：
   - client_secret / appSecret / secret_key
   - access_token / refresh_token
   - password / pwd / passwd（注意 MD5/SHA1 形态）
   - hardcoded URL（http://192.168 / http://10. / 内网域名）
4. 把命中行所在 JS 文件 URL + 行号 记录到 work-space
```

### 1.4 识别结果 → 测试动作（强制）

> **不要仅停留在"识别"，必须把识别结果转成测试动作。**

| 识别结果 | 立即追加的测试指令 |
|---|---|
| OAuth2 | 测 client_secret 弱口令 / userid 越权 / code 重放 |
| SSO/CAS | 测 userid 加密后置空 / cookie 复制 |
| JWT | 测 alg=none / 弱 HMAC 密钥 / 头部 kid 注入 |
| RSA | 提公钥 → 自签 userid/role 替换 |
| AES/SM4 | 用前端 key 重加密任意明文 → 覆盖身份字段 |
| HMAC | 跑 top1w 弱密钥 / 提取盐 → 重放 |

---

## 二、JS 指纹采集模板（沉淀到 work-space）

每次完成 JS 指纹识别，把以下字段补全保存：

```yaml
target: https://xxx.com
js_source:
  - https://xxx.com/static/js/app.xxx.js
  - https://xxx.com/static/js/chunk-vendors.xxx.js
keywords_found:
  rsa: found / not_found
  aes: found (CryptoJS.AES.encrypt) / not_found
  oauth2: found (client_id=xxx) / not_found
  sso: found / not_found
  jwt: found / not_found
crypto:
  algo: AES / RSA / HMAC-SHA256 / SM4
  key_or_iv: found_in_js / obfuscated / unknown
  field: userid / role / timestamp
next_steps:
  - 用 frontend key 重加密 userid=admin
  - 测 client_secret 弱口令
```
