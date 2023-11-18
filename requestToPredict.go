package main

import "time"

type RequestToPredict struct {
	ReqId    uint64    `json:"ReqId"`
	Datetime time.Time `json:"Datetime"`
	Open     float64   `json:"Open"`
	High     float64   `json:"High"`
	Low      float64   `json:"Low"`
	Close    float64   `json:"Close"`
	AdjClose float64   `json:"Adj Close"`
	Volume   int64     `json:"Volume"`
}

// ResponseAction gets returned from the Python server: Action = 0 - HOLD, Action = 1 - BUY, Action = 2 - SELL
type ResponseAction struct {
	RespId int    `json:"RespId"`
	Action int    `json:"Action"`
	Error  string `json:"Error"`
}

type Position struct {
	Balance int64
	Id      string
}

type TradingStatistics struct {
	successTransactionCount int
	failedTransactionCount  int
	transactionLength       int
	totalTransactionLength  int
	transactionCount        int
	money                   float64
	sellPoint               float64
	buyPoint                float64
	maximumGain             float64
	maximumProfitPercent    float64
	maximumLost             float64
	maximumLostPercent      float64
	maximumMoney            float64
	minimumMoney            float64
	totalPercentProfit      float64
	totalGain               float64
}
