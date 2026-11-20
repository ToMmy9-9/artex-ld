# 输出格式详细说明

## 0. 自研系统矩阵输出（判定自研时必有）

落盘：`work-space/[目标]/findings/self-dev-auth-matrix.md`（模板见 [06-self-dev-auth-matrix.md](06-self-dev-auth-matrix.md)）

```
[自研矩阵] system_type=自研
[自研矩阵] a 接口未授权: hit|miss|partial  (例: GET /api/order/detail 无 Cookie → 200)
[自研矩阵] b 前端硬编码: hit|miss         (例: demo/demo 登录成功)
[自研矩阵] c 路由守卫绕过: hit|miss       (例: /admin 前端拦截, /api/admin/list 200)
[自研矩阵] d 注册获 token: ok|fail
[自研矩阵] e 其他发 token: list...
[自研矩阵] f 垂直越权/role: hit|miss
[自研矩阵] g SSO/OAuth2/JWT: n/a|见专项
[自研矩阵] h 鉴权字段绕过: hit|miss       (例: X-User-Id 替换 → 用户B)
[自研矩阵] i 签名伪造: hit|miss           (例: MD5+secret 重签 amount=0.01)
[自研矩阵] 组合测试: 已对 N 个接口完成
```

## 1. JS 分析结果

```
[+] 发现 JS 文件：
    - https://example.com/static/js/main.js
    - https://example.com/static/js/vendor.js
    - https://example.com/static/js/admin.js

[+] 提取 API 端点：
    - /api/v1/users
    - /api/v1/admin/users
    - /api/v1/orders

[+] 提取参数：
    - { "user_id": 1, "role": "admin" }
    - { "product_id": 1, "quantity": 1, "price": 100 }

[+] 提取密钥：
    - API Key: sk-xxxxx
    - Secret: abc123
    - Token: eyJhbGciOiJIUzI1NiIs...
```

## 2. 注册流程结果

```
[+] 发现注册接口：/api/v1/register
[+] 注册参数：username, email, password, phone
[+] 注册成功：
    - 用户名：testuser
    - 邮箱：test@tempmail.com
    - 密码：Test123456
[+] 登录成功：
    - Token: eyJhbGciOiJIUzI1NiIs...
    - 用户 ID：12345
    - 权限：user
```

## 3. 越权测试结果

```
[!] 水平越权漏洞：
    - 接口：/api/v1/users/{id}
    - 描述：用户 B 可以访问用户 A 的数据
    - 影响：可枚举所有用户数据
    - 验证：GET /api/v1/users/1 (token_B) 返回 200

[!] 垂直越权漏洞：
    - 接口：/api/v1/admin/users
    - 描述：普通用户可以访问管理员接口
    - 影响：可管理所有用户
    - 验证：GET /api/v1/admin/users (user_token) 返回 200

[!] 权限提升：
    - 接口：/api/v1/users/profile
    - 描述：修改 role 参数可提升权限
    - 影响：普通用户可变为管理员
    - 验证：PUT /api/v1/users/profile {"role": "admin"} 返回 200
```

## 4. 逻辑漏洞结果

```
[!] 支付逻辑漏洞：
    - 接口：/api/v1/order
    - 描述：可修改价格为负数
    - 影响：可免费购买商品
    - 验证：POST /api/v1/order {"price": -100} 返回 200

[!] 参数覆盖漏洞：
    - 接口：/api/v1/order
    - 描述：JSON 参数覆盖可修改价格
    - 影响：可修改订单金额
    - 验证：POST /api/v1/order {"price": 100, "price": 1} 返回 200

[!] 优惠券滥用：
    - 接口：/api/v1/coupon/use
    - 描述：可重复使用同一优惠券
    - 影响：可无限次使用优惠券
    - 验证：多次 POST 使用同一 coupon_id 均返回 200
```

## 5. 发现的接口列表

```
[+] 公开接口：
    - GET /api/v1/users - 用户列表
    - GET /api/v1/products - 产品列表
    - POST /api/v1/login - 用户登录
    - POST /api/v1/register - 用户注册

[+] 需要认证的接口：
    - GET /api/v1/profile - 用户资料
    - GET /api/v1/orders - 订单列表
    - POST /api/v1/order - 创建订单

[+] 管理接口：
    - GET /api/v1/admin/users - 管理用户
    - GET /api/v1/admin/orders - 管理订单
    - PUT /api/v1/admin/config - 修改配置

[+] 隐藏接口：
    - GET /api/v1/internal/stats - 内部统计
    - GET /api/v1/debug/info - 调试信息
```

## 6. 提取的数据

```
[+] 用户数据：
    - 总数：100 条记录
    - 字段：id, username, email, phone, role
    - 示例：
      * ID: 1, 用户名: admin, 邮箱: admin@example.com, 角色: admin
      * ID: 2, 用户名: user1, 邮箱: user1@example.com, 角色: user

[+] 订单数据：
    - 总数：500 条记录
    - 字段：order_id, user_id, product_id, quantity, price, status

[+] 配置信息：
    - 数据库类型：MySQL
    - 数据库版本：8.0
    - API 密钥：sk-xxxxx
    - 密钥：abc123

[+] 敏感文件：
    - backup.sql - 数据库备份
    - config.json - 配置文件
    - .git/config - Git 配置

[+] 管理员账户：
    - admin@example.com / admin123
    - manager@example.com / manager123
```

## 7. 测试报告模板

```markdown
# API 安全测试报告

## 测试目标
- URL: https://example.com
- 时间: 2024-01-01 12:00:00
- 测试者: security-tester

## 测试范围
- [x] JS 文件分析
- [x] 接口发现
- [x] 注册流程测试
- [x] 越权测试
- [x] 逻辑漏洞测试

## 发现的安全问题

### 高危漏洞
1. **垂直越权漏洞**
   - 接口: /api/v1/admin/users
   - 描述: 普通用户可访问管理员接口
   - 风险: 可获取所有用户信息

2. **支付逻辑漏洞**
   - 接口: /api/v1/order
   - 描述: 可修改价格为负数
   - 风险: 可免费购买商品

### 中危漏洞
1. **水平越权漏洞**
   - 接口: /api/v1/users/{id}
   - 描述: 可访问其他用户数据
   - 风险: 用户隐私泄露

### 低危漏洞
1. **敏感信息泄露**
   - 接口: /api/v1/debug/info
   - 描述: 调试接口未关闭
   - 风险: 服务器信息泄露

## 测试凭据
- 用户: testuser / Test123456
- Token: eyJhbGciOiJIUzI1NiIs...

## 建议
1. 加强权限验证
2. 修复支付逻辑漏洞
3. 关闭调试接口
```
