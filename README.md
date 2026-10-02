# dbseal 🦭

> **Local-first Database Subsetting & Masking Toolkit**  
> *Sealed locally. Zero cloud uploads.*

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev)
[![Status: MVP](https://img.shields.io/badge/Release-v0.1.0-emerald.svg)](https://github.com/sealkit/dbseal/releases)

---

## ⚡ Why dbseal?

Testing with production data is risky, but dummy mock data breaks real business scenarios. Existing tools fall short:
- **Greenmask**: PostgreSQL-only dump masking; no subsetting; cannot handle MySQL.
- **Jailer**: Heavy Java UI; relies 100% on physical foreign keys (completely fails on modern ORMs like Prisma, GORM, TypeORM).
- **mysqldump / pg_dump**: Cold backup only; no relational subsetting; no schema drift tolerance.

**dbseal** solves Test Data Management (TDM) end-to-end:
1. **Lightweight Subsetting (`slice`)**: Extract relation-consistent sub-databases with cyclic foreign key resolution and auto-increment healing.
2. **Direct In-Memory Refresh (`sync`)**: Read replica → in-memory anonymization → target staging database without intermediate disk files.
3. **Virtual FK Sampling Validation**: Automatically infer logical foreign keys and validate them with **1000-row sampling JOIN checks** (confirmed ≥ 95%).
4. **Schema Drift Adaptation**: Automatically project common column intersections (`Auto-Skip`) so newer migrations in staging don't break data loads.
5. **Interactive Local Web UI (`ui`)**: Built-in visual canvas for inspecting table topology and configuring masking rules with live 3-row preview.
6. **Zero-Lock Production Safety**: 100% snapshot isolation (`REPEATABLE READ`), zero table locks (`FLUSH TABLES` strictly prohibited).

---

## 🚀 Quick Start

### 1. Initialize Configuration
```bash
# Generate a starter configuration (defaults to dbseal.yaml)
dbseal init

# Or create a custom configuration for staging
dbseal init staging.yaml
```

### 2. Pre-flight Validation & Connectivity Check
```bash
# Verify YAML syntax, relational integrity, and mode readiness
dbseal validate staging.yaml

# Test network connectivity and credentials with --ping
dbseal validate staging.yaml --mode sync --ping
```

### 3. Export Standalone Self-Healing SQL (`slice`)
```bash
# Extract relation-consistent subset with self-healing SQL
dbseal slice --config staging.yaml --output ./dev_seed.sql.gz

# Local developers don't need dbseal! Native mysql/psql imports out-of-the-box:
gzip -dc dev_seed.sql.gz | mysql -u root dev_db
```

### 4. Direct Staging Refresh (`sync`)
```bash
# Stream from production read-replica straight into staging test database (Zero Disk IO)
dbseal sync --config staging.yaml
```

### 5. Interactive 5-Section Web Canvas (`ui`)
```bash
# Launch interactive 5-section Web UI at http://127.0.0.1:8080
dbseal ui
```

---

## 📊 Command Reference

| Command | Usage Scenario | Description | Target DB Required? |
| :--- | :--- | :--- | :--- |
| `dbseal init [file]` | Quickstart & Scaffolding | Generates a clean, documented YAML template (e.g. `dbseal init staging.yaml`) | No |
| `dbseal validate [file]` | CI/CD & Pre-flight | Checks YAML syntax, rules, mode readiness (`--mode slice\|sync`), and tests connectivity (`--ping`) | Only with `--mode sync` |
| `dbseal slice` | Local Development & CI | Exports relation-consistent subset into self-healing `.sql.gz` with auto-increment seed repair | No |
| `dbseal sync` | Staging Refresh | Direct in-memory streaming from source to target DB with schema drift adaptation | **Yes** |
| `dbseal dump` | Cold Anonymization / Audit | Full database anonymized export to `.sql.gz` (entire database masked without subsetting) | No |
| `dbseal restore` | Offline Seed Injection | Injects offline `.sql.gz` slice into target test database with schema drift auto-skip | **Yes** |
| `dbseal ui` | Visual Canvas | Embedded 5-section web UI for connections, virtual FKs, subsetting, masking canvas, and export | Optional |

### 🛠 Command Flags & Cheat Sheet

```bash
# validate: Check configuration syntax, rules, and network connectivity
dbseal validate [config-file] [-c, --config <file>] [-m, --mode slice|sync|all] [-p, --ping]

# slice: Extract subset to compressed SQL file
dbseal slice [-c, --config <file>] [-o, --output <path>]

# sync: Zero-disk in-memory stream to remote staging database
dbseal sync [-c, --config <file>] [--tables <t1,t2>]

# dump: Full database anonymized export (100% masked, no subsetting)
dbseal dump [-c, --config <file>] [-o, --output <path>]
```

---

## ⚙️ 5-Section Configuration Architecture

Both `dbseal.yaml` and the Web UI follow a unified 5-section structure:

1. **Connections (`source`, `target`)**: Production read-replica (read-only) and optional test staging DB connection strings.
2. **Virtual Foreign Keys (`virtual_relations`)**: Non-physical logical relations discovered automatically or defined manually for modern ORMs.
3. **Subsetting & Root Targets (`subset.targets`)**: Root entrypoint queries (e.g. active users or recent orders) with BFS/DFS cascading closures.
4. **Masking Rules & Schema Canvas (`masking`)**: Sensitive PII field rules (e.g. `phone`, `email`, `id_card`, `fake_name`, `hash_sha256`) with live previews.
5. **Output & Export (`output`)**: Target archive filename, compression algorithm, and inline live validation.

---

## 🔒 Data Protection & Licensing FAQ

### 1. Does using dbseal infect my application or data with AGPL?
**No.** `dbseal` is an independent command-line toolkit. Using `dbseal` to extract, slice, or anonymize your database does **NOT** subject your application code or database data to AGPL v3. The data and SQL generated by `dbseal` belong 100% to you.

### 2. Commercial Edition (`dbseal Pro`)
For organizations with enterprise compliance policies that prohibit AGPL v3 tools, or those requiring:
- **Multi-table parallel streaming pipelines** (2x~5x speedup)
- **JSON/JSONB AST deep path anonymization**
- **Dynamic rotatable HMAC-SHA256 salting**
- **Automated Markdown audit and compliance reports**

Commercial licenses (Individual $99/yr / Lifetime $249 / Enterprise $999 buy-out) and documentation are available on our [Official Website](https://db.sealkit.org) or by contacting `license@sealkit.org`.

---

## ⚡ 1-Minute Playground Sandbox
Try dbseal immediately with our pre-configured Docker Compose sandbox (includes circular foreign keys, virtual relations, and sensitive PII):
```bash
cd demo
docker compose up -d
dbseal sync --config dbseal.demo.yaml
```
Check out [demo/README.md](demo/README.md) for full instructions!

---

## 🐟 给 Sammy 喂条鱼（打赏赞助 / Buy Coffee）
`dbseal` 与 `PDFSeal` 一样由独立开发者社区驱动。如果这个工具为你节省了时间、减少了烦恼，欢迎支持小海豹后续迭代：
- 👉 **[给小海豹 Sammy 喂条鱼（Ko-fi）☕ 🐟](https://ko-fi.com/muffin27)**

All sponsors contributing over $5 will be permanently featured in our **Backers** hall of fame!

---

## 🤝 Contributing
Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on submitting PRs and our Contributor License Agreement (CLA).

