package sabnzbd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Folders maps SabnzbdFolders fields to their misc keys.
var Folders = map[string]string{
	"completeDir": "complete_dir", "incompleteDir": "download_dir", "tempDownloadDir": "tmp_dir",
	"nzbBackupDir": "nzb_backup_dir", "scriptDir": "script_dir",
}

// Switches maps SabnzbdGeneral's on/off settings to their misc keys.
var Switches = map[string]string{
	"preCheck":                  "pre_check",
	"pauseOnPostProcessing":     "pause_on_post_processing",
	"topOnly":                   "top_only",
	"failHopelessJobs":          "fail_hopeless_jobs",
	"directUnpack":              "direct_unpack",
	"enableAllPar":              "enable_all_par",
	"sfvCheck":                  "sfv_check",
	"safePostProcessing":        "safe_postproc",
	"enableRecursive":           "enable_recursive",
	"flatUnpack":                "flat_unpack",
	"ignoreSamples":             "ignore_samples",
	"deobfuscateFinalFilenames": "deobfuscate_final_filenames",
	"scriptCanFail":             "script_can_fail",
	"allowProperDuplicates":     "dupes_propercheck",
}

// Texts maps SabnzbdGeneral's text settings to their misc keys.
var Texts = map[string]string{
	"downloadSpeedLimit":  "bandwidth_max",
	"historyRetention":    "history_retention_option",
	"permissions":         "permissions",
	"downloadFree":        "download_free",
	"completeFree":        "complete_free",
	"cacheLimit":          "cache_limit",
	"preQueueScript":      "pre_script",
	"queueCompleteScript": "end_queue_script",
}

// Numbers maps SabnzbdGeneral's number settings to their misc keys.
var Numbers = map[string]string{
	"historyRetentionNumber": "history_retention_number",
	"propagationDelay":       "propagation_delay",
}

// Lists maps SabnzbdGeneral's list settings to their misc keys. SABnzbd takes
// a list comma-separated and returns it as an array.
var Lists = map[string]string{
	"unwantedExtensions": "unwanted_extensions",
	"cleanupList":        "cleanup_list",
	"hostWhitelist":      "host_whitelist",
}

// Choice is a setting SABnzbd numbers, with the spec's name for each number.
type Choice struct {
	Key    string
	Values map[string]string
}

var jobActions = map[string]string{"off": "0", "pause": "1", "fail": "2"}

var duplicateActions = map[string]string{"off": "0", "discard": "1", "pause": "2", "fail": "3", "tag": "4"}

// Choices maps SabnzbdGeneral's named choices to their misc keys and values.
var Choices = map[string]Choice{
	"encryptedRarAction":       {"pause_on_pwrar", jobActions},
	"unwantedExtensionsAction": {"action_on_unwanted_extensions", jobActions},
	"unwantedExtensionsMode":   {"unwanted_extensions_mode", map[string]string{"blocklist": "0", "allowlist": "1"}},
	"duplicateDetection":       {"no_dupes", duplicateActions},
	"smartDuplicateDetection":  {"no_smart_dupes", duplicateActions},
}

// QuotaPeriods maps quota period names to quota_period values.
var QuotaPeriods = map[string]string{"day": "d", "week": "w", "month": "m", "manual": "x"}

// SSLVerify maps sslVerify names to a server's ssl_verify values.
var SSLVerify = map[string]string{"strict": "3", "medium": "2", "minimal": "1", "disabled": "0"}

// PostProcessing maps postProcessing names to a category's pp values.
var PostProcessing = map[string]string{"download": "0", "repair": "1", "unpack": "2", "delete": "3"}

// Priorities maps priority names to a category's priority values.
var Priorities = map[string]string{"default": "-100", "paused": "-2", "low": "-1", "normal": "0", "high": "1", "force": "2"}

