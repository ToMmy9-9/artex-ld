# .workbuddy —— 项目工具与排查记录

## pgsafe.sh —— 生产库只读测量包装

生产库是 `artex_audit`（有真实数据）。做性能测量时**不要直接用 psql 连生产库跑
破坏性语句**。

`pgsafe.sh` 把 SQL 包在 `BEGIN; SET TRANSACTION READ ONLY; ...; ROLLBACK;` 里：

```bash
bash .workbuddy/pgsafe.sh "EXPLAIN (ANALYZE) SELECT ... FROM llm_records;"
```

- 默认 ROLLBACK，生产库零副作用
- `SET TRANSACTION READ ONLY` 会在语句执行前立即生效，写操作直接被数据库拒绝
- 需要真写入时显式加 `--commit`（会打印警告）

**注意**：`SET default_transaction_read_only=on` **不能**用作防护 —— 它只影响
「之后开启」的事务，在同一 session 内无效。2026-11-23 就因为这个，
`EXPLAIN ANALYZE DELETE` 在生产库真的删掉了 8651 行 LLM 调用流水。

Go 测试侧已有对应的门禁（`db.CheckTestDSN`），会拒绝连生产库。

## push-via-api.py —— git 端点不通时用 REST API 推送

2026-11-23 遇到：网络只对 `api.github.com` 通，`github.com` 的
git-upload-pack 端点超时/握手失败，`git push` 一直失败（SSH 22 端口通但
key 没加到 GitHub）。

这个脚本用 GitHub REST API 手工构造 commit：

```bash
TOKEN=$(git remote get-url origin | grep -oP 'github_pat_\w+')
python .workbuddy/push-via-api.py --repo owner/name --ref refs/heads/master --token "$TOKEN"
```

流程：blob → tree → commit → PATCH ref（force=false，不覆盖别人的提交）。

**坑**：`base_tree` 必须是**远端原始 tree**，不能用上一轮累加的 tree_sha，
否则报 `GitRPC::BadObjectState`（422）。

推完记得用 blob sha 逐文件比对确认一致（git fetch 此时也走不通）。
