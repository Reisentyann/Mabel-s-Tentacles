# 构建阶段（构建上下文 = 仓库根：同级 describer-go/indexer-go/manager-go/common 模块必须一起进镜像）
FROM golang:1.26-alpine AS builder
# 国内默认 Go 模块代理 proxy.golang.org 被墙，改用 goproxy.cn（含 sumdb），失败回退直连
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src/mcp-server-go
COPY mcp-server-go/go.mod mcp-server-go/go.sum ./
COPY common/go.mod /src/common/go.mod
COPY describer-go/go.mod describer-go/go.sum /src/describer-go/
COPY indexer-go/go.mod /src/indexer-go/go.mod
COPY manager-go/go.mod manager-go/go.sum /src/manager-go/
RUN go mod download
COPY mcp-server-go/ ./
COPY common/ /src/common/
COPY describer-go/ /src/describer-go/
COPY indexer-go/ /src/indexer-go/
COPY manager-go/ /src/manager-go/
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# 运行阶段
FROM alpine:latest
# 国内 alpine 源加速（阿里云镜像）
RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.aliyun.com/alpine#g' /etc/apk/repositories \
    && apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/server /app/server
EXPOSE 8080
ENTRYPOINT ["/app/server"]
