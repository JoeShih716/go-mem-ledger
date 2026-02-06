FROM golang:1.24-alpine

WORKDIR /app

# 安裝基本工具
RUN apk add --no-cache git make

# 預先複製依賴檔以利用 Docker Cache
COPY go.mod ./
# COPY go.sum ./ # 暫時沒有 go.sum

# 下載依賴
# RUN go mod download

# 複製其餘檔案
COPY . .

# 預設指令 (會被 docker-compose command 覆蓋)
CMD ["go", "run", "cmd/server/main.go"]
