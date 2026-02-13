package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	pb "github.com/JoeShih716/go-mem-ledger/proto"
)

const (
	TotalCount  = 1000000
	Concurrency = 1000
)

func main() {
	// 計算單筆交易 buffer大小
	// measureTransactionSize()
	// return
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewLedgerServiceClient(conn)

	totalCount := TotalCount
	concurrency := Concurrency
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(totalCount)

	sem := make(chan struct{}, concurrency)

	startTime := time.Now()

	for i := 0; i < totalCount; i++ {
		sem <- struct{}{}

		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			refID := uuid.New().String()
			_, err := c.Transfer(ctx, &pb.TransferRequest{
				RefId:         refID,
				Type:          pb.TransactionType_DEPOSIT,
				FromAccountId: 0,
				ToAccountId:   1,
				Amount:        10000,
			})

			if err != nil {
				if idx%10000 == 0 {
					log.Printf("Transfer %d failed: %v", idx, err)
				}
			}
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	fmt.Printf("Completed %d requests in %v\n", totalCount, elapsed)
	fmt.Printf("TPS: %.2f\n", float64(totalCount)/elapsed.Seconds())
}

// measureTransactionSize 測試計算單筆交易大小
// result // Single Transaction JSON Size: 192 bytes
func measureTransactionSize() {
	// 模擬一筆典型的交易
	tx := &domain.Transaction{
		TransactionID: uuid.New(),                     // 36 bytes string (JSON)
		From:          1234567890123456789,            // int64 (JSON ~10 bytes)
		To:            1234567890123456789,            // int64 (JSON ~10 bytes)
		Amount:        1000000000000000000,            // int64 (JSON ~5 bytes)
		Type:          domain.TransactionTypeTransfer, // int (JSON 1 byte)
		CreatedAt:     time.Now().UnixNano(),          // int64 (JSON ~19 bytes)
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(tx); err != nil {
		panic(err)
	}

	fmt.Printf("Single Transaction JSON Size: %d bytes\n", buf.Len())
}
