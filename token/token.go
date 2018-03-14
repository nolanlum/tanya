package main

import "encoding/json"
import "fmt"
import "io/ioutil"
import "net/http"
import "net/url"

import "github.com/nlopes/slack"
import "golang.org/x/crypto/ssh/terminal"

type findTeamResponseFull struct {
    SSO bool `json:"sso"`
    TeamID string `json:"team_id"`
    slack.SlackResponse
}

type loginResponseFull struct {
    Token string `json:"token"`
    slack.SlackResponse
}

func main() {
    var domain, email string

    fmt.Print("Team domain (*.slack.com): ")
    fmt.Scanln(&domain)

    resp, err := http.PostForm("https://slack.com/api/auth.findTeam", url.Values{"domain": {domain}})
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }

    var findTeamResponse findTeamResponseFull
    err = json.Unmarshal(body, &findTeamResponse)
    if err != nil {
        panic(err)
    }

    if findTeamResponse.SSO == true {
        panic("SSO teams not yet supported")
    }

    fmt.Print("Slack email: ")
    fmt.Scanln(&email)

    fmt.Print("Slack password: ")
    passwordBytes, _ := terminal.ReadPassword(0)
    fmt.Println("")

    password := string(passwordBytes)

    resp, err = http.PostForm("https://slack.com/api/auth.signin",
        url.Values{"team": {findTeamResponse.TeamID}, "email": {email}, "password": {password}})
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, err = ioutil.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }

    var loginResponse loginResponseFull
    err = json.Unmarshal(body, &loginResponse)
    if err != nil {
        panic(err)
    }

    fmt.Println(loginResponse.Token)
}

