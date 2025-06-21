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

REPO="FMotalleb/junction"
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
pushd "$TMPDIR" >/dev/null
echo -e "${BLUE}Using temporary directory: $TMPDIR${NC}"

echo -e "${CYAN}Downloading files...${NC}"
curl --fail --show-headers -LO "$ASSET_URL"
curl --fail --show-headers -LO "$CHECKSUM_URL"

echo -e "${CYAN}Verifying checksum...${NC}"
EXPECTED_SUM=$(grep "$TARBALL" "$CHECKSUM_FILE" | awk '{print $1}')

if command -v sha256sum &>/dev/null; then
  ACTUAL_SUM=$(sha256sum "$TARBALL" | awk '{print $1}')
elif command -v shasum &>/dev/null; then
  ACTUAL_SUM=$(shasum -a 256 "$TARBALL" | awk '{print $1}')
else
  echo -e "${RED}No checksum utility found (sha256sum or shasum)${NC}" >&2
  input -p "Press Enter to continue without checksum verification (not recommended)..." || true
  ACTUAL_SUM="${EXPECTED_SUM}"
fi

if [[ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]]; then
  echo -e "${RED}Checksum verification failed!${NC}" >&2
  exit 1
fi
echo -e "${GREEN}Checksum OK. Extracting...${NC}"
tar -xzf "$TARBALL"

echo -e "${CYAN}Moving binary to current working directory...${NC}"
mv junction "$WORKING_DIR/junction"
popd >/dev/null

chmod +x "$WORKING_DIR/junction"

echo -e "${GREEN}Installation complete.${NC}"
echo -e "${BOLD}Run: ${NC}${YELLOW}./junction --help${NC}"
echo
echo -e "${BOLD}To see example config:${NC} ${YELLOW}./junction example${NC}"
echo -e "${BOLD}To install globally:${NC} ${YELLOW}sudo mv ./junction /usr/local/bin/junction${NC}"
echo
echo -e "${BLUE}Temporary files are in:${NC} ${TMPDIR}"
echo -e "${RED}Remember: Clean it up manually if needed: rm -rf \"$TMPDIR\"${NC} ${GREEN}# (not mandatory since it will be wiped after reboot)${NC}"
echo -e "${CYAN}Thank you for using Junction!${NC}"
