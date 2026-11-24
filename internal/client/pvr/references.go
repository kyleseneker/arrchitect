package pvr

func hasTagLabels(opts Options) bool {
	for _, item := range opts.DownloadClients {
		if len(item.TagLabels) > 0 {
			return true
		}
	}
	for _, item := range opts.Indexers {
		if len(item.TagLabels) > 0 {
			return true
		}
	}
	for _, item := range opts.Notifications {
		if len(item.TagLabels) > 0 {
			return true
		}
	}
	for _, item := range opts.ImportLists {
		if len(item.TagLabels) > 0 {
			return true
		}
	}
	for _, item := range opts.ReleaseProfiles {
		if len(item.TagLabels) > 0 {
			return true
		}
	}
	for _, item := range opts.RootFolders {
		if len(item.DefaultTagLabels) > 0 {
			return true
		}
	}
	return len(opts.AutoTags) > 0
}
