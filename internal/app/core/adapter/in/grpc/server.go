package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	pb "github.com/JoeShih716/go-mem-ledger/proto"
)

type GrpcServer struct {
	pb.UnimplementedLedgerServiceServer
	core *usecase.CoreUseCase
}

func NewGrpcServer(core *usecase.CoreUseCase) *GrpcServer {
	return &GrpcServer{
		core: core,
	}
}

func (s *GrpcServer) Transfer(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	// 1. UUID 解析
	u, err := uuid.Parse(req.RefId)
	if err != nil {
		return &pb.TransferResponse{
			Success: false,
			Message: "invalid ref_id: " + err.Error(),
		}, nil
	}

	// 2. 轉換交易類型
	var txType domain.TransactionType
	switch req.Type {
	case pb.TransactionType_DEPOSIT:
		txType = domain.TransactionTypeDeposit
	case pb.TransactionType_WITHDRAW:
		txType = domain.TransactionTypeWithdraw
	case pb.TransactionType_TRANSFER:
		txType = domain.TransactionTypeTransfer
	default:
		return &pb.TransferResponse{
			Success: false,
			Message: "invalid transaction type",
		}, nil
	}

	// 3. 組裝 Domain Transaction
	// domain.TransactionID 是 [16]byte, uuid.UUID 是 [16]byte
	tx := &domain.Transaction{
		TransactionID: u,
		From:          req.FromAccountId,
		To:            req.ToAccountId,
		Amount:        req.Amount,
		Type:          txType,
	}

	// 4. 執行交易
	err = s.core.PostTransaction(ctx, tx)
	if err != nil {
		// 業務邏輯錯誤，回傳 Success=false (Soft Failure)
		return &pb.TransferResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// 5. [Optional] 取得最新餘額 (Best Effort)
	// 根據 Proto 定義，轉帳/提款回傳 From 的餘額，存款回傳 To 的餘額
	var targetAccountID int64
	if txType == domain.TransactionTypeDeposit {
		targetAccountID = req.ToAccountId
	} else {
		targetAccountID = req.FromAccountId
	}

	balance, _ := s.core.GetAccountBalance(ctx, targetAccountID)

	return &pb.TransferResponse{
		Success:        true,
		CurrentBalance: balance,
	}, nil
}

func (s *GrpcServer) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	balance, err := s.core.GetAccountBalance(ctx, req.AccountId)
	if err != nil {
		if err == domain.ErrAccountNotFound {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetBalanceResponse{
		Balance: balance,
	}, nil
}
