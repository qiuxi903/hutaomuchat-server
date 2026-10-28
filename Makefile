BINARY := hutaomuchat-server
VERSION := 2.0.0

.PHONY: build build-windows build-linux run dev fmt vet test clean tidy

build: ## 构建当前平台的二进制文件
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

build-windows: ## 构建 Windows 版本
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY).exe .

build-linux: ## 构建 Linux 版本
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)-linux-amd64 .

run: ## 运行服务端
	go run .

dev: ## 以调试日志级别运行
	LOG_LEVEL=DEBUG go run .

fmt: ## 格式化所有 Go 源码
	gofmt -w .

vet: ## 静态检查
	go vet ./...

test: ## 运行单元测试
	go test ./...

tidy: ## 整理依赖（本项目零依赖，一般无需执行）
	go mod tidy

clean: ## 清理构建产物
	rm -f $(BINARY) $(BINARY).exe $(BINARY)-linux-amd64
