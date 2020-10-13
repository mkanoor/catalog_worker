package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// WorkHandler is an interface to start a worker
type WorkHandler interface {
	StartWork(config *CatalogConfig, params JobParam, client *http.Client, channel chan ResponsePayload) error
}

// DefaultAPIWorker is struct to start a worker
type DefaultAPIWorker struct {
}

// StartWork can be started as a go routine to start a unit of work based on a given JobParam
// The responses are sent to the Responder's channel so that it can rely it to the Receptor
func (aw *DefaultAPIWorker) StartWork(config *CatalogConfig, params JobParam, client *http.Client, channel chan ResponsePayload) error {
	w := &WorkUnit{outputChannel: channel}
	w.setConfig(config)
	w.setJobParameters(params)
	err := w.setURL()
	if err != nil {
		log.Error(err)
		return err
	}
	w.setClient(client)
	return w.dispatch()
}

// WorkUnit is a data struct to store a single unit of work
type WorkUnit struct {
	config        *CatalogConfig
	hostURL       *url.URL
	client        *http.Client
	input         *JobParam
	outputChannel chan ResponsePayload
	filterValue   *Filter
	parsedURL     *url.URL
	parsedValues  url.Values
}

func (w *WorkUnit) setConfig(p *CatalogConfig) {
	w.config = p
	w.parseHost(p.URL)
}

func (w *WorkUnit) setJobParameters(data JobParam) {
	if data.ApplyFilter != nil {
		fltr := Filter{}
		fltr.Parse(data.ApplyFilter)
		w.filterValue = &fltr
	}
	if data.Params == nil {
		data.Params = make(map[string]interface{})
	}
	w.input = &data
}

func (w *WorkUnit) setClient(c *http.Client) error {
	if c == nil {
		var tr *http.Transport
		if w.config.SkipVerifyCertificate {
			config := &tls.Config{InsecureSkipVerify: true}
			tr = &http.Transport{TLSClientConfig: config}
		}
		w.client = &http.Client{Transport: tr}
	} else {
		w.client = c
	}
	return nil
}

func (w *WorkUnit) dispatch() error {
	var err error
	switch strings.ToLower(w.input.Method) {
	case "get":
		err = w.get()
	case "post":
		err = w.post()
	case "monitor":
		err = w.monitor()
	default:
		err = errors.New("Invalid method received " + w.input.Method)
		w.sendError(err.Error(), 0)
	}
	return err
}

func (w *WorkUnit) setURL() error {
	var err error
	w.parsedURL, err = url.Parse(w.input.HrefSlug)
	if err != nil {
		log.Error(err)
		return err
	}
	w.parsedValues, err = url.ParseQuery(w.parsedURL.RawQuery)
	if err != nil {
		log.Error(err)
		return err
	}
	w.parsedURL.Scheme = w.hostURL.Scheme
	w.parsedURL.Host = w.hostURL.Host
	return nil
}

func (w *WorkUnit) overrideQueryParams(override map[string]interface{}) error {
	for key, element := range override {
		switch v := element.(type) {
		case int64:
			w.parsedValues.Set(key, strconv.FormatInt(element.(int64), 10))
		case string:
			w.parsedValues.Set(key, element.(string))
		case float64:
			w.parsedValues.Set(key, strconv.FormatFloat(element.(float64), 'E', -1, 64))
		case bool:
			w.parsedValues.Set(key, strconv.FormatBool(element.(bool)))
		case json.Number:
			w.parsedValues.Set(key, element.(json.Number).String())
		default:
			log.Infof("I don't know about type %T!\n", v)
		}
	}
	for key, element := range w.parsedValues {
		log.Info("Key:", key, "=>", "Element:", element[0])
	}
	w.parsedURL.RawQuery = w.parsedValues.Encode()
	return nil
}

func (w *WorkUnit) parseHost(host string) error {
	u, err := url.Parse(host)
	if err != nil {
		log.Error(err)
		return err
	}
	w.hostURL = u
	return nil
}

func (w *WorkUnit) getPage() ([]byte, int, error) {
	err := w.overrideQueryParams(w.input.Params)
	if err != nil {
		log.Error(err)
		return nil, 0, err
	}

	req, err := http.NewRequest("GET", w.parsedURL.String(), nil)
	req.Header.Add("Authorization", "Bearer "+w.config.Token)
	resp, err := w.client.Do(req)
	if err != nil {
		log.Error(err)
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return nil, 0, err
	}

	log.Info("GET " + w.parsedURL.String() + " Status " + resp.Status)

	err = w.validateHTTPResponse(resp, body)
	if err != nil {
		return nil, 0, err
	}
	return []byte(body), resp.StatusCode, nil
}

func (w *WorkUnit) validateHTTPResponse(resp *http.Response, body []byte) error {
	if !successHTTPCode(resp.StatusCode) {
		err := errors.New("HTTP GET call failed with " + resp.Status)
		w.sendError(string(body), resp.StatusCode)
		log.Errorf("%v", err)
		return err
	}
	return nil
}

