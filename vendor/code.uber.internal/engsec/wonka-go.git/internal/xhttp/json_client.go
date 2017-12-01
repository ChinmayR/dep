package xhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

const (
	// MIMETypeApplicationJSON defines the MIME type for application/json content.
	MIMETypeApplicationJSON = "application/json"
)

// CallOptions represents per-request options available for users to tweak.
type CallOptions struct {
	// CloseRequest indicates whether the request should be closed. If true, the
	// connection will not be reused.
	CloseRequest bool
	// Headers typically store Uber specific headers such as 'X-Uber-Source'.
	// Note that header entry here will replace existing header in the request.
	Headers map[string]string
}

// ResponseError is a server-generated error
type ResponseError struct {
	StatusCode   int
	ResponseBody []byte
}

func (e ResponseError) Error() string {
	return fmt.Sprintf("%d error from server: %s", e.StatusCode, e.ResponseBody)
}

// PostJSON posts a JSON document and expects to get a JSON response
func PostJSON(ctx context.Context, client *Client, url string, in interface{}, out interface{}, options *CallOptions) error {
	return roundTripJSONBody(ctx, client, "POST", url, in, out, options)
}

// GetJSON fetches a JSON document from a server using the given client
func GetJSON(ctx context.Context, client *Client, url string, out interface{}, options *CallOptions) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", MIMETypeApplicationJSON)
	return roundTripJSON(ctx, client, req, out, options)
}

func roundTripJSONBody(ctx context.Context, client *Client, method, url string, in interface{}, out interface{}, options *CallOptions) error {
	reqBody, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", MIMETypeApplicationJSON)
	req.Header.Add("Accept", MIMETypeApplicationJSON)
	return roundTripJSON(ctx, client, req, out, options)
}

func roundTripJSON(ctx context.Context, client *Client, req *http.Request, out interface{}, options *CallOptions) error {

	if options != nil {
		req.Close = options.CloseRequest

		for key, value := range options.Headers {
			req.Header.Set(key, value)
		}
	}

	if client == nil {
		client = DefaultClient
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode > 399 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		return ResponseError{
			StatusCode:   resp.StatusCode,
			ResponseBody: body,
		}
	}

	if out == nil {
		io.Copy(ioutil.Discard, resp.Body)
		return nil
	}

	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(out)
}
