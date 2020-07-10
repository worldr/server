package api4

import (
	"fmt"
	"testing"
	"time"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/stretchr/testify/assert"
)

func populateChannels(th *TestHelper, msgCount int) ([]string, map[string][]string) {
	team := th.CreateMainTeam()

	ch1 := th.CreateChannelWithClientAndTeam(th.Client, "O", team.Id)
	ch2 := th.CreateChannelWithClientAndTeam(th.Client, "O", team.Id)
	ch3 := th.CreateChannelWithClientAndTeam(th.Client, "O", team.Id)

	th.AddUserToChannel(th.BasicUser, ch1)
	th.AddUserToChannel(th.BasicUser2, ch1)
	th.AddUserToChannel(th.BasicUser, ch2)
	th.AddUserToChannel(th.BasicUser2, ch2)
	th.AddUserToChannel(th.BasicUser, ch3)
	th.AddUserToChannel(th.BasicUser2, ch3)

	postIds := map[string][]string{
		ch1.Id: make([]string, msgCount),
		ch2.Id: make([]string, msgCount),
		ch3.Id: make([]string, msgCount),
	}

	time.Sleep(1 * time.Millisecond)

	t := time.Now().UnixNano() / int64(1000000)
	for i := 0; i < msgCount; i++ {
		message := fmt.Sprintf("message%v", i)
		t++
		postIds[ch1.Id][i] = th.CreateMessagePostNoClient(ch1, message, t).Id
		t++
		postIds[ch2.Id][i] = th.CreateMessagePostNoClient(ch2, message, t).Id
		t++
		postIds[ch3.Id][i] = th.CreateMessagePostNoClient(ch3, message, t).Id
	}

	return []string{ch1.Id, ch2.Id, ch3.Id}, postIds
}

func checkRecentPosts(
	t *testing.T,
	channels []string,
	expectedPerChannel int,
	postsByChannel map[string][]string,
	r *model.RecentPostsResponseData,
) {
	byChannel := map[string][]*model.Post{}
	for _, v := range *r.Content {
		list, exists := byChannel[v.ChannelId]
		if !exists {
			list = make([]*model.Post, expectedPerChannel)[:0]
		}
		list = append(list, v)
		byChannel[v.ChannelId] = list
	}

	assert.Equal(t, 3, len(byChannel), "unexpected number of channels")

	// check we have posts for all the expected channels
	for _, channelId := range channels {
		list, exists := byChannel[channelId]
		assert.True(t, exists, "no data for channel")
		// The first message in channel is of type system_join_channel, so we may have 1 extra
		if len(list) == expectedPerChannel+1 {
			// if we have one more than expected, check its type and drop it
			assert.Equal(t, "system_join_channel", list[0].Type, "unexpected message type")
			list = list[1:]
		} else {
			assert.Equal(t, expectedPerChannel, len(list), "unexpected number of posts")
		}
		expected := postsByChannel[channelId]
		if expectedPerChannel < len(expected) {
			expected = expected[len(expected)-expectedPerChannel:]
		}
		// check the order of posts
		for i, v := range list {
			assert.Equal(t, expected[i], v.Id, "unexpected order of posts")
		}
	}
}

func TestRecent(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	perChannel := 10
	channels, postsByChannel := populateChannels(th, perChannel)

	t.Run("all messages in channels fit limits", func(t *testing.T) {
		request := &model.RecentPostsRequestData{
			ChannelIds:         channels,
			MaxTotalMessages:   perChannel * 10,
			MessagesPerChannel: perChannel * 2,
		}
		r, err := th.WClient.GetRecentPosts(request)
		CheckNoError(t, err)
		checkRecentPosts(t, channels, perChannel, postsByChannel, r)
	})

	t.Run("some messages in channels fit limits", func(t *testing.T) {
		request := &model.RecentPostsRequestData{
			ChannelIds:         channels,
			MaxTotalMessages:   perChannel * 10,
			MessagesPerChannel: perChannel / 2,
		}
		r, err := th.WClient.GetRecentPosts(request)
		CheckNoError(t, err)
		checkRecentPosts(t, channels, perChannel/2, postsByChannel, r)
	})
}
