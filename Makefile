# --- 設定變數 ---

# GCP 專案 ID (從您的範例中擷取: colin-repo)
PROJECT_ID := colin-repo
# GCR 所在區域/主機 (從您的範例中擷取: asia.gcr.io)
REPO_HOST  := asia.gcr.io
# 映像檔名稱
IMAGE_NAME := watchtwii

# VERSION 必須從外部傳入 (例如: make build VERSION=v1.0.0)
# 最終的 TAG 格式: asia.gcr.io/colin-repo/watchtwii:v0.0.1
TAG := $(REPO_HOST)/$(PROJECT_ID)/$(IMAGE_NAME):$(VERSION)

.PHONY: all build clean help

# 預設目標: 執行 build
all: build

## build: 建構並推送 (Build and push) Docker 映像檔至 GCR。
#   - 必須透過環境變數傳入 VERSION (例如: make build VERSION=v0.0.1)
build:
	@echo "--- 檢查版本變數 ---"
	# 檢查 VERSION 變數是否為空，若為空則報錯退出
	@[ -z "$(VERSION)" ] && { echo "❌ 錯誤: 必須設定 VERSION 變數 (e.g., make build VERSION=v1.0.0)"; exit 1; }

	@echo "--- 執行 GCLOUD 建構與推送 ---"
	@echo "目標映像檔標籤: $(TAG)"
	# 執行 gcloud builds submit
	gcloud builds submit --tag $(TAG)

## help: 顯示所有可用目標 (Show available targets)
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# clean: 範例清除目標 (Example cleanup target, usually for local builds)
clean:
	@echo "無本地清除動作，跳過。"
