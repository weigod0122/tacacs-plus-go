#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"

# detect_platform 决定从 build/<platform>/ 下取哪个 binary。
# 优先 go env(与 Makefile 输出对齐);go 不在 PATH 时(典型场景:prod 机器
# 没装 Go,或 mac 自带终端没配 PATH)回落 uname 自动识别。
# 之前硬编码 `... || echo linux_amd64`,在 mac 上没 go 时会把 darwin_arm64
# 的构建产物误判成不存在,报"binary not found at .../linux_amd64/server"。
# PLATFORM 环境变量可显式覆盖(交叉编译 / 调试用)。
detect_platform() {
    local goos goarch
    if command -v go >/dev/null 2>&1; then
        goos="$(go env GOOS)"
        goarch="$(go env GOARCH)"
    else
        case "$(uname -s)" in
            Darwin) goos=darwin ;;
            Linux)  goos=linux ;;
            *)      goos="$(uname -s | tr '[:upper:]' '[:lower:]')" ;;
        esac
        case "$(uname -m)" in
            x86_64|amd64)  goarch=amd64 ;;
            arm64|aarch64) goarch=arm64 ;;
            *)             goarch="$(uname -m)" ;;
        esac
    fi
    echo "${goos}_${goarch}"
}
PLATFORM="${PLATFORM:-$(detect_platform)}"
BIN_DIR="${BUILD_DIR}/${PLATFORM}"
SERVICES=("server" "client" "swm")

# 启动后等待几秒观察进程,过早返回会让 cfg 解析失败 / db ping 失败这类
# "起来就死" 的情况静默通过。12 秒的下限来自 agollo 客户端的硬编码 10s 超时:
# 没配 Apollo 时它会等 10s 才报 "no config was read" 并触发本地 cfg 回退,
# 如果本地 cfg 也不存在,真正的退出会发生在 ~10s 之后。再加 2s 留给 DB ping
# 和 listener 起来。
STARTUP_OBSERVE_SECS=12
# 进程死了时回显的尾部日志行数。50 行足够覆盖大多数 panic + 配置错误,
# 又不至于刷屏。原始日志路径会同时打出来,需要看更多自己 cat / tail -f。
CRASH_LOG_LINES=50

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> <service>

Commands:
    start     Start a service in the background
    stop      Gracefully stop a service (SIGTERM)
    restart   Restart a service
    status    Show service status

Services: server, client, swm

Options (environment variables):
    APP_ENV     Runtime environment: test or prod (default: test)
    CFG_FILE    Config file path (default: cfg.yaml, resolved against \$PWD)
    LOG_DIR     Log output directory (default: build/)
    PLATFORM    Build platform subdir under build/ (default: auto-detect via
                go env, or uname when go is unavailable; e.g. darwin_arm64)

Examples:
    $(basename "$0") start server
    $(basename "$0") status swm
    APP_ENV=prod $(basename "$0") start server
    CFG_FILE=/etc/tacacs/server.yaml $(basename "$0") start server
EOF
    exit 1
}

validate_service() {
    local svc="$1"
    for s in "${SERVICES[@]}"; do
        [[ "$s" == "$svc" ]] && return 0
    done
    echo "Error: unknown service '$svc'. Must be one of: ${SERVICES[*]}"
    exit 1
}

pid_file() { echo "${BUILD_DIR}/$1.pid"; }

is_running() {
    local pf
    pf="$(pid_file "$1")"
    [[ -f "$pf" ]] && kill -0 "$(cat "$pf")" 2>/dev/null
}

