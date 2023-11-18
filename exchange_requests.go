package main

import (
	"errors"
	"fmt"
	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	pb "github.com/russianinvestments/invest-api-go-sdk/proto"
	"sync/atomic"
	"time"
)

func getInstrumentId(client *investgo.Client, logger investgo.Logger, ticker string) (string, error) {
	// далее вызываем нужные нам сервисы, используя счет, токен, и эндпоинт песочницы
	var (
		instrumentResp *investgo.FindInstrumentResponse
		err            error
	)
	defer func() {
		if err != nil {
			logger.Errorf(err.Error())
		}
	}()
	instrumentsService := client.NewInstrumentsServiceClient()

	instrumentResp, err = instrumentsService.FindInstrument(ticker)
	if err != nil {
		return "", err
	}
	instruments := instrumentResp.GetInstruments()
	for _, instrument := range instruments {
		if instrument.GetTicker() == "TCSG" {
			return instrument.GetUid(), nil
		}
	}
	err = fmt.Errorf("no instrument found")
	return "", err
}

func getAccountId(client *investgo.Client, sandboxService *investgo.SandboxServiceClient, logger investgo.Logger) (string, error) {
	var (
		newAccId     string
		err          error
		accountsResp *investgo.GetAccountsResponse
		openAccount  *investgo.OpenSandboxAccountResponse
	)
	defer func() {
		if err != nil {
			logger.Errorf("Failed to get account id: %v", err.Error())
		}
	}()

	accountsResp, err = sandboxService.GetSandboxAccounts()
	if err != nil {
		return "", err
	}
	accs := accountsResp.GetAccounts()
	if len(accs) > 0 {
		newAccId = accs[0].GetId()
	} else {
		openAccount, err = sandboxService.OpenSandboxAccount()
		if err != nil {
			return "", err
		}
		newAccId = openAccount.GetAccountId()
		client.Config.AccountId = newAccId
	}
	return newAccId, err
}

func depositMoney(sandboxService *investgo.SandboxServiceClient, accountId string, money int64, logger investgo.Logger) error {
	logger.Infof("Sent depositMoney request")
	payInResp, err := sandboxService.SandboxPayIn(&investgo.SandboxPayInRequest{
		AccountId: accountId,
		Currency:  "RUB",
		Unit:      money,
		Nano:      0,
	})
	logger.Infof("Got response for depositMoney request")
	if err != nil {
		logger.Errorf("Can't deposit money: %v", err.Error())
		return err
	}
	logger.Infof("sandbox account %v after deposition money: balance = %v\n", accountId, payInResp.GetBalance().ToFloat())
	return nil
}

func getHistoricalData(client *investgo.Client, instrumentId string, logger investgo.Logger) error {
	// минутные свечи за последние 2 месяца
	logger.Infof("Sent getHistoricalData request")
	MarketDataService := client.NewMarketDataServiceClient()
	logger.Infof("Got response for getHistoricalData request")
	_, err := MarketDataService.GetHistoricCandles(&investgo.GetHistoricCandlesRequest{
		Instrument: instrumentId,
		Interval:   pb.CandleInterval_CANDLE_INTERVAL_1_MIN,
		From:       time.Now().Add(-2 * 30 * 24 * time.Hour),
		To:         time.Now(),
		File:       true,
		FileName:   "historical_data/" + instrumentId + ".csv",
	})
	if err != nil {
		logger.Errorf("Can not get historical data: %v", err.Error())
	}
	return err
}

func getLastPrice(client *investgo.MarketDataServiceClient, instrumentId string, logger investgo.Logger) (float64, error) {
	logger.Infof("Sent GetLastPrices request")
	lastPriceResp, err := client.GetLastPrices([]string{instrumentId})
	logger.Infof("Got response for GetLastPrices request")
	if err != nil {
		logger.Errorf("Can't get last price: %v", err.Error())
		return -1.0, err
	}
	lp := lastPriceResp.GetLastPrices()
	return lp[0].GetPrice().ToFloat(), nil
}

