# Minimal image: 直接把 build/linux_amd64/<SERVICE> 拷进 alpine,以二进制本身为
# ENTRYPOINT。镜像里**不放任何配置**,Apollo 连接参数和 APP_ENV 全部走运行时 env 注入。
#
# Build context 应为 build/linux_amd64/(由 scripts/docker-build.sh 设置),
# 所以 COPY 路径写相对名,不写 build/linux_amd64/<svc>。
#
# Build:
#   docker build -f docker/Dockerfile --build-arg SERVICE=server \
#       -t tacacs/server:<tag> build/linux_amd64
#
# Run (必填 env: APOLLO_APP_ID / APOLLO_IP / APOLLO_SECRET;APP_ENV 默认 test):
#   docker run -d --name tacacs-server \
#       -e APOLLO_APP_ID=xxx -e APOLLO_IP=http://apollo.example.com:8080 \
#       -e APOLLO_SECRET=yyy -e APP_ENV=prod \
#       -p 8080:8080 tacacs/server:<tag>
#
# 可选 env: APOLLO_CLUSTER (default), APOLLO_NAMESPACE (application)
#
# ca-certificates: Apollo / 飞书 HTTPS 调用要校验证书链
# tzdata: 日志时间戳,Asia/Shanghai 之类靠它解析

FROM alpine:3.20

ARG SERVICE
RUN test -n "$SERVICE" || (echo "build-arg SERVICE is required (server|client|swm)" && false)

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY ${SERVICE} /app/${SERVICE}
RUN chmod +x /app/${SERVICE}

# ARG 在运行时不可见,转成 ENV 让下面的 sh -c 能引用到 SERVICE 名,
# 这样 ps / docker top 看到的还是 /app/server 而不是 /app/app 这种泛名。
ENV SERVICE=${SERVICE}

# sh -c 后跟 -- 是 POSIX 标准 idiom: -- 占位 $0,
# 用户传给 docker run 的额外参数依次成为 $1 $2 ...,"$@" 透传给真正的二进制。
# exec 让 binary 直接接管 PID 1,信号 (SIGTERM) 才能正常送到。
#
# 不设 CMD: 二进制 -c 的默认值就是 "cfg.yaml",镜像里没这个文件没关系 ——
# Apollo 优先级高于本地 cfg,只要 APOLLO_* env 齐全就走 Apollo 路径,
# 走不到本地 fallback。留着 CMD ["-c", "cfg.yaml"] 反而会让 Apollo 故障时
# 报 "cfg.yaml not found" 这种误导性错误盖住 Apollo 失败的真因。
ENTRYPOINT ["/bin/sh", "-c", "exec /app/$SERVICE \"$@\"", "--"]
