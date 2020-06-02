package gateway

import (
	"bufio"
	"fmt"
	"log"
	"strings"

	"github.com/slack-go/slack"
)

var slackFakeUser = &SlackUser{Nick: "SLACK", SlackID: "SLACK"}

func messageTextToEvents(sender *SlackUser, target, messageText string) []*SlackEvent {
	var events []*SlackEvent

	parsedMessage := bufio.NewScanner(strings.NewReader(messageText))
	for parsedMessage.Scan() {
		messageLine := parsedMessage.Text()
		if len(messageLine) > 0 {
			events = append(events, newSlackMessageEvent(sender, target, messageLine))
		}
	}

	return events
}

func (sc *SlackClient) handleMessageEvent(incomingChan chan<- *SlackEvent, messageData *slack.MessageEvent) {
	if messageData.User == "USLACKBOT" {
		return
	}

	var sender *SlackUser
	if messageData.User != "" {
		var err error
		if sender, err = sc.ResolveUser(messageData.User); err != nil {
			log.Printf("%s could not resolve user for message [%v]: %+v", sc.Tag(), err, messageData)
			return
		}
	}

	var target string
	wasSenderSwapped := false
	if messageData.Channel != "" {
		if isDmChannel(messageData.Channel) {
			target = sc.self.Nick

			// If we sent this DM, then the sender needs to be faked to the other user
			// because IRC clients can't display DMs done by others on your behalf
			if sender == sc.self {
				otherUser, err := sc.ResolveDMToUser(messageData.Channel)
				if err != nil {
					log.Printf("%s could not resolve DM user for message [%v]: %+v", sc.Tag(), err, messageData)
					return
				}

				sender = otherUser
				wasSenderSwapped = true
			}
		} else {
			channel, err := sc.ResolveChannel(messageData.Channel)
			if err != nil {
				log.Printf("%s could not resolve channel for message [%v]: %+v", sc.Tag(), err, messageData)
				return
			}
			target = channel.Name
		}
	}

	switch messageData.SubType {
	case "", "pinned_item", "thread_broadcast":
		if sender == nil || target == "" {
			return
		}

		messageText := sc.ParseMessageText(messageData.Text)
		for _, attachment := range messageData.Attachments {
			if messageText == "" {
				messageText = sc.ParseMessageText(attachment.Fallback)
			} else {
				messageText = messageText + "\n" + sc.ParseMessageText(attachment.Fallback)
			}
		}

		// If we had swapped senders earlier, make sure the message reflects this swap
		if wasSenderSwapped {
			messageText = "[" + sc.self.Nick + "] " + messageText
		}

		for _, messageEvent := range messageTextToEvents(sender, target, messageText) {
			incomingChan <- messageEvent
		}

		// Handle message file attachments
		verb := "shared"
		if messageData.Upload {
			verb = "uploaded"
		}

		for _, file := range messageData.Files {
			incomingChan <- newSlackMessageEvent(
				sender, target, fmt.Sprintf("@%s %s a file: %s %s",
					sender.Nick, verb, file.Name, file.URLPrivate))
		}

	case "bot_message":
		if sender == nil {
			sender = slackFakeUser
		}
		if target == "" {
			return
		}

		messageText := sc.ParseMessageText(messageData.Text)
		for _, attachment := range messageData.Attachments {
			if messageText == "" {
				messageText = sc.ParseMessageText(attachment.Fallback)
			} else {
				messageText = messageText + "\n" + sc.ParseMessageText(attachment.Fallback)
			}
		}

		for _, messageEvent := range messageTextToEvents(sender, target, messageText) {
			incomingChan <- messageEvent
		}

	case "message_changed":
		subMessage := messageData.SubMessage
		if subMessage == nil {
			log.Printf("%s message_changed with missing submessage: %+v SubMessage:%+v",
				sc.Tag(), messageData, messageData.SubMessage)
			return
		}

		if subMessage.Type != "message" {
			log.Printf("%s message_changed with unexpected submessage type: %+v SubMessage:%+v",
				sc.Tag(), messageData, messageData.SubMessage)
			return
		}

		switch subMessage.SubType {
		case "":
			// Continue handling messages with empty SubType.

		case "thread_broadcast", "bot_message":
			// Ignore thread broadcast expands and modified bot messages.
			return

		default:
			log.Printf("%s message_changed with unexpected submessage subtype: %+v SubMessage:%+v",
				sc.Tag(), messageData, messageData.SubMessage)
			return
		}

		// For now, only handle the Slack native expansion of archive links
		if !strings.Contains(subMessage.Text, "slack.com/archives") || len(subMessage.Attachments) < 1 {
			return
		}

		if target == "" {
			return
		}

		user, err := sc.ResolveUser(subMessage.User)
		if err != nil {
			log.Printf("%s could not resolve user for archive link [%v]: %+v",
				sc.Tag(), err, messageData.SubMessage)
			return
		}

		incomingChan <- newSlackMessageEvent(
			user,
			target,
			fmt.Sprint(sc.ParseMessageText(subMessage.Attachments[0].Fallback)),
		)

	case "channel_topic":
		if sender == nil || target == "" {
			return
		}

		incomingChan <- &SlackEvent{
			EventType: TopicChangeEvent,
			Data: &TopicChangeEventData{
				From:     *sender,
				Target:   target,
				NewTopic: messageData.Topic,
			},
		}

	case "channel_leave", "channel_join":
		// These are already handled elsewhere, drop the message event.
		return

	case "message_replied":
		// As far as I can tell, this is only useful for updating the reply count of a message thread.
		return

	default:
		log.Printf("%s unhandled message sub-type [%v]: %+v SubMessage:%+v",
			sc.Tag(), messageData.SubType, messageData, messageData.SubMessage)
	}
}
