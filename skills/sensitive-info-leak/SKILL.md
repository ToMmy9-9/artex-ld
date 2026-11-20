---
name: "sensitive-info-leak"
description: "识别、关联和评估前端资源、接口响应、错误信息、配置、日志、文档与源码中的敏感信息暴露，并形成可复核的结构化结果。"
---

# sensitive-info-leak

## 1. 核心职责

本 Skill 处理的是**敏感信息暴露风险分析**，不是简单的字符串匹配。需要回答：

- 暴露了什么信息；
- 从哪里暴露；
- 哪些主体可以获取；
- 信息对应什么身份、服务或业务对象；
- 是否仍然有效，权限和影响范围如何；
- 是否能与其他线索组合形成更高风险；
- 是否足以形成独立 Finding，或只能作为辅助线索。

不要因为命中关键词就直接下结论，也不要因为单个线索暂时无法证明有效就丢弃它。应保留线索并补充上下文判断。

## 2. 触发场景

遇到以下任一情况时进入专项分析：

- 前端 JS、Source Map、HTML 或静态资源出现疑似密钥、Token、内部地址或调试信息；
- 接口响应、错误页、调试页、导出文件或下载文件包含不应暴露的数据；
- 配置、环境变量、日志、备份、接口文档或源码中出现凭据和内部结构；
- 客户端存储、Cookie、页面变量或初始化配置暴露会话和身份材料；
- 已发现一个敏感线索，需要判断其真实性、作用范围或与其他线索的关联；
- 多个低置信线索共同指向同一服务、账户、接口或数据集。

## 3. 分析对象

按来源定位暴露边界：

1. **公开静态资源**：JS、HTML、CSS、Source Map、构建产物、字体和资源清单；
2. **接口与网络响应**：JSON、XML、GraphQL、RPC、错误响应、调试字段和导出结果；
3. **配置与部署信息**：环境配置、连接串、容器配置、CI/CD、依赖配置和启动参数；
4. **日志与备份**：访问日志、错误日志、SQL 日志、压缩包、临时文件和历史文件；
5. **源码与文档**：注释、示例配置、接口文档、README、测试数据和版本历史；
6. **客户端状态**：Cookie、Local Storage、Session Storage、IndexedDB、页面初始化对象。

每个结果都要记录来源、可见范围、首次发现位置、关联资源和时间上下文。

## 4. 信息分类与优先级

### 4.1 身份与会话材料

包括 Session、JWT、Refresh Token、Cookie、临时凭据、用户标识和身份映射信息。

重点分析：

- 对应用户、角色或服务身份；
- 是否具有跨请求、跨页面或跨环境复用特征；
- 是否能关联到高权限操作或敏感数据；
- 是否只在本地示例、测试数据或失效上下文中出现。

### 4.2 服务访问凭据

包括数据库连接信息、云平台密钥、第三方 API Key、消息队列凭据、对象存储凭据和内部服务 Token。

重点分析：

- 对应服务、环境和租户；
- 权限范围及资源边界；
- 是否暴露主密钥、子密钥或只读凭据；
- 是否与生产域名、内部域名或管理端点关联。

### 4.3 个人与业务数据

包括身份证件信息、手机号、邮箱、账户信息、订单信息、业务记录和批量导出数据。

重点分析：

- 数据主体和数据类型；
- 单条、少量还是批量暴露；
- 是否包含可关联身份的组合字段；
- 是否存在分页、导出、搜索或接口结构导致的规模化暴露。

### 4.4 内部结构与运行信息

包括内网 IP、内部域名、服务拓扑、管理端点、文件路径、调试标志、版本信息、部署信息和源代码映射。

这类信息通常先作为线索评估；当它能够帮助定位内部服务、绕过边界、识别高价值接口或与凭据关联时，风险等级相应提高。

### 4.5 密钥、证书与加密材料

包括私钥、证书链、签名密钥、加密密钥、云厂商标识和第三方平台密钥。

重点分析密钥用途、有效期、绑定域名或账户、可访问资源和是否存在密钥复用。

## 5. 上下文判断规则

### 5.1 先判断“暴露”，再判断“风险”

