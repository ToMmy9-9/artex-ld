# SSO 绕过测试执行矩阵（api-tester 专用）

> 本文件是 `06-self-dev-auth-matrix.md §g` 的 SSO 专项展开。
> **触发条件**：`fingerprint-finder` 识别到 SSO 指纹（见 [`sso-fingerprint.md`](../../fingerprint-finder/references/sso-fingerprint.md)）后，由 `api-tester` 加载本文件执行接口级验证。
> **职责边界**：指纹识别 → `fingerprint-finder`；**可执行测试 / fuzz / 凭据验证** → 本文件（api-tester）。

---

## 1. userid 置空绕过（加密后置空）

### 1.1 场景
SSO 服务把 userid / uid 加密后写到浏览器 Cookie / Storage 作为身份标识。
看上去像乱码：`u=7B8F2A9E1C...`、`_token=abc123==`

### 1.2 为什么能绕过
服务端在解密失败 / 字段为空时，往往 **降级到默认账号**（常见 bug 模式）：
- 解密失败 → 未报错 → 给一个"匿名但已登录"的会话
- 字段为空 → 后端未校验 → SQL 里 `WHERE user_id = ''` 命中"默认用户"
- 字段为 0 / null → 同上

### 1.3 识别信号（fingerprint-finder 已采集）
- 登录后 Cookie 中出现 **加密形式的长字符串字段**：`u`、`_u`、`_sso_user`、`token`、`uid`
- 字段名通常是单个字母或带下划线（开发者刻意模糊）
- 多个端共享同一个 SSO 服务（Web / 小程序 / APP）

### 1.4 测试 SOP
```
1. 正常登录 A 账号，记录加密字段值（如 u=xxx）
2. 抓包分析：浏览器如何解密？→ 前端 JS 中一定有 decrypt 函数
3. 平台工具 拦截后重发，但把加密字段替换为：
   - 空字符串：u=
   - null 序列化：u=null
   - 占位：u=AAAAAAAA（解密后是乱码）
   - 通用字符串：u=admin
4. 判断响应：
   - 返回 200 + 业务数据 → 可能落到默认账号
   - 返回 200 + admin 用户数据 → 高危垂直越权
   - 返回 401 / 403 → 未绕过，换下一个向量
```

### 1.5 关键判定
**重点不是能不能登录，而是登录后是哪个账号**。
返回的页面/接口要对比：
- 用户名 / 头像 / 用户 id 是否变成"系统默认账号"
- 是否有管理菜单 / 权限提升

---

## 2. 跨域跳转未校验（redirect_uri / returnUrl）

### 2.1 场景
SSO 登录后跳转回原页面，但 `redirect_uri` / `ReturnUrl` / `from` 参数未做白名单。

### 2.2 测试 SOP
```
1. 截取登录 URL，看是否有 returnUrl= 参数
2. 修改为：
   - returnUrl=https://attacker.com/
   - returnUrl=javascript:alert(1)
   - returnUrl=//attacker.com/
3. 第三方账号登录后 → 跳转到 attacker.com
4. 如果跳转 + URL 上携带了 code / token / sso_token → 一键登入他人账号
```

---

## 3. SSO callback 阶段携带账号信息

### 3.1 场景
与 OAuth2 类似，部分 SSO 实现 callback 时携带 userid / 用户名。

### 3.2 测试 SOP
```
1. 抓 callback URL：/sso/callback?ticket=xxx&userid=1001
2. 替换 userid 为他人账号
3. 如果服务端未校验 ticket 与 userid 关联 → 直接进他人账号
```

---

## 4. SSO cookie 复制（共享会话）

### 4.1 场景
服务端根据固定字段（如 `session_id`、`token`）识别会话，未绑定 IP / UA。

### 4.2 测试 SOP
```
1. 登录后从浏览器导出完整 Cookie
2. 另一个浏览器 / curl 复刻同样 Cookie
3. 如果直接登录同一账号 → 会话固定 / 共享会话问题
4. 进一步：替换 cookie 中的 userid（如果未加密）→ 直接水平越权
```

---

## 5. SSO + 加密字段组合绕过（实战最高危）

### 5.1 场景
SSO 既把 userid 加密，又用加密字段做鉴权，但加密算法本身可逆 / 可识别。

### 5.2 测试 SOP
```
1. 抓取加密的 userid 字段（如 u=abc123）
2. 用 Python 编解码脚本 跑：
   - Base64 解码
   - URL Decode
   - AES 解密（拿前端 JS 里的 key）
   - DES / RC4 / SM4 试探
3. 一旦成功解密 → 修改 userid 为他人账号 → 用同一算法加密回去 → 重放
```

---

## 6. 与 06-self-dev-auth-matrix.md 的衔接

本文件是 `06-self-dev-auth-matrix.md §g 认证结构` 的 SSO 专项展开。当 api-tester 执行自研矩阵 §g 时：

```
1. 读取 fingerprint-finder 的 sso-fingerprint.md 确认识别信号
2. 加载本文件执行可重复的 HTTP 验证：
   a. 加密 userid 置空 / 解码替换
   b. redirect_uri 跳转劫持
   c. callback userid 越权
   d. cookie 复制会话
   e. 加密字段组合绕过
3. 每项测试输出 HTTP 状态码 + 响应内容 + 当前身份（A 账号 / 默认账号 / 他人账号）
4. 结果写回 work-space 的 06-self-dev-auth-matrix.md 对应行
```

## 7. Agent 调度 Prompt 模板

```
目标已判定 SSO 认证结构。按 api-tester/references/08-sso-bypass.md 执行：
1. 确认 fingerprint-finder 的识别信号（Cookie 加密字段 _sso_user=xxx）
2. 用 平台工具 MITM 走完整 SSO 登录，抓 Cookie
3. 测试加密字段置空 / null / 0 / 解码后替换 userid
4. 测试 redirect_uri 跳转劫持（returnUrl 修改）
5. 测试 callback userid 替换越权
6. 测试 cookie 复制到另一会话
7. 加密字段组合：解密 → 改 userid → 重加密 → 重放
8. 每项输出 HTTP 证据 → 更新 work-space 矩阵
工具：平台 fetch 工具 + 平台流量录制 + 浏览器 MCP + exec_codec
```

## 8. 输出格式（沉淀到 work-space）

```yaml
sso_target: https://target.com
sso_product: 自研SSO / CAS / SAML / OAuth-style
cookie_fields:
  - name: _sso_user
    encrypted: yes / no
    algo: AES / DES / Base64 / 明文
test_results:
  field_null: vulnerable / not_vulnerable (置空后身份为: 默认账号 / 报错)
  field_decode_replace: vulnerable / not_vulnerable
  redirect_uri_abuse: vulnerable / not_vulnerable
  cookie_clone: vulnerable / not_vulnerable
severity_candidates: [高危, 严重]
```