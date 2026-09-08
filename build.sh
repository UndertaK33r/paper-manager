#!/usr/bin/env bash
# 一键构建各平台免安装可执行文件，输出到 dist/
# 用法: ./build.sh   （需要 Go 1.24+，zip 命令）
set -e
cd "$(dirname "$0")"

mkdir -p dist
cp 使用说明.txt dist/ 2>/dev/null || echo "（未找到 使用说明.txt，跳过）"

for t in darwin/arm64 darwin/amd64 windows/amd64 linux/amd64; do
  os=${t%/*}; arch=${t#*/}
  out="dist/paper-manager-${os}-${arch}"
  [ "$os" = "windows" ] && out="${out}.exe"
  echo "构建 $out"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/server
done

cd dist
for f in paper-manager-*; do
  [ -f "$f" ] || continue
  zip -q "${f}.zip" "$f"
  [ -f 使用说明.txt ] && zip -q "${f}.zip" 使用说明.txt
done
echo "完成。dist/ 下即为可分发的压缩包。"
