# 文件扫描完整指南

## 目录

1. [扫描前的准备工作](#扫描前的准备工作)
2. [不同文件类型的扫描策略](#不同文件类型的扫描策略)
3. [扫描执行流程](#扫描执行流程)
4. [结果分析](#结果分析)
5. [特殊场景处理](#特殊场景处理)
6. [自动化脚本](#自动化脚本)

## 扫描前的准备工作

### 1.1 确定扫描范围

#### 明确目标
- Web应用前端代码
- API接口代码
- 配置文件
- 日志文件
- 数据库备份

#### 获取权限
```bash
# 确认扫描授权范围
- 明确允许扫描的目录
- 确认可以测试的文件类型
- 了解是否有特殊的限制要求
```

#### 信息收集
```bash
# 收集目标信息
- 确定Web应用的URL
- 识别使用的技术栈
- 收集可能的子域名
- 了解目录结构
```

### 1.2 环境准备

#### 工具安装
```bash
# ripgrep - 快速文本搜索
choco install ripgrep  # Windows
brew install rg        # macOS
apt install ripgrep    # Linux

# gitrob - Git历史扫描
go install github.com/michenriksen/gitrob/cmd/gitrob@latest

# trufflehog - 密钥扫描
docker pull trufflesecurity/trufflehog:latest

# gitleaks - Git仓库扫描
brew install gitleaks  # macOS
```

#### 目录结构检查
```bash
# 查看项目结构
Get-ChildItem -Recurse -Depth 2

# 识别主要文件类型
$jsFiles = Get-ChildItem -Recurse -Include *.js,*.jsx,*.ts,*.tsx
$configFiles = Get-ChildItem -Recurse -Include *.json,*.yaml,*.yml,*.env
$htmlFiles = Get-ChildItem -Recurse -Include *.html,*.htm

Write-Host "JS文件数量: $($jsFiles.Count)"
Write-Host "配置文件数量: $($configFiles.Count)"
Write-Host "HTML文件数量: $($htmlFiles.Count)"
```

### 1.3 扫描计划

#### 优先级排序
```
P0 - 立即检查（最高风险）
├── 数据库连接字符串
├── 云服务API密钥
├── 私钥文件
└── 生产环境密码

P1 - 重点检查（高风险）
├── API密钥和Token
├── JWT Token
├── Session ID
└── 硬编码密码

P2 - 详细检查（中风险）
├── 身份证号
├── 手机号
├── 邮箱地址
└── 姓名信息

P3 - 辅助检查（低风险）
├── 内网IP地址
├── 调试信息
└── 注释中的示例
```

#### 时间规划
```bash
# 小型项目 (< 100个文件)
预计时间: 30分钟 - 1小时
扫描策略: 全量扫描

# 中型项目 (100-1000个文件)
预计时间: 1-3小时
扫描策略: 分层扫描

# 大型项目 (> 1000个文件)
预计时间: 3-8小时
扫描策略: 重点扫描 + 抽样验证
```

## 不同文件类型的扫描策略

### 2.1 JavaScript/TypeScript文件

#### 特点分析
```
优势:
- 最容易泄露敏感信息
- 通常包含前端逻辑和API调用
- 可能包含硬编码的API密钥

风险点:
- 前端代码对用户完全可见
- 压缩混淆的代码中也可能泄露信息
- Webpack等打包工具可能内嵌敏感配置
```

#### 扫描重点
```bash
# 1. 扫描所有JS文件
rg -t js -i "password|api_key|token|secret" .

# 2. 扫描.env相关变量
rg -t js "(AWS|API|STRIPE|SLACK)_KEY|TOKEN|SECRET" .

# 3. 扫描JWT
rg -t js "eyJ[a-zA-Z0-9_-]*\." .

# 4. 扫描API端点
rg -t js "fetch\(|axios\(|http\." .

# 5. 扫描localStorage/sessionStorage
rg -t js "localStorage|sessionStorage|getItem|setItem" .

# 6. 扫描Cookie操作
rg -t js "document\.cookie|Cookie" .

# 7. 扫描注释中的敏感信息
rg -t js "#.*password|TODO.*key|FIXME.*secret" .

# 8. 扫描配置对象
rg -t js "(config|options|settings)\s*[=:]\s*\{[^}]*password" .
```

#### 实际案例
```javascript
// 案例1: 硬编码的API密钥
const API_KEY = "sk_live_<STRIPE_KEY_EXAMPLE>";
const STRIPE_KEY = "sk_live_<STRIPE_KEY_EXAMPLE>";

// 案例2: 硬编码的数据库连接
const dbConfig = {
  host: "db.example.com",
  user: "admin",
  password: "SuperSecret123!",
  database: "production"
};

// 案例3: JWT Token泄露
const token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...";

// 案例4: localStorage中的敏感信息
localStorage.setItem('user_token', 'ghp_<GITHUB_TOKEN_EXAMPLE>');
```

#### 检测命令
```bash
# 扫描未压缩的JS文件
Get-ChildItem -Recurse -Include *.js,*.jsx | Where-Object {
  $_.Length -lt 1MB -and $_.Name -notmatch "\.min\."
} | ForEach-Object {
  rg -n $_.FullName -i "(password|api[_-]?key|secret|token)"
}

# 扫描压缩的JS文件
Get-ChildItem -Recurse -Include *.min.js | ForEach-Object {
  rg -n $_.FullName "(sk_live_|AKIA|ghp_|xox[baprs]-)"
}
```

### 2.2 HTML文件

#### 扫描重点
```bash
# 1. 扫描隐藏表单字段
rg -t html '<input[^>]*type=["\']?hidden["\']?[^>]*value=["\']?([^"\']+)["\']?'

# 2. 扫描注释中的敏感信息
rg -t html "<!--.*(password|key|secret|token).*-->"

# 3. 扫描JavaScript代码块
rg -t html '<script[^>]*>[\s\S]*?(password|api_key|token)[\s\S]*?</script>'

# 4. 扫描meta标签
rg -t html '<meta[^>]*(password|key|token)[^>]*content=["\']?([^"\']+)["\']?'

# 5. 扫描内联样式
rg -t html 'style=["\'][^"\']*(password|key|token)[^"\']*["\']'

# 6. 扫描base64编码的数据
rg -t html 'data:[^;]+;base64,[A-Za-z0-9+/]+={0,2}'
```

#### 常见泄露点
```html
<!-- 注释中的密码 -->
<!-- Database password: admin123 -->

<!-- 隐藏表单字段 -->
<input type="hidden" name="api_key" value="sk_live_xxxxx">

<!-- 内联JS配置 -->
<script>
  const config = {
    apiUrl: 'https://api.example.com',
    apiKey: 'AIzaSyDxxxxxxxxxxxxxxxxxxxxx',
    debug: true
  };
</script>

<!-- base64编码的密钥 -->
<img src="data:image/png;base64,SGVsbG8gV29ybGQh...">

<!-- 调试信息 -->
<div style="display:none">
  Server: db.prod.internal:3306
  User: root
  Pass: toor
</div>
```

### 2.3 JSON配置文件

#### 扫描重点
```bash
# 1. 扫描所有JSON文件
Get-ChildItem -Recurse -Include *.json | ForEach-Object {
  rg -n $_.FullName '"(password|api_key|token|secret|key)":\s*"'
}

# 2. 扫描AWS配置
rg -t json "aws_access_key_id|aws_secret_access_key"

# 3. 扫描数据库配置
rg -t json '"(mongodb|mysql|postgres|redis|sqlserver)":\s*"'

# 4. 扫描第三方服务配置
rg -t json '"(stripe|sendgrid|twilio|slack)":\s*"'

# 5. 扫描环境变量定义
rg -t json "^\s*\"(NODE_ENV|PORT|HOST)\":"
```

#### 常见泄露点
```json
// package.json中的scripts
{
  "scripts": {
    "deploy": "firebase deploy --token '1//abcdefghijklmnopqrstuvwxyz123456789'"
  }
}

// config.json
{
  "database": {
    "host": "localhost",
    "username": "admin",
    "password": "MySecretPassword123"
  },
  "apiKeys": {
    "googleMaps": "AIzaSyDxxxxxxxxxxxxxxxxxxxxx",
    "stripe": "sk_live_<STRIPE_KEY_EXAMPLE>"
  }
}

// .firebaserc
{
  "projects": {
    "default": "myapp-12345"
  },
  "tokens": {
    "deploy": "1//0gAxxxxxxxxxxxxxxxxxxxxx"
  }
}
```

### 2.4 YAML配置文件

#### 扫描重点
```bash
# 1. 扫描YAML文件
rg -t yaml -i "password:|api_key:|token:|secret:"

# 2. 扫描数据库配置
rg -t yaml -i "mongodb:|mysql:|postgres:|redis:"

# 3. 扫描环境变量
rg -t yaml "ENV:|environment:"

# 4. 扫描凭证配置
rg -t yaml "(aws_access_key|aws_secret|github_token|slack_token):"
```

#### 常见泄露点
```yaml
# docker-compose.yml
version: '3'
services:
  db:
    image: mysql:5.7
    environment:
      MYSQL_ROOT_PASSWORD: "SuperSecret123!"
      MYSQL_DATABASE: "app"
      MYSQL_USER: "admin"
      MYSQL_PASSWORD: "UserPassword456"

# GitHub Actions
name: Deploy
on: [push]
jobs:
  deploy:
    steps:
      - uses: actions/checkout@v2
      - name: Deploy
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          STRIPE_SECRET_KEY: sk_live_<STRIPE_KEY_EXAMPLE>

# Kubernetes secret
apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
type: Opaque
stringData:
  password: "MyDatabasePassword123!"
  api-key: "sk_live_<STRIPE_KEY_EXAMPLE>"
```

### 2.5 XML配置文件

#### 扫描重点
```bash
# 1. 扫描XML文件中的密码
rg -t xml -i "<password>|<passwd>"

# 2. 扫描数据库连接配置
rg -t xml "connectionstring|database|server|host"

# 3. 扫描API密钥
rg -t xml "<api[_-]?key>|<token>|<secret>"
```

#### 常见泄露点
```xml
<!-- web.config -->
<configuration>
  <connectionStrings>
    <add name="MyDb"
         connectionString="Server=db.example.com;Database=app;User=admin;Password=Secret123;" />
  </connectionStrings>
  <appSettings>
    <add key="ApiKey" value="sk_live_<STRIPE_KEY_EXAMPLE>" />
  </appSettings>
</configuration>

<!-- Spring配置 -->
<bean id="dataSource" class="org.springframework.jdbc.datasource.DriverManagerDataSource">
  <property name="driverClassName" value="com.mysql.cj.jdbc.Driver"/>
  <property name="url" value="jdbc:mysql://localhost:3306/app"/>
  <property name="username" value="root"/>
  <property name="password" value="password123"/>
</bean>
```

### 2.6 ENV环境变量文件

#### 扫描重点
```bash
# 1. 扫描所有ENV文件
rg -t env "^(AWS|API|STRIPE|SENDGRID|SLACK|DB|PASSWORD|SECRET|TOKEN|KEY)="

# 2. 扫描数据库相关
rg -t env "^DATABASE_|^DB_"

# 3. 扫描云服务配置
rg -t env "^AWS_|^AZURE_|^GCP_|^HEROKU_"

# 4. 扫描第三方服务
rg -t env "^(STRIPE|SENDGRID|TWILIO|SLACK|REDIS)"

# 5. 扫描私钥配置
rg -t env "PRIVATE_|SECRET_|CREDENTIAL"
```

#### 常见泄露点
```env
# .env文件示例
NODE_ENV=production
PORT=3000

# 数据库配置
DATABASE_URL=postgres://admin:Password123@db.example.com:5432/app
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASS=SuperSecretPassword

# AWS配置
AWS_ACCESS_KEY_ID=AKIA<AWS_KEY_ID_EXAMPLE>
AWS_SECRET_ACCESS_KEY=<AWS_SECRET_KEY_EXAMPLE>
AWS_REGION=us-east-1

# 第三方服务
STRIPE_SECRET_KEY=sk_live_<STRIPE_KEY_EXAMPLE>
SENDGRID_API_KEY=SG.<SENDGRID_KEY_EXAMPLE>
SLACK_BOT_TOKEN=xoxb-<SLACK_TOKEN_EXAMPLE>
TWILIO_ACCOUNT_SID=AC<TWILIO_SID_EXAMPLE>
TWILIO_AUTH_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# JWT密钥
JWT_SECRET=<JWT_SECRET_EXAMPLE>
JWT_EXPIRY=7d

# Redis
REDIS_URL=redis://:<REDIS_PASSWORD>@redis.example.com:6379
```

### 2.7 日志文件

#### 扫描重点
```bash
# 1. 扫描错误日志中的敏感信息
rg -t log -i "password|token|key|secret" .

# 2. 扫描SQL查询日志
rg -t log -i "select.*from.*where.*password"

# 3. 扫描认证日志
rg -t log "login|authenticate|token|session"

# 4. 扫描数据库连接日志
rg -t log "(mongodb|mysql|postgres):\/\/"

# 5. 扫描异常堆栈
rg -t log "(Exception|Error|Throwable).*"
```

#### 常见泄露点
```
# access.log示例
192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "POST /api/login HTTP/1.1" 200 123
  {"username":"admin","password":"SuperSecret123"}

# error.log示例
ERROR 2024-01-01 12:00:00 Database connection failed
  Host: db.prod.internal
  User: admin
  Password: Password123
  Stack: at Connection.connect() in db.js:42

# sql.log示例
2024-01-01 12:00:00 SELECT * FROM users WHERE username='admin' AND password='md5hashhere'

# debug.log示例
[DEBUG] Stripe API call:
  Method: POST
  URL: https://api.stripe.com/v1/charges
  Headers: Authorization: Bearer sk_live_<STRIPE_KEY_EXAMPLE>
```

### 2.8 数据库备份文件

#### 扫描重点
```bash
# 1. 扫描SQL备份文件
rg -t sql -i "insert into.*users.*values.*password"

# 2. 扫描数据库导出文件
rg -i "create table.*users" --glob "*.sql"

# 3. 扫描CSV导出
rg -t csv "password|email|phone" .

# 4. 扫描MongoDB导出
rg -i "\"(?:password|token|secret)\"" --glob "*.json" .
```

#### 常见泄露点
```sql
-- user_data.sql
INSERT INTO users (id, username, email, password, phone, id_card) VALUES
(1, 'admin', 'admin@example.com', '5f4dcc3b5aa765d61d8327deb882cf99', '13800138000', '110101199001011234');

-- backup.sql
-- 用户表备份
CREATE TABLE users_backup AS SELECT * FROM users;
-- admin账户
INSERT INTO users_backup VALUES (999, 'root', 'root@example.com', 'P@ssw0rd123!', '13900000000', '110101199001011234');
```

## 扫描执行流程

### 3.1 初步扫描（P0级）

#### 步骤1: 扫描最高风险项
```bash
# 并行扫描最高风险项
# 1. 私钥文件
rg "-----BEGIN.*PRIVATE KEY-----" .

# 2. 数据库连接字符串
rg -i "(mongodb|mysql|postgres|sqlserver):\/\/" .

# 3. 云服务密钥
rg -i "AKIA|AWS_ACCESS_KEY|SG\.|xoxb-|sk_live_" .

# 4. JWT Token
rg "eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*" .

# 5. 硬编码密码
rg -i "password\s*[=:]\s*[\"'][^\"']+[\"']"
```

#### 步骤2: 快速评估风险等级
```
高风险 (> 10个发现):
└── 立即报告，可能存在系统性配置问题

中风险 (1-10个发现):
└── 详细分析，评估实际影响

低风险 (0个发现):
└── 继续深度扫描
```

### 3.2 深度扫描（P1-P2级）

#### 步骤3: 扫描个人信息
```bash
# 身份证号
rg "\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b"

# 手机号
rg "\b1[3-9]\d{9}\b"

# 邮箱
rg "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"
```

#### 步骤4: 扫描API和Token
```bash
# 扫描所有Token
rg -i "(token|bearer|auth|oauth)\s*[=:]\s*[\"'][^\"']+[\"']"

# 扫描Session ID
rg -i "(session[_-]?id|phpsessid|jsessionid)\s*[=:]\s*[\"'][^\"']+[\"']"

# 扫描API密钥
rg -i "(api[_-]?key|apikey|api[_-]?secret)\s*[=:]\s*[\"'][^\"']+[\"']"
```

#### 步骤5: 扫描配置文件
```bash
# 扫描ENV文件
Get-ChildItem -Recurse -Include .env*,*env* | ForEach-Object {
  rg -n $_.FullName "^(AWS|DATABASE|DB_|PASSWORD|SECRET|TOKEN|KEY|PRIVATE)="
}

# 扫描YAML配置
Get-ChildItem -Recurse -Include *.yaml,*.yml | ForEach-Object {
  rg -n $_.FullName -i "(password|key|secret|token):"
}

# 扫描JSON配置
Get-ChildItem -Recurse -Include config*.json,settings*.json | ForEach-Object {
  rg -n $_.FullName '"(password|api_key|token|secret)":\s*"'
}
```

### 3.3 辅助扫描（P3级）

#### 步骤6: 扫描低风险项
```bash
# 扫描IP地址
rg -n "\b(?:\d{1,3}\.){3}\d{1,3}\b"

# 扫描内网IP段
rg -n "(192\.168\.|10\.|172\.(?:1[6-9]|2\d|3[01])\.)"

# 扫描调试信息
rg -i "debug|console\.log|printStackTrace"
```

### 3.4 特殊场景扫描

#### 步骤7: Git历史扫描
```bash
# 使用gitleaks扫描Git历史
gitleaks --repo-path=. --verbose

# 使用trufflehog扫描Git历史
docker run -v "$(pwd):/scan" trufflesecurity/trufflehog:latest filesystem /scan

# 使用gitrob扫描
gitrob clone <repo_url>
gitrob analyze -commits
```

#### 步骤8: 扫描第三方依赖
```bash
# 检查package-lock.json
rg package-lock.json -i "(password|token|key|secret)"

# 检查yarn.lock
rg yarn.lock -i "(password|token|key|secret)"

# 检查composer.lock
rg composer.lock -i "(password|token|key|secret)"
```

## 结果分析

### 4.1 分类整理

#### 创建发现列表
```json
{
  "scan_date": "2024-01-01 12:00:00",
  "scan_target": "/path/to/project",
  "total_files_scanned": 150,
  "findings": [
    {
      "id": 1,
      "severity": "CRITICAL",
      "type": "数据库连接字符串",
      "file": "config/database.js",
      "line": 42,
      "match": "mysql://admin:Password123@db.example.com:3306",
      "context": "const dbConfig = {...",
      "recommendation": "使用环境变量替代硬编码"
    }
  ]
}
```

#### 风险评估矩阵
```
严重程度 x 可能性 = 风险等级

严重程度:
- 高: 直接导致安全事件
- 中: 可能导致安全事件
- 低: 理论风险

可能性:
- 高: 容易被利用
- 中: 需要特定条件
- 低: 难以利用
```

### 4.2 验证发现

#### 真实性验证
```bash
# 检查是否为真实凭据
# 1. 查看上下文
# 2. 尝试访问对应服务（需授权）
# 3. 检查是否为测试数据
# 4. 确认是否已过期

# 示例：验证数据库连接字符串
# 连接数据库测试（需要授权）
mysql -h db.example.com -u admin -p'Password123'

# 验证API密钥
curl -H "Authorization: Bearer <API_KEY>" https://api.example.com/v1/user
```

#### 误报识别
```
常见误报:
- 注释中的示例代码
- 已加密的配置文件
- Base64编码的测试数据
- 占位符文本（如 "your-api-key"）
- 公共信息（如开源项目中的示例）
```

### 4.3 编写报告

#### 报告结构
```
1. 执行摘要
   - 扫描范围
   - 发现数量
   - 风险等级

2. 详细发现
   - 每个发现的详细信息
   - 复现步骤
   - 影响分析

3. 修复建议
   - 短期措施
   - 长期建议

4. 附录
   - 使用的工具和命令
   - 扫描配置
```

## 特殊场景处理

### 5.1 加密文件处理

#### 检测加密文件
```bash
# 查找加密文件
Get-ChildItem -Recurse | Where-Object {
  $_.Extension -in @('.gpg', '.pgp', '.enc', '.encrypted')
}

# 查找密码文件
Get-ChildItem -Recurse -Include "*.kdbx", "*.key", "*.pfx", "*.p12"
```

#### 注意事项
```
- 不要尝试暴力破解加密文件
- 如果有密钥，尝试使用密钥解密
- 记录加密文件的位置供后续分析
```

### 5.2 大文件处理

#### 分块扫描
```bash
# 分割大文件
Get-ChildItem -Recurse | Where-Object { $_.Length -gt 100MB } | ForEach-Object {
  Write-Host "大文件: $($_.FullName) - 大小: $($_.Length / 1MB) MB"
}

# 扫描大文件的指定行
Get-Content largefile.js -TotalCount 1000 | Out-File temp.txt
rg "password" temp.txt
Remove-Item temp.txt
```

### 5.3 二进制文件处理

#### 提取字符串
```bash
# 使用strings提取字符串
# Windows
strings.exe binaryfile.dll | rg "password"

# macOS/Linux
strings binaryfile.dll | rg "password"

# 扫描二进制文件中的字符串
Get-ChildItem -Recurse -Include *.dll,*.so,*.dylib | ForEach-Object {
  strings $_.FullName | rg "(password|key|token|secret)"
}
```

### 5.4 Git历史深度扫描

#### 完整历史扫描
```bash
# 克隆仓库
git clone <repo_url>

# 扫描所有分支和标签
git log --all --source --remotes --tags

# 使用gitleaks完整扫描
gitleaks --repo-path=. --verbose --report-path=gitleaks-report.json

# 使用trufflehog扫描
docker run -v "$(pwd):/scan" trufflesecurity/trufflehog:latest git file:///scan
```

### 5.5 容器镜像扫描

#### Dockerfile分析
```bash
# 扫描Dockerfile
rg -t dockerfile "ENV.*(PASSWORD|KEY|TOKEN|SECRET|PRIVATE)"

# 扫描.dockerignore
Get-Content .dockerignore

# 扫描构建产物
docker history <image_id>
docker run <image_id> env
```

#### 镜像内容提取
```bash
# 导出镜像
docker save <image> -o image.tar

# 提取内容
tar -xf image.tar -C ./extracted

# 分析镜像层
Get-ChildItem ./extracted -Recurse | ForEach-Object {
  if ($_.Extension -eq ".json") {
    rg $_.FullName -i "(password|key|token|secret)"
  }
}
```

## 自动化脚本

### 6.1 完整扫描脚本

```powershell
# sensitive-scan.ps1
param(
    [string]$TargetPath = ".",
    [string]$OutputFile = "scan-results.json",
    [switch]$DeepScan,
    [switch]$IncludeGitHistory
)

$ErrorActionPreference = "SilentlyContinue"

# 初始化结果
$results = @{
    scan_date = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    target = $TargetPath
    total_files = 0
    findings = @()
}

# 定义扫描模式
$modes = @{
    critical = @(
        "-----BEGIN.*PRIVATE KEY-----",
        "(mongodb|mysql|postgres|sqlserver):\/\/",
        "AKIA[A-Z0-9]{16}",
        "sk_live_[a-zA-Z0-9]{24,}",
        "eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*"
    )
    high = @(
        "(?i)(password|passwd|pwd)\s*[=:]\s*[\"'][^\"']+[\"']",
        "(?i)(api[_-]?key|apikey)\s*[=:]\s*[\"'][^\"']+[\"']",
        "(?i)(token|bearer)\s*[=:]\s*[\"'][^\"']+[\"']"
    )
    medium = @(
        "\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b",
        "\b1[3-9]\d{9}\b",
        "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"
    )
}

# 文件扩展名
$extensions = @('*.js', '*.jsx', '*.ts', '*.tsx', '*.html', '*.json', 
                 '*.xml', '*.yaml', '*.yml', '*.env', '*.css', '*.java',
                 '*.php', '*.py', '*.rb', '*.go', '*.sql', '*.log')

# 执行扫描
Write-Host "开始扫描: $TargetPath" -ForegroundColor Cyan

$files = Get-ChildItem -Path $TargetPath -Recurse -Include $extensions
$results.total_files = $files.Count

$severityMap = @{
    critical = "CRITICAL"
    high = "HIGH"
    medium = "MEDIUM"
}

foreach ($file in $files) {
    Write-Host "扫描文件: $($file.FullName)" -ForegroundColor Gray
    
    $content = Get-Content $file.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    
    foreach ($severity in $modes.Keys) {
        foreach ($pattern in $modes[$severity]) {
            $matches = [regex]::Matches($content, $pattern)
            foreach ($match in $matches) {
                $lineNumber = ($content.Substring(0, $match.Index) -split "`n").Count
                
                $results.findings += @{
                    severity = $severityMap[$severity]
                    file = $file.FullName
                    line = $lineNumber
                    match = $match.Value.Substring(0, [Math]::Min(50, $match.Value.Length))
                    pattern = $pattern
                }
            }
        }
    }
}

# 输出结果
$results | ConvertTo-Json -Depth 10 | Out-File -FilePath $OutputFile -Encoding UTF8

Write-Host "`n扫描完成!" -ForegroundColor Green
Write-Host "总文件数: $($results.total_files)" -ForegroundColor Yellow
Write-Host "发现数量: $($results.findings.Count)" -ForegroundColor Yellow
Write-Host "结果已保存到: $OutputFile" -ForegroundColor Cyan

# 显示统计
$severityCounts = @{}
foreach ($finding in $results.findings) {
    $severity = $finding.severity
    if (-not $severityCounts.ContainsKey($severity)) {
        $severityCounts[$severity] = 0
    }
    $severityCounts[$severity]++
}

Write-Host "`n风险分布:" -ForegroundColor Cyan
foreach ($severity in $severityCounts.Keys) {
    $count = $severityCounts[$severity]
    $color = switch ($severity) {
        "CRITICAL" { "Red" }
        "HIGH" { "Yellow" }
        "MEDIUM" { "Magenta" }
        default { "White" }
    }
    Write-Host "  $severity : $count" -ForegroundColor $color
}
```

### 6.2 使用方法

```bash
# 基本扫描
.\sensitive-scan.ps1 -TargetPath "C:\Projects\MyApp"

# 深度扫描
.\sensitive-scan.ps1 -TargetPath "." -DeepScan

# 包含Git历史扫描
.\sensitive-scan.ps1 -TargetPath "." -IncludeGitHistory -OutputFile "full-scan-results.json"

# 扫描特定目录
.\sensitive-scan.ps1 -TargetPath ".\src\components" -OutputFile "component-scan.json"
```

### 6.3 CI/CD集成

```yaml
# GitHub Actions示例
name: Secret Scanning
on: [push, pull_request]

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0  # 扫描所有历史
      
      - name: Run Gitleaks
        uses: gitleaks/gitleaks-action@v2
      
      - name: Custom Scan
        run: |
          docker run -v "${{ github.workspace }}:/scan" \
            trufflesecurity/trufflehog:latest \
            filesystem /scan --json > scan-results.json
      
      - name: Upload Results
        uses: actions/upload-artifact@v3
        with:
          name: scan-results
          path: scan-results.json
```

## 持续优化

### 7.1 规则更新

定期更新检测规则以应对新出现的泄露模式：
- 关注云服务提供商的新密钥格式
- 跟踪常见框架的新配置方式
- 分析最新安全事件中的泄露模式

### 7.2 性能监控

监控扫描性能：
- 记录扫描时间
- 优化大文件处理
- 调整并发度

### 7.3 误报优化

根据实际使用优化规则：
- 收集误报案例
- 添加排除规则
- 改进正则表达式精确度
