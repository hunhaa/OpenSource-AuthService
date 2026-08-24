#!/bin/bash
# 用 curl --preproxy 双层代理模式测试用户代理
# preproxy = 第一层（本地隧道），proxy = 第二层（用户代理）

PROXIES=(
  "150.139.247.191:17303"
  "171.109.111.206:53677"
  "171.214.18.60:53589"
  "117.68.5.80:12251"
  "117.68.1.241:42687"
  "117.68.5.239:28354"
  "117.68.1.241:20539"
  "222.216.120.176:33807"
  "182.247.255.69:43405"
  "150.139.247.172:11349"
)

USER="ydl84074816"
PASS="CiuEhvwj"
TUNNEL="http://127.0.0.1:18080"

echo "===== curl --preproxy 双层代理测试 ====="
echo "preproxy=$TUNNEL   user=$USER"
echo "目标: https://httpbin.org/ip (CONNECT 模式, 20s 超时)"
echo ""

PASS_COUNT=0
i=0
for p in "${PROXIES[@]}"; do
  i=$((i+1))
  printf "[%02d] %-30s  " "$i" "$p"

  # 方式A：明文代理认证（Proxy-Authorization: user:pass 直接值）
  # curl 没有直接给明文的选项，它默认用 Basic。我们得用 --proxy-header 手动塞
  IP_PORT="$p"

  # 先试标准 Basic 认证（curl 默认）
  OUT1=$(curl -s --max-time 20 \
    --preproxy "$TUNNEL" \
    -x "http://${USER}:${PASS}@${IP_PORT}" \
    https://httpbin.org/ip 2>&1)
  STATUS1="?"
  IP1=""
  if echo "$OUT1" | grep -q "origin"; then
    STATUS1="BASIC_OK"
    IP1=$(echo "$OUT1" | tr -d '\n ' | sed 's/.*origin":"\([0-9.]*\).*/\1/')
    PASS_COUNT=$((PASS_COUNT+1))
  else
    STATUS1="BASIC_FAIL"
  fi

  # 如果 Basic 失败，试明文认证（用 --proxy-header 覆盖）
  IP2=""
  STATUS2="skip"
  if [ "$STATUS1" = "BASIC_FAIL" ]; then
    OUT2=$(curl -s --max-time 20 \
      --preproxy "$TUNNEL" \
      -x "http://${IP_PORT}" \
      --proxy-header "Proxy-Authorization: ${USER}:${PASS}" \
      https://httpbin.org/ip 2>&1)
    if echo "$OUT2" | grep -q "origin"; then
      STATUS2="PLAIN_OK"
      IP2=$(echo "$OUT2" | tr -d '\n ' | sed 's/.*origin":"\([0-9.]*\).*/\1/')
      PASS_COUNT=$((PASS_COUNT+1))
    else
      STATUS2="PLAIN_FAIL"
    fi
  fi

  printf "%-10s %-18s | %-10s %-18s\n" "$STATUS1" "$IP1" "$STATUS2" "$IP2"
  if [ "$STATUS1" = "BASIC_FAIL" ] && [ "$STATUS2" = "PLAIN_FAIL" ]; then
    ERR=$(echo "$OUT1" | head -c 80)
    [ -n "$ERR" ] && echo "        BasicErr: $ERR"
    ERR2=$(echo "$OUT2" | head -c 80)
    [ -n "$ERR2" ] && echo "        PlainErr: $ERR2"
  fi
done

echo ""
echo "===== 汇总 ====="
echo "共 ${#PROXIES[@]} 个代理，可用 (Basic or Plain): $PASS_COUNT"

if [ "$PASS_COUNT" = "0" ]; then
  echo ""
  echo "⚠️  全部失败！可能原因:"
  echo "   1) 这些代理已经过期了（短效动态代理通常1-5分钟有效）"
  echo "   2) 用户名/密码不对（ydl84074816:CiuEhvwj）"
  echo "   3) 代理供应商那边要求不同的认证格式或绑定出口IP"
fi
