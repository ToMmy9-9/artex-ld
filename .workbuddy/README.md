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