# abs_path 把 cfg/apollo 这种可能是相对路径的入参展开成绝对路径,
# 解析基准是当前 shell 的 $PWD —— 与 nohup 起来的 binary 看到的 CWD 完全一致,
# 所以打出来的"绝对路径"就是 binary 真正去 open 的那条路径。
abs_path() {
    local p="$1"
    if [[ "$p" = /* ]]; then
        echo "$p"
    else
        echo "${PWD}/$p"
    fi
}

# preflight_config 反映 binary 真实的加载顺序:Apollo 优先,失败时回落本地 cfg
# (见 pkg/public/cfg/cfg.go:loadConfigContent)。所以:
#   - apollo.yaml 或 APOLLO_APP_ID 任一可用即可,本地 cfg 缺席只是没了"兜底",起服不受影响;
#   - 两者都没有时,binary 起来必死,直接 abort,不要白白走一遍 nohup + 12s 观察。
# 同时回显一行"将按什么顺序尝试",方便排查"Apollo 起不来为啥走了本地"。
preflight_config() {
    local svc="$1" cfg="$2"
    local cfg_abs apollo_abs
    cfg_abs="$(abs_path "$cfg")"
    apollo_abs="${PWD}/apollo.yaml"

    local has_cfg=false has_apollo=false
    [[ -f "$cfg_abs" ]] && has_cfg=true
    if [[ -f "$apollo_abs" || -n "${APOLLO_APP_ID:-}" ]]; then
        has_apollo=true
    fi

    if ! $has_apollo && ! $has_cfg; then
        echo "  ❌ no config source — binary will fail to start"
        echo "      Apollo : neither ${apollo_abs} nor APOLLO_APP_ID env set"
        echo "      Local  : ${cfg_abs} not found"
        echo "      fix    : 任选其一(Apollo 失败会自动回落本地,两份都给最稳)"
        echo "       1) cp ${ROOT_DIR}/static/cfg/apollo_example.yaml ${apollo_abs}"
        echo "       2) cp ${ROOT_DIR}/static/cfg/cfg_${svc}_example.yaml ${cfg_abs}"
        exit 1
    fi

    if $has_apollo && $has_cfg; then
        echo "  ℹ️  config: Apollo first → fallback ${cfg_abs}"
    elif $has_apollo; then
        echo "  ℹ️  config: Apollo only (no local fallback — Apollo 起不来即崩)"
    else
        echo "  ℹ️  config: local ${cfg_abs} only (Apollo 未配置)"
    fi
}

# dump_crash_log 把 .out 文件最后 N 行打出来,前缀缩进便于和脚本输出区分。
# 文件不存在(binary 在 fork 之后立刻挂,nohup 还没来得及写)时给出明确提示。
dump_crash_log() {
    local out_file="$1"
    echo "  ─────── last ${CRASH_LOG_LINES} lines of ${out_file} ───────"
    if [[ -f "$out_file" ]]; then
        tail -n "$CRASH_LOG_LINES" "$out_file" 2>/dev/null | sed 's/^/  │ /' || true
    else
        echo "  │ (log file not created — process likely died before writing stdout)"
    fi
    echo "  ─────── end of log ───────"
    echo "  full log: ${out_file}"
}

do_start() {
    local svc="$1"
    local bin="${BIN_DIR}/${svc}"
    local pf
    pf="$(pid_file "$svc")"
    local cfg="${CFG_FILE:-cfg.yaml}"
    local log_dir="${LOG_DIR:-${BUILD_DIR}}"
    local out_file="${log_dir}/${svc}.out"

    if is_running "$svc"; then
        echo "${svc} is already running (pid $(cat "$pf"))"
        return 0
    fi

    if [[ ! -x "$bin" ]]; then
        echo "Error: binary not found at ${bin}"
        # 列出实际有哪些已构建平台,大概率能一眼看出是 PLATFORM 检测错了
        # (比如 mac 上没 go 默认回落到 linux_amd64,但 build/ 里其实只有 darwin_arm64)。
        local existing
        existing="$(ls -1 "$BUILD_DIR" 2>/dev/null | grep -E '^(linux|darwin)_(amd64|arm64)$' | tr '\n' ' ' || true)"
        if [[ -n "$existing" ]]; then
            echo "  built platforms in build/: ${existing}"
            echo "  current detected platform: ${PLATFORM}"
            echo "  hint: 平台不匹配可显式覆盖, 如  PLATFORM=darwin_arm64 $0 $cmd $svc"
        fi
        echo "Run 'make build-${svc}' first."
        exit 1
    fi

    if [[ "$svc" == "swm" ]] && ! is_running "server"; then
        echo "Warning: server is not running. SwM reverse proxy may not work until server starts."
    fi

    preflight_config "$svc" "$cfg"

    mkdir -p "$log_dir"
    APP_ENV="${APP_ENV:-test}" nohup "$bin" -c "$cfg" > "$out_file" 2>&1 &
    local pid=$!
    echo "$pid" > "$pf"
    echo "${svc} launched (pid ${pid}), observing for ${STARTUP_OBSERVE_SECS}s..."

    # 观察阶段:每秒一个点。两个提前退出条件:
    #   1. 进程消失 → 立刻 break + 打日志,不要傻等满 12s
    #   2. .out 文件出现 "READY: <svc> initialized" 标记 → 起服成功,提前返回
    # 标记由各服务在所有 init 完成、即将 sigwait 之前 println 出来,
    # 正常 1-2s 就能看到,不再死等 12s 观察窗口。
    # 兜底:如果走完 12s 都没看见标记但进程还活着,沿用原 "stable" 语义不报错——
    # 兼容老 binary 没打这一行的场景,以及打了但被人为吞掉 stdout 的边缘情况。
    local i
    local ready_marker="READY: ${svc} initialized"
    for ((i = 1; i <= STARTUP_OBSERVE_SECS; i++)); do
        sleep 1
        if ! kill -0 "$pid" 2>/dev/null; then
            printf '\n'
            echo "  ❌ ${svc} exited within ${i}s"
            dump_crash_log "$out_file"
            rm -f "$pf"
            exit 1
        fi
        if grep -qF "$ready_marker" "$out_file" 2>/dev/null; then
            printf '\n'
            echo "  ✓ ${svc} ready in ${i}s (pid ${pid})"
            return 0
        fi
        printf '.'
    done
    printf '\n'
    echo "  ✓ ${svc} stable after ${STARTUP_OBSERVE_SECS}s (pid ${pid}, no READY marker seen)"
}

do_stop() {
    local svc="$1"
    local pf
    pf="$(pid_file "$svc")"

    if ! is_running "$svc"; then
        echo "${svc} is not running"
        rm -f "$pf"
        return 0
    fi

    local pid
    pid="$(cat "$pf")"
    echo "Stopping ${svc} (pid ${pid})..."
    kill "$pid"

    local waited=0
    while kill -0 "$pid" 2>/dev/null; do
        sleep 1
        waited=$((waited + 1))
        if [[ $waited -ge 30 ]]; then
            echo "Warning: ${svc} did not exit after 30s, sending SIGKILL"
            kill -9 "$pid" 2>/dev/null || true
            break
        fi
    done

    rm -f "$pf"
    echo "${svc} stopped"
}

do_status() {
    local svc="$1"
    local pf
    pf="$(pid_file "$svc")"

    if is_running "$svc"; then
        echo "${svc} is running (pid $(cat "$pf"))"
    else
        echo "${svc} is not running"
        rm -f "$pf"
    fi
}

# --- main ---

[[ $# -lt 2 ]] && usage

cmd="$1"
svc="$2"
validate_service "$svc"

case "$cmd" in
    start)   do_start "$svc" ;;
    stop)    do_stop "$svc" ;;
    restart) do_stop "$svc"; do_start "$svc" ;;
    status)  do_status "$svc" ;;
    *)       usage ;;
esac
