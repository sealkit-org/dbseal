# dbseal 产品需求规格说明书 (PRD)

> **Local-first Database Subsetting & Masking Toolkit**
> Sealed locally. Zero cloud uploads. —— 专为开发测试环境打造的高性能安全切片与脱敏工具。
>
> 文档版本：**v1.5** | 更新日期：2026-10-01 | 状态：已定稿 (Ready for Development)

---

## 0. 版本变更记录

| # | 变更 | 类型 | 理由 |
| --- | --- | --- | --- |
| 1 | 产品代号 SnapSlice → **dbseal**，域名 `db.sealkit.org` | 品牌 | 统一 SealKit 系列资产（pdfseal → dbseal），延续 "Seal" 双关与吉祥物叙事 |
| 2 | 许可证确立为 **AGPL v3 + 商业双许可 (Dual-Licensing)** | 决策 | 社区版 AGPL v3 防竞品套壳并形成商业转化杠杆；商业版提供免除 AGPL 限制授权与技术支持 |
| 3 | **循环依赖解环从 Pro 移入 OSS**；Pro 改为解环策略增强与 SCC 分析报告 | 战略 | 真实生产库外键环极常见，"遇环报错"会让开源用户在第一个库上就失败，摧毁口碑 |
| 4 | 逻辑外键自动推导新增 **值采样验证（Sampling Validation）** | 差异化 | 纯命名规则误报率高；用 JOIN 匹配率背书自动连线，建立信任 |
| 5 | 新增 **切片正确性保障**（自增种子重置、单表行数上限、约束/触发器管理、导入完整性对照） | 正确性 | 决定产品是"玩具"还是"工具"的深水区 |
| 6 | **关联列保护与基础确定性哈希**：OSS 提供固定 Salt 确定性脱敏，解除关联列无法脱敏死穴 | 正确性 | 避免关联列完全无法脱敏导致业务库报错，同时防止随机化破坏外键一致性 |
| 7 | **竞品对照章节**：明确与 Greenmask / Jailer 的差异化定位 | 营销 | 用户第一问即"为何不用 X"，直接驱动 README 与首发文案 |

### v1.4 → v1.5 (开源闭环与商业双许可架构升级)

| # | 变更 | 类型 | 理由 |
| --- | --- | --- | --- |
| 35 | **基础 `dbseal sync` 与基础 Schema 列交集下放 OSS** | 战略 | 彻底杜绝 Bait-and-Switch 差评。开源版可独立跑通单流内存直刷新，商业版专攻多表并发加速与智能 Mock 回退 |
| 36 | **基础加盐确定性哈希下放 OSS** | 可用性 | 真实生产库关联列（email/phone）是脱敏刚需，放开基础确定性脱敏，Pro 专供动态 Salt 轮换与 JSONPath 深度脱敏 |
| 37 | **修正测试库清空策略**（MySQL 外键表禁用 `TRUNCATE` 改用 `DELETE`） | 正确性 | 解决 MySQL InnoDB 下即使 `FOREIGN_KEY_CHECKS=0` 遇到外键引用仍禁止 `TRUNCATE` 的底层报错 |
| 38 | **确立开源与商业双仓库分层架构** (`dbseal` + `dbseal-pro`) | 架构 | 商业仓库通过 Go Module 引入开源核心，通过依赖注入与构建扩展点补充 Pro 能力，杜绝代码泄露 |

---

## 1. 文档基本信息与定位

