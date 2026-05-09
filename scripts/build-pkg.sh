#!/usr/bin/env bash
set -euo pipefail
export COPYFILE_DISABLE=1

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
XCODE_DIR="${ROOT_DIR}/xcode-TM-DNS"
BUILD_DIR="${ROOT_DIR}/build/pkg"
STAGE_DIR="${BUILD_DIR}/stage"
SCRIPTS_DIR="${BUILD_DIR}/scripts"
PKG_ID="com.techmore.tmdns"
VERSION="${TMDNS_VERSION:-1.0.$(date +%Y%m%d.%H%M)}"
PKG_PATH="${BUILD_DIR}/TM-DNS-${VERSION}.pkg"
PKG_SIGN_IDENTITY="${TMDNS_PKG_SIGN_IDENTITY:-}"
NOTARY_PROFILE="${TMDNS_NOTARY_PROFILE:-}"
COMMIT="$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ ! -d "${XCODE_DIR}/xcode-TM-DNS.xcodeproj" ]]; then
	echo "Missing Xcode project at ${XCODE_DIR}" >&2
	exit 1
fi

command -v go >/dev/null
command -v xcodebuild >/dev/null
command -v pkgbuild >/dev/null

rm -rf "${BUILD_DIR}"
mkdir -p \
	"${STAGE_DIR}/Applications" \
	"${STAGE_DIR}/Library/Application Support/TM-DNS" \
	"${STAGE_DIR}/Library/LaunchDaemons" \
	"${STAGE_DIR}/Library/Logs/TM-DNS" \
	"${SCRIPTS_DIR}"

echo "Building TM-DNS daemon..."
(cd "${ROOT_DIR}" && go build -trimpath -ldflags="-s -w -X github.com/techmore/tm-dns/internal/version.Version=${VERSION} -X github.com/techmore/tm-dns/internal/version.Commit=${COMMIT} -X github.com/techmore/tm-dns/internal/version.BuildTime=${BUILD_TIME}" -o "${STAGE_DIR}/Library/Application Support/TM-DNS/tmdns" ./cmd/tmdns)

echo "Building TM-DNS.app..."
xcode_args=(
	-project "${XCODE_DIR}/xcode-TM-DNS.xcodeproj"
	-scheme xcode-TM-DNS
	-configuration Release
	-derivedDataPath "${BUILD_DIR}/DerivedData"
	CODE_SIGNING_ALLOWED="${CODE_SIGNING_ALLOWED:-NO}"
	CURRENT_PROJECT_VERSION="${VERSION//./}"
)
if [[ -n "${TMDNS_APP_SIGN_IDENTITY:-}" ]]; then
	xcode_args+=(
		CODE_SIGN_STYLE=Manual
		CODE_SIGN_IDENTITY="${TMDNS_APP_SIGN_IDENTITY}"
		DEVELOPMENT_TEAM="${TMDNS_DEVELOPMENT_TEAM:-G8D238C7EJ}"
	)
fi
xcode_args+=(build)
xcodebuild "${xcode_args[@]}"

APP_PATH="$(find "${BUILD_DIR}/DerivedData/Build/Products/Release" -maxdepth 1 -name 'TM-DNS.app' -type d | head -n 1)"
if [[ -z "${APP_PATH}" ]]; then
	echo "TM-DNS.app was not produced" >&2
	exit 1
fi
ditto "${APP_PATH}" "${STAGE_DIR}/Applications/TM-DNS.app"

cp "${ROOT_DIR}/packaging/launchd/com.techmore.tmdns.daemon.plist" "${STAGE_DIR}/Library/LaunchDaemons/com.techmore.tmdns.daemon.plist"
cp "${ROOT_DIR}/packaging/scripts/preinstall" "${SCRIPTS_DIR}/preinstall"
cp "${ROOT_DIR}/packaging/scripts/postinstall" "${SCRIPTS_DIR}/postinstall"
chmod 755 "${SCRIPTS_DIR}/preinstall" "${SCRIPTS_DIR}/postinstall"

chmod 755 "${STAGE_DIR}/Library/Application Support/TM-DNS/tmdns"
chmod 644 "${STAGE_DIR}/Library/LaunchDaemons/com.techmore.tmdns.daemon.plist"
find "${STAGE_DIR}" -name '.DS_Store' -o -name '._*' | xargs -r rm -rf
xattr -cr "${STAGE_DIR}" 2>/dev/null || true

echo "Building installer package..."
pkg_args=(
	--root "${STAGE_DIR}"
	--scripts "${SCRIPTS_DIR}"
	--identifier "${PKG_ID}"
	--version "${VERSION}"
	--install-location "/"
)
if [[ -n "${PKG_SIGN_IDENTITY}" ]]; then
	pkg_args+=(--sign "${PKG_SIGN_IDENTITY}")
fi
pkgbuild "${pkg_args[@]}" "${PKG_PATH}"

if [[ -n "${NOTARY_PROFILE}" ]]; then
	echo "Submitting package for notarization..."
	xcrun notarytool submit "${PKG_PATH}" --keychain-profile "${NOTARY_PROFILE}" --wait
	xcrun stapler staple "${PKG_PATH}"
fi

echo "Built ${PKG_PATH}"
