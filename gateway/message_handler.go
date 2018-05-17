package gateway

import (
	"bufio"
	"fmt"
	"log"
	"strings"

	"github.com/nlopes/slack"
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
	var sender *SlackUser
	if messageData.User != "" {
		var err error
		if sender, err = sc.ResolveUser(messageData.User); err != nil {
			log.Printf("could not resolve user for message [%v]: %+v", err, messageData)
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
					log.Printf("could not resolve DM user for message [%v]: %+v", err, messageData)
					return
				}

				sender = otherUser
				wasSenderSwapped = true
			}
		} else {
			channel, err := sc.ResolveChannel(messageData.Channel)
			if err != nil {
				log.Printf("could not resolve channel for message [%v]: %+v", err, messageData)
				return
			}
			target = channel.Name
		}
	}

	switch messageData.SubType {
	case "file_mention":
		if messageData.User == sc.self.SlackID {
			// Eat the "helpful" messages Slack sends when you send a link to a Slack upload.
			return
		}
		fallthrough
	case "file_share":
		if sender == nil || target == "" {
			return
		}

		// Slack "abuses" the display/canonical URL feature of its markdown for file upload messages,
		// so we need to keep them.
		messageText := sc.ParseMessageTextWithOptions(messageData.Text, true)

		// If we had swapped senders earlier, make sure the message reflects this swap
		if wasSenderSwapped {
			messageText = "[" + sc.self.Nick + "] " + messageText
		}

		for _, messageEvent := range messageTextToEvents(sender, target, messageText) {
			incomingChan <- messageEvent
		}

	case "", "pinned_item":
		if sender == nil || target == "" {
			return
		}

		messageText := sc.ParseMessageText(messageData.Text)
		for _, attachment := range messageData.Attachments {
			if messageText == "" {
				messageText = sc.slackURLDecoder.Replace(attachment.Fallback)
			} else {
				messageText = messageText + "\n" + sc.slackURLDecoder.Replace(attachment.Fallback)
			}
		}

		// If we had swapped senders earlier, make sure the message reflects this swap
		if wasSenderSwapped {
			messageText = "[" + sc.self.Nick + "] " + messageText
		}

		for _, messageEvent := range messageTextToEvents(sender, target, messageText) {
			incomingChan <- messageEvent
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
				messageText = sc.slackURLDecoder.Replace(attachment.Fallback)
			} else {
				messageText = messageText + "\n" + sc.slackURLDecoder.Replace(attachment.Fallback)
			}
		}

		for _, messageEvent := range messageTextToEvents(sender, target, messageText) {
			incomingChan <- messageEvent
		}

	case "message_changed":
		subMessage := messageData.SubMessage
		if subMessage == nil || subMessage.SubType != "" {
			log.Printf("message_changed with unexpected or missing submessage: %+v SubMessage:%+v",
				messageData, messageData.SubMessage)
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
			log.Printf("could not resolve user for archive link [%v]: %+v", err, messageData.SubMessage)
			return
		}

		incomingChan <- newSlackMessageEvent(
			user,
			target,
			fmt.Sprintf(sc.ParseMessageText(subMessage.Attachments[0].Fallback)),
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
		log.Printf("unhandled message sub-type [%v]: %+v SubMessage:%+v",
			messageData.SubType, messageData, messageData.SubMessage)
	}
}
