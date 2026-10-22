FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=https://goproxy.io,https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct

# 指定要打进镜像的配置文件, 部署脚本通过 --build-arg 传入
ARG CONFIG_FILE=etc/config_minute.yaml

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN if [ "${CONFIG_FILE}" != "etc/config.yaml" ]; then cp "${CONFIG_FILE}" etc/config.yaml; fi \
    && go build -o order_check .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/order_check .
COPY --from=builder /app/etc ./etc
RUN apk add --no-cache tzdata
ENV TZ=Asia/Shanghai
CMD ["./order_check", "-f", "./etc/config.yaml"]