| **项目** | **说明** |
| --- | --- |
| **产品代号** | **dbseal**（`db.sealkit.org`） |
| **产品形态** | 单一跨平台可执行二进制（CLI + 内嵌 React Flow Web UI），100% 离线运行（Local-First） |
| **品牌归属** | SealKit 系列，与 pdfseal 共享海豹吉祥物 Sammy 与 "Seal" 双关叙事（安全封存/海豹） |
| **目标受众** | 欧美独立开发者、中小型 SaaS 团队、Tech Lead、DevOps、后端架构师 |
| **核心卖点** | ① **分钟级轻量切片**：从数十/数百 GB 生产库中按业务链路提取结构完整的轻量开发库，外键环自动解环、自增种子自愈修复；<br>② **虚拟外键值采样背书**：无需物理外键，自动按命名规则推导并经 1000 行采样 JOIN 验证；<br>③ **双轨交付形态**：既支持网络直通场景的**零落盘内存同步 (`sync`)**，又支持隔离场景的**自愈式离线 SQL (`slice`)**；<br>④ **既有测试库抗漂移适配**：测试环境多了新字段或少了废弃字段时，自动按公共列交集注入，不报语法错误，不冲掉测试环境最新 DDL；<br>⑤ **生产从库零锁表保证**：全流程快照读，杜绝任何全局锁与长时间行锁。 |
| **支持数据源** | **PostgreSQL 12+ · MySQL 5.7+**。官方测试矩阵：PG 12–17，MySQL 5.7 / 8.0+。MariaDB / TiDB / Aurora 预期兼容，Pro 提供官方验证响应。 |
| **软件发布版本** | **v0.1.0** (遵从 SemVer 语义化版本，初始 MVP 研发阶段从 `v0.1.0` 起步) |
| **许可证** | **双许可体系 (Dual-Licensing)**：开源社区版 **AGPL v3**（配 CLA 机制与数据免传染声明）；商业版 Commercial License（免除 AGPL 限制与法务顾虑，个人版 $49 / 团队版 $199，Ed25519 离线校验） |

---

## 2. 总体架构与数据流设计

系统设计为解耦的四层结构：交互入口层、图拓扑与脱敏核心算法层、方言驱动层，以及底层双轨数据流管理管道。

```
                    ┌─────────────────────────────────────────┐
                    │               交互入口层                │
                    │   - 命令行接口 (CLI: sync / slice / ...)│
                    │   - 内嵌交互式本地 Web UI (React Flow)   │
                    └────────────────────┬────────────────────┘
                                         │
                    ┌────────────────────▼────────────────────┐
                    │         核心算法与规则引擎               │
                    │   - 关系拓扑图引擎 (DAG & SCC 环检测)     │
                    │   - 启发式逻辑外键推导 + 值采样验证器     │
                    │   - 测试环境 Schema 漂移感知器 (公共列交集)│
                    │   - 脱敏引擎 (Faker, 常量, 确定性全局哈希)│
                    │   - 自愈正确性守护 (种子重置/解环补齐)    │
                    └────────────────────┬────────────────────┘
                                         │
                    ┌────────────────────▼────────────────────┐
                    │          统一数据库方言抽象层           │
                    │   - MySQL (5.7 / 8.0) 方言驱动          │
                    │   - PostgreSQL (12–17) 方言驱动         │
                    └────────────────────┬────────────────────┘
                                         │
         ┌───────────────────────────────┴───────────────────────────────┐
         ▼ (网络互通场景: 零磁盘直接刷新)                 ▼ (网络隔离场景: 离线自愈文件)
┌─────────────────────────────────┐             ┌─────────────────────────────────┐
│        一键内存同步 (Sync)      │             │        离线切片文件 (Slice)     │
│ - 内存管道直达测试库 (Target DB)│             │ - 导出为独立 .sql.gz 文件       │
│ - Wipe & Insert 目标库清空刷新  │             │ - 尾部自带自增与解环自愈代码    │
│ - 适配测试库字段漂移 (Auto-Skip)│             │ - 原生 mysql/psql/Docker 即导即用│
└─────────────────────────────────┘             └─────────────────────────────────┘
```

---

## 3. 详细功能模块规格

### 产品流程与命令面 (Command Surface)

dbseal 提供 7 个直观的核心命令，覆盖从配置生成、预检验证、切片导出、内存直灌到可视化配置的全生命周期：

