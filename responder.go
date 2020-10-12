package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"io"
	"sync"
)

// ResponseData struct is the application level data format
type ResponseData struct {
	HrefSlug string `json:"href_slug"`
	Encoding string `json:"encoding"`
	Body     string `json:"body"`
	Status   int    `json:"status"`
}

// ResponsePayload is the internal struct to exchange data between the
// go routines (start worker to the responder)
type ResponsePayload struct {
	messageType string
	data        ResponseData
	code        int
}

// ResponseMessage is the full message format send back to the
// Platform Controller via the Responder go routine
type ResponseMessage struct {
	Account string `json:"account"`
	Sender  string `json:"sender"`
	// MessageType: eof|data
	MessageType  string       `json:"message_type"`
	MessageID    string       `json:"message_id"`
	Payload      ResponseData `json:"payload"`
	Code         int          `json:"code"`
	InResponseTo string       `json:"in_response_to"`
	Serial       int          `json:"serial"`
}

// ResponseHeader contains the values received from Platform controller
type ResponseHeader struct {
	Account      string
	Sender       string
	InResponseTo string
}

// Responder struct stores the data values for the responder
type Responder struct {
	Output       io.Writer
	messageCount int
	header       ResponseHeader
}

// start the Responder as a go routine. It waits for messages coming from the
// different worker go routines and delivers it to the receptor. Once all the jobs
// have submitted the data we get an "EOF" message type which indicates that all
// the workers have finished.
func startResponder(wg *sync.WaitGroup, rs *Responder, channel chan ResponsePayload) {
	defer wg.Done()
	log.Info("Responder has started")
	for {
		pl := <-channel
		log.Info("Read data from channel")
		str, err := rs.createResponse(&pl)
		if err != nil {
			log.Fatalf("Error creating response %v", err)
		}

		n, err := fmt.Fprintf(rs.Output, "%s\n", str)
		if err != nil {
			log.Fatalf("Error writing response %v", err)
		}
		log.Infof("Number of bytes written %d", n)
		if pl.messageType == "eof" {
			break
		}
	}
	log.Info("Finished Responder")
}

// createResponse builds a response payload that can be sent to the
// Receptor by the responder go routine.
func (r *Responder) createResponse(pl *ResponsePayload) (string, error) {

	r.messageCount++
	response := ResponseMessage{
		Account:      r.header.Account,
		Sender:       r.header.Sender,
		MessageType:  pl.messageType,
		MessageID:    uuid.New().String(),
		Payload:      pl.data,
		Code:         pl.code,
		InResponseTo: r.header.InResponseTo,
		Serial:       r.messageCount,
	}

	resp, err := json.Marshal(&response)
	if err != nil {
		log.Error(err)
		return "", err
	}

	return string(resp), nil
}
