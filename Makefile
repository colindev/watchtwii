# --- 設定變數 ---

PROJECT_ID := ${PROJECT_ID}
REPO_HOST  := ${REPO_HOST}
IMAGE_NAME := ${IMAGE_NAME}

RUN_PROJECT  := ${RUN_PROJECT}
REGION     := ${REGION}
JOB_NAME   := ${JOB_NAME}

TELEGRAM_TOKEN := ${TELEGRAM_TOKEN}
TELEGRAM_CHAT_IDS := ${TELEGRAM_CHAT_IDS}
THRESHOLD := ${THRESHOLD}

# VERSION 必須從外部傳入 (例如: make build VERSION=v1.0.0)
TAG := $(REPO_HOST)/$(PROJECT_ID)/$(IMAGE_NAME):$(VERSION)

# --- Cloud Scheduler 相關變數 ---
# 透過 Shell 指令取得 RUN_PROJECT 的計算服務帳號 (Compute SA)
SERVICE_ACCOUNT_EMAIL := $(shell gcloud projects describe $(RUN_PROJECT) --format='value(projectNumber)')-compute@developer.gserviceaccount.com
# 排程器 Job 名稱
SCHEDULER_JOB_NAME := cron-$(JOB_NAME)
# 排程頻率：每 15 分鐘一次
SCHEDULER_SCHEDULE := "*/5 * * * *"
SCHEDULER_DESCRIPTION := "每5分鐘"
# 目標 Cloud Run Job 的 URI (使用 RUN_PROJECT 才是正確的目標專案)
SCHEDULER_URI := "https://$(REGION)-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/$(RUN_PROJECT)/jobs/$(JOB_NAME):run"


.PHONY: all build deploy scheduler clean help

# 預設目標: 執行 build
all: build deploy scheduler

## build: 建構並推送 (Build and push) Docker 映像檔至 GCR。
#   - 必須透過環境變數傳入 VERSION (例如: make build VERSION=v0.0.1)
build:
	@echo "--- 檢查版本變數 ---"
	# 檢查 VERSION 變數是否為空，若為空則報錯退出
	@[ -z "$(VERSION)" ] && { echo "❌ 錯誤: 必須設定 VERSION 變數 (e.g., make build VERSION=v1.0.0)"; exit 1; } || true

	@echo "--- 執行 GCLOUD 建構與推送 ---"
	@echo "目標映像檔標籤: $(TAG)"
	# 執行 gcloud builds submit
	gcloud builds submit --tag $(TAG)

## deploy: 部署或更新 Cloud Run Job。
#   - 必須透過環境變數傳入 VERSION
#   - 會自動使用 $TAG 作為映像檔
deploy:
	@echo "--- 檢查版本變數 ---"
	# 檢查 VERSION 變數是否為空，若為空則報錯退出
	@[ -z "$(VERSION)" ] && { echo "❌ 錯誤: 必須設定 VERSION 變數 (e.g., make build VERSION=v1.0.0)"; exit 1; } || true
	@echo "--- 部署 Cloud Run Job (Target: $(RUN_PROJECT)) ---"
	@echo "部署版本: $(VERSION)"
	@echo "映像檔: $(TAG)"
	
	# 使用 gcloud run jobs deploy 統一處理創建和更新
	gcloud run jobs deploy $(JOB_NAME) \
	  --project $(RUN_PROJECT) \
	  --image $(TAG) \
	  --region $(REGION) \
	  --task-timeout 60s \
	  --cpu 1 \
	  --memory 512Mi \
	  --max-retries 0 \
	  --set-env-vars TELEGRAM_TOKEN="$(TELEGRAM_TOKEN)",TELEGRAM_CHAT_IDS="$(TELEGRAM_CHAT_IDS)",THRESHOLD="$(THRESHOLD),THRESHOLD_CHANGED=$(THRESHOLD_CHANGED),GCP_PROJECT=$(RUN_PROJECT)"
	
	@echo "✅ Cloud Run Job 部署成功或已更新至版本 $(VERSION)!"

## scheduler: 創建或更新 Cloud Scheduler Job，每15分鐘觸發一次 Cloud Run Job。
scheduler:
	@echo "--- 創建/更新 Cloud Scheduler Job ---"
	@echo "排程名稱: $(SCHEDULER_JOB_NAME)"
	@echo "頻率: $(SCHEDULER_SCHEDULE)"
	@echo "目標 URI: $(SCHEDULER_URI)"
	@echo "服務帳號: $(SERVICE_ACCOUNT_EMAIL)"
	
	# 嘗試更新現有的 Job，如果失敗 (Job不存在)，則創建新的 Job
	gcloud scheduler jobs update http $(SCHEDULER_JOB_NAME) \
	  --project $(RUN_PROJECT) \
	  --location $(REGION) \
	  --schedule $(SCHEDULER_SCHEDULE) \
	  --uri $(SCHEDULER_URI) \
	  --description "$(SCHEDULER_DESCRIPTION) 觸發 $(JOB_NAME) Job" \
	  --oauth-service-account-email "$(SERVICE_ACCOUNT_EMAIL)" \
	  --oauth-token-scope "https://www.googleapis.com/auth/cloud-platform" \
	  --http-method POST \
	|| \
	gcloud scheduler jobs create http $(SCHEDULER_JOB_NAME) \
	  --project $(RUN_PROJECT) \
	  --location $(REGION) \
	  --schedule $(SCHEDULER_SCHEDULE) \
	  --uri $(SCHEDULER_URI) \
	  --description "$(SCHEDULER_DESCRIPTION) 觸發 $(JOB_NAME) Job" \
	  --oauth-service-account-email "$(SERVICE_ACCOUNT_EMAIL)" \
	  --oauth-token-scope "https://www.googleapis.com/auth/cloud-platform" \
	  --http-method POST

	@echo "✅ Cloud Scheduler Job 已設定為每 15 分鐘運行一次。"

## help: 顯示所有可用目標 (Show available targets)
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# clean: 範例清除目標 (Example cleanup target, usually for local builds)
clean:
	@echo "無本地清除動作，跳過。"
