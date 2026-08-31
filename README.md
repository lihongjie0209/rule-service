# rule-service

平台规则管理与执行服务。它拥有规则集、不可变规则版本和发布状态，通过 Google CEL 对调用方提交的 facts 做确定性求值；它不执行任意脚本，也不读取其他服务的数据库。

## 接口

前端接口全部使用 `POST + JSON` 和统一 `{code,message,body,request_id}` 响应：

- `/api/v1/rule-sets/create|update|get|list`
- `/api/v1/rule-versions/create|validate|publish|list`
- `/api/v1/rules/evaluate`

内部服务使用中央 `platform.rule.v1.RuleService` gRPC 契约，支持相同管理操作以及单次/批量 Evaluate。契约由 `platform-protos v0.15.0` 统一发布，本仓库不复制 Proto。

规则定义示例：

```json
{
  "rules": [
    {
      "name": "vip",
      "condition": "facts.tier == 'vip' && facts.amount >= 100",
      "result": {"discount": 20}
    }
  ],
  "default_result": {"discount": 0}
}
```

条件按顺序执行，首个匹配项返回；发布前会校验 JSON、CEL 返回类型、规则数量、表达式长度和求值成本。调用方必须显式提供 `tenant_id`，JWT 用户受到 tenant scope 约束，服务账号可通过配置化 PSK/JWT/mTLS 调用。

## 持久化与事件

- 默认 PostgreSQL 数据库 `platform`、Schema `rule_service`、迁移表 `rule_service_schema_migrations`。
- 同时维护 PostgreSQL、Kingbase 和 MySQL 迁移。
- 所有表包含版本号及完整审计字段；更新和发布使用乐观锁。
- 创建规则版本使用持久化幂等键。
- 发布事件与领域状态在同一事务写入 `rule_outbox_events`，随后通过公共 SDK 投递到 NATS JetStream `PLATFORM_EVENTS`，主题为 `platform.rule.rule-version.published.v1`。

## 开发与验证

```bash
make test-race
go vet ./...
make swagger-check
go test -tags=integration -run '^$' ./integration/...
```

本机不执行 Testcontainers。`make test-integration` 仅供 GitHub CI 使用，CI 会独立验证 PostgreSQL/MySQL 迁移、Repository/领域生命周期、Redis 和 HTTP/gRPC 鉴权。服务测试不要求其他平台服务在线。

启动和构建：

```bash
make build
./bin/api -version
go run ./cmd/api -config config/config.yaml -env development
```

构建通过 ldflags 注入版本、Git commit 和构建时间。生产配置要求 Identity JWKS、服务 audience、数据库、Redis、TLS 与 Secret 均由部署平台注入，并在启动前自动执行本服务迁移。
