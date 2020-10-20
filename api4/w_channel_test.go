package api4

import (
	"fmt"
	"testing"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/stretchr/testify/assert"
)

func createChannelsForCategories(t *testing.T, th *TestHelper) []*model.Channel {
	channels := []*model.Channel{
		{
			DisplayName: "Test1",
			Name:        GenerateTestChannelName(),
			Type:        model.CHANNEL_OPEN,
			TeamId:      th.BasicTeam.Id,
		},
		{
			DisplayName: "Test2",
			Name:        GenerateTestChannelName(),
			Type:        model.CHANNEL_OPEN,
			TeamId:      th.BasicTeam.Id,
		},
		{
			DisplayName: "Test3",
			Name:        GenerateTestChannelName(),
			Type:        model.CHANNEL_OPEN,
			TeamId:      th.BasicTeam.Id,
		},
	}
	for i, v := range channels {
		c, resp := th.Client.CreateChannel(v)
		CheckNoError(t, resp)
		channels[i] = c
	}
	return channels
}

func TestChannelCategoriesAssignGet(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channels := createChannelsForCategories(t, th)

	// No categories expected
	list, r := th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 0, len(list), "unexpected count of categories")

	// One category for one channel
	created, r := th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[0].Id,
		Name:      "cats",
		Sort:      1,
	})
	CheckNoError(t, r)
	assert.Equal(t, "cats", created.Name, "unexpected category name")
	assert.Equal(t, channels[0].Id, created.ChannelId, "unexpected channel id")

	// Second category to another channel
	created, r = th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[1].Id,
		Name:      "dogs",
		Sort:      2,
	})
	CheckNoError(t, r)
	assert.Equal(t, "dogs", created.Name, "unexpected category name")
	assert.Equal(t, channels[1].Id, created.ChannelId, "unexpected channel id")

	list, r = th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 2, len(list), "unexpected count of categories")
	assert.Equal(t, 1, len(list[0].Channels), "unexpected count of channels in category 1")
	assert.Equal(t, list[0].Channels[0], channels[0].Id, "unexpected channel under category 1")
	assert.Equal(t, 1, len(list[1].Channels), "unexpected count of channels in category 2")
	assert.Equal(t, list[1].Channels[0], channels[1].Id, "unexpected channel under category 2")

	// Another channel added to second category
	created, r = th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[2].Id,
		Name:      "dogs",
		Sort:      2,
	})
	CheckNoError(t, r)
	assert.Equal(t, "dogs", created.Name, "unexpected category name")

	list, r = th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 2, len(list), "unexpected count of categories")
	assert.Equal(t, 1, len(list[0].Channels), "unexpected count of channels in category 1")
	assert.Equal(t, list[0].Channels[0], channels[0].Id, "unexpected channel under category 1")
	assert.Equal(t, 2, len(list[1].Channels), "unexpected count of channels in category 2")
	for _, v := range list[1].Channels {
		assert.True(t, v == channels[1].Id || v == channels[2].Id, "unexpected channel under category 2")
	}

	// Invalid category
	created, r = th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[2].Id,
		Name:      "    ",
		Sort:      2,
	})
	CheckBadRequestStatus(t, r)

	// Non-existent channel
	created, r = th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: "junk",
		Name:      "ok",
		Sort:      2,
	})
	CheckForbiddenStatus(t, r)

	// Missing channel parameter
	created, r = th.WClient.AssignChannelCategory(&model.ChannelCategory{
		Name: "ok",
		Sort: 2,
	})
	CheckForbiddenStatus(t, r)
}

func TestRemoveCategoryFromChannel(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channels := createChannelsForCategories(t, th)

	_, r := th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[0].Id,
		Name:      "cats",
		Sort:      1,
	})
	CheckNoError(t, r)

	// One category
	list, r := th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 1, len(list), "unexpected count of categories")

	// Missing parameter
	r = th.WClient.RemoveCategoryFromChannel(map[string]string{})
	CheckBadRequestStatus(t, r)

	// Non-existent channel is ignored, nothing happens
	r = th.WClient.RemoveCategoryFromChannel(map[string]string{"channel_id": "junk"})
	CheckNoError(t, r)

	// Success
	th.WClient.RemoveCategoryFromChannel(map[string]string{"channel_id": channels[0].Id})
	CheckNoError(t, r)

	// No categories
	list, r = th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 0, len(list), "category was not removed")
}

func TestChangeCategory(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channels := createChannelsForCategories(t, th)
	cats := &model.ChannelCategory{
		Name: "cats",
		Sort: 1,
	}
	dogs := &model.ChannelCategory{
		Name: "dogs",
		Sort: 2,
	}

	cats.ChannelId = channels[0].Id
	_, r := th.WClient.AssignChannelCategory(cats)
	CheckNoError(t, r)

	cats.ChannelId = channels[1].Id
	_, r = th.WClient.AssignChannelCategory(cats)
	CheckNoError(t, r)

	cats.ChannelId = channels[2].Id
	_, r = th.WClient.AssignChannelCategory(cats)
	CheckNoError(t, r)

	// Change category to dogs
	dogs.ChannelId = channels[0].Id
	_, r = th.WClient.AssignChannelCategory(dogs)
	CheckNoError(t, r)

	// Two categories, one channels in dogs, two in cats
	list, r := th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 2, len(list), "unexpected number of categories")
	assert.Equal(t, dogs.Name, list[1].Name, "unexpected category name")

	dogs.Name = "Dogs"
	dogs.ChannelId = channels[0].Id
	_, r = th.WClient.AssignChannelCategory(dogs)
	CheckNoError(t, r)

	// Category name should be capitalised
	list, r = th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, dogs.Name, list[1].Name, "unexpected category name")
}

func TestReorderCategories(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channels := createChannelsForCategories(t, th)

	_, r := th.WClient.AssignChannelCategory(&model.ChannelCategory{
		ChannelId: channels[0].Id,
		Name:      "cats",
		Sort:      1,
	})
	CheckNoError(t, r)

	// Success
	r = th.WClient.ReorderChannelCategory(map[string]string{
		"name": "cats",
		"sort": "2",
	})
	CheckNoError(t, r)

	list, r := th.WClient.GetChannelCategories()
	CheckNoError(t, r)
	assert.Equal(t, 1, len(list), "unexpected number of categories")
	assert.Equal(t, int32(2), list[0].Sort, fmt.Sprintf("unexpected category sorting: %+v", list[0]))

	// Bad sort parameter
	r = th.WClient.ReorderChannelCategory(map[string]string{
		"name": "cats",
		"sort": "junk",
	})
	CheckBadRequestStatus(t, r)

	// Missing name parameter
	r = th.WClient.ReorderChannelCategory(map[string]string{
		"sort": "junk",
	})
	CheckBadRequestStatus(t, r)

	// Non-existent category
	r = th.WClient.ReorderChannelCategory(map[string]string{
		"name": "dogs",
		"sort": "3",
	})
	CheckNotFoundStatus(t, r)
}