| 命令 | 适用场景 | 行为定义 | 是否依赖 Target |
| :--- | :--- | :--- | :--- |
| `dbseal init [file]` | **新手起手与配置脚手架** | **生成配置模板**：产出带详尽注释与最佳实践的 `dbseal.yaml`（支持 `-f` 覆盖、指定文件名如 `staging.yaml`）。 | 否 |
| `dbseal validate [file]` | **CI/CD 门禁与运行前预检** | **静态规则检查与连通性嗅探**：校验 YAML 语法、模式就绪度（`--mode slice\|sync`）、规则合法性，支持 `--ping` 测试库连通性。 | 仅在 `--mode sync` 时依赖 |
| `dbseal sync` | **网络打通的中小企业日常** | **直连内存刷新**：从源端只读拉取 -> 内存流式脱敏 -> 直写目标测试库。适配测试库字段漂移，零中间磁盘消耗，并发管道提速 2x。 | **是** |
| `dbseal slice` | **网络隔离 / 分发给全员本地开发** | **切片导出文件**：产出带自愈代码的独立 `.sql.gz`，丢给开发人员本地启动或 CI 测试。 | 否 |
| `dbseal restore` | **离线文件的恢复** | 将 `slice` 生成的离线文件注入目标测试库（支持测试库 Schema 漂移适配）。 | **是** |
| `dbseal dump` | **全库脱敏冷备 / 外部合规审计** | **整库流式脱敏导出**：不做子集切片，全库所有表与数据行 100% 规则脱敏后流式导出为 `.sql.gz`。 | 否 |
| `dbseal ui` | **可视化连线与规则配置** | **交互式 5 分区画布**：启动本地 Web UI（默认绑定 `127.0.0.1:8080`），点选表关系拓扑、脱敏即时预览并一键校验/导出 YAML。 | 可选 |

---

### 模块一：测试环境 Schema 漂移感知与容错适配 (Schema Drift Adaptation)

专为**既有测试/预发环境刷新（Staging Refresh）**服务。解决“测试库刚跑了新 Migration，字段与生产不一致导致灌数据报错”的顽疾。

- **公共列交集（Column Intersection）投影 [OSS 核心能力]：**
  - 在写入目标测试库前，快速读取目标端对应表的列定义；
  - 自动计算源端与目标端列的**交集（Common Columns）**；
  - 自动生成带显式列名的写入语句：`INSERT INTO users (id, name, email) VALUES (...)`；
  - 自动剔除生产端独有（测试端已删除）的废弃列。

- **测试端新增字段的优雅降级策略：**
  - **默认值优先 [OSS & Pro]**：若测试库新列具有 `DEFAULT` 或允许 `NULL`，由数据库底层自动填充；
  - **非空无默认列（NOT NULL without DEFAULT）降级**：
    - **OSS 行为**：Pre-flight 检查阶段发现目标端存在非空无默认列时，安全阻断并输出明确字段差异指引，避免注入失败；
    - **Pro 智能回退**：自动填充语义零值兜底（数字 `0`，字符串 `""`，布尔 `false`，当前时间戳）；支持在 YAML 的 `drift_fallbacks` 中显式指定 Mock 填充值；复杂不可推导类型（如无默认值 ENUM）输出诊断报告。

- **目标端执行模式：清空重写（Wipe & Insert）：**
  - **严禁增量合并（No Upsert）**：外键拓扑树下做 Upsert 极易破坏级联一致性，刷新模式明确定义为“重置测试库数据”；
  - **保留 DDL**：严禁执行 `DROP TABLE`（防止冲掉测试环境正在测试的表结构变更）；
  - **清空策略**：在目标端关闭外键检查后，按表批量执行 `DELETE FROM <table>`（**严禁使用 `TRUNCATE`**，彻底规避 MySQL InnoDB 即使在 `FOREIGN_KEY_CHECKS=0` 下遇到外键引用仍强制报错 `Cannot truncate a table referenced in a foreign key constraint` 的底层陷阱），随后写入切片数据。

---

### 模块二：关系依赖图与子集提取引擎 (Graph & Subsetting Engine)

构建数据拓扑关系网，按业务切片规则向下递归关联数据，向上补全字典配置。

- **物理外键扫描：** 自动读取系统表中的全部 `FOREIGN KEY` 约束，初始化为有向图。

