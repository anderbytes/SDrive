# Contributing

For everyday use and troubleshooting, start with the [user guide](usage.md). To change the code, follow the [development guide](development.md). Prefer small changes with focused tests for the behavior being changed. Keep source code, comments, documentation, issues and developer-facing messages in English. Put interface translations in the [locale catalog](localization.md). Do not include credentials, data-directory files or real account content.

The maintainer cannot personally test every connection type, feature, account configuration or Windows environment. Thank you for your understanding and for helping cover these gaps. You can contribute by reproducing problems, testing the connections and features available to you, or improving documentation; code contributions are not required.

Use the [bug report form](../.github/ISSUE_TEMPLATE/bug-report.yml) to report the installed version, environment, reproduction steps, expected behavior and observed behavior. Review diagnostics and screenshots for private information before sharing. Use disposable accounts, folders and files for testing, especially for writes, recovery and disconnection; never put important data at risk to validate a change. Report what passed, failed or was not tested, without implying coverage of other environments.

To validate a provider, follow the [matrix and checklist](provider-validation.md). Open an issue using the [provider validation form](../.github/ISSUE_TEMPLATE/provider-validation.yml) and, optionally, a PR updating only results you actually observed. Automated tests and local fixtures do not establish live-provider compatibility; do not mark a provider as fully validated based on mocked tests.

When updating rclone, check the field catalog, authentication question protocol, mount options and the cache `Dirty` metadata reader. Run the real self-check, tests and recovery checklist. Do not remove that reader without replacing its protection for files still open for writing.

CI builds for Windows, runs tests and produces artifacts. It does not access real accounts, publish releases or hold provider secrets. Maintainers can add code signing and publishing later.

With WinFsp installed, the mount integration test uses only a temporary folder and a free drive letter. In PowerShell:

```powershell
$env:SDRIVE_TEST_MOUNT = '1'
try { ./.tools/go/bin/go.exe test ./tests/integration -run '^TestWindowsMountLifecycle$' -v }
finally { Remove-Item Env:SDRIVE_TEST_MOUNT }
```
