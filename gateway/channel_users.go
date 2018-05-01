package gateway

import (
	"log"
	"sync"
	"time"

	"github.com/nlopes/slack"
)

// getChannelUsersFromAPI queries the Slack API for a list of users in the given channel, returning
// SlackUser objects for each one
func (sc *SlackClient) getChannelUsersFromAPI(channelID string) (users []*SlackUser, err error) {
	hasMore := true
	guicp := &slack.GetUsersInConversationParameters{
		ChannelID: channelID,
		Limit:     1000,
	}
	for hasMore {
		var userIDs []string
		userIDs, guicp.Cursor, err = sc.client.GetUsersInConversation(guicp)
		if err != nil {
			return
		}

		for _, userID := range userIDs {
			var user *SlackUser
			user, err = sc.ResolveUser(userID)
			if err != nil {
				return
			}
			users = append(users, user)
		}

		hasMore = guicp.Cursor != ""
	}

	return
}

// bootstrapChannelUserList fetches user lists for all channels the SlackClient is a member of
func (sc *SlackClient) bootstrapChannelUserList() {
	var wg sync.WaitGroup
	var channelIDs []string
	startTime := time.Now()

	sc.RLock()
	for channelID := range sc.channelMemberships {
		channelIDs = append(channelIDs, channelID)
	}
	sc.RUnlock()

	wg.Add(len(channelIDs))
	for _, channelID := range channelIDs {
		channelID := channelID
		go func() {
			if _, err := sc.GetChannelUsers(channelID); err != nil {
				log.Printf("error while bootstrapping user list for %v: %v", channelID, err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	log.Printf("slack:init channel_userlists:%v time:%v", len(sc.channelMembers), time.Since(startTime))
}

// GetChannelUsers returns a locally cached list of users in the given channel
func (sc *SlackClient) GetChannelUsers(channelID string) ([]SlackUser, error) {
	sc.RLock()
	if channelMembers, found := sc.channelMembers[channelID]; found {
		users := make([]SlackUser, 0)
		for _, user := range channelMembers {
			users = append(users, *user)
		}

		sc.RUnlock()
		return users, nil
	}
	sc.RUnlock()

	channelMembers, err := sc.getChannelUsersFromAPI(channelID)
	if err != nil {
		return nil, err
	}

	channelMemberMap := make(map[string]*SlackUser)
	for _, user := range channelMembers {
		channelMemberMap[user.SlackID] = user
	}

	sc.Lock()
	sc.channelMembers[channelID] = channelMemberMap
	sc.Unlock()

	users := make([]SlackUser, 0)
	for _, user := range channelMembers {
		users = append(users, *user)
	}
	return users, nil
}

func (sc *SlackClient) handleMemberJoinedChannel(channelID, userID string) (*SlackEvent, error) {
	user, err := sc.ResolveUser(userID)
	if err != nil {
		return nil, err
	}

	target, err := sc.ResolveChannel(channelID)
	if err != nil {
		return nil, err
	}

	sc.RLock()
	_, found := sc.channelMembers[channelID]
	sc.RUnlock()
	if found {
		sc.Lock()
		sc.channelMembers[channelID][userID] = user
		sc.Unlock()
	}

	return &SlackEvent{
		EventType: JoinEvent,
		Data: &JoinPartEventData{
			User:   *user,
			Target: target.Name,
		},
	}, nil
}

func (sc *SlackClient) handleMemberLeftChannel(channelID, userID string) (*SlackEvent, error) {
	user, err := sc.ResolveUser(userID)
	if err != nil {
		return nil, err
	}

	target, err := sc.ResolveChannel(channelID)
	if err != nil {
		return nil, err
	}

	sc.RLock()
	_, found := sc.channelMembers[channelID]
	sc.RUnlock()
	if found {
		sc.Lock()
		delete(sc.channelMembers[channelID], userID)
		sc.Unlock()
	}

	return &SlackEvent{
		EventType: PartEvent,
		Data: &JoinPartEventData{
			User:   *user,
			Target: target.Name,
		},
	}, nil
}
