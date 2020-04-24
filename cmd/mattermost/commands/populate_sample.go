// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/icrowley/fake"
	"github.com/mattermost/mattermost-server/v5/app"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store/sqlstore"
	"github.com/spf13/cobra"
)

var PopulateSampleCmd = &cobra.Command{
	Use:   "populatesample",
	Short: "Generate sample data",
	RunE:  populateSampleCmdF,
}

const (
	PARAM_ADMINS            = "admins"
	PARAM_SEED              = "seed"
	PARAM_USERS             = "users"
	PARAM_GUESTS            = "guests"
	PARAM_DEACTIVEATED      = "deactivated-users"
	PARAM_POSTS_PER_CHANNEL = "posts-per-channel"
	PARAM_AVATARS           = "profile-images"
)

func init() {
	PopulateSampleCmd.Flags().StringSlice(PARAM_ADMINS, []string{}, "Server admins.")
	PopulateSampleCmd.Flags().Int64P(PARAM_SEED, "s", 1, "Seed used for generating the random data (Different seeds generate different data).")
	PopulateSampleCmd.Flags().IntP(PARAM_USERS, "u", 15, "The number of sample users.")
	PopulateSampleCmd.Flags().IntP(PARAM_GUESTS, "g", 0, "The number of sample guests.")
	PopulateSampleCmd.Flags().Int(PARAM_DEACTIVEATED, 0, "The number of deactivated users.")
	PopulateSampleCmd.Flags().Int(PARAM_POSTS_PER_CHANNEL, 50, "The number of sample post per channel.")
	PopulateSampleCmd.Flags().String(PARAM_AVATARS, "", "Optional. Path to folder with images to randomly pick as user profile image.")
	RootCmd.AddCommand(PopulateSampleCmd)
}

func paramError(name string) error {
	return fmt.Errorf("Invalid %s parameter", name)
}

func createChannelsW(
	encoder *json.Encoder,
	team string,
	channelType string,
	kind string,
	category string,
	channelNames *[]string,
	count int,
) *[]string {
	for i := 0; i < count; i++ {
		var line app.LineImportData
		if i < len(*channelNames) {
			line = createChannelW(team, channelType, kind, category, (*channelNames)[i])
			(*channelNames)[i] = *line.Channel.Name
		} else {
			line = createChannelW(team, channelType, kind, category, "")
			cn := append(*channelNames, *line.Channel.Name)
			channelNames = &cn
		}
		encoder.Encode(line)
	}
	return channelNames
}

func some(values *[]string, prob float32) *[]string {
	result := make([]string, len(*values))[:0]
	for _, v := range *values {
		if rand.Float32() < prob {
			result = append(result, v)
		}
	}
	return &result
}

