#!/usr/bin/env bash
# FunAuth 一键启动脚本
# 用法：./start.sh [监听地址]
#   默认监听 :8090
set -euo pipefail

ADDR="${1:-${FUNAUTH_ADDR:-:8090}}"
BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$BIN_DIR/funauth-linux-amd64"

if [ ! -x "$BIN" ]; then
  echo "错误：找不到可执行文件 $BIN" >&2
  exit 1
fi

# 确保 ocr_resources 在二进制同目录（OCR 识别用）
if [ ! -d "$BIN_DIR/ocr_resources" ]; then
  echo "提示：未发现 ocr_resources 目录，OCR 相关功能可能不可用"
fi

echo "================================================="
echo "  FunAuth 用户中心  vPR00011"
echo "  监听地址: $ADDR"
echo "================================================="
echo
echo "首次启动：直接运行，浏览器访问 http://localhost${ADDR#:}/ 完成配置向导"
echo "配置完成后，重启本程序进入正常服务模式"
echo
exec "$BIN" serve --addr "$ADDR"
