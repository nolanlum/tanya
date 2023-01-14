package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"syscall"

	"github.com/slack-go/slack"
	"golang.org/x/term"
)

type findTeamResponseFull struct {
	SSO    bool   `json:"sso"`
	TeamID string `json:"team_id"`
	slack.SlackResponse
}

type loginResponseFull struct {
	Token string `json:"token"`
	slack.SlackResponse
}

// GetSlackToken interactively prompts the user and obtains a slack token
// by authenticating against the Slack API.
func GetSlackToken() (string, error) {
	var domain, email string

	fmt.Print("Team domain (*.slack.com): ")
	fmt.Scanln(&domain)

	resp, err := http.PostForm("https://slack.com/api/auth.findTeam", url.Values{"domain": {domain}})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var findTeamResponse findTeamResponseFull
	err = json.Unmarshal(body, &findTeamResponse)
	if err != nil {
		return "", err
	}
	if findTeamResponse.SSO {
		return "", errors.New("SSO teams not yet supported")
	}

	fmt.Print("Slack email: ")
	fmt.Scanln(&email)

	fmt.Print("Slack password: ")
	passwordBytes, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Println("")

	password := string(passwordBytes)

	resp, err = http.PostForm("https://slack.com/api/auth.signin",
		url.Values{"team": {findTeamResponse.TeamID}, "email": {email}, "password": {password}})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var loginResponse loginResponseFull
	err = json.Unmarshal(body, &loginResponse)
	if err != nil {
		return "", err
	}

	if !loginResponse.Ok {
		return "", errors.New(loginResponse.Error)
	}

	return loginResponse.Token, nil
}
