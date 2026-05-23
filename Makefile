MODULE   := tacacs
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS  := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# GO 用 ?= 而不是 := ,让环境变量 (GO=/path make build) 也能覆盖;
# := 的话只有命令行参数 (make GO=/path build) 才覆盖,反直觉。
GO       ?= go
BUILD_DIR := build
SERVICES := server client swm

# GOOS/GOARCH 用 ?= (lazy), OUT_DIR 用 = 而不是 := ——
# 让 `go env ...` 推迟到 recipe 执行时再触发,而不是 Makefile 解析阶段就跑。
# 否则 `go` 不在 PATH 时,会先甩一堆 "make: go: Command not found",
# 最后报 "/ is not supported" 这种谜语错,根因藏在噪声里。
# check-platform 会先验 go 能否找到,再让后续 recipe 触发 shell 调用。
GOOS   ?= $(shell $(GO) env GOOS 2>/dev/null)
GOARCH ?= $(shell $(GO) env GOARCH 2>/dev/null)
OUT_DIR  = $(BUILD_DIR)/$(GOOS)_$(GOARCH)

SUPPORTED := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: all build clean release check-platform $(addprefix build-,$(SERVICES))

all: build

check-platform:
	@command -v $(GO) >/dev/null 2>&1 || { \
		echo "Error: 找不到 \`$(GO)\` 命令,Go 没装或者不在 PATH 里。"; \
		echo ""; \
		echo "修法 (任选其一):"; \
		echo "  1. 永久: ~/.zshrc 里加 export PATH=\"/usr/local/go/bin:\$$PATH\",然后 source ~/.zshrc"; \
		echo "  2. 单次: GO=/full/path/to/go make build   (环境变量)"; \
		echo "          或 make GO=/full/path/to/go build (命令行参数)"; \
		echo "  3. IDE : 用 Goland / VS Code 内置终端 (会继承 IDE 配置的 PATH)"; \
		exit 1; \
	}
	@if echo "$(SUPPORTED)" | grep -qw "$(GOOS)/$(GOARCH)"; then :; else \
		echo "Error: $(GOOS)/$(GOARCH) is not supported."; \
		echo "Supported platforms: $(SUPPORTED)"; \
		echo "Recommend using Linux or macOS."; \
		exit 1; \
	fi

build: check-platform $(addprefix build-,$(SERVICES))

build-server: check-platform
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/server ./cmd/server

build-client: check-platform
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/client ./cmd/client

build-swm: check-platform
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/swm ./cmd/swm

release:
	@for platform in $(SUPPORTED); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		echo "Building $$os/$$arch ..."; \
		GOOS=$$os GOARCH=$$arch $(MAKE) build --no-print-directory || exit 1; \
	done
	@echo "All platforms built successfully."

clean:
	rm -rf $(BUILD_DIR)
