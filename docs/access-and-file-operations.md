# Application access and mounted file operations

## Application lock

Choose **Application lock** to configure a local PIN containing 6–12 digits. The lock is initially disabled. Enabling it generates a recovery code, shown once; keep it privately outside SDrive before dismissing the screen. Changing the PIN or using recovery rotates that code. Updating only the inactivity policy keeps it unchanged.

The lock protects SDrive's management controls, including direct bridge requests and tray drive actions. It does not encrypt cached files, revoke Windows access, or hide drives from Explorer. Connected drives remain usable by other applications. It does not protect against the Windows account owner, administrators, malware running as that user, profile modification, or older software run against modified settings. Use Windows session locking and disk protection for those boundaries.

With protection enabled, reopening starts locked and defers automatic mounting until a successful unlock. Manual **Lock SDrive** hides management controls immediately. The tray and a second launch can reveal the unlock screen, but cannot unlock it. **Exit safely** remains available; unresolved uploads, open writers, unknown cache state, recovery, or unfinished setup can prevent exit. Locked exit errors do not reveal connection names.

Locking requests safe cancellation of an unfinished configuration session. It does not forcibly stop configuration jobs or erase credentials when cancellation is uncertain. After unlocking, **Cancel unfinished setup** retries cleanup of a retained session. Previously accepted filesystem operations may finish while management is locked; their recovery checks remain active.

### Inactivity and reconnect

The default timeout when configuring protection is 15 minutes. Set it to 0 for manual/startup locking only. Activity is measured in the current Windows login session, using `GetLastInputInfo`; using Explorer or another application counts as computer activity. The application samples every five seconds, including while hidden. An unavailable idle measurement never authorizes disconnect.

**Disconnect safely after inactivity** is off by default. When enabled, reaching the timeout first locks management, then tries the existing safe disconnect flow for each connected drive. It never forces an unmount, stops another application, closes its handles, clears cache, or treats unknown pending state as clean. Failure leaves an explanatory warning after unlocking. Closing open files and retrying remains the user's responsibility.

**Reconnect idle-disconnected drives after unlock** is also off by default. It is available only with idle disconnect and remembers, in memory, only successful disconnections from that idle episode. It does not persist a reconnect list across restarts or reconnect drives manually disconnected earlier. Initial application startup retains the existing saved AutoMount behavior after unlock. Session-only access, expired authentication and recovery still require their normal account/destination checks; unlock is not account authorization.

### Storage and forgotten PINs

`application-lock.dpapi` is independent of configuration exports and settings restoration. It contains a format version, random salt, Argon2id PIN verifier (64 MiB, three iterations, one lane), SHA-256 verifier of a random 160-bit recovery code, inactivity policy, and failed-attempt state. Windows DPAPI protects the whole record for the current user; the PIN and recovery code themselves are never stored. The profile's existing permissions and atomic writes also apply. Diagnostics do not include these values.

After five incorrect verifications, a 30-second wait doubles on further failures to a maximum of 15 minutes. PIN and recovery attempts share the counter. The wait survives reopening; adjusting the system clock can affect its duration. Verification must be durably saved before unlocking, and missing/corrupt expected records fail closed. This is a local management lock, not a tamper-proof authentication service.

Use **Forgot PIN?** and the recovery code to set a replacement PIN. Save the replacement recovery code, then enter the new PIN to unlock. Connection data, provider credentials, cache and pending uploads are preserved. Export/import never transports the lock, and restoring settings retains the current protection marker.

If both secrets are lost or the lock record is damaged, administrative reset under the original Windows account is possible because the account owner controls the profile. First use **Exit safely** and privately back up the complete profile and selected cache. If safe exit is refused, preserve the running recovery state and seek assistance rather than force-stopping it or deleting data. Once SDrive is fully closed, archive only `application-lock.dpapi` outside the active profile and set the top-level `accessProtected` value in `settings.json` to `false`, retaining a private copy of the original settings. Do not alter connection entries, version numbers, keys, encrypted configurations, recovery markers or cache. Reopen and configure a new PIN. Invalid settings must use the documented settings-recovery flow. Never publish the profile or archived lock record.

Settings writes use version 5; unversioned settings and versions 1–4 remain readable. Older applications reject version 5. Do not edit the version to bypass compatibility checks.

## Applications using a file

For a connected drive, open its **More actions** menu and choose **Applications using a file**. Select one regular file in that mounted drive. **Refresh** repeats the query for that selected relative path; it does not scan the drive or maintain background polling.

