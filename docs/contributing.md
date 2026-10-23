# Contributing

Start with the [development guide](development.md). Prefer small changes with focused tests for the behavior being changed. Keep source code, comments, documentation, issues and developer-facing messages in English. Put interface translations in the [locale catalog](localization.md). Do not include credentials, data-directory files or real account content.

To validate a provider, follow the [matrix and checklist](provider-validation.md). Open an issue using the appropriate form and, optionally, a PR updating only results you actually observed. Do not mark a provider as fully validated based on mocked tests.

When updating rclone, check the field catalog, authentication question protocol, mount options and the cache `Dirty` metadata reader. Run the real self-check, tests and recovery checklist. Do not remove that reader without replacing its protection for files still open for writing.

CI builds for Windows, runs tests and produces artifacts. It does not access real accounts, publish releases or hold provider secrets. Maintainers can add code signing and publishing later.

With WinFsp installed, the mount integration test uses only a temporary folder and a free drive letter. In PowerShell:

```powershell
$env:SDRIVE_TEST_MOUNT = '1'
try { ./.tools/go/bin/go.exe test -run TestWindowsMountLifecycle -v . }
finally { Remove-Item Env:SDRIVE_TEST_MOUNT }
```
