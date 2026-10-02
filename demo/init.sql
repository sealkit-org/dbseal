-- dbseal 1-Minute Playground Demo Schema & Seed Data
-- Database: shop_prod

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- 1. Products (Reference Table: 100% full sync intended)
DROP TABLE IF EXISTS `products`;
CREATE TABLE `products` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `name` VARCHAR(100) NOT NULL,
    `category` VARCHAR(50) NOT NULL,
    `price` DECIMAL(10, 2) NOT NULL,
    `stock` INT NOT NULL DEFAULT 100,
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2. Users (Contains PII and Circular FK to orders.id)
DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `name` VARCHAR(100) NOT NULL,
    `email` VARCHAR(150) NOT NULL,
    `phone` VARCHAR(30) NOT NULL,
    `id_card` VARCHAR(30) NOT NULL,
    `address` VARCHAR(255) NOT NULL,
    `metadata` JSON DEFAULT NULL,
    `last_order_id` INT DEFAULT NULL,
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_users_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3. Orders (Depends on users, circular link to users.last_order_id)
DROP TABLE IF EXISTS `orders`;
CREATE TABLE `orders` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `order_no` VARCHAR(64) NOT NULL UNIQUE,
    `amount` DECIMAL(10, 2) NOT NULL,
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT `fk_orders_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Add circular foreign key: users.last_order_id -> orders.id
ALTER TABLE `users` ADD CONSTRAINT `fk_users_last_order` FOREIGN KEY (`last_order_id`) REFERENCES `orders` (`id`) ON DELETE SET NULL;

-- 4. Order Items (1:N downstream cascade from orders)
DROP TABLE IF EXISTS `order_items`;
CREATE TABLE `order_items` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `order_id` INT NOT NULL,
    `product_id` INT NOT NULL,
    `quantity` INT NOT NULL DEFAULT 1,
    `unit_price` DECIMAL(10, 2) NOT NULL,
    CONSTRAINT `fk_items_order` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_items_product` FOREIGN KEY (`product_id`) REFERENCES `products` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 5. Audit Logs (Virtual Foreign Key without physical DDL constraint: operator_id -> users.id)
DROP TABLE IF EXISTS `audit_logs`;
CREATE TABLE `audit_logs` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `operator_id` INT NOT NULL,
    `action` VARCHAR(50) NOT NULL,
    `client_ip` VARCHAR(45) NOT NULL,
    `payload` JSON DEFAULT NULL,
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_audit_operator` (`operator_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Seed Data Insertion
INSERT INTO `products` (`id`, `name`, `category`, `price`, `stock`) VALUES
(1, 'MacBook Pro 16 M3 Max', 'Electronics', 3499.00, 50),
(2, 'Sony WH-1000XM5 Headphones', 'Electronics', 399.00, 120),
(3, 'Ergonomic Office Chair', 'Furniture', 450.00, 80),
(4, 'Mechanical Keyboard 75%', 'Electronics', 129.00, 200),
(5, '4K UltraWide Monitor 34"', 'Electronics', 799.00, 45);

INSERT INTO `users` (`id`, `name`, `email`, `phone`, `id_card`, `address`, `metadata`, `last_order_id`) VALUES
(1, '张伟', 'zhangwei.corp@example.com', '13800138001', '110101199003072381', '北京市海淀区中关村南大街1号', '{"vip_level": 5, "card_no": "622202100012345678"}', NULL),
(2, 'Alice Smith', 'alice.smith@techcorp.io', '15811223344', '310104198811124562', '上海市浦东新区张江高科张东路1387号', '{"vip_level": 3, "card_no": "621700200098765432"}', NULL),
(3, '李娜', 'lina.finance@example.org', '18699887766', '440301199505201149', '深圳市南山区科技园南区高新南道', '{"vip_level": 1, "card_no": "436742100055443322"}', NULL),
(4, 'Bob Johnson', 'bob.johnson@startup.dev', '13955667788', '330106199208153321', '杭州市西湖区文三路古翠路口', '{"vip_level": 2, "card_no": "518710300066778899"}', NULL);

INSERT INTO `orders` (`id`, `user_id`, `order_no`, `amount`, `status`) VALUES
(101, 1, 'ORD-20261001-0001', 3898.00, 'completed'),
(102, 1, 'ORD-20261001-0002', 129.00, 'processing'),
(103, 2, 'ORD-20261001-0003', 799.00, 'completed'),
(104, 3, 'ORD-20261001-0004', 3499.00, 'refunded'),
(105, 4, 'ORD-20261001-0005', 450.00, 'cancelled');

-- Update circular FKs
UPDATE `users` SET `last_order_id` = 101 WHERE `id` = 1;
UPDATE `users` SET `last_order_id` = 103 WHERE `id` = 2;
UPDATE `users` SET `last_order_id` = 104 WHERE `id` = 3;
UPDATE `users` SET `last_order_id` = 105 WHERE `id` = 4;

INSERT INTO `order_items` (`id`, `order_id`, `product_id`, `quantity`, `unit_price`) VALUES
(1001, 101, 1, 1, 3499.00),
(1002, 101, 2, 1, 399.00),
(1003, 102, 4, 1, 129.00),
(1004, 103, 5, 1, 799.00),
(1005, 104, 1, 1, 3499.00),
(1006, 105, 3, 1, 450.00);

INSERT INTO `audit_logs` (`id`, `operator_id`, `action`, `client_ip`, `payload`) VALUES
(5001, 1, 'USER_LOGIN', '192.168.1.101', '{"browser": "Chrome", "os": "macOS"}'),
(5002, 1, 'ORDER_CREATE', '192.168.1.101', '{"order_id": 101, "amount": 3898.00}'),
(5003, 2, 'ORDER_PAY', '10.0.12.44', '{"order_id": 103, "channel": "wechat"}'),
(5004, 3, 'REFUND_REQUEST', '172.16.8.20', '{"order_id": 104, "reason": "item damaged"}');

SET FOREIGN_KEY_CHECKS = 1;
