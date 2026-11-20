# OAuth2 绕过测试执行矩阵（api-tester 专用）

> 本文件是 `06-self-dev-auth-matrix.md §g` 的 OAuth2 专项展开。
> **触发条件**：`fingerprint-finder` 识别到 OAuth2 指纹（见 [`oauth2-fingerprint.md`](../../fingerprint-finder/references/oauth2-fingerprint.md)）后，由 `api-tester` 加载本文件执行接口级验证。
> **职责边界**：指纹识别 → `fingerprint-finder`；**可执行测试 / fuzz / 凭据验证** → 本文件（api-tester）。

---

## 1. client_secret 攻击面

### 1.1 暴力破解（针对自研 OAuth2 服务）

**适用场景**
- 自研 OAuth2 服务，client_secret 由业务方自己生成
- 厂商 OAuth（微信 / 钉钉 / 飞书 / GitHub）的 client_secret 走厂商，暴力无意义

**测试 SOP**
```
1. 抓取 /oauth/token 或 /api/oauth/authorize 请求与响应
2. 用 平台 fetch 工具 对 client_secret 跑弱口令字典
3. 字典来源（构造业务侧字典，禁止默认字典滥用）：
   - 目标公司名 / 域名 / 系统名
   - 测试常见弱口令：123456 / password / secret / test / app123
   - 时间戳变体：目标上线年 + 123
4. 观察 HTTP 200 + access_token → 命中
```

### 1.2 前端泄露

**识别信号**（fingerprint-finder 已采集）
- JS / JSX / Vue 文件中出现明文 client_secret
- `.env.production`、`.env.local`、`config.js`、`constants.ts` 中出现

**测试 SOP**
```
1. 用 sensitive-info-leak 扫目标所有 JS 资源
2. grep 关键词: client_secret, appSecret, app_secret, secret_key
3. 命中后立即验证：把 secret 替换到本地 OAuth2 流程 → 看是否生效
```

### 1.3 登录抓包（authorization_code 流程）

**适用场景**
- 标准 OAuth2 authorization_code 流程
- code 仅一次性使用，但可能被中间人 / 抓包者截获

**测试 SOP**
```
1. 触发完整 OAuth 登录（走 平台流量录制）
2. 截获 redirect_uri?code=xxx
3. 在 30 秒内用同一 code 再请求一次 token 接口
4. 如果两次都拿到 access_token → code 重放漏洞（CVE 常见）
```

### 1.4 小程序 / APP 联动

**适用场景**
- 同一公司多个端（Web + 小程序 + APP）共用同一套 OAuth2 服务
- 小程序 / APP 端的 client_secret 可能更弱（暴露更彻底）

**测试 SOP**
```
1. 用 浏览器 MCP 或 wxapp 工具反编译小程序
2. 在反编译产物中搜 client_secret / appSecret
3. 把小程序端的 secret 拿到 Web 端 OAuth 流验证 → 跨端身份冒充
```

---

## 2. userid 字段攻击面

### 2.1 越权（OAuth2 callback 携带 userid）

**典型表现**
```
GET /oauth/callback?code=xxx&userid=1001
GET /api/oauth/login?code=xxx&userId=admin
```

**测试 SOP**
```
1. 用 A 账号走完整 OAuth 流程，截获 callback
2. 把 userid / userId 参数替换为他人账号 / 0 / admin
3. 重放请求，看是否返回他人的 access_token
```

### 2.2 置空（userid 留空 / null / 0）

**测试 SOP**
```
1. 同上截获 callback
2. 修改参数为：
   - userid=（空）
   - userid=null
   - userid=0
   - userId=admin
3. 看是否返回"系统默认账号"的 access_token（高频 bug：未校验 → 落到预置账号）
```

---

## 3. passwd 字段攻击面

**适用场景**
自研 OAuth2 服务，开发者直接把 `password` 做 SHA1/MD5 后当作 token / sign 字段。

**测试 SOP**
```
1. 抓包 OAuth 接口，找包含 passwd / password 的请求
2. 修改为：
   - passwd=（空）
   - passwd=null
   - passwd=任意值
3. 如果仍返回 access_token → 后端未校验密码（极严重）
4. 若 SHA1 字段：搜公开泄露库 / 字典反查明文密码
```

---

## 4. 与 06-self-dev-auth-matrix.md 的衔接

本文件是 `06-self-dev-auth-matrix.md §g 认证结构` 的 OAuth2 专项展开。当 api-tester 执行自研矩阵 §g 时：

```
1. 读取 fingerprint-finder 的 oauth2-fingerprint.md 确认识别信号
2. 加载本文件执行可重复的 HTTP 验证：
   a. client_secret 暴力 / 泄露 / 跨端
   b. callback userid 越权 / 置空
   c. passwd 字段绕过
   d. code 重放（30 秒窗口）
3. 每项测试输出 HTTP 状态码 + 响应内容 + 证据请求
4. 结果写回 work-space 的 06-self-dev-auth-matrix.md 对应行
```

## 5. Agent 调度 Prompt 模板

```
目标已判定 OAuth2 认证结构。按 api-tester/references/07-oauth2-bypass.md 执行：
1. 确认 fingerprint-finder 的识别信号（client_id / redirect_uri / response_type=code）
2. 用 平台工具 MITM 走完整 OAuth 登录，抓 callback
3. 测试 callback userid 替换 / 置空 / 越权
4. 用 sensitive-info-leak 扫 client_secret 泄露
5. 对 token 接口跑 client_secret 弱口令
6. 测试 code 重放（30 秒内重复用）
7. 每项输出 HTTP 证据 → 更新 work-space 矩阵
工具：平台 fetch 工具 + 平台流量录制 + 浏览器 MCP + sensitive-info-leak
```

## 6. 输出格式（沉淀到 work-space）

```yaml
oauth2_target: https://target.com
flow: authorization_code
client_id: xxxxx
client_secret: leaked_in_js_path  # 或 brute_success / unknown
endpoints:
  authorize: /oauth/authorize
  token: /oauth/token
  callback: /oauth/callback
test_results:
  code_replay: vulnerable / not_vulnerable
  userid_swap: vulnerable / not_vulnerable (替换为 userid=2 时进他人账号)
  userid_null: vulnerable / not_vulnerable (置空后返回默认账号)
  client_secret_brute: hit / miss / skip
severity_candidates: [高危, 严重]
```