#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'Linux x64 is required.' >&2; exit 1; }
export GOTOOLCHAIN=go1.26.10
for command in go node npm pkg-config gcc dpkg-deb dpkg-shlibdeps zip; do
  command -v "$command" >/dev/null || { echo "Missing build tool: $command" >&2; exit 1; }
done
pkg-config --exists gtk4 webkitgtk-6.0
[[ $(node -p 'Number(process.versions.node.split(".")[0])') -ge 24 ]]
output="$PWD/bin/linux-amd64"
mkdir -p "$output"
(cd frontend && npm ci && npm run build)
node scripts/engine-overlay.mjs --test
go build -trimpath -tags production -ldflags '-s -w' -o "$output/SDrive" .
install -m 755 .tools/rclone/rclone "$output/rclone"
if [[ ${1:-} == --check ]]; then
  bash scripts/test-linux.sh
fi
"$output/SDrive" --self-check
SDRIVE_DIST_DIR="$output" node scripts/licenses.mjs
version=$(node -p 'JSON.parse(require("fs").readFileSync("frontend/package.json","utf8")).version')
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
stage=$(mktemp -d "$PWD/.tools/linux-package.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$stage/portable" "$stage/deb/DEBIAN" "$stage/deb/usr/lib/sdrive" "$stage/deb/usr/bin" "$stage/deb/usr/share/applications" "$stage/deb/usr/share/icons/hicolor/256x256/apps" "$stage/deb/usr/share/doc/sdrive"
install -m 755 "$output/SDrive" "$output/rclone" "$stage/portable/"
cp -a "$output/licenses" "$stage/portable/"
chmod -R a+rX "$stage/portable/licenses"
cp LICENSE THIRD_PARTY_NOTICES.md docs/linux.md "$stage/portable/"
cp -a "$stage/portable/." "$stage/deb/usr/lib/sdrive/"
ln -s ../lib/sdrive/SDrive "$stage/deb/usr/bin/sdrive"
install -m 644 assets/sdrive.png "$stage/deb/usr/share/icons/hicolor/256x256/apps/sdrive.png"
cat > "$stage/deb/usr/share/applications/org.sdrive.SDrive.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=SDrive
Comment=Mount cloud storage and remote servers
Comment[pt_BR]=Monte armazenamento na nuvem e servidores remotos
Exec=/usr/lib/sdrive/SDrive
Icon=sdrive
Terminal=false
Categories=Utility;FileTools;
StartupNotify=true
DESKTOP
mkdir -p "$stage/debian"
printf 'Source: sdrive\nSection: utils\nPriority: optional\nMaintainer: SDrive contributors\n\nPackage: sdrive\nArchitecture: amd64\nDescription: Remote storage desktop application\n' > "$stage/debian/control"
dependencies=$(cd "$stage" && dpkg-shlibdeps -O -e"$output/SDrive" | sed -n 's/^shlibs:Depends=//p')
[[ -n $dependencies ]] || { echo 'Could not determine runtime dependencies.' >&2; exit 1; }
cat > "$stage/deb/DEBIAN/control" <<CONTROL
Package: sdrive
Version: $version
Architecture: amd64
Maintainer: SDrive contributors
Section: utils
Priority: optional
Depends: $dependencies, fuse3, xdg-utils, dbus-user-session, procps
Recommends: gnome-keyring, zenity
Description: Desktop access to cloud storage and remote servers
 Mount storage into private local folders with persistent upload recovery.
CONTROL
for hook in preinst prerm; do
  cat > "$stage/deb/DEBIAN/$hook" <<'HOOK'
#!/bin/sh
set -eu
if pgrep -x SDrive >/dev/null; then
  echo 'Exit SDrive safely before changing its installation. Your profile and cache are preserved.' >&2
  echo 'Saia do SDrive com segurança antes de alterar a instalação. O perfil e o cache estão preservados.' >&2
  exit 1
fi
HOOK
  chmod 755 "$stage/deb/DEBIAN/$hook"
done
dpkg-deb --root-owner-group --build "$stage/deb" "$output/SDrive-$version-linux-amd64.deb"
tar -C "$stage/portable" -czf "$output/SDrive-$version-linux-amd64.tar.gz" .
# Explicit source roots include new files, without distributing local backlog notes.
rm -f -- "$output/SDrive-source.zip"
zip -qr "$output/SDrive-source.zip" ./*.go go.mod go.sum assets internal tests frontend/src frontend/index.html frontend/package.json frontend/package-lock.json frontend/tsconfig.json frontend/vitest.config.ts scripts docs installer LICENSE README.md THIRD_PARTY_NOTICES.md .github .gitignore .editorconfig .gitattributes
(cd "$output" && sha256sum SDrive rclone "SDrive-$version-linux-amd64.deb" "SDrive-$version-linux-amd64.tar.gz" SDrive-source.zip rclone-source.zip > SHA256SUMS && sha256sum -c SHA256SUMS)
node scripts/release-assets.mjs linux
echo "Linux distribution ready at $output"
