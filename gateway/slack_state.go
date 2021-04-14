package gateway

import (
	"fmt"
	"log"
	"time"

	"github.com/slack-go/slack"
)

// Clear all stored state and reload workspace/conversation metadata from Slack.
// Called upon reconnection to ensure all cached state is up-to-date.
func (sc *SlackClient) bootstrapMappings() {
	startTime := time.Now()

	channelInfo := make(map[string]*SlackChannel)
	userInfo := make(map[string]*SlackUser)
	dmInfo := make(map[string]*SlackUser)
	channelMemberships := make(map[string]*SlackChannel)

	hasMore := true
	gcp := &slack.GetConversationsParameters{
		ExcludeArchived: "true",
		Limit:           1000,
		Types:           []string{"public_channel", "private_channel"},
	}
	for hasMore {
		var channels []slack.Channel
		var err error
		channels, gcp.Cursor, err = sc.client.GetConversations(gcp)
		if err != nil {
			log.Fatalln(err)
		}

		for _, channel := range channels {
			slackChannel := slackChannelFromDto(&channel)

			channelInfo[channel.ID] = slackChannel
			if channel.IsMember {
				channelMemberships[channel.ID] = slackChannel
			}
		}

		hasMore = gcp.Cursor != ""
	}

	users, err := sc.client.GetUsers()
	if err != nil {
		log.Fatalln(err)
	}
	for _, user := range users {
		userInfo[user.ID] = slackUserFromDto(&user)
	}

	ucParams := &slack.GetConversationsParameters{
		Cursor:          "",
		Types:           []string{"im"},
		Limit:           0,
		ExcludeArchived: "true",
	}
	ims, _, err := sc.client.GetConversations(ucParams)
	if err != nil {
		log.Fatalln(err)
	}
	for _, im := range ims {
		dmInfo[im.ID] = userInfo[im.User]
	}

	sc.Lock()
	sc.channelInfo = channelInfo
	sc.userInfo = userInfo
	sc.dmInfo = dmInfo
	sc.channelMemberships = channelMemberships
	sc.channelMembers = make(map[string]map[string]*SlackUser)
	sc.inflightMessageMap = make(map[int]*inflightMessage)
	sc.Unlock()

	sc.conversationMarker.Reset()
	sc.regenerateReverseMappings()
	sc.cleanupMappings()

	log.Printf("%s slack:init channels:%v users:%v dms:%v memberships:%v time:%v", sc.Tag(),
		len(sc.channelInfo), len(sc.userInfo), len(sc.dmInfo), len(sc.channelMemberships), time.Since(startTime))
}

// Regenerate the cached reverse nick/channel name mappings
// If two channels have the same name, then whelp the first one we find wins
func (sc *SlackClient) regenerateReverseMappings() {
	sc.Lock()
	defer sc.Unlock()

	sc.nickToUserMap = make(map[string]string)
	for _, user := range sc.userInfo {
		if user == nil {
			continue
		}
		sc.nickToUserMap[user.Nick] = user.SlackID
	}

	sc.channelNameToIDMap = make(map[string]string)
	for _, channel := range sc.channelInfo {
		if channel == nil {
			continue
		}
		sc.channelNameToIDMap[channel.Name] = channel.SlackID
	}

	sc.userIDToDMIDMap = make(map[string]string)
	for dmID, user := range sc.dmInfo {
		if user == nil {
			continue
		}
		sc.userIDToDMIDMap[user.SlackID] = dmID
	}
}

// Clean up our mappings if necessary
func (sc *SlackClient) cleanupMappings() {
	sc.Lock()
	defer sc.Unlock()

	for channelName, channel := range sc.channelInfo {
		if channel == nil {
			delete(sc.userInfo, channelName)
		}
	}

	for username, user := range sc.userInfo {
		if user == nil {
			delete(sc.userInfo, username)
		}
	}

	for dmName, dmUser := range sc.dmInfo {
		if dmUser == nil {
			delete(sc.dmInfo, dmName)
		}
	}
}

