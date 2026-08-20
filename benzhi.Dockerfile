# benzhi 评测 Dockerfile
# 以 Go 镜像为基础；本项目为纯 Go（前端经 //go:embed 打入二进制，无需 Node）。
# 必须使用与本机一致、与 go.mod 一致的 Go 1.26.3，不得切换或安装其他版本。
FROM golang:1.26.3

WORKDIR /app

# 先复制依赖文件并下载依赖，利用 Docker 缓存并保证容器内可用
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 预编译一次，把编译缓存留在镜像里；不影响模型修改源码
RUN go build ./...

# 容器启动后进入 shell，方便操作
CMD ["bash"]
