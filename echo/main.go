package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
//
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration

type Response events.APIGatewayProxyResponse
type Request events.APIGatewayProxyRequest

var (
	ENDPOINT   = os.Getenv("CRYPTOWATCH")
	OHLCSTRUCT = [6]string{"closeTime", "openPrice", "highPrice", "lowPrice", "closePrice", "volume"}
)

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(request Request) (Response, error) {
	timestamp := int(time.Now().Unix())
	target := fmt.Sprintf("%s/markets/bitfinex/btcusd/ohlc?periods=1800&before=%d&after=%d", ENDPOINT, timestamp, timestamp)
	r, err := http.Get(target)
	if err != nil {
		return Response{StatusCode: 404, Body: err.Error()}, nil
	}
	defer r.Body.Close()
	rBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err.Error())
	}
	rMap := map[string]map[string]interface{}{
		"result":    make(map[string]interface{}),
		"allowance": make(map[string]interface{}),
	}
	err = json.Unmarshal(rBody, &rMap)
	if err != nil {
		log.Printf("Unmarshal failed:%s", err.Error())
	}
	info := rMap["result"]["1800"].([]interface{})[0]
	OHLC := map[string]float64{}
	fmt.Println(info)
	for index, value := range OHLCSTRUCT {
		OHLC[value] = info.([]interface{})[index].(float64)
	}

	// Marshall that data into a map of AttributeValue object
	av, err := dynamodbattribute.MarshalMap(OHLC)
	if err != nil {
		log.Println(err.Error())
	}

	// Create DynamoDB client
	sess, err := session.NewSession(&aws.Config{})
	svc := dynamodb.New(sess)
	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String("BTC_30m"),
	}

	_, err = svc.PutItem(input)

	if err != nil {
		log.Println("Got error calling PutItem:")
		log.Println(err.Error())
		return Response{StatusCode: 500, Body: err.Error()}, nil
	}
	result := map[string]interface{}{}
	priceChange := math.Abs((OHLC["closePrice"] - OHLC["openPrice"]) / OHLC["openPrice"])
	if priceChange > 0.05 {
		result["message"] = fmt.Sprintf("[PRICE CHANGE] Notification:%.2f%", priceChange*100)
	} else {
		result["message"] = "Nothing happened"
	}
	for key, value := range OHLC {
		result[key] = value
	}
	body, _ := json.Marshal(result)
	resp := Response{
		StatusCode:      200,
		IsBase64Encoded: false,
		Body:            string(body),
		Headers: map[string]string{
			"Content-Type":           "application/json",
			"X-MyCompany-Func-Reply": "echo-handler",
		},
	}
	return resp, nil
}

func main() {
	lambda.Start(Handler)
}