记录一个字符串不等于形成漏洞。至少需要判断：

- 是否由不应看到它的访问者取得；
- 是否出现在公开、低权限或错误边界中；
- 是否指向真实系统、真实账户或真实数据；
- 是否具有可复用性、可推断性或批量获取条件；
- 是否存在占位符、示例值、测试值或公开文档语境。

### 5.2 结合字段、位置和邻近内容

同一字符串在不同上下文中的含义可能完全不同。综合判断：

- 字段名、变量名、注释和配置层级；
- 所在文件、接口、页面和功能；
- 前后文中的域名、端口、环境名、用户标识和权限字段；
- 是否同时出现请求路径、签名算法、过期时间或服务名称；
- 是否在多个资源中重复出现并保持一致。

### 5.3 处理编码、拆分和变形

关注 Base64、URL 编码、Unicode 转义、字符串拼接、分片变量、压缩配置和构建产物中的变形值。恢复后仍需回到来源上下文判断，不因解码成功就直接判定为有效凭据。

### 5.4 关联分析

将线索按以下维度聚合：

- 同一应用、域名、环境或版本；
- 同一用户、角色、租户或服务账号；
- 同一 Token、密钥、接口或数据对象；
- 同一时间窗口和同一资源链路；
- 静态资源、接口响应、错误信息之间的引用关系。

例如：公开 JS 暴露管理接口，接口返回内部服务地址，配置又出现对应访问材料，应作为一条关联链分析，而不是三个互不相关的低风险命中。

## 6. 有效性与影响评估

对每条高价值线索分别评估：

```yaml
validity: confirmed | probable | unknown | invalid
exposure: public | low_privilege | authenticated | internal_only | unknown
identity: user_or_service_identity
permission: none | read | write | admin | unknown
scope: single_record | tenant | application | environment | multiple_services | unknown
scale: single | limited | bulk | unknown
reusability: one_time | reusable | unknown
impact: confidentiality | integrity | availability | account_access | lateral_access | unknown
```

若无法确认有效性，使用 `unknown`，不要将“未知”写成“无效”。

## 7. 风险分层

### 高风险

- 可复用的高权限身份或会话材料；
- 生产数据库、云平台或内部核心服务的高权限凭据；
- 私钥、签名密钥或可影响多个服务的密钥；
- 大量个人或业务数据直接暴露；
- 暴露信息可串联到管理接口、跨租户数据或进一步访问链路。

### 中风险

- 权限有限但真实可用的 Token 或 API Key；
- 内部服务、调试配置、接口文档与其他线索组合后具有实际影响；
- 少量敏感个人或业务数据暴露；
- 有明确真实特征但有效性、权限或范围尚未完全确认的凭据。

### 低风险或线索级

- 单个内部地址、普通版本信息或公开 SDK 标识；
- 已明确为示例、占位符或失效值；
- 无法关联真实系统、身份、权限或敏感数据的随机字符串。

最终等级应由暴露边界、有效性、权限、规模和可复用性共同决定，不按关键词类型机械定级。

## 8. 误报与重复结果处理

- 公开文档中的示例值、占位符和测试值单独归为线索；
- 同一凭据在多个文件出现时合并来源，不重复创建 Finding；
- 同一接口返回多个字段时按一个暴露事件聚合，同时保留字段明细；
- 仅有格式相似性但缺乏上下文支持的内容降低置信度；
- 过期、撤销或不可复用材料仍可作为配置管理问题或历史暴露记录，但不要虚构当前影响；
- 与其他漏洞共同构成影响时，记录关联关系，避免重复计算风险。

## 9. 证据与影响依据

证据应能够证明“来源、内容类型、暴露边界和影响判断”，并保留足够上下文供复核。

建议保留：

- 来源 URL、文件路径、接口和参数；
- 响应状态、访问上下文、时间和版本；
- 命中的完整关键片段及其前后文；
- 字段名、页码、函数位置和关联资源；
- 有效性、权限和影响判断的依据。

对于凭据、Token、私钥、连接串和个人信息，按实际测试记录保留原始内容，避免在不同 Finding 中重复抄录；必要时通过引用原始证据位置复用。