func (w *WorkUnit) post() error {
	b, err := json.Marshal(w.input.Params)
	if err != nil {
		log.Fatal(err)
		return err
	}

	req, err := http.NewRequest("POST", w.parsedURL.String(), bytes.NewBuffer(b))
	req.Header.Add("Authorization", "Bearer "+w.config.Token)
	req.Header.Add("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		log.Error(err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return err
	}
	log.Info("POST " + w.parsedURL.String() + " Status " + resp.Status)
	err = w.validateHTTPResponse(resp, body)
	if err != nil {
		return err
	}
	_, err = w.sendResponse(body, resp.StatusCode)
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (w *WorkUnit) sendResponse(body []byte, status int) (map[string]interface{}, error) {
	jsonBody, err := w.createJSON(body)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	err = w.writePage(jsonBody, status)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return jsonBody, nil
}

func (w *WorkUnit) get() error {

	body, httpStatus, err := w.getPage()
	if err != nil {
		log.Error("Get failed")
		return err
	}

	jsonBody, err := w.sendResponse(body, httpStatus)
	if err != nil {
		log.Error(err)
		return err
	}

	if w.input.FetchAllPages {
		nextPage := jsonBody["next"]
		for page := 2; reflect.TypeOf(nextPage) == reflect.TypeOf("string"); page++ {
			w.input.Params["page"] = strconv.Itoa(page)
			body, httpStatus, err := w.getPage()
			if err != nil {
				log.Error("Get failed")
				return err
			}
			jsonBody, err := w.sendResponse(body, httpStatus)
			if err != nil {
				log.Error(err)
				return err
			}
			nextPage = jsonBody["next"]
		}
	}
	return nil
}

func (w *WorkUnit) monitor() error {

	var completedStatus = []string{"successful", "failed", "error", "canceled"}
	var allKnownStatus = []string{"new", "pending", "waiting", "running", "successful", "failed", "error", "canceled"}
	var body []byte
	var err error
	var httpStatus int
	if w.input.RefreshIntervalSeconds == 0 {
		w.input.RefreshIntervalSeconds = 10
	}
	for {
		body, httpStatus, err = w.getPage()
		if err != nil {
			log.Error("Get failed")
			return err
		}

		jsonBody, err := w.createJSON(body)
		if err != nil {
			log.Error(err)
			return err
		}

		v, ok := jsonBody["status"]
		if !ok {
			err = errors.New("Object does not contain a status attribute")
			w.sendError(err.Error(), 0)
			log.Error(err)
			return err
		}

		status := v.(string)
		if !includes(status, allKnownStatus) {
			err = errors.New("Status: " + status + " is not one of the known status")
			w.sendError(err.Error(), 0)
			log.Error(err)
			return err
		}

		if includes(status, completedStatus) {
			break
		} else {
			time.Sleep(time.Duration(w.input.RefreshIntervalSeconds) * time.Second)
		}
	}

	_, err = w.sendResponse(body, httpStatus)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func includes(s string, values []string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}

func (w *WorkUnit) createJSON(body []byte) (map[string]interface{}, error) {
	var jsonBody map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	err := decoder.Decode(&jsonBody)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if w.filterValue != nil {
		jsonBody, err = w.filterValue.Apply(jsonBody)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	}

	v, ok := jsonBody["artifacts"]
	if ok {
		s, err := sanctifyArtifacts(v.(map[string]interface{}))
		if err != nil {
			log.Error(err)
			return nil, err
		}
		jsonBody["artifacts"] = s
	}
	return jsonBody, nil
}

func (w *WorkUnit) writePage(jsonBody map[string]interface{}, status int) error {
	var bytes []byte
	rd := ResponseData{HrefSlug: w.input.HrefSlug, Status: status}
	bytes, err := json.Marshal(jsonBody)
	if err != nil {
		log.Error(err)
		return err
	}

	if w.input.AcceptEncoding == "gzip" {
		rd.Encoding = "gzip"
		bytes, err = compressBytes(bytes)
		if err != nil {
			log.Error(err)
			return err
		}
		bytes = []byte(base64.StdEncoding.EncodeToString(bytes))
	}
	rd.Body = string(bytes)
	log.Debugf("Sending response for %s", w.input.HrefSlug)
	w.outputChannel <- ResponsePayload{messageType: "data", code: 0, data: rd}
	return nil
}

func (w *WorkUnit) sendError(message string, httpStatus int) error {
	rd := ResponseData{HrefSlug: w.input.HrefSlug, Body: message, Status: httpStatus}
	w.outputChannel <- ResponsePayload{messageType: "data", code: 1, data: rd}
	return nil
}

func compressBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(b)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	err = w.Close()
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return buf.Bytes(), nil
}

func successHTTPCode(code int) bool {
	var validCodes = [...]int{200, 201, 202}
	for _, v := range validCodes {
		if v == code {
			return true
		}
	}
	return false
}
