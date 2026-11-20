# 敏感信息检测正则表达式模式库

## 一、认证凭据类

### 1.1 密码字段
```regex
(?i)(password|passwd|pwd|pass)\s*[=:]\s*["']?([^"'\s]+)["']?
```

### 1.2 用户名
```regex
(?i)(username|user|login|name)\s*[=:]\s*["']?([^"'\s]+)["']?
```

### 1.3 数据库连接字符串
```regex
(?i)(mongodb|mysql|postgresql|sqlserver|oracle|redis|mssql):\/\/([^\s:"']+):([^\s:"']+)@?([^\s:"']+)?:?
```

### 1.4 API密钥
```regex
(?i)(api[_-]?key|apikey|api[_-]?secret)\s*[=:]\s*["']?([^"'\s]+)["']?
```

### 1.5 Token
```regex
(?i)(token|bearer|auth[_-]?token|access[_-]?token)\s*[=:]\s*["']?([^"'\s]+)["']?
```

### 1.6 JWT Token
```regex
eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*
```

### 1.7 OAuth Token
```regex
(?i)(oauth[_-]?token|access[_-]?token|refresh[_-]?token)\s*[=:]\s*["']?([a-zA-Z0-9_\-\.]+)["']?
```

### 1.8 Session ID
```regex
(?i)(session[_-]?id|phpsessid|jsessionid|asp\.net[_-]?sessionid)\s*[=:]\s*["']?([^"'\s]+)["']?
```

### 1.9 Basic Auth
```regex
basic\s+[a-zA-Z0-9+/]+=*
```

## 二、个人信息类

### 2.1 中国大陆身份证号（18位）
```regex
\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b
```

### 2.2 中国大陆身份证号（15位）
```regex
\b[1-9]\d{7}(0\d|10|11|12)(0\d|1\d|2\d|3[01])\d{2}\d{2}(?\d)\b
```

### 2.3 中国大陆手机号
```regex
\b1[3-9]\d{9}\b
```

### 2.4 中国大陆固定电话
```regex
\b0\d{2,3}-?\d{7,8}\b
```

### 2.5 邮箱地址
```regex
[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}
```

### 2.6 中文姓名（常见姓氏）
```regex
[\u4e00-\u9fa5]{2,4}(?=\s*[\u4e00-\u9fa5]{1,3}(?:密码|手机|邮箱|身份证|账号))
```

### 2.7 护照号
```regex
[EeGg]\d{8,9}
```

### 2.8 社保号（中国）
```regex
[1-9]\d{9}(?:\d{3}|\d{2}[Xx])?
```

### 2.9 驾驶证号
```regex
[1-9]\d{12,13}
```

## 三、金融信息类

### 3.1 信用卡号（主要卡组织）
```regex
\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b
```

### 3.2 银行卡号
```regex
\b[1-9]\d{15,19}\b
```

### 3.3 CVV码
```regex
(?i)(cvv|cvc|cvv2|cvc2)\s*[=:]\s*["']?(\d{3,4})["']?
```

### 3.4 有效期
```regex
(?i)(exp(?:iry)?[_-]?(?:date|month|year)|valid[_-]?through)\s*[=:]\s*["']?(\d{2}\/\d{2,4}|\d{4}-\d{2})["']?
```

### 3.5 银行名称和账号组合
```regex
(?i)(中国银行|工商银行|建设银行|农业银行|招商银行|交通银行)[^\n]{0,50}[1-9]\d{14,19}
```

## 四、网络信息类

### 4.1 IPv4地址
```regex
\b(?<![0-9.])(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(?![0-9.])\b
```

### 4.2 IPv6地址
```regex
(?i)\b(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}\b
```

### 4.3 内网IP段
```regex
(?:192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})
```

### 4.4 MAC地址
```regex
(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}
```

### 4.5 URL地址
```regex
https?:\/\/(?:www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_\+.~#?&\/=]*)
```

### 4.6 域名
```regex
\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b
```

## 五、密钥和证书类

### 5.1 RSA私钥
```regex
-----BEGIN RSA PRIVATE KEY-----
```

### 5.2 OpenSSH私钥
```regex
-----BEGIN OPENSSH PRIVATE KEY-----
```

### 5.3 私钥文件通用模式
```regex
-----BEGIN.*PRIVATE KEY-----
```

### 5.4 证书文件
```regex
-----BEGIN CERTIFICATE-----
```

