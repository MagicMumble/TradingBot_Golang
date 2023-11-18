package main

import (
	"fmt"
	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"math"
	"os"
	"sync"
	"time"
)

var (
	openMoscowHour    = 10
	closeMoscowHour   = 17
	closeMoscowMinute = 30
)

// TODO: getAllPositions может просто не отвечать когда биржа перестаёт работать и я не могу закрыть позиции - тинькофф возьмёт комиссию за незакрытые позиции (250 руб за ночь)

func sellOpenPositions(stats *TradingStatistics, operationsService *investgo.OperationsServiceClient, ordersService *investgo.OrdersServiceClient, accId string, logger investgo.Logger) {
	logger.Infof("Start selling open positions before calling a day")
	positions, money, err := getAllPositions(operationsService, accId, logger)
	if err != nil {
		logger.Errorf(err.Error())
	}
	for _, pos := range positions {
		lotsExecuted, lotsRequested, priceOrderExecuted, err := sell(ordersService, pos.Id, accId, pos.Balance, logger)
		logger.Infof("SELL at the end of the day stats: lotsExecuted = %v, lotsRequested = %v, priceOrderExecuted = %v", lotsExecuted, lotsRequested, priceOrderExecuted)
		if err != nil {
			logger.Infof("Couldn't close position! Instrument_id = %v", pos.Id)
			continue
		}
		calculateStatisticsAfterSell(stats, lotsExecuted, lotsRequested, priceOrderExecuted, logger)
	}
	logger.Infof("moneyTotal = %v", money)
}

func calculateStatisticsAfterSell(stats *TradingStatistics, lotsExecuted int64, lotsRequested int64, priceOrderExecuted float64, logger investgo.Logger) {
	stats.money += priceOrderExecuted
	logger.Infof("SELL stats: lotsExecuted = %v, lotsRequested = %v, priceOrderExecuted = %v, moneyTotal = %v", lotsExecuted, lotsRequested, priceOrderExecuted, stats.money)
	stats.sellPoint = priceOrderExecuted / float64(lotsExecuted)
	gain := stats.sellPoint - stats.buyPoint
	if gain > 0 {
		stats.successTransactionCount += 1
	} else {
		stats.failedTransactionCount += 1
	}
	if gain >= stats.maximumGain {
		stats.maximumGain = gain
		stats.maximumProfitPercent = stats.maximumGain / stats.buyPoint * 100
	}
	if gain <= stats.maximumLost {
		stats.maximumLost = gain
		stats.maximumLostPercent = stats.maximumLost / stats.buyPoint * 100
	}
	if stats.money > stats.maximumMoney {
		stats.maximumMoney = stats.money
	}
	if stats.money < stats.minimumMoney {
		stats.minimumMoney = stats.money
	}
	stats.transactionCount += 1

	stats.totalPercentProfit = stats.totalPercentProfit + (gain / stats.buyPoint)

	stats.totalTransactionLength = stats.totalTransactionLength + stats.transactionLength
	stats.totalGain = stats.totalGain + gain
	logger.Infof("Transaction took %v minutes\n", stats.transactionLength)
}

func getNewLogger() *zap.SugaredLogger {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{fmt.Sprintf("./logs/%s_tradeStats.log", time.Now().Format("2006_January_02")), "stderr"}
	zapConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime)
	zapConfig.EncoderConfig.TimeKey = "time"
	l, err := zapConfig.Build()
	if err != nil {
		log.Fatalf("logger creating error %v", err)
	}
	return l.Sugar()
}

