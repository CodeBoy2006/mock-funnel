# mock-funnel

一个**可调延迟与稳定性**的极简 mock 版 `funnel` 服务，用于在本地/测试环境下验证 **WeJH-Go** 的新负载均衡策略（权重 / 健康检查 / 熔断 / 恢复）。

> 设计遵循 KISS 原则：无外部依赖、单二进制、无鉴权（仅供开发环境使用）。

## 功能

- 四条内/外网 + 统一/正方 线路一体化模拟：
  - `outer-unified`、`inner-unified`、`outer-zf`、`inner-zf`
- 可配置：基础延迟、抖动、错误率、超时率、超时时长、夜间屏蔽窗口、启用/禁用
- 公开 Mock API（无鉴权）：
  - `/{line}/api/ping`、`/{line}/api/schedule`、`/{line}/api/grades`
- 实时可视化（内置 Web 面板）：
  - 最近 60 秒每秒 RPS 和平均延迟曲线
  - 按线路的计数（成功/错误/超时）与 P50/P95/P99（在 `/metrics/snapshot` 返回）
- HTTP 响应头包含 `X-Mock-Line`、`X-Mock-Latency-Ms`，便于在 WeJH-Go 中打点/排查。

## 编译与运行

```bash
# 进入项目目录
cd mock-funnel

# 构建
go build ./cmd/mock-funnel

# 运行（默认 :8080）
./mock-funnel
# 或更换端口
ADDR=:9090 ./mock-funnel
```

打开浏览器访问：`http://127.0.0.1:8080/` 查看控制面板。

## 与 WeJH-Go 对接建议

- 在 WeJH-Go 的上游配置中，将四条线路指向同一个 mock-funnel 实例，但使用不同的 path 前缀：

```
/outer-unified/api/...
/inner-unified/api/...
/outer-zf/api/...
/inner-zf/api/...
```

- 登录方式（`oauth` / `zf`）作为参数传给 mock 即可（mock 会忽略并返回固定结构），真正的决策逻辑仍在 WeJH-Go。
- 你可以通过调整面板参数来模拟：
  - 夜间关停（内网）
  - 高错误率/超时率
  - 高延迟/抖动
  - 动态启停线路

## API 说明

- `GET /{line}/api/ping`
- `GET /{line}/api/schedule`
- `GET /{line}/api/grades`

响应（示例）：

```json
{
  "ok": true,
  "line": "outer-unified",
  "resource": "schedule",
  "now": "2025-11-07T11:22:33Z",
  "latency_ms": 123,
  "data": { "message": "mock data", "resource": "schedule" }
}
```

失败示例：`nightly window blocked` / `simulated timeout` / `simulated upstream error`。

## 管理接口

- `GET  /admin/config` — 读取所有线路配置
- `POST /admin/reset` — 重置统计（不影响配置）
- `GET  /admin/line/{line}` — 读取单线路配置
- `POST /admin/line/{line}` — 更新单线路配置（JSON，与 `GET` 返回结构一致）

## 代码结构

```
cmd/mock-funnel/main.go    # 程序入口
internal/mockfunnel/       # 业务实现（配置、指标、路由、模拟）
  ├── config.go
  ├── metrics.go
  ├── server.go
  └── server_extra.go      # 额外的指标快照 handler
web/                        # 内置 Web 面板（无外部 CDN）
  ├── templates/index.html
  └── static/main.js
```

## 注意事项

- 本项目只用于**开发/联调**环境，不要直接暴露在公网。
- 如果你的客户端超时时间较短（例如 3s），可以把某条线路的 `timeout_ms` 调到很大（例如 20000ms）并设置一定的 `timeout_rate`，用于稳定复现**客户端超时**。
- 指标采用滑动窗口（60s）+ 有界环形缓冲实现的 P50/P95/P99，内存占用可控。