### 5.5 AWS密钥
```regex
(?i)(aws[_-]?(?:access[_-]?key[_-]?id|secret[_-]?access[_-]?key))\s*[=:]\s*["']?([A-Z0-9]{20})["']?
```

### 5.6 Google API密钥
```regex
AIza[0-9A-Za-z\-_]{35}
```

### 5.7 GitHub Token
```regex
(?i)(github[_-]?(?:token|auth))\s*[=:]\s*["']?([a-f0-9]{40}|[a-zA-Z0-9_]{36,})["']?
```

### 5.8 Slack Token
```regex
xox[baprs]-[0-9a-zA-Z-]{10,}
```

### 5.9 Stripe API密钥
```regex
sk_live_[0-9a-zA-Z]{24,}
sk_test_[0-9a-zA-Z]{24,}
```

### 5.10 OpenAI API密钥
```regex
sk-[a-zA-Z0-9]{20,}
```

### 5.11 Facebook Access Token
```regex
EAACEdEose0cBA[0-9A-Za-z]+
```

### 5.12 Twilio API密钥
```regex
SK[a-f0-9]{32}
```

### 5.13 SendGrid API密钥
```regex
SG\.[a-zA-Z0-9_-]{22}\.[a-zA-Z0-9_-]{43}
```

## 六、配置文件类

### 6.1 .env文件变量
```regex
(?i)^[A-Z_]+=(.+)$
```

### 6.2 JSON配置项
```regex
"(?:password|api_key|token|secret|key)":\s*"(.{0,100})"
```

### 6.3 XML配置项
```regex
<(?:password|api_key|token|secret|key)>([^<]+)</
```

### 6.4 YAML配置项
```regex
(?i)(?:password|api_key|token|secret|key):\s+(.+)
```

## 七、日志信息类

### 7.1 SQL查询语句
```regex
(?i)(select|insert|update|delete|drop|create)\s+.+from\s+[a-zA-Z0-9_]+
```

### 7.2 错误堆栈信息
```regex
(?:Exception|Error|Throwable) in thread
```

### 7.3 IP地址+端口组合
```regex
(?:\d{1,3}\.){3}\d{1,3}:\d{1,5}
```

### 7.4 User-Agent字符串
```regex
User-Agent:\s*([^\n]+)
```

### 7.5 Cookie信息
```regex
(?i)Cookie:\s*([^;\n]+)
```

## 八、JSON Web Token (JWT) 详解

### 8.1 JWT结构
JWT由三部分组成，用点号分隔：
```
xxxxx.yyyyy.zzzzz
```

### 8.2 JWT正则表达式
```regex
eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*
```

### 8.3 JWT各部分特征
- Header: 以`eyJ`开头，通常是base64编码
- Payload: 以`eyJ`开头，包含用户信息
- Signature: 签名部分

## 九、Base64编码的敏感信息

### 9.1 Base64编码字符串检测
```regex
^[A-Za-z0-9+/]+={0,2}$
```

### 9.2 Base64编码的密钥
```regex
(?:secret|key|token|pass|cred)[_]?(?:encoded)?\s*[=:]\s*['"]?([A-Za-z0-9+/]+={0,2})['"]?
```

## 十、常见云服务提供商密钥模式

### 10.1 AWS Access Key
```
AKIA[0-9A-Z]{16}
```

### 10.2 AWS Secret Key
```
[A-Za-z0-9/+=]{40}
```

### 10.3 Azure Storage Account Key
```
DefaultEndpointsProtocol=https;AccountName=([^;]+);AccountKey=([^;]+)
```

### 10.4 Google Cloud Service Account
```
"type": "service_account"
```

### 10.5 阿里云AccessKey
```
LTAI[0-9a-zA-Z]{20,}
```

### 10.6 腾讯云SecretId
```
AKID[0-9a-zA-Z]{10,}
```

### 10.7 华为云AccessKey
```
HW[0-9a-zA-Z]{10,}
```

### 10.8 高德地图/阿里云地图API Key
```
[a-zA-Z0-9]{32}
```
或搜索：
```regex
(?i)(amap|gaode|高德|ali[_-]?map)[_-]?(key|api[_-]?key)\s*[=:]\s*["']?([a-zA-Z0-9]{16,32})["']?
```

