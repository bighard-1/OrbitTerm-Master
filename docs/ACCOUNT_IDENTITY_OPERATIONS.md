# Account identity canonicalization

OrbitTerm treats usernames as case-insensitive, trimmed email-style account
identifiers. The server stores and looks them up in their canonical lowercase
form, and PostgreSQL enforces uniqueness on `LOWER(BTRIM(username))`.

## Safe deployment preflight

Take a verified PostgreSQL backup before deploying the version that enables
the canonical unique index. Then run this read-only audit against the target
database:

```sql
SELECT LOWER(BTRIM(username)) AS canonical_username,
       ARRAY_AGG(id ORDER BY id) AS user_ids,
       COUNT(*) AS count
FROM users
GROUP BY LOWER(BTRIM(username))
HAVING COUNT(*) > 1;
```

An empty result is required. On startup, the migration runs the same audit,
normalizes stored usernames, invalidates only the sessions of rows whose
username changed, and creates `uq_users_canonical_username`.

## Historical duplicate identities

Do not delete or automatically merge duplicate users. Their server
configurations are encrypted and each row is scoped to a distinct user ID.
Automatic reassignment can silently join unrelated accounts or overwrite an
asset with the same logical ID.

If the audit returns rows:

1. Keep the production service on its current version and preserve the backup.
2. Confirm the intended owner with the account holder using the diagnostics
   fingerprint shown by the client; never ask for a password or master key.
3. Export the affected accounts through the existing encrypted migration and
   recovery workflow, or perform an operator-reviewed data migration in a
   maintenance window.
4. Re-run the read-only audit. Deploy only after it returns no rows.

The migration intentionally refuses to start when duplicate canonical names
remain. This is a safety stop, not a condition to bypass.
