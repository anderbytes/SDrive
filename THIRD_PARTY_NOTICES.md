# Third-party components

SDrive source code is GPL-3.0-only. Dependencies retain their own licenses.

| Component | Version | Primary license | Source |
| --- | --- | --- | --- |
| Go | 1.26.10 | BSD-3-Clause | https://go.dev/LICENSE |
| Wails and JS runtime | 3.0.0-beta.34 | MIT | https://github.com/wailsapp/wails |
| React / React DOM | See package-lock.json | MIT | https://github.com/facebook/react |
| TypeScript | See package-lock.json | Apache-2.0 | https://github.com/microsoft/TypeScript |
| rclone | 1.75.2, rebuilt with Go 1.26.10 | MIT | https://github.com/rclone/rclone/tree/v1.75.2 |
| WinFsp, downloaded automatically or included offline | 2.1.25156 | GPLv3 with FOSS exception | https://github.com/winfsp/winfsp/blob/v2.1/License.txt |
| WebView2, downloaded automatically or included offline | Evergreen x64, payload pinned in manifest | Microsoft terms, proprietary component | https://developer.microsoft.com/microsoft-edge/webview2/ |
| NSIS, build tool | 3.12 | NSIS / zlib licenses | https://nsis.sourceforge.io/License |

Packaging generates `bin/licenses/` with license/notice texts for Go modules present in the executables and production npm dependencies. `modules.json` records versions, checksums and source URLs. Exact rclone sources are provided in `rclone-source.zip`.

The offline package includes the unmodified WinFsp MSI and WebView2 installer. The online package downloads the same official files when needed. Both include the full WinFsp license in `licenses/WinFsp-LICENSE.txt`. WebView2 remains a separate component, subject to [Microsoft terms](https://www.microsoft.com/legal/terms-of-use) and [Microsoft privacy policy](https://privacy.microsoft.com/privacystatement), with its own updates. SDrive's GPL license does not apply to WebView2. Complete payloads support the [Microsoft-documented offline flow](https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution#offline-deployment) in the variant that includes them.

`licenses/prerequisites.json` records official addresses and SHA-256 values for both components. Packaging requires valid Authenticode signatures from their publishers; both variants recheck SHA-256, signature and publisher before executing any component.

rclone sources are unmodified. The build uses `-tags cmount`, `CGO_ENABLED=0`, `-trimpath` and injects only version `v1.75.2` into `fs.Version`. Module checksum: `h1:0cqa6P3WmhGnu2NeOdZSetJnacpwCRtpsNbHOICqPk8=`.

Mirrored NSIS archive source: `https://distfiles.macports.org/nsis/nsis-3.12.zip`. SHA-256 checked against the [MacPorts Portfile](https://github.com/macports/macports-ports/blob/master/devel/nsis/Portfile): `56581f90db321581c5381193d796fffcf2d24b2f8fed2160a6c6a3baa67f2c4f`.
