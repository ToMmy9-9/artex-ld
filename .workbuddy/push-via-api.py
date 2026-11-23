#!/usr/bin/env python3
"""push-via-api.py —— github.com 的 git 端点不通时，用 GitHub API 推送提交。

背景：2026-11-23 网络只对 api.github.com 通，github.com 的 git-upload-pack
端点超时，git push 一直失败。api.github.com 是可用的，于是走 REST API 手工
构造 commit：

  1. 对每个变更文件建 blob（base64 内容）
  2. 建 tree（基于远端 ref 的 tree）
  3. 建 commit（parent = 远端 ref）
  4. 更新 ref

用法：
  python push-via-api.py --repo owner/name --ref refs/heads/master --token <PAT>

安全性：只创建新 commit 并推进 ref，不删远端任何东西。
"""
import argparse
import base64
import http.client
import json
import os
import subprocess
import sys
import time
import urllib.request

API = "https://api.github.com"


def cat_file(rev, path):
    """读取 rev:path 的内容。路径含中文时 git 会加引号，所以走 --batch 协议。"""
    proc = subprocess.run(
        ["git", "cat-file", "--batch"],
        input=f"{rev}:{path}\n".encode(),
        capture_output=True,
    )
    data = proc.stdout
    nl = data.index(b"\n")
    header = data[:nl].decode("utf-8", "replace")
    parts = header.split()
    if len(parts) != 3 or parts[1] != "blob":
        raise SystemExit(f"cat-file {path} 失败: {header[:120]}")
    size = int(parts[2])
    return data[nl + 1 : nl + 1 + size]


def git(*args, binary=False):
    out = subprocess.run(["git", *args], capture_output=True)
    if out.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} 失败:\n{out.stderr.decode('utf-8', 'replace')}")
    return out.stdout if binary else out.stdout.decode("utf-8")


def api(path, token, method="GET", payload=None, retries=4):
    url = path if path.startswith("http") else API + path
    data = json.dumps(payload).encode() if payload is not None else None
    last = None
    for attempt in range(retries):
        req = urllib.request.Request(
            url,
            data=data,
            method=method,
            headers={
                "Authorization": f"Bearer {token}",
                "Accept": "application/vnd.github+json",
                "User-Agent": "workbuddy-push-script",
                "Content-Type": "application/json",
            },
        )
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                body = resp.read()
            return json.loads(body) if body else None
        except urllib.error.HTTPError as e:
            detail = e.read().decode("utf-8", "replace")
            # 5xx 与 429 是服务端/限流问题，重试有意义；4xx 是请求本身有问题，重试没用。
            if e.code in (429,) or 500 <= e.code < 600:
                last = f"HTTP {e.code}: {detail[:400]}"
                time.sleep(2 * (attempt + 1))
                continue
            raise SystemExit(f"HTTP {e.code} 于 {method} {path}\n{detail[:800]}")
        except (urllib.error.URLError, http.client.RemoteDisconnected, ConnectionError, TimeoutError) as e:
            # 网络抖动 —— 2026-11-24 推送时反复遇到，重试即可。
            last = f"{type(e).__name__}: {e}"
            time.sleep(2 * (attempt + 1))
    raise SystemExit(f"重试 {retries} 次仍失败于 {method} {path}\n{last}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", required=True)
    ap.add_argument("--ref", required=True)
    ap.add_argument("--token", required=True)
    args = ap.parse_args()

    # 远端当前 ref（git/ref/heads/master 这种格式；refs/heads/... 前缀要剥掉）
    ref_path = args.ref
    if ref_path.startswith("refs/"):
        ref_path = ref_path[len("refs/"):]
    ref_info = api(f"/repos/{args.repo}/git/ref/{ref_path}", args.token)
    remote_sha = ref_info["object"]["sha"]
    print(f"远端 ref  : {remote_sha}")

    # 待推送的提交：本地相对远端 SHA 的差异。
    # git fetch 在 git 端点不通时拿不到远端对象，所以不用 git diff-tree，
    # 而是把两边的「路径 → blob sha」映射各自取出来做差集。
    # 注意 -z：路径里有中文时 git 默认会转义成 "..." 形式，导致取不到文件。
    def tree_map(ref_or_sha, is_sha):
        m = {}
        raw = git("ls-tree", "-r", "-z", ref_or_sha).encode()
        for entry in raw.split(b"\0"):
            if not entry:
                continue
            meta, _, path = entry.partition(b"\t")
            parts = meta.split()
            if len(parts) >= 3:
                m[path.decode("utf-8")] = parts[2].decode()
        return m

    remote_commit = api(f"/repos/{args.repo}/git/commits/{remote_sha}", args.token)
    # 远端侧：用 API 递归列出 tree（git 端点不通，只能走 API）
    remote_files = {}
    # 递归 tree 时子 tree 的 entry.path 不含父目录前缀，必须自己拼 ——
    # 直接用会算出大量「删除」（本地路径带前缀，远端路径不带）。
    def walk_tree(tree_sha, prefix):
        t = api(f"/repos/{args.repo}/git/trees/{tree_sha}", args.token)
        for e in t.get("tree", []):
            full = prefix + e["path"]
            if e["type"] == "blob":
                remote_files[full] = e["sha"]
            elif e["type"] == "tree":
                walk_tree(e["sha"], full + "/")
        if t.get("truncated"):
            raise SystemExit("远端 tree 被截断，无法完整对比")

    walk_tree(remote_commit["tree"]["sha"], "")

    head = git("rev-parse", "HEAD").strip()
    local_files = tree_map(head, False)

    changed = [p for p in local_files if remote_files.get(p) != local_files[p]]
    deleted = [p for p in remote_files if p not in local_files]
    if not changed and not deleted:
        print("没有待推送的变更")
        return 0
    msg = git("log", "-1", "--format=%B", head).rstrip("\n")
    print(f"待推送: {len(changed)} 个变更, {len(deleted)} 个删除")
    print(f"本地 HEAD: {head[:8]}  远端: {remote_sha[:8]}")

    entries = []
    for path in changed:
        content = cat_file(head, path)
        blob = api(
            f"/repos/{args.repo}/git/blobs",
            args.token,
            method="POST",
            payload={"content": base64.b64encode(content).decode(), "encoding": "base64"},
        )
        ls = git("ls-tree", head, "--", path).strip()
        mode = "100755" if ls.startswith("100755") else "100644"
        entries.append({"path": path, "mode": mode, "type": "blob", "sha": blob["sha"]})
    for path in deleted:
        entries.append({"path": path, "mode": "100644", "type": "blob", "sha": None})

    new_tree = api(
        f"/repos/{args.repo}/git/trees",
        args.token,
        method="POST",
        payload={"base_tree": remote_commit["tree"]["sha"], "tree": entries},
    )["sha"]
    new_commit = api(
        f"/repos/{args.repo}/git/commits",
        args.token,
        method="POST",
        payload={"message": msg, "tree": new_tree, "parents": [remote_sha]},
    )["sha"]
    print(f"  创建 commit {new_commit[:8]}")

    # 推进 ref（用 force=false，防覆盖别人的提交）
    api(
        f"/repos/{args.repo}/git/refs/{ref_path}",
        args.token,
        method="PATCH",
        payload={"sha": new_commit, "force": False},
    )
    print(f"✓ ref 已推进到 {new_commit}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
