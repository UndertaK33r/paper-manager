#!/usr/bin/env bash
# 一键安装：自动下载最新版并去除隔离标记，绕开 macOS Gatekeeper
# 用法：curl -fsSL https://github.com/UndertaK33r/paper-manager/releases/latest/download/install.sh | bash
set -e
REPO="UndertaK33r/paper-manager"

os=$(uname -s); arch=$(uname -m)
case "$os" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo "仅支持 macOS / Linux"; exit 1 ;; esac
case "$arch" in arm64|aarch64) arch=arm64 ;; x86_64) arch=amd64 ;; *) echo "不支持的架构: $arch"; exit 1 ;; esac

echo "获取最新版本号..."
tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
[ -n "$tag" ] || { echo "获取版本失败，请稍后重试"; exit 1; }

name="paper-manager-${os}-${arch}"
url="https://github.com/$REPO/releases/download/$tag/${name}.zip"
echo "下载 $name ($tag)..."
curl -fsSL -o /tmp/pm-install.zip "$url"

rm -rf /tmp/pm-install && mkdir -p /tmp/pm-install
unzip -q -o /tmp/pm-install.zip -d /tmp/pm-install
bin="/tmp/pm-install/$name"
[ -f "$bin" ] || { echo "解压异常："; ls /tmp/pm-install; exit 1; }

chmod +x "$bin"
command -v xattr >/dev/null 2>&1 && xattr -cr "$bin" 2>/dev/null || true

mkdir -p "$HOME/paper-manager"
cp "$bin" "$HOME/paper-manager/paper-manager"
xattr -cr "$HOME/paper-manager/paper-manager" 2>/dev/null || true

echo ""
echo "✅ 已安装到 ~/paper-manager/paper-manager"
echo ""
echo "启动方式："
echo "  cd ~/paper-manager && ./paper-manager"
echo ""
echo "（数据会保存在 ~/paper-manager/data，备份拷走该文件夹即可）"
