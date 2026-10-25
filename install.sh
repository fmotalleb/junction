#!/usr/bin/env bash
set -euo pipefail

# ANSI colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color (RESET)

REPO="fmotalleb/junction"
API="https://api.github.com/repos/${REPO}/releases/latest"
WORKING_DIR="$(pwd)"

# Detect OS and ARCH
OS="$(uname | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
x86_64) ARCH="amd64" ;;
*)
  echo -e "${RED}Unsupported architecture: $ARCH${NC}" >&2
  exit 1
  ;;
esac

function fetch_version() {
  echo -e "${CYAN}Fetching latest release metadata...${NC}" >&2
  version="$(curl -fs "$API" | jq -r '.tag_name')"
  echo -e "${GREEN}Latest version: ${version}${NC}" >&2
  echo -n "${version}"
}

VERSION="${VERSION:-$(fetch_version)}"
VERSION="${VERSION#v}"
TARBALL="junction_${VERSION}_${OS}_${ARCH}.tar.gz"
CHECKSUM_FILE="junction_${VERSION}_checksums.txt"
echo -e "${BLUE}Using version: ${VERSION}${NC}\n"

BASE_ASSET_PATH="https://github.com/$REPO/releases/download/v${VERSION}"
ASSET_URL="${BASE_ASSET_PATH}/${TARBALL}"
CHECKSUM_URL="${BASE_ASSET_PATH}/${CHECKSUM_FILE}"

TMPDIR=$(mktemp -d)
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT
pushd "$TMPDIR" >/dev/null
echo -e "${BLUE}Using temporary directory: $TMPDIR${NC}"

echo -e "${CYAN}Downloading files...${NC}"
curl --fail --location --connect-timeout 10 --max-time 120 --output "$TARBALL" "$ASSET_URL"
curl --fail --location --connect-timeout 10 --max-time 120 --output "$CHECKSUM_FILE" "$CHECKSUM_URL"

echo -e "${CYAN}Verifying checksum...${NC}"
EXPECTED_LINES=$(awk -v file="$TARBALL" '$2 == file { print $1 }' "$CHECKSUM_FILE")
if [[ "$(printf '%s\n' "$EXPECTED_LINES" | sed '/^$/d' | wc -l | tr -d ' ')" != 1 ]] || [[ ! "$EXPECTED_LINES" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo -e "${RED}No unique, valid SHA-256 entry for $TARBALL${NC}" >&2
  exit 1
fi
EXPECTED_SUM="$(printf '%s' "$EXPECTED_LINES" | tr '[:upper:]' '[:lower:]')"

if command -v sha256sum &>/dev/null; then
  ACTUAL_SUM=$(sha256sum "$TARBALL" | awk '{print $1}')
elif command -v shasum &>/dev/null; then
  ACTUAL_SUM=$(shasum -a 256 "$TARBALL" | awk '{print $1}')
else
  echo -e "${RED}No checksum utility found (sha256sum or shasum)${NC}" >&2
  exit 1
fi
ACTUAL_SUM="$(printf '%s' "$ACTUAL_SUM" | tr '[:upper:]' '[:lower:]')"

if [[ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]]; then
  echo -e "${RED}Checksum verification failed!${NC}" >&2
  exit 1
fi
echo -e "${GREEN}Checksum OK. Extracting...${NC}"
if tar -tzf "$TARBALL" | awk 'BEGIN { bad=0; binary=0 } /^\// || /(^|\/)\.\.($|\/)/ { bad=1 } $0 == "junction" { binary++ } END { exit bad || binary != 1 }' \
  && tar -tvzf "$TARBALL" | awk 'substr($0, 1, 1) != "-" { bad=1 } END { exit bad }'; then
  tar -xzf "$TARBALL" --no-same-owner --no-same-permissions
else
  echo -e "${RED}Archive contains an unsafe path${NC}" >&2
  exit 1
fi
if [[ ! -f junction || -L junction ]]; then
  echo -e "${RED}Archive does not contain a regular junction binary${NC}" >&2
  exit 1
fi

echo -e "${CYAN}Moving binary to current working directory...${NC}"
chmod +x junction
install_tmp="$(mktemp "$WORKING_DIR/.junction.XXXXXX")"
cp junction "$install_tmp"
chmod +x "$install_tmp"
mv -f "$install_tmp" "$WORKING_DIR/junction"
popd >/dev/null

echo -e "${GREEN}Installation complete.${NC}"
echo -e "${BOLD}Run: ${NC}${YELLOW}./junction --help${NC}"
echo
echo -e "${BOLD}To see example config:${NC} ${YELLOW}./junction example${NC}"
echo -e "${BOLD}To install globally:${NC} ${YELLOW}sudo mv ./junction /usr/local/bin/junction${NC}"
echo
echo -e "${BLUE}Temporary files were removed.${NC}"
echo -e "${CYAN}Thank you for using Junction!${NC}"