func populateSampleCmdF(command *cobra.Command, args []string) error {
	a, err := InitDBCommandContextCobra(command)
	if err != nil {
		return err
	}
	defer a.Shutdown()

	admins, err := command.Flags().GetStringSlice(PARAM_ADMINS)
	if err != nil || len(admins) == 0 {
		return paramError(PARAM_ADMINS)
	}
	seed, err := command.Flags().GetInt64(PARAM_SEED)
	if err != nil {
		return paramError(PARAM_SEED)
	}
	users, err := command.Flags().GetInt(PARAM_USERS)
	if err != nil || users < 0 {
		return paramError(PARAM_USERS)
	}

	// Ignored for now
	deactivatedUsers, err := command.Flags().GetInt(PARAM_DEACTIVEATED)
	if err != nil || deactivatedUsers < 0 {
		return paramError(PARAM_DEACTIVEATED)
	}
	// Ignored for now
	guests, err := command.Flags().GetInt(PARAM_GUESTS)
	if err != nil || guests < 0 {
		return paramError(PARAM_GUESTS)
	}

	postsPerChannel, err := command.Flags().GetInt(PARAM_POSTS_PER_CHANNEL)
	if err != nil || postsPerChannel < 0 {
		return paramError(PARAM_POSTS_PER_CHANNEL)
	}
	profileImagesPath, err := command.Flags().GetString(PARAM_AVATARS)
	if err != nil {
		return paramError(PARAM_AVATARS)
	}
	profileImages := []string{}
	if profileImagesPath != "" {
		var profileImagesStat os.FileInfo
		profileImagesStat, err = os.Stat(profileImagesPath)
		if os.IsNotExist(err) {
			return errors.New("Profile images folder doesn't exists.")
		}
		if !profileImagesStat.IsDir() {
			return errors.New("profile-images parameters must be a folder path.")
		}
		var profileImagesFiles []os.FileInfo
		profileImagesFiles, err = ioutil.ReadDir(profileImagesPath)
		if err != nil {
			return errors.New("Invalid profile-images parameter")
		}
		for _, profileImage := range profileImagesFiles {
			profileImages = append(profileImages, path.Join(profileImagesPath, profileImage.Name()))
		}
		sort.Strings(profileImages)
	}

	bulkFile, err := os.OpenFile("logs/populate.sample.log", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("Unable to open import file for writing: %s.", err.Error())
	}

	encoder := json.NewEncoder(bulkFile)
	version := 1
	encoder.Encode(app.LineImportData{Type: "version", Version: &version})

	fake.Seed(seed)
	rand.Seed(seed)

	mainTeam := sqlstore.MAIN_TEAM_NAME

	fmt.Println("ADMINS:", admins)
	fmt.Println("MAIN TEAM:", mainTeam)

	encoder.Encode(createMainTeam(mainTeam, "I"))

	// Number of additional random channels of every kind
	additionalChannels := 10

	// Create open channels
	openChannelsNames := &[]string{
		"Worldr Technologies Ltd",
		"General",
		"Random",
		"Announcements",
	}
	openChannelsNames = createChannelsW(
		encoder,
		mainTeam,
		"O",
		"",
		"",
		openChannelsNames,
		len(*openChannelsNames)+additionalChannels,
	)

	// Create team channels
	teamChannelsNames := &[]string{
		"Management",
		"London Devs",
		"QA team",
		"DevOps",
	}
	teamChannelsNames = createChannelsW(
		encoder,
		mainTeam,
		"P",
		"team",
		"",
		teamChannelsNames,
		len(*teamChannelsNames),
	)

	// Create work channels
	workChannelsNames := &[]string{
		"Milestone 2",
		"Voice Calls",
		"Christmas Specials",
		"Website v1",
		"HR - Looking For Devs",
		"Quarantine Lifting Celebration",
	}
	workChannelsNames = createChannelsW(
		encoder,
		mainTeam,
		"P",
		"work",
		"",
		workChannelsNames,
		len(*workChannelsNames)+additionalChannels,
	)

	// Create private channels
	personalChannelsNames := createChannelsW(encoder, mainTeam, "P", "", "", &[]string{}, 10)

	allChannels := append(*openChannelsNames, *teamChannelsNames...)
	allChannels = append(allChannels, *workChannelsNames...)
	allChannels = append(allChannels, *personalChannelsNames...)

	fmt.Println("OPEN:", *openChannelsNames)
	fmt.Println("TEAM:", *teamChannelsNames)
	fmt.Println("WORK:", *workChannelsNames)
	fmt.Println("PRIVATE:", *personalChannelsNames)
	fmt.Println("ALL:", allChannels)

	adminUsers := []app.LineImportData{}
	catsPersonal := []string{
		"Everyday",
		"Friday drinks",
		"Banter",
		"",
		"",
	}
	catsOpen := []string{
		"Legal dept",
		"General",
		"Emergencies",
		"",
		"",
	}
	catsWork := []string{
		"ASAP",
		"Ongoing",
		"External",
		"",
		"",
	}

	// Create admins
	for i, v := range admins {
		// Categories for everything but team channels
		categories := createCategories(&catsPersonal, personalChannelsNames)
		add := createCategories(&catsWork, workChannelsNames)
		categories = append(categories, add...)
		add = createCategories(&catsOpen, openChannelsNames)
		categories = append(categories, add...)

		user := createUserW(i, mainTeam, &allChannels, &categories, profileImages, ADMIN, v)
		adminUsers = append(adminUsers, user)
		encoder.Encode(user)
	}

	// Remember members for all channels
	membersByChannel := map[string][]string{}
	for _, v := range allChannels {
		// Admins participate in all of the chats
		membersByChannel[v] = admins
	}

	randomUsers := []app.LineImportData{}

	// Create other users
	for i := 0; i < users; i++ {
		// All open teams and some teams, categories only for open ones
		add1 := some(teamChannelsNames, .5)
		channels := append(*openChannelsNames, *add1...)
		categories := createCategories(&catsOpen, openChannelsNames)

		// Some work channels AKA projects
		add1 = some(workChannelsNames, .5)
		channels = append(channels, *add1...)
		add2 := createCategories(&catsWork, add1)
		categories = append(categories, add2...)

		// Some private channels
		add1 = some(personalChannelsNames, .5)
		channels = append(channels, *add1...)
		add2 = createCategories(&catsPersonal, add1)
		categories = append(categories, add2...)

		user := createUserW(i, mainTeam, &channels, &categories, profileImages, "", "")
		randomUsers = append(randomUsers, user)
		encoder.Encode(user)

		for _, v := range channels {
			// Admins participate in all of the chats
			membersByChannel[v] = append(membersByChannel[v], *user.User.Username)
		}
	}

	allUsers := append(adminUsers, randomUsers...)

	// Create direct chats between all of the users
	for i := 0; i < len(allUsers); i++ {
		for j := i + 1; j < len(allUsers); j++ {
			u1 := allUsers[i]
			u2 := allUsers[j]
			if rand.Float32() < .5 {
				participants := []string{*u1.User.Username, *u2.User.Username}
				fmt.Println("Creating new direct channel:", participants, "with", postsPerChannel, "messages")
				encoder.Encode(createDirectChannelW(participants))
				// Create content for direct chats
				dates := sortedRandomDates(postsPerChannel)
				for k := 0; k < postsPerChannel; k++ {
					encoder.Encode(createDirectPost(participants, dates[k]))
				}
			}
		}
	}

	// Create content for non-direct chats
	for channel, members := range membersByChannel {
		fmt.Println("Adding", postsPerChannel, "message to channel ", channel, "with members: ", members)
		dates := sortedRandomDates(postsPerChannel)
		for i := 0; i < postsPerChannel; i++ {
			encoder.Encode(createPost(mainTeam, channel, members, dates[i]))
		}
	}

	fmt.Println("All set, saving...")

	// Save everything
	_, err = bulkFile.Seek(0, 0)
	if err != nil {
		return errors.New("Unable to read the temporary file.")
	}
	importErr, lineNumber := a.BulkImport(bulkFile, false, 2)
	if importErr != nil {
		return fmt.Errorf("%s: %s, %s (line: %d)", importErr.Where, importErr.Message, importErr.DetailedError, lineNumber)
	} else {
		fmt.Println("Import successful!")
	}
	err = bulkFile.Close()
	if err != nil {
		return fmt.Errorf("Unable to close the output file: %s", err.Error())
	}

	return nil
}

