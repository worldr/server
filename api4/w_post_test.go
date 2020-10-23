package api4

import (
	"fmt"
	"testing"
	"time"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func populateChannels(th *TestHelper, msgCount int) (channelIds []string, postsByChannel map[string][]string) {
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

func TestGetReactionsForPosts(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	WClient := th.WClient
	userId := th.BasicUser.Id
	user2Id := th.BasicUser2.Id
	post1 := &model.Post{UserId: userId, ChannelId: th.BasicChannel.Id, Message: "zz" + model.NewId() + "a"}
	post2 := &model.Post{UserId: userId, ChannelId: th.BasicChannel.Id, Message: "zz" + model.NewId() + "a"}
	post3 := &model.Post{UserId: userId, ChannelId: th.BasicChannel.Id, Message: "zz" + model.NewId() + "a"}

	post4 := &model.Post{UserId: user2Id, ChannelId: th.BasicChannel.Id, Message: "zz" + model.NewId() + "a"}
	post5 := &model.Post{UserId: user2Id, ChannelId: th.BasicChannel.Id, Message: "zz" + model.NewId() + "a"}

	post1, _ = Client.CreatePost(post1)
	post2, _ = Client.CreatePost(post2)
	post3, _ = Client.CreatePost(post3)
	post4, _ = Client.CreatePost(post4)
	post5, _ = Client.CreatePost(post5)

	expectedPostIdsReactionsMap := make(map[string][]*model.Reaction)
	expectedPostIdsReactionsMap[post1.Id] = []*model.Reaction{}
	expectedPostIdsReactionsMap[post2.Id] = []*model.Reaction{}
	expectedPostIdsReactionsMap[post3.Id] = []*model.Reaction{}
	expectedPostIdsReactionsMap[post5.Id] = []*model.Reaction{}

	userReactions := []*model.Reaction{
		{
			UserId:    userId,
			PostId:    post1.Id,
			EmojiName: "happy",
		},
		{
			UserId:    userId,
			PostId:    post1.Id,
			EmojiName: "sad",
		},
		{
			UserId:    userId,
			PostId:    post2.Id,
			EmojiName: "smile",
		},
		{
			UserId:    user2Id,
			PostId:    post4.Id,
			EmojiName: "smile",
		},
	}

	for _, userReaction := range userReactions {
		reactions := expectedPostIdsReactionsMap[userReaction.PostId]
		reaction, err := th.App.Srv().Store.Reaction().Save(userReaction)
		require.Nil(t, err)
		reactions = append(reactions, reaction)
		expectedPostIdsReactionsMap[userReaction.PostId] = reactions
	}

	postIds := []string{post1.Id, post2.Id, post3.Id, post4.Id, post5.Id}

	t.Run("get-reactions", func(t *testing.T) {
		response, resp := WClient.GetReactionsForPosts(postIds)
		CheckNoError(t, resp)

		assert.ElementsMatch(t, expectedPostIdsReactionsMap[post1.Id], response.Content[post1.Id])
		assert.ElementsMatch(t, expectedPostIdsReactionsMap[post2.Id], response.Content[post2.Id])
		assert.ElementsMatch(t, expectedPostIdsReactionsMap[post3.Id], response.Content[post3.Id])
		assert.ElementsMatch(t, expectedPostIdsReactionsMap[post4.Id], response.Content[post4.Id])
		assert.ElementsMatch(t, expectedPostIdsReactionsMap[post5.Id], response.Content[post5.Id])
		assert.Equal(t, expectedPostIdsReactionsMap, response.Content)

	})

	t.Run("get-reactions-as-anonymous-user", func(t *testing.T) {
		Client.Logout()

		_, resp := WClient.GetReactionsForPosts(postIds)
		CheckUnauthorizedStatus(t, resp)
	})
}

func TestCheckUpdates(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	perChannel := 5
	// NB: some channels are created by InitBasic(), so these are not the only ones
	// present for the user. This test relies on the number of channels,
	// because that's the functionality underneath: checking updates for all
	// channels available for the user.
	channels, posts := populateChannels(th, perChannel)

	t.Run("all channels have updates", func(t *testing.T) {
		request := make([]*model.ChannelWithPost, 0, len(channels))
		for _, v := range channels {
			request = append(request, &model.ChannelWithPost{
				ChannelId: v,
				PostId:    posts[v][0],
			})
		}
		result, err := th.WClient.CheckUpdates(request)
		CheckNoError(t, err)
		assert.Equal(t, len(channels), len(*result.Updated))
		assert.Equal(t, 7, len(*result.Added))
		assert.Equal(t, 0, len(*result.Removed))
	})

	t.Run("no channels have updates", func(t *testing.T) {
		request := make([]*model.ChannelWithPost, 0, len(channels))
		for _, v := range channels {
			request = append(request, &model.ChannelWithPost{
				ChannelId: v,
				PostId:    posts[v][len(posts[v])-1],
			})
		}
		result, err := th.WClient.CheckUpdates(request)
		CheckNoError(t, err)
		assert.Equal(t, 0, len(*result.Updated))
		assert.Equal(t, 7, len(*result.Added))
		assert.Equal(t, 0, len(*result.Removed))
	})

	t.Run("some channels have updates", func(t *testing.T) {
		request := make([]*model.ChannelWithPost, 0, len(channels))

		request = append(request, &model.ChannelWithPost{
			ChannelId: channels[0],
			PostId:    posts[channels[0]][len(posts[channels[0]])-1],
		})

		request = append(request, &model.ChannelWithPost{
			ChannelId: channels[1],
			PostId:    posts[channels[1]][0],
		})

		result, err := th.WClient.CheckUpdates(request)
		CheckNoError(t, err)
		assert.Equal(t, 1, len(*result.Updated))
		assert.Equal(t, 8, len(*result.Added))
		assert.Equal(t, 0, len(*result.Removed))
	})

	t.Run("one deleted", func(t *testing.T) {
		request := make([]*model.ChannelWithPost, 0, len(channels))
		for _, v := range channels {
			request = append(request, &model.ChannelWithPost{
				ChannelId: v,
				PostId:    posts[v][len(posts[v])-1],
			})
		}
		request = append(request, &model.ChannelWithPost{
			ChannelId: th.BasicDeletedChannel.Id,
			PostId:    "",
		})
		result, err := th.WClient.CheckUpdates(request)
		CheckNoError(t, err)
		assert.Equal(t, 0, len(*result.Updated))
		assert.Equal(t, 7, len(*result.Added))
		assert.Equal(t, 1, len(*result.Removed))
	})
}
