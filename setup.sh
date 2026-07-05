#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

say()  { echo -e "${GREEN}==>${NC} $1"; }
warn() { echo -e "${RED}==>${NC} $1"; }

echo -e "${CYAN}  H2APK dependency setup${NC}"
echo  "  ─────────────────────"
echo

# ── OS detection ──────────────────────────────────────────────
OS="unknown"
if [ -d /data/data/com.termux/files/usr ]; then
  OS="termux"
elif [ -f /etc/os-release ]; then
  . /etc/os-release
  case "$ID" in
    ubuntu|debian) OS="debian" ;;
    arch|manjaro)   OS="arch" ;;
    fedora)         OS="fedora" ;;
  esac
fi
say "Detected: $OS"
echo

# ── Install system packages ──────────────────────────────────
case "$OS" in
  termux)
    say "Updating package lists..."
    pkg update -y
    say "Installing packages..."
    pkg install -y openjdk-17 aapt2 aapt zip wget golang 2>/dev/null || pkg install -y openjdk-17 aapt2 aapt zip wget golang
    JAVA_HOME=""
    ANDROID_PLATFORMS="/data/data/com.termux/files/usr/opt/android-sdk/platforms"
    ;;
  debian)
    say "Installing packages..."
    sudo apt update -qq
    sudo apt install -y openjdk-17-jdk-headless zip wget unzip
    # aapt2/zipalign: try apt, then download
    if ! command -v aapt2 >/dev/null 2>&1; then
      say "Installing Android build-tools..."
      sudo apt install -y android-sdk-build-tools 2>/dev/null || true
    fi
    ANDROID_PLATFORMS="$HOME/Android/Sdk/platforms"
    ;;
  arch)
    say "Installing packages..."
    sudo pacman -S --noconfirm jdk17-openjdk zip wget unzip
    if ! command -v aapt2 >/dev/null 2>&1; then
      sudo pacman -S --noconfirm android-tools 2>/dev/null || true
    fi
    ANDROID_PLATFORMS="$HOME/Android/Sdk/platforms"
    ;;
  fedora)
    say "Installing packages..."
    sudo dnf install -y java-17-openjdk-devel zip wget unzip
    ANDROID_PLATFORMS="$HOME/Android/Sdk/platforms"
    ;;
  *)
    warn "Unknown OS. Install these manually: openjdk-17, aapt2, zipalign, zip"
    echo "Then place android.jar at tools/android.jar or set ANDROID_HOME"
    exit 0
    ;;
esac

# ── android.jar ──────────────────────────────────────────────
TOOLS_JAR="$PWD/tools/android.jar"
SDK_JAR=""

if [ -f "$TOOLS_JAR" ]; then
  say "android.jar already present at tools/android.jar"
elif [ -n "$ANDROID_HOME" ]; then
  SDK_JAR=$(find "$ANDROID_HOME/platforms" -name "android.jar" 2>/dev/null | head -1)
fi

if [ -z "$SDK_JAR" ] && [ -d "$ANDROID_PLATFORMS" ]; then
  SDK_JAR=$(find "$ANDROID_PLATFORMS" -name "android.jar" 2>/dev/null | head -1)
fi

if [ -n "$SDK_JAR" ]; then
  say "Found android.jar at $SDK_JAR"
elif [ ! -f "$TOOLS_JAR" ]; then
  say "Downloading android.jar (platform 34)..."
  DL_URLS="
    https://github.com/HashShin/H2APK/releases/download/android-34/android.jar
    https://dl.google.com/android/repository/platform-34-ext7_r03.zip
  "
  mkdir -p "$PWD/tools"
  DL_OK=false
  for DL_URL in $DL_URLS; do
    say "Trying: $DL_URL"
    for i in 1 2 3; do
      if command -v wget >/dev/null 2>&1; then
        wget -q --show-progress -O "$TOOLS_JAR" "$DL_URL" && DL_OK=true
      elif command -v curl >/dev/null 2>&1; then
        curl -L -o "$TOOLS_JAR" "$DL_URL" && DL_OK=true
      fi
      $DL_OK && break
      say "Retry $i/3..."
      sleep 3
    done
    # If downloaded a zip from Google, extract android.jar
    if $DL_OK && echo "$DL_URL" | grep -q '\.zip$'; then
      UNZIP_DIR=$(mktemp -d)
      unzip -qo "$TOOLS_JAR" -d "$UNZIP_DIR"
      JAR_PATH=$(find "$UNZIP_DIR" -name "android.jar" 2>/dev/null | head -1)
      if [ -n "$JAR_PATH" ]; then
        mv "$JAR_PATH" "$TOOLS_JAR"
        rm -rf "$UNZIP_DIR"
      else
        warn "android.jar not found in extracted zip"
        rm -f "$TOOLS_JAR"; rm -rf "$UNZIP_DIR"
        DL_OK=false
      fi
    fi
    $DL_OK && break
  done
  if ! $DL_OK; then
    warn "Failed to download android.jar — network / DNS unreachable"
    say "Manual fix: place android.jar in $(pwd)/tools/android.jar"
    rm -f "$TOOLS_JAR"
    exit 1
  fi
  say "Downloaded android.jar to tools/android.jar"
fi

# ── Verify ───────────────────────────────────────────────────
echo
say "Verifying..."
ALL_OK=true

for cmd in javac java zip wget go; do
  if command -v $cmd >/dev/null 2>&1; then
    echo "  ✓ $cmd  ($(command -v $cmd))"
  else
    echo "  ✗ $cmd  NOT FOUND"
    ALL_OK=false
  fi
done

# aapt2/zipalign: might be named differently or not in PATH
for cmd in aapt2 zipalign; do
  if command -v $cmd >/dev/null 2>&1; then
    echo "  ✓ $cmd  ($(command -v $cmd))"
  else
    # try common Termux paths
    found=$(find /data/data/com.termux/files/usr/bin -name "$cmd" 2>/dev/null | head -1)
    if [ -n "$found" ]; then
      echo "  ✓ $cmd  ($found)"
    else
      echo "  ✗ $cmd  NOT FOUND"
      ALL_OK=false
    fi
  fi
done

AJAR="$PWD/tools/android.jar"
[ ! -f "$AJAR" ] && AJAR="$SDK_JAR"
if [ -n "$AJAR" ] && [ -f "$AJAR" ]; then
  echo "  ✓ android.jar  ($AJAR)"
else
  echo "  ✗ android.jar  NOT FOUND"
  ALL_OK=false
fi

echo
if $ALL_OK; then
  echo -e "  ${GREEN}All dependencies ready. Run: go run main.go${NC}"
else
  warn "Some dependencies are missing. Review output above."
fi