// AppriseEvents are SABnzbd's names for the events Apprise can notify about.
var AppriseEvents = map[string]string{
	"startup": "startup", "download": "download", "pauseResume": "pause_resume", "pp": "pp",
	"complete": "complete", "failed": "failed", "diskFull": "disk_full", "quota": "quota",
	"newLogin": "new_login", "warning": "warning", "error": "error", "queueDone": "queue_done", "other": "other",
}

type Client struct {
	hc     *engine.HTTPClient
	apiKey string
}

func NewClient(hc *engine.HTTPClient, apiKey string) *Client {
	return &Client{
		hc:     hc,
		apiKey: apiKey,
	}
}

// apiParams returns form values containing the common API parameters.
// The API key is kept out of the URL and sent in the POST body instead,
// so it does not leak into server/proxy access logs.
func (c *Client) apiParams(mode string, extra string) url.Values {
	params := url.Values{}
	params.Set("mode", mode)
	params.Set("apikey", c.apiKey)
	params.Set("output", "json")
	if extra != "" {
		parsed, err := url.ParseQuery(extra)
		if err == nil {
			for k, vs := range parsed {
				for _, v := range vs {
					params.Set(k, v)
				}
			}
		}
	}
	return params
}

// Ping reads one setting. That needs the full API key, so a wrong key fails
// here; SABnzbd answers it with HTTP 200 and status false.
func (c *Client) Ping(ctx context.Context) error {
	data, err := c.hc.DoForm(ctx, "/api", c.apiParams("get_config", "section=misc&keyword=port"))
	if err != nil {
		return err
	}
	return refusal(data, "sabnzbd refused the request")
}

func (c *Client) GetConfig(ctx context.Context) (map[string]any, error) {
	data, err := c.hc.DoForm(ctx, "/api", c.apiParams("get_config", ""))
	if err != nil {
		return nil, err
	}
	if err := refusal(data, "sabnzbd refused the request"); err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if config, ok := result["config"].(map[string]any); ok {
		return config, nil
	}
	return result, nil
}

func (c *Client) SetConfig(ctx context.Context, section, keyword, value string) error {
	params := c.apiParams("set_config", "")
	params.Set("section", section)
	params.Set("keyword", keyword)
	params.Set("value", value)
	return c.write(ctx, params)
}

// SetNamed creates or updates one entry of a named section (servers,
// categories) in a single call, the only form SABnzbd accepts for them.
func (c *Client) SetNamed(ctx context.Context, section, name string, values map[string]string) error {
	params := c.apiParams("set_config", "")
	params.Set("section", section)
	params.Set("keyword", name)
	for k, v := range values {
		params.Set(k, v)
	}
	return c.write(ctx, params)
}

// write sends a configuration change. SABnzbd reports a rejected change with
// HTTP 200 and {"status": false, "error": ...}, so the body decides success.
func (c *Client) write(ctx context.Context, params url.Values) error {
	data, err := c.hc.DoForm(ctx, "/api", params)
	if err != nil {
		return err
	}
	return refusal(data, "sabnzbd rejected the change")
}

// refusal returns the error SABnzbd reports with HTTP 200 and status false.
func refusal(data []byte, what string) error {
	var reply struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(data, &reply) == nil && reply.Status != nil && !*reply.Status {
		return fmt.Errorf("%s: %s", what, reconciler.Redact(reply.Error))
	}
	return nil
}

func (c *Client) SetConfigMulti(ctx context.Context, section string, values map[string]string) error {
	for k, v := range values {
		if err := c.SetConfig(ctx, section, k, v); err != nil {
			return fmt.Errorf("setting %s.%s: %w", section, k, err)
		}
	}
	return nil
}

// DeleteNamed removes a server or category by name.
func (c *Client) DeleteNamed(ctx context.Context, section, name string) error {
	params := c.apiParams("del_config", "")
	params.Set("section", section)
	params.Set("keyword", name)
	return c.write(ctx, params)
}
