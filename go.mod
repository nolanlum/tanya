module github.com/nolanlum/tanya

go 1.13

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/slack-go/slack v0.8.1
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
)

replace github.com/slack-go/slack => ./vendor/slack