- **逻辑/虚拟外键（Virtual FK）推导（核心护城河）：**
  - **启发式命名匹配**：自动识别 `orders.user_id` → `users.id`，`audit_logs.operator_uuid` → `accounts.uuid` 等常见模式；
  - **值采样验证（Sampling Validation，OSS 核心）**：杜绝盲目自动连错表。
    | 验证结果 | 判定 | 处理 |
    | --- | --- | --- |
    | JOIN 匹配率 ≥ 阈值（默认 95%，采样 1000 行可配） | `confirmed` | 作为图边参与遍历，UI 渲染为实线 |
    | 匹配率 < 阈值 | `candidate` | **不参与遍历**，UI 渲染黄色虚线，提示人工确认 |
    | 空表 / 无法采样 | `unverified` | 同 candidate，标注原因 |
  - **多态与条件关联 (Pro)**：支持附加过滤条件（如 `commentable_type = 'Post'`）与复合键关联。

- **采样入口与孤岛字典表：**
  - **Root Targets**：支持多根节点 + 自定义 SQL 过滤（如活跃用户、近期租户）；
  - **`reference_tables` 孤岛字典表全量保留（v1.4 新增）**：无论图连通性如何，显式配置的配置表/角色表/枚举表 100% 完整抽取。

- **双向级联遍历与边界防御：**
  - **向下递归 (1:N)**：抓取子订单、明细、操作日志，支持 `max_depth`；
  - **向上补全 (N:1)**：自动补齐分类、国家等上游引用；
  - **单表行数上限 (`row_cap`)**：字典链路单表默认上限 10,000 行（防误抓百万大表），超限默认截断并输出告警，可配 `row_cap_exceeded: fail` 升级为硬失败。

- **循环依赖解环（Cycle Resolution）：**
  - Tarjan SCC 检测自循环与强连通分量；
  - 默认策略 `null_then_update`：环上依赖列先置 `NULL` 导入，最后追加 `UPDATE` 补齐；
  - **环上 NOT NULL 处理**：MySQL 在 `FOREIGN_KEY_CHECKS=0` 下乱序直插优化；PG 优先 `DEFERRABLE`，必要时临时调整约束并在尾部恢复。

---

### 模块三：数据脱敏与仿真引擎 (Anonymization Engine)

在内存流传输过程中实时修改敏感数据，杜绝数据落盘与外泄。

- **基础脱敏算子 (OSS)：**
  - 姓名、电子邮件、手机号、地址、IP 的 Faker 伪造；
  - 字符串掩码（`138****0000`）、置空（`NULL`）、常量替换（重置开发密码）。
- **关联列脱敏保护机制：**
  - 随机化脱敏（如随机 Faker 姓名）**严禁用于关联列**（主键、外键、参与图遍历的逻辑外键），防止静默破坏数据完整性；
  - **基础加盐确定性全局哈希 [OSS]**：若关联列（如以 email 或 phone 作为逻辑外键关联）需脱敏，OSS 提供全局固定 Salt 的确定性哈希，确保全库映射一致，关联不损坏。
- **高级确定性哈希与半结构化脱敏 (Pro)：**
  - **企业级动态加盐 HMAC-SHA256 (Pro)**：支持按租户/业务域密钥轮换、跨 Schema 一致性映射；
  - **半结构化深层数据脱敏 (Pro)**：JSONPath 定位内部字段（`$.billing.card_no`），仅脱敏特定键值，保留外层 JSON 结构与字段类型。

---

### 模块四：高速数据流管线与生产安全保证

兼顾极致导出吞吐量与生产库零影响。

- **生产只读零侵入保证 (Zero-Lock & Read Safe)：**
  - **快照读无锁事务**：MySQL 显式使用 `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`，PG 使用 `BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;`，**严格杜绝 `FLUSH TABLES`**；
  - **并发与流控保护 (Throttle)**：支持配置读取并发度（默认 2）与批次间微休眠（如 `--sleep-ms=10`），杜绝打满生产从库 IOPS 与 CPU。

