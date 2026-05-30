#!/usr/bin/env bash
# 把 build/linux_amd64/<svc> 打成 docker 镜像,一个服务一个镜像。
# 可选直接推到 registry。
# 不在脚本里跑 make build —— 让人显式 make 完再来打包,
# 避免 "镜像里到底是哪次编译的代码" 这种问题。
#
# Usage:
#   scripts/docker-build.sh build <service|all>             # 只构建
#   scripts/docker-build.sh push  <service|all>             # 只推送已存在的镜像
#   scripts/docker-build.sh build --push <service|all>      # 构建 + 推送
#   scripts/docker-build.sh promote <service|all>           # 把已发布的 tag 重新打 alias 推回 (默认 alias=stable)
#
# 兼容老用法(隐式 build):
#   scripts/docker-build.sh <service|all>
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
DOCKERFILE="${ROOT_DIR}/docker/Dockerfile"

SERVICES=("server" "client" "swm")
PLATFORM="${PLATFORM:-linux_amd64}"
BIN_DIR="${BUILD_DIR}/${PLATFORM}"

# 镜像命名: <prefix>/<service>:<tag>
# 用 "/" 而不是 "-",对齐 Docker 的 namespace/repo 约定。Harbor 等私有 registry
# 把第一段 path 当 project,所以 IMAGE_PREFIX=harbor.x.com/tacacs 会推到
# harbor.x.com/tacacs/server,落在 tacacs 项目下;之前用 "-" 拼成
# harbor.x.com/tacacs-server,Harbor 找不到名叫 "tacacs-server" 的 project,400。
# 推送时 IMAGE_PREFIX 必须包含 registry: IMAGE_PREFIX=harbor.x.com/tacacs
#
# Tag 策略: 默认用**构建时间戳** (UTC, YYYYMMDD-HHMMSS),与 git 状态无关。
# 整个脚本调用共享同一个时间戳,所以 `build all` 出来的三个镜像 tag 相同,便于成组发布。
# 想跑出可回溯的 release 镜像,显式 IMAGE_TAG=v1.2.0 即可。
# 每次构建/推送都会**同时**打 :latest tag,方便 `docker pull <image>` 不带 tag 也能拉到最新。
IMAGE_PREFIX="${IMAGE_PREFIX:-tacacs}"

RESOLVED_TAG=""
resolve_image_tag() {
    # 已解析过就直接返回,resolve 仅做一次,确保 build all 出来的多个镜像 tag 一致
    if [[ -n "$RESOLVED_TAG" ]]; then return; fi

    # IMAGE_TAG 显式传入 → 直接用 (release 打 tag / CI 自带版本号场景)
    if [[ -n "${IMAGE_TAG:-}" ]]; then
        RESOLVED_TAG="$IMAGE_TAG"
        return
    fi

    # 默认: 拿编译时刻的 UTC 时间戳做 tag,不依赖 git。
    # UTC 是为了跨时区团队 / CI 拿到一致的字符串;格式 YYYYMMDD-HHMMSS 字典序即时间序,
    # 列镜像时新版本自然排在最后。Docker tag 合法字符,无需转义。
    RESOLVED_TAG="$(date -u +%Y%m%d-%H%M%S)"
}

image_versioned() {
    resolve_image_tag
    echo "${IMAGE_PREFIX}/$1:${RESOLVED_TAG}"
}

image_latest() {
    echo "${IMAGE_PREFIX}/$1:latest"
}

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [--push] <service|all>

Commands:
    build           Build image from build/${PLATFORM}/<service>
    push            Push existing image to registry
    build --push    Build then push
    promote         给已发布的镜像追加一个 alias tag (默认 stable),不重新编译

Services: ${SERVICES[*]}, all