// ResolveUser takes a slackID and fetches a SlackUser for the ID
func (sc *SlackClient) ResolveUser(slackID string) (user *SlackUser, err error) {
	sc.RLock()
	user, found := sc.userInfo[slackID]
	if found {
		sc.RUnlock()
		return
	}

	sc.RUnlock()
	userInfo, err := sc.client.GetUserInfo(slackID)
	if err != nil {
		return
	}
	user = slackUserFromDto(userInfo)

	sc.Lock()
	sc.userInfo[user.SlackID] = user
	sc.nickToUserMap[user.Nick] = user.SlackID
	sc.Unlock()
	return
}

// ResolveChannel takes a slackID and fetches a SlackChannel for the ID
func (sc *SlackClient) ResolveChannel(slackID string) (channel *SlackChannel, err error) {
	sc.RLock()
	channel, found := sc.channelInfo[slackID]
	if found {
		sc.RUnlock()
		return
	}

	sc.RUnlock()
	channelInfo, err := sc.client.GetConversationInfo(slackID, false)
	if err != nil {
		return
	}
	channel = slackChannelFromDto(channelInfo)

	sc.Lock()
	sc.channelInfo[channel.SlackID] = channel
	sc.channelNameToIDMap[channel.Name] = channel.SlackID
	sc.Unlock()
	return
}

// ResolveNameToChannel takes a channel name and fetches a SlackChannel with that name
func (sc *SlackClient) ResolveNameToChannel(channelName string) *SlackChannel {
	sc.RLock()
	defer sc.RUnlock()

	if channelID, found := sc.channelNameToIDMap[channelName]; found {
		if channelInfo, found := sc.channelInfo[channelID]; found {
			if channelInfo.Name != channelName {
				log.Panicf("SlackClient.channelNameToIDMap had stale data: %v = %v != %v",
					channelName, channelID, channelInfo.Name)
			}

			return channelInfo
		}
	}

	return nil
}

// ResolveNickToUser takes a nick and fetches a SlackUser with that nick
func (sc *SlackClient) ResolveNickToUser(nick string) *SlackUser {
	sc.RLock()
	defer sc.RUnlock()

	if userID, found := sc.nickToUserMap[nick]; found {
		if userInfo, found := sc.userInfo[userID]; found {
			if userInfo.Nick != nick {
				log.Panicf("SlackClient.nickToUserMap had stale data: %v = %v != %v", nick, userID, userInfo.Nick)
			}

			return userInfo
		}
	}

	return nil
}

// ResolveUserToDM resolves a SlackUser to their DM channel, opening one if it doesn't exist
func (sc *SlackClient) ResolveUserToDM(user *SlackUser) (string, error) {
	sc.RLock()
	dmID, found := sc.userIDToDMIDMap[user.SlackID]
	sc.RUnlock()

	if found {
		return dmID, nil
	}

	ocp := &slack.OpenConversationParameters{
		ChannelID: user.SlackID,
		ReturnIM:  true,
		Users:     []string{user.SlackID},
	}
	channel, _, _, err := sc.client.OpenConversation(ocp)
	if err != nil {
		return "", err
	}
	dmID = channel.ID

	sc.Lock()
	sc.dmInfo[dmID] = user
	sc.userIDToDMIDMap[user.SlackID] = dmID
	sc.Unlock()

	return dmID, nil
}

// ResolveDMToUser resolves a DM/IM Channel ID to the User the DM is for
func (sc *SlackClient) ResolveDMToUser(dmChannelID string) (*SlackUser, error) {
	sc.RLock()
	slackUser, found := sc.dmInfo[dmChannelID]
	sc.RUnlock()

	if found {
		return slackUser, nil
	}

	slackUser = nil
	ucParams := &slack.GetConversationsParameters{
		Cursor:          "",
		Types:           []string{"im"},
		Limit:           0,
		ExcludeArchived: "true",
	}
	ims, _, err := sc.client.GetConversations(ucParams)
	if err != nil {
		return nil, err
	}

	sc.Lock()
	for _, im := range ims {
		// Skip this IM if we cannot find the user it belongs to
		if userInfo, found := sc.userInfo[im.User]; found {
			sc.dmInfo[im.ID] = userInfo
			sc.userIDToDMIDMap[userInfo.SlackID] = im.ID
			if im.ID == dmChannelID {
				slackUser = userInfo
			}
		}
	}
	sc.Unlock()

	if slackUser != nil {
		return slackUser, nil
	}

	return nil, fmt.Errorf("could not find user for DM: %s", dmChannelID)
}
