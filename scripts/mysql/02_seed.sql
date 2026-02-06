USE ledger_db;

-- 建立一個 Stored Procedure 來產生大量測試資料
DROP PROCEDURE IF EXISTS SeedUsers;

DELIMITER //

CREATE PROCEDURE SeedUsers(IN num_users INT)
BEGIN
    DECLARE i INT DEFAULT 1;
    DECLARE current_ts BIGINT;
    SET current_ts = UNIX_TIMESTAMP() * 1000;

    -- 使用 Transaction 加速插入
    START TRANSACTION;

    WHILE i <= num_users DO
        -- 假設初始每個帳號都有 10,000 元 (10000 * 10000 = 100000000)
        INSERT INTO users (id, balance, created_at, updated_at)
        VALUES (i, 100000000, current_ts, current_ts)
        ON DUPLICATE KEY UPDATE balance = balance; -- 如果已存在則不變

        SET i = i + 1;
    END WHILE;

    COMMIT;
END //

DELIMITER ;

-- 執行 Seed: 預設先產生 10 個帳號供開發測試 (壓測時可手動呼叫 CALL SeedUsers(100000))
CALL SeedUsers(10);