func getLastPriceAndVolume(client *investgo.Client, instrumentId string, requestCounter *uint64, logger investgo.Logger) (RequestToPredict, error) {
	atomic.AddUint64(requestCounter, 1)
	MarketDataService := client.NewMarketDataServiceClient()
	logger.Infof("Sent getLastPriceAndVolume request")
	candlesResp, err := MarketDataService.GetCandles(instrumentId, pb.CandleInterval_CANDLE_INTERVAL_1_MIN, time.Now().Add(-1*time.Minute), time.Now())
	logger.Infof("Got response for getLastPriceAndVolume request")
	if err != nil {
		logger.Errorf("Can't get last price and volume: %v", err.Error())
	} else {
		if len(candlesResp.GetCandles()) == 0 {
			logger.Infof("There are no candles/no prices. Error response in getLastPriceAndVolume: %v", candlesResp.String())
			return RequestToPredict{}, errors.New("got zero volume and close price")
		}
		candle := candlesResp.GetCandles()[0]
		if candle.GetVolume() == 0 && candle.GetClose().ToFloat() == 0 {
			logger.Infof("Got zero volume and close price. Error response in getLastPriceAndVolume: %v", candlesResp.String())
			return RequestToPredict{}, errors.New("got zero volume and close price")
		}
		//logger.Infof("PRICE:VOLUME: candle number %d, high price = %v, volume = %v, time = %v, is complete = %v\n", i, candle.GetHigh().ToFloat(), candle.GetVolume(), candle.GetTime().AsTime(), candle.GetIsComplete())
		return RequestToPredict{
			ReqId:    *requestCounter,
			Datetime: candle.GetTime().AsTime(),
			Open:     candle.GetOpen().ToFloat(),
			High:     candle.GetHigh().ToFloat(),
			Low:      candle.GetLow().ToFloat(),
			Close:    candle.GetClose().ToFloat(),
			AdjClose: candle.GetClose().ToFloat(),
			Volume:   candle.GetVolume(),
		}, nil

	}
	return RequestToPredict{}, err
}

func buy(ordersService *investgo.OrdersServiceClient, instrumentId string, accountId string, quantity int64, logger investgo.Logger) (int64, int64, float64, error) {
	logger.Infof("Sent buy request")
	buyResp, err := ordersService.Buy(&investgo.PostOrderRequestShort{
		InstrumentId: instrumentId,
		Quantity:     quantity,
		Price:        nil,
		AccountId:    accountId,
		OrderType:    pb.OrderType_ORDER_TYPE_MARKET,
		OrderId:      investgo.CreateUid(),
	})
	logger.Infof("Got response for GetCandles request")
	if err != nil {
		logger.Errorf("Failed to BUY: error = %v, headers = %v\n", err.Error(), investgo.MessageFromHeader(buyResp.GetHeader()))
		return -1, -1, -1, err
	}
	logger.Infof("Executed BUY: order status = %v\n", buyResp.GetExecutionReportStatus().String())
	return buyResp.GetLotsExecuted(), buyResp.GetLotsRequested(), buyResp.GetExecutedOrderPrice().ToFloat(), nil
}

func getAllPositions(operationsService *investgo.OperationsServiceClient, accountId string, logger investgo.Logger) ([]Position, float64, error) {
	logger.Infof("Sent getAllPositions request")
	positionsResp, err := operationsService.GetPositions(accountId)
	logger.Infof("Got response for getAllPositions request")
	if err != nil {
		logger.Errorf("Can't get all positions: %v", err.Error())
		return nil, 0, err
	}
	positions := positionsResp.GetSecurities()
	poss := make([]Position, len(positions))
	for i, position := range positions {
		logger.Infof("position number %v, uid = %v, balance = %v, figi = %v, instrument_type = %v", i, position.GetInstrumentUid(), position.GetBalance(), position.GetFigi(), position.GetInstrumentType())
		poss[i] = Position{Balance: position.GetBalance(), Id: position.GetInstrumentUid()}
	}
	return poss, positionsResp.GetMoney()[0].ToFloat(), nil
}

func getCurrentBalance(operationsService *investgo.OperationsServiceClient, accountId string, logger investgo.Logger) (float64, error) {
	logger.Infof("Sent getCurrentBalance request")
	positionsResp, err := operationsService.GetPositions(accountId)
	logger.Infof("Got response for getCurrentBalance request")
	if err != nil {
		logger.Errorf("Can't get current balance: %v", err.Error())
		return -1, err
	}
	currentBalance := positionsResp.GetMoney()[0].ToFloat()
	logger.Infof("Got current balance: %v", currentBalance)
	return currentBalance, nil
}

func sell(ordersService *investgo.OrdersServiceClient, instrumentId string, accountId string, quantity int64, logger investgo.Logger) (int64, int64, float64, error) {
	logger.Infof("Sent sell request")
	sellResp, err := ordersService.Sell(&investgo.PostOrderRequestShort{
		InstrumentId: instrumentId,
		Quantity:     quantity,
		Price:        nil,
		AccountId:    accountId,
		OrderType:    pb.OrderType_ORDER_TYPE_MARKET,
		OrderId:      investgo.CreateUid(),
	})
	logger.Infof("Got response for sell request")
	if err != nil {
		logger.Errorf("Failed to SELL: error = %v, headers = %v\n", err.Error(), investgo.MessageFromHeader(sellResp.GetHeader()))
		return -1, -1, -1, err
	}
	logger.Infof("Executed SELL: order status = %v", sellResp.GetExecutionReportStatus().String())
	return sellResp.GetLotsExecuted(), sellResp.GetLotsRequested(), sellResp.GetExecutedOrderPrice().ToFloat(), nil
}
