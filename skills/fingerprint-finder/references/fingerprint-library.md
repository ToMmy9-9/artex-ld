# 指纹库建设

## 1. CMS 指纹库

| CMS | 指纹特征 | 默认后台 |
|-----|---------|---------|
| WordPress | /wp-content/, /wp-admin/, generator meta | /wp-admin/ |
| Drupal | /modules/, /sites/default/, X-Generator | /user/login |
| Joomla | /administrator/, /templates/, component | /administrator/ |
| DedeCMS | /dede/, /uploads/, powered by DedeCMS | /dede/login.php |
| Discuz | /static/, /uc_server/, /forum.php | /admin.php |
| ECShop | /includs/, /admin/, ECSHOP | /admin/index.php |

## 2. 框架指纹库

| 框架 | 指纹特征 | 默认后台 |
|------|---------|---------|
| Spring Boot | /actuator/, X-Application-Context | /login |
| ThinkPHP | /index.php?s=, X-ThinkPHP-Version | /admin |
| Laravel | /vendor/, /storage/, laravel_session | /admin |
| Django | /admin/, /static/admin/, csrfmiddlewaretoken | /admin/ |
| Flask | SECRET_KEY, /static/, debug=true | 无默认 |

## 3. 组件指纹库

| 组件 | 指纹特征 | 默认端口 |
|------|---------|---------|
| phpMyAdmin | /phpmyadmin/, /pma/ | 80/443 |
| Redis | 无 HTTP 界面 | 6379 |
| Elasticsearch | /_cluster/health, /_nodes | 9200 |
| MongoDB | 无默认界面 | 27017 |
| RabbitMQ | /rabbitmq/, /management | 15672 |
| Jenkins | /login, /asynchPeople, crumb issuer | 8080 |
| UReport2（Java · bstek） | /ureport/designer/, /ureport/datasource/previewData | 随宿主 |

## 4. 服务器组件

### Nginx
```
响应头：Server: nginx
默认页面：Welcome to nginx!
403 页面特征
```

### Apache
```
响应头：Server: Apache
.htaccess 文件
/server-status
/server-info
```

### Tomcat
```
/manager/html/              - 管理后台
/host-manager/html/         - 虚拟主机管理
/tomcat.png                 - 默认图标
/manager/status             - 状态页面
```

### IIS
```
响应头：Server: Microsoft-IIS
404 页面：404 Object Not Found
X-Powered-By: ASP.NET
```

## 5. 中间件组件

### RabbitMQ
```
/rabbitmq/                  - 管理界面
/management/
```

### Elasticsearch
```
/_head/                     - Elasticsearch Head
/_plugin/head/
/_kibana/
/elasticsearch/
```

## 6. 数据库组件

### phpMyAdmin
```
/phpmyadmin/
/pma/
/mysql/
/admin/pma/
```

### Adminer
```
/adminer/
/adminer.php
/db/
/database/
```
