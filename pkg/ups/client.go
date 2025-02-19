// MIT License
//
// Copyright (c) 2024 UPS-API
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ups

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	timeoutDuration       = 30 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	idleConnTimeout       = 10 * time.Second
	responseHeaderTimeout = 10 * time.Second
	expectContinueTimeout = 10 * time.Second
	tokenUrl              = "https://onlinetools.ups.com/security/v1/oauth/token"
	timedOut              = `{"response":{"errors":[{"code":"10500","message":"Request Timed out."}]}}`
	internalServerError   = `{"response":{"errors":[{"code":"10500","message":"Internal server error"}]}}`
)

func setHttpClientTimeouts(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		return &http.Client{
			Timeout: timeoutDuration,
			Transport: &http.Transport{
				TLSHandshakeTimeout:   tlsHandshakeTimeout,
				IdleConnTimeout:       idleConnTimeout,
				ResponseHeaderTimeout: responseHeaderTimeout,
				ExpectContinueTimeout: expectContinueTimeout,
			},
		}
	}

	httpClient.Timeout = timeoutDuration

	if transport, ok := httpClient.Transport.(*http.Transport); ok {
		transport.TLSHandshakeTimeout = tlsHandshakeTimeout
		transport.IdleConnTimeout = idleConnTimeout
		transport.ResponseHeaderTimeout = responseHeaderTimeout
		transport.ExpectContinueTimeout = expectContinueTimeout
	} else {
		httpClient.Transport = &http.Transport{
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			IdleConnTimeout:       idleConnTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		}
	}
	return httpClient
}

func GetAccessToken(httpClient *http.Client, clientId string, clientSecret string, headers map[string]string, customClaims map[string]string) UpsOauthResponse {
	var hClient *http.Client = setHttpClientTimeouts(httpClient)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("scope", "public")

	for keys := range customClaims {
		claim := "{\"" + keys + "\":\"" + customClaims[keys] + "\"}"
		body.Add("custom_claims", claim)
	}
	encodedData := body.Encode()

	req, err := http.NewRequest(http.MethodPost, tokenUrl, strings.NewReader(encodedData))
	if err != nil {
		return apiErrorResponse(internalServerError)
	}

	req.SetBasicAuth(clientId, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for keys := range headers {
		req.Header.Set(keys, headers[keys])
	}

	res, err := hClient.Do(req)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return apiErrorResponse(timedOut)
		}
		return apiErrorResponse(err.Error())
	}

	defer res.Body.Close()

	response, err := io.ReadAll(res.Body)
	if err != nil {
		return apiErrorResponse(internalServerError)
	}

	respString := string(response)
	var data TokenInfo
	errr := json.Unmarshal([]byte(respString), &data)
	if errr != nil {
		return apiErrorResponse(internalServerError)
	}

	if !(res.StatusCode >= 200 && res.StatusCode <= 299) {
		return apiErrorResponse(respString)
	}

	upsResponse := UpsOauthResponse{
		Response: data,
		Error:    "",
	}

	return upsResponse
}

func apiErrorResponse(errorResponse string) UpsOauthResponse {
	response := UpsOauthResponse{
		Response: TokenInfo{},
		Error:    errorResponse,
	}
	return response
}

type TokenInfo struct {
	Issued_at    string `json: "issued_at`
	Token_type   string `json: "token_type`
	Client_id    string `json: "client_id`
	Access_token string `json: "access_token`
	Expires_in   string `json: "expires_in`
	Status       string `json: "status`
}

type UpsOauthResponse struct {
	Response TokenInfo
	Error    string
}
