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
	Use:   "populate",
	Short: "Generate data",
	RunE:  populateSampleCmdF,
}

const (
	PARAM_SEED              = "seed"
	PARAM_USERS             = "users"
	PARAM_GUESTS            = "guests"
	PARAM_DEACTIVEATED      = "deactivated-users"
	PARAM_POSTS_PER_CHANNEL = "posts-per-channel"
	PARAM_AVATARS           = "profile-images"
	PARAM_CHANNEL_AVATARS   = "channel-images"
	PARAM_CONFIG_FILE       = "configuration-file"
)

func init() {
	PopulateSampleCmd.Flags().Int64P(PARAM_SEED, "s", 0, "Seed used for generating the random data (Different seeds generate different data).")
	PopulateSampleCmd.Flags().IntP(PARAM_USERS, "u", 0, "The number of random sample users.")
	PopulateSampleCmd.Flags().IntP(PARAM_GUESTS, "g", 0, "The number of random sample guests.")
	PopulateSampleCmd.Flags().Int(PARAM_DEACTIVEATED, 0, "The number of random deactivated users.")
	PopulateSampleCmd.Flags().Int(PARAM_POSTS_PER_CHANNEL, 0, "The number of random sample post per channel.")
	PopulateSampleCmd.Flags().String(PARAM_AVATARS, "", "Optional. Path to folder with images to randomly pick as user profile image.")
	PopulateSampleCmd.Flags().String(PARAM_CHANNEL_AVATARS, "", "Optional. Path to folder with images to randomly pick as channel image.")
	PopulateSampleCmd.Flags().String(PARAM_CONFIG_FILE, "", "JSON configuration file to pick data from.")

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
	index int,
	images *[]string,
) *[]string {
	for i := 0; i < count; i++ {
		var line app.LineImportData
		if i < len(*channelNames) {
			line = createChannelW(team, channelType, kind, category, (*channelNames)[i], index+i, images)
			(*channelNames)[i] = *line.Channel.Name
		} else {
			line = createChannelW(team, channelType, kind, category, "", index+i, images)
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

func prepareImages(imagesFolder string, targetList *[]string, targetMap *map[string]string) error {
	var imagesStat os.FileInfo
	imagesStat, err := os.Stat(imagesFolder)
	if os.IsNotExist(err) {
		return errors.New("images folder doesn't exists.")
	}
	if !imagesStat.IsDir() {
		return errors.New("images parameter must be a folder path.")
	}
	var imagesFiles []os.FileInfo
	imagesFiles, err = ioutil.ReadDir(imagesFolder)
	if err != nil {
		return errors.New("Invalid images parameter")
	}
	images := []string{}
	for _, image := range imagesFiles {
		var fileName string = image.Name()
		file := path.Join(imagesFolder, fileName)
		images = append(images, file)
		if targetMap != nil {
			dot := strings.LastIndex(fileName, ".")
			if dot > 0 {
				fileName = fileName[0:dot]
			}
			(*targetMap)[fileName] = file
		}
	}
	sort.Strings(images)
	*targetList = images
	return nil
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

type User struct {
	Administrator      bool   `json:"administrator"`
	Biography          string `json:"biography"`
	ChannelDisplayMode string `json:"channel display mode"`
	CollapsePreviews   string `json:"collapse previews"`
	Email              string `json:"email"`
	FirstName          string `json:"first name"`
	LastName           string `json:"last name"`
	Location           string `json:"location"`
	MessageDisplay     string `json:"message display"`
	Nickname           string `json:"nickname"`
	Password           string `json:"password"`
	PhoneNumber        string `json:"phone number"`
	Position           string `json:"position"`
	SocialMedia        string `json:"social media"`
	TutorialStep       string `json:"tutorial step"`
	UseMilitaryTime    string `json:"use military time"`
	Username           string `json:"username"`
}

type InitialData struct {
	OpenChannelsNames     []string `json:"open channels names"`
	TeamChannelsNames     []string `json:"team channels names"`
	WorkChannelsNames     []string `json:"work channels names"`
	PersonalChannelsNames []string `json:"personal channels names"`
	Administrators        []User   `json:"administrators"`
	Users                 []User   `json:"users"`
}

var usersFile *os.File

func populateSampleCmdF(command *cobra.Command, args []string) error {
	a, err := InitDBCommandContextCobra(command)
	if err != nil {
		return err
	}
	defer a.Shutdown()

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
	profileImages := &[]string{}
	profileImagesMap := &map[string]string{}
	profileImagesUsed := map[string]bool{}
	if profileImagesPath != "" {
		err = prepareImages(profileImagesPath, profileImages, profileImagesMap)
		if err != nil {
			return paramError(PARAM_CHANNEL_AVATARS)
		}
	}
	fmt.Println("Profile images:", len(*profileImages))

	channelImagesPath, err := command.Flags().GetString(PARAM_CHANNEL_AVATARS)
	if err != nil {
		return paramError(PARAM_CHANNEL_AVATARS)
	}
	channelImages := &[]string{}
	if channelImagesPath != "" {
		err = prepareImages(channelImagesPath, channelImages, nil)
		if err != nil {
			return paramError(PARAM_CHANNEL_AVATARS)
		}
	}
	fmt.Println("Channel images:", len(*channelImages))

	// Get data from a configuration file.
	// TODO: this should define ALL the options.
	var initData InitialData
	configurationFilePath, err := command.Flags().GetString(PARAM_CONFIG_FILE)
	if configurationFilePath != "" {
		if fileExists(configurationFilePath) {
			fmt.Println("Reading config file from", configurationFilePath)
			cfgData, err1 := ioutil.ReadFile(configurationFilePath)
			if err1 != nil {
				return paramError(PARAM_CONFIG_FILE)
			}
			err = json.Unmarshal(cfgData, &initData)
			if err != nil {
				fmt.Println("error:", err)
				return paramError(PARAM_CONFIG_FILE)
			}
		} else {
			fmt.Println("error: configuration file not found or is not readable.")
			return paramError(PARAM_CONFIG_FILE)
		}
	} else {
		fmt.Println(fmt.Printf("Configuration file %q does not exist (or is a directory)", configurationFilePath))
		initData.OpenChannelsNames = []string{
			"Worldr Technologies Ltd",
			"General",
			"Random",
			"Announcements",
			"Briefings",
			"Special reports",
			"Indicators",
		}
		initData.PersonalChannelsNames = []string{
			"Management",
			"London Devs",
			"QA team",
			"DevOps",
			"Leaders",
			"International",
			"Ministry of silly walks",
		}
		initData.TeamChannelsNames = []string{
			"Milestone 2",
			"Voice Calls",
			"Christmas Specials",
			"Website v1",
			"HR - Looking For Devs",
			"Quarantine Lifting Celebration",
			"Letters",
			"Obituaries",
			"Graphic detail",
			"Calls recap",
		}
		initData.WorkChannelsNames = []string{
			"Puzzling Muzzle",
			"Buzzing Embezzlement",
			"Nitwit Blubber Oddment Tweak",
			"Weather In Llanfairpwllgwyngyllgogerychwyrndrobwllllantysiliogogogoch",
			"The Witcher 3 endings",
			"Elvis Lives",
			"Paul Is Dead",
			"Cthulhu Rises",
		}
	}

	if err != nil || len(initData.Administrators) == 0 {
		return paramError("Adminisrators")
	}

	bulkFile, err := os.OpenFile("logs/populate", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("Unable to open import file for writing: %s.", err.Error())
	}
	usersFile, err = os.OpenFile("logs/populate.users", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("Unable to open users file for writing: %s.", err.Error())
	}

	encoder := json.NewEncoder(bulkFile)
	version := 1
	encoder.Encode(app.LineImportData{Type: "version", Version: &version})

	seed, err := command.Flags().GetInt64(PARAM_SEED)
	fake.Seed(seed)
	rand.Seed(seed)

	mainTeam := sqlstore.MAIN_TEAM_NAME

	fmt.Println("MAIN TEAM:", mainTeam)

	encoder.Encode(createMainTeam(mainTeam, "I"))

	// Create open channels
	openChannelsNames := createChannelsW(
		encoder,
		mainTeam,
		"O",
		"",
		"",
		&(initData.OpenChannelsNames),
		len(initData.OpenChannelsNames),
		0,
		channelImages,
	)

	// Create team channels
	teamChannelsNames := createChannelsW(
		encoder,
		mainTeam,
		"P",
		"team",
		"",
		&(initData.TeamChannelsNames),
		len(initData.TeamChannelsNames),
		50,
		channelImages,
	)

	// Create work channels
	workChannelsNames := createChannelsW(
		encoder,
		mainTeam,
		"P",
		"work",
		"",
		&(initData.WorkChannelsNames),
		len(initData.WorkChannelsNames),
		100,
		channelImages,
	)

	// Create private channels
	personalChannelsNames := createChannelsW(
		encoder,
		mainTeam,
		"P",
		"",
		"",
		&(initData.PersonalChannelsNames),
		len(initData.PersonalChannelsNames),
		150,
		channelImages,
	)

	allChannels := append(*openChannelsNames, *teamChannelsNames...)
	allChannels = append(allChannels, *workChannelsNames...)
	allChannels = append(allChannels, *personalChannelsNames...)

	fmt.Println("OPEN:", *openChannelsNames)
	fmt.Println("TEAM:", *teamChannelsNames)
	fmt.Println("WORK:", *workChannelsNames)
	fmt.Println("PERSONAL:", *personalChannelsNames)
	fmt.Println("ALL:", allChannels)

	adminUsers := []app.LineImportData{}
	catsPersonal := []string{
		"Everyday",
		"Friday drinks",
		"Banter",
		"Tech guys",
		"",
	}
	catsOpen := []string{
		"Legal dept",
		"General",
		"Emergencies",
		"",
	}
	catsWork := []string{
		"ASAP",
		"Ongoing",
		"External",
		"",
	}

	// Create administrators from the configuration file.
	admins := make([]string, len(initData.Administrators))
	for i, admin := range initData.Administrators {
		// Categories for everything but team channels
		categories := createCategories(&catsPersonal, personalChannelsNames)
		add := createCategories(&catsWork, workChannelsNames)
		categories = append(categories, add...)
		add = createCategories(&catsOpen, openChannelsNames)
		categories = append(categories, add...)

		var avatar *string
		if file, exists := (*profileImagesMap)[admin.Username]; exists {
			avatar = &file
			profileImagesUsed[file] = true
		}

		fmt.Println(fmt.Sprintf("Registering admin %s", admin.Username))

		user := createConfigurationUserW(admin, mainTeam, &allChannels, &categories, avatar, true)
		adminUsers = append(adminUsers, user)
		encoder.Encode(user)

		admins[i] = admin.Username
	}

	// Create users from the configuration file.
	for _, u := range initData.Users {
		// Categories for everything but team channels
		categories := createCategories(&catsPersonal, personalChannelsNames)
		add := createCategories(&catsWork, workChannelsNames)
		categories = append(categories, add...)
		add = createCategories(&catsOpen, openChannelsNames)
		categories = append(categories, add...)

		var avatar *string
		if file, exists := (*profileImagesMap)[u.Username]; exists {
			avatar = &file
			profileImagesUsed[file] = true
		}

		fmt.Println(fmt.Sprintf("Registering user %s", u.Username))

		user := createConfigurationUserW(u, mainTeam, &allChannels, &categories, avatar, false)
		encoder.Encode(user)
	}

	// Remember members for all channels
	membersByChannel := map[string][]string{}
	for _, v := range allChannels {
		// Admins participate in all of the chats
		membersByChannel[v] = admins
	}

	if users > 0 {
		randomUsers := []app.LineImportData{}

		// Remove used personal avatars from the list for random users
		temp := make([]string, len(*profileImages))[:0]
		for _, v := range *profileImages {
			if _, exists := profileImagesUsed[v]; !exists {
				temp = append(temp, v)
			} else {
				fmt.Println("Evicting personal avatar from the list for random users:", v)
			}
		}
		profileImages = &temp

		// Create other users
		for i := 0; i < users; i++ {
			// All open channels and some team channels, categories only for open ones
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

			user := createUserW(i, mainTeam, &channels, &categories, profileImages, "", "", "")
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
	err = usersFile.Close()
	if err != nil {
		return fmt.Errorf("Unable to close the users file: %s", err.Error())
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
func createChannelW(
	teamName string,
	channelType string,
	channelKind string,
	channelCategory string,
	fixedName string,
	index int,
	images *[]string,
) app.LineImportData {
	var displayName string
	if len(fixedName) > 0 {
		displayName = fixedName
	} else {
		displayName = fake.Title()
	}
	limit := model.CHANNEL_DISPLAY_NAME_MAX_RUNES - 5
	if len(displayName) > limit {
		displayName = displayName[0:limit]
	}
	chunks := strings.Split(displayName, " ")
	if len(chunks) > 2 {
		chunks = chunks[0:2]
	}
	name := strings.ToLower(strings.Join(chunks, "-"))
	header := fake.Paragraph()
	purpose := fake.Paragraph()

	if len(purpose) > 250 {
		purpose = purpose[0:250]
	}

	var image *string = nil
	imgCount := len(*images)
	if imgCount == 1 {
		// if a single avatar id given, always set it
		image = &(*images)[0]
	} else if imgCount > 0 {
		selector := rand.Int()
		image = &(*images)[selector%imgCount]
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
		Image:       image,
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
	profileImages *[]string,
	userType string,
	fixedUsername string,
	fixedName string,
) app.LineImportData {
	firstName := fake.FirstName()
	lastName := fake.LastName()
	if len(fixedName) > 0 {
		chunks := strings.Split(fixedName, " ")
		firstName = chunks[0]
		if len(chunks) > 1 {
			lastName = chunks[1]
		} else {
			lastName = ""
		}
	}
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

	var profileImage *string = nil
	imgCount := len(*profileImages)
	if imgCount == 1 {
		// if a single avatar id given, always set it
		profileImage = &(*profileImages)[0]
	} else if imgCount > 0 {
		// if the name is fixed, choose an avatar, otherwise choose avatar in the 75% of cases
		if len(fixedUsername) > 0 || rand.Intn(4) != 0 {
			profileImageSelector := rand.Int()
			profileImage = &(*profileImages)[profileImageSelector%imgCount]
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

	notify := defaultNotifyProps()

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
		NotifyProps:        notify,
	}
	return app.LineImportData{
		Type: "user",
		User: &user,
	}
}

func ptrStr(s string) *string {
	return &s
}

func defaultNotifyProps() *app.UserNotifyPropsImportData {
	return &app.UserNotifyPropsImportData{
		Desktop:          ptrStr("all"),
		DesktopSound:     ptrStr("true"),
		Email:            ptrStr("false"),
		Mobile:           ptrStr("all"),
		MobilePushStatus: ptrStr("online"),
		ChannelTrigger:   ptrStr("true"),
		CommentsTrigger:  ptrStr("any"),
		MentionKeys:      ptrStr("@"),
		FirstName:        ptrStr("true"),
	}
}

func createConfigurationUserW(
	usr User,
	teamName string,
	channels *[]string,
	categories *[]app.CategoryImportData,
	avatar *string,
	isAdmin bool,
) app.LineImportData {

	systemRoles := "system_user"
	teamRoles := "team_user"
	channelRoles := "channel_user"
	if isAdmin {
		systemRoles += " system_admin"
		teamRoles += " team_admin"
		channelRoles += " channel_admin"
	}
	password := "W2020-" + fake.CharactersN(4)

	team := createTeamMembershipW(channels, &teamName, teamRoles, channelRoles)
	team.Categories = categories
	teams := []app.UserTeamImportData{team}

	usersFile.WriteString(fmt.Sprintf("Username: %s\n", usr.Username))
	usersFile.WriteString(fmt.Sprintf("Password: %s\n", password))
	usersFile.WriteString("\n")

	notify := defaultNotifyProps()

	user := app.UserImportData{
		ProfileImage:       avatar,
		Username:           &usr.Username,
		Email:              &usr.Email,
		Password:           &password, //TODO: replace with something appropriately random
		Nickname:           &usr.Nickname,
		FirstName:          &usr.FirstName,
		LastName:           &usr.LastName,
		Position:           &usr.Position,
		Roles:              &systemRoles,
		Teams:              &teams,
		UseMilitaryTime:    &usr.UseMilitaryTime,
		CollapsePreviews:   &usr.CollapsePreviews,
		MessageDisplay:     &usr.MessageDisplay,
		ChannelDisplayMode: &usr.ChannelDisplayMode,
		TutorialStep:       &usr.TutorialStep,
		DeleteAt:           nil,
		Location:           &usr.Location,
		PhoneNumber:        &usr.PhoneNumber,
		WorkRole:           &usr.Position,
		SocialMedia:        &usr.SocialMedia,
		Biography:          &usr.Biography,
		NotifyProps:        notify,
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
