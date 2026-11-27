// Command arrchitect writes resources from running apps with its export
// subcommand, for adopting an app that was set up by hand.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/kyleseneker/arrchitect/internal/export"
)

var (
	version = "dev"
	commit  = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "arrchitect:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("no command given")
	}
	switch args[0] {
	case "export":
		return exportCommand(ctx, args[1:], stdout, stderr)
	case "version":
		_, err := fmt.Fprintln(stdout, strings.TrimSpace(version+" "+commit))
		return err
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	}
	usage(stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  arrchitect export <app> --url URL [flags]   print a config and Secret describing an app
  arrchitect version

Apps: %s
Run "arrchitect export -h" for export's flags.
`, strings.Join(export.Apps(), ", "))
}

func exportCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: arrchitect export <%s> --url URL [flags]\n", strings.Join(export.Apps(), "|"))
		fs.PrintDefaults()
	}
	var o export.Options
	fs.StringVar(&o.URL, "url", "", "URL to read the app from")
	fs.StringVar(&o.ConnectionURL, "connection-url", "", "URL the operator should reach the app at, if not --url")
	fs.StringVar(&o.APIKey, "api-key", os.Getenv("ARRCHITECT_API_KEY"),
		"the app's API key (default $ARRCHITECT_API_KEY)")
	fs.StringVar(&o.Username, "username", os.Getenv("ARRCHITECT_USERNAME"),
		"username to sign in with, for apps without an API key (default $ARRCHITECT_USERNAME)")
	fs.StringVar(&o.Password, "password", os.Getenv("ARRCHITECT_PASSWORD"),
		"password to sign in with (default $ARRCHITECT_PASSWORD)")
	fs.StringVar(&o.Token, "token", os.Getenv("ARRCHITECT_TOKEN"),
		"the app's token, for Plex (default $ARRCHITECT_TOKEN)")
	fs.StringVar(&o.Name, "name", "", "name of the config (default: the app)")
	fs.StringVar(&o.Namespace, "namespace", "", "namespace for the config and Secret")
	fs.StringVar(&o.SecretName, "secret", "", "name of the Secret (default: <name>-credentials)")
	exclude := fs.String("exclude", "",
		"comma-separated spec fields to leave out, such as indexers when Prowlarr syncs them")
	fs.BoolVar(&o.InsecureSkipVerify, "insecure-skip-verify", false, "skip TLS certificate verification")
	fs.BoolVar(&o.Enforce, "enforce", false,
		"leave out observe mode, so the operator writes to the app as soon as the config is applied")
	secretFile := fs.String("secret-file", "", "write the Secret to this file instead of with the config")

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return nil
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fs.Usage()
		return errors.New("export needs an app")
	}
	o.App = args[0]
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if o.URL == "" {
		return errors.New("--url is required")
	}
	if o.APIKey == "" && export.NeedsAPIKey(o.App) {
		return errors.New("--api-key or $ARRCHITECT_API_KEY is required")
	}
	if o.Token == "" && export.NeedsToken(o.App) {
		return fmt.Errorf("%s needs --token or $ARRCHITECT_TOKEN", o.App)
	}
	if (o.Username == "" || o.Password == "") && export.NeedsLogin(o.App) &&
		(o.APIKey == "" || !export.TakesAPIKey(o.App)) {
		key := ""
		if export.TakesAPIKey(o.App) {
			key = "--api-key, or "
		}
		return fmt.Errorf("%s needs %s--username and --password, or $ARRCHITECT_USERNAME and $ARRCHITECT_PASSWORD",
			o.App, key)
	}
	if *exclude != "" {
		o.Exclude = strings.Split(*exclude, ",")
	}

	res, err := export.Run(ctx, o)
	if err != nil {
		return err
	}
	out, err := res.YAML()
	if *secretFile != "" {
		var secret []byte
		if secret, err = res.SecretYAML(); err != nil {
			return err
		}
		if err := os.WriteFile(*secretFile, secret, 0o600); err != nil {
			return err
		}
		out, err = res.ConfigYAML()
	}
	if err != nil {
		return err
	}
	if _, err := stdout.Write(out); err != nil {
		return err
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintln(stderr, "warning:", w)
	}
	if res.Secret != nil && len(res.Unfilled) > 0 {
		_, _ = fmt.Fprintf(stderr, "%s doesn't reveal these credentials; add them to Secret %s before applying: %s\n",
			o.App, res.Secret.Name, strings.Join(res.Unfilled, ", "))
	}
	return nil
}