func createCategories(cats *[]string, channels *[]string) []app.CategoryImportData {
	res := map[string]string{}
	for i, channel := range *channels {
		cat := (*cats)[i%len(*cats)]
		// Only assign category if the name is not empty
		if len(cat) > 0 {
			res[channel] = cat
		}
	}
	list := make([]app.CategoryImportData, len(res))[:0]
	for k, v := range res {
		channel := k
		cat := v
		list = append(list, app.CategoryImportData{
			Channel: &channel,
			Name:    &cat,
		})
	}
	return list
}

func createDirectChannelW(members []string) app.LineImportData {
	header := "Direct " + strings.Join(members, ", ")

	channel := app.DirectChannelImportData{
		Members: &members,
		Header:  &header,
	}
	return app.LineImportData{
		Type:          "direct_channel",
		DirectChannel: &channel,
	}
}

// channelType is "P" for "private" or "O" for "open"
func createChannelW(teamName string, channelType string, channelKind string, channelCategory string, fixedName string) app.LineImportData {
	var displayName string
	if len(fixedName) > 0 {
		displayName = fixedName
	} else {
		displayName = fake.Title()
	}
	name := strings.ToLower(strings.ReplaceAll(displayName, " ", "-"))
	header := fake.Paragraph()
	purpose := fake.Paragraph()

	if len(purpose) > 250 {
		purpose = purpose[0:250]
	}

	fmt.Println("Creating new channel:", name)

	channel := app.ChannelImportData{
		Team:        &teamName,
		Name:        &name,
		DisplayName: &displayName,
		Type:        &channelType,
		Kind:        &channelKind,
		Header:      &header,
		Purpose:     &purpose,
	}
	return app.LineImportData{
		Type:    "channel",
		Channel: &channel,
	}
}

func createTeamMembershipW(teamChannels *[]string, teamName *string, teamRoles string, channelRoles string) app.UserTeamImportData {
	channels := []app.UserChannelImportData{}
	for _, channelName := range *teamChannels {
		channels = append(channels, createChannelMembershipW(channelName, channelRoles))
	}

	return app.UserTeamImportData{
		Name:     teamName,
		Roles:    &teamRoles,
		Channels: &channels,
	}
}

