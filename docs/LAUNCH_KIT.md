# 📢 dbseal Open-Source Launch Kit & Distribution Guide

Comprehensive promotion and launch materials for `dbseal` across global and regional developer communities.

---

## 1. Show HN (Hacker News)

**Target**: [news.ycombinator.com/submit](https://news.ycombinator.com/submit)  
**Title**: `Show HN: dbseal – Local-first database subsetting and masking in Go`

### Post Content:
```markdown
Hi HN! We built dbseal (https://github.com/sealkit/dbseal), a local-first CLI and Web UI toolkit written in Go to solve the nightmare of pulling realistic, sanitized staging/dev databases from production.

### The Problem
Whenever engineers want to debug an edge case locally or set up a realistic staging environment, they hit three walls:
1. Production databases are hundreds of gigabytes or terabytes (too big to restore entirely on a laptop or staging cluster).
2. Naive `mysqldump --where` breaks foreign key constraints, leaving dangling records or cascading failure on insert.
3. Plain exports leak PII (names, phone numbers, emails, payment details), violating GDPR, HIPAA, and corporate security policies.
Existing enterprise masking solutions are heavy cloud SaaS or complex Java enterprise apps requiring huge infrastructure overhead.

### What dbseal does
dbseal runs as a single, zero-dependency static binary (`brew install sealkit/tap/dbseal` or `curl ... | bash`):
1. **Graph Closure & Cycle Breaking**: Builds a DAG of your schema relations. If it detects circular foreign keys (e.g. `users.last_order_id` ⟷ `orders.user_id`), it uses Tarjan's strongly connected components algorithm to extract the data and generates a self-healing SQL footer to guarantee constraint satisfaction.
2. **Virtual Foreign Key Discovery**: Most modern apps (especially Rails, Django, Laravel, Prisma, Go GORM) don't declare physical foreign keys in DDL. dbseal uses inflection heuristics (singular/plural matching) and 1000-row sampling validation (`JOIN ... WHERE to.pk IS NOT NULL >= 95%`) to infer undeclared relations automatically.
3. **In-Flight Masking**: Applies rule-based, deterministic hashing (consistent across tables), and faker anonymization in-stream. No unmasked data is ever written to intermediate unencrypted files.
4. **Direct Zero-Disk Streaming**: `dbseal sync` reads from prod, masks in memory, and writes directly into your target database via batch inserts without touching disk.
5. **Interactive Web UI**: `dbseal ui` spins up an embedded React Flow canvas allowing teams to visually inspect table dependencies, preview masked rows in real time, and export YAML configurations.
6. **Scaffolding & Pre-flight CI Checks**: `dbseal init` generates documented starter YAML configs, while `dbseal validate` tests rules, mode readiness, and live DB connectivity (`--ping`) before running jobs.

### Tech Stack
- 100% Go (1.24+ baseline), single static binary (~12MB).
- Supported databases: MySQL 5.7/8.0/8.4+, PostgreSQL 12-17.
- AGPL v3 license with explicit data non-infectious waiver.

GitHub: https://github.com/sealkit/dbseal
Demo Sandbox: Includes a 1-minute `docker compose` playground to test circular FKs and virtual relationships.

We'd love your feedback on the schema graph traversal and virtual FK inference heuristics!
```

---

## 2. Reddit Posts

### A. Subreddit: `r/golang`
**Title**: `[Project] dbseal: A local-first DB subsetting & masking tool written in Go (zero dependencies, graph DAG, embedded Web UI)`

```markdown
Hey Gophers,

Over the past months, our team got tired of having to write one-off Python or Bash scripts whenever someone needed a representative chunk of our production MySQL/Postgres database for local reproduction.

We decided to build **dbseal** (https://github.com/sealkit/dbseal) as a high-performance Go CLI.

Key Go engineering highlights:
- **No CGO**: Pure Go database drivers (`go-sql-driver/mysql`, `pgx/v5`).
- **Graph & Algorithms**: Uses Tarjan's SCC algorithm for cycle detection, topological sorting for table dump sequence, and automated self-healing SQL generation for circular foreign keys.
- **Embedded Web UI**: Uses `//go:embed` to ship an interactive React Flow canvas with zero external node/npm runtime needed for end-users.
- **Streaming Pipeline**: Channel-buffered producer-consumer pipeline with batching to keep memory under 128MB even on gigabyte datasets.

Check out the repo on GitHub: https://github.com/sealkit/dbseal
Looking forward to code reviews and feedback from fellow Gophers!
```

### B. Subreddit: `r/devops` & `r/database`
**Title**: `Stop dumping raw production databases to dev/staging: dbseal (open-source DB subsetting & masking)`

```markdown
Hi all,

How do you usually handle database provisioning for staging/test environments?
- Full dumps take hours and cost a fortune in staging disk storage.
- Anonymizing scripts frequently break foreign key constraints or omit newly added columns.
- Modern ORMs rarely declare physical foreign key constraints, making relational slicing nearly impossible.

We open-sourced **dbseal** (https://github.com/sealkit/dbseal) to automate this:
- **Seed-based slicing**: Define `orders WHERE created_at > NOW() - INTERVAL 7 DAY LIMIT 1000` and dbseal automatically pulls all referenced users, products, addresses, and logs.
- **Auto-discovers virtual relations**: Detects `user_id` without physical DDL constraints using sampling JOIN validation.
- **Schema Drift Safe**: Automatically skips missing columns between prod and dev without crashing.
- **Runs locally or in CI/CD**: Single binary, fast execution, AGPL-3.0.

A 1-minute docker sandbox is available in the repo if you'd like to test it out!
```

---

## 3. X (Twitter) Launch Thread

**Tweet 1 (Hook)**:
> 📦 Announcing dbseal: The local-first Database Subsetting & PII Masking Toolkit in Go!
> 
> Ever tried restoring a 500GB prod database just to reproduce a bug on your laptop? Or broke foreign keys trying to manually dump 1000 rows?
> 
> dbseal fixes this. 🧵👇 (1/5)
> https://github.com/sealkit/dbseal

**Tweet 2 (Graph & Cycles)**:
> 🔄 Relational Integrity & Self-Healing:
> dbseal builds a complete DAG of your database schema.
> 
> Circular FKs? dbseal uses Tarjan's SCC to isolate cycles, extracts seed records, and injects self-healing update queries automatically. Zero foreign key violations. (2/5)

**Tweet 3 (Virtual FKs)**:
> 🕵️ Virtual Foreign Key Auto-Discovery:
> Using Rails, Django, Prisma, or Go GORM without physical DDL constraints?
> 
> dbseal scans column names and validates 1000-row JOIN matches in production. It infers undeclared relations with >=95% confidence! (3/5)

**Tweet 4 (Zero-Disk & UI)**:
> ⚡ Zero-Disk Streaming + Web UI:
> - `dbseal sync`: Stream masked rows in-memory directly from Prod to Dev.
> - `dbseal ui`: Built-in graph canvas to visually preview masking rules and dependencies.
> - Single static binary, no JVM, no heavy cloud agent. (4/5)

**Tweet 5 (Call to Action)**:
> Try dbseal today!
> ⭐ Star on GitHub: https://github.com/sealkit/dbseal
> 🚀 1-minute playground sandbox included with Docker Compose.
> 
> Feedback & PRs welcome! (5/5)

---

## 4. 国内技术社区发布文案 (V2EX / 掘金 / 知乎 / 微信公众号)

### A. V2EX (程序员 / 问与答 / 分享创造节点)
**标题**: `[开源自荐] dbseal: 用 Go 写的本地优先数据库切片与数据脱敏工具 (支持虚拟外键、循环外键自愈、内置画布)`

**正文内容**:
```markdown
各位 V 友大家好！

在日常研发中，大家一定遇到过这样的痛点：
1. 生产环境数据库几百 GB，拉到本地或测试环境又大又慢；
2. 自己写脚本按 `WHERE id IN (...)` 切割，经常漏表或者外键报错；
3. 很多项目用 GORM / MyBatis / Prisma，数据库物理表上压根没有 Foreign Key 约束（虚拟外键），手工理依赖极其痛苦；
4. 数据合规要求高，明文导出手机号、身份证、邮箱直接违规。

为了解决这个问题，我们开源了 **dbseal**（海豹数据）：
👉 项目地址：https://github.com/sealkit/dbseal

### 核心亮点：
1. **单静态二进制**：Go 语言编写，无任何 Java/Python 运行环境依赖，开箱即用。
2. **关系闭包与循环外键自愈**：利用 Tarjan 强连通分量算法识别环状依赖，自动生成自愈 SQL，彻底解决循环外键导入报错。
3. **虚拟外键推断（ORM 友好）**：基于词形变化启发式算法 + 采样 JOIN 校验率，自动发掘未声明物理约束的隐式关联。
4. **内存级流式同步**：`dbseal sync` 边拉取、边脱敏、边写入目标库，零中间未加密落盘文件。
5. **内嵌可视化画布**：`dbseal ui` 内置交互式画布，无需配前端环境，直接在浏览器里拖拽查看表依赖与 3 行实时脱敏效果预览。
6. **表结构漂移自适应**：测试库比生产库少字段？自动取交集投影，智能规避写入崩溃。

欢迎大家试用、拍砖和提 Issue！如果觉得对你有帮助，欢迎点个 Star 🌟！
```

### B. 掘金 / 知乎技术专栏文章大纲
**文章标题**: 《生产数据脱敏切片太痛苦？我们用 Go 写了一个开源工具 dbseal：拓扑排序、自愈外键与流式脱敏》

**核心大纲**:
1. **背景与痛点**：为什么企业从生产拉测试数据总是“违规”或“报错”？
2. **核心算法设计**：
   - 依赖图拓扑排序与 Tarjan 强连通分量解环
   - 循环外键两阶段解耦与自愈补丁生成（Max Auto-Increment Guard）
   - 虚拟外键（Virtual FK）启发式推断与概率性采样验证
3. **性能与安全**：
   - 为什么放弃 `ORDER BY RAND()`？主键分块与确定性 Hash 脱敏
   - 零磁盘泄露：内存流式投影与目标库漂移容错
4. **快速上手**：
   - 60秒体验：Docker Compose 沙箱与 Web UI 画布实操
5. **开源与路线图**：
   - AGPL v3 商业友好声明与 Pro 版能力演进
