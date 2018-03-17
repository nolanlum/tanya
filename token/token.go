package token

import (
	"encoding/json"
	"syscall"
)
import "errors"
import "fmt"
import "io/ioutil"
import "net/http"
import "net/url"

import "github.com/nlopes/slack"
import "golang.org/x/crypto/ssh/terminal"

type findTeamResponseFull struct {
	SSO    bool   `json:"sso"`
	TeamID string `json:"team_id"`
	slack.SlackResponse
}

type LoginResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user"`
	TeamID string `json:"team"`
	slack.SlackResponse
}

// DoSlackLogin interactively prompts the user and obtains a slack token
// by authenticating against the Slack API.
func DoSlackLogin() (*LoginResponse, error) {
	var domain, email string

	fmt.Print("Team domain (*.slack.com): ")
	fmt.Scanln(&domain)

	resp, err := http.PostForm("https://slack.com/api/auth.findTeam", url.Values{"domain": {domain}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var findTeamResponse findTeamResponseFull
	err = json.Unmarshal(body, &findTeamResponse)
	if err != nil {
		return nil, err
	}
	if findTeamResponse.SSO {
		return nil, errors.New("SSO teams not yet supported")
	}

	fmt.Print("Slack email: ")
	fmt.Scanln(&email)

	fmt.Print("Slack password: ")
	passwordBytes, _ := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Println("")

	password := string(passwordBytes)

	resp, err = http.PostForm("https://slack.com/api/auth.signin",
		url.Values{"team": {findTeamResponse.TeamID}, "email": {email}, "password": {password}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var loginResponse LoginResponse
	err = json.Unmarshal(body, &loginResponse)
	if err != nil {
		return nil, err
	}

	if !loginResponse.Ok {
		return nil, errors.New(loginResponse.Error)
	}

	return &loginResponse, nil
}