func createChannelMembershipW(channelName string, roles string) app.UserChannelImportData {
	favorite := rand.Intn(5) == 0
	return app.UserChannelImportData{
		Name:     &channelName,
		Roles:    &roles,
		Favorite: &favorite,
	}
}

func createUserW(
	idx int,
	teamName string,
	channels *[]string,
	categories *[]app.CategoryImportData,
	profileImages []string,
	userType string,
	fixedUsername string,
) app.LineImportData {
	firstName := fake.FirstName()
	lastName := fake.LastName()
	position := fake.JobTitle()
	location := fake.Country()
	phoneNumber := fake.Phone()
	workRole := fake.JobTitle()
	biography := fake.Paragraph()

	systemRoles := "system_user"
	teamRoles := "team_user"
	channelRoles := "channel_user"

	var username string
	if len(fixedUsername) > 0 {
		username = fixedUsername
	} else {
		username = fmt.Sprintf("user-%d", idx)
	}
	switch userType {
	case GUEST_USER:
		username = fmt.Sprintf("guest-%d", idx)
		systemRoles = "system_guest"
		teamRoles = "team_guest"
		channelRoles = "channel_guest"
	case DEACTIVATED_USER:
		username = fmt.Sprintf("deactivated-%d", idx)
	case ADMIN:
		systemRoles = "system_user system_admin"
		teamRoles = "team_user team_admin"
		channelRoles = "channel_user channel_admin"
	}

	email := fmt.Sprintf("%s@sample.worldr.com", username)
	password := username
	socialMedia := "https://www.linkedin.com/in/" + username

	fmt.Println("Creating new user:", username, systemRoles)

	// The 75% of the users have custom profile image
	var profileImage *string = nil
	if rand.Intn(4) != 0 {
		profileImageSelector := rand.Int()
		if len(profileImages) > 0 {
			profileImage = &profileImages[profileImageSelector%len(profileImages)]
		}
	}

	useMilitaryTime := "false"
	if idx != 0 && rand.Intn(2) == 0 {
		useMilitaryTime = "true"
	}

	collapsePreviews := "false"
	if idx != 0 && rand.Intn(2) == 0 {
		collapsePreviews = "true"
	}

	messageDisplay := "clean"
	if idx != 0 && rand.Intn(2) == 0 {
		messageDisplay = "compact"
	}

	channelDisplayMode := "full"
	if idx != 0 && rand.Intn(2) == 0 {
		channelDisplayMode = "centered"
	}

	// Some users have nicknames
	nickname := ""
	if rand.Intn(5) == 0 {
		nickname = fake.Company()
	}

	// skip tutorial altogether
	tutorialStep := "999"

	var deleteAt int64
	if userType == DEACTIVATED_USER {
		deleteAt = model.GetMillis()
	}

	team := createTeamMembershipW(channels, &teamName, teamRoles, channelRoles)
	team.Categories = categories
	teams := []app.UserTeamImportData{team}

	user := app.UserImportData{
		ProfileImage:       profileImage,
		Username:           &username,
		Email:              &email,
		Password:           &password,
		Nickname:           &nickname,
		FirstName:          &firstName,
		LastName:           &lastName,
		Position:           &position,
		Roles:              &systemRoles,
		Teams:              &teams,
		UseMilitaryTime:    &useMilitaryTime,
		CollapsePreviews:   &collapsePreviews,
		MessageDisplay:     &messageDisplay,
		ChannelDisplayMode: &channelDisplayMode,
		TutorialStep:       &tutorialStep,
		DeleteAt:           &deleteAt,
		Location:           &location,
		PhoneNumber:        &phoneNumber,
		WorkRole:           &workRole,
		SocialMedia:        &socialMedia,
		Biography:          &biography,
	}
	return app.LineImportData{
		Type: "user",
		User: &user,
	}
}

// teamType is "I" for "invite" and "O" for "open"
func createMainTeam(name string, teamType string) app.LineImportData {
	displayName := fake.Word()
	allowOpenInvite := false

	description := fake.Paragraph()
	if len(description) > 255 {
		description = description[0:255]
	}

	fmt.Println("Creating new team:", name)

	team := app.TeamImportData{
		DisplayName:     &displayName,
		Name:            &name,
		AllowOpenInvite: &allowOpenInvite,
		Description:     &description,
		Type:            &teamType,
	}
	return app.LineImportData{
		Type: "team",
		Team: &team,
	}
}
