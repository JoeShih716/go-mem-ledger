package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gopkg.in/yaml.v3"

	grpc_adapter "github.com/JoeShih716/go-mem-ledger/internal/app/core/adapter/in/grpc"
	mysql_adapter "github.com/JoeShih716/go-mem-ledger/internal/app/core/adapter/out/mysql"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/mysql"
	pb "github.com/JoeShih716/go-mem-ledger/proto"
)

type Config struct {
	MySQL mysql.Config `yaml:"mysql"`
}

func main() {
	// 1. 載入設定
	cfgData, err := os.ReadFile("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// 補全 MySQL 預設配置 (如果 yaml 沒寫)
	if cfg.MySQL.MaxOpenConns == 0 {
		cfg.MySQL.MaxOpenConns = 100
	}
	if cfg.MySQL.MaxIdleConns == 0 {
		cfg.MySQL.MaxIdleConns = 10
	}
	if cfg.MySQL.ConnMaxLifetime == 0 {
		cfg.MySQL.ConnMaxLifetime = 30 * time.Minute
	}

	// 2. 初始化 MySQL Client (Base Infrastructure)
	dbClient, err := mysql.NewClient(cfg.MySQL)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}
	defer dbClient.Close()
	log.Println("Connected to MySQL successfully")

	// 3. 初始化 Adapters (Level 0)
	ledgerRepo := mysql_adapter.NewMySQLLedger(dbClient)

	// 4. 初始化 UseCase
	coreUseCase := usecase.NewCoreUseCase(ledgerRepo)

	// 5. 初始化 gRPC Adapter (Driving Adapter)
	grpcServer := grpc_adapter.NewGrpcServer(coreUseCase)

	// 6. 啟動 gRPC Server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterLedgerServiceServer(s, grpcServer)
	reflection.Register(s) // 方便 gRPC Client 測試 (如 Postman/BloomRPC)

	// Graceful Shutdown
	go func() {
		log.Printf("Starting gRPC server on :50051")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	s.GracefulStop()
	log.Println("Server exited")
}