## 10. 结构化输出

```yaml
finding:
  category: credential | session | personal_data | business_data | internal_info | key_material | debug_info
  title: concise finding title
  confidence: high | medium | low
  source:
    kind: javascript | html | response | config | log | backup | document | storage | source
    location: URL or file path
    context: page, endpoint, parameter, or function
  exposed_item:
    type: specific sensitive information type
    value: observed value
  exposure:
    access_level: public | low_privilege | authenticated | internal_only | unknown
    scope: single_record | application | environment | multiple_services | unknown
  assessment:
    validity: confirmed | probable | unknown | invalid
    identity: unknown
    permission: unknown
    impact: confidentiality | integrity | availability | account_access | lateral_access | unknown
    rationale: evidence-based reasoning
  relationships:
    related_sources: []
    related_endpoints: []
    related_findings: []
  evidence:
    - source_location
    - excerpt
    - response_or_file_context
  remediation:
    - revoke_or_rotate_exposed_material
    - remove_from_public_artifacts
    - restrict_access
    - review_history_and_downstream_usage
    - add_secret_management_and_preventive_checks
```

## 11. 示例：在大量 PDF 中快速定位敏感信息

### 场景

某对象存储目录、公开下载区或文档归档中包含数千个 PDF，需要快速判断哪些文件可能包含以下内容：

- 身份证件、手机号、邮箱、地址、账户或订单数据；
- 合同、报价单、发票、财务报表和客户名单；
- 内部网络地址、系统架构、运维手册、接口文档和部署信息；
- 数据库连接信息、访问 Token、云服务配置、证书或私钥；
- 历史版本、备份文档、会议纪要和故障报告中的敏感内容。

目标不是逐页阅读所有 PDF，而是先建立文件清单，再通过元数据、文本层、版式线索和 OCR 结果进行分层筛选，最后对高价值文件进行人工复核和关联分析。

### 处理流程

#### 1. 建立文件清单

为每个 PDF 记录：

```yaml
file:
  uri: object-storage/path-or-url
  name: filename.pdf
  size: bytes
  modified_at: timestamp
  content_type: application/pdf
  hash: sha256-or-unknown
  source_context: directory, listing, page, or download link
```

优先关注文件名、目录名和时间信息中出现的：`backup`、`export`、`客户`、`员工`、`财务`、`合同`、`接口`、`运维`、`架构`、`配置`、`故障`、`测试`、`历史` 等语义线索。文件名只能用于排序，不能单独作为敏感信息结论。

#### 2. 先做低成本预筛选

按照文件大小、页数、标题、作者、创建软件、修改时间、目录位置和是否存在文本层进行排序。优先处理：

- 位于备份、导出、临时、历史或内部文档目录的文件；
- 文件名表现为报表、名单、合同、配置、接口、日志或运维材料的文件；
- 最近修改、版本号较新或与当前系统组件相关的文件；
- 文本层可直接提取，或页面数量较少但信息密度较高的文件；
- 元数据、书签、附件或目录页显示出业务系统和内部环境信息的文件。

对于扫描件、图片型 PDF、字体异常或文本提取为空的文件，标记为 `ocr_required`，不要将“未提取到文本”判断为“没有敏感信息”。

#### 3. 分层提取内容

按成本逐步增加分析深度：

1. **元数据层**：文件名、标题、作者、时间、页数、附件和书签；
2. **文本层**：正文、页眉页脚、目录、表格文本、批注和隐藏文本；
3. **结构层**：表格列名、重复字段、页码范围、章节标题、二维码或条形码线索；
4. **图像层**：对扫描页、截图、印章、证件和嵌入图片进行 OCR；
5. **关联层**：将 PDF 中的域名、账号、项目名、接口路径、文件路径和时间线与其他已发现材料进行比对。

文本提取结果应保留页码映射，例如 `page_12`、`page_13`，确保后续可以准确复核和生成证据。

#### 4. 使用语义类别进行候选排序

不要只依赖单个关键词。将候选信号按类别累计：

