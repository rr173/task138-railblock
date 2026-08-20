# task138-railblock — Benzhi 评测说明

## 项目解决的业务问题

本项目实现一个**铁路区间闭塞与信号联锁进路计算引擎**：在计算机联锁（CBI）系统离线计算与一致性校验场景下，给定一个站场图（道岔/信号机/轨道区段/股道），引擎定义列车与调车进路、预计算敌对进路、排列进路（自动转辙道岔、锁闭道岔与区段、开放信号）、模拟列车运行（占用/出清区段触发接近锁闭与三点检查分段解锁），并提供人工取消（接近锁闭延时解锁）、故障解锁，以及进程重启后从事件流重建全部联锁状态的恢复能力。主要输入是站场图元素与进路定义、列车占用/出清事件；主要输出是进路状态机迁移、道岔/区段锁闭状态、信号显示（红/黄/双黄/绿）与重启一致性报告。

## 标准本地命令

```bash
go build ./...          # 编译
go run . --addr=:8080 --db=railblock.db   # 启动 HTTP 服务（前端在 http://localhost:8080/）
go test ./...           # 运行测试（含 selfcheck 全场景）
go run . --smoke-test    # 运行自检（覆盖站场图/进路/敌对/排列/接近锁闭/三点解锁/信号/故障解锁/重启恢复/前端），执行后退出
go run . --migrate-only  # 仅建库表后退出
```

环境变量 `RAILBLOCK_ADMIN_TOKEN` 保护 admin 路由（默认 `railblock-secret`）。

## 构建脚本参数

`build_benzhi_docker.sh <镜像名> <平台>`：
- 参数 1：镜像名（默认 `my-project`）
- 参数 2：Docker 平台（默认 `linux/amd64`，也可 `linux/arm64`）

## 双架构构建命令

```bash
# amd64
bash ./build_benzhi_docker.sh go-task-benzhi:amd64 linux/amd64
docker run --rm go-task-benzhi:amd64 go version

# arm64
bash ./build_benzhi_docker.sh go-task-benzhi:arm64 linux/arm64
docker run --rm go-task-benzhi:arm64 go version
```

构建后用 `docker run -it <镜像名>` 进入容器。

## 前端

原生 HTML/CSS/JavaScript（无构建、无 Node、无 npm）。源码在 `internal/webfs/web/`（index.html / app.js / style.css），经 `//go:embed web` 打入 Go 二进制。服务启动后页面在 `http://localhost:8080/`，覆盖：建车站 → 建区段/道岔/信号机/股道 → 定义进路 → 排列进路 → 列车占用/出清 → 看站场图状态 的真实读写流程。

## 页面 + API 联动 smoke-test

```bash
go run . --smoke-test
```

`scenarioFrontend` 实际请求 `GET /` 页面（断言返回 200 且引用 app.js），并调用页面使用的业务 API（`POST /stations` → `GET /stations/{id}`），断言页面与 API 联动成功。