The query uses Windows Restart Manager as the current user. Results include reported application names, PIDs, process start times and the check time. They identify Windows-reported use of that file, not current read/write activity, complete handle ownership, or a guarantee that the drive can disconnect. Queued uploads are a separate engine/cache measurement. No reported processes means only that Windows returned none for that file; errors, permissions, cancellation and unavailable information remain **unknown**. Processes that exit disappear on a fresh query; identity includes start time because PIDs can be reused.

Only one query runs at a time. The native query requests cancellation after five seconds; Windows resource registration may take longer, during which another query remains blocked. No service mutex is held while waiting on the dialog or Windows. A lock or changed mount discards late results. Queries never call Restart Manager shutdown/restart functions, terminate applications, install drivers, request elevation, persist private paths in support reports, or send telemetry.

## Shared editing and Explorer operations

Windows file sharing restrictions apply within a mount. They are not a distributed editing lock. The pinned rclone 1.75.2 mounting path does not establish a coordinated provider lock spanning independent clients. Two isolated clients mounted to disposable WebDAV storage both obtained exclusive local read/write handles to the same remote file. SDrive therefore makes no cross-computer exclusive-editing claim; no provider/application pair is certified. Local exclusive handles and read-only mounts must not be described as remote coordination.

Existing engine behavior must be measured before adding an integration. In the disposable WebDAV test:

| Operation | Observed behavior |
| --- | --- |
| Rename within a mount | Remote `MOVE`, without content GET/PUT |
| Windows Shell move within the same drive | Remote `MOVE`, without content GET/PUT |
| Windows Shell copy | Content downloaded and uploaded; no remote `COPY` |
| Delete a mounted file | Remote `DELETE`, without content download |
| Rejected remote move | Failure returned; original server content preserved |
| Shell rename-on-collision | Existing target preserved; renamed copy content verified |
| Rejected upload after Shell copy | Original source and dirty local target preserved; upload recovered after connectivity returned |
| Copy between independent backends | Upload/download fallback; source and target hashes verified |

These are pinned-engine observations for synthetic loopback WebDAV, not certification of all providers. Shell operations use Windows Shell automation over disposable paths, not a new shell extension or interception of every interactive Explorer workflow. Native Shell owns its conflict and progress behavior. Interactive conflict dialogs, provider differences, partial-copy cancellation and distributed editing remain separate acceptance. The same-host independent-client fixtures use distinct network display names: identical names collided because the current Windows network prefix derives from the display name. That existing namespace behavior is unchanged. No new overwrite/cancellation guarantee, server-side-copy toggle, engine fork, or Explorer interception was added.

## Focused validation

Core tests cover durable throttling, recovery rotation, malformed/missing records, write failure, all exported management gates, restricted snapshots, settings compatibility, inactivity and unknown-state preservation. File-access tests cover path containment, cancellation, overlapping queries and locking during file selection. Frontend tests cover immediate management removal, obsolete status responses, recovery acknowledgement, localization and unknown/empty process results.

Native checks require the existing WinFsp installation, free disposable letters and the repository's disk-space prerequisites. They never install prerequisites, use personal accounts, change startup entries or access existing user mounts. Enable `SDRIVE_TEST_MOUNT=1` only for the selected tests:

```powershell
./.tools/go/bin/go.exe test ./tests/integration -run 'TestWindowsMountedFileProcessDiscovery|TestIndependentMountedClientsDoNotShareExclusiveFileHandles|TestMountedRemoteOperationsMeasureExistingWindowsShellBehavior|TestWindowsMountOpenFiles' -count=1 -v -timeout 5m
```

The Wails smoke includes PIN setup, wrong-PIN rejection, recovery, manual lock/unlock in both languages, second-instance activation while locked and safe exit using a disposable profile. It records lock-screen images without PINs or recovery codes. Real hardware idle intervals, power failure, Linux, external providers, exhaustive application compatibility and installer execution are not certified by these checks.

Sources: [session idle input](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getlastinputinfo), [Restart Manager registration](https://learn.microsoft.com/en-us/windows/win32/api/restartmanager/nf-restartmanager-rmregisterresources), and [Restart Manager process lists](https://learn.microsoft.com/en-us/windows/win32/api/restartmanager/nf-restartmanager-rmgetlist), consulted October 25, 2026. Engine implementation was inspected in the locally pinned rclone 1.75.2 sources.
