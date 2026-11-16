package export

import (
	"reflect"

	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

type prowlarrExport struct {
	*reader
	appProfiles map[float64]string
}

func exportProwlarr(r *reader) (map[string]any, error) {
	r.prefix = "/api/v1"
	var err error
	if r.tags, err = r.names("/tag", "label"); err != nil {
		return nil, err
	}
	e := &prowlarrExport{reader: r}
	if e.appProfiles, err = r.names("/appprofile", "name"); err != nil {
		return nil, err
	}
	spec := map[string]any{}
	return spec, r.resources(spec, []resource{
		{"tags", "/tag", "", tag},
		{"appProfiles", "/appprofile", "", func(item map[string]any, _ map[string]map[string]any) map[string]any {
			return pick(item, "name", "enableRss", "enableAutomaticSearch", "enableInteractiveSearch", "minimumSeeders")
		}},
		{"applications", "/applications", "/applications/schema", e.application},
		{"indexers", "/indexer", "", e.indexer},
		{"proxies", "/indexerproxy", "", e.proxy},
		{"downloadClients", "/downloadclient", "/downloadclient/schema", e.downloadClient},
		{"notifications", "/notification", "/notification/schema", e.notification},
	})
}

func (e *prowlarrExport) application(item map[string]any, defaults map[string]map[string]any) map[string]any {
	a := pick(item, "name", "syncLevel", "implementation", "configContract")
	name, _ := item["name"].(string)
	values := fieldValues(item)
	a["prowlarrUrl"] = values["prowlarrUrl"]
	a["baseUrl"] = values["baseUrl"]
	a["syncCategories"] = values["syncCategories"]
	key, _ := values["apiKey"].(string)
	if reconciler.IsMasked(key) {
		key = ""
	}
	a["apiKeySecretRef"] = e.creds.ref(key, "application", name, "api-key")
	impl, _ := item["implementation"].(string)
	for field, value := range values {
		switch field {
		case "prowlarrUrl", "baseUrl", "syncCategories", "apiKey":
		default:
			if !reflect.DeepEqual(value, defaults[impl][field]) {
				e.warn("application %q: the config has no %s, so it stays as set in Prowlarr", name, field)
			}
		}
	}
	a["tagLabels"] = e.tagLabels(item)
	return a
}

func (e *prowlarrExport) indexer(item map[string]any, _ map[string]map[string]any) map[string]any {
	idx := pick(item, "name", "implementation", "configContract", "enable", "priority")
	if id, ok := item["appProfileId"].(float64); ok {
		idx["appProfileName"] = e.appProfiles[id]
	}
	idx["fields"] = e.providerFields(item, nil, "indexer")
	idx["tagLabels"] = e.tagLabels(item)
	return idx
}

func (e *prowlarrExport) proxy(item map[string]any, _ map[string]map[string]any) map[string]any {
	p := pick(item, "name", "implementation", "configContract")
	name, _ := item["name"].(string)
	for field, value := range fieldValues(item) {
		switch field {
		case "host", "requestTimeout":
			p[field] = value
		default:
			if value != nil && value != "" {
				e.warn("proxy %q: the config's proxies have only host and requestTimeout, so %s stays as set in Prowlarr", name, field)
			}
		}
	}
	p["tagLabels"] = e.tagLabels(item)
	return p
}

func (e *prowlarrExport) downloadClient(item map[string]any, defaults map[string]map[string]any) map[string]any {
	dc := pick(item, "name", "enable", "protocol", "implementation", "configContract", "priority", "categories")
	protocol(dc)
	dc["fields"] = e.providerFields(item, defaults, "download-client")
	dc["tagLabels"] = e.tagLabels(item)
	return dc
}

// providerFields keeps fields that differ from the implementation's defaults.
// Indexers pass no defaults, since Prowlarr's indexer schema lists every
// tracker, so empty values are left out too.
func (e *prowlarrExport) providerFields(item map[string]any, defaults map[string]map[string]any, kind string) []any {
	fields := e.fields(item, defaults, nil, kind, true)
	out := make([]any, 0, len(fields))
	for _, f := range fields {
		switch v := f.(map[string]any)["value"].(type) {
		case string:
			if v == "" {
				continue
			}
		case []any:
			if len(v) == 0 {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}
