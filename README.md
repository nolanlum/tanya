# tanya [![Build Status](https://travis-ci.org/nolanlum/tanya.svg?branch=master)](https://travis-ci.org/nolanlum/tanya)
slack irc gateway in a world with no generics

## Running tanya
Make sure your `$GOPATH\bin` is in your `$PATH` and simply run `tanya`. If you haven't created a configuration file yet, it will prompt you to log into Slack and automatically grab an API token. An IRC server will then start listening on `:6667` by default.

Tanya will report unhandled events from the Slack RTM event stream via stderr, and select error and status messages are also sent to all connected IRC clients via the `*tanya` virtual user. (Patches welcome for unhandled RTM events.)
