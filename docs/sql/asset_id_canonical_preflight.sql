-- Run against the actual target PostgreSQL database before deploying the
-- canonical AssetID migration. This transaction cannot change database data.
-- The result contains counts only; it does not print asset IDs or ciphertext.
BEGIN TRANSACTION READ ONLY;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

SELECT COUNT(*) AS canonical_collision_groups,
       COALESCE(SUM(group_size), 0) AS affected_rows
FROM (
    SELECT COUNT(*) AS group_size
    FROM server_configs
    WHERE BTRIM(asset_id) <> ''
    GROUP BY user_id, LOWER(BTRIM(asset_id))
    HAVING COUNT(*) > 1
) AS collisions;

SELECT COUNT(*) AS noncanonical_rows_to_normalize
FROM server_configs
WHERE asset_id <> LOWER(BTRIM(asset_id));

ROLLBACK;
