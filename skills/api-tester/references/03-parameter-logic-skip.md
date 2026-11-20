# 参数逻辑跳过 Fuzz 详细说明

## 1. 核心原理

通过猜测和枚举参数值，跳过多步骤流程中的前置步骤，直接到达目标功能点。

### 1.1 为什么有效
- 很多功能点采用多步骤流程（如：输入手机号 → 获取验证码 → 设置新密码）
- 每个步骤对应不同的参数值（如：type=GetPwd1、type=GetPwd2、type=GetPwd3）
- 通过猜测参数值的命名规律，可以直接跳过后续步骤，实现逻辑绕过

## 2. 命名规律分析

| 类型 | 规律示例 |
|------|----------|
| 数字递增型 | `type=step1`, `type=step2`, `type=step3` |
| 功能编号型 | `type=GetPwd1`, `type=GetPwd2`, `type=GetPwd3` |
| 操作类型型 | `action=send`, `action=verify`, `action=reset` |
| 状态机型 | `status=input`, `status=verify`, `status=complete` |
| 命令型 | `cmd=phone`, `cmd=code`, `cmd=password` |
| 模式型 | `mode=1`, `mode=2`, `mode=3` |
| 版本型 | `v=1`, `v=2`, `v=3` |
| 阶段型 | `phase=1`, `phase=2`, `phase=3` |

## 3. 常见功能场景

### 3.1 密码重置流程
```
?type=GetPwd1        → 输入手机号/邮箱
?type=GetPwd2        → 获取验证码
?type=GetPwd3        → 输入新密码
?type=GetPwd4        → 确认重置

猜测：?type=GetPwd5, ?type=reset_direct
```

### 3.2 用户注册流程
```
?step=1              → 输入基本信息
?step=2              → 验证邮箱/手机
?step=3              → 设置密码
?step=4              → 完成注册

猜测：?step=5, ?step=skip_verify
```

### 3.3 支付流程
```
?stage=cart          → 购物车
?stage=address       → 填写地址
?stage=payment       → 选择支付方式
?stage=confirm       → 确认订单
?stage=success       → 支付成功

猜测：?stage=direct_pay, ?stage=bypass_confirm
```

### 3.4 身份验证流程
```
?verify=phone        → 手机验证
?verify=email        → 邮箱验证
?verify=id_card      → 身份证验证
?verify=face         → 人脸验证

猜测：?verify=admin, ?verify=bypass
```

### 3.5 文件上传流程
```
?upload=select       → 选择文件
?upload=upload        → 上传文件
?upload=check         → 检查文件
?upload=save          → 保存文件

猜测：?upload=direct, ?upload=bypass_check
```

### 3.6 订单处理流程
```
?order=create        → 创建订单
?order=pay           → 支付订单
?order=ship          → 发货
?order=receive       → 收货
?order=complete      → 完成

猜测：?order=refund_direct, ?order=admin_complete
```

## 4. Fuzz 测试策略

### 4.1 发现阶段
```
1. 观察正常流程的参数值
2. 分析参数命名规律
3. 确定参数类型（数字、字母、混合）
4. 识别参数范围（1-3、a-z、A-Z）
```

### 4.2 枚举阶段
```
数字枚举：?step=1, ?step=2, ?step=3, ... ?step=10
字母枚举：?action=a, ?action=b, ?action=c
组合枚举：?type=GetPwd1, ?type=GetPwd2, ?type=GetPwd3
规律枚举：?mode=normal, ?mode=admin, ?mode=super
```

### 4.3 绕过阶段
```
1. 尝试跳过后续步骤
2. 尝试直接访问最终步骤
3. 尝试组合参数值
4. 尝试修改参数类型
```

## 5. 高级绕过技巧

### 5.1 参数值组合
```
?step=1,2,3          → 组合多个步骤
?step=1|2|3          → 使用管道符
?step=[1,2,3]        → 使用数组
?step=1,skip         → 跳过中间步骤
```

### 5.2 参数值覆盖
```
?step=1&step=3       → 覆盖参数
?step=1;step=3       → 使用分号
?step=1%26step=3     → URL编码覆盖
```

### 5.3 参数值模糊
```
?step=*              → 通配符
?step=%              → 特殊字符
?step=null           → 空值
?step=true           → 真值
```

### 5.4 参数名变形
```
?step=3             → 数字参数名
?Step=3             → 首字母大写
?STEP=3             → 全大写
?sTEp=3             → 混合大小写
?stp=3              → 缩写形式
```

## 6. 测试示例

### 示例1：密码重置逻辑绕过
**目标**：https://example.com/reset_password

**观察**：发现 `?type=GetPwd1`, `?type=GetPwd2`

**测试**：
```
?type=GetPwd3       → 尝试直接重置（成功绕过验证码）
?type=GetPwd4       → 尝试其他步骤
?type=GetPwd99      → 尝试极端值
```

**结果**：发现 `?type=GetPwd3` 可直接重置密码

### 示例2：注册流程跳过验证
**目标**：https://example.com/register

**观察**：发现 `?step=1`, `?step=2`（需要邮箱验证）

**测试**：
```
?step=3             → 尝试跳过验证步骤
?step=4             → 尝试最终步骤
?step=3&verified=1  → 组合参数
```

**结果**：发现 `?step=3&verified=1` 可绕过邮箱验证

### 示例3：支付流程跳过确认
**目标**：https://example.com/pay

**观察**：发现 `?stage=confirm`（确认支付）

**测试**：
```
?stage=success      → 尝试直接成功
?stage=complete     → 尝试完成
?stage=success&amount=0 → 尝试金额修改
```

**结果**：发现 `?stage=success&amount=0` 可0元支付

## 7. JS 逆向辅助

通过分析 JS 代码，发现参数值的生成逻辑：

```javascript
// 从JS中发现的代码
const steps = ['GetPwd1', 'GetPwd2', 'GetPwd3', 'GetPwd4'];
const actions = ['input', 'verify', 'reset', 'confirm'];

// 发现参数枚举规律
// 可以猜测 GetPwd5, GetPwd6 等后续步骤
```

## 8. 注意事项

- ⚠️ 逻辑跳过测试可能影响正常业务流程
- ⚠️ 部分功能可能有服务端校验，单纯修改参数可能无效
- ⚠️ 需要结合实际业务逻辑进行分析
- ✅ 记录所有尝试的参数值和响应结果
- ✅ 分析响应差异，判断是否绕过成功
