# 階段 1: 建置 (Builder Stage)
FROM golang:1.24 AS builder

# 設定工作目錄
WORKDIR /app

# 複製 go.mod 和 go.sum 並下載依賴
COPY go.mod go.sum ./
RUN go mod download

# 複製所有原始碼
COPY . .

# 建置靜態連結的二進位執行檔
# CGO_ENABLED=0 確保沒有 C 語言依賴，使得執行檔更輕量且可移植
RUN CGO_ENABLED=0 go build -o /main .


# 階段 2: 最終輕量級執行環境 (Final Stage)
FROM alpine:latest

# 安裝 ca-certificates 是為了讓 HTTPS 請求可以運作
RUN apk --no-cache add ca-certificates tzdata

# 設定時區 (非必須，但有利於日誌記錄)
ENV TZ Asia/Taipei

# 將建置好的二進位執行檔從 Builder 階段複製過來
COPY --from=builder /main /main

# 複製設定檔
COPY --from=builder /app/.env /.env

# 設定啟動點 (ENTRYPOINT)
CMD ["/main"]
