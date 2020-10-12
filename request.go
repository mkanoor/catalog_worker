package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"sync"
)

// JobParam stores the single parameter set for a job
type JobParam struct {
	Method                 string                 `json:"method"`
	HrefSlug               string                 `json:"href_slug"`
	FetchAllPages          bool                   `json:"fetch_all_pages"`
	Params                 map[string]interface{} `json:"params"`
	AcceptEncoding         string                 `json:"accept_encoding"`
	ApplyFilter            interface{}            `json:"apply_filter"`
	RefreshIntervalSeconds int64                  `json:"refresh_interval_seconds"`
}

// PayloadStruct contains a collection of JobParam
type PayloadStruct struct {
	Jobs []JobParam `json:"jobs"`
}

// RequestMessage is the message format sent from the
// Platform controller to the Receptor
type RequestMessage struct {
	Account   string        `json:"account"`
	Sender    string        `json:"sender"`
	MessageID string        `json:"message_id"`
	Payload   PayloadStruct `json:"payload"`
}

// RequestHandler interface allows for easy mocking during testing
type RequestHandler interface {
	getRequest(r io.Reader) ([]byte, error)
	parseRequest(b []byte) (*RequestMessage, error)
	processRequest(req *RequestMessage, config CatalogConfig, wh WorkHandler)
}

// DefaultRequestHandler implements the 3 RequestHandler methods
type DefaultRequestHandler struct {
}

// getRequest get data from the Receptor via Stdin
func (drh *DefaultRequestHandler) getRequest(ior io.Reader) ([]byte, error) {
	reader := bufio.NewReader(ior)
	line, err := reader.ReadString('\n')
	if len(line) > 0 && err != nil && err == io.EOF {
		line = strings.TrimSuffix(line, "\n")
	} else if err != nil {
		log.Println(err)
		return nil, err
	}

	line = strings.TrimSuffix(line, "\n")
	return []byte(line), nil
}

// Parse the request into RequestMessage
func (drh *DefaultRequestHandler) parseRequest(b []byte) (*RequestMessage, error) {
	req := RequestMessage{}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	err := decoder.Decode(&req)
	if err != nil {
		log.Errorf("Error decoding json %v", err)
		return nil, err
	}
	return &req, nil
}

// Process the incoming request, start a go routine responder that
// can send ack responses to the receptor. Then for each of the JobParam
// start a go routine to do the work.
func (drh *DefaultRequestHandler) processRequest(req *RequestMessage, config CatalogConfig, wh WorkHandler) {
	var workerGroup sync.WaitGroup
	var responderGroup sync.WaitGroup
	outputChannel := make(chan ResponsePayload)
	rs := &Responder{
		Output: os.Stdout,
		header: ResponseHeader{
			Account:      req.Account,
			Sender:       req.Sender,
			InResponseTo: req.MessageID,
		},
	}
	responderGroup.Add(1)
	log.Debug("Starting Responder")
	go startResponder(&responderGroup, rs, outputChannel)

	log.Debug("Starting Workers")
	req.dispatch(config, &workerGroup, wh, outputChannel)

	workerGroup.Wait()
	outputChannel <- ResponsePayload{messageType: "eof"}
	responderGroup.Wait()
}

// Foreach of the JobParam in the payload we can start a go routine
// to handle the request independently
func (req *RequestMessage) dispatch(config CatalogConfig, workerGroup *sync.WaitGroup, wh WorkHandler, outputChannel chan ResponsePayload) {
	for _, v := range req.Payload.Jobs {
		workerGroup.Add(1)
		log.Debugf("Job Input Data %v", v)
		go startWorker(config, workerGroup, wh, outputChannel, v)
	}
}

// Start a work
func startWorker(config CatalogConfig, wg *sync.WaitGroup, wh WorkHandler, outputChannel chan ResponsePayload, params JobParam) {
	log.Debugf("Worker starting")
	defer log.Debugf("Worker finished")
	defer wg.Done()
	wh.StartWork(&config, params, nil, outputChannel)
}