- **PostgreSQL 实现：**
  - 默认采用 `COPY (SELECT ...) TO STDOUT` 协议流式拉取；
  - 容错写入场景自动切换为精准列名的 INSERT 批次流；
  - 导出时识别并排除 `GENERATED ALWAYS AS` 生成列。

- **MySQL 实现：**
  - Key-seek 分页迭代拉取，禁用递归 CTE（兼容 5.7+）；
  - 生成列（VIRTUAL/STORED）在写入清单中自动剔除；
  - **会话变量权限探测降级**：云 RDS / 受限账号执行 `SET SQL_LOG_BIN=0` 失败时自动降级剥离，输出告警而非中断；
  - 多行批量拼装：单批次 2000 行或 4MB，显式批量 `COMMIT`。

- **双轨交付管道：**
  - **Direct Sync 管道**：Go Channels 内存缓冲池，边拉取边脱敏边写入目标库，零文件落盘。**OSS 提供单表流式同步**；**Pro 版提供多表并行流式管道（速度翻倍）**与断流自愈；
  - **File 管道 [OSS & Pro]**：内存边脱敏边流入 `gzip.Writer`，输出为 `.sql.gz` 自愈文件。

---

### 模块五：交互式可视化画布 (Local Web UI)

通过本地 Web 画布降低配置门槛。`dbseal ui` 默认仅监听 `127.0.0.1:8080`，自动打开浏览器。

- **ER 关系画布 (React Flow)：**
  - 节点卡片：表名、行数预估、字段清单；
  - 连线渲染：物理外键 = 灰色实线；采样验证通过的逻辑外键 = 黄色实线（标注匹配率）；未验证/候选 = 虚线；
  - 画布直接连线配置逻辑外键，弹出关联类型确认。
- **脱敏规则侧滑抽屉：**
  - 字段推荐脱敏规则下拉选框；
  - **真实样本预览**：点击“数据预览”实时展示当前规则处理 3 行真实样本的效果。
- **一键操作**：
  - 画布状态一键导出标准 YAML；
  - 提供“一键开始切片/同步”控制台，实时滚动展示吞吐速率与解环进度。

---

### 模块六：切片与刷新正确性保障 (Correctness Guard)

确保无论哪种交付形态，产物都是“开箱即用”的完整可用库。

- **自愈式独立 SQL（针对 `slice` 文件模式，v1.4 升级）：**
  - 导出时计算各表最大自增 ID；
  - 在 `.sql.gz` **文件末尾自动追加自愈 SQL 代码**：
    ```sql
    -- dbseal Correctness Guard: 环依赖补齐与自增种子修复
    UPDATE departments SET manager_id = 12 WHERE id = 1;
    ALTER TABLE users AUTO_INCREMENT = 10086;
    ```
  - **收益**：开发者在本地机器上**无需安装 dbseal**，直接执行 `mysql -u root dev_db < slice.sql` 或挂载给 Docker 容器，就能一次性完美启动！
- **自增种子重置（针对 `sync` 直通模式）：**
  - 数据注入完成后，在目标测试库自动执行 `ALTER TABLE ... AUTO_INCREMENT = max(id)+1` 与 PG `setval(...)`。
- **触发器与约束管理：**
  - 写入期临时禁用外键与触发器，防止触发业务副作用，注入完毕后恢复。
- **导入完整性对照：**
  - 终端打印“计划切片行数 vs 实际写入行数”对照清单；Pro 版输出 Markdown 审计报告。

---

## 4. 版本边界与商业化切分方案