### 10.9 百度地图API Key
```
(?i)(baidu|bdmap|百度)[_-]?(key|api[_-]?key|ak)\s*[=:]\s*["']?([a-zA-Z0-9]{20,32})["']?
```
常见格式：
```
[A-K][a-zA-Z0-9]{19,}  # 服务端AK
[A-G][a-zA-Z0-9]{15,}  # 浏览器端AK
[A-G][a-zA-Z0-9]{15,}  # Android/iOS端AK
```

### 10.10 腾讯地图/腾讯位置服务API Key
```
(?i)(tencent|qq|腾讯)[_-]?(map|地图)[_-]?(key|api[_-]?key)\s*[=:]\s*["']?([a-zA-Z0-9]{20,})["']?
```
常见格式：
```
[A-Za-z0-9]{20,}  # 腾讯地图Key
```

### 10.11 Google Maps API Key
```
AIza[0-9A-Za-z\-_]{35}
```
搜索模式：
```regex
(?i)(google[_-]?maps?|maps?)[_-]?(api[_-]?key|key)\s*[=:]\s*["']?(AIza[0-9A-Za-z\-_]{35})["']?
```

### 10.12 地图SDK密钥配置模式
```regex
(?i)(map[s]?|地图)[_-]?(sdk|key|secret|token)\s*[=:]\s*["']?([^"'\s]{10,})["']?
```

## 十一、综合扫描命令

### 11.1 使用ripgrep扫描多种模式
```bash
# 扫描密码和密钥
rg -i "(password|passwd|pwd)\s*[=:]\s*['\"]" .

# 扫描API密钥
rg -i "(api[_-]?key|apikey|api[_-]?secret)" .

# 扫描Token
rg -i "(token|bearer|auth[_-]?token)" .

# 扫描JWT
rg "eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*" .

# 扫描私钥
rg "-----BEGIN.*PRIVATE KEY-----" .

# 扫描身份证号
rg "\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b" .

# 扫描手机号
rg "\b1[3-9]\d{9}\b" .

# 扫描邮箱
rg "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}" .
```

### 11.2 文件类型过滤
```bash
# 仅扫描JavaScript文件
rg -t js "password|api_key" .

# 仅扫描JSON文件
rg -t json "api_key|token" .

# 扫描多种文件类型
rg -t js -t ts -t html -t json "secret|key|token" .
```

### 11.3 排除特定文件
```bash
# 排除node_modules
rg -t js "password" . --glob '!node_modules/**'

# 排除.min.js
rg "api_key" . --glob '!*.min.js'

# 排除测试文件
rg "password" . --glob '!*.test.*' --glob '!*.spec.*'
```

## 十二、性能优化建议

### 12.1 使用本地正则表达式
避免使用复杂的环视断言，选择更简单但效果相当的正则表达式。

### 12.2 分批扫描
对于大型代码库，按目录或文件类型分批扫描，减少内存占用。

### 12.3 增量扫描
使用文件监控工具，仅扫描新修改的文件。

### 12.4 预过滤
使用grep或其他工具先过滤明显不包含敏感信息的文件（如图片、二进制文件）。

## 十三、误报处理

### 13.1 常见误报场景
- 示例代码中的占位符（如`password: '***'`）
- 脱敏的测试数据
- 注释中的示例

### 13.2 验证方法
1. 检查上下文，确认是否为真实凭据
2. 尝试使用发现的密钥/密码（需获得授权）
3. 检查是否为测试环境数据
4. 确认是否已过期

### 13.3 排除规则
```regex
# 排除占位符模式
(?i)(password|passwd)\s*[=:]\s*["']?(?:\*+|xxx|000|123456|password|changeme)["']?

# 排除注释中的示例
#.*password\s*[=:]\s*['"][^'"]+['"]
```

## 十四、工具集成

### 14.1 与GitHub Actions集成
```yaml
- name: Scan for secrets
  run: |
    docker run -v "${{ github.workspace }}:/workspace" trufflehogh three --only-check 10
```

### 14.2 与CI/CD集成
在构建流程中加入敏感信息扫描步骤，防止敏感信息进入代码库。

### 14.3 与IDE集成
配置IDE插件，实时扫描代码中的敏感信息。

## 十五、更新维护

### 15.1 更新频率
- 云服务提供商密钥格式：按需更新
- 正则表达式模式：根据实际扫描结果优化
- 误报处理规则：持续迭代

### 15.2 社区资源
- TruffleHog项目：https://github.com/trufflesecurity/trufflehog
- Gitleaks项目：https://github.com/gitleaks/gitleaks
- SecretScanner项目：https://github.com/deepfence/SecretScanner
