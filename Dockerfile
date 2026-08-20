# syntax=docker/dockerfile:1

# builder: identical Go toolchain to the local machine
FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm AS builder
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn
RUN go build -o /out/railblock .

# runtime: minimal image
FROM docker.m.daocloud.io/library/alpine:3.20
WORKDIR /app
COPY --from=builder /out/railblock /railblock
EXPOSE 8080
ENTRYPOINT ["/railblock"]
CMD ["--addr", ":8080", "--db", "/app/railblock.db"]
