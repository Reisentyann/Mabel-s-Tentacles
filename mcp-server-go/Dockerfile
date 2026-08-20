# 构建阶段
FROM golang:1.26-alpine AS builder
# 国内默认 Go 模块代理 proxy.golang.org 被墙，改用 goproxy.cn（含 sumdb），失败回退直连
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
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
