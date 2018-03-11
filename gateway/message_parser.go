package gateway

import (
	"log"
	"strings"
)

// ParseMessageText takes raw Slack message payload and resolves the user
// and channel references
func (sc *SlackClient) ParseMessageText(text string) string {
	parsedMessageBuilder := strings.Builder{}

	// Find the first '<' if any, split into "before" and "after"
	textParts := strings.SplitN(text, "<", 2)
	parsedMessageBuilder.WriteString(textParts[0])

	for len(textParts) > 1 {
		// Grab the bit until the '>' and poop the rest elsewhere.
		textParts = strings.SplitN(textParts[1], ">", 2)
		ref := textParts[0]

		switch ref[0] {
		case '@':
			// User ID ref
			user, err := sc.ResolveUser(ref[1:])
			if err != nil {
				log.Printf("error while parsing message, could not resolve ref %v: %v", ref, err)
				parsedMessageBuilder.WriteString(ref)
				break
			}

			parsedMessageBuilder.WriteByte('@')
			parsedMessageBuilder.WriteString(user.Nick)

		case '#':
			// Channel ref -- as far as I can tell the real name will be included luckily.
			channelRefParts := strings.SplitN(ref, "|", 2)
			if len(channelRefParts) != 2 {
				log.Printf("error while parsing message, could not parse channel ref: %v", ref)
				parsedMessageBuilder.WriteString(ref)
				break
			}

			parsedMessageBuilder.WriteByte('#')
			parsedMessageBuilder.WriteString(channelRefParts[1])

		default:
			// A URL, we only care about the "display" portion that was actually sent.
			displayIdx := strings.Index(ref, "|")
			parsedMessageBuilder.WriteString(ref[displayIdx+1:])
		}

		// Do it again I guess.
		textParts = strings.SplitN(textParts[1], "<", 2)
		parsedMessageBuilder.WriteString(textParts[0])
	}

	return parsedMessageBuilder.String()
}
