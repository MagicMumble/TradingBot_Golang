package main

import (
	"bytes"
	"encoding/json"
	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	"net/http"
)

func send_request(request RequestToPredict, requestURL string, requestCounter *uint64, logger investgo.Logger) (ResponseAction, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		logger.Errorf("Cannot marshal json file to send request to the Python server: " + err.Error())
		return ResponseAction{}, err
	}
	logger.Infof("Marshalled json successfully")
	req, err := http.NewRequest(http.MethodGet, requestURL, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Errorf("Cannot create request to the Python server: " + err.Error())
		return ResponseAction{}, err
	}
	logger.Infof("Created request successfully")
	req.Header.Set("Content-Type", "application/json")
	http_client := &http.Client{}
	resp, err := http_client.Do(req)
	if err != nil {
		logger.Errorf("Cannot send request to the Python server: " + err.Error())
		return ResponseAction{}, err
	}
	logger.Infof("Sent request to the Python server successfully, id = %v", *requestCounter)
	defer resp.Body.Close()
	response := ResponseAction{}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&response)
	if err != nil {
		logger.Errorf("Cannot parse response from the Python server: %v, id = %v", err.Error(), *requestCounter)
		return ResponseAction{}, err
	}
	if response.Error != "" {
		logger.Errorf("Python server was not able to process the request/predict next action: %v, id = %v", response.Error, *requestCounter)
		return ResponseAction{}, err
	}
	logger.Infof("Got response from the Python server! Action = %v, id = %v\n", response.Action, *requestCounter)
	return response, nil
}
