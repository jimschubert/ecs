package main

import (
	"fmt"
	"os"
	"strings"

	app "github.com/jimschubert/ecs"

	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

var version = ""
var date = ""
var commit = ""
var projectName = ""

var opts struct {
	Cluster string `short:"c" long:"cluster" description:"Cluster name prefix to filter on"`
	// Query string `short:"q" long:"query" description:"AWS Query syntax for filtering clusters"`
	KeyName string `short:"k" long:"key" description:"SSH Key for connecting (work in progress)"`
	Version bool   `short:"v" long:"version" description:"Display version information"`
}

const parseArgs = flags.HelpFlag | flags.PassDoubleDash

func main() {
	parser := flags.NewParser(&opts, parseArgs)
	_, err := parser.Parse()
	if err != nil {
		flagError := err.(*flags.Error)
		if flagError.Type == flags.ErrHelp {
			parser.WriteHelp(os.Stdout)
			return
		}

		if flagError.Type == flags.ErrUnknownFlag {
			_, _ = fmt.Fprintf(os.Stderr, "%s. Please use --help for available options.\n", strings.Replace(flagError.Message, "unknown", "Unknown", 1))
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error parsing command line options: %s\n", err)
		return
	}

	if opts.Version {
		fmt.Printf("%s %s (%s)\n", projectName, version, commit)
		return
	}

	initLogging()

	application := app.App{
		PublicKey: opts.KeyName,
		Cluster:   opts.Cluster,
		// Query: opts.Query,
	}
	err = application.Run(os.Stdout)
	if err != nil {
		log.WithError(err).Errorf("execution failed.")
		return
	}

	// todo: add application specific logic here.
	_, _ = fmt.Fprint(os.Stdout, "Run complete.")
}

func initLogging() {
	logLevel, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		logLevel = "error"
	}
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)
	log.SetOutput(os.Stderr)
}
