-- 初始化資料庫
CREATE DATABASE IF NOT EXISTS ledger_db DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE ledger_db;

-- Users 表：會員與餘額
CREATE TABLE IF NOT EXISTS users (
    id BIGINT NOT NULL AUTO_INCREMENT,
    balance BIGINT NOT NULL DEFAULT 0 COMMENT '餘額 (定點數, 放大 10000 倍)',
    created_at BIGINT NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='會員餘額表';

-- Transactions 表：交易明細
CREATE TABLE IF NOT EXISTS transactions (
    id BIGINT NOT NULL AUTO_INCREMENT,
    ref_id BINARY(16) NOT NULL COMMENT '外部冪等金鑰 (UUID)',
    sequence BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '全域順序號 (由 Core 分配)',
    from_account_id BIGINT NOT NULL DEFAULT 0,
    to_account_id BIGINT NOT NULL DEFAULT 0,
    amount BIGINT NOT NULL DEFAULT 0,
    type TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '1:Deposit, 2:Withdraw, 3:Transfer',
    created_at BIGINT NOT NULL DEFAULT 0 COMMENT '交易時間戳 (Unix)',

    PRIMARY KEY (id),
    UNIQUE KEY uk_ref_id (ref_id), -- 確保冪等性
    KEY idx_from_account (from_account_id),
    KEY idx_to_account (to_account_id),
    KEY idx_sequence (sequence) -- 用於 WAL 重放檢查
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='交易明細表';

-- View: 開發者可讀的餘額表 (除以 10000)
CREATE OR REPLACE VIEW human_readable_users AS
SELECT
    id,
    balance / 10000.0 as real_balance,
    FROM_UNIXTIME(created_at) as created_at,
    FROM_UNIXTIME(updated_at) as updated_at
FROM users;
