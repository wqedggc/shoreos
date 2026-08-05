package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

type ParsedFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type ParsedStatement struct {
	Platform   string `json:"platform"`
	AccountKey string `json:"accountKey"`
	StartAt    string `json:"startAt"`
	EndAt      string `json:"endAt"`
	ExportedAt string `json:"exportedAt"`
}

type ParsedTransaction struct {
	OccurredAt             string         `json:"occurredAt"`
	Category               string         `json:"category"`
	CounterpartyName       string         `json:"counterpartyName"`
	CounterpartyAccount    string         `json:"counterpartyAccount"`
	ProductDescription     string         `json:"productDescription"`
	Direction              string         `json:"direction"`
	AmountCents            int64          `json:"amountCents"`
	PaymentMethod          string         `json:"paymentMethod"`
	Status                 string         `json:"status"`
	PlatformOrderNo        string         `json:"platformOrderNo"`
	MerchantOrderNo        string         `json:"merchantOrderNo"`
	Remark                 string         `json:"remark"`
	RawSnapshot            map[string]any `json:"rawSnapshot"`
	RawRowHash             string         `json:"rawRowHash"`
	TransactionFingerprint string         `json:"transactionFingerprint"`
}

type ParsedImport struct {
	SourceType       string              `json:"sourceType"`
	ParserVersion    string              `json:"parserVersion"`
	File             ParsedFile          `json:"file"`
	Statement        ParsedStatement     `json:"statement"`
	Transactions     []ParsedTransaction `json:"transactions"`
	TransactionCount int                 `json:"transactionCount"`
}

type workerError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type Worker struct {
	PythonBin string
	Script    string
}

func (w Worker) Parse(ctx context.Context, filePath string) (ParsedImport, error) {
	if w.PythonBin == "" || w.Script == "" {
		return ParsedImport{}, errors.New("ledger parser is not configured")
	}
	output, err := exec.CommandContext(ctx, w.PythonBin, w.Script, filePath).Output()
	if err != nil {
		var failed workerError
		if json.Unmarshal(output, &failed) == nil && failed.Error.Code != "" {
			return ParsedImport{}, fmt.Errorf("ledger parser %s: %s", failed.Error.Code, failed.Error.Message)
		}
		return ParsedImport{}, errors.New("ledger parser failed")
	}
	var parsed ParsedImport
	if err := json.Unmarshal(output, &parsed); err != nil {
		return ParsedImport{}, fmt.Errorf("decode ledger parser result: %w", err)
	}
	if parsed.SourceType == "" || parsed.File.SHA256 == "" || parsed.Statement.Platform == "" {
		return ParsedImport{}, errors.New("ledger parser returned an incomplete result")
	}
	if parsed.TransactionCount != len(parsed.Transactions) {
		return ParsedImport{}, errors.New("ledger parser returned an inconsistent transaction count")
	}
	return parsed, nil
}
