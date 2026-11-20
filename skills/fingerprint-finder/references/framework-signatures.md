# 框架特征指纹库

> ⚠️ **使用前提（防误报）**：本文件列出的是**路径 / 响应头"信号"**，用于生成候选假设与路径 Fuzz 提示，**单独一条路径不构成框架或后端语言的判定**。
> 定后端语言必须有"芯信号"（错误回显 / 异常文本 / JWT 报错 / 错误页字节 / 凭据行为差）；前端特征（Vue / React / Element UI / chunk 名）只能定"壳"。
> 详见 [stack-evidence-grading.md](stack-evidence-grading.md)。

## 1. Java 框架

### Spring Boot
```
/actuator/                          - Actuator 端点
/actuator/health                    - 健康检查
/actuator/env                       - 环境变量
/actuator/configprops               - 配置属性
/actuator/beans                     - Bean 列表
/mappings                           - 请求映射
/dump                               - 线程转储
/heapdump                           - 堆转储
/shutdown                           - 关闭接口
```
> ⚠️ **版本差异**：Spring Boot 1.x 的 Actuator 端点在**根路径**（`/health`、`/env`、`/beans`、`/dump`、`/trace`），2.x 才统一到 `/actuator/*` 前缀；只探 `/actuator/*` 会漏判仍在用 1.x 的老系统。
> ⚠️ **同名信号易混**：大体积配置 JSON 端点（`/actuator/env`、`/configprops`）≠ 一定是 Spring——**ABP(.NET) 框架**的 `/api/abp/application-configuration` 同样返回数十 KB 配置 JSON。区分依据：`.AspNetCore.Antiforgery.*` Cookie、`X-Powered-By: ASP.NET`。

### UReport2（Java 报表组件 · bstek）—— 历史高频被误判为 ThinkPHP
```
/ureport/                           - 报表设计器 / 预览入口（Java 组件）
/ureport/designer/                  - 设计器入口
/ureport/datasource/previewData     - 预览数据（CVE-2020-21124 未授权 SQL 执行）
/datasource/                        - 报表数据源配置（单独出现无指向性，需结合 /ureport 上下文）
```
> ⚠️ `/ureport/`、`/datasource/` 是 **Java** 组件特征，**不是** ThinkPHP / PHP 特征。

### Shiro
```
/login                               - 登录页面
/rememberMe=                         - Cookie 名称
/deleteRememberMeCookie              - 注销功能
```

### Struts
```
/struts/webconsole.html             - 控制台
/struts2 Showcase/                  - 示例应用
.do                                   - Action 后缀
```

### WebLogic
```
/console/                           - 管理控制台
/bea_wls_internal/                  - 内部路径
/wls-wsat/                          - WLS-AT
/uddiexplorer/                      - 查询工具
```

### JBoss
```
/jmx-console/                       - JMX 控制台
/web-console/                       - Web 控制台
/admin-console/                     - 管理控制台
```

## 2. Python 框架

### Django
```
/admin/                             - 管理后台
/manage.py                          - 入口文件
/middleware.py                      - 中间件
/settings.py                        - 设置文件
/static/admin/                      - Admin 静态文件
X-Frame-Options: DENY               - 响应头特征
```

### Flask
```
/debug=true                         - Debug 模式
/static/                            - 静态文件
/WTF_CSRF_ENABLED                  - CSRF 配置
SECRET_KEY                         - 密钥
```

### Tornado
```
/static/                            - 静态文件
/template/                          - 模板目录
/xsrf_cookie                        - XSRF Cookie
```

## 3. PHP 框架

### Laravel
```
/storage/                           - 存储目录
/vendor/                            - 依赖目录
/bootstrap/cache/                   - 缓存目录
/artisan                            - CLI 工具
Set-Cookie: laravel_session         - Cookie 特征
```

### ThinkPHP（PHP）
```
/index.php                          - 入口文件
/index.php?s=xxx                    - 路由参数
/index.php/home/index/index         - 控制器路径
/think                              - CLI 工具
/runtime/                           - 运行时目录
X-ThinkPHP-Version                  - 响应头版本
/.htaccess                          - Apache 配置
```
> ⚠️ 若探测中同时出现 `/ureport/`、`/datasource/`，那是 **Java** 组件特征（见 Java 章节 UReport2），不要据此判定 ThinkPHP。

### Discuz
```
/static/image/common/               - 静态资源
/uc_server/                        - UCenter
/plugin.php                         - 插件入口
```

### DedeCMS
```
/dede/                             - 管理后台
/uploads/                          - 上传目录
/data/tplcache/                    - 模板缓存
```

## 4. JavaScript 框架

### Node.js
```
/package.json                       - 项目配置
/node_modules/                     - 依赖目录
/npm-debug.log                     - NPM 日志
```

### Express
```
/routes/                           - 路由目录
/views/                            - 视图目录
X-Powered-By: Express              - 响应头
```

### Next.js
```
/_next/static/                     - 静态资源
/__nextjs_requirehashgzip          - 构建标识
```

### Vue
```
/static/js/app.xxx.js              - 主应用文件
/static/js/chunk-vendors.xxx.js    - 第三方库
<div id="app">                     - DOM 特征
```

### React
```
/static/js/main.xxx.js             - 主文件
/static/js/*.chunk.js              - 代码分割
<div id="root">                    - DOM 特征
```
