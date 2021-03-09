package api4

import (
	"testing"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/stretchr/testify/assert"
)

func TestCompanyInfo(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	t.Run("success", func(t *testing.T) {
		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.CompanyConfig = "company.json"
		})
		content, r := th.WClient.GetCompanyInfo()
		CheckNoError(t, r)
		assert.True(t, len(content.Server) > 0, "server address is empty")
		assert.True(t, len(content.Key) > 0, "server key is empty")
		assert.Equal(t, "local", content.Deployment, "unexpected deployment name")
	})
}
