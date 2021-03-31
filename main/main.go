package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Stock data response
type StockData struct {
	CikStr int    `json:"cik_str"`
	Ticker string `json:"ticker"`
	Title  string `json:"title"`
}

func fetchStocks() map[string]StockData {
	resp, err := http.Get("https://www.sec.gov/files/company_tickers.json")
	if err != nil {
		log.Println(err)
	}
	// Read body then convert to string
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	sb := string(body)

	defer resp.Body.Close()
	var cResp map[string]StockData
	// Parse the json string
	if json.Unmarshal([]byte(sb), &cResp); err != nil {
		log.Println(err)
	}

	return cResp
}

func writeJsonfile(stocks map[string]StockData) {
	b, _ := json.MarshalIndent(stocks, "", " ")

	_ = ioutil.WriteFile("/tmp/tickers.json", b, 0644)
}

// AddFileToS3 will upload a single file to S3
// and will set file info like content type and encryption on the uploaded file.
func AddFileToS3(s *session.Session, fileName string) error {
	// Open the file for use
	file, err := os.Open("/tmp/" + fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file size and read the file content into a buffer
	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	file.Read(buffer)

	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(os.Getenv("S3_FILES_BUCKET")),
		Key:                  aws.String(fileName),
		ACL:                  aws.String("private"),
		Body:                 bytes.NewReader(buffer),
		ContentLength:        aws.Int64(size),
		ContentType:          aws.String(http.DetectContentType(buffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})

	return err
}

func uploadToS3() {
	fileName := "tickers.json"
	// Create a single AWS session (we can re use this if we're uploading many files)
	s, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	if err != nil {
		log.Fatal(err)
	}

	err = AddFileToS3(s, fileName)
	if err != nil {
		log.Fatal(err)
	}
}

func writeToDB(stocks map[string]StockData) {
	tableName := "stocks"
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create DynamoDB client
	svc := dynamodb.New(sess)

	for _, item := range stocks {
		av, err := dynamodbattribute.MarshalMap(item)
		if err != nil {
			log.Fatalf("Got error marshalling item: %s", err)
		}

		input := &dynamodb.PutItemInput{
			Item:      av,
			TableName: aws.String(tableName),
		}

		_, err = svc.PutItem(input)
		if err != nil {
			log.Fatalf("Got error calling PutItem: %s", err)
		}
	}
}

func runLambda() {
	log.Println("Fetching stocks list...")
	stocks := fetchStocks()
	log.Println("Writing json file...")
	writeJsonfile(stocks)
	log.Println("Uploading to S3...")
	uploadToS3()
	log.Println("Write to DB...")
	writeToDB(stocks)
}

func main() {
	env := os.Getenv("ENV")
	if env == "local" {
		stocks := fetchStocks()
		writeJsonfile(stocks)
	} else {
		lambda.Start(runLambda)
	}

}
