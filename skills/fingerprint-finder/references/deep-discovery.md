# 深度指纹发现（重点）

> ⚠️ **使用前提（防误报）**：以下"关联目录"是**路径 Fuzz 的候选方向**，不是框架判定依据。命中某路径只说明"值得再探"，不等于"底层就是该框架"。
> 尤其：`/ureport/`、`/datasource/` 属 **Java** 组件（UReport2），不属于 ThinkPHP。详见 [stack-evidence-grading.md](stack-evidence-grading.md)。

## 核心原则

基于框架特征进行目录 Fuzz，尤其适用于二次开发系统。

## 1. 二次开发系统特征

**问题说明**：
- 二次开发的系统往往会弱化原始系统的指纹特征
- 管理员可能修改了默认的 logo、标题等表面特征
- 但底层框架代码和目录结构往往保持不变

**发现策略**：
```
看见 /datasource → 尝试 /ureport
看见 /admin → 尝试 /ureport, /datasource, /js, /api
看见特定接口 → 反推可能的其他接口
```

## 2. 框架关联目录字典

### Spring Boot 关联目录
```
已知路径 → 关联路径
/actuator/ → /actuator/health, /actuator/env, /actuator/beans
/datasource → /ureport, /druid, /webconsole
/admin → /actuator, /druid, /swagger-ui
/api → /swagger-ui.html, /v2/api-docs, /doc.html
```

### ThinkPHP 关联目录
```
已知路径 → 关联路径
/index.php → /phpmyadmin, /webroot, /admin
/home → /admin, /manage, /backend
```
> ⚠️ 原表中的 `/ureport`、`/datasource` 已移出——它们是 **Java 报表组件 UReport2** 的特征（见下方 Spring Boot 关联目录），不是 ThinkPHP 特征。

### 通用关联目录
```
已知路径 → 关联路径
/js → /static/js, /assets/js, /javascript
/css → /static/css, /assets/css, /styles
/images → /static/images, /assets/images, /img
/api → /api-docs, /swagger-ui, /v2/api-docs
/datasource → /ureport, /druid, /webconsole   （三者均为 Java 生态组件，命中后按 Java 方向验证）
/config → /api/config, /settings, /configuration
/backup → /backup, /bak, /.git, /.svn
```

## 3. JS 指纹挖掘

### JS 文件分析流程
```
1. 提取所有 JS 文件
2. 搜索框架特征关键词：
   - 框架名称：vue, react, angular, jquery
   - 版本号：v1.0, 2.0, 3.0
   - 路径特征：/static/, /vendor/, /dist/
   - API 端点：/api/, /v1/, /admin/
   - 配置信息：baseUrl, apiKey, token
3. 分析请求和响应结构
4. 推断可能的隐藏路径
```

### JS 中的指纹特征
```javascript
// Vue
Vue, vue, vueRouter, vuex
component, props, emit
"app", "vue"

// React
React, react, ReactDOM
component, useState, useEffect
"app", "root"

// jQuery
jQuery, $, fn.jquery
"jquery"

// Angular
angular, ng-app, ng-controller
"angular"

// Bootstrap
bootstrap, $.fn.modal
"bootstrap"

// Element UI
element, ElementUI, el-
"element-ui"

// Ant Design
ant, AntDesign, antd
"antd"

// 通用框架
baseUrl, apiUrl, apiPrefix
config, settings, constants
```

## 4. 接口指纹挖掘

### API 结构分析
```
1. 分析现有 API 路径模式
   /api/v1/users
   /api/v1/orders
   /api/v1/products

2. 推断可能的 API 路径
   /api/v1/admin
   /api/v1/config
   /api/v1/settings
   /api/v1/datasource

3. 尝试常见的 API 端点
   /api/login
   /api/logout
   /api/register
   /api/upload
   /api/download
```

### RESTful API 模式
```
GET    /api/v1/{resource}          - 获取列表
GET    /api/v1/{resource}/{id}     - 获取单个
POST   /api/v1/{resource}          - 创建
PUT    /api/v1/{resource}/{id}     - 更新
DELETE /api/v1/{resource}/{id}     - 删除
```

### GraphQL API 模式
```
POST   /graphql                    - GraphQL 端点
POST   /api/graphql                - GraphQL 端点
GET    /graphql-playground         - GraphQL Playground
GET    /altair                     - Altair GraphQL Client
```

## 5. 深度 Fuzz 策略

### 基于已知路径的 Fuzz
```
发现 /datasource 时，深度 Fuzz：
/datasource/ureport
/datasource/backup
/datasource/config
/datasource/api
/datasource/admin

发现 /ureport 时，深度 Fuzz：
/ureport/datasource
/ureport/backup
/ureport/config
/ureport/api
/ureport/admin
/ureport/report
/ureport/ureport2
```

### 基于框架的 Fuzz
```
ThinkPHP 深度 Fuzz：
/phpmyadmin/
/webroot/
/admin/
/manage/
/backend/
/index.php/                    （路由参数 / 控制器路径变体）

Spring Boot 深度 Fuzz：
/actuator/
/actuator/health
/actuator/env
/actuator/beans
/actuator/configprops
/actuator/mappings
/druid/
/webconsole/
/h2-console/
/swagger-ui.html
/v2/api-docs
/ureport/designer/                 （Java 报表组件 UReport2）
/ureport/datasource/previewData    （CVE-2020-21124 未授权 SQL 执行）
```
