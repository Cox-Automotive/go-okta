package okta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

// Client to access okta
type Client struct {
	client        *http.Client
	org           string
	Url           string
	ApiToken      string
	SessionCookie *http.Cookie
}

// errorResponse is an error wrapper for the okta response
type errorResponse struct {
	HTTPCode int
	Response ErrorResponse
	Endpoint string
}

func (e *errorResponse) Error() string {
	return fmt.Sprintf("Error hitting api endpoint %s %s", e.Endpoint, e.Response.ErrorCode)
}

// NewClient object for calling okta
func NewClient(org string) *Client {
	client := Client{
		client: &http.Client{},
		org:    org,
		Url:    "okta.com",
	}

	return &client
}

// Authenticate with okta using username and password
func (c *Client) Authenticate(username, password string) (*AuthnResponse, error) {
	var request = &AuthnRequest{
		Username: username,
		Password: password,
	}

	var response = &AuthnResponse{}
	err, _ := c.call("authn", "POST", request, response)
	return response, err
}

// Session takes a session token and returns a session, the ID is stored
// as a cookie so it can be consumed by this library and its clients.
func (c *Client) Session(sessionToken string) (*SessionResponse, error) {
	var request = &SessionRequest{
		SessionToken: sessionToken,
	}

	var response = &SessionResponse{}
	err, _ := c.call("sessions", "POST", request, response)
	if err == nil {
		c.SessionCookie = &http.Cookie{
			Name:     "sid",
			Value:    response.ID,
			Path:     "/",
			Domain:   c.org + "." + c.Url,
			Secure:   true,
			HttpOnly: true,
		}
	}
	return response, err
}

// User takes a user id and returns data about that user
func (c *Client) User(userID string) (*User, error) {

	var response = &User{}
	err, _ := c.call("users/"+userID, "GET", nil, response)
	return response, err
}

// Groups takes a user id and returns the groups the user belongs to
func (c *Client) Groups(userID string) (*[]Group, error) {

	var response = &[]Group{}
	var nextLink = "users/"+userID+"/groups?limit=200"

	for {
		var resp = &[]Group{}
		err, link := c.call(nextLink, "GET", nil, resp)

		if err != nil {
			return resp, err
		}

		*response = append(*response, *resp...)

		parts := strings.Split(link, ";")
		nextLink = strings.Replace(parts[0], fmt.Sprintf("<https://%s.okta.com/api/v1/", c.org), "", -1)
		nextLink = strings.Replace(nextLink, ">", "", -1)

		if nextLink == "" {
			break
		}

		fmt.Println("go next link")
	}

	return response, nil
}

func (c *Client) AppLinks(userID string, appName string) (*AppLinks, error) {
	u := "users/" + userID + "/appLinks"

	if len(appName) > 0 {
		v := &url.Values{}
		v.Add("filter", fmt.Sprintf(`appName eq "%s"`, appName))
		u += "?" + v.Encode()
	}

	var response = &AppLinks{}
	err, _ := c.call(u, "GET", nil, response)
	return response, err
}

func (c *Client) call(endpoint, method string, request, response interface{}) (error, string) {
	data, _ := json.Marshal(request)
	link := ""

	var url = "https://" + c.org + "." + c.Url + "/api/v1/" + endpoint
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return err, link
	}

	req.Header.Add("Accept", `application/json`)
	req.Header.Add("Content-Type", `application/json`)
	if c.ApiToken != "" {
		req.Header.Add("Authorization", "SSWS "+c.ApiToken)
	}
	if c.SessionCookie != nil {
		req.Header.Add("Cookie", c.SessionCookie.String())
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err, link
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err, link
	}

	if resp.StatusCode == http.StatusOK {
		err := json.Unmarshal(body, &response)
		if err != nil {
			return err, link
		}
	} else {
		var errors ErrorResponse
		err = json.Unmarshal(body, &errors)

		return &errorResponse{
			HTTPCode: resp.StatusCode,
			Response: errors,
			Endpoint: url,
		}, link
	}

	links := resp.Header.Values("Link")
	if links != nil {
		if len(links) == 2 {
			link = links[1]
		}
	}

	return nil, link
}
