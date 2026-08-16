#!/usr/bin/env python3
"""
计算网易我的世界 G79 (Android) 客户端 patchResourcesHash。
用法: python3 calc_patch_hash.py [版本号] [urlNew]
      python3 calc_patch_hash.py 3.9.23.298289 https://g79-102.gph.netease.com
"""
import sys
import io
import json
import hashlib
import zipfile
import urllib.request


def download(url: str) -> bytes:
    print(f"[下载] {url}")
    req = urllib.request.Request(url, headers={
        "User-Agent": "Dalvik/2.1.0 (Linux; U; Android 13)",
        "Accept": "*/*",
    })
    with urllib.request.urlopen(req, timeout=30) as resp:
        data = resp.read()
    print(f"       ↳ {len(data)} 字节, 状态 OK")
    return data


def read_first_zip_entry(raw: bytes) -> bytes:
    zf = zipfile.ZipFile(io.BytesIO(raw))
    name = zf.namelist()[0]
    print(f"[ZIP] 首个文件: {name}")
    with zf.open(name) as f:
        return f.read()


def md5_bytes(data: bytes) -> str:
    return hashlib.md5(data).hexdigest()


def calc_patch_hash(version: str, base_url: str) -> dict:
    base = base_url.rstrip("/")
    pre = f"{base}/android_{version}/{version}/android"

    manifest_url = f"{pre}/manifest.zip"
    manifest_raw = download(manifest_url)
    manifest_json = read_first_zip_entry(manifest_raw)
    manifest = json.loads(manifest_json)
    print(f"[manifest] assets 条目数: {len(manifest.get('assets', {}))}")

    assets = manifest.get("assets", {})
    vanilla_mcp_md5 = assets.get("vanilla.mcp", {}).get("md5", "")
    vanilla_patch_mcp_md5 = assets.get("vanilla_patch.mcp", {}).get("md5", "")
    print(f"  vanilla.mcp       md5 = {vanilla_mcp_md5}")
    print(f"  vanilla_patch.mcp md5 = {vanilla_patch_mcp_md5}")

    hash_base = vanilla_mcp_md5 + vanilla_patch_mcp_md5

    rn_url = f"{pre}/rn/index.bundle.backup"
    rn_hash = ""
    try:
        rn_raw = download(rn_url)
        try:
            rn_payload = read_first_zip_entry(rn_raw)
            print(f"       ↳ 检测到 zip 包装, 解压后 {len(rn_payload)} 字节")
        except Exception:
            rn_payload = rn_raw
        rn_hash = md5_bytes(rn_payload)
        print(f"  rn/index.bundle.backup md5 = {rn_hash}")
    except Exception as e:
        print(f"  rn bundle 获取失败 (非致命, 继续): {e}")

    concat = hash_base + rn_hash
    print(f"\n[拼接] hash_base + rn_hash = {concat}")
    final = md5_bytes(concat.encode("utf-8"))
    return {
        "patch_version": version,
        "vanilla_mcp_md5": vanilla_mcp_md5,
        "vanilla_patch_mcp_md5": vanilla_patch_mcp_md5,
        "rn_bundle_md5": rn_hash,
        "patch_resources_hash": final,
    }


def main():
    version = sys.argv[1] if len(sys.argv) > 1 else "3.9.23.298289"
    url_new = sys.argv[2] if len(sys.argv) > 2 else "https://g79-102.gph.netease.com"
    print(f"=" * 60)
    print(f"引擎版本 (EngineVersion): 3.9.23.298289")
    print(f"补丁版本 (patchVersion):  {version}")
    print(f"Patch 服务器 (urlNew):    {url_new}")
    print(f"=" * 60)
    result = calc_patch_hash(version, url_new)
    print(f"\n" + "=" * 60)
    print("结果汇总（复制到 globals.go / PatchMetadata 中）:")
    print("=" * 60)
    for k, v in result.items():
        print(f"  {k:30s} = {v}")
    print()
    print(f"PatchMetadata{{")
    print(f'    Version:       "{result["patch_version"]}",')
    print(f'    ResourcesHash: "{result["patch_resources_hash"]}",')
    print(f"}}")


if __name__ == "__main__":
    main()
