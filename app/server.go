package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	DELETE = "DELETE"
	PATCH  = "PATCH"
	GZIP   = "gzip"
)

type RequestParams struct {
	method  string
	path    string
	version string
	sender  string
	headers map[string]string
	reqBody []byte
}

var directoryName = flag.String("directory", "", "the directory to serve files from")

func main() {
	flag.Parse()
	fmt.Println("Logs from your program will appear here!")
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	request := make([]byte, 1024)
	_, err := conn.Read(request)
	if err != nil {
		return
	}
	reqParams, _ := getReqParams(request)
	switch reqParams.method {
	case GET:
		_ = handleGetRequest(reqParams, conn)
		return
	case POST:
		_ = handlePostRequest(reqParams, conn)
	default:
		return
	}
}

func handlePostRequest(reqParams RequestParams, conn net.Conn) error {
	switch reqParams.path {
	case "/":
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return nil
	default:
		reqPathAndValue := strings.Split(reqParams.path, "/")
		switch reqPathAndValue[1] {
		case "files":
			return handlePostFileReq(reqParams, conn, reqPathAndValue)
		default:
			return nil
		}
	}
}

func handlePostFileReq(reqParams RequestParams, conn net.Conn, reqPathAndValue []string) error {
	fmt.Println("Arguments passed -> ", *directoryName)
	if *directoryName == "" {
		fmt.Println("Directory not specified")
		return fmt.Errorf("directory not specified")
	}
	// Get the filename from the path
	fileName := reqPathAndValue[2]
	filePath := filepath.Join(*directoryName, fileName)
	content := reqParams.reqBody
	os.WriteFile(filePath, content, 0644)
	fmt.Println("reqBody recieved -> ", content)
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 201 Created\r\nContent-Type: "+
		"application/octet-stream\r\nContent-Length: %d\r\n\r\n%s", len(content), string(content))))
	return nil
}

func handleGetRequest(reqParams RequestParams, conn net.Conn) error {
	switch reqParams.path {
	case "/":
		return handleHomePageGetRequest(conn)
	default:
		reqPathAndValue := strings.Split(reqParams.path, "/")
		switch reqPathAndValue[1] {
		case "echo":
			return handleGetEchoRequest(conn, reqPathAndValue, reqParams.headers)
		case "user-agent":
			return handleGetUserAgentRequest(reqParams, conn)
		case "files":
			return handleGetFileContentRequest(reqPathAndValue, conn)
		default:
			return handleGetNotFound(conn)
		}
	}
}

func handleGetFileContentRequest(reqPathAndValue []string, conn net.Conn) error {
	fmt.Println("Arguments passed -> ", *directoryName)
	// Check for a valid directory argument
	if *directoryName == "" {
		fmt.Println("Directory not specified")
		return fmt.Errorf("directory not specified")
	}
	// Get the filename from the path
	fileName := reqPathAndValue[2]
	filePath := filepath.Join(*directoryName, fileName)
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s",
			0, make([]byte, 0))))
		return nil
	}
	defer file.Close()
	// Read the contents of the file
	fileContent := bufio.NewReader(file)
	fileData := make([]byte, 65507)
	contentLength, err := fileContent.Read(fileData)
	if err != nil {
		return err
	}
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s",
		contentLength, fileData)))
	return nil
}

func handleGetNotFound(conn net.Conn) error {
	conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	return nil
}

func handleGetUserAgentRequest(reqParams RequestParams, conn net.Conn) error {
	reqHeader := reqParams.headers["User-Agent"]
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(reqHeader), reqHeader)))
	return nil
}

func handleGetEchoRequest(conn net.Conn, reqPathAndValue []string, headers map[string]string) error {
	supportedEncodings := []string{GZIP}
	encodingsSent, exists := headers["Accept-Encoding"]
	encodingsList := strings.Split(encodingsSent, ", ")
	if exists {
		for _, valSent := range encodingsList {
			for _, valSupported := range supportedEncodings {
				if strings.EqualFold(valSupported, valSent) {
					compressedContent := compressContent(reqPathAndValue[2]).String()
					conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
						valSupported, len(compressedContent), compressedContent)))
					return nil
				}
			}
		}
	}
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(reqPathAndValue[2]), reqPathAndValue[2])))
	return nil
}

func compressContent(original string) *bytes.Buffer {
	// Compress the text string
	var compressedBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedBuffer)
	_, err := gzipWriter.Write([]byte(original))
	if err != nil {
		log.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		log.Fatal(err)
	}
	return &compressedBuffer
}

func handleHomePageGetRequest(conn net.Conn) error {
	conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	return nil
}

func extractRequestBody(data string, reqLen int) []byte {
	reqBody := make([]byte, 0)
	for i := range reqLen {
		reqBody = append(reqBody, data[i])
	}
	return reqBody
}

func getReqParams(request []byte) (RequestParams, error) {
	requestString := string(request)
	reqInfo := strings.Split(requestString, "\r\n\r\n")
	fields := strings.Split(reqInfo[0], "\r\n")
	// extract request type and path and http version
	reqDetails := strings.Split(fields[0], " ")
	hostDetails := strings.Split(fields[1], " ")
	reqHeaders := getHeaders(fields)
	reqLen, err := strconv.Atoi(reqHeaders["Content-Length"])
	if err != nil {
		fmt.Println("Request body was not passed")
		reqLen = 0
	}
	reqBodyData := extractRequestBody(reqInfo[1], reqLen)
	return RequestParams{
		method:  reqDetails[0],
		path:    reqDetails[1],
		version: reqDetails[2],
		sender:  strings.TrimSpace(hostDetails[1]),
		headers: reqHeaders,
		reqBody: reqBodyData,
	}, nil
}

func getHeaders(reqDetails []string) map[string]string {
	headers := make(map[string]string)
	for index, elem := range reqDetails {
		if index == 0 || index == 1 {
			continue
		} else if elem == "" || elem == " " {
			break
		}
		temp := strings.Split(elem, ":")
		headerName := strings.TrimSpace(temp[0])
		headerValue := strings.TrimSpace(temp[1])
		headers[headerName] = headerValue
	}
	return headers
}
