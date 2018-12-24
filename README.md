# tanya [![Build Status](https://travis-ci.org/nolanlum/tanya.svg?branch=master)](https://travis-ci.org/nolanlum/tanya)
slack irc gateway in a world with no generics

## Running tanya
Make sure your `$GOPATH\bin` is in your `$PATH` and simply run `tanya`. If you haven't created a configuration file yet, it will prompt you to log into Slack and automatically grab an API token. An IRC server will then start listening on `:6667` by default.

Tanya will report unhandled events from the Slack RTM event stream via stderr, and select error and status messages are also sent to all connected IRC clients via the `*tanya` virtual user. (Patches welcome for unhandled RTM events.)

## Configuring tanya
A [sample config file](https://github.com/nolanlum/tanya/blob/master/config.toml.example) is provided for your convenience. Multiple gateway instances can be configured by providing multiple `[[gateway]]` blocks. Note that tanya is not designed for overlaying multiple slack workspaces into a single IRC server, and no support for this use case is planned.

## Debugging tanya
If you experience a "hang" while running `tanya` (e.g. IRC clients staying connected, but no messages are sent/received; or "split-brain", where messaging becomes unidirectional), you've probably run into a bug which has caused a race condition. If possible, terminate the `tanya` instance with `SIGABRT`, which triggers a dump of all goroutine stacks to `stderr`, and open an issue with the aforementioned output.
