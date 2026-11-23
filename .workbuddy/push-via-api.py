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
import json
import os
import subprocess
import sys
import urllib.request

API = "https://api.github.com"


def git(*args, binary=False):
    out = subprocess.run(["git", *args], capture_output=True)
    if out.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} 失败:\n{out.stderr.decode('utf-8', 'replace')}")
    return out.stdout if binary else out.stdout.decode("utf-8")


def api(path, token, method="GET", payload=None):
    url = path if path.startswith("http") else API + path
    data = json.dumps(payload).encode() if payload is not None else None
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
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", "replace")
        raise SystemExit(f"HTTP {e.code} 于 {method} {path}\n{detail[:800]}")
    return json.loads(body) if body else None


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
    base_tree = api(f"/repos/{args.repo}/git/commits/{remote_sha}", args.token)["tree"]["sha"]

    # 本地待推送的提交（相对远端）
    commits = git("log", "--format=%H", f"{remote_sha}..HEAD").split()
    if not commits:
        print("没有待推送的提交")
        return 0
    head = commits[-1]
    message = git("log", "-1", "--format=%B", head).rstrip("\n")
    print(f"待推送提交: {head}")
    print(f"远端 ref  : {remote_sha}")

    # 待推送的提交（git log 是倒序：最新在前），要按时间正序逐个重建
    commits = list(reversed(git("log", "--format=%H", f"{remote_sha}..HEAD").split()))
    if not commits:
        print("没有待推送的提交")
        return 0
    print(f"待推送提交: {len(commits)} 个，最新 {commits[-1][:8]}")

    tree_sha = base_tree
    parent = remote_sha
    last_sha = remote_sha

    for sha in commits:
        msg = git("log", "-1", "--format=%B", sha).rstrip("\n")
        files = [f for f in git("diff-tree", "--no-commit-id", "--name-only", "-r", sha).split("\n") if f.strip()]
        entries = []
        for path in files:
            # 用 ls-tree 判断文件是否存在（新增/修改），不存在就是删除 → sha=None
            ls = git("ls-tree", sha, "--", path).strip()
            if ls:
                content = git("cat-file", "-p", f"{sha}:{path}", binary=True)
                blob = api(
                    f"/repos/{args.repo}/git/blobs",
                    args.token,
                    method="POST",
                    payload={
                        "content": base64.b64encode(content).decode(),
                        "encoding": "base64",
                    },
                )
                mode = "100755" if ls.startswith("100755") else "100644"
                entries.append({"path": path, "mode": mode, "type": "blob", "sha": blob["sha"]})
            else:
                entries.append({"path": path, "mode": "100644", "type": "blob", "sha": None})
        # base_tree 用远端原始 tree，不是上一轮累加的 tree_sha ——
        # 用 tree_sha 当 base 会报 GitRPC::BadObjectState。
        new_tree = api(
            f"/repos/{args.repo}/git/trees",
            args.token,
            method="POST",
            payload={"base_tree": base_tree, "tree": entries},
        )["sha"]
        new_commit = api(
            f"/repos/{args.repo}/git/commits",
            args.token,
            method="POST",
            payload={
                "message": msg,
                "tree": new_tree,
                "parents": [parent],
            },
        )["sha"]
        print(f"  创建 commit {new_commit[:8]}（{len(entries)} 个文件）: {msg.splitlines()[0][:50]}")
        tree_sha = new_tree
        parent = new_commit
        last_sha = new_commit

    # 推进 ref（用 force=false，防覆盖别人的提交）
    api(
        f"/repos/{args.repo}/git/refs/{ref_path}",
        args.token,
        method="PATCH",
        payload={"sha": last_sha, "force": False},
    )
    print(f"✓ ref 已推进到 {last_sha}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
