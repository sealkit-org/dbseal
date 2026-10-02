# 🚀 dbseal 1-Minute Playground Demo

Experience database subsetting, relational integrity preservation, and in-flight PII masking in under 60 seconds!

---

## 📋 What's in this demo?

This sandbox tests the most challenging real-world database challenges:
1. **Circular Foreign Keys**: `users.last_order_id` ⟷ `orders.user_id` (auto-detected and solved with self-healing SQL).
2. **Virtual Foreign Keys**: `audit_logs.operator_id` ➔ `users.id` (no physical foreign key constraint in DDL).
3. **Reference Tables**: `products` (synchronized 100% in full without downsampling).
4. **Cascading Relational Closure**: Selecting completed `orders` automatically extracts parent `users` and downstream `order_items`.
5. **PII Masking**: Names, phones, emails, ID cards, and IP addresses are masked dynamically with zero temporary unencrypted storage.

---

## ⚡ Quickstart in 3 Steps

### Step 1: Start Demo Databases
Launch the pre-seeded `source-db` (`shop_prod` on port 33061) and blank `staging-db` (`shop_dev` on port 33062):
```bash
docker compose up -d

# Verify configuration and database connectivity
dbseal validate dbseal.demo.yaml --ping
```

### Step 2: Launch Visual Graph Canvas
Inspect the 5-section configuration, relational schema, auto-discovered virtual foreign keys, and live masking preview in your browser:
```bash
dbseal ui --config dbseal.demo.yaml --port 8080
```
Open [http://localhost:8080](http://localhost:8080) to interact with the visual canvas.

### Step 3: Direct Streaming Sync
Execute zero-disk in-memory subsetting & masking directly into the staging database:
```bash
dbseal sync --config dbseal.demo.yaml
```

*Or generate an encrypted self-healing `.sql.gz` dump instead:*
```bash
dbseal slice --config dbseal.demo.yaml -o demo_dump.sql.gz
```

---

## 🔍 Verification

Connect to `staging-db` (`127.0.0.1:33062`, user `root`, pass `root`, db `shop_dev`):

```sql
-- 1. Check masked users (Notice fake names, masked ID cards, but original IDs preserved)
SELECT id, name, email, phone, id_card FROM users;

-- 2. Check relational integrity between users and orders
SELECT o.id, o.order_no, o.status, u.name as masked_user_name
FROM orders o 
JOIN users u ON o.user_id = u.id;

-- 3. Check reference table (all 5 products fully synchronized)
SELECT count(*) FROM products;

-- 4. Check virtual foreign key downstream logs
SELECT a.id, a.operator_id, a.action, a.client_ip, u.name 
FROM audit_logs a 
JOIN users u ON a.operator_id = u.id;
```

---

## 🧹 Cleanup
```bash
docker compose down -v
```
