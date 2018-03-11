package irc

import "testing"

// TODO: Improve the testing here
func TestStringToMessage(t *testing.T) {
	msg, err := StringToMessage("user a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != UserCmd {
		t.Error("Could not parse 'user a' as User command")
	}

	msg, err = StringToMessage("nick a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != NickCmd {
		t.Error("Could not parse 'nick a' as Nick command")
	}
}

func TestStringToMessageWithPrefix(t *testing.T) {
	msg, err := StringToMessage(":hello user a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != UserCmd {
		t.Error("Could not parse ':hello user a' as User command")
	}
}

func TestStringToMessageErr(t *testing.T) {
	_, err := StringToMessage("atest blah")
	if err == nil {
		t.Error("'atest blah' should not be a valid message")
	}
}

func TestStringToMessageOnlyPrefixErr(t *testing.T) {
	_, err := StringToMessage(":hello")
	if err == nil {
		t.Error("'atest blah' should not be a valid message")
	}
}