func startStrategy(actions chan int, client *investgo.Client, accId string, instrumentId string, wg *sync.WaitGroup) {
	defer wg.Done()

	logger := getNewLogger()
	defer func() {
		err := logger.Sync()
		if err != nil {
			log.Printf(err.Error())
		}
	}()

	operationsService := client.NewOperationsServiceClient()
	marketDataService := client.NewMarketDataServiceClient()
	ordersService := client.NewOrdersServiceClient()
	shareNumber, shareNumberBefore := int64(0), int64(0)

	positions, myMoney, err := getAllPositions(operationsService, accId, logger)
	if err != nil {
		logger.Errorf(err.Error())
		os.Exit(-1)
	}

	canSell, canBuy := false, true

	//если на ночь остались акции, то по дефолту надо начинать не с покупки активов, а с их продажи! иначе отправляется запрос на покупку 0 акций, он не выполняется и бот никогда не переходит к продаже
	if len(positions) != 0 {
		canSell = true
		canBuy = false
		// we assume that we always work with only one stock uid
		shareNumber = positions[0].Balance
		shareNumberBefore = shareNumber
	}
	//forceSell := false

	stats := TradingStatistics{
		money:        myMoney,
		maximumMoney: myMoney,
		minimumMoney: myMoney,
	}

	logger.Infof("--------- START TRADING DAY ---------")
	logger.Infof("Start Capital: %v", stats.money)
	for {
		action, ok := <-actions
		if !ok || action == 4 {
			logger.Infof("Actions channel is closed, stop trading")
			break
		}
		logger.Infof("Got an action: %v", action)
		stats.transactionLength += 1
		if action == 1 && canBuy {
			logger.Infof("Got action BUY")
			stats.transactionLength = 0
			canSell, canBuy = true, false
			// TODO: что если я могу купить больше чем есть на бирже? как такое вообще отслеживать (потестить на счету с большими деньгами)
			// надо ли пытаться купить в следующие минуты если не получилось купить всё?? (наверное надо)
			lastPrice, err := getLastPrice(marketDataService, instrumentId, logger)
			if err != nil {
				canSell, canBuy = false, true
				logger.Infof("Processed action BUY")
				continue
			}
			// the price may change and I won't be able to buy needed amount of stocks (that's why -1)
			shareNumberBefore = shareNumber
			shareNumber = int64(math.Floor(stats.money/lastPrice)) - 1
			lotsExecuted, lotsRequested, priceOrderExecuted, err := buy(ordersService, instrumentId, accId, shareNumber, logger)
			if err != nil {
				canSell, canBuy = false, true
				shareNumber = shareNumberBefore
				continue
			}
			stats.money -= priceOrderExecuted
			logger.Infof("BUY stats: lotsExecuted = %v, lotsRequested = %v, priceOrderExecuted = %v, moneyTotal = %v", lotsExecuted, lotsRequested, priceOrderExecuted, stats.money)
			shareNumber = lotsExecuted
			shareNumberBefore = shareNumber
			stats.buyPoint = priceOrderExecuted / float64(shareNumber)
			logger.Infof("Processed action BUY")
			// TODO: complete strategy with the stop-loss signals
			//forceSell = false
		} else if action == 2 && canSell {
			logger.Infof("Got action SELL")
			// надо обработать случай когда не продали всё, что хотели!
			lotsExecuted, lotsRequested, priceOrderExecuted, err := sell(ordersService, instrumentId, accId, shareNumber, logger)
			if err != nil {
				logger.Infof("Processed action SELL")
				continue
			}
			canSell, canBuy = false, true
			calculateStatisticsAfterSell(&stats, lotsExecuted, lotsRequested, priceOrderExecuted, logger)
			logger.Infof("Processed action SELL")
		}
	}

	logger.Infof("Closing positions at the end of the day")
	sellOpenPositions(&stats, operationsService, ordersService, accId, logger)

	logger.Infof("Our System => totalMoney = %v", math.Floor(stats.money))
	logger.Infof("Number of transaction (BUY+SELL = 1 transaction) => %v", stats.transactionCount)

	if stats.transactionCount == 0 {
		logger.Infof("Percent success of transaction => 0%")
		logger.Infof("Average percent profit per transaction => 0%")
		logger.Infof("Average transaction length => 0#")
	} else {
		logger.Infof("Percent success of transaction => %v %%", (stats.successTransactionCount/stats.transactionCount)*100)
		logger.Infof("Average percent profit per transaction => %v %%", stats.totalPercentProfit/float64(stats.transactionCount)*100)
		logger.Infof("Average transaction length => %v #", stats.totalTransactionLength/stats.transactionCount)
	}

	logger.Infof("Maximum profit percent in transaction => %v %%", stats.maximumProfitPercent)
	logger.Infof("Maximum loss percent in transaction => %v %%", stats.maximumLostPercent)
	logger.Infof("Maximum capital value => %v RUB", stats.maximumMoney)
	logger.Infof("Minimum capital value =>  %v RUB", stats.minimumMoney)
	logger.Infof("--------- FINISH TRADING DAY ---------\n")

	// для метода GenerateBrokerReport песочница вернет []
	// TODO:метод не выполняется, запрос висит, ответ не приходит
	//generateReportResp, err := operationsService.GenerateBrokerReport(accId, time.Now().Add(-15*time.Hour), time.Now())
	//if err != nil {
	//	logger.Errorf(err.Error())
	//}
	//logger.Infof("\n--------- START REPORT ----------\n")
	//logger.Infof(generateReportResp.String())
	//logger.Infof("\n--------- END REPORT ----------\n")
}
