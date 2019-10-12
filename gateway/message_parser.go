package gateway

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// ParseMessageText takes raw Slack message payload and resolves the user
// and channel references
func (sc *SlackClient) ParseMessageText(text string) string {
	return sc.ParseMessageTextWithOptions(text, false)
}

// ParseMessageTextWithOptions takes raw Slack message payload, resolves the
// user and channel references, and optionally preserves the Slack canonical URL.
func (sc *SlackClient) ParseMessageTextWithOptions(text string, includeCanonicalURL bool) string {
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
				log.Printf("%s error while parsing message, could not resolve ref %v: %v", sc.logtag(), ref, err)
				parsedMessageBuilder.WriteString(ref)
				break
			}

			parsedMessageBuilder.WriteByte('@')
			parsedMessageBuilder.WriteString(user.Nick)

		case '#':
			// Channel ref -- as far as I can tell the real name will be included luckily.
			channelRefParts := strings.SplitN(ref, "|", 2)
			if len(channelRefParts) != 2 {
				log.Printf("%s error while parsing message, could not parse channel ref: %v", sc.logtag(), ref)
				parsedMessageBuilder.WriteString(ref)
				break
			}

			parsedMessageBuilder.WriteByte('#')
			parsedMessageBuilder.WriteString(channelRefParts[1])

		default:
			// A URL, we usually only care about the "display" portion that was actually sent.
			displayIdx := strings.Index(ref, "|")
			parsedMessageBuilder.WriteString(ref[displayIdx+1:])
			if includeCanonicalURL && displayIdx > 0 {
				parsedMessageBuilder.WriteByte(' ')
				parsedMessageBuilder.WriteString(ref[0:displayIdx])
			}
		}

		// Do it again I guess.
		textParts = strings.SplitN(textParts[1], "<", 2)
		parsedMessageBuilder.WriteString(textParts[0])
	}

	return sc.slackURLDecoder.Replace(parsedMessageBuilder.String())
}

// UnparseMessageText takes a IRC message and inserts user references
func (sc *SlackClient) UnparseMessageText(text string) string {
	text = sc.slackURLEncoder.Replace(text)

	atMentionRegex := regexp.MustCompile(`@[A-Za-z][A-Za-z0-9_\- ]*`)
	uniqueMentions := make(map[string]string)
	for _, match := range atMentionRegex.FindAllString(text, -1) {
		uniqueMentions[match] = match
	}

	for mention := range uniqueMentions {
		if user := sc.ResolveNickToUser(mention[1:]); user != nil {
			uniqueMentions[mention] = fmt.Sprintf("<@%v>", user.SlackID)
		}
	}

	replacements := make([]string, 0)
	for mention, id := range uniqueMentions {
		replacements = append(replacements, mention, id)
	}

	replacer := strings.NewReplacer(replacements...)
	return replacer.Replace(text)
}
