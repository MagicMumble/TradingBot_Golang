package main

import (
	"context"
	"fmt"
	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// TODO: do I need shared config for the python and Go parts?
// конфиг, развернуть на стенде, пуш на гитхаб (+версионность, тэги), добавить реальный счет и деньги, левередж транзакций, смотреть чтобы файлы логов не переполнялись
// нужен другой токен? метрика живости стенда
// used with v1 api

func main() {
	var requestCounter uint64

	config := investgo.Config{
		EndPoint:                      endPointSandbox,
		Token:                         tokenSandbox,
		AppName:                       "invest-api-go-sdk",
		AccountId:                     "",
		DisableResourceExhaustedRetry: false,
		DisableAllRetry:               false,
		MaxRetries:                    3,
	}

	requestURL := fmt.Sprintf("http://localhost:%v/data", serverPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{fmt.Sprintf("./logs/%s_stats.log", time.Now().Format("2006_January_02")), "stderr"}
	zapConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime)
	zapConfig.EncoderConfig.TimeKey = "time"
	l, err := zapConfig.Build()
	logger := l.Sugar()
	defer func() {
		err := logger.Sync()
		if err != nil {
			log.Printf(err.Error())
		}
	}()
	if err != nil {
		log.Fatalf("logger creating error %v", err)
	}
	client, err := investgo.NewClient(ctx, config, logger)
	if err != nil {
		logger.Fatalf("client creating error %v", err.Error())
	}
	defer func() {
		logger.Infof("closing client connection")
		err := client.Stop()
		if err != nil {
			logger.Errorf("client shutdown error %v", err.Error())
		}
	}()

	// сервис песочницы нужен лишь для управления счетами песочницы и пополнения баланса
	// остальной функционал доступен через обычные сервисы, но с эндпоинтом песочницы
	sandboxService := client.NewSandboxServiceClient()

	// TODO: to work with real money I need a real account
	accId, err := getAccountId(client, sandboxService, logger)
	if err != nil {
		return
	}

	// пополняем счет песочницы на 100 000 рублей
	//depositMoney(sandboxService, accId, 100000, logger)

	// TCS - same but in dollars
	id_TCSG, err := getInstrumentId(client, logger, "TCSG")
	if err != nil {
		return
	}

	interruptSignalChan := make(chan os.Signal)
	signal.Notify(interruptSignalChan, os.Interrupt, syscall.SIGTERM)

	// change to 1 Minute
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	var wg sync.WaitGroup
	actions := make(chan int, 10)
	wg.Add(1)
	go func() {
		// default - true
		exchangeClosed := true
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// in case the response is sent to the closed channel
				logger.Infof("Recovered in trading strategy: %v", r)
			}
		}()

		for {
			select {
			case <-interruptSignalChan:
				logger.Infof("Caught interrupt signal: Close actions channel")
				close(actions)
				return
			case <-ticker.C:
				// 0 - Sunday, 6 - Saturday
				if weekday, hour, minute := getMoscowTime(); !(weekday != 0 && weekday != 6 && hour >= openMoscowHour && hour*60+minute < closeMoscowHour*60+closeMoscowMinute) {
					if !exchangeClosed {
						logger.Infof("Exchange is closed for today.")
						actions <- 4
						exchangeClosed = true
					}
					break
				}
				if exchangeClosed {
					logger.Infof("Exchange is open now.")
					wg.Add(1)
					go startStrategy(actions, client, accId, id_TCSG, &wg)
					exchangeClosed = false
				}
				request, err := getLastPriceAndVolume(client, id_TCSG, &requestCounter, logger)
				if err != nil {
					logger.Infof("Skipped one cycle stage")
					continue
				}
				logger.Infof("Got price and volume from exchange! Volume = %v and price = %v\n", request.Volume, request.Close)

				// TODO: should check the request/response ids!
				response, err := send_request(request, requestURL, &requestCounter, logger)
				if err != nil {
					logger.Errorf("Error happened on the Python server side")
					continue
				}

				actions <- response.Action
			}
		}
	}()
	wg.Wait()
}

func getMoscowTime() (int, int, int) {
	local := time.Now().UTC()
	location, err := time.LoadLocation("Europe/Moscow")
	if err == nil {
		local = local.In(location)
	}
	return int(local.Weekday()), local.Hour(), local.Minute()
}