| **功能维度** | **开源社区版 (AGPL v3)** | **商业版 (Pro / Commercial License)** |
| :--- | :--- | :--- |
| **数据库支持** | PostgreSQL 12+ / MySQL 5.7+ | + AWS Aurora / TiDB / MariaDB 官方验证与支持 |
| **交付形态** | `slice` 导出自愈式文件；`dump` 全量脱敏；`ui` 画布配置；<br>**`dbseal sync` 基础单流直连内存刷新**（零磁盘消耗） | + `dbseal sync` **多表并发流式管道**（零磁盘极速通道，速度提升 2x~5x） |
| **测试库漂移适配** | **公共列交集（Auto-Skip）自动跳过废弃列**；<br>测试端新增 nullable/default 字段自动填充；CLI 差异告警 | + **非空无默认列（NOT NULL without DEFAULT）智能语义回退**<br>+ YAML 显式 Mock 规则覆盖 + 字段漂移深度诊断 |
| **外键与关系网** | 物理外键；无限制手动声明；**命名推导 + 1000 行采样验证**；<br>`reference_tables` 孤岛配置表保留 | + 多态/条件关联；复合键；跨 schema 推断；UI 一键全库自动连线 |
| **脱敏深度** | Faker 生成；整列置空/掩码/常量；<br>**基础固定 Salt 确定性全局哈希**（保障关联列脱敏不破坏连通性） | + **企业级动态加盐 HMAC-SHA256 轮换**；<br>+ **JSON/JSONB 内部路径 AST 深度脱敏**；仿真算法 |
| **性能加速** | 单表流式导出/写入；基础多行 INSERT | **多表并行流式管道**；MySQL 离线 DDL 索引后置构建 |
| **审计与交付** | 终端打印完整性对照 | **完整 Markdown 审计报告**（Schema 漂移 / SCC / 完整性对照） |
| **定价与授权** | 免费开源 (AGPL v3，配 CLA 机制与数据免传染声明) | **免除 AGPL 限制的商业授权**（个人版 $49 / 团队版 $199 买断制，包含 1 年更新与 CI 授权） |

---

## 5. 配置规范定义 (YAML Specification v1.4)

```yaml
version: "1.4"

# 数据库连接
source:
  driver: "mysql"           # mysql | postgres
  dsn: "${SOURCE_DB_URL}"   # 生产只读从库连接 (推荐 Read-Replica)
  options:
    ssl_mode: "preferred"
    max_open_conns: 5
    throttle_sleep_ms: 10   # 生产读取流控，每次拉取微休眠，防打满 IOPS

target:
  driver: "mysql"
  dsn: "${TARGET_DB_URL}"   # 测试环境数据库连接 (sync / restore 必需)

# 1. 虚拟/逻辑外键规则
relations:
  auto_detect:
    enabled: true
    patterns:
      - "{table_singular}_id -> {table}.id"
    sampling:
      sample_rows: 1000
      match_threshold: 0.95   # >= 95% 自动确认为实线；低于则降级为待确认虚线
  virtual:
    - from: "orders.created_by"
      to: "users.id"
      type: "many-to-one"
    - from: "comments.commentable_id"
      to: "posts.id"
      type: "many-to-one"
      condition: "comments.commentable_type = 'Post'"  # Pro: 多态关联

# 2. 子集切片定义
subset:
  targets:
    - table: "users"
      query: "SELECT id FROM users WHERE status = 'active' ORDER BY updated_at DESC LIMIT 500"
  
  # v1.4 新增：无论图遍历是否连通，100% 完整抽取的孤岛字典/配置表
  reference_tables:
    - "system_settings"
    - "roles"
    - "permissions"
    - "currencies"

  strategy:
    follow_downstream: true
    complete_dictionaries: true
    max_depth: 4
    row_cap: 10000            # 单表上限，超出截断并告警
    row_cap_exceeded: warn    # warn 截断告警 | fail 硬失败
    exclude_tables:
      - "audit_logs_archive_*"

# 3. 脱敏规则（关联列禁止随机化脱敏）
masking:
  tables:
    users:
      email: "faker.email"
      phone: "faker.phone_number"
      password_hash: "constant.$2a$12$dummyhashforlocaldev..."
      extra_metadata:
        "$.billing.ssn": "faker.ssn"                   # Pro: JSON 内部字段脱敏
    orders:
      credit_card: "faker.credit_card_number"

# 4. 测试环境漂移容错设置 (v1.4 新增)
target_drift:
  mode: "wipe_and_insert"     # 清空重写测试库数据，保留现有 DDL
  auto_skip_missing_columns: true  # 自动投影公共列交集 (Pro)
  fallbacks:                  # 测试库新增非空无默认值列的模拟填充值
    users:
      referral_code: "MOCK_SEED"

# 5. 正确性保障
correctness:
  resolve_cycles: "null_then_update"
  reset_auto_increment: true
  disable_triggers: true
  integrity_check: true

# 6. 文件输出设置 (针对 slice / dump)
output:
  file: "./dev_seed_data.sql.gz"
  format: "sql.gz"
  self_healing: true          # 将自增重置与解环 UPDATE 写入文件尾部
```