- **身份/个人数据**：姓名、证件号、手机号、邮箱、地址、账户号等字段组合；
- **业务数据**：客户、订单、金额、合同、报价、交易、员工或供应商信息；
- **内部结构**：内网 IP、内部域名、系统名称、环境名、管理入口、部署路径；
- **凭据材料**：密码、Token、连接串、密钥、证书、私钥、签名材料；
- **运维/开发信息**：错误堆栈、日志、接口参数、版本、源码路径、架构图和配置片段。

同一页同时出现“字段名 + 多条记录 + 系统/环境标识”时，提高优先级；只有标题或目录出现关键词时，保持较低置信度并继续查看正文上下文。

#### 5. 进行文件级和批量级判断

对每个候选 PDF 分别判断：

```yaml
triage:
  sensitivity: none | low | medium | high | critical
  content_type: personal_data | business_data | credential | internal_info | mixed | unknown
  evidence_pages: [12, 13]
  record_scale: single | limited | bulk | unknown
  credential_status: confirmed | probable | unknown | not_present
  exposure_context: public | low_privilege | authenticated | internal_only | unknown
  related_files: []
  rationale: evidence-based summary
```

需要特别区分：

- 单个示例、模板或培训材料，与包含真实批量记录的文件；
- 已公开的产品手册，与暴露内部部署细节的运维文档；
- 失效的历史凭据，与仍可能复用的当前凭据；
- 同一批文件的重复导出，与不同时间点、不同环境的独立暴露。

#### 6. 对高优先级 PDF 做定向复核

复核时围绕“真实性、范围和可利用影响”展开：

- 是否存在真实主体、真实业务编号或真实环境标识；
- 数据是单条、少量还是可批量获取；
- 文档是否包含可复用身份材料或服务访问材料；
- 内容是否能定位到管理接口、内部服务、数据库或其他敏感资源；
- 是否有相同账号、域名、项目名或密钥片段出现在其他文件中；
- 文档时间是否反映当前环境，还是历史、测试或失效状态。

#### 7. 形成聚合结果和证据

同一 PDF 的多个敏感字段通常聚合为一个文件级暴露事件，并在结果中列出字段和页码。多个版本的同一文档合并来源，保留最早、最新和内容变化摘要；不同业务对象或不同权限边界则分别记录。

输出示例：

```yaml
finding:
  category: mixed
  title: 公开文档归档包含客户批量信息及内部系统细节
  confidence: high
  source:
    kind: document
    location: object-storage/reports/customer-export-2026-08.pdf
    context: pages 12-18
  exposed_item:
    type: personal_data_and_internal_info
    value: 真实姓名、手机号、邮箱及内网域名
  assessment:
    validity: confirmed
    permission: unknown
    scope: application
    scale: bulk
    impact: confidentiality
    rationale: 多页表格包含重复个人字段，并出现内部系统名称和服务地址
  evidence:
    - page_12_excerpt
    - page_15_table_headers
    - page_18_internal_service_reference
  relationships:
    related_sources: []
    related_files: []
```

证据保留必要页码、字段名、上下文和原始片段；对于大型 PDF，记录命中页和原文件位置，避免无关内容重复进入结果。

### 效率优化要点

- 先索引文件元数据，再提取文本，最后对少量候选执行 OCR；
- 对相同哈希、相同标题和相近时间的重复 PDF 去重；
- 按页保存提取结果，支持命中页的定向复核；
- 将“文件名命中”“正文命中”“字段组合命中”“OCR 命中”区分记录；
- 对候选文件使用分数排序，但最终结论必须基于上下文和证据；
- 对大批量文件优先发现规模、字段组合和暴露边界，再深入单个值的有效性；
- 将低置信候选保留为线索，避免因文本提取失败、编码异常或扫描件漏识别而遗漏。

## 12. 参考资料

按需查阅，不将参考资料中的示例或模式直接当作结论：

- `references/file-scanning-guide.md`：文件类型、来源边界和批量材料的分析参考；
- `references/regex-patterns.md`：需要确认候选线索类型时使用的模式目录；
- `references/case-studies.md`：典型暴露场景、关联判断和影响评估示例。


