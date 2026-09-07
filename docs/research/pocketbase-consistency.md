# PocketBase transaction and file consistency guarantees

**Research ticket:** [#407](https://github.com/platformod/united/issues/407)
**Parent:** [#405](https://github.com/platformod/united/issues/405)
**Research branch:** `research/pocketbase-consistency`
**Research date:** 2026-09-05
**Baseline:** fresh from `main`; no `nrh/plan-two` code or conclusions were used as authority.

## Executive conclusion

A single PocketBase process is a reasonable database coordination point, but PocketBase record transactions do **not** provide an atomic transaction spanning SQLite metadata and the file backend.

What is guaranteed:

- `app.RunInTransaction` commits the database operations performed through the callback's `txApp` only when the callback returns `nil`; an error rolls the database transaction back.
- SQLite allows many concurrent readers but only one concurrent writer. PocketBase deliberately exposes a concurrent read pool and a one-connection nonconcurrent write/transaction pool, so competing writes queue at the PocketBase write pool and still obey SQLite's single-writer rule.
- SQLite's WAL transaction machinery gives atomic database commit/recovery subject to its documented filesystem/durability assumptions.

What is **not** guaranteed:

- A file upload/delete and the SQLite row update commit atomically together. PocketBase's file interceptor uploads before the SQL write, deletes replaced files after successful persistence, and attempts cleanup on failures. Those filesystem operations are separate side effects and cleanup is best effort.
- A process crash can be treated as an ordinary transaction rollback for files. A crash after a file upload but before the row commit can leave an orphan; a crash after metadata commit but before post-commit deletion can leave an obsolete file. PocketBase cleans its temporary directory at bootstrap, but that is not a general storage reconciliation pass.
- Concurrent request arrival order is preserved. Requests run concurrently; the single writer serializes database commits, but the order is scheduler/queue order, not request-send order.
- A database row that names a file proves that the file is readable at the moment of use, especially with an external S3-compatible backend or after partial failure.

For immutable encrypted Terraform state versions, the safe design is therefore: make each version an immutable, content-addressed or uniquely identified object whose durable/servable status is represented by database metadata only after the object write has succeeded; never overwrite a committed version; reconcile orphan/unreadable objects; and make readers select only committed metadata. For exclusive lock leases, use one short database transaction for compare-and-swap ownership/expiry state, keep external work outside that transaction, and use expiry plus fencing/version tokens so a crashed or delayed holder cannot continue authoritatively after lease loss.

## Sources and scope

Primary sources consulted:

- PocketBase current Go record documentation (currently rendered as v0.40.2): [Go record operations](https://pocketbase.io/docs/go-records/), especially the **Transaction** section.
- PocketBase current file documentation: [Files upload and handling](https://pocketbase.io/docs/files-handling/).
- PocketBase current source, `core/field_file.go`: [raw source](https://raw.githubusercontent.com/pocketbase/pocketbase/master/core/field_file.go).
- PocketBase current source, `core/base.go`: [raw source](https://raw.githubusercontent.com/pocketbase/pocketbase/master/core/base.go).
- PocketBase current source, `core/app.go`: [raw source](https://raw.githubusercontent.com/pocketbase/pocketbase/master/core/app.go).
- PocketBase current source, `tools/filesystem/filesystem.go`: [raw source](https://raw.githubusercontent.com/pocketbase/pocketbase/master/tools/filesystem/filesystem.go).
- SQLite documentation: [Write-Ahead Logging](https://sqlite.org/wal.html) and [Transactions](https://sqlite.org/lang_transaction.html).

The conclusions below separate direct source facts from implications for United. PocketBase's public maintainer discussions were used only as corroborating context, not as the primary basis for the guarantees.

## Record transactions

PocketBase's Go documentation states that `app.RunInTransaction(fn)` persists the database operations only when the callback returns `nil`, and that nested transactions are safe when the callback's `txApp` is used. It specifically warns to use `txApp`, not the original `app`, because PocketBase allows only one writer/transaction at a time and mixing the instances can deadlock. It also advises keeping slow or external work out of the transaction.

The current `core.App` interface documents the same contract: `RunInTransaction` wraps the regular app database and is safe to nest using the callback app. The interface documents separate `ConcurrentDB` and `NonconcurrentDB` builders; the latter is limited to one open connection and queues database operations. PocketBase's `BaseApp.initDataDB` configures the concurrent pool with the normal configured pool size and the nonconcurrent pool with `MaxOpenConns(1)` and `MaxIdleConns(1)`.

The current `core/field_file.go` implementation is important because file processing is hook/interceptor behavior around the record save, not a SQLite primitive. Its create/update interceptor:

1. Finds the old file value.
2. Calls `processFilesToUpload` before the SQL action. That opens a filesystem and calls `UploadFile` for each new file.
3. Runs the SQL action.
4. On SQL failure, attempts to delete the newly uploaded files.
5. On SQL success, records files to delete and later processes those deletes in the after-success path.

The source explicitly treats transactional errors specially: when `app.IsTransactional()` is true, the file interceptor assumes cleanup was handled by the transaction's failure path. The cleanup paths log failures rather than making an all-or-nothing cross-system commit promise. On a non-transactional record-save failure, the interceptor tries to delete new files and, for a failed create, tries to remove an empty record directory.

The current `core.App` documentation also says that pre-commit model hooks can run before the wrapping transaction commits; the after-success hooks are delayed until after commit and are not triggered on rollback. That distinction prevents an after-success hook from being interpreted as proof that an earlier external side effect participated in the database commit.

### Guarantees and limits

| Situation | Database row | File/object side effect | Resulting guarantee |
| --- | --- | --- | --- |
| Callback returns `nil` | Atomic SQLite commit | Any file uploaded before SQL remains unless post-commit cleanup removes old files | Row commit is atomic; file set is only eventually/operationally consistent |
| Callback returns error | SQLite transaction rolls back | New uploads are attempted for cleanup | No row commit, but cleanup failure can leave orphan files |
| Upload fails before SQL | No SQL action should commit from that save | Partial successful uploads are attempted for deletion | Usually no row, but cleanup is best effort |
| SQL fails after upload | Row rolls back | Newly uploaded files are attempted for deletion | Orphan is possible if delete fails or process dies |
| Commit succeeds, old-file delete fails | New row is visible | Old file can remain | Safe read availability, but storage leak/stale object remains |
| Process crashes during the sequence | SQLite recovery handles DB transaction according to SQLite guarantees | No application cleanup callback is guaranteed to run | Cross-system atomicity is absent |

## SQLite concurrency in the single-process/single-writer deployment

SQLite's transaction documentation says it supports multiple simultaneous read transactions, including across connections/processes, but only one simultaneous write transaction. A read transaction upgraded to a write transaction can return `SQLITE_BUSY` if another connection has modified or is modifying the database. In WAL mode, `BEGIN IMMEDIATE` and `BEGIN EXCLUSIVE` both start a write transaction immediately; WAL permits readers and writers to proceed concurrently, but there is still only one writer.

SQLite's WAL documentation states:

- Readers and writers can run at the same time.
- Writers append to one WAL, so only one writer can exist at a time.
- A reader sees a stable snapshot for the duration of its transaction.
- A commit in WAL is represented by a valid commit record in the WAL; checkpointing later copies it back to the main database.
- With `synchronous=NORMAL`, durability after power loss is weaker; with `FULL`, writers sync the WAL on each commit.
- A WAL database has associated `-wal` and `-shm` state that must be kept with the database when copying/moving it.
- SQLite can return `SQLITE_BUSY` in unusual WAL cases, including recovery after a crash or cleanup when the last connection closes.

PocketBase's current source routes reads to a concurrent pool and writes to a one-connection nonconcurrent pool specifically to minimize `SQLITE_BUSY`. This is useful serialization in one process, but it is not a fairness or request-order guarantee. Concurrent HTTP requests can perform reads and construct records concurrently; the writer pool determines which write reaches SQLite first. A read-modify-write operation can therefore lose an update unless it is protected by a transaction and/or an optimistic condition/version check.

For this ticket's deployment assumption, a single PocketBase process means there is one application-level write queue and one SQLite database file. That substantially reduces coordination risk compared with multiple independent writers. It does not make a request's whole handler atomic, does not coordinate filesystem operations, and does not turn a lease into a fencing mechanism. Avoid holding a database transaction open while doing encryption, S3/file I/O, network calls, or other slow work; doing so extends the single-writer critical section and increases contention/timeout risk.

## File-field staging, commit, and cleanup

PocketBase's file documentation says normal local storage is `pb_data/storage`, while S3-compatible storage is also supported. The current source constructs a filesystem per operation and calls its `UploadFile`, `Delete`, and `DeletePrefix` methods. The filesystem abstraction has no transaction or rollback interface.

The file-field source provides a useful pseudo-protocol:

```text
validate record
  -> upload each new file to its final record path
  -> execute SQL INSERT/UPDATE
  -> if SQL failed: try deleting newly uploaded files
  -> if SQL succeeded: remember old files to delete
  -> after DB success: delete removed/replaced files
```

Uploads are attempted sequentially. If one fails, PocketBase stops and attempts to delete the files that succeeded earlier. A close/write error is returned by the filesystem writer, but there is no atomic rename/commit coupling to the database row. The local and S3 implementations are both behind the same non-transactional filesystem interface.

The source also shows that deleting a record's entire storage prefix after a record deletion is deliberately optimistic and asynchronous (`FireAndForget`) so it does not block the database delete transaction. This is an explicit example of eventual file cleanup even after a successful database operation.

PocketBase removes `pb_data/.pb_temp_to_delete` during bootstrap. That protects a particular application-managed temporary area, but current source does not turn it into a journal of every record-file operation and does not scan/reconcile arbitrary record directories against database rows. Therefore it should not be relied on as crash recovery for a custom state-version file protocol.

### Required interpretation for encrypted state objects

If a state version is stored as a file field, the row should be treated as the authoritative visibility/index record, not the existence of a file alone. A robust write protocol should be designed explicitly, for example:

1. Generate an immutable version ID and encrypt the state before or during a staging phase.
2. Write the encrypted bytes to a unique staging/final object key; verify the write and, where supported, size/digest/ETag or a read-after-write check.
3. In a short PocketBase transaction, insert the version metadata with a status such as `committed` (or insert an intent first and transition it only after verification). The transaction must use `txApp`.
4. Readers select only committed metadata and fetch the exact immutable object key; they never infer committed state from a directory listing.
5. A reconciler removes staged/orphan objects and marks/remediates committed metadata whose object is missing or unreadable.
6. Never overwrite an existing committed version key. A failed retry creates a new version ID or safely reuses an idempotency key after verifying the existing object.

Whether the final object should be written directly under its immutable key or first under a staging prefix is a design choice, but either choice requires reconciliation because the object store and SQLite cannot share a commit record. If the file field itself is used, PocketBase's ordinary file interceptor already writes to the record's storage path before row commit and performs best-effort cleanup; it should not be advertised as a durable two-phase state publication mechanism.

## Behavior matrix for concurrent requests, failures, and crashes

### Concurrent requests

- Different records: database writes are serialized at the SQLite/PocketBase writer, while reads can overlap. Throughput is bounded by the single writer, especially for long transactions.
- Same logical state key: two handlers can both read the same current version or lock row before either writes. A transaction alone serializes the writes but does not automatically reject stale intent. Use a unique constraint and/or conditional update (`WHERE version = expected`) to make the conflict observable.
- Same record with file replacements: each handler can stage/upload independently. The later database commit may refer to one file set while cleanup from another request races with it. PocketBase calculates old values from a current lookup, but this is not a cross-request compare-and-swap protocol. Immutable, unique object keys avoid destructive overwrite races.
- Request order: no guarantee. The runtime schedules request goroutines independently; writer serialization is not “first HTTP request wins.”

### Failed commits and file failures

- File failure before SQL: PocketBase aborts the save and attempts cleanup of any files already uploaded in that batch.
- SQL constraint/validation failure after upload: database changes do not become visible, while cleanup is best effort.
- Old-file cleanup failure after successful SQL: the new row remains valid and old storage may leak. This is preferable to deleting a file before the new row is committed, but requires monitoring/reconciliation.
- S3 or filesystem outage after metadata commit: readers can observe a committed row whose object is temporarily or permanently unavailable. The application needs a defined response and repair state; SQLite cannot roll back the object operation.

### Process crashes

- Crash before SQL commit: SQLite can recover/rollback the database transaction according to its journal/WAL and filesystem assumptions; uploaded files have no corresponding automatic rollback.
- Crash after SQL commit but before old-file deletion: the committed metadata survives, and obsolete files can remain.
- Crash while uploading: a partial/complete orphan may remain depending on backend writer semantics; the row may not exist.
- Crash during WAL recovery: SQLite documents that the first new connection performs recovery under an exclusive lock, and another concurrent connection may receive `SQLITE_BUSY`. Startup/retry behavior must account for this.
- Hard power loss: SQLite durability depends on journal mode, `synchronous`, storage, and filesystem assumptions. The external file backend has its own durability/visibility guarantees. Treat them as separate recovery domains.

## Exclusive lock leases

PocketBase/SQLite can serialize the database mutation that acquires, renews, or releases a lease, but a lease is not automatically exclusive in the presence of pauses, crashes, or delayed requests. A safe lock record needs at least:

- a unique logical lock key;
- owner identity/token generated per acquisition;
- expiry timestamp/monotonic-safe policy (wall-clock handling must be deliberate);
- a fencing generation/version incremented on each successful acquisition;
- conditional renew/release requiring the current owner token and generation;
- an atomic acquisition transaction that either inserts the lock or updates an expired lock, with a uniqueness constraint preventing two live owners;
- an operational rule that work must stop when renewal fails or the lease expires.

A database lease can prevent two successful acquisitions from committing simultaneously, but it cannot stop an old holder that is paused and later resumes. If a state write must be protected by the lock, carry the fencing generation into the state-version write and reject stale generations in the same database transaction, or use a single-writer command path that checks the generation immediately before publication. Do not rely on PocketBase's single writer alone: it serializes database statements, not external S3 writes or already-running client work.

Keep lease transactions short. Do not hold the SQLite write transaction while encrypting a large state, uploading to S3, waiting on a filesystem, or calling a remote service. If publication is multi-step, represent intent and completion in metadata and make retries/reconciliation idempotent.

## Decisions supported by this research

1. **PocketBase-only is acceptable only with explicit consistency boundaries.** Use PocketBase/SQLite for metadata, uniqueness, lease CAS, and visibility; use a separate object protocol for encrypted bytes.
2. **Do not claim atomic row+file commits.** The externally visible contract must document possible orphan objects and temporary/unrecoverable missing objects, with cleanup/reconciliation.
3. **Immutable versions are strongly preferred.** Unique keys eliminate overwrite races and make retries/recovery safer; deletion is garbage collection, not part of publication correctness.
4. **Use commit metadata/status.** Readers must filter on a committed/complete state and verify the referenced object as needed.
5. **Use fencing for lock-protected work.** Owner/expiry alone is insufficient against stale holders.
6. **Add failure-injection tests/prototype before implementation.** Tests should kill the process at upload-before-row, row-commit-before-cleanup, and lease-renewal boundaries, then restart and reconcile.

## Newly surfaced decision and prototype questions

### Decisions

- Is PocketBase local storage the deployment target, S3-compatible storage, or both? The answer changes visibility, retry, and reconciliation assumptions.
- Is a missing object after a committed metadata row a hard corruption/error, or can the row transition to `unavailable` and be repaired/retried?
- Should committed state metadata include a cryptographic digest and byte length, and should reads verify them?
- Is the consistency goal linearizable publication per Terraform state key, or only “latest committed version eventually readable”?
- What is the lease duration and maximum tolerated process pause/network partition? This determines the fencing and retry policy.
- Does lock release need to be durable/auditable after expiry, or is expiry-only reclamation sufficient?
- Which SQLite/PocketBase version and driver build will be pinned? Current SQLite documentation calls out a WAL-reset bug fixed in SQLite 3.51.3 and later (with backports 3.44.6 and 3.50.7); the deployed driver/runtime must be checked rather than assumed.

### Prototype/test cases

- Concurrent writers for one logical state key: prove one unique version/lock outcome and identify stale-read behavior.
- Upload succeeds, process is killed before SQL: restart, list storage, and reconcile the orphan.
- SQL commits, process is killed before old-file cleanup: confirm the new version remains readable and cleanup later removes only unreferenced objects.
- Upload fails halfway through a multi-file batch: confirm no committed metadata and no undeleted staged objects after reconciliation.
- Object write returns success but immediate read fails or is delayed: decide whether metadata stays pending and how retries work.
- Lease holder pauses past expiry, a second holder acquires, and the first resumes: verify fencing rejects the stale holder's publication.
- Concurrent renewal and acquisition around expiry: verify conditional owner/generation checks and deterministic outcomes.
- Crash/restart during SQLite WAL recovery: verify startup retries and does not serve inconsistent metadata.

## Bottom line

PocketBase gives United a solid single-process SQLite transaction and write-serialization substrate. It does not provide the missing distributed transaction between SQLite and file storage. The replatform can preserve immutable encrypted state-version safety and exclusive lock semantics only by making metadata the publication authority, using immutable object keys, short conditional database transactions, fencing tokens, and an explicit crash/failure reconciliation loop.