---

## 6. 工程技术选型与目录结构

- **核心语言**：Go (1.24+) —— 单二进制、低资源占用、原生 Channel/Goroutine 适合流式网络管道。
- **前端技术栈**：React 18 + Vite + Tailwind CSS + React Flow，经 `//go:embed` 打包嵌入二进制。
- **数据库驱动**：PostgreSQL [jackc/pgx/v5](https://github.com/jackc/pgx/v5)；MySQL [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)。
- **许可证与代码管理**：
  - **双仓库隔离架构**：开源仓库 `dbseal` 采用 **AGPL v3** 开源在 GitHub；商业版存放于独立的私有仓库 `dbseal-pro`。
  - **代码集成方式（依赖注入与扩展点模式）**：商业版仓库通过 Go Modules 直接 `require github.com/sealkit/dbseal`，利用接口反转（IoC）在 `main.go` 中注册并发管道与 Pro 脱敏器，无需侵入开源仓库源码，100% 避免商业源码与密钥意外泄漏。

### 推荐工程目录结构与仓库分层

#### 1. 开源主仓库 (`github.com/sealkit/dbseal`, Public / AGPL v3)
```
dbseal/
├── cmd/
│   └── dbseal/
│       └── main.go               # 开源版统一 CLI 入口 (sync / slice / dump / restore / ui)
├── pkg/
│   └── extension/                # 暴露给 Pro 版或第三方插件的扩展点与接口定义
├── internal/
│   ├── config/                   # YAML 解析与环境变量替换
│   ├── graph/                    # 拓扑排序、DAG/SCC、图遍历、启发式推导
│   ├── sampling/                 # 逻辑外键值采样验证器 (JOIN 命中率统计)
│   ├── drift/                    # 测试环境 Schema 漂移感知与公共列交集计算
│   ├── correctness/              # 种子计算、解环补齐、约束管理、自愈 SQL 注入
│   ├── masking/                  # 脱敏引擎 (Faker, 常量, 掩码, 基础全局加盐哈希)
│   ├── drivers/                  # 方言抽象与实现
│   │   ├── driver.go             # 统一 Driver 接口
│   │   ├── postgres/             # PG COPY 流拦截与 DDL 处理
│   │   └── mysql/                # MySQL Key-seek 批式拉取与参数探测降级
│   ├── pipeline/                 # 双轨流式管道 (基础单流 Direct Sync / File 压缩写入)
│   └── ui/                       # 内嵌本地 Web 服务与 API
│       ├── server.go
│       └── dist/                 # 前端打包产物 (go:embed)
├── web/                          # 前端源码 (React Flow 画布)
├── docs/
│   └── PRD.md                    # 本文档
├── go.mod
└── Makefile
```

#### 2. 商业私有仓库 (`github.com/sealkit/dbseal-pro`, Private / Commercial)
```
dbseal-pro/
├── cmd/
│   └── dbseal-pro/
│       ├── main.go               # 商业版入口：import dbseal 核心并注入 Pro 插件
│       └── license.go            # Ed25519 离线 License 激活与校验中间件
├── pkg/
│   ├── sync/                     # 多表并发流式同步管道 (实现 extension.Pipeline 接口)
│   ├── hash/                     # 动态加盐 HMAC-SHA256 密钥轮换哈希
│   ├── jsonpath/                 # JSON/JSONB AST 深度路径脱敏
│   └── audit/                    # 自动生成 Markdown 审计与合规报告
├── go.mod                        # require github.com/sealkit/dbseal v1.x (支持本地 replace)
└── Makefile                      # 编译商业二进制：dbseal (含 Pro 完整能力)
```

---

## 7. 敏捷研发迭代计划 (v1.4 修订：6 周交付)

聚焦核心切片与脱敏，研发周期由 8 周收敛至 **6 周**，第 3 周末完成核心 MVP 验收。

```
[第 1 周] 驱动层与全量脱敏管道 (Foundation & Dump)
   ├── 搭建 Go 目录骨架，统一 Driver 接口，集成 MySQL 驱动
   ├── 实现单表流式 SELECT、Faker 脱敏管道、多行 INSERT 拼装
   └── 跑通 dbseal dump，生成脱敏后的 .sql.gz

[第 2 周] 关系拓扑图引擎与字典保留 (Graph Engine)
   ├── 物理外键读取、内存 DAG 结构、visited 去重
   ├── 实现向下递归 (1:N) 与向上补全 (N:1)，支持 max_depth 与 row_cap 截断
   └── 支持 subset.reference_tables 孤岛字典表 100% 完整保留

[第 3 周] 离线切片闭环与自愈正确性 (Slice & Self-Healing)
   ├── 自增种子最大值计算，SCC 解环语句追加
   ├── 实现自愈式 SQL 生成：文件末尾追加 ALTER AUTO_INCREMENT 与解环 UPDATE
   └── ✅ MVP 验收：生产真实库执行 dbseal slice，原生 mysql 导入空库开箱即用

[第 4 周] 逻辑外键与值采样验证 (Virtual FK & Sampling)
   ├── 启发式命名模式匹配 (table_id -> table.id)
   ├── 实现 sampling/ 采样验证器（1000 行采样计算 JOIN 匹配率）
   └── 状态分级（confirmed / candidate），杜绝错误推导

[第 5 周] 直连同步管道与测试库抗漂移 (Sync & Drift Adaptation)
   ├── 实现 dbseal sync 基础单流直通管道（Channel 缓冲，零磁盘消耗）
   ├── 实现 drift/ 模块：读取目标测试库列结构，计算公共列交集 (Column Intersection) 投影
   └── 适配目标端 Wipe & Insert 安全清空重写（DELETE 全表防外键阻断）

[第 6 周] 可视化画布与产品交付 (Web UI & Release)
   ├── React Flow 表卡片拓扑与连线交互（实线/虚线区分）
   ├── 脱敏规则抽屉与 3 行样本实时预览，go:embed 打包
   ├── PG 方言驱动基础支持 (jackc/pgx)
   └── 准备 README、Demo 动图与首发上线材料
```

---

## 8. 竞品与差异化定位

| 竞品 | 定位 | 与 dbseal 的差异与优势 |
| --- | --- | --- |
| **Greenmask** | PG-only dump 脱敏工具（Apache 2.0） | 无子集/切片能力；不支持 MySQL；无逻辑外键推导；无 Web UI；依赖本地安装 pg_dump |
| **Jailer** | 数据库子集工具（Java/Swing，老牌） | 极其依赖物理外键（遇到现代 ORM 逻辑外键抓瞎）；无现代 Web UI；配置繁琐；无字段漂移适配 |
| **Tonic.ai** | 企业级测试数据平台（商业闭源） | 极度重型、价格昂贵（数万美元/年）；云端依赖，部署复杂；非 Local-first |
| **mysqldump / pg_dump** | 官方冷备工具 | 只能全量或单表导出；无关系拓扑切片；无内置脱敏；无法解决测试环境字段漂移 |

**差异化价值主张**：
*“Greenmask 只能脱敏不能切片，Jailer 只能切片不能脱敏且对无外键数据库无能为力。dbseal 两者兼备，不仅用采样验证攻克了无物理外键难题，更原生支持直通测试库抗漂移刷新，单二进制离线运行。”*

---

*dbseal PRD v1.4 — 修订完毕。明确测试数据管理定位，确立直通同步与自愈文件双轨架构，研发周期收敛至 6 周。*
