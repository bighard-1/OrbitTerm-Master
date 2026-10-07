# Server maturity rollout and rollback

This branch starts at `04f47db` and retains the canonical asset UUID changes and their collision preflight. No online database or client installation is changed by this branch.

## Compatibility gates

1. The default mode is `production`. Inject a random JWT secret and a strong database password before starting; startup fails before a database connection if these are absent or demonstrably weak. Local development must set `APP_ENV=development` explicitly. Rollback: restore the prior binary with the same secrets. Do not rotate the JWT secret merely to roll back.
2. Managed-account creation and administrator password reset use the registration password policy. Existing passwords are not rehashed or rejected at login. Reset only works while `login_password_reset_enabled` is true. Login and refresh responses include `data.must_change_password` as a boolean, so a reset account can enter the change-password flow immediately. Successful `/api/v1/auth/password` returns `false`. A reset account can use only that endpoint until the password is changed; HTTP 403 `PASSWORD_CHANGE_REQUIRED` remains the server-enforced fallback. Rollback: the old binary ignores this gate; avoid rolling back while reset accounts remain forced-change unless that temporary exposure is acceptable.
3. Session-store rollout must be staged. Existing refresh JWTs lack a session ID; accept them during a documented compatibility window, then disable legacy refresh only after all five clients have upgraded. New refresh JWTs use an opaque ID and rotate atomically in PostgreSQL. A duplicate refresh within 10 seconds returns HTTP 409 `REFRESH_IN_PROGRESS` without revoking the winning session; a later replay revokes the family. Clients must serialize refresh and persist both returned tokens together. Device logout revokes the current family; all-device logout increments the user's token version. Rollback: keep the new table and return to the prior binary; newly issued tokens remain ordinary signed JWTs there and cannot be individually revoked, so treat rollback as an authentication-security downgrade.
4. Request limits introduce HTTP 413 for oversize encrypted uploads and batches. Keep limits above currently supported client payloads and run five-client boundary tests before release. A rejected batch makes no write. Rollback: old binary removes the limit; no data migration needed.
5. Full export reads all tables under one repeatable-read snapshot. Restore rejects a package older than current business data, sync changes, device acknowledgements, or non-export audit records unless the multipart form includes `allow_rollback=ALLOW_ROLLBACK`. Later audit records are retained during an explicit rollback restore. Database overwrite is a cutover: take and verify a separate PostgreSQL backup, stop writers, then restore. Restored users receive a higher token version and refresh sessions are cleared. Rollback requires restoring the pre-cutover database snapshot, not only changing binaries.

Password change can return HTTP 409 `AUTH_STATE_CHANGED` if a concurrent logout or administrator action changed the account. The client should reload its session and prompt the user to sign in again. Administrators marked for password change can complete the required flow in the built-in console.

`BACKUP_LAST_SUCCESS_AT` and `RESTORE_DRILL_VERIFIED_AT` are RFC3339 UTC evidence timestamps supplied by an external backup operator. They are prerequisites for a green readiness report, but do not by themselves prove that backup media is intact. Keep the actual backup job and restore drill logs outside the database being protected.

The asset tombstone minimum remains permanent. The existing optional physical cleanup policy must remain disabled in production until a separate sync-protocol rollout is complete.

## Verification

Run `go test ./...` and the PostgreSQL integration suite with an isolated `ORBITTERM_TEST_DATABASE_URL`. Those integration tests create and remove test data; never point them at production. Run the five client test suites against a staging server before deploying a behavior-changing build.