Tag policy:
    默认 tag = 编译时的 UTC 时间戳 (YYYYMMDD-HHMMSS),与 git 状态无关。
    一次脚本调用内 (含 build all) 共享同一个时间戳,三个服务镜像 tag 一致。
    每次 build/push 都会**同时**打 :<timestamp> 和 :latest 两个 tag,
    让 \`docker pull <image>\` 默认拉到最新。
    打 release 显式传 IMAGE_TAG=v1.2.0,会跳过时间戳直接用。

Promote (latest → stable 提升):
    构建产物默认带 :<timestamp> + :latest,生产想要 pin 一个稳定版本时,
    用 promote 把某个已发布的时戳重新打成 :stable (或自定义 alias) 推回 registry。
    不重新编译,registry 侧 :<target> 与 :<source> 指向同一个 digest。
    本地没有该镜像时会先 docker pull。

    必填: IMAGE_PREFIX (带 registry), SOURCE_TAG (要 promote 的源 tag)
    可选: TARGET_TAG (默认 stable)

Environment overrides:
    IMAGE_PREFIX  Image name prefix              (default: tacacs)
                  推送/promote 时必须带 registry,会拼成 <prefix>/<service>:<tag>
                  例: IMAGE_PREFIX=harbor.x.com/tacacs → harbor.x.com/tacacs/server:tag
    IMAGE_TAG     Image tag                      (default: UTC build timestamp)
    SOURCE_TAG    promote 的源 tag               (无默认,promote 必填)
    TARGET_TAG    promote 的目标 alias           (default: stable)
    PLATFORM      Source binary platform subdir  (default: linux_amd64)

Examples:
    # 本地构建 (生成 tacacs/server:<UTC 时间戳> + tacacs/server:latest)
    $(basename "$0") build server
    $(basename "$0") build all

    # 构建并推送 (一步到位,落在 harbor 的 tacacs 项目下;同时推 :<timestamp> 和 :latest)
    IMAGE_PREFIX=harbor.x.com/tacacs $(basename "$0") build --push all

    # 推送已有镜像 (IMAGE_TAG 必须明确传,因为构建时戳在脚本结束就丢了)
    IMAGE_PREFIX=harbor.x.com/tacacs IMAGE_TAG=20260526-103015 $(basename "$0") push server

    # 打 release: 显式传 release 版本号
    IMAGE_TAG=v1.2.0 $(basename "$0") build all

    # promote: 把 20260526-103015 这版标记为 stable 推到 registry
    IMAGE_PREFIX=harbor.x.com/tacacs SOURCE_TAG=20260526-103015 $(basename "$0") promote server
    # 自定义 alias 名:
    IMAGE_PREFIX=harbor.x.com/tacacs SOURCE_TAG=v1.2.0 TARGET_TAG=production $(basename "$0") promote all

注意: 推送/promote 前先 docker login <registry>。
EOF
    exit 1
}

require_docker() {
    command -v docker >/dev/null 2>&1 || {
        echo "Error: docker not found in PATH"
        exit 1
    }
}

build_one() {
    local svc="$1"
    local bin="${BIN_DIR}/${svc}"

    if [[ ! -x "$bin" ]]; then
        echo "Error: binary not found or not executable: ${bin}"
        echo "  hint: GOOS=linux GOARCH=amd64 make build-${svc}"
        exit 1
    fi

    # 必须在父 shell 里先解析 tag,RESOLVED_TAG 才落到当前进程的变量空间;
    # 否则下面 `$(image_versioned ...)` 的命令替换是子 shell,父 shell 的
    # RESOLVED_TAG 仍为空,每个子 shell 会各自跑 `date` 拿不同时间戳,
    # 导致 build 与 push 的镜像 tag 不一致 → push 找不到本地镜像。
    resolve_image_tag

    local ver latest
    ver="$(image_versioned "$svc")"
    latest="$(image_latest "$svc")"
    echo "==> Building ${ver}"
    echo "             ${latest}"
    echo "    binary : ${bin}"
    echo "    size   : $(du -h "$bin" | awk '{print $1}')"

    # --platform=linux/amd64 强制 alpine 基础层和二进制 arch 对齐。
    # Apple Silicon 上不加这行,docker 会拉 arm64 alpine,镜像里 sh/libc 全是 arm64,
    # 但 COPY 进去的 binary 是 amd64,推到真正的 amd64 服务器上 sh 无法启动 → exec format error。
    # Build context = BIN_DIR,只包含三个二进制,context 总大小 < 100MB,
    # 不需要 .dockerignore 也不会把 .git / static / pkg 这些塞进去。
    # 同时打 :<tag> 和 :latest,让 `docker pull <image>` 默认拉到最新。
    docker build \
        --platform=linux/amd64 \
        -f "$DOCKERFILE" \
        --build-arg SERVICE="$svc" \
        -t "$ver" \
        -t "$latest" \
        "$BIN_DIR"

    echo "    ✓ ${ver}"
    echo "    ✓ ${latest}"
}

push_one() {
    local svc="$1"
    # 同 build_one,先在父 shell 落 RESOLVED_TAG,后面命令替换里读到的才是同一个值。
    resolve_image_tag
    local ver latest
    ver="$(image_versioned "$svc")"
    latest="$(image_latest "$svc")"

    # 镜像必须先存在于本地 daemon。校验 versioned tag 即可,:latest 由 build 一并打。
    if ! docker image inspect "$ver" >/dev/null 2>&1; then
        echo "Error: image not found locally: ${ver}"
        echo "  hint: 先 $(basename "$0") build ${svc},或检查 IMAGE_PREFIX / IMAGE_TAG 是否匹配"
        exit 1
    fi

    # 没有 registry 前缀(形如 tacacs-server)docker 会默认推到 Docker Hub,
    # 极易把内部镜像推到公网,所以推送时强制 IMAGE_PREFIX 包含 "/"。
    # 例: harbor.x.com/tacacs / registry.cn-hangzhou.aliyuncs.com/myns
    if [[ "$IMAGE_PREFIX" != */* ]]; then
        echo "Error: IMAGE_PREFIX='${IMAGE_PREFIX}' 不像 registry 地址 (缺 '/')."
        echo "  推送前请设置带 registry 的 prefix, 如:"
        echo "    IMAGE_PREFIX=harbor.x.com/tacacs $(basename "$0") push ${svc}"
        echo "  否则 docker 会默认推到 Docker Hub。"
        exit 1
    fi

    echo "==> Pushing ${ver}"
    docker push "$ver"
    echo "    ✓ pushed ${ver}"

    echo "==> Pushing ${latest}"
    docker push "$latest"
    echo "    ✓ pushed ${latest}"
}

# promote_one: 把 ${IMAGE_PREFIX}/<svc>:${SOURCE_TAG} 重新打成
# ${IMAGE_PREFIX}/<svc>:${TARGET_TAG} 推回 registry。
# 不重新编译,registry 侧两个 tag 指向同一个 digest;
# 本地缺源镜像时先 docker pull,让 promote 在任意机器都能跑(不依赖原始构建机)。
promote_one() {
    local svc="$1"
    local source_image="${IMAGE_PREFIX}/${svc}:${SOURCE_TAG}"
    local target_image="${IMAGE_PREFIX}/${svc}:${TARGET_TAG}"

    if ! docker image inspect "$source_image" >/dev/null 2>&1; then
        echo "==> Pulling ${source_image} (not in local daemon)"
        if ! docker pull "$source_image"; then
            echo "Error: failed to pull ${source_image}"
            echo "  hint: 检查 IMAGE_PREFIX / SOURCE_TAG 是否拼对,以及 docker login 状态"
            exit 1
        fi
    fi

    echo "==> Tagging ${source_image}"
    echo "         → ${target_image}"
    docker tag "$source_image" "$target_image"

    echo "==> Pushing ${target_image}"
    docker push "$target_image"
    echo "    ✓ promoted ${svc}: ${SOURCE_TAG} → ${TARGET_TAG}"
}

run_for_target() {
    local action="$1" target="$2"
    case "$target" in
        all)
            for s in "${SERVICES[@]}"; do "$action" "$s"; done
            ;;
        server|client|swm)
            "$action" "$target"
            ;;
        *)
            echo "Error: unknown service '$target' (expected: ${SERVICES[*]}, all)"
            exit 1
            ;;
    esac
}

# --- arg parsing ---
[[ $# -lt 1 ]] && usage
require_docker

cmd="$1"; shift
push_flag=0

case "$cmd" in
    build)
        # build [--push] <target>
        if [[ "${1:-}" == "--push" ]]; then
            push_flag=1
            shift
        fi
        [[ $# -lt 1 ]] && usage
        target="$1"
        run_for_target build_one "$target"
        if [[ $push_flag -eq 1 ]]; then
            run_for_target push_one "$target"
        fi
        ;;
    push)
        [[ $# -lt 1 ]] && usage
        run_for_target push_one "$1"
        ;;
    promote)
        [[ $# -lt 1 ]] && usage
        # SOURCE_TAG 必填(没有默认值),源 tag 都不知道是哪个就别玩 promote;
        # TARGET_TAG 默认 stable,符合「latest 灰度 / stable 生产」的常见拓扑。
        : "${SOURCE_TAG:?SOURCE_TAG 必填,例: SOURCE_TAG=20260526-103015 或 SOURCE_TAG=v1.2.0}"
        TARGET_TAG="${TARGET_TAG:-stable}"
        # promote 必须推到真 registry,不允许默认 prefix 兜底(否则会推到 Docker Hub)。
        if [[ "$IMAGE_PREFIX" != */* ]]; then
            echo "Error: IMAGE_PREFIX='${IMAGE_PREFIX}' 不像 registry 地址 (缺 '/')."
            echo "  promote 前请设置带 registry 的 prefix:"
            echo "    IMAGE_PREFIX=harbor.x.com/tacacs SOURCE_TAG=... $(basename "$0") promote ${1}"
            exit 1
        fi
        # 同 tag 自指 = 没意义,提前拒绝避免误用
        if [[ "$SOURCE_TAG" == "$TARGET_TAG" ]]; then
            echo "Error: SOURCE_TAG (${SOURCE_TAG}) 和 TARGET_TAG (${TARGET_TAG}) 相同,promote 无效"
            exit 1
        fi
        run_for_target promote_one "$1"
        ;;
    -h|--help|help)
        usage
        ;;
    # 兼容老用法: 第一个参数直接是 service|all,等价于 `build <target>`
    server|client|swm|all)
        run_for_target build_one "$cmd"
        ;;
    *)
        echo "Error: unknown command '$cmd'"
        usage
        ;;
esac
