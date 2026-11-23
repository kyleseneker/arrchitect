package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
)

// exportFakeSabnzbd serves config as get_config's answer and exports it.
func exportFakeSabnzbd(t *testing.T, config string) (Result, *downloadsv1alpha1.Sabnzbd) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if r.Form.Get("apikey") != "k" || r.Form.Get("mode") != "get_config" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"config":` + config + `}`))
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "sabnzbd", URL: srv.URL, APIKey: "k"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*downloadsv1alpha1.Sabnzbd)
	require.True(t, ok)
	return res, cfg
}

func TestSabnzbdExportsMiscServersAndCategories(t *testing.T) {
	res, cfg := exportFakeSabnzbd(t, `{
			"misc":{"complete_dir":"/done","download_dir":"/incomplete","tmp_dir":"","script_dir":"/scripts",
				"bandwidth_max":"50M","pre_check":1,"pause_on_post_processing":false,"auto_sort":"avg_age asc",
				"fail_hopeless_jobs":1,"pause_on_pwrar":2,"action_on_unwanted_extensions":7,"unwanted_extensions_mode":0,
				"unwanted_extensions":["exe","lnk"],"cleanup_list":[],"host_whitelist":["sabnzbd.media"],
				"history_retention_option":"days-delete","history_retention_number":30,"permissions":"775","propagation_delay":0},
			"servers":[{"name":"Primary","host":"news","port":563,"ssl":1,"connections":20,"priority":0,"retention":3000,
				"enable":1,"username":"me","password":"**********","ssl_verify":2,"optional":0,"required":1,"timeout":60,
				"ssl_ciphers":"AES128"}],
			"categories":[{"name":"*","dir":"","script":"None","priority":0,"pp":"3"},
				{"name":"tv","dir":"tv","script":"","priority":-100,"pp":""}]}`)
	assert.Equal(t, &downloadsv1alpha1.SabnzbdFolders{CompleteDir: "/done", IncompleteDir: "/incomplete", ScriptDir: "/scripts"}, cfg.Spec.Folders)
	assert.Equal(t, "50M", cfg.Spec.General.DownloadSpeedLimit)
	assert.True(t, *cfg.Spec.General.PreCheck)
	assert.False(t, *cfg.Spec.General.PauseOnPostProcessing)
	assert.True(t, *cfg.Spec.General.AutoSort)
	general := cfg.Spec.General
	assert.True(t, *general.FailHopelessJobs)
	assert.Equal(t, "fail", general.EncryptedRarAction)
	assert.Equal(t, "blocklist", general.UnwantedExtensionsMode)
	assert.Empty(t, general.UnwantedExtensionsAction)
	assert.Contains(t, res.Warnings, "the config can't express action_on_unwanted_extensions 7; unwantedExtensionsAction is left out")
	assert.Equal(t, []string{"exe", "lnk"}, general.UnwantedExtensions)
	assert.Empty(t, general.CleanupList)
	assert.Equal(t, []string{"sabnzbd.media"}, general.HostWhitelist)
	assert.Equal(t, "days-delete", general.HistoryRetention)
	assert.Equal(t, int32(30), general.HistoryRetentionNumber)
	assert.Equal(t, "775", general.Permissions)
	assert.Equal(t, int32(0), *general.PropagationDelay)

	require.Len(t, cfg.Spec.Servers, 1)
	server := cfg.Spec.Servers[0]
	assert.Equal(t, int32(563), server.Port)
	assert.True(t, *server.SSL)
	assert.Equal(t, int32(3000), *server.Retention)
	assert.Equal(t, "medium", server.SSLVerify)
	assert.False(t, *server.Optional)
	assert.True(t, *server.Required)
	assert.Equal(t, int32(60), server.Timeout)
	assert.Equal(t, "AES128", server.SSLCiphers)
	assert.Equal(t, "me", res.Secret.StringData[server.UsernameSecretRef.Key])
	assert.Equal(t, []string{"server-primary-password"}, res.Unfilled)

	assert.Equal(t, []downloadsv1alpha1.SabnzbdCategory{
		{Name: "*", Script: "None", Priority: "normal", PostProcessing: "delete"},
		{Name: "tv", Dir: "tv", Priority: "default"},
	}, cfg.Spec.Categories)
}

func TestSabnzbdExportsLoginQuotaAndDuplicates(t *testing.T) {
	res, cfg := exportFakeSabnzbd(t, `{"misc":{"username":"admin","password":"**********",
		"no_dupes":4,"no_smart_dupes":"2","dupes_propercheck":0,"pre_script":"check.sh","end_queue_script":"None",
		"quota_size":"500G","quota_period":"w","quota_day":"3 04:30","quota_resume":1}}`)

	general := cfg.Spec.General
	assert.Equal(t, "tag", general.DuplicateDetection)
	assert.Equal(t, "pause", general.SmartDuplicateDetection)
	assert.False(t, *general.AllowProperDuplicates)
	assert.Equal(t, "check.sh", general.PreQueueScript)
	assert.Equal(t, "None", general.QueueCompleteScript)

	assert.Equal(t, downloadsv1alpha1.SabnzbdQuota{
		Size: "500G", Period: "week", ResetDay: 3, ResetTime: "04:30", Resume: new(true),
	}, cfg.Spec.Quota)

	assert.Equal(t, "admin", res.Secret.StringData[cfg.Spec.Login.UsernameSecretRef.Key])
	assert.Equal(t, []string{"login-password"}, res.Unfilled)
}

func TestSabnzbdExportsTheQuotaResetAsSABnzbdReadsIt(t *testing.T) {
	for _, tt := range []struct {
		day, period string
		want        downloadsv1alpha1.SabnzbdQuota
		warning     string
	}{
		{"", "m", downloadsv1alpha1.SabnzbdQuota{Size: "1T", Period: "month", Resume: new(false)}, ""},
		{"15", "m", downloadsv1alpha1.SabnzbdQuota{Size: "1T", Period: "month", ResetDay: 15, Resume: new(false)}, ""},
		{"05:30", "d", downloadsv1alpha1.SabnzbdQuota{
			Size: "1T", Period: "day", ResetDay: 5, ResetTime: "05:30", Resume: new(false),
		}, ""},
		{"9", "w", downloadsv1alpha1.SabnzbdQuota{Size: "1T", Period: "week", Resume: new(false)},
			`the config can't express the quota reset day "9"; resetDay is left out`},
		{"1 25:00", "m", downloadsv1alpha1.SabnzbdQuota{Size: "1T", Period: "month", ResetDay: 1, Resume: new(false)},
			`the config can't express the quota reset time "25:00"; resetTime is left out`},
	} {
		res, cfg := exportFakeSabnzbd(t, `{"misc":{"quota_size":"1T","quota_period":"`+tt.period+`","quota_day":"`+tt.day+`"}}`)
		assert.Equal(t, tt.want, cfg.Spec.Quota, tt.day)
		if tt.warning != "" {
			assert.Contains(t, res.Warnings, tt.warning)
		}
	}

	_, cfg := exportFakeSabnzbd(t, `{"misc":{"quota_size":"","quota_period":"m"}}`)
	assert.Zero(t, cfg.Spec.Quota, "a quota without a size is off")
}
